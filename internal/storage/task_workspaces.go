package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

func syncTaskWorkspacesWithConn(
	ctx context.Context,
	connection *sql.Conn,
	rootPath string,
	snapshots map[string]taskspace.TaskSnapshot,
	strict bool,
) error {
	taskIDs := make([]string, 0, len(snapshots))
	for taskID := range snapshots {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	service := taskspace.Service{}
	for _, taskID := range taskIDs {
		result, err := service.Ensure(rootPath, snapshots[taskID])
		if err != nil {
			if strict {
				return fmt.Errorf("ensure task workspace %q: %w", taskID, err)
			}
			if persistErr := persistTaskWorkspaceFailure(
				ctx,
				connection,
				rootPath,
				snapshots[taskID],
				err,
			); persistErr != nil {
				return fmt.Errorf("record task workspace %q failure: %w", taskID, persistErr)
			}
			continue
		}
		if err := persistTaskWorkspaceResult(ctx, connection, snapshots[taskID], result); err != nil {
			return fmt.Errorf("persist task workspace %q: %w", taskID, err)
		}
	}
	return nil
}

func persistTaskWorkspaceFailure(
	ctx context.Context,
	connection *sql.Conn,
	rootPath string,
	snapshot taskspace.TaskSnapshot,
	workspaceErr error,
) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rootPath = filepath.Join(rootPath, snapshot.ID)
	workspaceID := failedWorkspaceID(snapshot.ID, rootPath)
	createdAt := now
	manifestRevision := 1
	existing, err := scanTaskWorkspace(connection.QueryRowContext(ctx, `
		SELECT task_id, workspace_id, root_path, schema_version,
		       manifest_revision, state, legacy_context_path, created_at,
		       updated_at, last_reconciled_at, error_message
		  FROM task_workspaces WHERE task_id = ?`, snapshot.ID))
	if err == nil {
		workspaceID = existing.WorkspaceID
		createdAt = existing.CreatedAt
		manifestRevision = existing.ManifestRevision
	} else if !errors.Is(err, ErrTaskWorkspaceNotFound) {
		return err
	}
	message := strings.TrimSpace(workspaceErr.Error())
	if len(message) > 1000 {
		message = message[:1000] + "…"
	}
	legacyContextPath := filepath.Join(rootPath, taskContextFilename)
	if _, err := filepath.EvalSymlinks(legacyContextPath); err != nil {
		legacyContextPath = ""
	}
	if err := upsertTaskWorkspaceWithConn(ctx, connection, TaskWorkspaceRecord{
		TaskID:            snapshot.ID,
		WorkspaceID:       workspaceID,
		RootPath:          rootPath,
		SchemaVersion:     taskspace.SchemaVersion,
		ManifestRevision:  manifestRevision,
		State:             "error",
		LegacyContextPath: stringPointer(legacyContextPath),
		CreatedAt:         createdAt,
		UpdatedAt:         now,
		LastReconciledAt:  &now,
		ErrorMessage:      &message,
	}); err != nil {
		return err
	}
	return upsertLegacyTaskMigrationWithConn(ctx, connection, LegacyTaskMigrationRecord{
		TaskID:            snapshot.ID,
		SourceRevision:    snapshot.Revision,
		LegacyPath:        rootPath,
		TargetWorkspaceID: workspaceID,
		State:             "failed",
		Warnings:          []string{},
		StartedAt:         now,
		ErrorMessage:      &message,
	})
}

func failedWorkspaceID(taskID string, rootPath string) string {
	hash := sha256.Sum256([]byte(taskID + "\x00" + rootPath))
	return "workspace_error_" + hex.EncodeToString(hash[:16])
}

