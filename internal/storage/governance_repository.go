package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

type GitBindingRecord struct {
	ID                string  `json:"id"`
	TaskID            string  `json:"taskId"`
	SourcePath        string  `json:"sourcePath"`
	SourceRealPath    string  `json:"sourceRealPath"`
	CommonGitDir      string  `json:"commonGitDir"`
	WorktreePath      *string `json:"worktreePath,omitempty"`
	Branch            *string `json:"branch,omitempty"`
	BaselineCommit    string  `json:"baselineCommit"`
	SourceBranch      *string `json:"sourceBranch,omitempty"`
	SourceDirtyAtBind bool    `json:"sourceDirtyAtBind"`
	State             string  `json:"state"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	ErrorMessage      *string `json:"errorMessage,omitempty"`
}

type PermissionRequestRecord struct {
	ID               string  `json:"id"`
	TaskID           string  `json:"taskId"`
	SessionID        string  `json:"sessionId"`
	RunID            string  `json:"runId"`
	ToolCallID       string  `json:"toolCallId"`
	Capability       string  `json:"capability"`
	Target           string  `json:"target"`
	NormalizedTarget *string `json:"normalizedTarget,omitempty"`
	Subject          string  `json:"subject"`
	RiskLevel        string  `json:"riskLevel"`
	State            string  `json:"state"`
	RequestedAt      string  `json:"requestedAt"`
	ResolvedAt       *string `json:"resolvedAt,omitempty"`
	ResolvedBy       *string `json:"resolvedBy,omitempty"`
	DecisionScope    *string `json:"decisionScope,omitempty"`
	Reason           *string `json:"reason,omitempty"`
}

type PermissionGrantRecord struct {
	ID            string  `json:"id"`
	TaskID        *string `json:"taskId,omitempty"`
	SessionID     *string `json:"sessionId,omitempty"`
	RequestID     *string `json:"requestId,omitempty"`
	Capability    string  `json:"capability"`
	TargetPattern string  `json:"targetPattern"`
	Scope         string  `json:"scope"`
	Decision      string  `json:"decision"`
	RiskCeiling   string  `json:"riskCeiling"`
	CreatedAt     string  `json:"createdAt"`
	ExpiresAt     *string `json:"expiresAt,omitempty"`
	ConsumedAt    *string `json:"consumedAt,omitempty"`
	RevokedAt     *string `json:"revokedAt,omitempty"`
	CreatedBy     string  `json:"createdBy"`
}

type WorkspaceArtifactRecord struct {
	ID          string  `json:"id"`
	TaskID      string  `json:"taskId"`
	SessionID   *string `json:"sessionId,omitempty"`
	RunID       *string `json:"runId,omitempty"`
	LogicalPath string  `json:"logicalPath"`
	Kind        string  `json:"kind"`
	MIMEType    *string `json:"mimeType,omitempty"`
	ByteSize    int64   `json:"byteSize"`
	SHA256      string  `json:"sha256"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	DeletedAt   *string `json:"deletedAt,omitempty"`
}

