package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rxagent "reasonix/internal/agent"
	rxcontrol "reasonix/internal/control"
	rxevent "reasonix/internal/event"
	rxeventwire "reasonix/internal/eventwire"
)

// 以下类型与 reasonix 桌面端 wailsjs 生成的 models（main.TabMeta / SessionMeta /
// HistoryMessage / ModelInfo 等）保持 JSON 字段一致，前端无需改动即可消费。

// TabView 对应桌面端 TabMeta（精简到前端主流程使用的字段）。
type TabView struct {
	ID                string `json:"id"`
	Scope             string `json:"scope"`
	WorkspaceRoot     string `json:"workspaceRoot"`
	WorkspaceName     string `json:"workspaceName"`
	WorkspacePath     string `json:"workspacePath,omitempty"`
	GitBranch         string `json:"gitBranch,omitempty"`
	TopicID           string `json:"topicId"`
	TopicTitle        string `json:"topicTitle"`
	SessionPath       string `json:"sessionPath,omitempty"`
	Label             string `json:"label"`
	Ready             bool   `json:"ready"`
	Running           bool   `json:"running"`
	PendingPrompt     bool   `json:"pendingPrompt,omitempty"`
	Cancellable       bool   `json:"cancellable"`
	Mode              string `json:"mode"`
	CollaborationMode string `json:"collaborationMode"`
	ToolApprovalMode  string `json:"toolApprovalMode"`
	TokenMode         string `json:"tokenMode"`
	Goal              string `json:"goal,omitempty"`
	GoalStatus        string `json:"goalStatus,omitempty"`
	Recovered         bool   `json:"recovered,omitempty"`
	StartupErr        string `json:"startupErr,omitempty"`
	Active            bool   `json:"active"`
	Cwd               string `json:"cwd"`
}

// TabViewOf 从控制器构建前端 TabView。
func TabViewOf(tab *TaskTab, taskID string, workspaceRoot string) TabView {
	taskTitle := tab.Title
	if taskTitle == "" {
		taskTitle = taskID
	}
	view := TabView{
		ID:            tab.ID,
		Scope:         "project",
		WorkspaceRoot: workspaceRoot,
		WorkspaceName: workspaceName(workspaceRoot),
		TopicID:       "topic_" + shortHash(taskID),
		TopicTitle:    taskTitle,
		SessionPath:   tab.Ctrl.SessionPath(),
		Label:         tab.Ctrl.Label(),
		Mode:          metaMode(tab.Ctrl),
		Active:        true,
		Cwd:           workspaceRoot,
	}
	view.Running = tab.Ctrl.Running()
	view.Cancellable = tab.Ctrl.RuntimeStatus().Cancellable
	view.Goal = tab.Ctrl.Goal()
	view.GoalStatus = tab.Ctrl.GoalStatus()
	view.ToolApprovalMode = tab.Ctrl.ToolApprovalMode()
	view.Ready = true
	return view
}

func metaMode(ctrl *rxcontrol.Controller) string {
	if ctrl.PlanMode() {
		return "plan"
	}
	return "normal"
}

