package storage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationRunnerUpgradesLegacyV2AndPreservesWorkspaceState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE workspace_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			payload TEXT NOT NULL,
			revision INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE task_context_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			custom_root TEXT,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO schema_migrations(version) VALUES (1), (2);
		INSERT INTO workspace_state(id, payload) VALUES (1, '{"state":{"tasks":[]}}');
	`
	if _, err := database.Exec(legacySchema); err != nil {
		_ = database.Close()
		t.Fatalf("seed legacy database: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store := NewSQLiteStoreAt(path)
	if err := store.Open(); err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	if payload, err := store.Load(); err != nil || payload != `{"state":{"tasks":[]}}` {
		t.Fatalf("legacy payload changed: %q, %v", payload, err)
	}
	database, err = store.readyDatabase()
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaVersion(t, database, currentSchemaVersion)
	for _, table := range []string{
		"task_workspaces",
		"task_resources",
		"agent_sessions",
		"agent_messages",
		"agent_events",
		"execution_runs",
		"tool_calls",
		"permission_requests",
		"permission_grants",
		"git_bindings",
		"workspace_artifacts",
		"legacy_task_migrations",
	} {
		var count int
		if err := database.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
			table,
		).Scan(&count); err != nil || count != 1 {
			t.Fatalf("missing v3 table %q: count %d, error %v", table, count, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := NewSQLiteStoreAt(path)
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Open(); err != nil {
		t.Fatalf("repeat Open: %v", err)
	}
	database, err = reopened.readyDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var migrationCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil || migrationCount != 3 {
		t.Fatalf("migrations were replayed: count %d, error %v", migrationCount, err)
	}
}

func TestMigrationRunnerCreatesFreshV3Database(t *testing.T) {
	store := NewSQLiteStoreAt(filepath.Join(t.TempDir(), "fresh.db"))
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	database, err := store.readyDatabase()
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaVersion(t, database, currentSchemaVersion)
	for _, pragma := range []struct {
		name string
		want int
	}{
		{"foreign_keys", 1},
		{"busy_timeout", 5000},
	} {
		var value int
		if err := database.QueryRow(`PRAGMA ` + pragma.name).Scan(&value); err != nil || value != pragma.want {
			t.Fatalf("PRAGMA %s = %d, want %d, error %v", pragma.name, value, pragma.want, err)
		}
	}
}

func TestMigrationFailureRollsBackVersionAndDDL(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "failed.db"))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if err := configureDatabase(database); err != nil {
		t.Fatal(err)
	}
	err = runMigrations(database, []migration{{
		version: 3,
		statements: []string{
			`CREATE TABLE migration_should_rollback (id INTEGER PRIMARY KEY)`,
			`THIS IS NOT VALID SQL`,
		},
	}})
	if err == nil {
		t.Fatal("expected migration failure")
	}
	var count int
	if queryErr := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE name = 'migration_should_rollback'`,
	).Scan(&count); queryErr != nil || count != 0 {
		t.Fatalf("failed migration DDL was not rolled back: count %d, error %v", count, queryErr)
	}
	if queryErr := database.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 3`,
	).Scan(&count); queryErr != nil || count != 0 {
		t.Fatalf("failed migration version was recorded: count %d, error %v", count, queryErr)
	}
}

func TestOpenRejectsDatabaseFromNewerApplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO schema_migrations(version) VALUES (99);
	`); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	_ = database.Close()
	store := NewSQLiteStoreAt(path)
	err = store.Open()
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected newer schema rejection, got %v", err)
	}
}

func TestV3ForeignKeysRejectCrossTaskSessionAndRunReferences(t *testing.T) {
	store := NewSQLiteStoreAt(filepath.Join(t.TempDir(), "scope.db"))
	t.Cleanup(func() { _ = store.Close() })
	database, err := store.readyDatabase()
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []struct {
		taskID      string
		workspaceID string
		rootPath    string
	}{
		{"task_scope_a", "workspace-a", "/tasks/task_scope_a"},
		{"task_scope_b", "workspace-b", "/tasks/task_scope_b"},
	} {
		if _, err := database.Exec(`
			INSERT INTO task_workspaces(
				task_id, workspace_id, root_path, schema_version,
				manifest_revision, state, created_at, updated_at
			) VALUES (?, ?, ?, 1, 1, 'ready', 'now', 'now')`,
			values.taskID,
			values.workspaceID,
			values.rootPath,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`
		INSERT INTO agent_sessions(
			id, task_id, engine, title, mode, resource_policy, state,
			created_at, updated_at, last_active_at
		) VALUES ('session-a', 'task_scope_a', 'pi', 'A', 'ask', 'isolated', 'idle', 'now', 'now', 'now')`,
	); err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
		INSERT INTO execution_runs(
			id, task_id, session_id, mode, state, events_path,
			stdout_path, stderr_path, result_path, started_at
		) VALUES (
			'run-cross', 'task_scope_b', 'session-a', 'ask', 'queued',
			'runs/events.jsonl', 'runs/stdout.log', 'runs/stderr.log',
			'runs/result.json', 'now'
		)`,
	)
	if err == nil {
		t.Fatal("cross-task session/run relation was accepted")
	}
}

func assertSchemaVersion(t *testing.T, database *sql.DB, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}
