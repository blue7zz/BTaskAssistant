package taskspace

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var tinyPNG = mustDecodeBase64(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
)

func fixedService() Service {
	return Service{Now: func() time.Time {
		return time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	}}
}

func sampleSnapshot(taskID string) TaskSnapshot {
	imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG)
	snapshot := TaskSnapshot{
		ID:          taskID,
		Title:       "实现任务空间",
		Summary:     "为每个任务保存完整上下文。\n\n![图](" + imageURL + ")",
		ProjectName: "BTaskAssistant",
		ProjectPath: "/workspace/BTaskAssistant",
		Priority:    "high",
		Status:      "requirements",
		Revision:    3,
		CreatedAt:   "2026-08-01T08:00:00Z",
		UpdatedAt:   "2026-08-01T09:00:00Z",
		Evidence: []Evidence{
			{ID: "manual-1", Type: "manual", Title: "用户说明", Content: "只复制旧资料。", CreatedAt: "2026-08-01T08:01:00Z"},
			{ID: "plane-1", Type: "plane", Title: "Plane BT-18", Content: "Plane 原始内容", CreatedAt: "2026-08-01T08:02:00Z"},
			{ID: "chat-1", Type: "chat", Title: "聊天记录", Content: "用户确认目录合同。", CreatedAt: "2026-08-01T08:03:00Z"},
			{ID: "project-1", Type: "project", Title: "项目观察", Content: "已有 SQLiteStore。", CreatedAt: "2026-08-01T08:04:00Z"},
			{ID: "file-1", Type: "file", Title: "notes.md", Content: "附件正文", CreatedAt: "2026-08-01T08:05:00Z"},
			{ID: "image-1", Type: "file", Title: "screen.png", Content: "![图](" + imageURL + ")", CreatedAt: "2026-08-01T08:06:00Z"},
		},
	}
	snapshot.Requirements.Objective = "建立任务隔离空间"
	snapshot.Requirements.Scope = []string{"SQLite v3", "安全文件读取"}
	snapshot.Requirements.OutOfScope = []string{"PI RPC"}
	snapshot.Requirements.AcceptanceCriteria = []string{"完整目录树存在", "旧资料不删除"}
	snapshot.Requirements.Risks = []string{"符号链接越界"}
	snapshot.Requirements.Document = "# 当前需求正文\n\n必须保持人工确认。"
	snapshot.Requirements.ExecutionPrompt = "只根据已确认范围执行。"
	snapshot.Requirements.ApprovedRevisions = []ApprovedRequirementRevision{
		{
			Version:         1,
			Document:        "# 需求 v1\n\n保持人工门禁。",
			ExecutionPrompt: "只实施阶段 1。",
			ConfirmedAt:     "2026-08-01T08:30:00Z",
		},
	}
	snapshot.Requirements.Interview.ProjectObservations = []ProjectObservation{
		{ID: "observation-1", Content: "Open 使用旧 initialSchema。", FilePath: "internal/storage/sqlite_store.go", LineRange: "1-40"},
	}
	raw, _ := json.Marshal(snapshot)
	snapshot.RawJSON = raw
	return snapshot
}

func TestEnsureCreatesCompleteWorkspaceAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	snapshot := sampleSnapshot("task_workspace")
	service := fixedService()

	first, err := service.Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("ensure task workspace: %v", err)
	}
	if first.ManifestRevision != 1 || first.WorkspaceID == "" {
		t.Fatalf("unexpected first manifest result %#v", first)
	}
	for _, directory := range managedDirectories {
		info, err := os.Lstat(filepath.Join(root, snapshot.ID, filepath.FromSlash(directory)))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("managed directory %q: info %#v, error %v", directory, info, err)
		}
	}
	for _, name := range []string{
		".btask/manifest.json",
		".btask/permissions.json",
		".btask/resources.json",
		".btask/sessions.json",
		"context/task.md",
		"context/requirements/current.md",
		"context/requirements/approved-v1.md",
		"context/acceptance-criteria.md",
	} {
		info, err := os.Lstat(filepath.Join(root, snapshot.ID, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("managed file %q: info %#v, error %v", name, info, err)
		}
	}
	currentRequirements := readFile(t, root, snapshot.ID, "context/requirements/current.md")
	if !strings.Contains(currentRequirements, "当前需求正文") || !strings.Contains(currentRequirements, "只根据已确认范围执行") {
		t.Fatalf("current requirements are incomplete: %s", currentRequirements)
	}
	manifestPath := filepath.Join(root, snapshot.ID, ".btask", "manifest.json")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	second, err := service.Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("repeat ensure task workspace: %v", err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read repeated manifest: %v", err)
	}
	if second.WorkspaceID != first.WorkspaceID || second.ManifestRevision != 1 || string(after) != string(before) {
		t.Fatalf("Ensure was not idempotent: first %#v, second %#v", first, second)
	}
}

