package storage

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

var ErrTaskWorkspaceNotFound = errors.New("task workspace not found")

type TaskWorkspaceRecord struct {
	TaskID            string  `json:"taskId"`
	WorkspaceID       string  `json:"workspaceId"`
	RootPath          string  `json:"rootPath"`
	SchemaVersion     int     `json:"schemaVersion"`
	ManifestRevision  int     `json:"manifestRevision"`
	State             string  `json:"state"`
	LegacyContextPath *string `json:"legacyContextPath,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	LastReconciledAt  *string `json:"lastReconciledAt,omitempty"`
	ErrorMessage      *string `json:"errorMessage,omitempty"`
}

type TaskResourceRecord struct {
	ID           string  `json:"id"`
	TaskID       string  `json:"taskId"`
	Kind         string  `json:"kind"`
	SourceType   string  `json:"sourceType"`
	LogicalPath  string  `json:"logicalPath"`
	StoragePath  *string `json:"storagePath,omitempty"`
	ExternalPath *string `json:"externalPath,omitempty"`
	MIMEType     *string `json:"mimeType,omitempty"`
	ByteSize     *int64  `json:"byteSize,omitempty"`
	SHA256       *string `json:"sha256,omitempty"`
	Immutable    bool    `json:"immutable"`
	Readable     bool    `json:"readable"`
	CreatedAt    string  `json:"createdAt"`
	RemovedAt    *string `json:"removedAt,omitempty"`
}

type LegacyTaskMigrationRecord struct {
	TaskID            string   `json:"taskId"`
	SourceRevision    int      `json:"sourceRevision"`
	LegacyPath        string   `json:"legacyPath"`
	TargetWorkspaceID string   `json:"targetWorkspaceId"`
	State             string   `json:"state"`
	Warnings          []string `json:"warnings"`
	StartedAt         string   `json:"startedAt"`
	CompletedAt       *string  `json:"completedAt,omitempty"`
	ErrorMessage      *string  `json:"errorMessage,omitempty"`
}

func (s *SQLiteStore) UpsertTaskWorkspace(record TaskWorkspaceRecord) error {
	if err := validateTaskWorkspaceRecord(record); err != nil {
		return err
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	return withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		return upsertTaskWorkspaceWithConn(ctx, connection, record)
	})
}

func (s *SQLiteStore) TaskWorkspace(taskID string) (TaskWorkspaceRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return TaskWorkspaceRecord{}, fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return TaskWorkspaceRecord{}, err
	}
	return scanTaskWorkspace(database.QueryRow(`
		SELECT task_id, workspace_id, root_path, schema_version,
		       manifest_revision, state, legacy_context_path, created_at,
		       updated_at, last_reconciled_at, error_message
		  FROM task_workspaces
		 WHERE task_id = ?`, taskID))
}

func (s *SQLiteStore) DeleteTaskWorkspace(taskID string) error {
	if !taskIDPattern.MatchString(taskID) {
		return fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	return withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(
			ctx,
			`DELETE FROM task_workspaces WHERE task_id = ?`,
			taskID,
		)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrTaskWorkspaceNotFound
		}
		return nil
	})
}

func (s *SQLiteStore) ReplaceTaskResources(
	taskID string,
	resources []TaskResourceRecord,
) error {
	if !taskIDPattern.MatchString(taskID) {
		return fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	return withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		return replaceTaskResourcesWithConn(ctx, connection, taskID, resources)
	})
}

func (s *SQLiteStore) TaskResources(taskID string) ([]TaskResourceRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return nil, fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return nil, err
	}
	rows, err := database.Query(`
		SELECT id, task_id, kind, source_type, logical_path, storage_path,
		       external_path, mime_type, byte_size, sha256, immutable,
		       readable, created_at, removed_at
		  FROM task_resources
		 WHERE task_id = ? AND removed_at IS NULL
		 ORDER BY logical_path`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resources := make([]TaskResourceRecord, 0)
	for rows.Next() {
		resource, err := scanTaskResource(rows)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, rows.Err()
}

func (s *SQLiteStore) TaskResource(
	taskID string,
	resourceID string,
) (TaskResourceRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return TaskResourceRecord{}, fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return TaskResourceRecord{}, err
	}
	resource, err := scanTaskResource(database.QueryRow(`
		SELECT id, task_id, kind, source_type, logical_path, storage_path,
		       external_path, mime_type, byte_size, sha256, immutable,
		       readable, created_at, removed_at
		  FROM task_resources
		 WHERE task_id = ? AND id = ? AND removed_at IS NULL`, taskID, resourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return TaskResourceRecord{}, ErrTaskWorkspaceNotFound
	}
	return resource, err
}

func (s *SQLiteStore) LegacyTaskMigration(
	taskID string,
) (LegacyTaskMigrationRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return LegacyTaskMigrationRecord{}, fmt.Errorf("invalid task id %q", taskID)
	}
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return LegacyTaskMigrationRecord{}, err
	}
	var record LegacyTaskMigrationRecord
	var warningsJSON string
	var completedAt, errorMessage sql.NullString
	err = database.QueryRow(`
		SELECT task_id, source_revision, legacy_path, target_workspace_id,
		       state, warnings_json, started_at, completed_at, error_message
		  FROM legacy_task_migrations WHERE task_id = ?`, taskID).Scan(
		&record.TaskID,
		&record.SourceRevision,
		&record.LegacyPath,
		&record.TargetWorkspaceID,
		&record.State,
		&warningsJSON,
		&record.StartedAt,
		&completedAt,
		&errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LegacyTaskMigrationRecord{}, ErrTaskWorkspaceNotFound
	}
	if err != nil {
		return LegacyTaskMigrationRecord{}, err
	}
	if err := json.Unmarshal([]byte(warningsJSON), &record.Warnings); err != nil {
		return LegacyTaskMigrationRecord{}, fmt.Errorf("decode legacy migration warnings: %w", err)
	}
	record.CompletedAt = nullableString(completedAt)
	record.ErrorMessage = nullableString(errorMessage)
	return record, nil
}

func upsertTaskWorkspaceWithConn(
	ctx context.Context,
	connection *sql.Conn,
	record TaskWorkspaceRecord,
) error {
	if err := validateTaskWorkspaceRecord(record); err != nil {
		return err
	}
	_, err := connection.ExecContext(ctx, `
		INSERT INTO task_workspaces(
			task_id, workspace_id, root_path, schema_version,
			manifest_revision, state, legacy_context_path, created_at,
			updated_at, last_reconciled_at, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			root_path = excluded.root_path,
			schema_version = excluded.schema_version,
			manifest_revision = excluded.manifest_revision,
			state = excluded.state,
			legacy_context_path = excluded.legacy_context_path,
			updated_at = excluded.updated_at,
			last_reconciled_at = excluded.last_reconciled_at,
			error_message = excluded.error_message`,
		record.TaskID,
		record.WorkspaceID,
		record.RootPath,
		record.SchemaVersion,
		record.ManifestRevision,
		record.State,
		record.LegacyContextPath,
		record.CreatedAt,
		record.UpdatedAt,
		record.LastReconciledAt,
		record.ErrorMessage,
	)
	return err
}

func replaceTaskResourcesWithConn(
	ctx context.Context,
	connection *sql.Conn,
	taskID string,
	resources []TaskResourceRecord,
) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := connection.ExecContext(
		ctx,
		`UPDATE task_resources SET removed_at = ? WHERE task_id = ? AND removed_at IS NULL`,
		now,
		taskID,
	); err != nil {
		return err
	}
	seenIDs := make(map[string]struct{}, len(resources))
	seenPaths := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if resource.TaskID != taskID {
			return errors.New("task resource belongs to a different task")
		}
		if err := validateTaskResourceRecord(resource); err != nil {
			return err
		}
		if _, exists := seenIDs[resource.ID]; exists {
			return fmt.Errorf("duplicate task resource id %q", resource.ID)
		}
		if _, exists := seenPaths[resource.LogicalPath]; exists {
			return fmt.Errorf("duplicate task resource path %q", resource.LogicalPath)
		}
		seenIDs[resource.ID] = struct{}{}
		seenPaths[resource.LogicalPath] = struct{}{}
		result, err := connection.ExecContext(ctx, `
			INSERT INTO task_resources(
				id, task_id, kind, source_type, logical_path, storage_path,
				external_path, mime_type, byte_size, sha256, immutable,
				readable, created_at, removed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
			ON CONFLICT(id) DO UPDATE SET
				kind = excluded.kind,
				source_type = excluded.source_type,
				logical_path = excluded.logical_path,
				storage_path = excluded.storage_path,
				external_path = excluded.external_path,
				mime_type = excluded.mime_type,
				byte_size = excluded.byte_size,
				sha256 = excluded.sha256,
				immutable = excluded.immutable,
				readable = excluded.readable,
				removed_at = NULL
			WHERE task_resources.task_id = excluded.task_id`,
			resource.ID,
			resource.TaskID,
			resource.Kind,
			resource.SourceType,
			resource.LogicalPath,
			resource.StoragePath,
			resource.ExternalPath,
			resource.MIMEType,
			resource.ByteSize,
			resource.SHA256,
			resource.Immutable,
			resource.Readable,
			resource.CreatedAt,
		)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("task resource id %q belongs to a different task", resource.ID)
		}
	}
	return nil
}

func upsertLegacyTaskMigrationWithConn(
	ctx context.Context,
	connection *sql.Conn,
	record LegacyTaskMigrationRecord,
) error {
	warnings, err := json.Marshal(record.Warnings)
	if err != nil {
		return err
	}
	_, err = connection.ExecContext(ctx, `
		INSERT INTO legacy_task_migrations(
			task_id, source_revision, legacy_path, target_workspace_id,
			state, warnings_json, started_at, completed_at, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			source_revision = excluded.source_revision,
			legacy_path = excluded.legacy_path,
			target_workspace_id = excluded.target_workspace_id,
			state = excluded.state,
			warnings_json = excluded.warnings_json,
			completed_at = excluded.completed_at,
			error_message = excluded.error_message`,
		record.TaskID,
		record.SourceRevision,
		record.LegacyPath,
		record.TargetWorkspaceID,
		record.State,
		string(warnings),
		record.StartedAt,
		record.CompletedAt,
		record.ErrorMessage,
	)
	return err
}

func validateTaskWorkspaceRecord(record TaskWorkspaceRecord) error {
	if !taskIDPattern.MatchString(record.TaskID) {
		return fmt.Errorf("invalid task id %q", record.TaskID)
	}
	if strings.TrimSpace(record.WorkspaceID) == "" || strings.TrimSpace(record.RootPath) == "" {
		return errors.New("task workspace identity and root are required")
	}
	if !filepath.IsAbs(record.RootPath) || filepath.Clean(record.RootPath) != record.RootPath {
		return errors.New("task workspace root must be an absolute canonical path")
	}
	if filepath.Base(record.RootPath) != record.TaskID {
		return errors.New("task workspace root must end with its task id")
	}
	if record.LegacyContextPath != nil &&
		(!filepath.IsAbs(*record.LegacyContextPath) || filepath.Clean(*record.LegacyContextPath) != *record.LegacyContextPath) {
		return errors.New("legacy context path must be an absolute canonical path")
	}
	if record.LegacyContextPath != nil {
		relative, err := filepath.Rel(record.RootPath, *record.LegacyContextPath)
		if err != nil || !isNestedRelativePath(relative) {
			return errors.New("legacy context path must stay inside its task workspace")
		}
	}
	if record.SchemaVersion < 1 || record.ManifestRevision < 1 {
		return errors.New("task workspace schema and manifest revisions must be positive")
	}
	if record.CreatedAt == "" || record.UpdatedAt == "" {
		return errors.New("task workspace timestamps are required")
	}
	return nil
}

func validateTaskResourceRecord(record TaskResourceRecord) error {
	if !taskIDPattern.MatchString(record.TaskID) {
		return fmt.Errorf("invalid task id %q", record.TaskID)
	}
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.LogicalPath) == "" {
		return errors.New("task resource identity and logical path are required")
	}
	logicalPath, err := taskspace.ValidateLogicalPath(record.LogicalPath)
	if err != nil || logicalPath == "." {
		return errors.New("task resource logical path must be canonical")
	}
	if record.StoragePath != nil {
		storagePath, err := taskspace.ValidateLogicalPath(*record.StoragePath)
		if err != nil || storagePath == "." {
			return errors.New("task resource storage path must be canonical")
		}
	}
	if record.ExternalPath != nil &&
		(!filepath.IsAbs(*record.ExternalPath) || filepath.Clean(*record.ExternalPath) != *record.ExternalPath) {
		return errors.New("task resource external path must be an absolute canonical path")
	}
	if record.CreatedAt == "" {
		return errors.New("task resource created timestamp is required")
	}
	if record.ByteSize != nil && *record.ByteSize < 0 {
		return errors.New("task resource byte size must not be negative")
	}
	if record.SHA256 != nil && !validSHA256(*record.SHA256) {
		return errors.New("task resource hash must be a SHA-256 hex value")
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTaskWorkspace(scanner rowScanner) (TaskWorkspaceRecord, error) {
	var record TaskWorkspaceRecord
	var legacyContextPath sql.NullString
	var lastReconciledAt sql.NullString
	var errorMessage sql.NullString
	err := scanner.Scan(
		&record.TaskID,
		&record.WorkspaceID,
		&record.RootPath,
		&record.SchemaVersion,
		&record.ManifestRevision,
		&record.State,
		&legacyContextPath,
		&record.CreatedAt,
		&record.UpdatedAt,
		&lastReconciledAt,
		&errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskWorkspaceRecord{}, ErrTaskWorkspaceNotFound
	}
	if err != nil {
		return TaskWorkspaceRecord{}, err
	}
	record.LegacyContextPath = nullableString(legacyContextPath)
	record.LastReconciledAt = nullableString(lastReconciledAt)
	record.ErrorMessage = nullableString(errorMessage)
	return record, nil
}

func scanTaskResource(scanner rowScanner) (TaskResourceRecord, error) {
	var record TaskResourceRecord
	var storagePath sql.NullString
	var externalPath sql.NullString
	var mimeType sql.NullString
	var byteSize sql.NullInt64
	var sha256Value sql.NullString
	var removedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.Kind,
		&record.SourceType,
		&record.LogicalPath,
		&storagePath,
		&externalPath,
		&mimeType,
		&byteSize,
		&sha256Value,
		&record.Immutable,
		&record.Readable,
		&record.CreatedAt,
		&removedAt,
	)
	if err != nil {
		return TaskResourceRecord{}, err
	}
	record.StoragePath = nullableString(storagePath)
	record.ExternalPath = nullableString(externalPath)
	record.MIMEType = nullableString(mimeType)
	if byteSize.Valid {
		value := byteSize.Int64
		record.ByteSize = &value
	}
	record.SHA256 = nullableString(sha256Value)
	record.RemovedAt = nullableString(removedAt)
	return record, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