func workspaceName(root string) string {
	trimmed := strings.TrimRight(root, "/\\")
	if trimmed == "" {
		return "任务工作区"
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

// SessionMetaView 对应前端 SessionMeta（字段与 reasonix frontend types.ts 一致）。
type SessionMetaView struct {
	Path           string `json:"path"`
	Preview        string `json:"preview"`
	Title          string `json:"title,omitempty"`
	Turns          int    `json:"turns"`
	CreatedAt      int64  `json:"createdAt"`      // unix ms
	LastActivityAt int64  `json:"lastActivityAt"` // unix ms
	ModTime        int64  `json:"modTime"`        // unix ms
	Current        bool   `json:"current"`
	Open           bool   `json:"open"`
	Scope          string `json:"scope,omitempty"`
	WorkspaceRoot  string `json:"workspaceRoot,omitempty"`
	TopicID        string `json:"topicId,omitempty"`
	TopicTitle     string `json:"topicTitle,omitempty"`
}

// HistoryMessageView 对应前端 HistoryMessage。
type HistoryMessageView struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Detail    string `json:"detail,omitempty"`
	Turn      int    `json:"turn,omitempty"`
	Model     string `json:"model,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"` // unix ms
	Level     string `json:"level,omitempty"`
}

// HistoryPageView 对应前端 HistoryPage（startTurn/endTurn/totalTurns/hasOlder
// 驱动历史分页状态机）。
type HistoryPageView struct {
	Messages   []HistoryMessageView `json:"messages"`
	StartTurn  int                  `json:"startTurn"`
	EndTurn    int                  `json:"endTurn"`
	TotalTurns int                  `json:"totalTurns"`
	HasOlder   bool                 `json:"hasOlder"`
}

// MetaView 对应前端 Meta（MetaForTab）。eventChannel/cwd 为前端必读字段。
type MetaView struct {
	TabID            string `json:"tabId"`
	Model            string `json:"model"`
	Effort           string `json:"effort"`
	TokenMode        string `json:"tokenMode"`
	Mode             string `json:"mode"`
	ToolApprovalMode string `json:"toolApprovalMode"`
	SessionPath      string `json:"sessionPath"`
	Label            string `json:"label"`
	Ready            bool   `json:"ready"`
	EventChannel     string `json:"eventChannel"`
	Cwd              string `json:"cwd"`
	WorkspaceRoot    string `json:"workspaceRoot,omitempty"`
	WorkspaceName    string `json:"workspaceName,omitempty"`
	Goal             string `json:"goal,omitempty"`
	GoalStatus       string `json:"goalStatus,omitempty"`
}

// --- 方法实现（按任务路由） -------------------------------------------------

// View 返回任务的 TabView。
func (m *Manager) View(taskID string, workspaceRoot string) (TabView, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return TabView{}, fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	return TabViewOf(tab, taskID, workspaceRoot), nil
}

// Sessions 列出任务的历史会话文件（字段对齐前端 SessionMeta 契约）。
func (m *Manager) Sessions(taskID string) ([]SessionMetaView, error) {
	files, err := m.ListSessionFiles(taskID)
	if err != nil {
		return nil, err
	}
	current := m.CurrentSessionPath(taskID)
	titles := m.SessionTitles(taskID)
	sessions := make([]SessionMetaView, 0, len(files))
	for _, file := range files {
		preview, turns := sessionPreview(file.Path)
		created := fileCreatedAt(file.Name, file.Modified)
		modified := parseRFC3339Ms(file.Modified)
		title := titles[filepath.Base(file.Path)]
		if title == "" {
			title = sessionTitle(file.Name)
		}
		sessions = append(sessions, SessionMetaView{
			Path:           file.Path,
			Preview:        preview,
			Title:          title,
			Turns:          turns,
			CreatedAt:      created,
			LastActivityAt: modified,
			ModTime:        modified,
			Current:        current != "" && file.Path == current,
			Scope:          "project",
		})
	}
	return sessions, nil
}

// sessionTitles 读取任务会话目录的标题侧车（.titles.json，与桌面端同约定）。
func (m *Manager) SessionTitles(taskID string) map[string]string {
	raw, err := os.ReadFile(filepath.Join(m.TaskSessionDir(taskID), ".titles.json"))
	if err != nil {
		return map[string]string{}
	}
	var titles map[string]string
	if json.Unmarshal(raw, &titles) != nil {
		return map[string]string{}
	}
	return titles
}

// RenameSession 重命名会话并持久化到标题侧车。
func (m *Manager) RenameSession(taskID string, path string, title string) error {
	dir := m.TaskSessionDir(taskID)
	titles := m.SessionTitles(taskID)
	key := filepath.Base(filepath.Clean(path))
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		delete(titles, key)
	} else {
		titles[key] = trimmed
	}
	raw, err := json.Marshal(titles)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".titles.json"), raw, 0o644)
}