func TestEnsureUpdatesMutableContextAndNeverOverwritesApprovedRevision(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	snapshot := sampleSnapshot("task_immutable")
	first, err := service.Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	snapshot.Summary = "更新后的摘要"
	raw, _ := json.Marshal(snapshot)
	snapshot.RawJSON = raw
	second, err := service.Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("update mutable context: %v", err)
	}
	if second.ManifestRevision != first.ManifestRevision+1 {
		t.Fatalf("manifest revision did not advance: %#v then %#v", first, second)
	}
	taskMarkdown := readFile(t, root, snapshot.ID, "context/task.md")
	if !strings.Contains(taskMarkdown, "更新后的摘要") {
		t.Fatalf("mutable task context was not updated: %s", taskMarkdown)
	}

	approvedPath := "context/requirements/approved-v1.md"
	original := readFile(t, root, snapshot.ID, approvedPath)
	snapshot.Requirements.ApprovedRevisions[0].Document = "篡改后的历史需求"
	raw, _ = json.Marshal(snapshot)
	snapshot.RawJSON = raw
	if _, err := service.Ensure(root, snapshot); err == nil || !strings.Contains(err.Error(), "immutable file") {
		t.Fatalf("expected immutable revision rejection, got %v", err)
	}
	if current := readFile(t, root, snapshot.ID, approvedPath); current != original {
		t.Fatal("approved requirement revision was overwritten")
	}
}

func TestEnsureRestoresMissingManagedDirectory(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	snapshot := sampleSnapshot("task_restore_directory")
	first, err := service.Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	directory := filepath.Join(root, snapshot.ID, "artifacts", "exports")
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	contextPath := filepath.Join(root, snapshot.ID, "context", "task.md")
	if err := os.Chmod(contextPath, 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := service.Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("restore workspace directory: %v", err)
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("managed directory was not restored: %#v, error %v", info, err)
	}
	contextInfo, err := os.Stat(contextPath)
	if err != nil || contextInfo.Mode().Perm() != 0o600 {
		t.Fatalf("managed file permissions were not restored: %#v, error %v", contextInfo, err)
	}
	if second.ManifestRevision != first.ManifestRevision+1 {
		t.Fatalf("directory recovery did not advance manifest revision: %#v then %#v", first, second)
	}
}

