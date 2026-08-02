// Package bridge 是 BTaskAssistant 与 Reasonix 内核之间的适配层。
// module 名以 reasonix/ 开头以满足 Go internal 规则（reasonix/internal/* 仅允许
// reasonix/ 前缀的包导入）。一个任务 = 一个 Reasonix 会话：每个 taskId 持有
// 一个独立控制器的 tab，会话 JSONL 文件落在任务隔离目录，事件经注册的回调
// 转发给宿主（BTask 通过 wails EventsEmit 发出）。
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"reasonix/internal/agent"

	rxboot "reasonix/internal/boot"
	rxconfig "reasonix/internal/config"
	rxcontrol "reasonix/internal/control"
	rxevent "reasonix/internal/event"
	rxeventwire "reasonix/internal/eventwire"
	// provider 子包靠 init 副作用注册 kind（与 reasonix 桌面端/CLI main.go 一致）：
	// 缺失会导致模型切换时 "provider: unknown kind"。
	_ "reasonix/internal/provider/anthropic"
	_ "reasonix/internal/provider/openai"
)

// EventSink 是宿主注册的事件转发回调：内核控制器的事件（会话流/回合状态/
// 工具调用等）按 wire 形状（与 reasonix 桌面端 agent:event 完全一致）回调。
type EventSink func(tabID string, payload map[string]any)

// TaskTab 是「一个任务一个会话」的运行时条目（阶段 1：RuntimeEntry 化）。
// 会话覆盖值（model/effort/tokenMode）与生命周期状态在重复 Activate 时保留，
// 不会被 Ensure 覆盖；RequestSeq 用于并发激活的序号仲裁。
type TaskTab struct {
	TaskID string
	ID     string // tab 标识：task_<taskID 摘要>
	Title  string // 任务显示标题（会话-任务绑定）
	Ctrl   *rxcontrol.Controller
	Dir    string // 会话目录（任务隔离）

	// 会话覆盖值（SetModel/SetEffort 等重建后仍保留，重复 Activate 不覆盖）
	Model     string
	Effort    string
	TokenMode string

	// 工作区身份（阶段 2：绑定任务 Git 工作树）
	WorkspaceIdentity WorkspaceIdentity

	// 生命周期
	RequestSeq uint64    // 最近一次 Activate 的序号（并发激活仲裁）
	LastActive time.Time // 最后激活时间（idle LRU 回收依据，阶段 3）
}

// ErrStaleActivate 表示激活请求序号已过期（更晚的请求已接管该任务）。
var ErrStaleActivate = errors.New("stale reasonix activate request")

// WorkspaceIdentity 是任务工作区的稳定身份：绑定 ID + 真实路径 + 代次。
// 重新绑定/工作树失效时身份变化，宿主据此原子重建控制器（阶段 2）。
type WorkspaceIdentity struct {
	TaskID     string
	BindingID  string
	RootPath   string // 真实路径（符号链接解析后）
	Generation string // baseline commit / 绑定代次
}

// MaxSessionsPerTask 每任务保留的会话文件上限（超出时按最后使用时间清理最旧）。
const MaxSessionsPerTask = 10

// MaxIdleRuntimes 后台保留的 idle 运行时上限（阶段 3：idle LRU 回收）。
// 运行中 / 等待审批提问 / 有后台任务的会话绝不回收。
const MaxIdleRuntimes = 4

// BridgeMaxSessionsPerTask / BridgeMaxIdleRuntimes 供宿主设置面板展示。
const BridgeMaxSessionsPerTask = MaxSessionsPerTask
const BridgeMaxIdleRuntimes = MaxIdleRuntimes

// lastSessionMarker 记录任务最后活跃会话路径（关闭/重建后恢复续写）。
const lastSessionMarker = "last-session.txt"

// Manager 管理全部任务的 Reasonix 会话。
type Manager struct {
	mu         sync.Mutex
	roots      map[string]*TaskTab // taskID → tab
	dataRoot   string              // 会话根目录（BTask 数据目录）
	emit       EventSink
	snapshotWG sync.WaitGroup // 跟踪 in-flight 回合快照（关闭时等待落盘）
}