// RemoveSessionTitles 删除会话的标题侧车条目（删除会话时清理）。
func (m *Manager) RemoveSessionTitles(taskID string, path string) error {
	dir := m.TaskSessionDir(taskID)
	titles := m.SessionTitles(taskID)
	key := filepath.Base(filepath.Clean(path))
	if _, ok := titles[key]; !ok {
		return nil
	}
	delete(titles, key)
	raw, err := json.Marshal(titles)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".titles.json"), raw, 0o644)
}

// sessionPreview 从会话文件提取首条用户消息作为预览，并统计用户回合数。
// 轻量路径：直接解析主 .jsonl（预览可接受滞后一回合），避免 ListSessions
// 对每个会话做事件日志全量重放（会话多时列表加载慢）。
func sessionPreview(path string) (string, int) {
	entries, err := readSessionFile(path)
	if err != nil {
		return "", 0
	}
	turns := 0
	preview := ""
	for _, entry := range entries {
		role := entry.Role
		if role == "" {
			if entry.Type == "user" {
				role = "user"
			} else {
				role = "assistant"
			}
		}
		if role == "user" {
			turns++
			if preview == "" {
				preview = entryText(entry)
			}
		}
	}
	if len(preview) > 120 {
		preview = preview[:120] + "…"
	}
	return preview, turns
}

// fileCreatedAt 优先解析文件名时间戳（unix ms），失败回退文件修改时间。
func fileCreatedAt(name string, fallback string) int64 {
	trimmed := strings.TrimSuffix(name, ".jsonl")
	const tsLen = len("20060102-150405.000000000")
	if len(trimmed) >= tsLen {
		if parsed, err := time.Parse("20060102-150405.000000000", trimmed[:tsLen]); err == nil {
			return parsed.UnixMilli()
		}
	}
	return parseRFC3339Ms(fallback)
}

// parseEntryAt 把会话条目时间戳解析为 unix ms（支持 RFC3339 与 ISO 变体）。
func parseEntryAt(value string) int64 {
	if value == "" {
		return 0
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UnixMilli()
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UnixMilli()
	}
	return 0
}