func TestEnsureImportsSourcesAttachmentsAndPreservesLegacyBytes(t *testing.T) {
	root := t.TempDir()
	taskID := "task_legacy"
	legacyRoot := filepath.Join(root, taskID)
	if err := os.MkdirAll(filepath.Join(legacyRoot, "files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacyRoot, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(legacyRoot, legacyTaskMarker),
		[]byte(`{"id":"task_legacy"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, legacyContext), []byte(`{"title":"旧任务"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyDocument := []byte("用户放入的旧文档")
	if err := os.WriteFile(filepath.Join(legacyRoot, "files", "user.txt"), legacyDocument, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "images", "old.png"), tinyPNG, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := fixedService().Ensure(root, sampleSnapshot(taskID))
	if err != nil {
		t.Fatalf("migrate legacy task: %v", err)
	}
	if result.LegacyContextPath != filepath.Join(legacyRoot, legacyContext) {
		t.Fatalf("unexpected legacy context path %q", result.LegacyContextPath)
	}
	if string(readBytes(t, filepath.Join(legacyRoot, "files", "user.txt"))) != string(legacyDocument) {
		t.Fatal("legacy document changed or was removed")
	}
	if string(readBytes(t, filepath.Join(legacyRoot, "images", "old.png"))) != string(tinyPNG) {
		t.Fatal("legacy image changed or was removed")
	}
	for _, directory := range []string{
		"sources/manual",
		"sources/plane",
		"sources/chats",
		"sources/project-observations",
		"attachments/documents",
	} {
		entries, err := os.ReadDir(filepath.Join(legacyRoot, filepath.FromSlash(directory)))
		if err != nil || len(entries) == 0 {
			t.Fatalf("expected imported files in %q, entries %v, error %v", directory, entries, err)
		}
	}
	imageEntries, err := os.ReadDir(filepath.Join(legacyRoot, "attachments", "images"))
	if err != nil {
		t.Fatal(err)
	}
	if len(imageEntries) != 1 {
		t.Fatalf("embedded and legacy duplicate image bytes were not deduplicated: %v", imageEntries)
	}
	imageHash := sha256.Sum256(tinyPNG)
	foundHash := false
	foundLegacyContext := false
	for _, resource := range result.Resources {
		if resource.SHA256 == hex.EncodeToString(imageHash[:]) && resource.SourceType == "image" {
			foundHash = true
		}
		if resource.SourceType == "legacy_context" {
			foundLegacyContext = true
		}
	}
	if !foundHash {
		t.Fatal("image resource hash was not persisted")
	}
	if !foundLegacyContext {
		t.Fatal("legacy context.json was not copied into immutable sources")
	}
}

func TestEnsureValidatesDataURLMIMEAndCanRetryLegacyMigration(t *testing.T) {
	root := t.TempDir()
	snapshot := sampleSnapshot("task_retry")
	badURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(tinyPNG)
	snapshot.Summary = badURL
	snapshot.RawJSON, _ = json.Marshal(snapshot)
	if _, err := fixedService().Ensure(root, snapshot); err == nil || !strings.Contains(err.Error(), "does not match bytes") {
		t.Fatalf("expected MIME mismatch, got %v", err)
	}

	snapshot = sampleSnapshot("task_retry")
	taskRoot := filepath.Join(root, snapshot.ID)
	if err := os.MkdirAll(filepath.Join(taskRoot, "files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(taskRoot, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskRoot, legacyTaskMarker), []byte(`{"id":"task_retry"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	link := filepath.Join(taskRoot, "files", "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := fixedService().Ensure(root, snapshot); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected interrupted migration, got %v", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("修复后的旧资料"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixedService().Ensure(root, snapshot); err != nil {
		t.Fatalf("retry migration: %v", err)
	}
	if string(readBytes(t, link)) != "修复后的旧资料" {
		t.Fatal("retry removed legacy input")
	}
}

func TestTwoTaskWorkspacesKeepSameNamedAttachmentsIsolated(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	first := sampleSnapshot("task_first")
	second := sampleSnapshot("task_second")
	first.Evidence = []Evidence{{ID: "file", Type: "file", Title: "same.md", Content: "first"}}
	second.Evidence = []Evidence{{ID: "file", Type: "file", Title: "same.md", Content: "second"}}
	first.RawJSON, _ = json.Marshal(first)
	second.RawJSON, _ = json.Marshal(second)
	if _, err := service.Ensure(root, first); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(root, second); err != nil {
		t.Fatal(err)
	}
	firstEntries, _ := os.ReadDir(filepath.Join(root, first.ID, "attachments", "documents"))
	secondEntries, _ := os.ReadDir(filepath.Join(root, second.ID, "attachments", "documents"))
	if len(firstEntries) != 1 || len(secondEntries) != 1 || firstEntries[0].Name() == secondEntries[0].Name() {
		t.Fatalf("unexpected isolated attachment names: %v and %v", firstEntries, secondEntries)
	}
	if string(readBytes(t, filepath.Join(root, first.ID, "attachments", "documents", firstEntries[0].Name()))) != "first" {
		t.Fatal("first task attachment crossed task scope")
	}
	if string(readBytes(t, filepath.Join(root, second.ID, "attachments", "documents", secondEntries[0].Name()))) != "second" {
		t.Fatal("second task attachment crossed task scope")
	}
}

func TestEnsureRejectsUnavailableRootAndOversizedLegacyFile(t *testing.T) {
	service := fixedService()
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := service.Ensure(missing, sampleSnapshot("task_missing_root")); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable root error, got %v", err)
	}

	root := t.TempDir()
	taskID := "task_large_legacy"
	taskRoot := filepath.Join(root, taskID)
	if err := os.MkdirAll(filepath.Join(taskRoot, "files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(taskRoot, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskRoot, legacyTaskMarker), []byte(`{"id":"task_large_legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	largePath := filepath.Join(taskRoot, "files", "large.bin")
	large, err := os.OpenFile(largePath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(MaxAttachmentBytes + 1); err != nil {
		_ = large.Close()
		t.Fatal(err)
	}
	if err := large.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(root, sampleSnapshot(taskID)); err == nil || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("expected oversized legacy file rejection, got %v", err)
	}
}

func TestEnsureRejectsLegacyImageWithMismatchedBytes(t *testing.T) {
	root := t.TempDir()
	taskID := "task_legacy_mime"
	taskRoot := filepath.Join(root, taskID)
	if err := os.MkdirAll(filepath.Join(taskRoot, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(taskRoot, legacyTaskMarker),
		[]byte(`{"id":"task_legacy_mime"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(taskRoot, "images", "not-really.png"),
		[]byte("plain text"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := (Service{}).Ensure(root, sampleSnapshot(taskID))
	if err == nil || !strings.Contains(err.Error(), "not a supported") {
		t.Fatalf("expected legacy image MIME rejection, got %v", err)
	}
	if content := readBytes(t, filepath.Join(taskRoot, "images", "not-really.png")); string(content) != "plain text" {
		t.Fatal("legacy image changed after failed MIME validation")
	}
}

func TestEnsureRecordsUnknownEngineIdentityWithoutRewritingSnapshot(t *testing.T) {
	root := t.TempDir()
	snapshot := sampleSnapshot("task_unknown_engine")
	snapshot.Development.Engine = "future-engine"
	snapshot.Requirements.Interview.Analyst = "future-analyst"

	result, err := fixedService().Ensure(root, snapshot)
	if err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "development.engine 含未知") ||
		!strings.Contains(warnings, "interview.analyst 含未知") {
		t.Fatalf("unknown engine identity was not retained as a warning: %v", result.Warnings)
	}
	if snapshot.Development.Engine != "future-engine" ||
		snapshot.Requirements.Interview.Analyst != "future-analyst" {
		t.Fatal("unknown imported engine identity was rewritten")
	}
}

func mustDecodeBase64(value string) []byte {
	content, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return content
}

func readFile(t *testing.T, root string, taskID string, name string) string {
	t.Helper()
	return string(readBytes(t, filepath.Join(root, taskID, filepath.FromSlash(name))))
}

func readBytes(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %q: %v", name, err)
	}
	return content
}