// saveLastSession 持久化任务最后活跃会话（阶段 2：相对文件名 + 原子替换）。
// 会话文件位于任务隔离目录内，文件名即唯一 ID（同目录无重名）。
func (m *Manager) saveLastSession(taskID string, path string) {
	if path == "" {
		return
	}
	dir := m.TaskSessionDir(taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	base := filepath.Base(path)
	if base == "." || base == "/" || strings.HasSuffix(base, ".tmp") {
		return
	}
	target := filepath.Join(dir, lastSessionMarker)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(base), 0o644); err != nil {
		return
	}
	// 原子替换：临时文件写入完成后 rename 覆盖
	_ = os.Rename(tmp, target)
}

// loadLastSession 读取任务最后活跃会话的绝对路径（不存在/非法时返回空）。
// 兼容旧格式（绝对路径）：仅接受仍位于该任务会话目录内的路径。
func (m *Manager) loadLastSession(taskID string) string {
	dir := m.TaskSessionDir(taskID)
	data, err := os.ReadFile(filepath.Join(dir, lastSessionMarker))
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" || !strings.HasSuffix(raw, ".jsonl") {
		return ""
	}
	path := raw
	if filepath.Base(raw) == raw {
		// 新格式：相对文件名 → 拼回任务目录
		path = filepath.Join(dir, raw)
	}
	cleaned := filepath.Clean(path)
	rel, relErr := filepath.Rel(filepath.Clean(dir), cleaned)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "" // 逃逸任务目录：拒绝
	}
	if info, statErr := os.Stat(cleaned); statErr != nil || info.IsDir() {
		return ""
	}
	return cleaned
}

// NewManager 创建管理器。dataRoot 是任务会话的父目录
// （每个任务的会话落在 <dataRoot>/tasks/<taskID>/reasonix-sessions）。
func NewManager(dataRoot string, emit EventSink) *Manager {
	if emit == nil {
		emit = func(string, map[string]any) {}
	}
	return &Manager{roots: map[string]*TaskTab{}, dataRoot: dataRoot, emit: emit}
}

// TaskSessionDir 返回任务的隔离会话目录。
func (m *Manager) TaskSessionDir(taskID string) string {
	base := m.dataRoot
	if base == "" {
		base = rxconfig.ReasonixHomeDir()
	}
	return filepath.Join(base, "tasks", taskID, "reasonix-sessions")
}

func (m *Manager) tabID(taskID string) string {
	return "task_" + shortHash(taskID)
}

