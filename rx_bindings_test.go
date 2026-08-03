package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/bridge"

	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

func TestRxWorkspacePath(t *testing.T) {
	root := t.TempDir()

	t.Run("normal path resolves", func(t *testing.T) {
		got, err := rxWorkspacePath(root, "src/main.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(root, "src", "main.go")
		if got != want {
			t.Fatalf("want %s, got %s", want, got)
		}
	})

	t.Run("dot paths normalize", func(t *testing.T) {
		got, err := rxWorkspacePath(root, "./src/../src/a.ts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(root, "src", "a.ts")
		if got != want {
			t.Fatalf("want %s, got %s", want, got)
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		for _, rel := range []string{"../etc/passwd", "../../secret", "a/../../etc/hosts"} {
			if _, err := rxWorkspacePath(root, rel); err == nil {
				t.Fatalf("expected rejection for %q", rel)
			}
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		if _, err := rxWorkspacePath(root, "/etc/passwd"); err == nil {
			t.Fatal("expected rejection for absolute path")
		}
	})
}

func TestRxValidateSessionPath(t *testing.T) {
	dataRoot := t.TempDir()
	app := &App{
		rxManager: bridge.NewManager(dataRoot, nil),
		rxTabs:    map[string]rxTabEntry{},
		rxMu:      &sync.Mutex{},
	}
	tabID := "task_abcd"
	app.rxTabs[tabID] = rxTabEntry{taskID: "task_1"}

	sessionDir := app.rxManager.TaskSessionDir("task_1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(sessionDir, "20260101-000000.000000000-demo.jsonl")
	if err := os.WriteFile(inside, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("session file inside dir allowed", func(t *testing.T) {
		if err := app.rxValidateSessionPath(inside); err != nil {
			t.Fatalf("unexpected rejection: %v", err)
		}
	})

	t.Run("outside file rejected", func(t *testing.T) {
		outside := filepath.Join(dataRoot, "victim.txt")
		if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.rxValidateSessionPath(outside); err == nil {
			t.Fatal("expected rejection for outside file")
		}
		if _, err := os.Stat(outside); err != nil {
			t.Fatalf("outside file must not be touched: %v", err)
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		evil := filepath.Join(sessionDir, "..", "..", "..", "etc", "passwd")
		if err := app.rxValidateSessionPath(evil); err == nil {
			t.Fatal("expected rejection for traversal")
		}
	})
}

// TestDeleteCurrentSessionRotates 验证删除当前激活会话时控制器轮换到新文件
// （防止 turn_done 快照复活已删文件）。
func TestDeleteCurrentSessionRotates(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	events := make(chan map[string]any, 4096)
	manager := bridge.NewManager(dataRoot, func(_ string, p map[string]any) { events <- p })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	tab, err := manager.Ensure(ctx, "task_del", workspace)
	if err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_del")
	if err := manager.Submit("task_del", "删除测试消息"); err != nil {
		t.Fatal(err)
	}
	// 等回合结束落盘
	deadline := time.After(120 * time.Second)
	for {
		path := manager.CurrentSessionPath("task_del")
		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("等待会话文件超时")
		case <-time.After(300 * time.Millisecond):
		}
	}
	// 等回合完全停止（先取消，避免模型响应慢导致测试超时）
	stopDeadline := time.After(60 * time.Second)
	for tab.Ctrl.Running() {
		tab.Ctrl.Cancel()
		select {
		case <-stopDeadline:
			t.Fatal("回合未停止")
		case <-time.After(200 * time.Millisecond):
		}
	}

	oldPath := manager.CurrentSessionPath("task_del")
	app := &App{
		rxManager: manager,
		rxMu:      &sync.Mutex{},
		rxTabs:    map[string]rxTabEntry{tab.ID: {taskID: "task_del"}},
	}

	if err := app.DeleteSession(oldPath); err != nil {
		t.Fatalf("DeleteSession 失败: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("旧会话文件未删除: %v", err)
	}
	newPath := manager.CurrentSessionPath("task_del")
	if newPath == oldPath || newPath == "" {
		t.Fatalf("控制器未轮换到新文件: %q", newPath)
	}
	// 新会话文件在首个回合结束才落盘；轮换后提交不应报错且事件正常
	if err := manager.Submit("task_del", "轮换后继续"); err != nil {
		t.Fatalf("轮换后提交失败: %v", err)
	}
	deadline = time.After(120 * time.Second)
	for {
		if info, statErr := os.Stat(newPath); statErr == nil && info.Size() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("新会话文件未落盘")
		case <-time.After(300 * time.Millisecond):
		}
	}
	manager.Close("task_del")
}

// TestPreviewSessionReads 验证历史预览只读读取 + 路径校验。
func TestPreviewSessionReads(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	events := make(chan map[string]any, 4096)
	manager := bridge.NewManager(dataRoot, func(_ string, p map[string]any) { events <- p })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	tab, err := manager.Ensure(ctx, "task_pv", workspace)
	if err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_pv")
	if err := manager.Submit("task_pv", "预览测试消息"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(120 * time.Second)
	for {
		path := manager.CurrentSessionPath("task_pv")
		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("等待会话文件超时")
		case <-time.After(300 * time.Millisecond):
		}
	}
	// 等回合停止
	stopDeadline := time.After(60 * time.Second)
	for tab.Ctrl.Running() {
		tab.Ctrl.Cancel()
		select {
		case <-stopDeadline:
			t.Fatal("回合未停止")
		case <-time.After(200 * time.Millisecond):
		}
	}

	app := &App{
		rxManager: manager,
		rxMu:      &sync.Mutex{},
		rxTabs:    map[string]rxTabEntry{tab.ID: {taskID: "task_pv"}},
	}
	path := manager.CurrentSessionPath("task_pv")
	messages, err := app.PreviewSession(path)
	if err != nil {
		t.Fatalf("PreviewSession 失败: %v", err)
	}
	found := false
	for _, message := range messages {
		if message.Role == "user" && strings.Contains(message.Content, "预览测试消息") {
			found = true
		}
	}
	if !found {
		t.Fatalf("预览未包含用户消息: %d 条", len(messages))
	}
	// 路径校验：目录外拒绝
	if _, err := app.PreviewSession(filepath.Join(dataRoot, "outside.jsonl")); err == nil {
		t.Fatal("目录外路径应被拒绝")
	}
	manager.Close("task_pv")
}

// TestRxValidateSessionPathSymlinkEscape 验证符号链接逃逸被拒绝：
// 会话目录内的 .jsonl 指向外部文件时不得通过校验。
func TestRxValidateSessionPathSymlinkEscape(t *testing.T) {
	dataRoot := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.jsonl")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{
		rxManager: bridge.NewManager(dataRoot, nil),
		rxTabs:    map[string]rxTabEntry{},
		rxMu:      &sync.Mutex{},
	}
	sessionDir := app.rxManager.TaskSessionDir("task_1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sessionDir, "20260101-000000.000000000-evil.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("环境不支持符号链接: %v", err)
	}
	if err := app.rxValidateSessionPath(link); err == nil {
		t.Fatal("符号链接逃逸应被拒绝")
	}
}

// TestRxWorkspaceForTask 验证 RX 始终使用任务自己的文件空间，Git Binding
// 是否存在及其状态都不改变 Reasonix 的工作目录。
func TestRxWorkspaceForTask(t *testing.T) {
	store := storage.NewSQLiteStoreAt(filepath.Join(t.TempDir(), "btask.db"))
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Save(`{"state":{"tasks":[
		{"id":"task_nobind","title":"无 Git 绑定","revision":1},
		{"id":"task_wt","title":"已有 Git 绑定","revision":1},
		{"id":"task_pending","title":"绑定创建中","revision":1}
	]}}`); err != nil {
		t.Fatalf("准备任务文件空间失败: %v", err)
	}
	app := &App{
		store:     store,
		rxManager: bridge.NewManager(t.TempDir(), nil),
		rxTabs:    map[string]rxTabEntry{},
		rxMu:      &sync.Mutex{},
	}

	assertTaskWorkspace := func(t *testing.T, taskID string) {
		t.Helper()
		workspace, err := store.TaskWorkspace(taskID)
		if err != nil {
			t.Fatalf("读取任务文件空间失败: %v", err)
		}
		identity, err := app.rxWorkspaceForTask(taskID)
		if err != nil {
			t.Fatalf("解析 RX 工作区失败: %v", err)
		}
		real, err := filepath.EvalSymlinks(workspace.RootPath)
		if err != nil {
			t.Fatalf("解析任务文件空间真实路径失败: %v", err)
		}
		if identity.RootPath != real {
			t.Fatalf("RX 未使用任务文件空间: %s != %s", identity.RootPath, real)
		}
		if identity.BindingID != "" || identity.Generation != workspace.WorkspaceID {
			t.Fatalf("RX 工作区身份不应来自 Git Binding: %+v", identity)
		}
	}

	t.Run("no binding uses task workspace", func(t *testing.T) {
		assertTaskWorkspace(t, "task_nobind")
	})

	t.Run("ready binding still uses task workspace", func(t *testing.T) {
		source := t.TempDir()
		worktree := t.TempDir()
		binding := storage.GitBindingRecord{
			ID:             "bind_1",
			TaskID:         "task_wt",
			SourcePath:     source,
			SourceRealPath: source,
			WorktreePath:   &worktree,
			CommonGitDir:   source,
			Branch:         ptr("main"),
			BaselineCommit: "abc123",
			State:          "ready",
			CreatedAt:      time.Now().Format(time.RFC3339),
			UpdatedAt:      time.Now().Format(time.RFC3339),
		}
		if err := app.store.UpsertGitBinding(binding); err != nil {
			t.Fatalf("UpsertGitBinding 失败: %v", err)
		}
		assertTaskWorkspace(t, "task_wt")
	})

	t.Run("non-ready binding does not block task workspace", func(t *testing.T) {
		source := t.TempDir()
		worktree := t.TempDir()
		binding := storage.GitBindingRecord{
			ID:             "bind_2",
			TaskID:         "task_pending",
			SourcePath:     source,
			SourceRealPath: source,
			WorktreePath:   &worktree,
			CommonGitDir:   source,
			BaselineCommit: "def456",
			State:          "creating",
			CreatedAt:      time.Now().Format(time.RFC3339),
			UpdatedAt:      time.Now().Format(time.RFC3339),
		}
		if err := app.store.UpsertGitBinding(binding); err != nil {
			t.Fatal(err)
		}
		assertTaskWorkspace(t, "task_pending")
	})
}

func ptr(s string) *string { return &s }
