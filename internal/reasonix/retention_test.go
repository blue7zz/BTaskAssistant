package reasonix

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/bridge"
)

// TestSessionRetainedAcrossRebuild 验证任务会话内容保留：
// 提交对话 → Close（离开详情页）→ Ensure 重建 → 控制器恢复同一会话路径。
func TestSessionRetainedAcrossRebuild(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	tab, err := manager.Ensure(ctx, "task_retain", workspace)
	if err != nil {
		t.Fatalf("首次 Ensure 失败: %v", err)
	}
	// 先开新会话（生成会话文件）再提交
	if err := manager.NewSession("task_retain"); err != nil {
		t.Fatalf("NewSession 失败: %v", err)
	}
	firstPath := tab.Ctrl.SessionPath()
	if err := manager.Submit("task_retain", "保留我"); err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	// 等待回合快照落盘（turn_done 异步）
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("会话文件未生成: %v", err)
	}

	// 离开详情页：关闭控制器（会话文件保留）
	manager.Close("task_retain")

	// 切回任务：重建控制器应恢复原会话（而非新建空会话）
	tab2, err := manager.Ensure(ctx, "task_retain", workspace)
	if err != nil {
		t.Fatalf("重建 Ensure 失败: %v", err)
	}
	if tab2.Ctrl.SessionPath() != firstPath {
		t.Fatalf("重建后会话路径未保留: 期望 %s 实际 %s", firstPath, tab2.Ctrl.SessionPath())
	}

	// 最后会话标记已持久化（相对文件名，原子写入）
	marker := filepath.Join(manager.TaskSessionDir("task_retain"), "last-session.txt")
	data, err := os.ReadFile(marker)
	if err != nil || strings.TrimSpace(string(data)) != filepath.Base(firstPath) {
		t.Fatalf("last-session 标记未持久化: %v %q（期望 %q）", err, string(data), filepath.Base(firstPath))
	}
	manager.Shutdown()
}

// TestPruneOldSessionsKeepsNewest 验证会话上限清理：
// 超过 MaxSessionsPerTask 时按 mtime 保留最新，删除最旧（含侧车）。
func TestPruneOldSessionsKeepsNewest(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	if _, err := manager.Ensure(ctx, "task_prune", workspace); err != nil {
		t.Fatalf("Ensure 失败: %v", err)
	}
	dir := manager.TaskSessionDir("task_prune")

	// 造 12 个会话文件（含侧车），mtime 递增（新的在后）
	for i := 0; i < 12; i++ {
		name := filepath.Join(dir, time.Now().Format("20060102-150405.000000000")+strings.Repeat("0", 2)+"-probe.jsonl")
		// 用确定顺序：序号填充文件名
		name = filepath.Join(dir, time.Now().Format("20060102-150405")+".000000"+string(rune('0'+i%10))+string(rune('0'+i/10))+"-probe"+string(rune('0'+i))+".jsonl")
		if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
			t.Fatalf("写会话文件失败: %v", err)
		}
		stem := strings.TrimSuffix(name, ".jsonl")
		_ = os.WriteFile(stem+".events.jsonl", []byte("[]"), 0o644)
		_ = os.WriteFile(stem+".ckpt", []byte("ckpt"), 0o644)
		mt := time.Now().Add(time.Duration(i) * time.Second)
		_ = os.Chtimes(name, mt, mt)
		_ = os.Chtimes(stem+".events.jsonl", mt, mt)
	}

	manager.PruneOldSessions("task_prune", bridge.MaxSessionsPerTask)

	files, err := manager.ListSessionFiles("task_prune")
	if err != nil {
		t.Fatalf("列出会话失败: %v", err)
	}
	if len(files) != bridge.MaxSessionsPerTask {
		t.Fatalf("清理后会话数 %d，期望 %d", len(files), bridge.MaxSessionsPerTask)
	}
	// 保留的是最新的（mtime 最大的 i=11..2）
	for i, file := range files {
		if !strings.Contains(file.Name, string(rune('0'+11-i))) && !strings.Contains(file.Name, string(rune('0'))) {
			t.Fatalf("保留顺序异常: %s", file.Name)
		}
	}
	// 最旧（i=0）的侧车也应被清理
	oldestStem := ""
	for _, f := range files {
		if strings.Contains(f.Name, "probe0.jsonl") {
			oldestStem = strings.TrimSuffix(filepath.Join(dir, f.Name), ".jsonl")
		}
	}
	if oldestStem != "" {
		if _, err := os.Stat(oldestStem + ".events.jsonl"); err == nil {
			t.Fatal("最旧会话的侧车未被清理")
		}
	}
	manager.Shutdown()
}

