package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/execution"
	"github.com/blue7zz/BTaskAssistant/internal/gitrepo"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

const (
	maxPromptBytes          = 256 * 1024
	maxMessageBytes         = 4 * 1024 * 1024
	maxEventTextBytes       = 32 * 1024
	interruptedRestartError = "应用已重启，上一活动运行已中断"
)

type Service struct {
	store             Store
	factory           RuntimeFactory
	executable        string
	startupTimeout    time.Duration
	requestTimeout    time.Duration
	shutdownGrace     time.Duration
	permissionTimeout time.Duration
	now               func() time.Time
	delivery          *eventDeliveryQueue
	git               *gitrepo.Service
	execution         *execution.Service

	mutex    sync.Mutex
	sessions map[string]*managedSession
	closed   bool
}

type managedSession struct {
	mutex         sync.Mutex
	record        storage.AgentSessionRecord
	workspaceRoot string
	runtime       Runtime
	run           *activeRun
	closing       bool
	gate          gateIdentity
}

type activeRun struct {
	record           storage.ExecutionRunRecord
	assistant        storage.AgentMessageRecord
	assistantContent string
	assistantStarted bool
	reasoningChars   int
	pendingDeltas    []pendingDelta
	pendingDeltaSize int
	deltaTimer       *time.Timer
	errorMessage     string
	stopRequested    bool
	tools            map[string]storage.ToolCallRecord
	toolArgs         map[string]json.RawMessage
	permissions      map[string]*pendingPermission
	receipts         map[string]permissionReceipt
}

type pendingDelta struct {
	kind             string
	delta            string
	accumulatedChars int
}

type pendingPermission struct {
	request              storage.PermissionRequestRecord
	extensionUIRequestID string
	expiresAt            time.Time
}

type permissionReceipt struct {
	RequestID        string
	Capability       string
	NormalizedTarget string
	ArgsDigest       string
	Scope            string
	Decision         string
	Mutating         bool
	OperationStarted bool
}

type entriesResponse struct {
	Entries []sessionEntry `json:"entries"`
	LeafID  *string        `json:"leafId"`
}

type sessionEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

