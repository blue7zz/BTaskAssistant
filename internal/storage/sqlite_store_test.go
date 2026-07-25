package storage

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStoreRoundTrip(t *testing.T) {
	store := NewSQLiteStoreAt(filepath.Join(t.TempDir(), "btask.db"))
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	if value, err := store.Load(); err != nil || value != "" {
		t.Fatalf("expected an empty store, got value %q and error %v", value, err)
	}
	if err := store.Save(`{"version":1}`); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if value, err := store.Load(); err != nil || value != `{"version":1}` {
		t.Fatalf("unexpected round trip value %q and error %v", value, err)
	}
}

func TestSQLiteStoreClear(t *testing.T) {
	store := NewSQLiteStoreAt(filepath.Join(t.TempDir(), "btask.db"))
	t.Cleanup(func() {
		_ = store.Close()
	})

	if err := store.Save(`{"tasks":[1]}`); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear state: %v", err)
	}
	if value, err := store.Load(); err != nil || value != "" {
		t.Fatalf("expected cleared state, got value %q and error %v", value, err)
	}
}

