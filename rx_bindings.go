package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"reasonix/bridge"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// reasonixEventName 是 Reasonix 内核事件通道（与 BTask agent 事件分离，
// 由 ReasonixPage 的桥转发进 iframe）。
const reasonixEventName = "reasonix:event"

// rxSessionOverrides 记录仅存在于本会话的运行时覆盖（effort/token 模式等），
// 在下次控制器重建时应用。
type rxSessionOverrides struct {
	effort   string
	token    string
	collab   string
	approval string
}

// rxTabEntry 是 tabID → 任务 的登记项。
type rxTabEntry struct {
	taskID        string
	workspaceRoot string
	overrides     rxSessionOverrides
}

// initReasonix 在 NewApp 中调用，初始化 Reasonix 会话管理器。
// 会话数据落在 BTask 的应用数据目录（与 SQLite 同目录），
// 每个任务独立子目录，事件经 wails "reasonix:event" 通道发出。
func (a *App) initReasonix() {
	dataDir := rxDataDirectory()
	a.rxManager = bridge.NewManager(dataDir, func(tabID string, payload map[string]any) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, reasonixEventName, payload)
		}
	})
	a.rxMu = &sync.Mutex{}
	a.rxTabs = map[string]rxTabEntry{}
}

func rxDataDirectory() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "BTaskAssistant")
}

// rxActiveTask 返回最近激活的 Reasonix 任务（RX 页面一次只显示一个任务）。
func (a *App) rxActiveTask() (string, string, error) {
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	if a.rxActiveTaskID == "" {
		return "", "", errors.New("请先从任务详情打开 RX 工作台")
	}
	return a.rxActiveTaskID, a.rxActiveWorkspaceRoot, nil
}

func (a *App) rxTaskForTab(tabID string) (string, error) {
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	entry, ok := a.rxTabs[tabID]
	if !ok {
		return "", fmt.Errorf("未知的 Reasonix 标签页 %s", tabID)
	}
	return entry.taskID, nil
}

// rxTabIDForTask 返回任务的 tab 标识（与 bridge 的 tabID 派生一致）。
func rxTabIDForTask(taskID string) string {
	return "task_" + rxShortHash(taskID)
}

