package storage

import (
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

INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);
`

// SQLiteStore is the first persistence slice of the architecture. The UI
// workspace remains a single versioned document for now, but SQLite is already
// the durable source in desktop mode so normalized repositories can be added
// through migrations without changing the Wails bridge.
type SQLiteStore struct {
	appName      string
	explicitPath string
	mutex        sync.Mutex
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
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(
		configDirectory,
		s.appName,
		"database",
		"btask.db",
	), nil
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
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}

	_, err = database.Exec(
		`INSERT INTO workspace_state(id, payload, revision, updated_at)
		 VALUES (1, ?, 1, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   payload = excluded.payload,
		   revision = workspace_state.revision + 1,
		   updated_at = CURRENT_TIMESTAMP`,
		payload,
	)
	return err
}

func (s *SQLiteStore) Clear() error {
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	_, err = database.Exec(`DELETE FROM workspace_state WHERE id = 1`)
	return err
}

func (s *SQLiteStore) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.database == nil {
		return nil
	}
	err := s.database.Close()
	s.database = nil
	return err
}

