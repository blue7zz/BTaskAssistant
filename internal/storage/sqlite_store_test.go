package storage

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func readTaskContext(t *testing.T, root string, taskID string) map[string]any {
	t.Helper()
	return readTaskContextAtRoot(t, filepath.Join(root, "tasks"), taskID)
}

func readTaskContextAtRoot(
	t *testing.T,
	root string,
	taskID string,
) map[string]any {
	t.Helper()
	content, err := os.ReadFile(
		filepath.Join(root, taskID, taskContextFilename),
	)
	if err != nil {
		t.Fatalf("read task context %q: %v", taskID, err)
	}
	var context map[string]any
	if err := json.Unmarshal(content, &context); err != nil {
		t.Fatalf("decode task context %q: %v", taskID, err)
	}
	return context
}

func workspaceRevision(t *testing.T, store *SQLiteStore) int {
	t.Helper()
	database, err := store.readyDatabase()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	var revision int
	if err := database.QueryRow(
		`SELECT revision FROM workspace_state WHERE id = 1`,
	).Scan(&revision); err != nil {
		t.Fatalf("read workspace revision: %v", err)
	}
	return revision
}

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
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() {
		_ = store.Close()
	})

	payload := `{"state":{"tasks":[{"id":"task_clear","title":"clear me"}]}}`
	if err := store.Save(payload); err != nil {
		t.Fatalf("save state: %v", err)
	}
	readTaskContext(t, root, "task_clear")
	if err := store.Clear(); err != nil {
		t.Fatalf("clear state: %v", err)
	}
	if value, err := store.Load(); err != nil || value != "" {
		t.Fatalf("expected cleared state, got value %q and error %v", value, err)
	}
	if context := readTaskContext(t, root, "task_clear"); context["title"] != "clear me" {
		t.Fatalf("clear must preserve the task directory, got %#v", context)
	}
	if err := store.ReconcileTaskContexts(); err != nil {
		t.Fatalf("reconcile cleared state: %v", err)
	}
	if context := readTaskContext(t, root, "task_clear"); context["title"] != "clear me" {
		t.Fatalf("reconcile must not delete preserved task context, got %#v", context)
	}
}

func TestSQLiteStoreWritesIndependentActiveAndTrashedTaskContexts(
	t *testing.T,
) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	payload := `{
  "state": {
    "tasks": [{
      "id": "task_active",
      "title": "活动任务",
      "evidence": [{"id":"source_1","content":"完整原文"}],
      "requirements": {"approvedRevisions":[{"version":1,"document":"v1"}]},
      "development": {"resultNote":"已完成开发"},
      "review": {"note":"人工验收"}
    }],
    "trashedTasks": [{
      "id": "task_trashed",
      "title": "回收站任务",
      "trashedAt": "2026-08-01T01:02:03Z"
    }]
  },
  "version": 13
}`
	if err := store.Save(payload); err != nil {
		t.Fatalf("save task contexts: %v", err)
	}

	active := readTaskContext(t, root, "task_active")
	if active["title"] != "活动任务" {
		t.Fatalf("unexpected active task context %#v", active)
	}
	evidence, ok := active["evidence"].([]any)
	if !ok || len(evidence) != 1 {
		t.Fatalf("task evidence was not preserved: %#v", active["evidence"])
	}
	trashed := readTaskContext(t, root, "task_trashed")
	if trashed["trashedAt"] != "2026-08-01T01:02:03Z" {
		t.Fatalf("trash metadata was not preserved: %#v", trashed)
	}
}