// rxShortHash 与 bridge.shortHash 保持一致的短哈希（用于 tabID 派生）。
func rxShortHash(value string) string {
	if value == "" {
		return "default"
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

// rxTaskContext 解析任务工作区根目录（与 PI 工作台同一 workspace）。
func (a *App) rxTaskContext(taskID string) (string, error) {
	if a.store == nil {
		return "", errors.New("存储未就绪")
	}
	workspace, err := a.store.EnsureTaskWorkspace(taskID)
	if err != nil {
		return "", fmt.Errorf("解析任务工作区: %w", err)
	}
	return workspace.RootPath, nil
}

// ── 绑定方法（reasonix 前端 wailsjs 同名调用）─────────────────────────────

// ReasonixEnsureTab 供 BTask 主 frame 桥调用：为任务建立（或复用）Reasonix 会话。
func (a *App) ReasonixEnsureTab(taskID string, workspaceRoot string, taskTitle string) (bridge.TabView, error) {
	if a.rxManager == nil {
		return bridge.TabView{}, errors.New("Reasonix 内核未初始化")
	}
	// 会话-任务绑定：记录任务标题（TabView 的 TopicTitle 展示）
	a.rxManager.SetTaskTitle(taskID, taskTitle)
	if workspaceRoot == "" {
		root, err := a.rxTaskContext(taskID)
		if err != nil {
			return bridge.TabView{}, err
		}
		workspaceRoot = root
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	tab, err := a.rxManager.Ensure(ctx, taskID, workspaceRoot)
	if err != nil {
		return bridge.TabView{}, err
	}
	a.rxMu.Lock()
	a.rxTabs[tab.ID] = rxTabEntry{taskID: taskID, workspaceRoot: workspaceRoot}
	a.rxActiveTaskID = taskID
	a.rxActiveWorkspaceRoot = workspaceRoot
	a.rxMu.Unlock()
	return bridge.TabViewOf(tab, taskID, workspaceRoot), nil
}

// ReasonixCloseTab 供 BTask 主 frame 桥调用：关闭任务的会话运行时。
func (a *App) ReasonixCloseTab(taskID string) {
	if a.rxManager != nil {
		a.rxManager.Close(taskID)
	}
	a.rxMu.Lock()
	for tabID, entry := range a.rxTabs {
		if entry.taskID == taskID {
			delete(a.rxTabs, tabID)
		}
	}
	if a.rxActiveTaskID == taskID {
		a.rxActiveTaskID = ""
		a.rxActiveWorkspaceRoot = ""
	}
	a.rxMu.Unlock()
}

// ListTabs 返回 Reasonix 会话标签页（当前激活任务）。
func (a *App) ListTabs() ([]bridge.TabView, error) {
	taskID, root, err := a.rxActiveTask()
	if err != nil {
		return []bridge.TabView{}, nil
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return []bridge.TabView{}, nil
	}
	return []bridge.TabView{bridge.TabViewOf(tab, taskID, root)}, nil
}

// SetActiveTab 激活标签页（单任务场景下为幂等操作）。
func (a *App) SetActiveTab(tabID string) error {
	_, err := a.rxTaskForTab(tabID)
	return err
}

// EnsureBlankTab 确保任务有一个空白会话（前端首次打开时调用）。
func (a *App) EnsureBlankTab(_scope string, workspaceRoot string) (bridge.TabView, error) {
	taskID, root, err := a.rxActiveTask()
	if err != nil {
		return bridge.TabView{}, err
	}
	if workspaceRoot == "" {
		workspaceRoot = root
	}
	return a.ReasonixEnsureTab(taskID, workspaceRoot, "")
}

// OpenGlobalTab 全局会话等价于任务会话（单任务映射）。
func (a *App) OpenGlobalTab(_topicID string) (bridge.TabView, error) {
	return a.EnsureBlankTab("global", "")
}

// NewSessionForTab 为任务开新会话。
func (a *App) NewSessionForTab(tabID string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	return a.rxManager.NewSession(taskID)
}

// SubmitToTab 提交一轮对话。
func (a *App) SubmitToTab(tabID string, input string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	return a.rxManager.Submit(taskID, input)
}

// SubmitInitialGoalToTab 提交带目标的一轮。
func (a *App) SubmitInitialGoalToTab(
	tabID string,
	goal string,
	_display string,
	input string,
	_invocations []any,
	_collaborationMode string,
	_toolApprovalMode string,
	_targetKind string,
	_targetIdentityGen int,
	_targetRequestSeq int,
) ([]string, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	if err := a.rxManager.SubmitWithGoal(taskID, goal, input); err != nil {
		return nil, err
	}
	return []string{}, nil
}

// CancelTab 取消进行中的回合。
func (a *App) CancelTab(tabID string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	a.rxManager.Cancel(taskID)
	return nil
}

// ListSessions 列出当前任务的历史会话。
func (a *App) ListSessions() ([]bridge.SessionMetaView, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return []bridge.SessionMetaView{}, nil
	}
	return a.rxManager.Sessions(taskID)
}

// ResumeSessionPageForTab 按页读取会话历史。
func (a *App) ResumeSessionPageForTab(tabID string, path string, limit int) (bridge.HistoryPageView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return bridge.HistoryPageView{}, err
	}
	return a.rxManager.Resume(taskID, path, 0, limit)
}

// DeleteSession 删除会话文件（仅允许任务隔离会话目录内的文件）。
func (a *App) DeleteSession(path string) error {
	if err := a.rxValidateSessionPath(path); err != nil {
		return err
	}
	cleaned := filepath.Clean(path)
	// 删除当前激活会话前先让控制器轮换到新文件——否则下一次 turn_done
	// 快照会重建已删文件（对应桌面端 quiesceTabAutosave 的 #4384 复活问题）。
	a.rxMu.Lock()
	for _, entry := range a.rxTabs {
		if filepath.Dir(cleaned) != filepath.Clean(a.rxManager.TaskSessionDir(entry.taskID)) {
			continue
		}
		if tab := a.rxManager.Tab(entry.taskID); tab != nil && tab.Ctrl != nil {
			if filepath.Clean(tab.Ctrl.SessionPath()) == cleaned {
				if err := tab.Ctrl.NewSession(); err != nil {
					a.rxMu.Unlock()
					return fmt.Errorf("当前会话正在运行，无法删除（请先停止回合）: %w", err)
				}
			}
		}
		break
	}
	a.rxMu.Unlock()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	for _, entry := range a.rxTabs {
		if filepath.Dir(filepath.Clean(path)) == filepath.Clean(a.rxManager.TaskSessionDir(entry.taskID)) {
			_ = a.rxManager.RemoveSessionTitles(entry.taskID, path)
			break
		}
	}
	// 清理会话侧车（与桌面端 deleteSessionFile 同清单）：
	// .events.jsonl/.event-index.json/.ckpt/.goal-state.json/.recovery.json/
	// .meta/.jobs/.conflicts.jsonl/.telemetry.json/.jsonl.lock
	stem := strings.TrimSuffix(path, ".jsonl")
	for _, artifact := range []string{
		stem + ".events.jsonl",
		stem + ".events.jsonl.damaged",
		stem + ".event-index.json",
		stem + ".goal-state.json",
		stem + ".recovery.json",
		stem + ".conflicts.jsonl",
		path + ".meta",
		path + ".telemetry.json",
		path + ".lock",
		stem + ".ckpt",
		stem + ".jobs",
	} {
		_ = os.RemoveAll(artifact)
	}
	return nil
}

// rxValidateSessionPath 校验 path 落在某个任务的 reasonix-sessions 目录内。
func (a *App) rxValidateSessionPath(path string) error {
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	cleaned := filepath.Clean(path)
	for _, entry := range a.rxTabs {
		dir := filepath.Clean(a.rxManager.TaskSessionDir(entry.taskID))
		rel, err := filepath.Rel(dir, cleaned)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("拒绝删除会话目录外的文件: %s", path)
}

// RenameSession 重命名会话（持久化到 .titles.json 侧车，与桌面端同约定）。
func (a *App) RenameSession(path string, title string) error {
	if err := a.rxValidateSessionPath(path); err != nil {
		return err
	}
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	for _, entry := range a.rxTabs {
		if filepath.Dir(filepath.Clean(path)) == filepath.Clean(a.rxManager.TaskSessionDir(entry.taskID)) {
			return a.rxManager.RenameSession(entry.taskID, path, title)
		}
	}
	return fmt.Errorf("拒绝重命名会话目录外的文件: %s", path)
}

// MetaForTab 返回任务的运行元信息。
func (a *App) MetaForTab(tabID string) (bridge.MetaView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return bridge.MetaView{}, err
	}
	view, err := a.rxManager.Meta(taskID)
	if err != nil {
		return view, err
	}
	a.rxMu.Lock()
	entry, ok := a.rxTabs[tabID]
	a.rxMu.Unlock()
	if ok {
		view.EventChannel = reasonixEventName
		view.Cwd = entry.workspaceRoot
		view.WorkspaceRoot = entry.workspaceRoot
		view.WorkspaceName = workspaceDisplayName(entry.workspaceRoot)
		if tab := a.rxManager.Tab(taskID); tab != nil {
			view.Goal = tab.Ctrl.Goal()
			view.GoalStatus = tab.Ctrl.GoalStatus()
		}
	}
	return view, nil
}

// rxModels 从任务配置解析可用模型列表。
func (a *App) rxModels(taskID string, workspaceRoot string) ([]bridge.ModelInfoView, error) {
	current := ""
	if tab := a.rxManager.Tab(taskID); tab != nil {
		current = tab.Ctrl.ModelRef()
	}
	return bridge.Models(workspaceRoot, current), nil
}

// Models 返回可用模型列表（从任务配置解析）。
func (a *App) Models() ([]bridge.ModelInfoView, error) {
	taskID, root, err := a.rxActiveTask()
	if err != nil {
		return []bridge.ModelInfoView{}, nil
	}
	return a.rxModels(taskID, root)
}

// ModelsForTab 返回任务的模型列表。
func (a *App) ModelsForTab(tabID string) ([]bridge.ModelInfoView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	a.rxMu.Lock()
	entry, ok := a.rxTabs[tabID]
	a.rxMu.Unlock()
	if !ok {
		return []bridge.ModelInfoView{}, nil
	}
	return a.rxModels(taskID, entry.workspaceRoot)
}

// SetModelForTab 切换任务会话模型（重建控制器）。
func (a *App) SetModelForTab(tabID string, name string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	a.rxMu.Lock()
	entry, ok := a.rxTabs[tabID]
	a.rxMu.Unlock()
	if !ok {
		return fmt.Errorf("未知的 Reasonix 标签页 %s", tabID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := a.rxManager.SetModel(ctx, taskID, entry.workspaceRoot, name, entry.overrides.effort, entry.overrides.token); err != nil {
		return err
	}
	a.rxEmitRuntimeRebuilt(tabID)
	return nil
}

// rxEmitRuntimeRebuilt 通知前端控制器已重建（审批/ask id 重置），
// 对应桌面端 runtime:rebuilt 事件。
func (a *App) rxEmitRuntimeRebuilt(tabID string) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, reasonixEventName, map[string]any{
			"type":         "runtime:rebuilt",
			"tabId":        tabID,
			"runtimeEpoch": "",
		})
	}
}