func (s *SQLiteStore) UpsertGitBinding(record GitBindingRecord) error {
	if err := validateGitBindingRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO git_bindings(
				id, task_id, source_path, source_real_path, common_git_dir,
				worktree_path, branch, baseline_commit, source_branch,
				source_dirty_at_bind, state, created_at, updated_at, error_message
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				source_path = excluded.source_path,
				source_real_path = excluded.source_real_path,
				common_git_dir = excluded.common_git_dir,
				worktree_path = excluded.worktree_path,
				branch = excluded.branch,
				baseline_commit = excluded.baseline_commit,
				source_branch = excluded.source_branch,
				source_dirty_at_bind = excluded.source_dirty_at_bind,
				state = excluded.state,
				updated_at = excluded.updated_at,
				error_message = excluded.error_message
			WHERE git_bindings.task_id = excluded.task_id`,
			record.ID,
			record.TaskID,
			record.SourcePath,
			record.SourceRealPath,
			record.CommonGitDir,
			record.WorktreePath,
			record.Branch,
			record.BaselineCommit,
			record.SourceBranch,
			record.SourceDirtyAtBind,
			record.State,
			record.CreatedAt,
			record.UpdatedAt,
			record.ErrorMessage,
		)
		return requireScopedWrite(result, err, "git binding")
	})
}

func (s *SQLiteStore) GitBinding(taskID string) (GitBindingRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return GitBindingRecord{}, fmt.Errorf("invalid task id %q", taskID)
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return GitBindingRecord{}, err
	}
	defer unlock()
	return scanGitBinding(database.QueryRow(`
		SELECT id, task_id, source_path, source_real_path, common_git_dir,
		       worktree_path, branch, baseline_commit, source_branch,
		       source_dirty_at_bind, state, created_at, updated_at, error_message
		  FROM git_bindings WHERE task_id = ?`, taskID))
}

func (s *SQLiteStore) UpsertPermissionRequest(record PermissionRequestRecord) error {
	if err := validatePermissionRequestRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO permission_requests(
				id, task_id, session_id, run_id, tool_call_id, capability,
				target, normalized_target, subject, risk_level, state,
				requested_at, resolved_at, resolved_by, decision_scope, reason
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				normalized_target = excluded.normalized_target,
				state = excluded.state,
				resolved_at = excluded.resolved_at,
				resolved_by = excluded.resolved_by,
				decision_scope = excluded.decision_scope,
				reason = excluded.reason
			WHERE permission_requests.task_id = excluded.task_id
			  AND permission_requests.session_id = excluded.session_id
			  AND permission_requests.run_id = excluded.run_id
			  AND permission_requests.tool_call_id = excluded.tool_call_id
			  AND permission_requests.capability = excluded.capability
			  AND permission_requests.target = excluded.target`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.ToolCallID,
			record.Capability,
			record.Target,
			record.NormalizedTarget,
			record.Subject,
			record.RiskLevel,
			record.State,
			record.RequestedAt,
			record.ResolvedAt,
			record.ResolvedBy,
			record.DecisionScope,
			record.Reason,
		)
		return requireScopedWrite(result, err, "permission request")
	})
}

func (s *SQLiteStore) PermissionRequest(
	taskID string,
	requestID string,
) (PermissionRequestRecord, error) {
	if err := validateScopedID(taskID, requestID, "permission request"); err != nil {
		return PermissionRequestRecord{}, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return PermissionRequestRecord{}, err
	}
	defer unlock()
	return scanPermissionRequest(database.QueryRow(`
		SELECT id, task_id, session_id, run_id, tool_call_id, capability,
		       target, normalized_target, subject, risk_level, state,
		       requested_at, resolved_at, resolved_by, decision_scope, reason
		  FROM permission_requests WHERE task_id = ? AND id = ?`, taskID, requestID))
}

func (s *SQLiteStore) UpsertPermissionGrant(record PermissionGrantRecord) error {
	if err := validatePermissionGrantRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO permission_grants(
				id, task_id, session_id, request_id, capability, target_pattern,
				scope, decision, risk_ceiling, created_at, expires_at,
				consumed_at, revoked_at, created_by
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				expires_at = excluded.expires_at,
				consumed_at = excluded.consumed_at,
				revoked_at = excluded.revoked_at
			WHERE permission_grants.task_id IS excluded.task_id
			  AND permission_grants.session_id IS excluded.session_id
			  AND permission_grants.request_id IS excluded.request_id
			  AND permission_grants.capability = excluded.capability
			  AND permission_grants.target_pattern = excluded.target_pattern
			  AND permission_grants.scope = excluded.scope
			  AND permission_grants.decision = excluded.decision
			  AND permission_grants.risk_ceiling = excluded.risk_ceiling`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RequestID,
			record.Capability,
			record.TargetPattern,
			record.Scope,
			record.Decision,
			record.RiskCeiling,
			record.CreatedAt,
			record.ExpiresAt,
			record.ConsumedAt,
			record.RevokedAt,
			record.CreatedBy,
		)
		return requireScopedWrite(result, err, "permission grant")
	})
}

func (s *SQLiteStore) PermissionGrant(
	taskID *string,
	grantID string,
) (PermissionGrantRecord, error) {
	if strings.TrimSpace(grantID) == "" {
		return PermissionGrantRecord{}, errors.New("permission grant id is required")
	}
	var taskValue any
	if taskID != nil {
		if !taskIDPattern.MatchString(*taskID) {
			return PermissionGrantRecord{}, fmt.Errorf("invalid task id %q", *taskID)
		}
		taskValue = *taskID
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return PermissionGrantRecord{}, err
	}
	defer unlock()
	return scanPermissionGrant(database.QueryRow(`
		SELECT id, task_id, session_id, request_id, capability, target_pattern,
		       scope, decision, risk_ceiling, created_at, expires_at,
		       consumed_at, revoked_at, created_by
		  FROM permission_grants
		 WHERE id = ? AND task_id IS ?`, grantID, taskValue))
}

func (s *SQLiteStore) UpsertWorkspaceArtifact(record WorkspaceArtifactRecord) error {
	if err := validateWorkspaceArtifactRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO workspace_artifacts(
				id, task_id, session_id, run_id, logical_path, kind, mime_type,
				byte_size, sha256, created_at, updated_at, deleted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				session_id = excluded.session_id,
				run_id = excluded.run_id,
				mime_type = excluded.mime_type,
				byte_size = excluded.byte_size,
				sha256 = excluded.sha256,
				updated_at = excluded.updated_at,
				deleted_at = excluded.deleted_at
			WHERE workspace_artifacts.task_id = excluded.task_id
			  AND workspace_artifacts.logical_path = excluded.logical_path
			  AND workspace_artifacts.kind = excluded.kind`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.LogicalPath,
			record.Kind,
			record.MIMEType,
			record.ByteSize,
			record.SHA256,
			record.CreatedAt,
			record.UpdatedAt,
			record.DeletedAt,
		)
		return requireScopedWrite(result, err, "workspace artifact")
	})
}

