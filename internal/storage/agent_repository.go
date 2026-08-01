package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

var ErrAgentDataNotFound = errors.New("task agent data not found")

const (
	maxAgentEventPayloadBytes = 256 * 1024
	maxToolPreviewBytes       = 32 * 1024
)

type AgentSessionRecord struct {
	ID                  string  `json:"id"`
	TaskID              string  `json:"taskId"`
	Engine              string  `json:"engine"`
	ExternalSessionPath *string `json:"externalSessionPath,omitempty"`
	ExternalSessionID   *string `json:"externalSessionId,omitempty"`
	Title               string  `json:"title"`
	Mode                string  `json:"mode"`
	Model               *string `json:"model,omitempty"`
	ThinkingLevel       *string `json:"thinkingLevel,omitempty"`
	ResourcePolicy      string  `json:"resourcePolicy"`
	State               string  `json:"state"`
	LastEntryID         *string `json:"lastEntryId,omitempty"`
	LastSequence        int64   `json:"lastSequence"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
	LastActiveAt        string  `json:"lastActiveAt"`
	ErrorMessage        *string `json:"errorMessage,omitempty"`
}

type ExecutionRunRecord struct {
	ID                  string  `json:"id"`
	TaskID              string  `json:"taskId"`
	SessionID           string  `json:"sessionId"`
	RequirementRevision *string `json:"requirementRevision,omitempty"`
	GitBindingID        *string `json:"gitBindingId,omitempty"`
	BaselineCommit      *string `json:"baselineCommit,omitempty"`
	Mode                string  `json:"mode"`
	State               string  `json:"state"`
	EventsPath          string  `json:"eventsPath"`
	StdoutPath          string  `json:"stdoutPath"`
	StderrPath          string  `json:"stderrPath"`
	ResultPath          string  `json:"resultPath"`
	StartedAt           string  `json:"startedAt"`
	FinishedAt          *string `json:"finishedAt,omitempty"`
	ResultSummary       *string `json:"resultSummary,omitempty"`
	ErrorMessage        *string `json:"errorMessage,omitempty"`
}

type AgentMessageRecord struct {
	ID          string                 `json:"id"`
	TaskID      string                 `json:"taskId"`
	SessionID   string                 `json:"sessionId"`
	RunID       *string                `json:"runId,omitempty"`
	Role        string                 `json:"role"`
	Kind        string                 `json:"kind"`
	Status      string                 `json:"status"`
	Content     *string                `json:"content,omitempty"`
	ContentRef  *string                `json:"contentRef,omitempty"`
	Sequence    int64                  `json:"sequence"`
	PIEntryID   *string                `json:"piEntryId,omitempty"`
	CreatedAt   string                 `json:"createdAt"`
	CompletedAt *string                `json:"completedAt,omitempty"`
	References  []AgentReferenceRecord `json:"references,omitempty"`
}

type AgentEventRecord struct {
	EventID     string  `json:"eventId"`
	Version     int     `json:"version"`
	TaskID      string  `json:"taskId"`
	SessionID   string  `json:"sessionId"`
	RunID       *string `json:"runId,omitempty"`
	ToolCallID  *string `json:"toolCallId,omitempty"`
	Sequence    int64   `json:"sequence"`
	Kind        string  `json:"kind"`
	PayloadJSON string  `json:"payloadJson"`
	PayloadRef  *string `json:"payloadRef,omitempty"`
	OccurredAt  string  `json:"occurredAt"`
}

type ToolCallRecord struct {
	ID                 string  `json:"id"`
	TaskID             string  `json:"taskId"`
	SessionID          string  `json:"sessionId"`
	RunID              string  `json:"runId"`
	ExternalToolCallID string  `json:"externalToolCallId"`
	ToolName           string  `json:"toolName"`
	Capability         string  `json:"capability"`
	Target             *string `json:"target,omitempty"`
	RiskLevel          string  `json:"riskLevel"`
	State              string  `json:"state"`
	ArgsJSON           *string `json:"argsJson,omitempty"`
	ArgsRef            *string `json:"argsRef,omitempty"`
	OutputSummary      *string `json:"outputSummary,omitempty"`
	OutputRef          *string `json:"outputRef,omitempty"`
	IsError            bool    `json:"isError"`
	StartedAt          *string `json:"startedAt,omitempty"`
	FinishedAt         *string `json:"finishedAt,omitempty"`
}

func (s *SQLiteStore) UpsertAgentSession(record AgentSessionRecord) error {
	if err := validateAgentSessionRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO agent_sessions(
				id, task_id, engine, external_session_path, external_session_id,
				title, mode, model, thinking_level, resource_policy, state,
				last_entry_id, last_sequence, created_at, updated_at,
				last_active_at, error_message
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				external_session_path = excluded.external_session_path,
				external_session_id = excluded.external_session_id,
				title = excluded.title,
				mode = excluded.mode,
				model = excluded.model,
				thinking_level = excluded.thinking_level,
				resource_policy = excluded.resource_policy,
				state = excluded.state,
				last_entry_id = excluded.last_entry_id,
				last_sequence = excluded.last_sequence,
				updated_at = excluded.updated_at,
				last_active_at = excluded.last_active_at,
				error_message = excluded.error_message
			WHERE agent_sessions.task_id = excluded.task_id`,
			record.ID,
			record.TaskID,
			record.Engine,
			record.ExternalSessionPath,
			record.ExternalSessionID,
			record.Title,
			record.Mode,
			record.Model,
			record.ThinkingLevel,
			record.ResourcePolicy,
			record.State,
			record.LastEntryID,
			record.LastSequence,
			record.CreatedAt,
			record.UpdatedAt,
			record.LastActiveAt,
			record.ErrorMessage,
		)
		return requireScopedWrite(result, err, "agent session")
	})
}

