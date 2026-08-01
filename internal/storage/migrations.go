package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

const currentSchemaVersion = 5

type migration struct {
	version    int
	statements []string
}

var schemaMigrations = []migration{
	{
		version: 1,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS workspace_state (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				payload TEXT NOT NULL,
				revision INTEGER NOT NULL DEFAULT 1,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
		},
	},
	{
		version: 2,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS task_context_settings (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				custom_root TEXT,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
		},
	},
	{
		version: 3,
		statements: []string{
			`CREATE TABLE task_workspaces (
				task_id TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL UNIQUE,
				root_path TEXT NOT NULL UNIQUE,
				schema_version INTEGER NOT NULL,
				manifest_revision INTEGER NOT NULL DEFAULT 1,
				state TEXT NOT NULL CHECK (state IN ('ready', 'legacy', 'error', 'archived')),
				legacy_context_path TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				last_reconciled_at TEXT,
				error_message TEXT,
				UNIQUE (task_id, workspace_id)
			)`,
			`CREATE TABLE task_resources (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL REFERENCES task_workspaces(task_id) ON DELETE RESTRICT,
				kind TEXT NOT NULL CHECK (kind IN ('source', 'attachment', 'external', 'context')),
				source_type TEXT NOT NULL,
				logical_path TEXT NOT NULL,
				storage_path TEXT,
				external_path TEXT,
				mime_type TEXT,
				byte_size INTEGER,
				sha256 TEXT,
				immutable INTEGER NOT NULL CHECK (immutable IN (0, 1)),
				readable INTEGER NOT NULL CHECK (readable IN (0, 1)),
				created_at TEXT NOT NULL,
				removed_at TEXT,
				UNIQUE (task_id, id)
			)`,
			`CREATE UNIQUE INDEX task_resources_active_path
			 ON task_resources(task_id, logical_path) WHERE removed_at IS NULL`,
			`CREATE INDEX task_resources_task_kind
			 ON task_resources(task_id, kind, source_type)`,
			`CREATE TABLE agent_sessions (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL REFERENCES task_workspaces(task_id) ON DELETE RESTRICT,
				engine TEXT NOT NULL CHECK (engine = 'pi'),
				external_session_path TEXT,
				external_session_id TEXT,
				title TEXT NOT NULL,
				mode TEXT NOT NULL CHECK (mode IN ('ask', 'plan', 'agent')),
				model TEXT,
				thinking_level TEXT,
				resource_policy TEXT NOT NULL CHECK (resource_policy IN ('isolated', 'explicit-inherit')),
				state TEXT NOT NULL CHECK (state IN ('created', 'starting', 'idle', 'running', 'stopping', 'interrupted', 'failed', 'closed')),
				last_entry_id TEXT,
				last_sequence INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				last_active_at TEXT NOT NULL,
				error_message TEXT,
				UNIQUE (task_id, id)
			)`,
			`CREATE UNIQUE INDEX agent_sessions_one_active
			 ON agent_sessions(task_id)
			 WHERE state IN ('starting', 'running', 'stopping')`,
			`CREATE TABLE git_bindings (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL UNIQUE REFERENCES task_workspaces(task_id) ON DELETE RESTRICT,
				source_path TEXT NOT NULL,
				source_real_path TEXT NOT NULL,
				common_git_dir TEXT NOT NULL,
				worktree_path TEXT UNIQUE,
				branch TEXT,
				baseline_commit TEXT NOT NULL,
				source_branch TEXT,
				source_dirty_at_bind INTEGER NOT NULL CHECK (source_dirty_at_bind IN (0, 1)),
				state TEXT NOT NULL CHECK (state IN ('bound', 'creating', 'ready', 'missing', 'cleanup_failed', 'archived')),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				error_message TEXT,
				UNIQUE (task_id, id)
			)`,
			`CREATE TABLE execution_runs (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL REFERENCES task_workspaces(task_id) ON DELETE RESTRICT,
				session_id TEXT NOT NULL,
				requirement_revision TEXT,
				git_binding_id TEXT,
				baseline_commit TEXT,
				mode TEXT NOT NULL CHECK (mode IN ('ask', 'plan', 'agent', 'utility')),
				state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'waiting_permission', 'stopping', 'succeeded', 'failed', 'cancelled', 'interrupted')),
				events_path TEXT NOT NULL,
				stdout_path TEXT NOT NULL,
				stderr_path TEXT NOT NULL,
				result_path TEXT NOT NULL,
				started_at TEXT NOT NULL,
				finished_at TEXT,
				result_summary TEXT,
				error_message TEXT,
				UNIQUE (task_id, id),
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, git_binding_id) REFERENCES git_bindings(task_id, id) ON DELETE RESTRICT
			)`,
			`CREATE INDEX execution_runs_task_session
			 ON execution_runs(task_id, session_id, started_at)`,
			`CREATE UNIQUE INDEX execution_runs_one_active
			 ON execution_runs(task_id)
			 WHERE state IN ('queued', 'running', 'waiting_permission', 'stopping')`,
			`CREATE TABLE agent_messages (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL,
				session_id TEXT NOT NULL,
				run_id TEXT,
				role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'tool', 'system')),
				kind TEXT NOT NULL CHECK (kind IN ('text', 'reasoning', 'tool_call', 'tool_result', 'notice')),
				status TEXT NOT NULL CHECK (status IN ('pending', 'streaming', 'complete', 'error', 'cancelled')),
				content TEXT,
				content_ref TEXT,
				sequence INTEGER NOT NULL,
				pi_entry_id TEXT,
				created_at TEXT NOT NULL,
				completed_at TEXT,
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, run_id) REFERENCES execution_runs(task_id, id) ON DELETE RESTRICT,
				UNIQUE (session_id, sequence),
				UNIQUE (task_id, id)
			)`,
			`CREATE TABLE agent_events (
				event_id TEXT PRIMARY KEY,
				version INTEGER NOT NULL,
				task_id TEXT NOT NULL,
				session_id TEXT NOT NULL,
				run_id TEXT,
				tool_call_id TEXT,
				sequence INTEGER NOT NULL,
				kind TEXT NOT NULL,
				payload_json TEXT NOT NULL,
				payload_ref TEXT,
				occurred_at TEXT NOT NULL,
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, run_id) REFERENCES execution_runs(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, tool_call_id) REFERENCES tool_calls(task_id, id) ON DELETE RESTRICT,
				UNIQUE (session_id, sequence),
				UNIQUE (task_id, event_id)
			)`,
			`CREATE INDEX agent_events_task_session
			 ON agent_events(task_id, session_id, sequence)`,
			`CREATE TABLE tool_calls (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL,
				session_id TEXT NOT NULL,
				run_id TEXT NOT NULL,
				external_tool_call_id TEXT NOT NULL,
				tool_name TEXT NOT NULL,
				capability TEXT NOT NULL,
				target TEXT,
				risk_level TEXT NOT NULL,
				state TEXT NOT NULL CHECK (state IN ('received', 'waiting_permission', 'running', 'succeeded', 'failed', 'denied', 'cancelled')),
				args_json TEXT,
				args_ref TEXT,
				output_summary TEXT,
				output_ref TEXT,
				is_error INTEGER NOT NULL DEFAULT 0 CHECK (is_error IN (0, 1)),
				started_at TEXT,
				finished_at TEXT,
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, run_id) REFERENCES execution_runs(task_id, id) ON DELETE RESTRICT,
				UNIQUE (session_id, external_tool_call_id),
				UNIQUE (task_id, id)
			)`,
			`CREATE TABLE permission_requests (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL,
				session_id TEXT NOT NULL,
				run_id TEXT NOT NULL,
				tool_call_id TEXT NOT NULL,
				capability TEXT NOT NULL,
				target TEXT NOT NULL,
				normalized_target TEXT,
				subject TEXT NOT NULL,
				risk_level TEXT NOT NULL,
				state TEXT NOT NULL CHECK (state IN ('pending', 'allowed', 'denied', 'expired', 'cancelled')),
				requested_at TEXT NOT NULL,
				resolved_at TEXT,
				resolved_by TEXT,
				decision_scope TEXT,
				reason TEXT,
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, run_id) REFERENCES execution_runs(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, tool_call_id) REFERENCES tool_calls(task_id, id) ON DELETE RESTRICT,
				UNIQUE (task_id, id)
			)`,
			`CREATE INDEX permission_requests_pending
			 ON permission_requests(task_id, session_id, state)`,
			`CREATE TABLE permission_grants (
				id TEXT PRIMARY KEY,
				task_id TEXT,
				session_id TEXT,
				request_id TEXT,
				capability TEXT NOT NULL,
				target_pattern TEXT NOT NULL,
				scope TEXT NOT NULL CHECK (scope IN ('once', 'session', 'task', 'permanent')),
				decision TEXT NOT NULL CHECK (decision IN ('allow', 'deny')),
				risk_ceiling TEXT NOT NULL,
				created_at TEXT NOT NULL,
				expires_at TEXT,
				consumed_at TEXT,
				revoked_at TEXT,
				created_by TEXT NOT NULL,
				CHECK (
					(scope = 'permanent' AND task_id IS NULL AND session_id IS NULL) OR
					(scope = 'task' AND task_id IS NOT NULL AND session_id IS NULL) OR
					(scope IN ('once', 'session') AND task_id IS NOT NULL AND session_id IS NOT NULL)
				),
				CHECK (scope != 'once' OR request_id IS NOT NULL),
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, request_id) REFERENCES permission_requests(task_id, id) ON DELETE RESTRICT
			)`,
			`CREATE INDEX permission_grants_scope
			 ON permission_grants(task_id, session_id, capability, scope, revoked_at)`,
			`CREATE TABLE workspace_artifacts (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL REFERENCES task_workspaces(task_id) ON DELETE RESTRICT,
				session_id TEXT,
				run_id TEXT,
				logical_path TEXT NOT NULL,
				kind TEXT NOT NULL CHECK (kind IN ('plan', 'report', 'proposal', 'export')),
				mime_type TEXT,
				byte_size INTEGER NOT NULL,
				sha256 TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				deleted_at TEXT,
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, run_id) REFERENCES execution_runs(task_id, id) ON DELETE RESTRICT,
				UNIQUE (task_id, id)
			)`,
			`CREATE UNIQUE INDEX workspace_artifacts_active_path
			 ON workspace_artifacts(task_id, logical_path) WHERE deleted_at IS NULL`,
			`CREATE TABLE legacy_task_migrations (
				task_id TEXT PRIMARY KEY REFERENCES task_workspaces(task_id) ON DELETE RESTRICT,
				source_revision INTEGER NOT NULL,
				legacy_path TEXT NOT NULL,
				target_workspace_id TEXT NOT NULL,
				state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'completed', 'failed')),
				warnings_json TEXT NOT NULL DEFAULT '[]',
				started_at TEXT NOT NULL,
				completed_at TEXT,
				error_message TEXT,
				FOREIGN KEY (task_id, target_workspace_id)
				 REFERENCES task_workspaces(task_id, workspace_id)
				 ON UPDATE CASCADE ON DELETE RESTRICT
			)`,
			`CREATE INDEX legacy_task_migrations_state
			 ON legacy_task_migrations(state, started_at)`,
		},
	},
	{
		version: 4,
		statements: []string{
			`CREATE TABLE message_attachments (
				task_id TEXT NOT NULL,
				session_id TEXT NOT NULL,
				message_id TEXT NOT NULL,
				resource_id TEXT NOT NULL,
				position INTEGER NOT NULL CHECK (position >= 0),
				created_at TEXT NOT NULL,
				PRIMARY KEY (message_id, resource_id),
				UNIQUE (message_id, position),
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, message_id) REFERENCES agent_messages(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, resource_id) REFERENCES task_resources(task_id, id) ON DELETE RESTRICT
			)`,
			`CREATE INDEX message_attachments_task_session
			 ON message_attachments(task_id, session_id, message_id, position)`,
			`CREATE TABLE resource_references (
				task_id TEXT NOT NULL,
				session_id TEXT NOT NULL,
				message_id TEXT NOT NULL,
				resource_id TEXT NOT NULL,
				target_type TEXT NOT NULL CHECK (target_type IN ('resource', 'artifact')),
				method TEXT NOT NULL CHECK (method IN ('mention', 'attachment', 'generated')),
				position INTEGER NOT NULL CHECK (position >= 0),
				created_at TEXT NOT NULL,
				PRIMARY KEY (message_id, resource_id, method),
				UNIQUE (message_id, position),
				FOREIGN KEY (task_id, session_id) REFERENCES agent_sessions(task_id, id) ON DELETE RESTRICT,
				FOREIGN KEY (task_id, message_id) REFERENCES agent_messages(task_id, id) ON DELETE RESTRICT
			)`,
			`CREATE INDEX resource_references_task_session
			 ON resource_references(task_id, session_id, message_id, position)`,
			`CREATE TABLE requirement_proposals (
				task_id TEXT NOT NULL,
				artifact_id TEXT NOT NULL,
				base_revision INTEGER NOT NULL CHECK (base_revision >= 0),
				state TEXT NOT NULL CHECK (state IN ('pending', 'accepted', 'rejected')),
				accepted_revision INTEGER,
				created_at TEXT NOT NULL,
				resolved_at TEXT,
				PRIMARY KEY (artifact_id),
				FOREIGN KEY (task_id, artifact_id) REFERENCES workspace_artifacts(task_id, id) ON DELETE RESTRICT,
				CHECK ((state = 'pending' AND accepted_revision IS NULL AND resolved_at IS NULL) OR
				       (state = 'accepted' AND accepted_revision IS NOT NULL AND resolved_at IS NOT NULL) OR
				       (state = 'rejected' AND accepted_revision IS NULL AND resolved_at IS NOT NULL))
			)`,
			`CREATE INDEX requirement_proposals_task_state
			 ON requirement_proposals(task_id, state, created_at)`,
		},
	},
	{
		version: 5,
		statements: []string{
			`CREATE INDEX agent_messages_task_session_sequence
			 ON agent_messages(task_id, session_id, sequence DESC)`,
			`CREATE INDEX legacy_task_migrations_completion
			 ON legacy_task_migrations(state, completed_at, task_id)`,
			`CREATE TRIGGER legacy_task_migrations_completion_insert
			 BEFORE INSERT ON legacy_task_migrations
			 WHEN (NEW.state = 'completed' AND NEW.completed_at IS NULL) OR
			      (NEW.state != 'completed' AND NEW.completed_at IS NOT NULL)
			 BEGIN
				SELECT RAISE(ABORT, 'legacy migration completion state is inconsistent');
			 END`,
			`CREATE TRIGGER legacy_task_migrations_completion_update
			 BEFORE UPDATE OF state, completed_at ON legacy_task_migrations
			 WHEN (NEW.state = 'completed' AND NEW.completed_at IS NULL) OR
			      (NEW.state != 'completed' AND NEW.completed_at IS NOT NULL)
			 BEGIN
				SELECT RAISE(ABORT, 'legacy migration completion state is inconsistent');
			 END`,
		},
	},
}