func (s *SQLiteStore) WorkspaceArtifacts(taskID string) ([]WorkspaceArtifactRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return nil, fmt.Errorf("invalid task id %q", taskID)
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := database.Query(`
		SELECT id, task_id, session_id, run_id, logical_path, kind, mime_type,
		       byte_size, sha256, created_at, updated_at, deleted_at
		  FROM workspace_artifacts
		 WHERE task_id = ? AND deleted_at IS NULL
		 ORDER BY logical_path`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]WorkspaceArtifactRecord, 0)
	for rows.Next() {
		record, err := scanWorkspaceArtifact(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func validateGitBindingRecord(record GitBindingRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "git binding"); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"source path":     record.SourcePath,
		"source realpath": record.SourceRealPath,
		"common git dir":  record.CommonGitDir,
		"baseline commit": record.BaselineCommit,
		"state":           record.State,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("git binding %s is required", label)
		}
	}
	for label, value := range map[string]string{
		"source path":     record.SourcePath,
		"source realpath": record.SourceRealPath,
		"common git dir":  record.CommonGitDir,
	} {
		if err := validateAbsoluteCleanPath(value); err != nil {
			return fmt.Errorf("git binding %s: %w", label, err)
		}
	}
	if record.WorktreePath != nil {
		if err := validateAbsoluteCleanPath(*record.WorktreePath); err != nil {
			return fmt.Errorf("git binding worktree path: %w", err)
		}
	}
	if record.CreatedAt == "" || record.UpdatedAt == "" {
		return errors.New("git binding timestamps are required")
	}
	return nil
}

func validateAbsoluteCleanPath(value string) error {
	if strings.ContainsRune(value, '\x00') || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return errors.New("path must be an absolute canonical path")
	}
	return nil
}

func validatePermissionRequestRecord(record PermissionRequestRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "permission request"); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"session":    record.SessionID,
		"run":        record.RunID,
		"tool call":  record.ToolCallID,
		"capability": record.Capability,
		"target":     record.Target,
		"subject":    record.Subject,
		"risk level": record.RiskLevel,
		"state":      record.State,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("permission request %s is required", label)
		}
	}
	if record.RequestedAt == "" {
		return errors.New("permission request timestamp is required")
	}
	return nil
}

func validatePermissionGrantRecord(record PermissionGrantRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("permission grant id is required")
	}
	for label, value := range map[string]string{
		"capability":     record.Capability,
		"target pattern": record.TargetPattern,
		"scope":          record.Scope,
		"decision":       record.Decision,
		"risk ceiling":   record.RiskCeiling,
		"created by":     record.CreatedBy,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("permission grant %s is required", label)
		}
	}
	if record.CreatedAt == "" {
		return errors.New("permission grant creation timestamp is required")
	}
	if record.TaskID != nil && !taskIDPattern.MatchString(*record.TaskID) {
		return fmt.Errorf("invalid task id %q", *record.TaskID)
	}
	switch record.Scope {
	case "permanent":
		if record.TaskID != nil || record.SessionID != nil || record.RequestID != nil {
			return errors.New("permanent permission grant must not have task, session, or request scope")
		}
	case "task":
		if record.TaskID == nil || record.SessionID != nil {
			return errors.New("task permission grant requires only a task id")
		}
	case "session":
		if record.TaskID == nil || record.SessionID == nil {
			return errors.New("session permission grant requires task and session ids")
		}
	case "once":
		if record.TaskID == nil || record.SessionID == nil || record.RequestID == nil {
			return errors.New("once permission grant requires task, session, and request ids")
		}
	default:
		return fmt.Errorf("unsupported permission grant scope %q", record.Scope)
	}
	return nil
}