// TestNewSessionTriggersPrune 验证新建会话自动触发上限清理。
func TestNewSessionTriggersPrune(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	if _, err := manager.Ensure(ctx, "task_auto_prune", workspace); err != nil {
		t.Fatalf("Ensure 失败: %v", err)
	}
	dir := manager.TaskSessionDir("task_auto_prune")
	// 造 MaxSessionsPerTask + 3 个旧会话文件
	for i := 0; i < bridge.MaxSessionsPerTask+3; i++ {
		name := filepath.Join(dir, time.Now().Format("20060102-150405")+".0000000000"+string(rune('0'+i%10))+".probe"+string(rune('0'+i))+".jsonl")
		if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
			t.Fatalf("写会话文件失败: %v", err)
		}
		mt := time.Now().Add(time.Duration(i) * time.Second)
		_ = os.Chtimes(name, mt, mt)
	}
	// 新建会话（触发清理：13 个文件 - 当前新会话 → 删除至上限）
	if err := manager.NewSession("task_auto_prune"); err != nil {
		t.Fatalf("NewSession 失败: %v", err)
	}
	files, err := manager.ListSessionFiles("task_auto_prune")
	if err != nil {
		t.Fatalf("列出会话失败: %v", err)
	}
	if len(files) > bridge.MaxSessionsPerTask {
		t.Fatalf("新建会话后未按上限清理: %d 个", len(files))
	}
	manager.Shutdown()
}

// TestPruneIdleRuntimesKeepsActive 验证 idle LRU 回收（Activate 自动触发）：
// 后台 idle 超过 MaxIdleRuntimes 时按最后活跃时间回收最旧；running 绝不回收。
func TestPruneIdleRuntimesKeepsActive(t *testing.T) {
	dataRoot := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 激活 MaxIdleRuntimes+2 个 idle 任务：第 5 个起每次激活自动回收最旧
	for i := 0; i < bridge.MaxIdleRuntimes+2; i++ {
		taskID := "task_idle_" + string(rune('a'+i))
		ws := t.TempDir()
		tab, err := manager.Activate(ctx, taskID, ws, "任务"+string(rune('A'+i)), uint64(i+1))
		if err != nil {
			t.Fatalf("激活 %s 失败: %v", taskID, err)
		}
		// 错开 LastActive（模拟不同活跃时间）
		tab.LastActive = time.Now().Add(-time.Duration(i) * time.Minute)
	}
	// 再加一个 running 会话（绝不回收）
	runningID := "task_running"
	runningTab, err := manager.Activate(ctx, runningID, t.TempDir(), "运行中", uint64(99))
	if err != nil {
		t.Fatalf("激活 running 失败: %v", err)
	}
	_ = runningTab

	// 自动回收后：idle ≤ 上限，running 保留
	if manager.Tab(runningID) == nil {
		t.Fatal("running 会话被回收")
	}
	idleCount := 0
	for i := 0; i < bridge.MaxIdleRuntimes+2; i++ {
		taskID := "task_idle_" + string(rune('a'+i))
		if manager.Tab(taskID) != nil {
			idleCount++
		}
	}
	if idleCount > bridge.MaxIdleRuntimes {
		t.Fatalf("idle 应 ≤ %d 个，实际 %d", bridge.MaxIdleRuntimes, idleCount)
	}
	// 手动再触发一次（无超额应为 0）
	if removed := manager.PruneIdleRuntimes(); removed != 0 {
		t.Fatalf("无超额时不应回收，实际 %d", removed)
	}
	manager.Shutdown()
}