// SetEffortForTab 切换推理强度（持久化覆盖并重建控制器生效，与桌面端一致）。
func (a *App) SetEffortForTab(tabID string, level string) error {
	if err := a.rxApplyOverride(tabID, func(o *rxSessionOverrides) { o.effort = level }); err != nil {
		return err
	}
	a.rxMu.Lock()
	entry, ok := a.rxTabs[tabID]
	a.rxMu.Unlock()
	if !ok {
		return fmt.Errorf("未知的 Reasonix 标签页 %s", tabID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := a.rxManager.SetModel(ctx, entry.taskID, entry.workspaceRoot, "", entry.overrides.effort, entry.overrides.token); err != nil {
		return err
	}
	a.rxEmitRuntimeRebuilt(tabID)
	return nil
}

// SetTokenModeForTab 切换 token 模式（持久化覆盖并重建控制器生效）。
func (a *App) SetTokenModeForTab(tabID string, mode string) error {
	if err := a.rxApplyOverride(tabID, func(o *rxSessionOverrides) { o.token = mode }); err != nil {
		return err
	}
	a.rxMu.Lock()
	entry, ok := a.rxTabs[tabID]
	a.rxMu.Unlock()
	if !ok {
		return fmt.Errorf("未知的 Reasonix 标签页 %s", tabID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := a.rxManager.SetModel(ctx, entry.taskID, entry.workspaceRoot, "", entry.overrides.effort, entry.overrides.token); err != nil {
		return err
	}
	a.rxEmitRuntimeRebuilt(tabID)
	return nil
}

// SetModeForTab 切换 plan/normal 模式。
func (a *App) SetModeForTab(tabID string, mode string) ([]string, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return nil, errors.New("Reasonix 会话尚未初始化")
	}
	tab.Ctrl.SetPlanMode(mode == "plan")
	return []string{}, nil
}

// SetToolApprovalModeForTab 设置工具审批姿态。
func (a *App) SetToolApprovalModeForTab(tabID string, mode string) ([]string, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return nil, errors.New("Reasonix 会话尚未初始化")
	}
	tab.Ctrl.SetToolApprovalMode(mode)
	return []string{}, nil
}

// SetCollaborationModeForTab 记录协作模式覆盖。
func (a *App) SetCollaborationModeForTab(tabID string, mode string) error {
	return a.rxApplyOverride(tabID, func(o *rxSessionOverrides) { o.collab = mode })
}

// SetComposerProfileForTab 组合设置协作模式与审批姿态。
func (a *App) SetComposerProfileForTab(
	tabID string,
	collaborationMode string,
	toolApprovalMode string,
	_goal string,
) ([]string, error) {
	if err := a.rxApplyOverride(tabID, func(o *rxSessionOverrides) { o.collab = collaborationMode }); err != nil {
		return nil, err
	}
	return a.SetToolApprovalModeForTab(tabID, toolApprovalMode)
}

func (a *App) rxApplyOverride(tabID string, apply func(*rxSessionOverrides)) error {
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	entry, ok := a.rxTabs[tabID]
	if !ok {
		return fmt.Errorf("未知的 Reasonix 标签页 %s", tabID)
	}
	apply(&entry.overrides)
	a.rxTabs[tabID] = entry
	return nil
}

// SetGoalForTab 设置任务的当前目标。
func (a *App) SetGoalForTab(tabID string, goal string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return errors.New("Reasonix 会话尚未初始化")
	}
	tab.Ctrl.SetGoal(goal)
	return nil
}

// ClearGoalForTab 清除任务目标。
func (a *App) ClearGoalForTab(tabID string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		tab.Ctrl.ClearGoal()
	}
	return nil
}

// ResumeGoalForTab 恢复可恢复的目标。
func (a *App) ResumeGoalForTab(tabID string) (bool, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return false, err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return false, nil
	}
	return tab.Ctrl.ResumeGoal(), nil
}

