package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type AgentReferenceRecord struct {
	TaskID      string  `json:"taskId"`
	SessionID   string  `json:"sessionId"`
	MessageID   string  `json:"messageId"`
	ResourceID  string  `json:"resourceId"`
	TargetType  string  `json:"targetType"`
	Method      string  `json:"method"`
	Position    int     `json:"position"`
	CreatedAt   string  `json:"createdAt"`
	Kind        string  `json:"kind"`
	SourceType  *string `json:"sourceType,omitempty"`
	LogicalPath string  `json:"logicalPath"`
	MIMEType    *string `json:"mimeType,omitempty"`
	ByteSize    *int64  `json:"byteSize,omitempty"`
	Immutable   bool    `json:"immutable"`
}

type RequirementProposalRecord struct {
	TaskID           string  `json:"taskId"`
	ArtifactID       string  `json:"artifactId"`
	BaseRevision     int     `json:"baseRevision"`
	State            string  `json:"state"`
	AcceptedRevision *int    `json:"acceptedRevision,omitempty"`
	CreatedAt        string  `json:"createdAt"`
	ResolvedAt       *string `json:"resolvedAt,omitempty"`
}

func (s *SQLiteStore) UpsertAgentMessageWithReferences(
	record AgentMessageRecord,
	session AgentSessionRecord,
	references []AgentReferenceRecord,
) error {
	if err := validateAgentMessageRecord(record); err != nil {
		return err
	}
	if err := validateAgentSessionRecord(session); err != nil {
		return err
	}
	if record.TaskID != session.TaskID || record.SessionID != session.ID || record.Sequence != session.LastSequence {
		return errors.New("agent message and session sequence do not match")
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		if _, err := connection.ExecContext(ctx, `
			INSERT INTO agent_messages(
				id, task_id, session_id, run_id, role, kind, status, content,
				content_ref, sequence, pi_entry_id, created_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.ID, record.TaskID, record.SessionID, record.RunID, record.Role,
			record.Kind, record.Status, record.Content, record.ContentRef,
			record.Sequence, record.PIEntryID, record.CreatedAt, record.CompletedAt,
		); err != nil {
			return err
		}
		for index := range references {
			reference := references[index]
			reference.TaskID = record.TaskID
			reference.SessionID = record.SessionID
			reference.MessageID = record.ID
			reference.Position = index
			if reference.CreatedAt == "" {
				reference.CreatedAt = record.CreatedAt
			}
			if err := insertAgentReference(ctx, connection, reference); err != nil {
				return err
			}
		}
		result, err := connection.ExecContext(ctx, `
			UPDATE agent_sessions
			   SET last_sequence = ?, updated_at = ?, last_active_at = ?
			 WHERE task_id = ? AND id = ?`,
			session.LastSequence, session.UpdatedAt, session.LastActiveAt,
			session.TaskID, session.ID,
		)
		return requireScopedWrite(result, err, "agent session")
	})
}

func (s *SQLiteStore) AddAgentMessageReference(reference AgentReferenceRecord) error {
	if err := validateAgentReference(reference); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		return insertAgentReference(ctx, connection, reference)
	})
}

func insertAgentReference(
	ctx context.Context,
	connection *sql.Conn,
	reference AgentReferenceRecord,
) error {
	if err := validateAgentReference(reference); err != nil {
		return err
	}
	var exists int
	switch reference.TargetType {
	case "resource":
		err := connection.QueryRowContext(ctx, `
			SELECT 1 FROM task_resources
			 WHERE task_id = ? AND id = ? AND removed_at IS NULL`,
			reference.TaskID, reference.ResourceID,
		).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskWorkspaceNotFound
		}
		if err != nil {
			return err
		}
	case "artifact":
		err := connection.QueryRowContext(ctx, `
			SELECT 1 FROM workspace_artifacts
			 WHERE task_id = ? AND id = ? AND deleted_at IS NULL`,
			reference.TaskID, reference.ResourceID,
		).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAgentDataNotFound
		}
		if err != nil {
			return err
		}
	}
	if reference.Method == "attachment" {
		if reference.TargetType != "resource" {
			return errors.New("message attachments must target a task resource")
		}
		if _, err := connection.ExecContext(ctx, `
			INSERT INTO message_attachments(
				task_id, session_id, message_id, resource_id, position, created_at
			) VALUES (?, ?, ?, ?, ?, ?)`,
			reference.TaskID, reference.SessionID, reference.MessageID,
			reference.ResourceID, reference.Position, reference.CreatedAt,
		); err != nil {
			return err
		}
	}
	_, err := connection.ExecContext(ctx, `
		INSERT INTO resource_references(
			task_id, session_id, message_id, resource_id, target_type,
			method, position, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		reference.TaskID, reference.SessionID, reference.MessageID,
		reference.ResourceID, reference.TargetType, reference.Method,
		reference.Position, reference.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) AgentMessageReferences(
	taskID string,
	sessionID string,
	messageID string,
) ([]AgentReferenceRecord, error) {
	if err := validateScopedID(taskID, sessionID, "agent session"); err != nil {
		return nil, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return agentReferencesWithDB(database, taskID, sessionID, messageID)
}

func agentReferencesWithDB(
	database *sql.DB,
	taskID string,
	sessionID string,
	messageID string,
) ([]AgentReferenceRecord, error) {
	query := `
		SELECT rr.task_id, rr.session_id, rr.message_id, rr.resource_id,
		       rr.target_type, rr.method, rr.position, rr.created_at,
		       COALESCE(tr.kind, wa.kind, ''), tr.source_type,
		       COALESCE(tr.logical_path, wa.logical_path, ''),
		       COALESCE(tr.mime_type, wa.mime_type),
		       COALESCE(tr.byte_size, wa.byte_size),
		       COALESCE(tr.immutable, 0)
		  FROM resource_references rr
		  LEFT JOIN task_resources tr
		    ON rr.target_type = 'resource' AND tr.task_id = rr.task_id
		   AND tr.id = rr.resource_id AND tr.removed_at IS NULL
		  LEFT JOIN workspace_artifacts wa
		    ON rr.target_type = 'artifact' AND wa.task_id = rr.task_id
		   AND wa.id = rr.resource_id AND wa.deleted_at IS NULL
		 WHERE rr.task_id = ? AND rr.session_id = ?`
	args := []any{taskID, sessionID}
	if messageID != "" {
		query += " AND rr.message_id = ?"
		args = append(args, messageID)
	}
	query += " ORDER BY rr.message_id, rr.position"
	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	references := make([]AgentReferenceRecord, 0)
	for rows.Next() {
		var record AgentReferenceRecord
		var sourceType, mimeType sql.NullString
		var byteSize sql.NullInt64
		if err := rows.Scan(
			&record.TaskID, &record.SessionID, &record.MessageID,
			&record.ResourceID, &record.TargetType, &record.Method,
			&record.Position, &record.CreatedAt, &record.Kind, &sourceType,
			&record.LogicalPath, &mimeType, &byteSize, &record.Immutable,
		); err != nil {
			return nil, err
		}
		record.SourceType = nullableString(sourceType)
		record.MIMEType = nullableString(mimeType)
		if byteSize.Valid {
			value := byteSize.Int64
			record.ByteSize = &value
		}
		references = append(references, record)
	}
	return references, rows.Err()
}

func (s *SQLiteStore) RemoveAgentMessageReference(
	taskID string,
	sessionID string,
	messageID string,
	resourceID string,
) error {
	if err := validateScopedID(taskID, sessionID, "agent session"); err != nil {
		return err
	}
	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(resourceID) == "" {
		return errors.New("message and resource ids are required")
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		if _, err := connection.ExecContext(ctx, `
			DELETE FROM message_attachments
			 WHERE task_id = ? AND session_id = ? AND message_id = ? AND resource_id = ?`,
			taskID, sessionID, messageID, resourceID,
		); err != nil {
			return err
		}
		result, err := connection.ExecContext(ctx, `
			DELETE FROM resource_references
			 WHERE task_id = ? AND session_id = ? AND message_id = ? AND resource_id = ?`,
			taskID, sessionID, messageID, resourceID,
		)
		return requireScopedWrite(result, err, "agent message reference")
	})
}

func (s *SQLiteStore) UpsertRequirementProposal(record RequirementProposalRecord) error {
	if err := validateRequirementProposal(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO requirement_proposals(
				task_id, artifact_id, base_revision, state, accepted_revision,
				created_at, resolved_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(artifact_id) DO UPDATE SET
				base_revision = requirement_proposals.base_revision
			WHERE requirement_proposals.task_id = excluded.task_id`,
			record.TaskID, record.ArtifactID, record.BaseRevision, record.State,
			record.AcceptedRevision, record.CreatedAt, record.ResolvedAt,
		)
		return requireScopedWrite(result, err, "requirement proposal")
	})
}

func (s *SQLiteStore) RequirementProposals(taskID string) ([]RequirementProposalRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return nil, fmt.Errorf("invalid task id %q", taskID)
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := database.Query(`
		SELECT task_id, artifact_id, base_revision, state, accepted_revision,
		       created_at, resolved_at
		  FROM requirement_proposals WHERE task_id = ?
		 ORDER BY created_at, artifact_id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]RequirementProposalRecord, 0)
	for rows.Next() {
		var record RequirementProposalRecord
		var accepted sql.NullInt64
		var resolved sql.NullString
		if err := rows.Scan(
			&record.TaskID, &record.ArtifactID, &record.BaseRevision,
			&record.State, &accepted, &record.CreatedAt, &resolved,
		); err != nil {
			return nil, err
		}
		if accepted.Valid {
			value := int(accepted.Int64)
			record.AcceptedRevision = &value
		}
		record.ResolvedAt = nullableString(resolved)
		records = append(records, record)
	}
	return records, rows.Err()
}

func validateAgentReference(record AgentReferenceRecord) error {
	if err := validateScopedID(record.TaskID, record.SessionID, "agent session"); err != nil {
		return err
	}
	if strings.TrimSpace(record.MessageID) == "" || strings.TrimSpace(record.ResourceID) == "" {
		return errors.New("agent reference message and resource ids are required")
	}
	if record.TargetType != "resource" && record.TargetType != "artifact" {
		return errors.New("agent reference target type is unsupported")
	}
	if record.Method != "mention" && record.Method != "attachment" && record.Method != "generated" {
		return errors.New("agent reference method is unsupported")
	}
	if record.Position < 0 || record.CreatedAt == "" {
		return errors.New("agent reference position and timestamp are required")
	}
	return nil
}

func validateRequirementProposal(record RequirementProposalRecord) error {
	if err := validateScopedID(record.TaskID, record.ArtifactID, "requirement proposal artifact"); err != nil {
		return err
	}
	if record.BaseRevision < 0 || record.State != "pending" || record.AcceptedRevision != nil || record.ResolvedAt != nil {
		return errors.New("new requirement proposal must be unresolved and pending")
	}
	if record.CreatedAt == "" {
		return errors.New("requirement proposal timestamp is required")
	}
	return nil
}