func shortHash(value string) string {
	if value == "" {
		return "default"
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

// Ensure 返回任务的 tab；不存在则构建控制器（首次会加载任务工作区的
// reasonix 配置与模型解析，RequireKey=false 保证无凭据时界面仍可达）。
// Activate 是任务级激活的原子入口：requestSeq 单调递增（宿主前端每次切换
// 任务递增）。同任务乱序请求（旧序号晚到）返回 ErrStaleActivate；不同任务
// 可并行建立控制器（后台保活，阶段 3）。"活动任务"指针由宿主侧
// （rx_bindings.ActivateReasonixTask 的 rxActivateSeq）仲裁。
// title 在控制器构建前即保存（首次打开不丢）；已存在的会话覆盖值
// （model/effort/tokenMode）不会被重复激活覆盖。
func (m *Manager) Activate(ctx context.Context, taskID string, workspaceRoot string, title string, requestSeq uint64) (*TaskTab, error) {
	m.mu.Lock()
	if tab, ok := m.roots[taskID]; ok {
		// 复用：标题/序号更新，覆盖值保留；同任务乱序（旧序号晚到）拒绝
		if requestSeq != 0 && requestSeq < tab.RequestSeq {
			m.mu.Unlock()
			return nil, ErrStaleActivate
		}
		if requestSeq > tab.RequestSeq {
			tab.RequestSeq = requestSeq
		}
		tab.LastActive = time.Now()
		if title != "" {
			tab.Title = title
		}
		if workspaceRoot != "" {
			tab.WorkspaceIdentity.RootPath = workspaceRoot
		}
		m.mu.Unlock()
		return tab, nil
	}
	dir := m.TaskSessionDir(taskID)
	tab := &TaskTab{
		TaskID:     taskID,
		ID:         m.tabID(taskID),
		Title:      title, // 控制器创建前即保存（首次打开标题不丢）
		Dir:        dir,
		RequestSeq: requestSeq,
		LastActive: time.Now(),
		WorkspaceIdentity: WorkspaceIdentity{
			TaskID:   taskID,
			RootPath: workspaceRoot,
		},
	}
	// 占位注册：构建期间并发 Activate 复用它而不是重复构建
	m.roots[taskID] = tab
	m.mu.Unlock()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.mu.Lock()
		if m.roots[taskID] == tab {
			delete(m.roots, taskID)
		}
		m.mu.Unlock()
		return nil, fmt.Errorf("创建 Reasonix 会话目录: %w", err)
	}

	ctrl, err := m.build(ctx, taskID, workspaceRoot, dir, "", "", "")
	if err != nil {
		m.mu.Lock()
		if m.roots[taskID] == tab {
			delete(m.roots, taskID)
		}
		m.mu.Unlock()
		return nil, err
	}
	// 重建后恢复任务最后活跃会话（任务切换/离开后返回时续写原会话内容）。
	if lastPath := m.loadLastSession(taskID); lastPath != "" {
		if loaded, loadErr := agent.LoadSession(lastPath); loadErr == nil {
			ctrl.Resume(loaded, lastPath)
		} else {
			fmt.Fprintf(os.Stderr, "reasonix-bridge: 恢复会话失败 task=%s path=%s: %v\n", taskID, lastPath, loadErr)
		}
	}

	m.mu.Lock()
	// 只有本占位仍是当前条目且序号仍最新时写回；期间被更新的 Activate
	// 接管（占位被替换或 RequestSeq 前进）则关闭本次构建。
	if cur, ok := m.roots[taskID]; ok && cur == tab && tab.RequestSeq == requestSeq {
		tab.Ctrl = ctrl
	} else {
		ctrl.Close()
	}
	m.mu.Unlock()
	// idle 运行时 LRU 回收（锁外；运行中/等待审批不回收）
	m.PruneIdleRuntimes()
	return tab, nil
}

// Ensure 保留旧名（无序号激活：等价于 Activate(seq=0)）。
func (m *Manager) Ensure(ctx context.Context, taskID string, workspaceRoot string) (*TaskTab, error) {
	return m.Activate(ctx, taskID, workspaceRoot, "", 0)
}

// build 构建任务的控制器（boot.Build），事件经 sink 回调转发给宿主。
func (m *Manager) build(
	ctx context.Context,
	taskID string,
	workspaceRoot string,
	sessionDir string,
	model string,
	effort string,
	tokenMode string,
) (*rxcontrol.Controller, error) {
	tabID := m.tabID(taskID)
	// 回合结束时的持久化由宿主触发（与桌面端 tabEventSink 在 TurnDone 时
	// scheduleTabSnapshot 一致）：内核不自动落盘，漏触发会导致会话文件
	// 停留在上一轮。
	var ctrlRef atomic.Pointer[rxcontrol.Controller]
	sink := rxevent.FuncSink(func(e rxevent.Event) {
		if e.Kind == rxevent.TurnDone {
			if ctrl := ctrlRef.Load(); ctrl != nil {
				m.snapshotWG.Add(1)
				go func() {
					defer m.snapshotWG.Done()
					if err := ctrl.Snapshot(); err != nil {
						fmt.Fprintf(os.Stderr, "reasonix-bridge: 回合快照失败 task=%s: %v\n", taskID, err)
					}
				}()
			}
		}
		m.emit(tabID, toWire(e, tabID))
	})
	var effortOverride *string
	if effort != "" {
		effortOverride = &effort
	}
	ctrl, err := rxboot.Build(ctx, rxboot.Options{
		Model:          model,
		RequireKey:     false,
		Sink:           rxevent.Sync(sink),
		WorkspaceRoot:  workspaceRoot,
		SessionDir:     sessionDir,
		EffortOverride: effortOverride,
		TokenMode:      tokenMode,
	})
	if err != nil {
		return nil, err
	}
	ctrlRef.Store(ctrl)
	return ctrl, nil
}

// SetTaskTitle 记录任务显示标题（ReasonixEnsureTab 时由宿主传入）——
// 绑定到任务运行时条目，TabView 的 TopicTitle 展示任务标题。
func (m *Manager) SetTaskTitle(taskID string, title string) {
	if title == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if tab, ok := m.roots[taskID]; ok {
		tab.Title = title
	}
}

// Tab 返回任务的 tab（未构建时返回 nil）。
func (m *Manager) Tab(taskID string) *TaskTab {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.roots[taskID]
}

// NewSession 为任务开一个新会话文件并激活，然后按上限清理最旧会话。
func (m *Manager) NewSession(taskID string) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	if err := tab.Ctrl.NewSession(); err != nil {
		return err
	}
	m.saveLastSession(taskID, tab.Ctrl.SessionPath())
	m.PruneOldSessions(taskID, MaxSessionsPerTask)
	return nil
}