// SteerForTab 发送转向指令。
func (a *App) SteerForTab(tabID string, text string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		tab.Ctrl.Steer(text)
	}
	return nil
}

// ApproveTab 批准/拒绝工具审批请求。
func (a *App) ApproveTab(tabID string, id string, allow bool, session bool, persist bool) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		tab.Ctrl.Approve(id, allow, session, persist)
	}
	return nil
}

// AnswerQuestionForTab 回答会话中的提问（驱动内核 ask 卡继续对话）。
func (a *App) AnswerQuestionForTab(tabID string, id string, answers []any) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	converted := make([]map[string]any, 0, len(answers))
	for _, raw := range answers {
		if obj, ok := raw.(map[string]any); ok {
			converted = append(converted, obj)
		}
	}
	return a.rxManager.AnswerQuestion(taskID, id, converted)
}

// ReplayPendingPrompts 重放挂起的提示。
func (a *App) ReplayPendingPrompts() error {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return nil
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		tab.Ctrl.ReplayPendingPrompts()
	}
	return nil
}

// CompactForTab 压缩会话上下文。
func (a *App) CompactForTab(tabID string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()
		return tab.Ctrl.Compact(ctx, "")
	}
	return nil
}

// ContextUsageForTab 返回上下文用量。
func (a *App) ContextUsageForTab(tabID string) (map[string]any, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return map[string]any{}, nil
	}
	used, window := tab.Ctrl.ContextSnapshot()
	return map[string]any{"usedTokens": used, "contextWindow": window}, nil
}

// BalanceForTab 返回余额信息（宿主无计费，返回空）。
func (a *App) BalanceForTab(_tabID string) (map[string]any, error) {
	return map[string]any{}, nil
}

// JobsForTab 返回后台任务视图（宿主空列表）。
func (a *App) JobsForTab(_tabID string) ([]any, error) {
	return []any{}, nil
}

// ToolResultForTab 返回工具调用结果。
func (a *App) ToolResultForTab(tabID string, toolID string) (map[string]any, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		if data := tab.Ctrl.ToolResult(toolID); data != nil {
			return map[string]any{"args": data.Args, "output": data.Output}, nil
		}
	}
	return nil, nil
}

// HistoryForTab 返回当前会话历史。
func (a *App) HistoryForTab(tabID string) ([]bridge.HistoryMessageView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return nil, err
	}
	page, err := a.rxManager.Resume(taskID, "", 0, 200)
	if err != nil {
		if os.IsNotExist(err) {
			return []bridge.HistoryMessageView{}, nil
		}
		return nil, err
	}
	return page.Messages, nil
}

// HistoryPageForTab 按页返回会话历史（beforeTurn>0 时返回更早的一页）。
func (a *App) HistoryPageForTab(tabID string, beforeTurn int, limit int) (bridge.HistoryPageView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return bridge.HistoryPageView{}, err
	}
	return a.rxManager.Resume(taskID, "", beforeTurn, limit)
}

// ScanPromptHistory 供 ↑/↓ 提示历史：按 nonce 读取会话中的用户消息。
func (a *App) ScanPromptHistory(nonce string) (map[string]any, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return map[string]any{"entries": []any{}, "nonce": nonce}, nil
	}
	page, err := a.rxManager.Resume(taskID, "", 0, 100)
	if err != nil {
		return map[string]any{"entries": []any{}, "nonce": nonce}, nil
	}
	entries := make([]map[string]any, 0, len(page.Messages))
	for _, message := range page.Messages {
		if message.Role != "user" {
			continue
		}
		at := message.CreatedAt
		entries = append(entries, map[string]any{
			"text":        message.Content,
			"at":          at,
			"sessionPath": a.rxManager.CurrentSessionPath(taskID),
			"turn":        message.Turn,
		})
	}
	return map[string]any{"entries": entries, "nonce": nonce}, nil
}

// ListDirForTab 列出任务工作区目录（供 @ 引用与文件选择）。
func (a *App) ListDirForTab(tabID string, rel string) ([]map[string]any, error) {
	if _, err := a.rxTaskForTab(tabID); err != nil {
		return nil, err
	}
	root, err := a.rxTaskContextByTab(tabID)
	if err != nil {
		return []map[string]any{}, nil
	}
	return listDirEntries(root, rel)
}

func (a *App) rxTaskContextByTab(tabID string) (string, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return "", err
	}
	return a.rxTaskContext(taskID)
}

// rxWorkspacePath 把工作区相对路径解析为绝对路径，并拒绝逃逸工作区。
func rxWorkspacePath(root string, rel string) (string, error) {
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return "", fmt.Errorf("路径超出任务工作区: %s", rel)
	}
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	relToRoot, err := filepath.Rel(filepath.Clean(root), path)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径超出任务工作区: %s", rel)
	}
	return path, nil
}

// ReadFileForTab 读取任务工作区文件。
func (a *App) ReadFileForTab(tabID string, rel string) (map[string]any, error) {
	if _, err := a.rxTaskForTab(tabID); err != nil {
		return nil, err
	}
	root, err := a.rxTaskContextByTab(tabID)
	if err != nil {
		return nil, err
	}
	path, err := rxWorkspacePath(root, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":       rel,
		"name":       info.Name(),
		"byteSize":   info.Size(),
		"modifiedAt": info.ModTime().UTC().Format(time.RFC3339),
		"content":    string(raw),
	}, nil
}

// WorkspaceChanges 返回任务工作区 Git 变更（宿主简化：空）。
func (a *App) WorkspaceChanges(_tabID string) (map[string]any, error) {
	return map[string]any{"files": []any{}}, nil
}

// OpenWorkspacePathForTab 打开工作区中的路径。
func (a *App) OpenWorkspacePathForTab(tabID string, rel string) error {
	if _, err := a.rxTaskForTab(tabID); err != nil {
		return err
	}
	root, err := a.rxTaskContextByTab(tabID)
	if err != nil {
		return err
	}
	path, err := rxWorkspacePath(root, rel)
	if err != nil {
		return err
	}
	return a.openPath(context.Background(), path)
}