func parseRFC3339Ms(value string) int64 {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func sessionTitle(name string) string {
	trimmed := strings.TrimSuffix(name, ".jsonl")
	// 文件名模板：<YYYYMMDD-HHMMSS.NNNNNNNNN>-<model>.jsonl
	// （模型名可含空格与 "+"，时间戳是固定 26 字符前缀）
	const tsLen = len("20060102-150405.000000000")
	if len(trimmed) > tsLen+1 && trimmed[tsLen] == '-' {
		timestamp := trimmed[:tsLen]
		model := trimmed[tsLen+1:]
		if parsed, err := time.Parse("20060102-150405.000000000", timestamp); err == nil {
			return parsed.Local().Format("2006-01-02 15:04") + " · " + model
		}
	}
	return trimmed
}

// Resume 读取会话文件的对话历史（按页）。beforeTurn>0 时返回该 turn 之前的
// 一页（历史"加载更早"）；否则返回最后一页。
func (m *Manager) Resume(taskID string, path string, beforeTurn int, limit int) (HistoryPageView, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return HistoryPageView{}, fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	if path == "" {
		path = tab.Ctrl.SessionPath()
	}
	if limit <= 0 {
		limit = 50
	}
	entries, err := loadSessionMessages(path)
	if err != nil {
		return HistoryPageView{}, err
	}
	totalTurns := 0
	for _, entry := range entries {
		if entry.Role == "user" || entry.Type == "user" {
			totalTurns++
		}
	}
	// 计算每条消息的 turn 并筛选
	type pageMessage struct {
		view      HistoryMessageView
		userTurn  int
	}
	messages := make([]pageMessage, 0, len(entries))
	turn := 0
	for _, entry := range entries {
		role := entry.Role
		if role == "" {
			if entry.Type == "user" {
				role = "user"
			} else {
				role = "assistant"
			}
		}
		if role == "user" {
			turn++
		}
		if beforeTurn > 0 && turn >= beforeTurn {
			continue
		}
		text := entryText(entry)
		detail := ""
		if entry.Display != "" && entry.Display != text {
			detail = entry.Display
		}
		createdAt := entry.CreatedAt
		if createdAt == 0 {
			createdAt = parseEntryAt(entry.At)
		}
		messages = append(messages, pageMessage{
			view: HistoryMessageView{
				Role:      role,
				Content:   text,
				Detail:    detail,
				Turn:      entry.Turn,
				Model:     entry.Model,
				CreatedAt: createdAt,
			},
			userTurn: turn,
		})
	}
	// 取窗口（beforeTurn 语义：该 turn 之前的最后一页）
	start := 0
	if len(messages) > limit {
		start = len(messages) - limit
	}
	window := messages[start:]
	views := make([]HistoryMessageView, 0, len(window))
	startTurn, endTurn := 0, 0
	for _, item := range window {
		if item.userTurn > 0 {
			if startTurn == 0 || item.userTurn < startTurn {
				startTurn = item.userTurn
			}
			if item.userTurn > endTurn {
				endTurn = item.userTurn
			}
		}
		views = append(views, item.view)
	}
	return HistoryPageView{
		Messages:   views,
		StartTurn:  startTurn,
		EndTurn:    endTurn,
		TotalTurns: totalTurns,
		HasOlder:   startTurn > 1,
	}, nil
}

// Meta 返回任务的运行元信息。
func (m *Manager) Meta(taskID string) (MetaView, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return MetaView{}, fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	return MetaView{
		TabID:            tab.ID,
		Model:            tab.Ctrl.ModelRef(),
		Mode:             metaMode(tab.Ctrl),
		ToolApprovalMode: tab.Ctrl.ToolApprovalMode(),
		SessionPath:      tab.Ctrl.SessionPath(),
		Label:            tab.Ctrl.Label(),
		Ready:            true,
	}, nil
}

// SetModel 切换任务会话的模型/effort/token 模式（重建控制器，会话文件续写）。
func (m *Manager) SetModel(ctx context.Context, taskID string, workspaceRoot string, model string, effort string, tokenMode string) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	oldPath := tab.Ctrl.SessionPath()
	oldCtrl := tab.Ctrl
	if oldCtrl.RuntimeStatus().Cancellable {
		oldCtrl.Cancel()
	}
	history := oldCtrl.History()
	oldCtrl.Close()

	dir := tab.Dir
	newCtrl, err := m.build(ctx, taskID, workspaceRoot, dir, model, effort, tokenMode)
	if err != nil {
		return err
	}
	tab.Ctrl = newCtrl
	if oldPath != "" {
		tab.Ctrl.AdoptHistory(history, oldPath)
	}
	return nil
}

// SubmitWithGoal 提交带目标的一轮（对应 SubmitInitialGoalToTab 的简化路径）。
func (m *Manager) SubmitWithGoal(taskID string, goal string, input string) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	if goal != "" {
		tab.Ctrl.SetGoal(goal)
	}
	tab.Ctrl.Submit(input)
	return nil
}

// Rewind 回滚会话到指定回合（scope: "code"/"conversation"/"both"）。
func (m *Manager) Rewind(taskID string, turn int, scope string) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	var rxScope rxcontrol.RewindScope
	switch scope {
	case "code":
		rxScope = rxcontrol.RewindCode
	case "both":
		rxScope = rxcontrol.RewindBoth
	default:
		rxScope = rxcontrol.RewindConversation
	}
	return tab.Ctrl.Rewind(turn, rxScope)
}

// Fork 在指定回合分支出新会话，返回新会话路径。
func (m *Manager) Fork(taskID string, turn int) (string, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return "", fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	return tab.Ctrl.Fork(turn)
}

// SummarizeFrom 从指定回合起压缩会话为摘要。
func (m *Manager) SummarizeFrom(ctx context.Context, taskID string, turn int) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	return tab.Ctrl.SummarizeFrom(ctx, turn)
}