func (s *SQLiteStore) AgentSession(taskID string, sessionID string) (AgentSessionRecord, error) {
	if err := validateScopedID(taskID, sessionID, "agent session"); err != nil {
		return AgentSessionRecord{}, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return AgentSessionRecord{}, err
	}
	defer unlock()
	return scanAgentSession(database.QueryRow(`
		SELECT id, task_id, engine, external_session_path, external_session_id,
		       title, mode, model, thinking_level, resource_policy, state,
		       last_entry_id, last_sequence, created_at, updated_at,
		       last_active_at, error_message
		  FROM agent_sessions WHERE task_id = ? AND id = ?`, taskID, sessionID))
}

func (s *SQLiteStore) AgentSessions(taskID string) ([]AgentSessionRecord, error) {
	if !taskIDPattern.MatchString(taskID) {
		return nil, fmt.Errorf("invalid task id %q", taskID)
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := database.Query(`
		SELECT id, task_id, engine, external_session_path, external_session_id,
		       title, mode, model, thinking_level, resource_policy, state,
		       last_entry_id, last_sequence, created_at, updated_at,
		       last_active_at, error_message
		  FROM agent_sessions WHERE task_id = ?
		 ORDER BY last_active_at DESC, id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]AgentSessionRecord, 0)
	for rows.Next() {
		record, err := scanAgentSession(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) UpsertExecutionRun(record ExecutionRunRecord) error {
	if err := validateExecutionRunRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO execution_runs(
				id, task_id, session_id, requirement_revision, git_binding_id,
				baseline_commit, mode, state, events_path, stdout_path,
				stderr_path, result_path, started_at, finished_at,
				result_summary, error_message
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				requirement_revision = excluded.requirement_revision,
				git_binding_id = excluded.git_binding_id,
				baseline_commit = excluded.baseline_commit,
				mode = excluded.mode,
				state = excluded.state,
				events_path = excluded.events_path,
				stdout_path = excluded.stdout_path,
				stderr_path = excluded.stderr_path,
				result_path = excluded.result_path,
				finished_at = excluded.finished_at,
				result_summary = excluded.result_summary,
				error_message = excluded.error_message
			WHERE execution_runs.task_id = excluded.task_id
			  AND execution_runs.session_id = excluded.session_id`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RequirementRevision,
			record.GitBindingID,
			record.BaselineCommit,
			record.Mode,
			record.State,
			record.EventsPath,
			record.StdoutPath,
			record.StderrPath,
			record.ResultPath,
			record.StartedAt,
			record.FinishedAt,
			record.ResultSummary,
			record.ErrorMessage,
		)
		return requireScopedWrite(result, err, "execution run")
	})
}