// MemoryForTab 返回任务记忆（docs/facts/storeDir 来自内核真实数据）。
func (a *App) MemoryForTab(tabID string) (map[string]any, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return emptyRxMemoryView(), nil
	}
	return a.rxManager.MemoryView(taskID)
}

func emptyRxMemoryView() map[string]any {
	return map[string]any{
		"docs": []any{}, "facts": []any{}, "archives": []any{}, "scopes": []any{},
		"instructionDiagnostics": []any{}, "conflicts": []any{}, "lastRecall": nil,
		"storeDir": "", "available": false,
	}
}

// Commands 返回当前任务控制器的真实斜杠命令（/help /new /memory /skill …）。
func (a *App) Commands() ([]map[string]any, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return []map[string]any{}, nil
	}
	return a.rxManager.CommandViews(taskID)
}

// Capabilities 返回能力视图（技能来自内核真实数据）。
func (a *App) Capabilities() (map[string]any, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return a.rxManager.CapabilitiesView(""), nil
	}
	return a.rxManager.CapabilitiesView(taskID), nil
}

// Platform 返回平台标识（reasonix 前端据此渲染平台相关 UI）。
func (a *App) Platform() (string, error) {
	switch goruntime.GOOS {
	case "windows":
		return "windows", nil
	case "linux":
		return "linux", nil
	default:
		return "darwin", nil
	}
}

// CloseMainWindow 由 Reasonix 前端窗口按钮调用（宿主窗口不因此关闭）。
func (a *App) CloseMainWindow() error {
	return nil
}

// MinimiseMainWindow 最小化宿主窗口。
func (a *App) MinimiseMainWindow() error {
	return nil
}

// ToggleMaximiseMainWindow 切换最大化。
func (a *App) ToggleMaximiseMainWindow() error {
	return nil
}

// IsMainWindowMaximised 窗口是否最大化。
func (a *App) IsMainWindowMaximised() (bool, error) {
	return false, nil
}

// ── 工具函数 ────────────────────────────────────────────────────────────────

func listDirEntries(root string, rel string) ([]map[string]any, error) {
	dir, err := rxWorkspacePath(root, rel)
	if err != nil {
		return []map[string]any{}, nil
	}
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		return []map[string]any{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []map[string]any{}, nil
	}
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
		}
		result = append(result, map[string]any{
			"name":       entry.Name(),
			"path":       filepath.ToSlash(filepath.Join(rel, entry.Name())),
			"type":       entryType,
			"byteSize":   info.Size(),
			"modifiedAt": info.ModTime().UTC().Format(time.RFC3339),
			"readable":   true,
		})
	}
	return result, nil
}

// rxCloseAll 关闭全部 Reasonix 会话运行时（shutdown 调用）。
func (a *App) rxCloseAll() {
	if a.rxManager != nil {
		a.rxManager.Shutdown()
	}
}

// ── P0 补充绑定（reasonix 前端高频调用面，补全缺口）──────────────────────

// ClearSessionForTab 丢弃当前会话并轮换新会话。
func (a *App) ClearSessionForTab(tabID string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return errors.New("Reasonix 会话尚未初始化")
	}
	return tab.Ctrl.ClearSession()
}

// ClearSession 丢弃激活任务的当前会话。
func (a *App) ClearSession() error {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return nil
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		return tab.Ctrl.ClearSession()
	}
	return nil
}

// EnsureBlankSurface 确保任务有空白会话（等价 EnsureBlankTab）。
func (a *App) EnsureBlankSurface(workspaceRoot string) (bridge.TabView, error) {
	return a.EnsureBlankTab("project", workspaceRoot)
}

// ListProjectTree 返回项目树（宿主：空树，前端容错）。
func (a *App) ListProjectTree() ([]any, error) {
	return []any{}, nil
}

// CreateTopic 创建主题分组（宿主：虚拟分组，不持久化）。
func (a *App) CreateTopic(_scope string, _workspaceRoot string, title string) (map[string]any, error) {
	return map[string]any{"id": "topic_host_" + newRxID(), "title": title}, nil
}

// CheckpointsForTab 返回会话检查点（宿主：由内核提供，空数组容错）。
func (a *App) CheckpointsForTab(tabID string) ([]any, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return []any{}, nil
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		checkpoints := tab.Ctrl.Checkpoints()
		out := make([]any, 0, len(checkpoints))
		for _, cp := range checkpoints {
			out = append(out, map[string]any{
				"turn": cp.Turn,
				"time": cp.Time.UnixMilli(),
			})
		}
		return out, nil
	}
	return []any{}, nil
}

// HistoryCheckpointTurnsForTab 返回带检查点的回合号（历史回滚标记）。
func (a *App) HistoryCheckpointTurnsForTab(tabID string) ([]int, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return []int{}, nil
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		checkpoints := tab.Ctrl.Checkpoints()
		turns := make([]int, 0, len(checkpoints))
		for _, cp := range checkpoints {
			turns = append(turns, cp.Turn)
		}
		return turns, nil
	}
	return []int{}, nil
}

// ListTrashedSessions 列出回收站会话（宿主：空）。
func (a *App) ListTrashedSessions() ([]bridge.SessionMetaView, error) {
	return []bridge.SessionMetaView{}, nil
}

// DeleteRecoveryCopy 删除恢复副本（宿主：no-op）。
func (a *App) DeleteRecoveryCopy(_path string) error {
	return nil
}

// Meta 返回激活任务的运行元信息。
func (a *App) Meta() (bridge.MetaView, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return bridge.MetaView{}, nil
	}
	return a.rxManager.Meta(taskID)
}