// MigrateLegacyLastSession 把旧格式（绝对路径）last-session.txt 非破坏性
// 迁移为相对文件名；路径不合法（越界/不存在）时保留原文件并输出警告，
// 不自动删除（阶段 7）。
func (m *Manager) MigrateLegacyLastSession(taskID string) {
	dir := m.TaskSessionDir(taskID)
	marker := filepath.Join(dir, lastSessionMarker)
	data, err := os.ReadFile(marker)
	if err != nil {
		return
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" || !strings.HasSuffix(raw, ".jsonl") {
		return
	}
	if filepath.Base(raw) == raw {
		return // 已是相对格式
	}
	cleaned := filepath.Clean(raw)
	rel, relErr := filepath.Rel(filepath.Clean(dir), cleaned)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		fmt.Fprintf(os.Stderr, "reasonix-bridge: 任务 %s 的 last-session 指向目录外，保留原文件并忽略: %s\n", taskID, raw)
		return
	}
	if info, statErr := os.Stat(cleaned); statErr != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "reasonix-bridge: 任务 %s 的 last-session 文件不存在，保留原文件: %s\n", taskID, raw)
		return
	}
	// 原子改写为相对格式
	tmp := marker + ".tmp"
	if err := os.WriteFile(tmp, []byte(filepath.Base(raw)), 0o644); err == nil {
		_ = os.Rename(tmp, marker)
	}
}

// MigrateAllLegacyLastSessions 遍历全部任务目录执行迁移。
func (m *Manager) MigrateAllLegacyLastSessions() {
	base := m.dataRoot
	if base == "" {
		return
	}
	tasksDir := filepath.Join(base, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m.MigrateLegacyLastSession(entry.Name())
	}
}

// PruneIdleRuntimes 回收超出上限的 idle 运行时（按最后活跃时间 LRU）。
// 返回回收数量。running / 等待审批提问 / 有后台任务的会话绝不回收；
// 回收前先快照并记录最后会话（Close 语义）。
func (m *Manager) PruneIdleRuntimes() int {
	m.mu.Lock()
	type idleEntry struct {
		taskID string
		tab    *TaskTab
	}
	var idle []idleEntry
	for taskID, tab := range m.roots {
		if tab == nil || tab.Ctrl == nil {
			continue
		}
		status := tab.Ctrl.RuntimeStatus()
		if status.Running || status.PendingPrompt || status.BackgroundJobs > 0 {
			continue
		}
		idle = append(idle, idleEntry{taskID: taskID, tab: tab})
	}
	if len(idle) <= MaxIdleRuntimes {
		m.mu.Unlock()
		return 0
	}
	sort.Slice(idle, func(i, j int) bool {
		return idle[i].tab.LastActive.Before(idle[j].tab.LastActive)
	})
	excess := idle[:len(idle)-MaxIdleRuntimes]
	m.mu.Unlock()
	removed := 0
	for _, entry := range excess {
		m.Close(entry.taskID)
		removed++
	}
	if removed > 0 {
		fmt.Fprintf(os.Stderr, "reasonix-bridge: 回收 idle 运行时 %d 个（上限 %d）\n", removed, MaxIdleRuntimes)
	}
	return removed
}

// sessionSidecars 返回会话路径对应的全部侧车文件（与桌面端 deleteSessionFile
// 及 rx_bindings.DeleteSession 的清单一致）。
func sessionSidecars(path string) []string {
	stem := strings.TrimSuffix(path, ".jsonl")
	return []string{
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
	}
}

// removeSessionFiles 删除会话主文件与全部侧车（不存在时忽略）。
func removeSessionFiles(path string) {
	_ = os.Remove(path)
	for _, sidecar := range sessionSidecars(path) {
		_ = os.Remove(sidecar)
	}
}

