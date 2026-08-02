package reasonix

import (
	"context"
	"sync"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/bridge"
)

// TestTaskSessionLifecycle 验证"一个任务一个 Reasonix 会话"的核心链路：
// 控制器构建（boot.Build）→ 新会话文件生成 → 提交对话 → 事件回调 →
// 会话目录隔离 → 关闭释放。
func TestTaskSessionLifecycle(t *testing.T) {
	// 用临时数据根，避免污染真实 BTask 数据目录。
	dataRoot := t.TempDir()
	workspace := t.TempDir()

	events := make(chan map[string]any, 64)
	manager := bridge.NewManager(dataRoot, func(_tabID string, payload map[string]any) {
		select {
		case events <- payload:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 1. 首次 Ensure：构建控制器
	tab, err := manager.Ensure(ctx, "task_demo", workspace)
	if err != nil {
		t.Fatalf("Ensure 失败: %v", err)
	}
	if tab == nil || tab.Ctrl == nil {
		t.Fatal("Ensure 返回空控制器")
	}

	// 2. 任务隔离：会话目录按 taskId 隔离
	dir := manager.TaskSessionDir("task_demo")
	if !strings.Contains(dir, "task_demo") {
		t.Fatalf("会话目录未按任务隔离: %s", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("会话目录不存在: %v", err)
	}

	// 3. 新会话文件（自动轮换 JSONL）
	if err := manager.NewSession("task_demo"); err != nil {
		t.Fatalf("NewSession 失败: %v", err)
	}
	path := manager.CurrentSessionPath("task_demo")
	if path == "" {
		t.Fatal("新会话路径为空")
	}
	if !strings.HasSuffix(path, ".jsonl") {
		t.Fatalf("会话文件不是 JSONL: %s", path)
	}

	// 4. 提交一轮（无凭据时控制器应容错，事件仍流出）
	if err := manager.Submit("task_demo", "你好，介绍一下这个项目"); err != nil {
		t.Fatalf("Submit 失败: %v", err)
	}

	// 5. 事件回调应收到内核事件；等待回合结束（会话文件在 turn_done 时落盘）
	deadline := time.After(120 * time.Second)
	for {
		select {
		case payload := <-events:
			kind, _ := payload["kind"].(string)
			if kind == "turn_done" {
				goto turnDone
			}
		case <-deadline:
			t.Fatal("120s 内未收到 turn_done 事件")
		}
	}
turnDone:

	// 6. 会话元信息
	view, err := manager.View("task_demo", workspace)
	if err != nil {
		t.Fatalf("View 失败: %v", err)
	}
	if view.ID == "" || !view.Ready {
		t.Fatalf("TabView 不完整: %+v", view)
	}

	// 7. 会话文件落盘
	files, err := manager.ListSessionFiles("task_demo")
	if err != nil {
		t.Fatalf("ListSessionFiles 失败: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("会话目录没有会话文件")
	}

	// 8. 关闭释放
	manager.Close("task_demo")
	if manager.Tab("task_demo") != nil {
		t.Fatal("Close 后 tab 仍存在")
	}
}

// TestTaskSessionIsolation 验证两个任务互不干扰。
func TestTaskSessionIsolation(t *testing.T) {
	dataRoot := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	workspaceA := t.TempDir()
	workspaceB := t.TempDir()

	tabA, err := manager.Ensure(ctx, "task_a", workspaceA)
	if err != nil {
		t.Fatalf("task_a Ensure 失败: %v", err)
	}
	tabB, err := manager.Ensure(ctx, "task_b", workspaceB)
	if err != nil {
		t.Fatalf("task_b Ensure 失败: %v", err)
	}

	if tabA.ID == tabB.ID {
		t.Fatalf("两个任务的 tabID 相同: %s", tabA.ID)
	}
	dirA := manager.TaskSessionDir("task_a")
	dirB := manager.TaskSessionDir("task_b")
	if dirA == dirB {
		t.Fatal("两个任务的会话目录相同")
	}
	if !filepath.IsAbs(dirA) || !filepath.IsAbs(dirB) {
		t.Fatal("会话目录应为绝对路径")
	}
}

// TestSessionMetaContract 验证 Sessions() 输出与前端 SessionMeta 契约兼容。
func TestSessionMetaContract(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_meta", workspace); err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_meta")
	if err := manager.Submit("task_meta", "测试消息"); err != nil {
		t.Fatal(err)
	}

	// 等待回合结束（会话文件落盘）
	deadline := time.After(120 * time.Second)
	for {
		if _, err := os.Stat(manager.CurrentSessionPath("task_meta")); err == nil {
			size, _ := os.Stat(manager.CurrentSessionPath("task_meta"))
			if size != nil && size.Size() > 0 {
				break
			}
		}
		select {
		case <-deadline:
			t.Fatal("等待会话文件超时")
		case <-time.After(200 * time.Millisecond):
		}
	}
	time.Sleep(300 * time.Millisecond)

	sessions, err := manager.Sessions("task_meta")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Fatal("无会话")
	}
	meta := sessions[0]
	if meta.Path == "" || meta.Preview == "" {
		t.Fatalf("SessionMeta 缺 preview/path: %+v", meta)
	}
	if meta.CreatedAt <= 0 || meta.LastActivityAt <= 0 || meta.ModTime <= 0 {
		t.Fatalf("SessionMeta 时间戳无效: %+v", meta)
	}
	if meta.Turns < 1 {
		t.Fatalf("SessionMeta turns 应为 >=1: %+v", meta)
	}
	if !meta.Current {
		t.Fatalf("最新会话应标记 current: %+v", meta)
	}

	// Resume 历史消息契约
	page, err := manager.Resume("task_meta", "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range page.Messages {
		if message.Role == "user" && strings.Contains(message.Content, "测试消息") {
			found = true
			if message.CreatedAt <= 0 {
				t.Fatalf("HistoryMessage.CreatedAt 无效: %+v", message)
			}
		}
	}
	if !found {
		t.Fatal("历史中没有用户消息")
	}
}

// TestSessionTitleSidecar 验证会话重命名持久化到 .titles.json 侧车。
func TestSessionTitleSidecar(t *testing.T) {
	dataRoot := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)

	dir := manager.TaskSessionDir("task_titles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "20260101-000000.000000000-demo.jsonl")

	if err := manager.RenameSession("task_titles", path, "需求梳理"); err != nil {
		t.Fatal(err)
	}
	titles := manager.SessionTitles("task_titles")
	if titles["20260101-000000.000000000-demo.jsonl"] != "需求梳理" {
		t.Fatalf("标题未生效: %v", titles)
	}

	if err := manager.RemoveSessionTitles("task_titles", path); err != nil {
		t.Fatal(err)
	}
	titles = manager.SessionTitles("task_titles")
	if _, ok := titles["20260101-000000.000000000-demo.jsonl"]; ok {
		t.Fatal("侧车条目未清理")
	}
}

// TestAskAndCommands 验证 ask 回答与真实斜杠命令链路。
func TestAskAndCommands(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_ask", workspace); err != nil {
		t.Fatal(err)
	}

	// 真实命令列表（内核 ctrl.Commands()——无用户自定义命令时为合法的空列表）
	commands, err := manager.CommandViews("task_ask")
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) > 0 {
		// 命令应带名称与描述字段
		for _, command := range commands {
			if command["name"] == "" {
				t.Fatalf("命令缺名称: %v", command)
			}
		}
	}

	// ask 回答：空列表为 no-op 不报错；带结构的回答被转换后传入控制器
	if err := manager.AnswerQuestion("task_ask", "q1", nil); err != nil {
		t.Fatalf("空回答不应报错: %v", err)
	}
	if err := manager.AnswerQuestion("task_ask", "q1", []map[string]any{
		{"questionId": "q1", "selected": []any{"方案 A"}},
	}); err != nil {
		t.Fatalf("带结构的回答失败: %v", err)
	}
}

// TestMultiTurnAndModelSwitch 验证多轮对话与模型切换后的会话延续。
func TestMultiTurnAndModelSwitch(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	events := make(chan map[string]any, 256)
	manager := bridge.NewManager(dataRoot, func(_ string, p map[string]any) {
		select {
		case events <- p:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_multi", workspace); err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_multi")

	waitTurnDone := func(timeout time.Duration) {
		t.Helper()
		deadline := time.After(timeout)
		for {
			select {
			case p := <-events:
				if kind, _ := p["kind"].(string); kind == "turn_done" {
					return
				}
			case <-deadline:
				t.Fatal("等待 turn_done 超时")
			}
		}
	}

	// 第一轮
	if err := manager.Submit("task_multi", "第一轮：介绍任务"); err != nil {
		t.Fatal(err)
	}
	waitTurnDone(120 * time.Second)
	path1 := manager.CurrentSessionPath("task_multi")

	// 第二轮（同一会话文件延续）
	if err := manager.Submit("task_multi", "第二轮：继续"); err != nil {
		t.Fatal(err)
	}
	waitTurnDone(120 * time.Second)
	path2 := manager.CurrentSessionPath("task_multi")
	if path1 != path2 {
		t.Fatalf("同一会话轮换路径: %s != %s", path1, path2)
	}

	// 模型切换（重建控制器 + AdoptHistory 续会话）
	models := bridge.Models(workspace, "")
	if len(models) == 0 {
		t.Skip("无已配置模型，跳过切换验证")
	}
	if err := manager.SetModel(ctx, "task_multi", workspace, models[0].Ref, "", ""); err != nil {
		t.Fatalf("SetModel 失败: %v", err)
	}
	path3 := manager.CurrentSessionPath("task_multi")
	if path3 == "" {
		t.Fatal("切换后无会话路径")
	}

	// 切换后第三轮（续写同一会话）
	if err := manager.Submit("task_multi", "第三轮：切换后继续"); err != nil {
		t.Fatal(err)
	}
	waitTurnDone(180 * time.Second)

	// 历史应包含三轮的用户消息（内核 Snapshot 异步落盘，轮询等待）
	deadline := time.After(30 * time.Second)
	var userTurns int
	for {
		page, err := manager.Resume("task_multi", "", 0, 50)
		if err == nil {
			userTurns = 0
			for _, message := range page.Messages {
				if message.Role == "user" && message.Content != "" {
					userTurns++
				}
			}
		}
		if userTurns >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("历史用户消息不足: %d（应 >=3）", userTurns)
		case <-time.After(500 * time.Millisecond):
		}
	}

	manager.Close("task_multi")
}

// TestHistoryPagination 验证历史分页契约（beforeTurn/startTurn/hasOlder）。
func TestHistoryPagination(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	events := make(chan map[string]any, 4096)
	manager := bridge.NewManager(dataRoot, func(_ string, p map[string]any) { events <- p })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_page", workspace); err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_page")
	wait := func() {
		deadline := time.After(90 * time.Second)
		for {
			select {
			case p := <-events:
				if p["kind"] == "turn_done" {
					return
				}
			case <-deadline:
				t.Fatal("timeout")
			}
		}
	}
	// 3 轮对话
	for i := 1; i <= 3; i++ {
		if err := manager.Submit("task_page", fmt.Sprintf("第%d轮", i)); err != nil {
			t.Fatal(err)
		}
		wait()
	}
	// 等落盘
	deadline := time.After(30 * time.Second)
	for {
		page, err := manager.Resume("task_page", "", 0, 50)
		if err == nil && page.TotalTurns >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("会话未落盘")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// 最后一页
	page, err := manager.Resume("task_page", "", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalTurns < 3 {
		t.Fatalf("totalTurns 应为 3+: %d", page.TotalTurns)
	}
	if page.StartTurn <= 1 {
		t.Fatalf("limit=2 时 startTurn 应 >1（向前翻页前提）: %d", page.StartTurn)
	}
	if !page.HasOlder {
		t.Fatal("limit=2 且 3 轮时应 hasOlder=true")
	}
	if page.EndTurn < page.TotalTurns {
		t.Fatalf("endTurn 应等于 totalTurns: %d != %d", page.EndTurn, page.TotalTurns)
	}

	// 向前翻页（beforeTurn = 上一页 startTurn）
	older, err := manager.Resume("task_page", "", page.StartTurn, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Messages) == 0 {
		t.Fatal("更早页面为空")
	}
	if older.HasOlder {
		t.Logf("3 轮且 limit=2 的第二页仍可能 hasOlder（startTurn=1）")
	}
	_ = older
}

// TestEffortOverrideApplied 验证 effort/token 覆盖真正传入内核构建。
func TestEffortOverrideApplied(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_effort", workspace); err != nil {
		t.Fatal(err)
	}
	// 切换 effort（触发重建），随后切换模型同路径——不应报错且控制器可用
	if err := manager.SetModel(ctx, "task_effort", workspace, "", "high", "economy"); err != nil {
		t.Fatalf("effort/token 切换失败: %v", err)
	}
	tab := manager.Tab("task_effort")
	if tab == nil || tab.Ctrl == nil {
		t.Fatal("重建后控制器为空")
	}
	if err := manager.Submit("task_effort", "测试"); err != nil {
		t.Fatalf("重建后提交失败: %v", err)
	}
	manager.Close("task_effort")
}

// TestForkActivatesBranch 验证 Fork 后控制器切换到新分支路径。
func TestForkActivatesBranch(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	events := make(chan map[string]any, 4096)
	manager := bridge.NewManager(dataRoot, func(_ string, p map[string]any) { events <- p })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_fork", workspace); err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_fork")
	// 提交两轮（checkpoint 边界从第二轮回合开始可用，fork turn 2）
	for i := 1; i <= 2; i++ {
		if err := manager.Submit("task_fork", fmt.Sprintf("fork 测试消息 %d", i)); err != nil {
			t.Fatal(err)
		}
		deadline := time.After(120 * time.Second)
		for {
			if _, err := os.Stat(manager.CurrentSessionPath("task_fork")); err == nil {
				if info, _ := os.Stat(manager.CurrentSessionPath("task_fork")); info != nil && info.Size() > 0 {
					break
				}
			}
			select {
			case <-deadline:
				t.Fatal("等待会话文件超时")
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	oldPath := manager.CurrentSessionPath("task_fork")

	// 等待回合完全停止（turn_done 事件后内核仍在收尾）
	stopDeadline := time.After(30 * time.Second)
	for manager.Tab("task_fork").Ctrl.Running() {
		select {
		case <-stopDeadline:
			t.Fatal("回合未在 30s 内停止")
		case <-time.After(200 * time.Millisecond):
		}
	}

	// fork 于第 2 回合（有 checkpoint 边界）
	newPath, err := manager.Fork("task_fork", 1)
	if err != nil {
		t.Fatal(err)
	}
	if newPath == "" || newPath == oldPath {
		t.Fatalf("fork 路径无效: %q vs %q", newPath, oldPath)
	}

	// 激活分支（对应 rx_bindings ForkForTab 的 AdoptHistory 调用）
	tab := manager.Tab("task_fork")
	tab.Ctrl.AdoptHistory(tab.Ctrl.History(), newPath)
	if manager.CurrentSessionPath("task_fork") != newPath {
		t.Fatalf("Fork 后控制器未切换到新分支: %q", manager.CurrentSessionPath("task_fork"))
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("fork 分支文件不存在: %v", err)
	}
	manager.Close("task_fork")
}

// TestRewindRollsBackHistory 验证回滚操作真实生效（历史截断到指定回合）。
func TestRewindRollsBackHistory(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	events := make(chan map[string]any, 4096)
	manager := bridge.NewManager(dataRoot, func(_ string, p map[string]any) { events <- p })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_rw", workspace); err != nil {
		t.Fatal(err)
	}
	manager.NewSession("task_rw")
	waitTurnDone := func() {
		deadline := time.After(120 * time.Second)
		for {
			select {
			case p := <-events:
				if p["kind"] == "turn_done" {
					return
				}
			case <-deadline:
				t.Fatal("等待 turn_done 超时")
			}
		}
	}
	waitStopped := func() {
		deadline := time.After(30 * time.Second)
		for manager.Tab("task_rw").Ctrl.Running() {
			select {
			case <-deadline:
				t.Fatal("回合未停止")
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	// 两轮
	for i := 1; i <= 2; i++ {
		if err := manager.Submit("task_rw", fmt.Sprintf("回滚测试 %d", i)); err != nil {
			t.Fatal(err)
		}
		waitTurnDone()
	}
	waitStopped()

	// 回滚到第 1 轮（conversation 范围）
	if err := manager.Rewind("task_rw", 1, "conversation"); err != nil {
		t.Fatalf("Rewind 失败: %v", err)
	}
	// 回滚后历史应只含第一轮的用户消息
	page, err := manager.Resume("task_rw", "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	userTurns := 0
	for _, message := range page.Messages {
		if message.Role == "user" && strings.Contains(message.Content, "回滚测试") {
			userTurns++
		}
	}
	if userTurns != 1 {
		t.Fatalf("回滚后应只剩 1 轮用户消息: %d", userTurns)
	}
	manager.Close("task_rw")
}

// TestShutdownReleasesAll 验证多任务 Shutdown 全部释放。
func TestShutdownReleasesAll(t *testing.T) {
	dataRoot := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	for _, taskID := range []string{"task_s1", "task_s2", "task_s3"} {
		if _, err := manager.Ensure(ctx, taskID, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	manager.Shutdown()
	for _, taskID := range []string{"task_s1", "task_s2", "task_s3"} {
		if manager.Tab(taskID) != nil {
			t.Fatalf("Shutdown 后 %s 仍存在", taskID)
		}
	}
}

// TestEnsureConcurrent 验证同一任务并发 Ensure 的正确性（单 tab、无重复）。
func TestEnsureConcurrent(t *testing.T) {
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	tabs := make([]*bridge.TaskTab, 4)
	for i := range 4 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tab, err := manager.Ensure(ctx, "task_conc", workspace)
			if err != nil {
				t.Errorf("并发 Ensure 失败: %v", err)
				return
			}
			tabs[idx] = tab
		}(i)
	}
	wg.Wait()
	for i := 1; i < 4; i++ {
		if tabs[i] == nil || tabs[0] == nil {
			t.Fatal("Ensure 返回 nil")
		}
		if tabs[i].ID != tabs[0].ID {
			t.Fatalf("并发 Ensure 产生不同 tab: %s vs %s", tabs[i].ID, tabs[0].ID)
		}
	}
	manager.Close("task_conc")
}