func NewService(store Store, options ServiceOptions) *Service {
	factory := options.RuntimeFactory
	if factory == nil {
		factory = nativeRuntimeFactory{}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	if options.StartupTimeout <= 0 {
		options.StartupTimeout = defaultStartupTimeout
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = defaultRequestTimeout
	}
	if options.ShutdownGrace <= 0 {
		options.ShutdownGrace = defaultShutdownGrace
	}
	if options.PermissionTimeout <= 0 {
		options.PermissionTimeout = 5 * time.Minute
	}
	gitService := options.GitService
	if gitService == nil {
		gitService = gitrepo.NewService(store)
	}
	executionService := options.ExecutionService
	if executionService == nil {
		executionService = execution.NewService()
	}
	return &Service{
		store:             store,
		factory:           factory,
		executable:        options.Executable,
		startupTimeout:    options.StartupTimeout,
		requestTimeout:    options.RequestTimeout,
		shutdownGrace:     options.ShutdownGrace,
		permissionTimeout: options.PermissionTimeout,
		now:               now,
		delivery:          newEventDeliveryQueue(options.Emit),
		git:               gitService,
		execution:         executionService,
		sessions:          make(map[string]*managedSession),
	}
}

func (service *Service) RecoverInterrupted() error {
	if _, err := service.store.ExpirePendingPermissionRequests(
		service.timestamp(),
		"应用已重启，待处理权限请求已过期",
	); err != nil {
		return err
	}
	return service.store.InterruptActiveAgentActivity(
		service.timestamp(),
		interruptedRestartError,
	)
}

func (service *Service) Sessions(taskID string) ([]storage.AgentSessionRecord, error) {
	if _, err := service.store.EnsureTaskWorkspace(taskID); err != nil {
		return nil, err
	}
	return service.store.AgentSessions(taskID)
}

func (service *Service) Messages(
	taskID string,
	sessionID string,
) ([]storage.AgentMessageRecord, error) {
	if _, err := service.store.AgentSession(taskID, sessionID); err != nil {
		return nil, err
	}
	return service.store.AgentMessages(taskID, sessionID)
}

func (service *Service) HistoryPage(request HistoryPageRequest) (HistoryPage, error) {
	if _, err := service.store.AgentSession(request.TaskID, request.SessionID); err != nil {
		return HistoryPage{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = 60
	}
	if limit < 1 || limit > 200 {
		return HistoryPage{}, errors.New("PI 历史分页大小必须在 1 到 200 之间")
	}
	var beforeSequence int64
	if request.Cursor != "" {
		parsed, err := strconv.ParseInt(request.Cursor, 10, 64)
		if err != nil || parsed < 1 {
			return HistoryPage{}, errors.New("PI 历史游标无效")
		}
		beforeSequence = parsed
	}
	messages, hasMore, err := service.store.AgentMessagePage(
		request.TaskID,
		request.SessionID,
		beforeSequence,
		limit,
	)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Messages: messages, HasMore: hasMore}
	if hasMore && len(messages) > 0 {
		page.NextCursor = strconv.FormatInt(messages[0].Sequence, 10)
	}
	return page, nil
}

func (service *Service) Runs(
	taskID string,
	sessionID string,
) ([]storage.ExecutionRunRecord, error) {
	if _, err := service.store.AgentSession(taskID, sessionID); err != nil {
		return nil, err
	}
	return service.store.ExecutionRuns(taskID, sessionID)
}

func (service *Service) CreateSession(
	ctx context.Context,
	request CreateSessionRequest,
) (storage.AgentSessionRecord, error) {
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return storage.AgentSessionRecord{}, err
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" {
		request.Title = "PI 会话"
	}
	if len(request.Title) > 200 || strings.ContainsAny(request.Title, "\x00\r\n") {
		return storage.AgentSessionRecord{}, errors.New("PI 会话标题不正确")
	}
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = "ask"
	}
	if request.Mode != "ask" && request.Mode != "plan" && request.Mode != "agent" {
		return storage.AgentSessionRecord{}, errors.New("PI 会话模式不受支持")
	}
	if err := validateThinkingLevel(request.ThinkingLevel); err != nil {
		return storage.AgentSessionRecord{}, err
	}
	if len(strings.TrimSpace(request.Model)) > 200 {
		return storage.AgentSessionRecord{}, errors.New("PI 模型标识过长")
	}
	resourcePolicy, err := normalizePIResourcePolicy(request.ResourcePolicy)
	if err != nil {
		return storage.AgentSessionRecord{}, err
	}

	now := service.timestamp()
	record := storage.AgentSessionRecord{
		ID:             newID("session"),
		TaskID:         request.TaskID,
		Engine:         "pi",
		Title:          request.Title,
		Mode:           request.Mode,
		ResourcePolicy: resourcePolicy,
		State:          "created",
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActiveAt:   now,
	}
	if model := strings.TrimSpace(request.Model); model != "" {
		record.Model = &model
	}
	if thinking := strings.ToLower(strings.TrimSpace(request.ThinkingLevel)); thinking != "" {
		record.ThinkingLevel = &thinking
	}
	if err := service.store.UpsertAgentSession(record); err != nil {
		return storage.AgentSessionRecord{}, err
	}
	managed := &managedSession{record: record, workspaceRoot: workspace.RootPath}
	service.mutex.Lock()
	if service.closed {
		service.mutex.Unlock()
		return storage.AgentSessionRecord{}, errors.New("PI 会话服务已关闭")
	}
	service.sessions[sessionKey(record.TaskID, record.ID)] = managed
	service.mutex.Unlock()

	managed.mutex.Lock()
	err = service.startRuntimeLocked(ctx, managed, workspace)
	updated := managed.record
	managed.mutex.Unlock()
	if err != nil {
		return updated, err
	}
	return updated, nil
}

func (service *Service) ResumeSession(
	ctx context.Context,
	request SessionRequest,
) (storage.AgentSessionRecord, error) {
	managed, err := service.loadManagedSession(request.TaskID, request.SessionID)
	if err != nil {
		return storage.AgentSessionRecord{}, err
	}
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return storage.AgentSessionRecord{}, err
	}
	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	if managed.run != nil {
		return managed.record, errors.New("PI 正在运行，无需恢复当前会话")
	}
	if err := service.startRuntimeLocked(ctx, managed, workspace); err != nil {
		return managed.record, err
	}
	return managed.record, nil
}