func (s *SQLiteStore) ExecutionRun(taskID string, runID string) (ExecutionRunRecord, error) {
	if err := validateScopedID(taskID, runID, "execution run"); err != nil {
		return ExecutionRunRecord{}, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return ExecutionRunRecord{}, err
	}
	defer unlock()
	return scanExecutionRun(database.QueryRow(`
		SELECT id, task_id, session_id, requirement_revision, git_binding_id,
		       baseline_commit, mode, state, events_path, stdout_path,
		       stderr_path, result_path, started_at, finished_at,
		       result_summary, error_message
		  FROM execution_runs WHERE task_id = ? AND id = ?`, taskID, runID))
}

func (s *SQLiteStore) ExecutionRuns(
	taskID string,
	sessionID string,
) ([]ExecutionRunRecord, error) {
	if err := validateScopedID(taskID, sessionID, "agent session"); err != nil {
		return nil, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := database.Query(`
		SELECT id, task_id, session_id, requirement_revision, git_binding_id,
		       baseline_commit, mode, state, events_path, stdout_path,
		       stderr_path, result_path, started_at, finished_at,
		       result_summary, error_message
		  FROM execution_runs
		 WHERE task_id = ? AND session_id = ?
		 ORDER BY started_at, id`, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]ExecutionRunRecord, 0)
	for rows.Next() {
		record, err := scanExecutionRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) UpsertAgentMessage(record AgentMessageRecord) error {
	if err := validateAgentMessageRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO agent_messages(
				id, task_id, session_id, run_id, role, kind, status, content,
				content_ref, sequence, pi_entry_id, created_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				run_id = excluded.run_id,
				status = excluded.status,
				content = excluded.content,
				content_ref = excluded.content_ref,
				pi_entry_id = excluded.pi_entry_id,
				completed_at = excluded.completed_at
			WHERE agent_messages.task_id = excluded.task_id
			  AND agent_messages.session_id = excluded.session_id
			  AND agent_messages.sequence = excluded.sequence
			  AND agent_messages.role = excluded.role
			  AND agent_messages.kind = excluded.kind`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.Role,
			record.Kind,
			record.Status,
			record.Content,
			record.ContentRef,
			record.Sequence,
			record.PIEntryID,
			record.CreatedAt,
			record.CompletedAt,
		)
		return requireScopedWrite(result, err, "agent message")
	})
}

// UpsertAgentMessageAndUpdateSession persists a newly allocated message
// sequence together with the owning session cursor.
func (s *SQLiteStore) UpsertAgentMessageAndUpdateSession(
	record AgentMessageRecord,
	session AgentSessionRecord,
) error {
	if err := validateAgentMessageRecord(record); err != nil {
		return err
	}
	if err := validateAgentSessionRecord(session); err != nil {
		return err
	}
	if record.TaskID != session.TaskID || record.SessionID != session.ID ||
		record.Sequence != session.LastSequence {
		return errors.New("agent message and session sequence do not match")
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		if _, err := connection.ExecContext(ctx, `
			INSERT INTO agent_messages(
				id, task_id, session_id, run_id, role, kind, status, content,
				content_ref, sequence, pi_entry_id, created_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.Role,
			record.Kind,
			record.Status,
			record.Content,
			record.ContentRef,
			record.Sequence,
			record.PIEntryID,
			record.CreatedAt,
			record.CompletedAt,
		); err != nil {
			return err
		}
		result, err := connection.ExecContext(ctx, `
			UPDATE agent_sessions
			   SET last_sequence = ?, updated_at = ?, last_active_at = ?
			 WHERE task_id = ? AND id = ?`,
			session.LastSequence,
			session.UpdatedAt,
			session.LastActiveAt,
			session.TaskID,
			session.ID,
		)
		return requireScopedWrite(result, err, "agent session")
	})
}