// SummarizeUpTo 压缩到指定回合为止。
func (m *Manager) SummarizeUpTo(ctx context.Context, taskID string, turn int) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	return tab.Ctrl.SummarizeUpTo(ctx, turn)
}

// AnswerQuestion 回答会话中的提问（answers 为前端 QuestionAnswer[] 的 JSON 形状，
// 即 [{questionId, selected: [string]}]）。
func (m *Manager) AnswerQuestion(taskID string, id string, answers []map[string]any) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	converted := make([]rxevent.AskAnswer, 0, len(answers))
	for _, answer := range answers {
		questionID, _ := answer["questionId"].(string)
		if questionID == "" {
			questionID = id
		}
		var selected []string
		if raw, ok := answer["selected"].([]any); ok {
			for _, item := range raw {
				if text, ok := item.(string); ok {
					selected = append(selected, text)
				}
			}
		}
		converted = append(converted, rxevent.AskAnswer{QuestionID: questionID, Selected: selected})
	}
	tab.Ctrl.AnswerQuestion(id, converted)
	return nil
}

// CommandViews 返回控制器的真实斜杠命令列表。
func (m *Manager) CommandViews(taskID string) ([]map[string]any, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return []map[string]any{}, nil
	}
	commands := tab.Ctrl.Commands()
	out := make([]map[string]any, 0, len(commands))
	for _, command := range commands {
		out = append(out, map[string]any{
			"name":        command.Name,
			"description": command.Description,
		})
	}
	return out, nil
}

// Compose 输出控制器合成后的输入（@ 引用等）。
func (m *Manager) Compose(taskID string, text string) string {
	if tab := m.Tab(taskID); tab != nil {
		return tab.Ctrl.Compose(text)
	}
	return text
}

// --- 会话文件读取 -----------------------------------------------------------

type sessionEntry struct {
	Role       string `json:"role"`
	Type       string `json:"type"`
	Text       string `json:"text"`
	Display    string `json:"display"`
	Content    string `json:"content"`
	RawContent string `json:"raw_content"` // 用户原始输入（content 含注入指令前缀）
	CreatedAt  int64  `json:"createdAt"`   // unix ms
	Turn       int    `json:"turn"`
	Model      string `json:"model"`
	At         string `json:"at"`
}

// entryText 提取会话条目的正文：user 用 raw_content（原始输入），
// assistant 用 content，兼容旧格式的 text/display。
func entryText(entry sessionEntry) string {
	if entry.RawContent != "" {
		return entry.RawContent
	}
	if entry.Content != "" {
		return entry.Content
	}
	if entry.Text != "" {
		return entry.Text
	}
	return entry.Display
}

// loadSessionMessages 读取会话消息：优先经内核 agent.LoadSession 重放
// （native 事件日志模式下主 .jsonl 是滞后快照，直接读会丢最新回合），
// 失败时回退到主文件 JSON 解析。
func loadSessionMessages(path string) ([]sessionEntry, error) {
	if sess, err := rxagent.LoadSession(path); err == nil && sess != nil {
		messages := sess.Snapshot()
		entries := make([]sessionEntry, 0, len(messages))
		turn := 0
		for _, message := range messages {
			role := string(message.Role)
			if role == "user" {
				turn++
			}
			entries = append(entries, sessionEntry{
				Role:       role,
				Content:    message.Content,
				RawContent: message.RawContent,
				CreatedAt:  message.CreatedAt,
				Turn:       turn,
			})
		}
		return entries, nil
	}
	return readSessionFile(path)
}

// readSessionFile 读取 JSONL 会话文件（回退路径；权威读取走 loadSessionMessages）。
func readSessionFile(path string) ([]sessionEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries := make([]sessionEntry, 0)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry sessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

var _ = rxagent.CanonicalSessionPath
var _ = rxevent.Event{}
var _ = rxeventwire.ToWire