func TestSQLiteStoreLoadIsReadOnlyAndReconcileBackfillsTaskContexts(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Open(); err != nil {
		t.Fatalf("open store: %v", err)
	}
	payload := `{"state":{"tasks":[{"id":"task_legacy","title":"旧任务"}]}}`
	database, err := store.readyDatabase()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workspace_state(id, payload) VALUES (1, ?)`,
		payload,
	); err != nil {
		t.Fatalf("seed legacy state: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "tasks", "task_legacy")); !os.IsNotExist(err) {
		t.Fatalf("legacy directory should not exist before load, got %v", err)
	}
	loaded, err := store.Load()
	if err != nil || loaded != payload {
		t.Fatalf("load legacy state: payload %q, error %v", loaded, err)
	}
	if _, err := os.Stat(filepath.Join(root, "tasks")); !os.IsNotExist(err) {
		t.Fatalf("Load must not create task directories, got %v", err)
	}
	if err := store.ReconcileTaskContexts(); err != nil {
		t.Fatalf("reconcile legacy task contexts: %v", err)
	}
	legacy := readTaskContext(t, root, "task_legacy")
	if legacy["title"] != "旧任务" {
		t.Fatalf("unexpected backfilled context %#v", legacy)
	}
}

func TestSQLiteStorePreservesTaskDirectoriesAfterTaskDeletion(
	t *testing.T,
) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Save(
		`{"state":{"tasks":[{"id":"task_keep","revision":1}],"trashedTasks":[{"id":"task_delete","revision":2}]}}`,
	); err != nil {
		t.Fatalf("save initial contexts: %v", err)
	}
	unmanaged := filepath.Join(root, "tasks", "manual-notes")
	if err := os.MkdirAll(unmanaged, 0o700); err != nil {
		t.Fatalf("create unmanaged directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unmanaged, "notes.md"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unmanaged note: %v", err)
	}

	if err := store.Save(
		`{"state":{"tasks":[],"trashedTasks":[{"id":"task_keep","revision":2,"trashedAt":"now"}]}}`,
	); err != nil {
		t.Fatalf("save updated contexts: %v", err)
	}
	keep := readTaskContext(t, root, "task_keep")
	if keep["trashedAt"] != "now" {
		t.Fatalf("soft-deleted task context was not updated: %#v", keep)
	}
	deleted := readTaskContext(t, root, "task_delete")
	if deleted["revision"] != float64(2) {
		t.Fatalf("deleted task directory must retain its last context: %#v", deleted)
	}
	if _, err := os.Stat(filepath.Join(unmanaged, "notes.md")); err != nil {
		t.Fatalf("unmanaged directory must be preserved: %v", err)
	}
}

func TestSQLiteStoreMaterializesFileEvidenceAndDataImages(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	imageBytes, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatalf("decode fixture image: %v", err)
	}
	imageDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	payloadBytes, err := json.Marshal(map[string]any{
		"state": map[string]any{
			"tasks": []any{map[string]any{
				"id":      "task_materials",
				"title":   "资料物化",
				"summary": "设计图：![preview](" + imageDataURL + ")",
				"evidence": []any{
					map[string]any{
						"id":      "source_text",
						"type":    "file",
						"title":   "notes.md",
						"content": "第一行\n第二行",
					},
					map[string]any{
						"id":      "source_image",
						"type":    "file",
						"title":   "preview.png",
						"content": "![preview](" + imageDataURL + ")",
					},
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("encode workspace fixture: %v", err)
	}
	if err := store.Save(string(payloadBytes)); err != nil {
		t.Fatalf("save workspace materials: %v", err)
	}

	textSum := sha256.Sum256([]byte("source_text"))
	textPath := filepath.Join(
		root,
		"tasks",
		"task_materials",
		"files",
		hex.EncodeToString(textSum[:8])+".md",
	)
	textContent, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatalf("read materialized text evidence: %v", err)
	}
	if string(textContent) != "第一行\n第二行" {
		t.Fatalf("unexpected text evidence %q", textContent)
	}

	imageSum := sha256.Sum256(imageBytes)
	imagePath := filepath.Join(
		root,
		"tasks",
		"task_materials",
		"images",
		hex.EncodeToString(imageSum[:])+".png",
	)
	materializedImage, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read materialized image: %v", err)
	}
	if string(materializedImage) != string(imageBytes) {
		t.Fatal("materialized image bytes differ from the data URL payload")
	}
	entries, err := os.ReadDir(filepath.Dir(imagePath))
	if err != nil {
		t.Fatalf("read image directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("duplicate data URLs must share one image file, got %d", len(entries))
	}
	contextContent, err := os.ReadFile(
		filepath.Join(root, "tasks", "task_materials", taskContextFilename),
	)
	if err != nil {
		t.Fatalf("read materialized task context: %v", err)
	}
	if !strings.Contains(string(contextContent), imageDataURL) {
		t.Fatal("context must retain the legacy data URL for recovery")
	}
}

func TestSQLiteStoreFilesystemFailureLeavesDatabaseUnchanged(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	initial := `{"state":{"tasks":[{"id":"task_stable","title":"旧标题"}]}}`
	if err := store.Save(initial); err != nil {
		t.Fatalf("save initial workspace: %v", err)
	}
	initialRevision := workspaceRevision(t, store)
	contextPath := filepath.Join(root, "tasks", "task_stable", taskContextFilename)
	if err := os.Remove(contextPath); err != nil {
		t.Fatalf("remove context fixture: %v", err)
	}
	if err := os.Mkdir(contextPath, 0o700); err != nil {
		t.Fatalf("create context write conflict: %v", err)
	}

	updated := `{"state":{"tasks":[{"id":"task_stable","title":"新标题"}]}}`
	err := store.Save(updated)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected filesystem write failure, got %v", err)
	}
	loaded, loadErr := store.Load()
	if loadErr != nil || loaded != initial {
		t.Fatalf("failed filesystem sync changed DB payload to %q: %v", loaded, loadErr)
	}
	if revision := workspaceRevision(t, store); revision != initialRevision {
		t.Fatalf("failed filesystem sync changed revision from %d to %d", initialRevision, revision)
	}
}

func TestSQLiteStoreRejectsUnsafeTaskContextID(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	err := store.Save(`{"state":{"tasks":[{"id":"../outside"}]}}`)
	if err == nil || !strings.Contains(err.Error(), "invalid task id") {
		t.Fatalf("expected invalid task id error, got %v", err)
	}
	if value, loadErr := store.Load(); loadErr != nil || value != "" {
		t.Fatalf("unsafe payload must not be saved, got %q and %v", value, loadErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "outside")); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe task escaped context root: %v", statErr)
	}
}

func TestSQLiteStoreRejectsTaskDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	outside := t.TempDir()
	tasksRoot := filepath.Join(root, "tasks")
	if err := os.Mkdir(tasksRoot, 0o700); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(tasksRoot, "task_link")); err != nil {
		t.Skipf("task directory symlinks are unavailable: %v", err)
	}

	err := store.Save(`{"state":{"tasks":[{"id":"task_link"}]}}`)
	if err == nil || !strings.Contains(err.Error(), "not a regular directory") {
		t.Fatalf("expected task directory symlink error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, taskContextFilename)); !os.IsNotExist(statErr) {
		t.Fatalf("task context escaped through symlink: %v", statErr)
	}
}

func TestSQLiteStoreRejectsTaskMaterialDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	initial := `{"state":{"tasks":[{"id":"task_material_link","title":"初始"}]}}`
	if err := store.Save(initial); err != nil {
		t.Fatalf("save initial task: %v", err)
	}
	filesPath := filepath.Join(root, "tasks", "task_material_link", "files")
	if err := os.Remove(filesPath); err != nil {
		t.Fatalf("remove managed files directory: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filesPath); err != nil {
		t.Skipf("material directory symlinks are unavailable: %v", err)
	}

	err := store.Save(`{"state":{"tasks":[{"id":"task_material_link","title":"更新","evidence":[{"id":"source_link","type":"file","title":"notes.txt","content":"不得越界"}]}]}}`)
	if err == nil || !strings.Contains(err.Error(), "not a regular directory") {
		t.Fatalf("expected material directory symlink error, got %v", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("material escaped through symlink: entries %v, error %v", entries, readErr)
	}
	loaded, loadErr := store.Load()
	if loadErr != nil || loaded != initial {
		t.Fatalf("symlink failure changed DB state to %q: %v", loaded, loadErr)
	}
}

func TestSQLiteStoreRejectsCaseFoldedTaskDirectoryCollision(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })

	empty := `{"state":{"tasks":[]}}`
	if err := store.Save(empty); err != nil {
		t.Fatalf("prepare task root: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "tasks", "TASK_CASE"), 0o700); err != nil {
		t.Fatalf("create case-folded directory: %v", err)
	}

	err := store.Save(`{"state":{"tasks":[{"id":"task_case"}]}}`)
	if err == nil || !strings.Contains(err.Error(), "directory name collision") {
		t.Fatalf("expected case-folded directory collision, got %v", err)
	}
	loaded, loadErr := store.Load()
	if loadErr != nil || loaded != empty {
		t.Fatalf("case collision changed DB state to %q: %v", loaded, loadErr)
	}
}

func TestSQLiteStoreConcurrentInstancesKeepContextAlignedWithDatabase(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "btask.db")
	first := NewSQLiteStoreAt(databasePath)
	second := NewSQLiteStoreAt(databasePath)
	t.Cleanup(func() {
		_ = first.Close()
		_ = second.Close()
	})
	if err := first.Open(); err != nil {
		t.Fatalf("open first store: %v", err)
	}
	if err := second.Open(); err != nil {
		t.Fatalf("open second store: %v", err)
	}
	if err := first.Save(`{"state":{"tasks":[{"id":"task_concurrent","title":"seed"}]}}`); err != nil {
		t.Fatalf("seed concurrent task: %v", err)
	}

	payloads := []string{
		`{"state":{"tasks":[{"id":"task_concurrent","title":"first"}]}}`,
		`{"state":{"tasks":[{"id":"task_concurrent","title":"second"}]}}`,
	}
	stores := []*SQLiteStore{first, second}
	start := make(chan struct{})
	errors := make(chan error, len(stores))
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			errors <- stores[index].Save(payloads[index])
		}(index)
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent save failed: %v", err)
		}
	}

	loaded, err := first.Load()
	if err != nil {
		t.Fatalf("load concurrent result: %v", err)
	}
	if loaded != payloads[0] && loaded != payloads[1] {
		t.Fatalf("unexpected concurrent payload %q", loaded)
	}
	var persisted struct {
		State struct {
			Tasks []struct {
				Title string `json:"title"`
			} `json:"tasks"`
		} `json:"state"`
	}
	if err := json.Unmarshal([]byte(loaded), &persisted); err != nil {
		t.Fatalf("decode concurrent result: %v", err)
	}
	context := readTaskContext(t, root, "task_concurrent")
	if context["title"] != persisted.State.Tasks[0].Title {
		t.Fatalf("context title %v does not match DB title %q", context["title"], persisted.State.Tasks[0].Title)
	}
}