// EffortForTab 返回任务的推理强度设置（反映用户已保存的 override）。
func (a *App) EffortForTab(tabID string) (map[string]any, error) {
	current := "auto"
	a.rxMu.Lock()
	if entry, ok := a.rxTabs[tabID]; ok && entry.overrides.effort != "" {
		current = entry.overrides.effort
	}
	a.rxMu.Unlock()
	return rxEffortWith(current), nil
}

// Effort 返回激活任务的推理强度设置。
func (a *App) Effort() (map[string]any, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return rxDefaultEffort(), nil
	}
	return a.EffortForTab(rxTabIDForTask(taskID))
}

func rxDefaultEffort() map[string]any {
	return rxEffortWith("auto")
}

func rxEffortWith(current string) map[string]any {
	if current == "" {
		current = "auto"
	}
	return map[string]any{
		"supported": true,
		"current":   current,
		"default":   "auto",
		"levels":    []string{"auto", "low", "medium", "high", "xhigh", "max"},
	}
}

// ResumeSession 恢复指定会话（激活任务）。
func (a *App) ResumeSession(path string) ([]bridge.HistoryMessageView, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return []bridge.HistoryMessageView{}, nil
	}
	page, err := a.rxManager.Resume(taskID, path, 0, 200)
	if err != nil {
		return []bridge.HistoryMessageView{}, nil
	}
	return page.Messages, nil
}

// ResumeSessionForTab 恢复指定会话。
func (a *App) ResumeSessionForTab(tabID string, path string) ([]bridge.HistoryMessageView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return []bridge.HistoryMessageView{}, nil
	}
	page, err := a.rxManager.Resume(taskID, path, 0, 200)
	if err != nil {
		return []bridge.HistoryMessageView{}, nil
	}
	return page.Messages, nil
}

// ListWorkspaces 列出工作区（宿主：当前任务工作区）。
func (a *App) ListWorkspaces() ([]any, error) {
	taskID, root, err := a.rxActiveTask()
	if err != nil {
		return []any{}, nil
	}
	return []any{map[string]any{
		"root":   root,
		"name":   workspaceDisplayName(root),
		"active": true,
		"taskId": taskID,
	}}, nil
}

// ForgetForTab 遗忘指定记忆（内核 MemoryControl）。
func (a *App) ForgetForTab(tabID string, ref string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		return tab.Ctrl.ForgetMemory(ref)
	}
	return nil
}

// Forget 遗忘激活任务的指定记忆。
func (a *App) Forget(ref string) error {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return nil
	}
	if tab := a.rxManager.Tab(taskID); tab != nil {
		return tab.Ctrl.ForgetMemory(ref)
	}
	return nil
}

// ContextPanel 返回上下文面板信息（宿主：同 ContextUsage）。
func (a *App) ContextPanel(tabID string) (map[string]any, error) {
	return a.ContextUsageForTab(tabID)
}

// DesktopStartupSettings 返回桌面启动设置（宿主默认值，前端据此渲染侧栏）。
func (a *App) DesktopStartupSettings() (map[string]any, error) {
	return map[string]any{
		"bot":                map[string]any{},
		"desktopLanguage":    "",
		"desktopLayoutStyle": "classic",
		"desktopTheme":       "auto",
		"desktopThemeStyle":  "",
		"displayMode":        "standard",
		"statusBarStyle":     "icon",
		"statusBarItems":     []string{},
		"checkUpdates":       false,
		"updateChannel":      "stable",
		"conversationWidth":  "standard",
	}, nil
}

// NeedsOnboarding 是否首次使用引导（宿主：false，已融入主应用）。
func (a *App) NeedsOnboarding() (bool, error) {
	return false, nil
}

// WorkbenchPendingProviderTrust 挂起的 provider 信任提示（宿主：无）。
func (a *App) WorkbenchPendingProviderTrust() (any, error) {
	return nil, nil
}

// WorkbenchActiveTarget 当前 workbench 目标（宿主：本地）。
func (a *App) WorkbenchActiveTarget() (map[string]any, error) {
	return map[string]any{"kind": "local"}, nil
}

// RemoteHosts 远程主机列表（宿主：空）。
func (a *App) RemoteHosts() ([]any, error) {
	return []any{}, nil
}

// RemoteConnectionStatuses 远程连接状态（宿主：空）。
func (a *App) RemoteConnectionStatuses() ([]any, error) {
	return []any{}, nil
}

// ExternalOpeners 外部打开器（宿主：空）。
func (a *App) ExternalOpeners() (map[string]any, error) {
	return map[string]any{"openers": []any{}}, nil
}

// SetTrayLocale 托盘语言（宿主：no-op）。
func (a *App) SetTrayLocale(_locale string) error {
	return nil
}

// CheckUpdate 检查更新（宿主：无更新）。参数声明为 any——reasonix 前端契约
// 为 channel string，但 wails 绑定对类型不匹配会报参数解析错误（曾出现
// "cannot unmarshal string into Go value of type bool"）；any 接受任何传入
// 值，杜绝解析失败。更新逻辑在宿主环境已禁用（useUpdater host 短路 +
// 设置面板无更新标签）。
func (a *App) CheckUpdate(_channel any) (map[string]any, error) {
	return map[string]any{"status": "none"}, nil
}

// HeartbeatListTasks 心跳任务列表（宿主：空）。
func (a *App) HeartbeatListTasks() ([]any, error) {
	return []any{}, nil
}

// HeartbeatReloadTasks 心跳重载任务（宿主：空）。
func (a *App) HeartbeatReloadTasks() ([]any, error) {
	return []any{}, nil
}

// HeartbeatSaveTasks 心跳保存任务（宿主：no-op）。
func (a *App) HeartbeatSaveTasks(_tasks any) error {
	return nil
}

// HeartbeatTriggerNow 立即触发心跳（宿主：no-op）。
func (a *App) HeartbeatTriggerNow(_id string) error {
	return nil
}

