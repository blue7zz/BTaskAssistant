package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskContextRootDefaultsAndPersistsCustomSelection(t *testing.T) {
	dataDirectory := t.TempDir()
	databasePath := filepath.Join(dataDirectory, "btask.db")
	store := NewSQLiteStoreAt(databasePath)

	info, err := store.TaskContextRootInfo()
	if err != nil {
		t.Fatalf("read default task context root: %v", err)
	}
	defaultRoot := filepath.Join(dataDirectory, "tasks")
	if info.Path != defaultRoot || info.DefaultPath != defaultRoot || info.Custom {
		t.Fatalf("unexpected default root info %#v", info)
	}

	initial := `{"state":{"tasks":[{"id":"task_root","title":"默认目录"}]}}`
	if err := store.Save(initial); err != nil {
		t.Fatalf("save to default task context root: %v", err)
	}
	info, err = store.TaskContextRootInfo()
	if err != nil {
		t.Fatalf("refresh default task context root: %v", err)
	}
	if !info.Available {
		t.Fatalf("default task context root should be available: %#v", info)
	}

	customRoot := t.TempDir()
	resolvedCustomRoot, err := filepath.EvalSymlinks(customRoot)
	if err != nil {
		t.Fatalf("resolve custom task context root: %v", err)
	}
	selected, err := store.SetTaskContextRoot(customRoot)
	if err != nil {
		t.Fatalf("set custom task context root: %v", err)
	}
	if selected.Path != resolvedCustomRoot || !selected.Custom || !selected.Available {
		t.Fatalf("unexpected selected root info %#v", selected)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store before reopen: %v", err)
	}

	reopened := NewSQLiteStoreAt(databasePath)
	t.Cleanup(func() { _ = reopened.Close() })
	persisted, err := reopened.TaskContextRootInfo()
	if err != nil {
		t.Fatalf("read persisted task context root: %v", err)
	}
	if persisted.Path != resolvedCustomRoot || persisted.DefaultPath != defaultRoot || !persisted.Custom {
		t.Fatalf("custom root was not persisted: %#v", persisted)
	}

	updated := `{"state":{"tasks":[{"id":"task_root","title":"自定义目录"}]}}`
	if err := reopened.Save(updated); err != nil {
		t.Fatalf("save through persisted custom root: %v", err)
	}
	context := readTaskContextAtRoot(t, resolvedCustomRoot, "task_root")
	if context["title"] != "自定义目录" {
		t.Fatalf("custom root did not receive updated context: %#v", context)
	}
	oldContext := readTaskContextAtRoot(t, defaultRoot, "task_root")
	if oldContext["title"] != "默认目录" {
		t.Fatalf("old root should be retained unchanged: %#v", oldContext)
	}
}

func TestSetTaskContextRootCopiesAllUserFilesAndKeepsOldRoot(t *testing.T) {
	dataDirectory := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(dataDirectory, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	payload := `{"state":{"tasks":[{"id":"task_migrate","title":"迁移任务","evidence":[{"id":"source_file","type":"file","title":"source.txt","content":"原始资料"}]}]}}`
	if err := store.Save(payload); err != nil {
		t.Fatalf("save source task context: %v", err)
	}
	oldRoot := filepath.Join(dataDirectory, "tasks")
	userTextPath := filepath.Join(oldRoot, "task_migrate", "files", "user-notes.txt")
	userImagePath := filepath.Join(oldRoot, "task_migrate", "images", "user-image.png")
	textBytes := []byte("用户额外保存的文本\n第二行")
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x01, 0x02}
	if err := os.WriteFile(userTextPath, textBytes, 0o600); err != nil {
		t.Fatalf("write user text fixture: %v", err)
	}
	if err := os.WriteFile(userImagePath, imageBytes, 0o600); err != nil {
		t.Fatalf("write user image fixture: %v", err)
	}

	newRoot := t.TempDir()
	if _, err := store.SetTaskContextRoot(newRoot); err != nil {
		t.Fatalf("migrate task context root: %v", err)
	}
	for _, fixture := range []struct {
		name    string
		oldPath string
		newPath string
		want    []byte
	}{
		{
			name:    "text",
			oldPath: userTextPath,
			newPath: filepath.Join(newRoot, "task_migrate", "files", "user-notes.txt"),
			want:    textBytes,
		},
		{
			name:    "image",
			oldPath: userImagePath,
			newPath: filepath.Join(newRoot, "task_migrate", "images", "user-image.png"),
			want:    imageBytes,
		},
	} {
		copied, err := os.ReadFile(fixture.newPath)
		if err != nil {
			t.Fatalf("read copied user %s: %v", fixture.name, err)
		}
		if string(copied) != string(fixture.want) {
			t.Fatalf("copied user %s changed bytes", fixture.name)
		}
		original, err := os.ReadFile(fixture.oldPath)
		if err != nil {
			t.Fatalf("old user %s was removed: %v", fixture.name, err)
		}
		if string(original) != string(fixture.want) {
			t.Fatalf("old user %s changed bytes", fixture.name)
		}
	}
	context := readTaskContextAtRoot(t, newRoot, "task_migrate")
	if context["title"] != "迁移任务" {
		t.Fatalf("migrated context is incomplete: %#v", context)
	}
}

