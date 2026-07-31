package storage

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCreatesTaskWorkspaceProjectionAndSafePreview(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	payload, err := json.Marshal(map[string]any{
		"state": map[string]any{
			"tasks": []any{map[string]any{
				"id":        "task_projection",
				"title":     "任务空间投影",
				"summary":   "保留原始图片 ![preview](" + dataURL + ")",
				"status":    "requirements",
				"priority":  "high",
				"revision":  2,
				"createdAt": "2026-08-01T08:00:00Z",
				"updatedAt": "2026-08-01T09:00:00Z",
				"evidence": []any{map[string]any{
					"id": "manual", "type": "manual", "title": "用户说明", "content": "不得删除旧资料",
				}},
				"requirements": map[string]any{
					"objective":          "建立 Task Workspace",
					"acceptanceCriteria": []string{"完整目录树存在"},
					"interview": map[string]any{
						"analyst": "oh-my-pi",
					},
					"approvedRevisions": []any{map[string]any{
						"version": 1, "document": "# 已确认需求", "executionPrompt": "只实施阶段 1", "confirmedAt": "2026-08-01T08:30:00Z",
					}},
				},
				"development": map[string]any{"engine": "omp"},
			}},
			"trashedTasks": []any{},
		},
		"version": 14,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(string(payload)); err != nil {
		t.Fatalf("save workspace state: %v", err)
	}
	record, err := store.TaskWorkspace("task_projection")
	if err != nil {
		t.Fatalf("read task workspace: %v", err)
	}
	if record.State != "ready" || record.SchemaVersion != 1 || record.ManifestRevision != 1 {
		t.Fatalf("unexpected workspace record %#v", record)
	}
	if _, err := os.Stat(filepath.Join(record.RootPath, ".btask", "manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	resources, err := store.TaskResources("task_projection")
	if err != nil || len(resources) < 5 {
		t.Fatalf("unexpected resource projection %#v, error %v", resources, err)
	}
	preview, err := store.ReadTaskWorkspaceFile("task_projection", "context/task.md")
	if err != nil || !strings.Contains(preview.Content, "任务空间投影") {
		t.Fatalf("unexpected task preview %#v, error %v", preview, err)
	}
	entries, err := store.ListTaskWorkspaceFiles("task_projection", "context")
	if err != nil || len(entries) < 3 {
		t.Fatalf("unexpected context entries %#v, error %v", entries, err)
	}
	firstWorkspaceID := record.WorkspaceID
	if _, err := store.EnsureTaskWorkspace("task_projection"); err != nil {
		t.Fatalf("repeat explicit ensure: %v", err)
	}
	record, err = store.TaskWorkspace("task_projection")
	if err != nil || record.WorkspaceID != firstWorkspaceID || record.ManifestRevision != 1 {
		t.Fatalf("explicit Ensure was not idempotent: %#v, error %v", record, err)
	}
	migration, err := store.LegacyTaskMigration("task_projection")
	if err != nil || migration.State != "completed" || migration.SourceRevision != 2 {
		t.Fatalf("unexpected migration record: %#v, error %v", migration, err)
	}
	warnings := strings.Join(migration.Warnings, "\n")
	if !strings.Contains(warnings, "development.engine") || !strings.Contains(warnings, "interview.analyst") {
		t.Fatalf("legacy engine warnings were not recorded: %v", migration.Warnings)
	}
}

func TestSaveKeepsTaskStateWhenOneTaskWorkspaceNeedsRepair(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStoreAt(filepath.Join(root, "btask.db"))
	t.Cleanup(func() { _ = store.Close() })
	initial := `{"state":{"tasks":[{"id":"task_repair","title":"初始","revision":1}]}}`
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}
	approvedPath := filepath.Join(root, "tasks", "task_repair", "context", "requirements", "approved-v1.md")
	if err := os.WriteFile(approvedPath, []byte("用户已有不可变版本"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated := `{"state":{"tasks":[{"id":"task_repair","title":"已保存的新标题","revision":2,"requirements":{"approvedRevisions":[{"version":1,"document":"不同内容"}]}}]}}`
	if err := store.Save(updated); err != nil {
		t.Fatalf("task state should survive workspace projection failure: %v", err)
	}
	if loaded, err := store.Load(); err != nil || loaded != updated {
		t.Fatalf("updated task state was lost: %q, %v", loaded, err)
	}
	record, err := store.TaskWorkspace("task_repair")
	if err != nil || record.State != "error" || record.ErrorMessage == nil {
		t.Fatalf("workspace repair state was not recorded: %#v, %v", record, err)
	}
	if string(readBytesFromPath(t, approvedPath)) != "用户已有不可变版本" {
		t.Fatal("immutable approved file changed after failed projection")
	}
}

func readBytesFromPath(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return content
}