func (s *SQLiteStore) AgentMessages(taskID string, sessionID string) ([]AgentMessageRecord, error) {
	if err := validateScopedID(taskID, sessionID, "agent session"); err != nil {
		return nil, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := database.Query(`
		SELECT id, task_id, session_id, run_id, role, kind, status, content,
		       content_ref, sequence, pi_entry_id, created_at, completed_at
		  FROM agent_messages WHERE task_id = ? AND session_id = ?
		 ORDER BY sequence`, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	records := make([]AgentMessageRecord, 0)
	for rows.Next() {
		record, err := scanAgentMessage(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	references, err := agentReferencesWithDB(database, taskID, sessionID, "")
	if err != nil {
		return nil, err
	}
	byMessage := make(map[string][]AgentReferenceRecord)
	for _, reference := range references {
		byMessage[reference.MessageID] = append(byMessage[reference.MessageID], reference)
	}
	for index := range records {
		records[index].References = byMessage[records[index].ID]
	}
	return records, nil
}

func (s *SQLiteStore) AppendAgentEvent(record AgentEventRecord) error {
	if err := validateAgentEventRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		_, err := connection.ExecContext(ctx, `
			INSERT INTO agent_events(
				event_id, version, task_id, session_id, run_id, tool_call_id,
				sequence, kind, payload_json, payload_ref, occurred_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.EventID,
			record.Version,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.ToolCallID,
			record.Sequence,
			record.Kind,
			record.PayloadJSON,
			record.PayloadRef,
			record.OccurredAt,
		)
		return err
	})
}

// AppendAgentEventAndUpdateSession reserves the session sequence and appends
// its event in one SQLite transaction. Wails emitters can therefore publish
// only after both durable records agree.
func (s *SQLiteStore) AppendAgentEventAndUpdateSession(
	record AgentEventRecord,
	session AgentSessionRecord,
) error {
	if err := validateAgentEventRecord(record); err != nil {
		return err
	}
	if err := validateAgentSessionRecord(session); err != nil {
		return err
	}
	if record.TaskID != session.TaskID || record.SessionID != session.ID ||
		record.Sequence != session.LastSequence {
		return errors.New("agent event and session sequence do not match")
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		if _, err := connection.ExecContext(ctx, `
			INSERT INTO agent_events(
				event_id, version, task_id, session_id, run_id, tool_call_id,
				sequence, kind, payload_json, payload_ref, occurred_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.EventID,
			record.Version,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.ToolCallID,
			record.Sequence,
			record.Kind,
			record.PayloadJSON,
			record.PayloadRef,
			record.OccurredAt,
		); err != nil {
			return err
		}
		result, err := connection.ExecContext(ctx, `
			UPDATE agent_sessions
			   SET external_session_path = ?, external_session_id = ?, title = ?,
			       mode = ?, model = ?, thinking_level = ?, resource_policy = ?,
			       state = ?, last_entry_id = ?, last_sequence = ?, updated_at = ?,
			       last_active_at = ?, error_message = ?
			 WHERE task_id = ? AND id = ?`,
			session.ExternalSessionPath,
			session.ExternalSessionID,
			session.Title,
			session.Mode,
			session.Model,
			session.ThinkingLevel,
			session.ResourcePolicy,
			session.State,
			session.LastEntryID,
			session.LastSequence,
			session.UpdatedAt,
			session.LastActiveAt,
			session.ErrorMessage,
			session.TaskID,
			session.ID,
		)
		return requireScopedWrite(result, err, "agent session")
	})
}

func (s *SQLiteStore) AgentEvents(taskID string, sessionID string) ([]AgentEventRecord, error) {
	if err := validateScopedID(taskID, sessionID, "agent session"); err != nil {
		return nil, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rows, err := database.Query(`
		SELECT event_id, version, task_id, session_id, run_id, tool_call_id,
		       sequence, kind, payload_json, payload_ref, occurred_at
		  FROM agent_events WHERE task_id = ? AND session_id = ?
		 ORDER BY sequence`, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]AgentEventRecord, 0)
	for rows.Next() {
		record, err := scanAgentEvent(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) UpsertToolCall(record ToolCallRecord) error {
	if err := validateToolCallRecord(record); err != nil {
		return err
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `
			INSERT INTO tool_calls(
				id, task_id, session_id, run_id, external_tool_call_id,
				tool_name, capability, target, risk_level, state, args_json,
				args_ref, output_summary, output_ref, is_error, started_at,
				finished_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				capability = excluded.capability,
				target = excluded.target,
				risk_level = excluded.risk_level,
				state = excluded.state,
				args_json = excluded.args_json,
				args_ref = excluded.args_ref,
				output_summary = excluded.output_summary,
				output_ref = excluded.output_ref,
				is_error = excluded.is_error,
				started_at = excluded.started_at,
				finished_at = excluded.finished_at
			WHERE tool_calls.task_id = excluded.task_id
			  AND tool_calls.session_id = excluded.session_id
			  AND tool_calls.run_id = excluded.run_id
			  AND tool_calls.external_tool_call_id = excluded.external_tool_call_id
			  AND tool_calls.tool_name = excluded.tool_name`,
			record.ID,
			record.TaskID,
			record.SessionID,
			record.RunID,
			record.ExternalToolCallID,
			record.ToolName,
			record.Capability,
			record.Target,
			record.RiskLevel,
			record.State,
			record.ArgsJSON,
			record.ArgsRef,
			record.OutputSummary,
			record.OutputRef,
			record.IsError,
			record.StartedAt,
			record.FinishedAt,
		)
		return requireScopedWrite(result, err, "tool call")
	})
}

func (s *SQLiteStore) ToolCall(taskID string, toolCallID string) (ToolCallRecord, error) {
	if err := validateScopedID(taskID, toolCallID, "tool call"); err != nil {
		return ToolCallRecord{}, err
	}
	database, unlock, err := s.repositoryRead()
	if err != nil {
		return ToolCallRecord{}, err
	}
	defer unlock()
	return scanToolCall(database.QueryRow(`
		SELECT id, task_id, session_id, run_id, external_tool_call_id,
		       tool_name, capability, target, risk_level, state, args_json,
		       args_ref, output_summary, output_ref, is_error, started_at,
		       finished_at
		  FROM tool_calls WHERE task_id = ? AND id = ?`, taskID, toolCallID))
}

// InterruptActiveAgentActivity repairs process-owned states after an app
// restart without deleting completed messages or PI session files.
func (s *SQLiteStore) InterruptActiveAgentActivity(
	interruptedAt string,
	reason string,
) error {
	if strings.TrimSpace(interruptedAt) == "" || strings.TrimSpace(reason) == "" {
		return errors.New("agent interruption timestamp and reason are required")
	}
	return s.withRepositoryWrite(func(ctx context.Context, connection *sql.Conn) error {
		if _, err := connection.ExecContext(ctx, `
			UPDATE agent_messages
			   SET status = 'error', completed_at = COALESCE(completed_at, ?)
			 WHERE status IN ('pending', 'streaming')`, interruptedAt); err != nil {
			return err
		}
		if _, err := connection.ExecContext(ctx, `
			UPDATE execution_runs
			   SET state = 'interrupted', finished_at = COALESCE(finished_at, ?),
			       error_message = ?
			 WHERE state IN ('queued', 'running', 'waiting_permission', 'stopping')`,
			interruptedAt,
			reason,
		); err != nil {
			return err
		}
		_, err := connection.ExecContext(ctx, `
			UPDATE agent_sessions
			   SET state = 'interrupted', updated_at = ?, error_message = ?
			 WHERE state IN ('starting', 'running', 'stopping')`,
			interruptedAt,
			reason,
		)
		return err
	})
}

func (s *SQLiteStore) withRepositoryWrite(
	operation func(context.Context, *sql.Conn) error,
) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	return withImmediateWrite(database, operation)
}

func (s *SQLiteStore) repositoryRead() (*sql.DB, func(), error) {
	s.writeMutex.Lock()
	database, err := s.readyDatabase()
	if err != nil {
		s.writeMutex.Unlock()
		return nil, nil, err
	}
	return database, s.writeMutex.Unlock, nil
}

func requireScopedWrite(result sql.Result, err error, kind string) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%s id belongs to a different task or parent", kind)
	}
	return nil
}