// HeartbeatGenerateID 生成心跳 ID。
func (a *App) HeartbeatGenerateID() (string, error) {
	return newRxID(), nil
}

// GetDesktopZoomFactor 窗口缩放（宿主：1）。
func (a *App) GetDesktopZoomFactor() (float64, error) {
	return 1, nil
}

func newRxID() string {
	return fmt.Sprintf("rx-%d-%d", time.Now().UnixNano(), os.Getpid())
}

func workspaceDisplayName(root string) string {
	trimmed := strings.TrimRight(root, "/\\")
	if trimmed == "" {
		return "任务工作区"
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

// SearchFileRefsForTab 在任务工作区内按文件名搜索（@ 引用面板）。
// 递归深度受限，结果上限 100 项，避免大仓库卡顿。
func (a *App) SearchFileRefsForTab(tabID string, query string) ([]map[string]any, error) {
	if _, err := a.rxTaskForTab(tabID); err != nil {
		return []map[string]any{}, nil
	}
	root, err := a.rxTaskContextByTab(tabID)
	if err != nil {
		return []map[string]any{}, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []map[string]any{}, nil
	}
	results := make([]map[string]any, 0, 100)
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(results) >= 100 || depth > 4 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if len(results) >= 100 {
				return
			}
			name := entry.Name()
			// 跳过隐藏目录与常见依赖目录（搜索噪音/性能）
			if entry.IsDir() && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == "build") {
				continue
			}
			path := filepath.Join(dir, name)
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			entryType := "file"
			if entry.IsDir() {
				entryType = "directory"
			}
			if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
				results = append(results, map[string]any{
					"name":       name,
					"path":       filepath.ToSlash(rel),
					"type":       entryType,
					"byteSize":   info.Size(),
					"modifiedAt": info.ModTime().UTC().Format(time.RFC3339),
					"readable":   true,
				})
			}
			if entry.IsDir() {
				walk(path, depth+1)
			}
		}
	}
	walk(root, 0)
	return results, nil
}

// RewindForTab 回滚任务会话到指定回合（scope: code/conversation/both）。
func (a *App) RewindForTab(tabID string, turn int, scope string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	return a.rxManager.Rewind(taskID, turn, scope)
}

// ForkForTab 在指定回合分支出新会话，返回新 tab 视图。
func (a *App) ForkForTab(tabID string, turn int) (bridge.TabView, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return bridge.TabView{}, err
	}
	newPath, err := a.rxManager.Fork(taskID, turn)
	if err != nil {
		return bridge.TabView{}, err
	}
	// 激活新分支：把控制器切换到 fork 出的会话文件（否则前端后续读取
	// meta.sessionPath / 历史仍是旧会话，fork 不生效）。
	if tab := a.rxManager.Tab(taskID); tab != nil {
		tab.Ctrl.AdoptHistory(tab.Ctrl.History(), newPath)
	}
	a.rxMu.Lock()
	entry, ok := a.rxTabs[tabID]
	a.rxMu.Unlock()
	if !ok {
		return bridge.TabView{}, fmt.Errorf("未知的 Reasonix 标签页 %s", tabID)
	}
	view, err := a.rxManager.View(taskID, entry.workspaceRoot)
	if err != nil {
		return bridge.TabView{}, err
	}
	view.SessionPath = newPath
	return view, nil
}

// SummarizeFromForTab 从指定回合起压缩会话。
func (a *App) SummarizeFromForTab(tabID string, turn int) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	return a.rxManager.SummarizeFrom(ctx, taskID, turn)
}

// SummarizeUpToForTab 压缩到指定回合为止。
func (a *App) SummarizeUpToForTab(tabID string, turn int) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	return a.rxManager.SummarizeUpTo(ctx, taskID, turn)
}

// Settings 返回设置视图（最小完整结构：模型/供应商来自配置，其余默认空结构）。
// 缺失时 reasonix 设置面板会显示"加载失败"，故必须返回结构完整的对象。
func (a *App) Settings() (map[string]any, error) {
	taskID, root, err := a.rxActiveTask()
	if err != nil {
		return rxMinimalSettings("", nil), nil
	}
	models := bridge.Models(root, "")
	view := rxMinimalSettings(root, models)
	if tab := a.rxManager.Tab(taskID); tab != nil {
		view["defaultModel"] = tab.Ctrl.ModelRef()
	}
	return view, nil
}

func rxMinimalSettings(workspaceRoot string, models []bridge.ModelInfoView) map[string]any {
	providers := make([]map[string]any, 0, len(models))
	for _, model := range models {
		found := false
		for _, provider := range providers {
			if provider["name"] == model.Provider {
				found = true
				provider["models"] = append(provider["models"].([]string), model.Name)
			}
		}
		if !found {
			providers = append(providers, map[string]any{
				"name":                   model.Provider,
				"builtIn":                true,
				"added":                  true,
				"kind":                   "",
				"baseUrl":                "",
				"models":                 []string{model.Name},
				"visionModels":           []string{},
				"visionModelsConfigured": false,
				"modelsUrl":              "",
				"default":                model.Name,
				"apiKeyEnv":              "",
			})
		}
	}
	return map[string]any{
		"defaultModel":      "",
		"plannerModel":      "",
		"subagentModel":     "",
		"subagentEffort":    "",
		"autoPlan":          "",
		"providers":         providers,
		"officialProviders": []any{},
		"providerPresets":   []any{},
		"permissions": map[string]any{
			"rules":          []any{},
			"allowlist":      []any{},
			"denylist":       []any{},
			"allowByDefault": false,
		},
		"sandbox": map[string]any{
			"enabled":          false,
			"readOnly":         false,
			"network":          "off",
			"extraDirectories": []any{},
		},
		"network": map[string]any{
			"proxy":        "",
			"allowedHosts": []any{},
			"blockedHosts": []any{},
		},
		"agent": map[string]any{
			"keep":              "default",
			"maxSteps":          0,
			"language":          "",
			"reasoningLanguage": "",
		},
		"hooks": map[string]any{},
		"desktop": map[string]any{
			"language":          "",
			"layoutStyle":       "classic",
			"theme":             "auto",
			"themeStyle":        "",
			"displayMode":       "standard",
			"statusBarStyle":    "icon",
			"statusBarItems":    []string{},
			"checkUpdates":      false,
			"updateChannel":     "stable",
			"conversationWidth": "standard",
		},
		"memory": map[string]any{
			"autoRecall":   true,
			"quickCapture": true,
			"maxFacts":     100,
		},
		"skills": map[string]any{},
		"mcp":    map[string]any{},
		"remote": map[string]any{},
	}
}