func (service *Service) SendPrompt(
	ctx context.Context,
	request PromptRequest,
) (storage.ExecutionRunRecord, error) {
	request, err := normalizePromptRequest(request)
	if err != nil {
		return storage.ExecutionRunRecord{}, err
	}
	managed, err := service.loadManagedSession(request.TaskID, request.SessionID)
	if err != nil {
		return storage.ExecutionRunRecord{}, err
	}
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return storage.ExecutionRunRecord{}, err
	}

	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	managed.workspaceRoot = workspace.RootPath
	if managed.run != nil {
		return storage.ExecutionRunRecord{}, errors.New("该任务已有正在运行的 PI 请求")
	}
	if err := service.startRuntimeLocked(ctx, managed, workspace); err != nil {
		return storage.ExecutionRunRecord{}, err
	}

	now := service.timestamp()
	promptReferences, err := service.resolvePromptReferences(workspace, request, now)
	if err != nil {
		return storage.ExecutionRunRecord{}, err
	}
	runID := newID("run")
	run := storage.ExecutionRunRecord{
		ID:         runID,
		TaskID:     request.TaskID,
		SessionID:  request.SessionID,
		Mode:       managed.record.Mode,
		State:      "queued",
		EventsPath: "runs/" + runID + "/events.jsonl",
		StdoutPath: "runs/" + runID + "/stdout.jsonl",
		StderrPath: "runs/" + runID + "/stderr.log",
		ResultPath: "runs/" + runID + "/result.md",
		StartedAt:  now,
	}
	if binding, bindingErr := service.git.Binding(ctx, request.TaskID); bindingErr == nil && binding.State == "ready" {
		bindingID := binding.ID
		baseline := binding.BaselineCommit
		run.GitBindingID = &bindingID
		run.BaselineCommit = &baseline
	}
	if err := prepareRunFiles(workspace.RootPath, run); err != nil {
		return storage.ExecutionRunRecord{}, err
	}
	if err := service.store.UpsertExecutionRun(run); err != nil {
		discardRunFiles(workspace.RootPath, run.ID)
		return storage.ExecutionRunRecord{}, err
	}
	userContent := request.Message
	userMessage := storage.AgentMessageRecord{
		ID: newID("message"), TaskID: request.TaskID, SessionID: request.SessionID,
		RunID: &runID, Role: "user", Kind: "text", Status: "complete",
		Content: &userContent, Sequence: service.nextSequenceLocked(managed),
		CreatedAt: now, CompletedAt: &now,
	}
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentMessageWithReferences(
		userMessage,
		managed.record,
		promptReferences.references,
	); err != nil {
		service.failRunRecord(run, err.Error())
		return storage.ExecutionRunRecord{}, err
	}
	assistantContent := ""
	assistant := storage.AgentMessageRecord{
		ID: newID("message"), TaskID: request.TaskID, SessionID: request.SessionID,
		RunID: &runID, Role: "assistant", Kind: "text", Status: "streaming",
		Content: &assistantContent, Sequence: service.nextSequenceLocked(managed), CreatedAt: now,
	}
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentMessageAndUpdateSession(assistant, managed.record); err != nil {
		service.failRunRecord(run, err.Error())
		return storage.ExecutionRunRecord{}, err
	}
	managed.run = &activeRun{
		record: run, assistant: assistant, assistantStarted: true,
		tools:       make(map[string]storage.ToolCallRecord),
		toolArgs:    make(map[string]json.RawMessage),
		permissions: make(map[string]*pendingPermission),
		receipts:    make(map[string]permissionReceipt),
	}
	managed.record.State = "running"
	managed.record.ErrorMessage = nil
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentSession(managed.record); err != nil {
		managed.run = nil
		service.failRunRecord(run, err.Error())
		return storage.ExecutionRunRecord{}, err
	}
	if err := service.emitLocked(managed, "run.state", runID, "", map[string]any{
		"state": "queued",
	}); err != nil {
		service.finishRunLocked(managed, "failed", err.Error())
		return storage.ExecutionRunRecord{}, err
	}
	_ = service.emitLocked(managed, "message.start", runID, "", map[string]any{
		"messageId": userMessage.ID, "role": "user", "kind": "text",
	})
	_ = service.emitLocked(managed, "message.end", runID, "", map[string]any{
		"messageId": userMessage.ID, "status": "complete", "content": request.Message,
	})
	_ = service.emitLocked(managed, "message.start", runID, "", map[string]any{
		"messageId": assistant.ID, "role": "assistant", "kind": "text",
	})

	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	promptFields := map[string]any{"message": promptReferences.prompt}
	if len(promptReferences.images) > 0 {
		promptFields["images"] = promptReferences.images
	}
	runtime := managed.runtime
	// PI runs before_agent_start hooks before acknowledging a prompt. The BTask
	// gate answers those hooks from the event consumer, which needs this mutex.
	managed.mutex.Unlock()
	err = runtime.Call(
		requestCtx,
		"prompt",
		promptFields,
		nil,
	)
	cancel()
	managed.mutex.Lock()
	if err != nil {
		err = contextualizePICredentialError(err, managed.record.ResourcePolicy)
		if managed.runtime == runtime && managed.run != nil && managed.run.record.ID == runID {
			service.finishRunLocked(managed, "failed", sanitizeError(err.Error(), workspace.RootPath))
		}
		return storage.ExecutionRunRecord{}, err
	}
	return run, nil
}

