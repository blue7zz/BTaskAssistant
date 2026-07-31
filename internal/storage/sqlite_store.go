package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const initialSchema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS workspace_state (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	payload TEXT NOT NULL,
	revision INTEGER NOT NULL DEFAULT 1,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS task_context_settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	custom_root TEXT,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);
INSERT OR IGNORE INTO schema_migrations(version) VALUES (2);
`

// SQLiteStore is the first persistence slice of the architecture. The UI
// workspace remains a single versioned document for now, but SQLite is already
// the durable source in desktop mode so normalized repositories can be added
// through migrations without changing the Wails bridge.
type SQLiteStore struct {
	appName      string
	explicitPath string
	mutex        sync.Mutex
	writeMutex   sync.Mutex
	database     *sql.DB
}

func NewSQLiteStore(appName string) *SQLiteStore {
	return &SQLiteStore{appName: appName}
}

func NewSQLiteStoreAt(path string) *SQLiteStore {
	return &SQLiteStore{explicitPath: path}
}

func (s *SQLiteStore) path() (string, error) {
	if s.explicitPath != "" {
		return s.explicitPath, nil
	}
	dataDirectory, err := s.dataDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDirectory, "database", "btask.db"), nil
}

func (s *SQLiteStore) dataDirectory() (string, error) {
	if s.explicitPath != "" {
		if s.explicitPath == ":memory:" {
			return "", nil
		}
		return filepath.Dir(s.explicitPath), nil
	}
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirectory, s.appName), nil
}

func (s *SQLiteStore) Open() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.database != nil {
		return nil
	}

	path, err := s.path()
	if err != nil {
		return err
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	}

	database, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)

	if _, err := database.Exec(initialSchema); err != nil {
		database.Close()
		return fmt.Errorf("initialize database: %w", err)
	}
	s.database = database
	return nil
}

func (s *SQLiteStore) readyDatabase() (*sql.DB, error) {
	if err := s.Open(); err != nil {
		return nil, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.database == nil {
		return nil, errors.New("database is not open")
	}
	return s.database, nil
}

func (s *SQLiteStore) Load() (string, error) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	database, err := s.readyDatabase()
	if err != nil {
		return "", err
	}

	var payload string
	err = database.QueryRow(
		`SELECT payload FROM workspace_state WHERE id = 1`,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return payload, nil
}

func (s *SQLiteStore) Save(payload string) error {
	contexts, err := decodeTaskContexts(payload)
	if err != nil {
		return fmt.Errorf("decode task contexts: %w", err)
	}

	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	database, err := s.readyDatabase()
	if err != nil {
		return err
	}

	return withImmediateWrite(database, func(ctx context.Context, conn *sql.Conn) error {
		root, err := s.taskContextRootWithConn(ctx, conn)
		if err != nil {
			return err
		}
		if err := syncTaskContexts(root, contexts); err != nil {
			return fmt.Errorf("sync task contexts: %w", err)
		}
		_, err = conn.ExecContext(
			ctx,
			`INSERT INTO workspace_state(id, payload, revision, updated_at)
			 VALUES (1, ?, 1, CURRENT_TIMESTAMP)
			 ON CONFLICT(id) DO UPDATE SET
			   payload = excluded.payload,
			   revision = workspace_state.revision + 1,
			   updated_at = CURRENT_TIMESTAMP`,
			payload,
		)
		return err
	})
}

func (s *SQLiteStore) Clear() error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	return withImmediateWrite(database, func(ctx context.Context, conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, `DELETE FROM workspace_state WHERE id = 1`)
		return err
	})
}

func (s *SQLiteStore) Close() error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.database == nil {
		return nil
	}
	err := s.database.Close()
	s.database = nil
	return err
}

func withImmediateWrite(
	database *sql.DB,
	operation func(context.Context, *sql.Conn) error,
) (err error) {
	ctx := context.Background()
	connection, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()

	if _, err := connection.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(ctx, `ROLLBACK`)
		}
	}()

	if err := operation(ctx, connection); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}