func validateScopedID(taskID string, id string, kind string) error {
	if !taskIDPattern.MatchString(taskID) {
		return fmt.Errorf("invalid task id %q", taskID)
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s id is required", kind)
	}
	return nil
}

func validateAgentSessionRecord(record AgentSessionRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "agent session"); err != nil {
		return err
	}
	if record.Engine != "pi" {
		return errors.New("agent session engine must be native pi")
	}
	if strings.TrimSpace(record.Title) == "" || strings.TrimSpace(record.Mode) == "" ||
		strings.TrimSpace(record.ResourcePolicy) == "" || strings.TrimSpace(record.State) == "" {
		return errors.New("agent session title, mode, resource policy, and state are required")
	}
	if record.LastSequence < 0 {
		return errors.New("agent session last sequence must not be negative")
	}
	if record.CreatedAt == "" || record.UpdatedAt == "" || record.LastActiveAt == "" {
		return errors.New("agent session timestamps are required")
	}
	if record.ExternalSessionPath != nil {
		if err := validateAbsoluteCleanPath(*record.ExternalSessionPath); err != nil {
			return fmt.Errorf("agent external session path: %w", err)
		}
	}
	return nil
}

func validateExecutionRunRecord(record ExecutionRunRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "execution run"); err != nil {
		return err
	}
	if strings.TrimSpace(record.SessionID) == "" || strings.TrimSpace(record.Mode) == "" ||
		strings.TrimSpace(record.State) == "" {
		return errors.New("execution run session, mode, and state are required")
	}
	for label, value := range map[string]string{
		"events": record.EventsPath,
		"stdout": record.StdoutPath,
		"stderr": record.StderrPath,
		"result": record.ResultPath,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("execution run %s path is required", label)
		}
		normalized, err := taskspace.ValidateLogicalPath(value)
		if err != nil || !strings.HasPrefix(normalized, "runs/") {
			return fmt.Errorf("execution run %s path must be canonical under runs", label)
		}
	}
	if record.StartedAt == "" {
		return errors.New("execution run start timestamp is required")
	}
	return nil
}