func normalizePromptRequest(request PromptRequest) (PromptRequest, error) {
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		if len(request.ResourceIDs) == 0 {
			return PromptRequest{}, errors.New("消息不能为空")
		}
		request.Message = "请查看所附的当前任务资源。"
	}
	if len([]byte(request.Message)) > maxPromptBytes || strings.ContainsRune(request.Message, '\x00') {
		return PromptRequest{}, errors.New("消息不能超过 256 KiB 且不得包含 NUL")
	}
	return request, nil
}

func (service *Service) Steer(
	ctx context.Context,
	request PromptRequest,
) (storage.AgentMessageRecord, error) {
	return service.queuePrompt(ctx, request, "steer")
}

func (service *Service) FollowUp(
	ctx context.Context,
	request PromptRequest,
) (storage.AgentMessageRecord, error) {
	return service.queuePrompt(ctx, request, "follow_up")
}

func (service *Service) queuePrompt(
	ctx context.Context,
	request PromptRequest,
	command string,
) (storage.AgentMessageRecord, error) {
	request, err := normalizePromptRequest(request)
	if err != nil {
		return storage.AgentMessageRecord{}, err
	}
	if command != "steer" && command != "follow_up" {
		return storage.AgentMessageRecord{}, errors.New("PI 队列命令不受支持")
	}
	managed, err := service.loadManagedSession(request.TaskID, request.SessionID)
	if err != nil {
		return storage.AgentMessageRecord{}, err
	}
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return storage.AgentMessageRecord{}, err
	}

	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	managed.workspaceRoot = workspace.RootPath
	if managed.run == nil || managed.runtime == nil || managed.run.stopRequested {
		return storage.AgentMessageRecord{}, errors.New("当前没有可接收队列消息的 PI 运行")
	}
	now := service.timestamp()
	promptReferences, err := service.resolvePromptReferences(workspace, request, now)
	if err != nil {
		return storage.AgentMessageRecord{}, err
	}
	runID := managed.run.record.ID
	content := request.Message
	message := storage.AgentMessageRecord{
		ID: newID("message"), TaskID: request.TaskID, SessionID: request.SessionID,
		RunID: &runID, Role: "user", Kind: "text", Status: "pending",
		Content: &content, Sequence: service.nextSequenceLocked(managed), CreatedAt: now,
	}
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentMessageWithReferences(
		message,
		managed.record,
		promptReferences.references,
	); err != nil {
		return storage.AgentMessageRecord{}, err
	}
	_ = service.emitLocked(managed, "message.start", runID, "", map[string]any{
		"messageId": message.ID, "role": "user", "kind": "text", "content": content,
	})
	fields := map[string]any{"message": promptReferences.prompt}
	if len(promptReferences.images) > 0 {
		fields["images"] = promptReferences.images
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	err = managed.runtime.Call(requestCtx, command, fields, nil)
	cancel()
	if err != nil {
		completedAt := service.timestamp()
		message.Status = "error"
		message.CompletedAt = &completedAt
		_ = service.store.UpsertAgentMessage(message)
		_ = service.emitLocked(managed, "message.end", runID, "", map[string]any{
			"messageId": message.ID, "status": "error", "content": content,
		})
		return message, err
	}
	references, err := service.store.AgentMessageReferences(
		request.TaskID,
		request.SessionID,
		message.ID,
	)
	if err != nil {
		return storage.AgentMessageRecord{}, err
	}
	message.References = references
	_ = service.emitLocked(managed, "queue.queued", runID, "", map[string]any{
		"messageId": message.ID, "behavior": command,
	})
	return message, nil
}