func TestSetTaskContextRootFailureDoesNotSwitchConfiguredRoot(t *testing.T) {
	dataDirectory := t.TempDir()
	databasePath := filepath.Join(dataDirectory, "btask.db")
	store := NewSQLiteStoreAt(databasePath)

	initial := `{"state":{"tasks":[{"id":"task_failure","title":"迁移前"}]}}`
	if err := store.Save(initial); err != nil {
		t.Fatalf("save task before failed migration: %v", err)
	}
	oldRoot := filepath.Join(dataDirectory, "tasks")
	outside := t.TempDir()
	linkPath := filepath.Join(oldRoot, "task_failure", "files", "unsupported-link")
	if err := os.Symlink(outside, linkPath); err != nil {
		_ = store.Close()
		t.Skipf("symlinks are unavailable: %v", err)
	}

	failedTarget := t.TempDir()
	_, err := store.SetTaskContextRoot(failedTarget)
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		_ = store.Close()
		t.Fatalf("expected migration symlink failure, got %v", err)
	}
	info, infoErr := store.TaskContextRootInfo()
	if infoErr != nil {
		_ = store.Close()
		t.Fatalf("read root after failed migration: %v", infoErr)
	}
	if info.Path != oldRoot || info.Custom {
		_ = store.Close()
		t.Fatalf("failed migration switched root: %#v", info)
	}

	updated := `{"state":{"tasks":[{"id":"task_failure","title":"仍写旧目录"}]}}`
	if err := store.Save(updated); err != nil {
		_ = store.Close()
		t.Fatalf("save after failed migration: %v", err)
	}
	context := readTaskContextAtRoot(t, oldRoot, "task_failure")
	if context["title"] != "仍写旧目录" {
		_ = store.Close()
		t.Fatalf("old root was not retained after migration failure: %#v", context)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store after failed migration: %v", err)
	}

	reopened := NewSQLiteStoreAt(databasePath)
	t.Cleanup(func() { _ = reopened.Close() })
	persisted, err := reopened.TaskContextRootInfo()
	if err != nil {
		t.Fatalf("reopen root after failed migration: %v", err)
	}
	if persisted.Path != oldRoot || persisted.Custom {
		t.Fatalf("failed migration persisted a root switch: %#v", persisted)
	}
}

func TestSetTaskContextRootRejectsNonEmptyTargetWithoutChangingIt(t *testing.T) {
	dataDirectory := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(dataDirectory, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Save(`{"state":{"tasks":[{"id":"task_existing","title":"现有任务"}]}}`); err != nil {
		t.Fatalf("save source task context: %v", err)
	}
	target := t.TempDir()
	targetFile := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(targetFile, []byte("do not overwrite"), 0o600); err != nil {
		t.Fatalf("write target fixture: %v", err)
	}

	if _, err := store.SetTaskContextRoot(target); err == nil || !strings.Contains(err.Error(), "空目录") {
		t.Fatalf("expected non-empty target rejection, got %v", err)
	}
	content, err := os.ReadFile(targetFile)
	if err != nil || string(content) != "do not overwrite" {
		t.Fatalf("target content changed after rejected migration: %q, %v", content, err)
	}
	info, err := store.TaskContextRootInfo()
	if err != nil {
		t.Fatalf("read root after rejected migration: %v", err)
	}
	if info.Path != filepath.Join(dataDirectory, "tasks") || info.Custom {
		t.Fatalf("rejected migration switched root: %#v", info)
	}
}