func validateAgentMessageRecord(record AgentMessageRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "agent message"); err != nil {
		return err
	}
	if strings.TrimSpace(record.SessionID) == "" || strings.TrimSpace(record.Role) == "" ||
		strings.TrimSpace(record.Kind) == "" || strings.TrimSpace(record.Status) == "" {
		return errors.New("agent message session, role, kind, and status are required")
	}
	if record.Sequence < 1 {
		return errors.New("agent message sequence must be positive")
	}
	if record.CreatedAt == "" {
		return errors.New("agent message creation timestamp is required")
	}
	if err := validateOptionalWorkspaceReference(record.ContentRef); err != nil {
		return fmt.Errorf("agent message content reference: %w", err)
	}
	return nil
}

func validateAgentEventRecord(record AgentEventRecord) error {
	if err := validateScopedID(record.TaskID, record.EventID, "agent event"); err != nil {
		return err
	}
	if record.Version < 1 || record.Sequence < 1 {
		return errors.New("agent event version and sequence must be positive")
	}
	if strings.TrimSpace(record.SessionID) == "" || strings.TrimSpace(record.Kind) == "" ||
		strings.TrimSpace(record.PayloadJSON) == "" || record.OccurredAt == "" {
		return errors.New("agent event session, kind, payload, and timestamp are required")
	}
	if len(record.PayloadJSON) > maxAgentEventPayloadBytes {
		return errors.New("agent event payload exceeds the 256 KiB inline limit")
	}
	if !json.Valid([]byte(record.PayloadJSON)) {
		return errors.New("agent event payload must be valid JSON")
	}
	if err := validateOptionalWorkspaceReference(record.PayloadRef); err != nil {
		return fmt.Errorf("agent event payload reference: %w", err)
	}
	return nil
}

func validateToolCallRecord(record ToolCallRecord) error {
	if err := validateScopedID(record.TaskID, record.ID, "tool call"); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"session":     record.SessionID,
		"run":         record.RunID,
		"external id": record.ExternalToolCallID,
		"name":        record.ToolName,
		"capability":  record.Capability,
		"risk level":  record.RiskLevel,
		"state":       record.State,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("tool call %s is required", label)
		}
	}
	if err := validateOptionalWorkspaceReference(record.ArgsRef); err != nil {
		return fmt.Errorf("tool call arguments reference: %w", err)
	}
	if err := validateOptionalWorkspaceReference(record.OutputRef); err != nil {
		return fmt.Errorf("tool call output reference: %w", err)
	}
	if record.ArgsJSON != nil && len(*record.ArgsJSON) > maxToolPreviewBytes {
		return errors.New("tool call arguments exceed the 32 KiB inline limit")
	}
	if record.ArgsJSON != nil && !json.Valid([]byte(*record.ArgsJSON)) {
		return errors.New("tool call arguments must be valid JSON")
	}
	if record.OutputSummary != nil && len(*record.OutputSummary) > maxToolPreviewBytes {
		return errors.New("tool call output summary exceeds the 32 KiB inline limit")
	}
	return nil
}

func validateOptionalWorkspaceReference(value *string) error {
	if value == nil {
		return nil
	}
	normalized, err := taskspace.ValidateLogicalPath(*value)
	if err != nil || normalized == "." || !strings.HasPrefix(normalized, "runs/") {
		return errors.New("path must be canonical under runs")
	}
	return nil
}