func configureDatabase(database *sql.DB) error {
	for _, statement := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := database.Exec(statement); err != nil {
			return fmt.Errorf("configure database: %w", err)
		}
	}
	return nil
}

func runMigrations(database *sql.DB, migrations []migration) error {
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("bootstrap schema migrations: %w", err)
	}

	var existingMax sql.NullInt64
	if err := database.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&existingMax); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if existingMax.Valid && existingMax.Int64 > currentSchemaVersion {
		return fmt.Errorf(
			"database schema version %d is newer than supported version %d",
			existingMax.Int64,
			currentSchemaVersion,
		)
	}

	ordered := append([]migration(nil), migrations...)
	sort.Slice(ordered, func(left int, right int) bool {
		return ordered[left].version < ordered[right].version
	})
	previous := 0
	for _, item := range ordered {
		if item.version <= previous || item.version < 1 {
			return errors.New("database migrations contain duplicate or invalid versions")
		}
		previous = item.version
		if err := applyMigration(database, item); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(database *sql.DB, item migration) error {
	return withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		var exists int
		err := connection.QueryRowContext(
			ctx,
			`SELECT 1 FROM schema_migrations WHERE version = ?`,
			item.version,
		).Scan(&exists)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		for _, statement := range item.statements {
			if _, err := connection.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply database migration %d: %w", item.version, err)
			}
		}
		if _, err := connection.ExecContext(
			ctx,
			`INSERT INTO schema_migrations(version) VALUES (?)`,
			item.version,
		); err != nil {
			return fmt.Errorf("record database migration %d: %w", item.version, err)
		}
		return nil
	})
}