func validateWorkspaceArtifactRecord(record WorkspaceArtifactRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "workspace artifact"); err != nil {
		return err
	}
	logicalPath, err := taskspace.ValidateLogicalPath(record.LogicalPath)
	if err != nil || logicalPath == "." || !strings.HasPrefix(logicalPath, "artifacts/") {
		return errors.New("workspace artifact path must be a canonical path under artifacts")
	}
	if strings.TrimSpace(record.Kind) == "" || !validSHA256(record.SHA256) {
		return errors.New("workspace artifact kind and valid SHA-256 hash are required")
	}
	if record.ByteSize < 0 {
		return errors.New("workspace artifact byte size must not be negative")
	}
	if record.CreatedAt == "" || record.UpdatedAt == "" {
		return errors.New("workspace artifact timestamps are required")
	}
	return nil
}

func scanGitBinding(scanner rowScanner) (GitBindingRecord, error) {
	var record GitBindingRecord
	var worktreePath, branch, sourceBranch, errorMessage sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.SourcePath,
		&record.SourceRealPath,
		&record.CommonGitDir,
		&worktreePath,
		&branch,
		&record.BaselineCommit,
		&sourceBranch,
		&record.SourceDirtyAtBind,
		&record.State,
		&record.CreatedAt,
		&record.UpdatedAt,
		&errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return GitBindingRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return GitBindingRecord{}, err
	}
	record.WorktreePath = nullableString(worktreePath)
	record.Branch = nullableString(branch)
	record.SourceBranch = nullableString(sourceBranch)
	record.ErrorMessage = nullableString(errorMessage)
	return record, nil
}

func scanPermissionRequest(scanner rowScanner) (PermissionRequestRecord, error) {
	var record PermissionRequestRecord
	var normalizedTarget, resolvedAt, resolvedBy, decisionScope, reason sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.SessionID,
		&record.RunID,
		&record.ToolCallID,
		&record.Capability,
		&record.Target,
		&normalizedTarget,
		&record.Subject,
		&record.RiskLevel,
		&record.State,
		&record.RequestedAt,
		&resolvedAt,
		&resolvedBy,
		&decisionScope,
		&reason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PermissionRequestRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return PermissionRequestRecord{}, err
	}
	record.NormalizedTarget = nullableString(normalizedTarget)
	record.ResolvedAt = nullableString(resolvedAt)
	record.ResolvedBy = nullableString(resolvedBy)
	record.DecisionScope = nullableString(decisionScope)
	record.Reason = nullableString(reason)
	return record, nil
}

func scanPermissionGrant(scanner rowScanner) (PermissionGrantRecord, error) {
	var record PermissionGrantRecord
	var taskID, sessionID, requestID sql.NullString
	var expiresAt, consumedAt, revokedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&taskID,
		&sessionID,
		&requestID,
		&record.Capability,
		&record.TargetPattern,
		&record.Scope,
		&record.Decision,
		&record.RiskCeiling,
		&record.CreatedAt,
		&expiresAt,
		&consumedAt,
		&revokedAt,
		&record.CreatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PermissionGrantRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return PermissionGrantRecord{}, err
	}
	record.TaskID = nullableString(taskID)
	record.SessionID = nullableString(sessionID)
	record.RequestID = nullableString(requestID)
	record.ExpiresAt = nullableString(expiresAt)
	record.ConsumedAt = nullableString(consumedAt)
	record.RevokedAt = nullableString(revokedAt)
	return record, nil
}

func scanWorkspaceArtifact(scanner rowScanner) (WorkspaceArtifactRecord, error) {
	var record WorkspaceArtifactRecord
	var sessionID, runID, mimeType, deletedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&sessionID,
		&runID,
		&record.LogicalPath,
		&record.Kind,
		&mimeType,
		&record.ByteSize,
		&record.SHA256,
		&record.CreatedAt,
		&record.UpdatedAt,
		&deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceArtifactRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return WorkspaceArtifactRecord{}, err
	}
	record.SessionID = nullableString(sessionID)
	record.RunID = nullableString(runID)
	record.MIMEType = nullableString(mimeType)
	record.DeletedAt = nullableString(deletedAt)
	return record, nil
}