func persistTaskWorkspaceResult(
	ctx context.Context,
	connection *sql.Conn,
	snapshot taskspace.TaskSnapshot,
	result taskspace.Result,
) error {
	legacyContextPath := stringPointer(result.LegacyContextPath)
	lastReconciledAt := stringPointer(result.MigrationCompletedAt)
	state := "ready"
	if snapshot.Archived {
		state = "archived"
	}
	if err := upsertTaskWorkspaceWithConn(ctx, connection, TaskWorkspaceRecord{
		TaskID:            result.TaskID,
		WorkspaceID:       result.WorkspaceID,
		RootPath:          result.RootPath,
		SchemaVersion:     result.SchemaVersion,
		ManifestRevision:  result.ManifestRevision,
		State:             state,
		LegacyContextPath: legacyContextPath,
		CreatedAt:         result.CreatedAt,
		UpdatedAt:         result.UpdatedAt,
		LastReconciledAt:  lastReconciledAt,
	}); err != nil {
		return err
	}
	resources := make([]TaskResourceRecord, 0, len(result.Resources))
	for _, resource := range result.Resources {
		storagePath := stringPointer(resource.StoragePath)
		mimeType := stringPointer(resource.MIMEType)
		sha256Value := stringPointer(resource.SHA256)
		byteSize := resource.ByteSize
		resources = append(resources, TaskResourceRecord{
			ID:          resource.ID,
			TaskID:      resource.TaskID,
			Kind:        resource.Kind,
			SourceType:  resource.SourceType,
			LogicalPath: resource.LogicalPath,
			StoragePath: storagePath,
			MIMEType:    mimeType,
			ByteSize:    &byteSize,
			SHA256:      sha256Value,
			Immutable:   resource.Immutable,
			Readable:    resource.Readable,
			CreatedAt:   resource.CreatedAt,
		})
	}
	if err := replaceTaskResourcesWithConn(ctx, connection, result.TaskID, resources); err != nil {
		return err
	}
	completedAt := stringPointer(result.MigrationCompletedAt)
	legacyPath := result.LegacyContextPath
	if legacyPath == "" {
		legacyPath = result.RootPath
	}
	return upsertLegacyTaskMigrationWithConn(ctx, connection, LegacyTaskMigrationRecord{
		TaskID:            result.TaskID,
		SourceRevision:    snapshot.Revision,
		LegacyPath:        legacyPath,
		TargetWorkspaceID: result.WorkspaceID,
		State:             "completed",
		Warnings:          result.Warnings,
		StartedAt:         result.MigrationStartedAt,
		CompletedAt:       completedAt,
	})
}

func (s *SQLiteStore) EnsureTaskWorkspace(taskID string) (TaskWorkspaceRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return TaskWorkspaceRecord{}, fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return TaskWorkspaceRecord{}, err
	}
	var record TaskWorkspaceRecord
	var ensureErr error
	err = withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		var payload string
		if err := connection.QueryRowContext(
			ctx,
			`SELECT payload FROM workspace_state WHERE id = 1`,
		).Scan(&payload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("workspace state is empty")
			}
			return err
		}
		snapshots, err := decodeTaskSnapshots(payload)
		if err != nil {
			return err
		}
		snapshot, exists := snapshots[taskID]
		if !exists {
			return ErrTaskWorkspaceNotFound
		}
		root, err := s.taskContextRootWithConn(ctx, connection)
		if err != nil {
			return err
		}
		contexts, err := decodeTaskContexts(payload)
		if err != nil {
			return err
		}
		if err := syncTaskContexts(root, map[string][]byte{taskID: contexts[taskID]}); err != nil {
			return err
		}
		result, err := (taskspace.Service{}).Ensure(root, snapshot)
		if err != nil {
			ensureErr = err
			return persistTaskWorkspaceFailure(ctx, connection, root, snapshot, err)
		}
		if err := persistTaskWorkspaceResult(ctx, connection, snapshot, result); err != nil {
			return err
		}
		record, err = scanTaskWorkspace(connection.QueryRowContext(ctx, `
			SELECT task_id, workspace_id, root_path, schema_version,
			       manifest_revision, state, legacy_context_path, created_at,
			       updated_at, last_reconciled_at, error_message
			  FROM task_workspaces WHERE task_id = ?`, taskID))
		return err
	})
	if err != nil {
		return TaskWorkspaceRecord{}, err
	}
	if ensureErr != nil {
		return TaskWorkspaceRecord{}, ensureErr
	}
	return record, nil
}

func (s *SQLiteStore) TaskRevision(taskID string) (int, error) {
	if !taskIDPattern.MatchString(taskID) {
		return 0, fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return 0, err
	}
	var payload string
	if err := database.QueryRow(`SELECT payload FROM workspace_state WHERE id = 1`).Scan(&payload); err != nil {
		return 0, err
	}
	snapshots, err := decodeTaskSnapshots(payload)
	if err != nil {
		return 0, err
	}
	snapshot, exists := snapshots[taskID]
	if !exists {
		return 0, ErrTaskWorkspaceNotFound
	}
	if snapshot.Revision < 0 {
		return 0, errors.New("task revision must not be negative")
	}
	return snapshot.Revision, nil
}

func (s *SQLiteStore) ListTaskWorkspaceFiles(
	taskID string,
	logicalPath string,
) ([]taskspace.WorkspaceEntry, error) {
	record, err := s.TaskWorkspace(taskID)
	if err != nil {
		return nil, err
	}
	return (taskspace.Service{}).List(filepath.Dir(record.RootPath), taskID, logicalPath)
}

func (s *SQLiteStore) ReadTaskWorkspaceFile(
	taskID string,
	logicalPath string,
) (taskspace.FilePreview, error) {
	record, err := s.TaskWorkspace(taskID)
	if err != nil {
		return taskspace.FilePreview{}, err
	}
	return (taskspace.Service{}).Read(filepath.Dir(record.RootPath), taskID, logicalPath)
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