// PruneOldSessions 清理任务会话目录：按最后使用时间（mtime）保留最新 keep 个
// 会话（当前激活会话始终保留），删除更旧的会话文件与侧车。
func (m *Manager) PruneOldSessions(taskID string, keep int) {
	if keep < 1 {
		keep = 1
	}
	files, err := m.ListSessionFiles(taskID)
	if err != nil {
		return
	}
	current := ""
	if tab := m.Tab(taskID); tab != nil && tab.Ctrl != nil {
		current = filepath.Clean(tab.Ctrl.SessionPath())
	}
	// ListSessionFiles 按 mtime 倒序（最新在前）——保留前 keep 个（当前会话
	// 即使在最旧也必须保留，因此先收集删除集合再检查）。
	if len(files) <= keep {
		return
	}
	excess := files[keep:]
	removed := 0
	for _, file := range excess {
		if filepath.Clean(file.Path) == current {
			continue
		}
		removeSessionFiles(file.Path)
		removed++
	}
	if removed > 0 {
		fmt.Fprintf(os.Stderr, "reasonix-bridge: 清理旧会话 task=%s 删除 %d 个（上限 %d）\n", taskID, removed, keep)
	}
}

// Submit 提交一轮对话（非阻塞：事件流经回调发出）。
func (m *Manager) Submit(taskID string, input string) error {
	tab := m.Tab(taskID)
	if tab == nil {
		return fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	if strings.TrimSpace(input) == "" {
		return fmt.Errorf("消息不能为空")
	}
	tab.Ctrl.Submit(input)
	return nil
}

// Cancel 取消进行中的回合。
func (m *Manager) Cancel(taskID string) {
	if tab := m.Tab(taskID); tab != nil {
		tab.Ctrl.Cancel()
	}
}

// Close 关闭任务的控制器并移除运行时（会话文件保留）。
func (m *Manager) Close(taskID string) {
	m.mu.Lock()
	tab := m.roots[taskID]
	delete(m.roots, taskID)
	m.mu.Unlock()
	if tab != nil && tab.Ctrl != nil {
		// 记录最后活跃会话（离开详情页后重建时恢复续写）。
		m.saveLastSession(taskID, tab.Ctrl.SessionPath())
		// 等待 in-flight 回合快照落盘，避免与删除/关闭竞态（对应桌面端
		// quiesceTabAutosave 的 #4384 类问题）。
		m.snapshotWG.Wait()
		tab.Ctrl.Close()
	}
}

// Shutdown 关闭全部任务的控制器。
func (m *Manager) Shutdown() {
	m.mu.Lock()
	tabs := make([]*TaskTab, 0, len(m.roots))
	for _, tab := range m.roots {
		tabs = append(tabs, tab)
	}
	m.roots = map[string]*TaskTab{}
	m.mu.Unlock()
	for _, tab := range tabs {
		if tab.Ctrl != nil {
			m.saveLastSession(tab.TaskID, tab.Ctrl.SessionPath())
		}
	}
	m.snapshotWG.Wait()
	for _, tab := range tabs {
		if tab.Ctrl != nil {
			tab.Ctrl.Close()
		}
	}
}

// ListSessionFiles 列出任务会话目录下的 .jsonl 会话文件（按修改时间倒序）。
func (m *Manager) ListSessionFiles(taskID string) ([]SessionFile, error) {
	dir := m.TaskSessionDir(taskID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]SessionFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		// 排除侧车文件：.events.jsonl 是事件流，不构成独立会话
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".events.jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, SessionFile{
			Path:      filepath.Join(dir, entry.Name()),
			Name:      entry.Name(),
			Modified:  info.ModTime().UTC().Format(time.RFC3339),
			SizeBytes: info.Size(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Modified > files[j].Modified })
	return files, nil
}

// SessionFile 描述一个会话文件。
type SessionFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Modified  string `json:"modified"`
	SizeBytes int64  `json:"sizeBytes"`
}

// CurrentSessionPath 返回任务当前会话路径。
func (m *Manager) CurrentSessionPath(taskID string) string {
	if tab := m.Tab(taskID); tab != nil {
		return tab.Ctrl.SessionPath()
	}
	return ""
}

// --- wire 转换 ---------------------------------------------------------------

// toWire 把内核事件转换为与 reasonix 桌面端 agent:event 一致的 payload
// （wireEventTab：eventwire.Event + tabId 路由字段）。
func toWire(e rxevent.Event, tabID string) map[string]any {
	payload := map[string]any{
		"tabId": tabID,
	}
	w := rxeventwire.ToWire(e)
	raw, err := json.Marshal(w)
	if err == nil {
		var fields map[string]any
		if json.Unmarshal(raw, &fields) == nil {
			for key, value := range fields {
				payload[key] = value
			}
		}
	}
	payload["sessionHitTokens"] = e.SessionHit
	payload["sessionMissTokens"] = e.SessionMiss
	return payload
}