// rxTaskForPath 从会话文件路径反查任务（路径必须在某任务会话目录内）。
func (a *App) rxTaskForPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	a.rxMu.Lock()
	defer a.rxMu.Unlock()
	for _, entry := range a.rxTabs {
		dir := filepath.Clean(a.rxManager.TaskSessionDir(entry.taskID))
		if filepath.Dir(cleaned) == dir {
			return entry.taskID, nil
		}
	}
	return "", fmt.Errorf("拒绝访问会话目录外的路径: %s", path)
}

// PreviewSession 只读预览会话历史（历史面板单击，不切换控制器）。
func (a *App) PreviewSession(path string) ([]bridge.HistoryMessageView, error) {
	if err := a.rxValidateSessionPath(path); err != nil {
		return nil, err
	}
	taskID, err := a.rxTaskForPath(path)
	if err != nil {
		return []bridge.HistoryMessageView{}, nil
	}
	page, err := a.rxManager.Resume(taskID, path, 0, 200)
	if err != nil {
		return []bridge.HistoryMessageView{}, nil
	}
	return page.Messages, nil
}

// ResumeSessionPage 按页恢复会话历史（无 tab 版，激活任务）。
func (a *App) ResumeSessionPage(path string, limit int) (bridge.HistoryPageView, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return bridge.HistoryPageView{}, nil
	}
	if path != "" {
		if err := a.rxValidateSessionPath(path); err != nil {
			return bridge.HistoryPageView{}, err
		}
	}
	return a.rxManager.Resume(taskID, path, 0, limit)
}

// NewSession 为激活任务开启新会话（无 tab 版）。
func (a *App) NewSession() error {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return nil
	}
	return a.rxManager.NewSession(taskID)
}

// RememberForTab 手动添加记忆（内核 QuickAdd，记忆面板"添加"按钮）。
func (a *App) RememberForTab(tabID string, scope string, note string) (string, error) {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(note) == "" {
		return "", errors.New("记忆内容不能为空")
	}
	return a.rxManager.QuickAdd(taskID, scope, strings.TrimSpace(note))
}

// Remember 为激活任务添加记忆。
func (a *App) Remember(scope string, note string) (string, error) {
	taskID, _, err := a.rxActiveTask()
	if err != nil {
		return "", nil
	}
	if strings.TrimSpace(note) == "" {
		return "", errors.New("记忆内容不能为空")
	}
	return a.rxManager.QuickAdd(taskID, scope, strings.TrimSpace(note))
}

// ResolveRecoveryTab 答复 Auto Guard 恢复卡（continue/continue_task/revise）。
func (a *App) ResolveRecoveryTab(tabID string, id string, action string, feedback string) error {
	taskID, err := a.rxTaskForTab(tabID)
	if err != nil {
		return err
	}
	tab := a.rxManager.Tab(taskID)
	if tab == nil {
		return errors.New("Reasonix 会话尚未初始化")
	}
	return tab.Ctrl.ResolveRecovery(id, rxagentRecoveryAction(action), feedback)
}

// rxagentRecoveryAction 把前端 action 字符串转为内核 RecoveryAction。
// bridge 引用内核类型；rx_bindings 经 bridge 中转（main 包不直接引用内核）。
func rxagentRecoveryAction(action string) bridge.RecoveryAction {
	return bridge.RecoveryAction(action)
}

// SlashArgs 返回斜杠命令参数补全（/ 菜单）：基础命令 + /model 的模型列表。
func (a *App) SlashArgs(input string) (map[string]any, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(input, "/"))
	items := make([]map[string]any, 0, 8)
	if trimmed == "" || strings.HasPrefix(trimmed, "model") || strings.HasPrefix(trimmed, "m") {
		taskID, root, err := a.rxActiveTask()
		if err == nil {
			models := bridge.Models(root, "")
			for _, model := range models {
				items = append(items, map[string]any{
					"label":  model.Ref,
					"insert": model.Ref,
					"hint":   model.Provider,
				})
			}
			_ = taskID
		}
	}
	if trimmed == "" || strings.HasPrefix(trimmed, "help") || strings.HasPrefix(trimmed, "h") {
		items = append(items, map[string]any{"label": "help", "insert": "help", "hint": "显示帮助"})
	}
	if trimmed == "" || strings.HasPrefix(trimmed, "new") || strings.HasPrefix(trimmed, "n") {
		items = append(items, map[string]any{"label": "new", "insert": "new", "hint": "开启新会话"})
	}
	return map[string]any{"items": items, "from": 0}, nil
}

// GetActiveThemePack 返回当前主题包（宿主无主题包系统，返回空）。
func (a *App) GetActiveThemePack() (any, error) {
	return nil, nil
}

// GetThemeExperience 返回主题体验（宿主无主题包，返回默认外观）。
func (a *App) GetThemeExperience() (map[string]any, error) {
	return map[string]any{
		"safeMode":          false,
		"activePack":        nil,
		"baseStyle":         "graphite",
		"themeMode":         "auto",
		"themeStyle":        "",
		"language":          "",
		"conversationWidth": "standard",
	}, nil
}