func scanAgentSession(scanner rowScanner) (AgentSessionRecord, error) {
	var record AgentSessionRecord
	var externalSessionPath, externalSessionID, model, thinkingLevel sql.NullString
	var lastEntryID, errorMessage sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.Engine,
		&externalSessionPath,
		&externalSessionID,
		&record.Title,
		&record.Mode,
		&model,
		&thinkingLevel,
		&record.ResourcePolicy,
		&record.State,
		&lastEntryID,
		&record.LastSequence,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.LastActiveAt,
		&errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentSessionRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return AgentSessionRecord{}, err
	}
	record.ExternalSessionPath = nullableString(externalSessionPath)
	record.ExternalSessionID = nullableString(externalSessionID)
	record.Model = nullableString(model)
	record.ThinkingLevel = nullableString(thinkingLevel)
	record.LastEntryID = nullableString(lastEntryID)
	record.ErrorMessage = nullableString(errorMessage)
	return record, nil
}

func scanExecutionRun(scanner rowScanner) (ExecutionRunRecord, error) {
	var record ExecutionRunRecord
	var requirementRevision, gitBindingID, baselineCommit sql.NullString
	var finishedAt, resultSummary, errorMessage sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.SessionID,
		&requirementRevision,
		&gitBindingID,
		&baselineCommit,
		&record.Mode,
		&record.State,
		&record.EventsPath,
		&record.StdoutPath,
		&record.StderrPath,
		&record.ResultPath,
		&record.StartedAt,
		&finishedAt,
		&resultSummary,
		&errorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionRunRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return ExecutionRunRecord{}, err
	}
	record.RequirementRevision = nullableString(requirementRevision)
	record.GitBindingID = nullableString(gitBindingID)
	record.BaselineCommit = nullableString(baselineCommit)
	record.FinishedAt = nullableString(finishedAt)
	record.ResultSummary = nullableString(resultSummary)
	record.ErrorMessage = nullableString(errorMessage)
	return record, nil
}

func scanAgentMessage(scanner rowScanner) (AgentMessageRecord, error) {
	var record AgentMessageRecord
	var runID, content, contentRef, piEntryID, completedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.SessionID,
		&runID,
		&record.Role,
		&record.Kind,
		&record.Status,
		&content,
		&contentRef,
		&record.Sequence,
		&piEntryID,
		&record.CreatedAt,
		&completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentMessageRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return AgentMessageRecord{}, err
	}
	record.RunID = nullableString(runID)
	record.Content = nullableString(content)
	record.ContentRef = nullableString(contentRef)
	record.PIEntryID = nullableString(piEntryID)
	record.CompletedAt = nullableString(completedAt)
	return record, nil
}

func scanAgentEvent(scanner rowScanner) (AgentEventRecord, error) {
	var record AgentEventRecord
	var runID, toolCallID, payloadRef sql.NullString
	err := scanner.Scan(
		&record.EventID,
		&record.Version,
		&record.TaskID,
		&record.SessionID,
		&runID,
		&toolCallID,
		&record.Sequence,
		&record.Kind,
		&record.PayloadJSON,
		&payloadRef,
		&record.OccurredAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentEventRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return AgentEventRecord{}, err
	}
	record.RunID = nullableString(runID)
	record.ToolCallID = nullableString(toolCallID)
	record.PayloadRef = nullableString(payloadRef)
	return record, nil
}

func scanToolCall(scanner rowScanner) (ToolCallRecord, error) {
	var record ToolCallRecord
	var target, argsJSON, argsRef, outputSummary, outputRef sql.NullString
	var startedAt, finishedAt sql.NullString
	err := scanner.Scan(
		&record.ID,
		&record.TaskID,
		&record.SessionID,
		&record.RunID,
		&record.ExternalToolCallID,
		&record.ToolName,
		&record.Capability,
		&target,
		&record.RiskLevel,
		&record.State,
		&argsJSON,
		&argsRef,
		&outputSummary,
		&outputRef,
		&record.IsError,
		&startedAt,
		&finishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ToolCallRecord{}, ErrAgentDataNotFound
	}
	if err != nil {
		return ToolCallRecord{}, err
	}
	record.Target = nullableString(target)
	record.ArgsJSON = nullableString(argsJSON)
	record.ArgsRef = nullableString(argsRef)
	record.OutputSummary = nullableString(outputSummary)
	record.OutputRef = nullableString(outputRef)
	record.StartedAt = nullableString(startedAt)
	record.FinishedAt = nullableString(finishedAt)
	return record, nil
}