func (service *Service) Abort(ctx context.Context, request AbortRequest) error {
	managed, err := service.loadManagedSession(request.TaskID, request.SessionID)
	if err != nil {
		return err
	}
	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	if managed.run == nil || managed.run.record.ID != request.RunID {
		return errors.New("没有匹配的活动 PI 运行")
	}
	service.execution.StopRun(request.TaskID, request.SessionID, request.RunID)
	service.cancelPendingPermissionsLocked(managed, "用户停止运行，待处理权限请求已取消")
	managed.run.stopRequested = true
	managed.run.record.State = "stopping"
	if err := service.store.UpsertExecutionRun(managed.run.record); err != nil {
		return err
	}
	managed.record.State = "stopping"
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentSession(managed.record); err != nil {
		return err
	}
	_ = service.emitLocked(managed, "run.state", request.RunID, "", map[string]any{
		"state": "stopping", "reason": "用户已请求停止",
	})
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	if err := managed.runtime.Call(requestCtx, "abort", nil, nil); err != nil {
		service.finishRunLocked(managed, "failed", sanitizeError(err.Error(), ""))
		return err
	}
	return nil
}

func (service *Service) Close(ctx context.Context) error {
	service.mutex.Lock()
	if service.closed {
		service.mutex.Unlock()
		return nil
	}
	service.closed = true
	sessions := make([]*managedSession, 0, len(service.sessions))
	for _, managed := range service.sessions {
		sessions = append(sessions, managed)
	}
	service.mutex.Unlock()

	var firstErr error
	for _, managed := range sessions {
		managed.mutex.Lock()
		managed.closing = true
		runtime := managed.runtime
		service.cancelPendingPermissionsLocked(managed, "应用关闭，待处理权限请求已取消")
		if managed.run != nil {
			service.finishRunLocked(managed, "interrupted", "应用关闭时运行尚未完成")
		}
		managed.mutex.Unlock()
		_, _ = service.store.ExpireSessionPermissionGrants(
			managed.record.TaskID, managed.record.ID, service.timestamp(),
		)
		if runtime != nil {
			if err := runtime.Close(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if err := service.delivery.close(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := service.execution.Close(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (service *Service) StopToolExecution(request execution.StopRequest) error {
	tool, err := service.store.ToolCall(request.TaskID, request.ToolCallID)
	if err != nil {
		return err
	}
	if tool.SessionID != request.SessionID || tool.RunID != request.RunID ||
		tool.ToolName != "btask_shell" || tool.State != "running" {
		return errors.New("没有匹配的运行中 BTask Shell 工具")
	}
	return service.execution.Stop(request)
}

// Command 把会话窗口的扩展 RPC 命令透传给 PI 进程：get_commands、bash、
// set_model、set_thinking_level、compact、fork 等。
func (service *Service) Command(
	ctx context.Context,
	request CommandRequest,
) (map[string]any, error) {
	if strings.TrimSpace(request.Type) == "" {
		return nil, errors.New("PI 会话命令不能为空")
	}
	managed, err := service.loadManagedSession(request.TaskID, request.SessionID)
	if err != nil {
		return nil, err
	}
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return nil, err
	}
	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	if managed.runtime == nil {
		if err := service.startRuntimeLocked(ctx, managed, workspace); err != nil {
			return nil, err
		}
	}
	var result map[string]any
	if err := managed.runtime.Call(ctx, request.Type, request.Payload, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (service *Service) ActiveTask(taskID string) bool {
	service.mutex.Lock()
	sessions := make([]*managedSession, 0, len(service.sessions))
	for _, managed := range service.sessions {
		if managed.record.TaskID == taskID {
			sessions = append(sessions, managed)
		}
	}
	service.mutex.Unlock()
	for _, managed := range sessions {
		managed.mutex.Lock()
		active := managed.run != nil
		managed.mutex.Unlock()
		if active {
			return true
		}
	}
	return false
}

func (service *Service) loadManagedSession(
	taskID string,
	sessionID string,
) (*managedSession, error) {
	key := sessionKey(taskID, sessionID)
	service.mutex.Lock()
	if service.closed {
		service.mutex.Unlock()
		return nil, errors.New("PI 会话服务已关闭")
	}
	if managed := service.sessions[key]; managed != nil {
		service.mutex.Unlock()
		return managed, nil
	}
	service.mutex.Unlock()
	record, err := service.store.AgentSession(taskID, sessionID)
	if err != nil {
		return nil, err
	}
	managed := &managedSession{record: record}
	service.mutex.Lock()
	if existing := service.sessions[key]; existing != nil {
		service.mutex.Unlock()
		return existing, nil
	}
	service.sessions[key] = managed
	service.mutex.Unlock()
	return managed, nil
}

func (service *Service) startRuntimeLocked(
	ctx context.Context,
	managed *managedSession,
	workspace storage.TaskWorkspaceRecord,
) error {
	managed.workspaceRoot = workspace.RootPath
	if managed.runtime != nil {
		select {
		case <-managed.runtime.Done():
			managed.runtime = nil
		default:
			return nil
		}
	}
	if _, err := service.store.ExpireSessionPermissionGrants(
		managed.record.TaskID, managed.record.ID, service.timestamp(),
	); err != nil {
		return err
	}
	managed.record.State = "starting"
	managed.record.ErrorMessage = nil
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentSession(managed.record); err != nil {
		return err
	}
	_ = service.emitLocked(managed, "session.state", "", "", map[string]any{
		"state": "starting",
	})

	configDir, err := resolvePIConfigDirectory(
		managed.record.ResourcePolicy,
		filepath.Join(workspace.RootPath, ".btask", "pi-agent"),
	)
	if err != nil {
		service.failSessionLocked(managed, sanitizeError(err.Error(), workspace.RootPath))
		return err
	}
	options := ProcessOptions{
		Executable:     service.executable,
		WorkDir:        workspace.RootPath,
		ConfigDir:      configDir,
		SessionDir:     filepath.Join(workspace.RootPath, ".btask", "pi-sessions"),
		StartupTimeout: service.startupTimeout,
		RequestTimeout: service.requestTimeout,
		ShutdownGrace:  service.shutdownGrace,
	}
	gate, err := installGateExtension(
		workspace.RootPath,
		managed.record.TaskID,
		managed.record.ID,
		managed.record.Mode,
	)
	if err != nil {
		service.failSessionLocked(managed, sanitizeError(err.Error(), workspace.RootPath))
		return err
	}
	options.Gate = &GateProcessOptions{
		ExtensionPath: gate.ExtensionPath,
		Version:       gate.Version,
		Nonce:         gate.Nonce,
		SHA256:        gate.SHA256,
	}
	runtime, state, err := service.factory.Start(ctx, options)
	if err != nil {
		service.failSessionLocked(managed, sanitizeError(err.Error(), workspace.RootPath))
		return err
	}
	if managed.record.ExternalSessionPath != nil {
		path, err := validateSessionPath(options.SessionDir, *managed.record.ExternalSessionPath, false)
		if err != nil {
			_ = runtime.Close(context.Background())
			service.failSessionLocked(managed, err.Error())
			return err
		}
		_, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			messages, messagesErr := service.store.AgentMessages(
				managed.record.TaskID,
				managed.record.ID,
			)
			if messagesErr != nil {
				_ = runtime.Close(context.Background())
				service.failSessionLocked(managed, messagesErr.Error())
				return messagesErr
			}
			if len(messages) > 0 {
				err = errors.New("已登记的 PI Session 文件不存在，无法恢复历史消息")
				_ = runtime.Close(context.Background())
				service.failSessionLocked(managed, err.Error())
				return err
			}
			// PI creates a new Session file lazily after the first assistant
			// response. An unused Session can safely restart with a fresh identity.
			managed.record.ExternalSessionPath = nil
			managed.record.ExternalSessionID = nil
		} else {
			if statErr != nil {
				_ = runtime.Close(context.Background())
				service.failSessionLocked(managed, statErr.Error())
				return statErr
			}
			requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
			err = runtime.Call(requestCtx, "switch_session", map[string]any{"sessionPath": path}, nil)
			cancel()
			if err == nil {
				requestCtx, cancel = context.WithTimeout(ctx, service.requestTimeout)
				err = runtime.Call(requestCtx, "get_state", nil, &state)
				cancel()
			}
			if err != nil {
				_ = runtime.Close(context.Background())
				service.failSessionLocked(managed, sanitizeError(err.Error(), workspace.RootPath))
				return err
			}
		}
	}
	if err := applySessionSettings(ctx, runtime, managed.record, service.requestTimeout); err != nil {
		err = contextualizePICredentialError(err, managed.record.ResourcePolicy)
		_ = runtime.Close(context.Background())
		service.failSessionLocked(managed, sanitizeError(err.Error(), workspace.RootPath))
		return err
	}
	if state.SessionFile != "" {
		path, err := validateSessionPath(options.SessionDir, state.SessionFile, false)
		if err != nil {
			_ = runtime.Close(context.Background())
			service.failSessionLocked(managed, err.Error())
			return err
		}
		managed.record.ExternalSessionPath = &path
	}
	if state.SessionID != "" {
		managed.record.ExternalSessionID = &state.SessionID
	}
	managed.runtime = runtime
	managed.gate = gate
	managed.record.State = "idle"
	managed.record.ErrorMessage = nil
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentSession(managed.record); err != nil {
		_ = runtime.Close(context.Background())
		managed.runtime = nil
		return err
	}
	if err := service.syncEntriesLocked(ctx, managed); err != nil {
		_ = runtime.Close(context.Background())
		managed.runtime = nil
		service.failSessionLocked(managed, sanitizeError(err.Error(), workspace.RootPath))
		return err
	}
	_ = service.emitLocked(managed, "session.state", "", "", map[string]any{
		"state": "idle", "model": stringValue(managed.record.Model),
		"thinkingLevel": stringValue(managed.record.ThinkingLevel),
	})
	go service.consume(managed, runtime, workspace.RootPath)
	return nil
}

func (service *Service) consume(
	managed *managedSession,
	runtime Runtime,
	workspaceRoot string,
) {
	for event := range runtime.Events() {
		managed.mutex.Lock()
		if managed.runtime != runtime || managed.closing {
			managed.mutex.Unlock()
			continue
		}
		service.handleRawEventLocked(managed, event, workspaceRoot)
		managed.mutex.Unlock()
	}
	<-runtime.Done()
	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	if managed.runtime != runtime {
		return
	}
	managed.runtime = nil
	if managed.closing {
		return
	}
	_, _ = service.store.ExpireSessionPermissionGrants(
		managed.record.TaskID,
		managed.record.ID,
		service.timestamp(),
	)
	exit := runtime.Exit()
	message := processExitMessage(exit, workspaceRoot)
	if managed.run != nil {
		service.cancelPendingPermissionsLocked(managed, "PI 进程退出，待处理权限请求已取消")
		service.finishRunLocked(managed, "interrupted", message)
	}
	managed.record.State = "failed"
	managed.record.ErrorMessage = &message
	service.touchSessionLocked(managed)
	_ = service.store.UpsertAgentSession(managed.record)
	_ = service.emitLocked(managed, "error", "", "", errorPayload(exitErrorCode(exit), message, true))
	_ = service.emitLocked(managed, "session.state", "", "", map[string]any{
		"state": "failed", "error": errorPayload(exitErrorCode(exit), message, true),
	})
}

func (service *Service) nextSequenceLocked(managed *managedSession) int64 {
	managed.record.LastSequence++
	return managed.record.LastSequence
}

func (service *Service) touchSessionLocked(managed *managedSession) {
	now := service.timestamp()
	managed.record.UpdatedAt = now
	managed.record.LastActiveAt = now
}

func (service *Service) timestamp() string {
	return service.now().UTC().Format(time.RFC3339Nano)
}

func (service *Service) failSessionLocked(managed *managedSession, message string) {
	managed.record.State = "failed"
	managed.record.ErrorMessage = &message
	service.touchSessionLocked(managed)
	_ = service.store.UpsertAgentSession(managed.record)
	_ = service.emitLocked(managed, "session.state", "", "", map[string]any{
		"state": "failed", "error": errorPayload(errorCode(errors.New(message)), message, true),
	})
}

func (service *Service) failRunRecord(run storage.ExecutionRunRecord, message string) {
	now := service.timestamp()
	run.State = "failed"
	run.FinishedAt = &now
	run.ErrorMessage = &message
	_ = service.store.UpsertExecutionRun(run)
}

func sessionKey(taskID string, sessionID string) string {
	return taskID + "\x00" + sessionID
}

func newID(prefix string) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(value)
}

func validateThinkingLevel(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return nil
	default:
		return errors.New("PI 思考强度不受支持")
	}
}

func applySessionSettings(
	ctx context.Context,
	runtime Runtime,
	record storage.AgentSessionRecord,
	timeout time.Duration,
) error {
	if record.Model != nil && strings.TrimSpace(*record.Model) != "" {
		provider, modelID, found := strings.Cut(strings.TrimSpace(*record.Model), "/")
		if !found || strings.TrimSpace(provider) == "" || strings.TrimSpace(modelID) == "" {
			return errors.New("PI 模型需使用 provider/model 格式")
		}
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		err := runtime.Call(requestCtx, "set_model", map[string]any{
			"provider": provider, "modelId": modelID,
		}, nil)
		cancel()
		if err != nil {
			return err
		}
	}
	if record.ThinkingLevel != nil && strings.TrimSpace(*record.ThinkingLevel) != "" {
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		err := runtime.Call(requestCtx, "set_thinking_level", map[string]any{
			"level": strings.ToLower(strings.TrimSpace(*record.ThinkingLevel)),
		}, nil)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func validateSessionPath(base string, candidate string, mustExist bool) (string, error) {
	base, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	base = filepath.Clean(base)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(base, candidate)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("PI Session 路径不在当前任务目录中")
	}
	if filepath.Dir(candidate) != base {
		return "", errors.New("PI Session 文件必须直接位于当前任务的 Session 目录")
	}
	if filepath.Ext(candidate) != ".jsonl" {
		return "", errors.New("PI Session 路径必须是 JSONL 文件")
	}
	info, statErr := os.Lstat(candidate)
	if statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("PI Session 路径不是普通文件")
		}
	} else if mustExist || !errors.Is(statErr, os.ErrNotExist) {
		return "", errors.New("已登记的 PI Session 文件不存在")
	}
	return candidate, nil
}

func sanitizeError(value string, workspaceRoot string) string {
	value = strings.TrimSpace(value)
	if workspaceRoot != "" {
		value = strings.ReplaceAll(value, workspaceRoot, "<task-workspace>")
	}
	value = strings.ReplaceAll(value, "\x1b", "")
	if len(value) > 600 {
		value = value[:600] + "…"
	}
	if value == "" {
		return "PI 运行失败"
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func contextualizePICredentialError(err error, resourcePolicy string) error {
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no api key") {
		return err
	}
	if resourcePolicy == resourcePolicyExplicitInherit {
		return errors.New("本机 PI 登录配置中没有当前模型凭据；请在终端运行 pi 并使用 /login，完成后恢复或新建会话")
	}
	return errors.New("当前 PI 会话使用隔离配置，未继承本机模型凭据；请在设置 > PI 中选择“使用本机 PI 登录配置”，再新建会话")
}
