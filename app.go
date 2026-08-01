package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/agent"
	"github.com/blue7zz/BTaskAssistant/internal/credentials"
	"github.com/blue7zz/BTaskAssistant/internal/engine"
	"github.com/blue7zz/BTaskAssistant/internal/execution"
	"github.com/blue7zz/BTaskAssistant/internal/gitrepo"
	"github.com/blue7zz/BTaskAssistant/internal/plane"
	"github.com/blue7zz/BTaskAssistant/internal/report"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
	"github.com/blue7zz/BTaskAssistant/internal/workflow"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type directoryDialogOpener func(
	context.Context,
	wailsruntime.OpenDialogOptions,
) (string, error)

type pathOpener func(context.Context, string) error

const dailyReportGenerationProgressEvent = "daily-report:generation-progress"

type dailyReportProgressEmitter func(
	context.Context,
	engine.DailyReportGenerationProgress,
)

// App exposes the deliberately small native boundary used by the React client.
// Product decisions stay in the workflow layer; AI engines stay behind adapters.
type App struct {
	ctx                     context.Context
	store                   *storage.SQLiteStore
	agentService            agent.AgentAPI
	gitService              *gitrepo.Service
	executionService        *execution.Service
	credentials             credentials.Store
	dailyReportGenerator    engine.DailyReportGenerating
	emitDailyReportProgress dailyReportProgressEmitter
	openDirectoryDialog     directoryDialogOpener
	openPath                pathOpener
	startupErr              error
}

func NewApp() *App {
	app := &App{
		store:                storage.NewSQLiteStore("BTaskAssistant"),
		credentials:          credentials.NewSystemStore("BTaskAssistant"),
		dailyReportGenerator: engine.DailyReportGenerator{},
		emitDailyReportProgress: func(
			ctx context.Context,
			progress engine.DailyReportGenerationProgress,
		) {
			wailsruntime.EventsEmit(
				ctx,
				dailyReportGenerationProgressEvent,
				progress,
			)
		},
		openDirectoryDialog: wailsruntime.OpenDirectoryDialog,
		openPath:            openPathInFileManager,
	}
	app.gitService = gitrepo.NewService(app.store)
	app.executionService = execution.NewService()
	app.agentService = agent.NewService(app.store, agent.ServiceOptions{
		GitService:       app.gitService,
		ExecutionService: app.executionService,
		Emit: func(event agent.Event) {
			if app.ctx != nil {
				wailsruntime.EventsEmit(app.ctx, agent.EventName, event)
			}
		},
	})
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startupErr = a.store.Open()
	if a.startupErr == nil {
		_ = a.store.ReconcileTaskContexts()
		if a.agentService != nil {
			a.startupErr = a.agentService.RecoverInterrupted()
		}
	}
}

func (a *App) shutdown(_ context.Context) {
	if a.agentService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.agentService.Close(ctx)
		cancel()
	}
	_ = a.store.Close()
}

// LoadState returns the complete persisted Zustand snapshot.
func (a *App) LoadState() (string, error) {
	if a.startupErr != nil {
		return "", a.startupErr
	}
	return a.store.Load()
}

// SaveState persists the complete Zustand snapshot and materializes its task
// contexts in the configured local directory.
func (a *App) SaveState(payload string) error {
	if a.startupErr != nil {
		return a.startupErr
	}
	return a.store.Save(payload)
}

func (a *App) ClearState() error {
	if a.startupErr != nil {
		return a.startupErr
	}
	return a.store.Clear()
}

// ValidateTransition is the native policy gate. An empty string means the
// requested manual transition is allowed; otherwise the string explains why it
// is blocked.
func (a *App) ValidateTransition(
	from string,
	to string,
	requirementsConfirmed bool,
	developmentCompleted bool,
	reviewApproved bool,
) string {
	err := workflow.ValidateTransition(workflow.TransitionRequest{
		From:                  workflow.Status(from),
		To:                    workflow.Status(to),
		RequirementsConfirmed: requirementsConfirmed,
		DevelopmentCompleted:  developmentCompleted,
		ReviewApproved:        reviewApproved,
	})
	if err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) EngineStatuses() []engine.Status {
	return engine.Statuses()
}

func (a *App) appContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) GetTaskContextRoot() (storage.TaskContextRootInfo, error) {
	if a.startupErr != nil {
		return storage.TaskContextRootInfo{}, a.startupErr
	}
	return a.store.TaskContextRootInfo()
}

func (a *App) EnsureTaskWorkspace(
	taskID string,
) (storage.TaskWorkspaceRecord, error) {
	if a.startupErr != nil {
		return storage.TaskWorkspaceRecord{}, a.startupErr
	}
	return a.store.EnsureTaskWorkspace(taskID)
}

func (a *App) ListTaskWorkspaceFiles(
	taskID string,
	path string,
) ([]taskspace.WorkspaceEntry, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	return a.store.ListTaskWorkspaceFiles(taskID, path)
}

func (a *App) ReadTaskWorkspaceFile(
	taskID string,
	path string,
) (taskspace.FilePreview, error) {
	if a.startupErr != nil {
		return taskspace.FilePreview{}, a.startupErr
	}
	return a.store.ReadTaskWorkspaceFile(taskID, path)
}

func (a *App) ListAgentSessions(
	taskID string,
) ([]storage.AgentSessionRecord, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Sessions(taskID)
}

func (a *App) ListAgentMessages(
	taskID string,
	sessionID string,
) ([]storage.AgentMessageRecord, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Messages(taskID, sessionID)
}

func (a *App) GetAgentHistoryPage(
	request agent.HistoryPageRequest,
) (agent.HistoryPage, error) {
	if a.startupErr != nil {
		return agent.HistoryPage{}, a.startupErr
	}
	if a.agentService == nil {
		return agent.HistoryPage{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.HistoryPage(request)
}

func (a *App) ListExecutionRuns(
	taskID string,
	sessionID string,
) ([]storage.ExecutionRunRecord, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Runs(taskID, sessionID)
}

func (a *App) ListAgentToolCalls(
	taskID string,
	sessionID string,
) ([]storage.ToolCallRecord, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.ToolCalls(taskID, sessionID)
}

func (a *App) ListAgentPermissionRequests(
	taskID string,
	sessionID string,
) ([]agent.PermissionRequest, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.PermissionRequests(taskID, sessionID)
}

func (a *App) ListAgentPermissionGrants(
	taskID string,
	sessionID string,
) ([]storage.PermissionGrantRecord, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.PermissionGrants(taskID, sessionID)
}

func (a *App) ResolveAgentPermission(
	request agent.ResolvePermissionRequest,
) (agent.PermissionRequest, error) {
	if a.startupErr != nil {
		return agent.PermissionRequest{}, a.startupErr
	}
	if a.agentService == nil {
		return agent.PermissionRequest{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.ResolvePermission(a.appContext(), request)
}

func (a *App) RevokeAgentPermissionGrant(
	request agent.RevokePermissionGrantRequest,
) (storage.PermissionGrantRecord, error) {
	if a.startupErr != nil {
		return storage.PermissionGrantRecord{}, a.startupErr
	}
	if a.agentService == nil {
		return storage.PermissionGrantRecord{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.RevokePermissionGrant(request)
}

func (a *App) ReadAgentToolOutput(
	request agent.ToolOutputRequest,
) (agent.ToolOutput, error) {
	if a.startupErr != nil {
		return agent.ToolOutput{}, a.startupErr
	}
	if a.agentService == nil {
		return agent.ToolOutput{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.ReadToolOutput(request)
}

func (a *App) ListAgentResources(
	request agent.ResourceSearchRequest,
) ([]agent.ResourceDescriptor, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Resources(request)
}

func (a *App) ListAgentArtifacts(
	taskID string,
) ([]agent.ResourceDescriptor, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Artifacts(taskID)
}

func (a *App) ImportAgentAttachments(
	request agent.ImportAttachmentsRequest,
) ([]agent.ResourceDescriptor, error) {
	if a.startupErr != nil {
		return nil, a.startupErr
	}
	if a.agentService == nil {
		return nil, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.ImportAttachments(request)
}

func (a *App) PreviewAgentResource(
	request agent.ResourcePreviewRequest,
) (taskspace.FilePreview, error) {
	if a.startupErr != nil {
		return taskspace.FilePreview{}, a.startupErr
	}
	if a.agentService == nil {
		return taskspace.FilePreview{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.PreviewResource(request)
}

func (a *App) RemoveAgentMessageReference(
	request agent.RemoveReferenceRequest,
) error {
	if a.startupErr != nil {
		return a.startupErr
	}
	if a.agentService == nil {
		return errors.New("PI 会话服务未初始化")
	}
	return a.agentService.RemoveReference(request)
}

func (a *App) OpenAgentArtifact(
	taskID string,
	artifactID string,
) error {
	if a.startupErr != nil {
		return a.startupErr
	}
	if a.ctx == nil {
		return errors.New("桌面客户端尚未初始化")
	}
	workspace, err := a.store.EnsureTaskWorkspace(taskID)
	if err != nil {
		return err
	}
	artifact, err := a.store.WorkspaceArtifact(taskID, artifactID)
	if err != nil {
		return err
	}
	if _, err := (taskspace.Service{}).Read(
		filepath.Dir(workspace.RootPath), taskID, artifact.LogicalPath,
	); err != nil {
		return fmt.Errorf("artifact cannot be opened safely: %w", err)
	}
	openPath := a.openPath
	if openPath == nil {
		openPath = openPathInFileManager
	}
	return openPath(a.ctx, filepath.Join(workspace.RootPath, filepath.FromSlash(artifact.LogicalPath)))
}

func (a *App) CreateAgentSession(
	request agent.CreateSessionRequest,
) (storage.AgentSessionRecord, error) {
	if a.startupErr != nil {
		return storage.AgentSessionRecord{}, a.startupErr
	}
	if a.agentService == nil {
		return storage.AgentSessionRecord{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.CreateSession(a.appContext(), request)
}

func (a *App) ResumeAgentSession(
	request agent.SessionRequest,
) (storage.AgentSessionRecord, error) {
	if a.startupErr != nil {
		return storage.AgentSessionRecord{}, a.startupErr
	}
	if a.agentService == nil {
		return storage.AgentSessionRecord{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.ResumeSession(a.appContext(), request)
}

func (a *App) SendAgentPrompt(
	request agent.PromptRequest,
) (storage.ExecutionRunRecord, error) {
	if a.startupErr != nil {
		return storage.ExecutionRunRecord{}, a.startupErr
	}
	if a.agentService == nil {
		return storage.ExecutionRunRecord{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.SendPrompt(a.appContext(), request)
}

func (a *App) SteerAgent(
	request agent.PromptRequest,
) (storage.AgentMessageRecord, error) {
	if a.startupErr != nil {
		return storage.AgentMessageRecord{}, a.startupErr
	}
	if a.agentService == nil {
		return storage.AgentMessageRecord{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Steer(a.appContext(), request)
}

func (a *App) FollowUpAgent(
	request agent.PromptRequest,
) (storage.AgentMessageRecord, error) {
	if a.startupErr != nil {
		return storage.AgentMessageRecord{}, a.startupErr
	}
	if a.agentService == nil {
		return storage.AgentMessageRecord{}, errors.New("PI 会话服务未初始化")
	}
	return a.agentService.FollowUp(a.appContext(), request)
}

func (a *App) AbortAgentRun(request agent.AbortRequest) error {
	if a.startupErr != nil {
		return a.startupErr
	}
	if a.agentService == nil {
		return errors.New("PI 会话服务未初始化")
	}
	return a.agentService.Abort(a.appContext(), request)
}

func (a *App) StopAgentToolExecution(request execution.StopRequest) error {
	if a.startupErr != nil {
		return a.startupErr
	}
	if a.agentService == nil {
		return errors.New("PI 会话服务未初始化")
	}
	return a.agentService.StopToolExecution(request)
}

func (a *App) SelectGitRepository() (string, error) {
	if a.ctx == nil {
		return "", errors.New("桌面客户端尚未初始化")
	}
	openDirectoryDialog := a.openDirectoryDialog
	if openDirectoryDialog == nil {
		openDirectoryDialog = wailsruntime.OpenDirectoryDialog
	}
	selected, err := openDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:           "选择任务的本地 Git 仓库",
		ResolvesAliases: true,
	})
	if err != nil {
		return "", fmt.Errorf("选择本地 Git 仓库目录失败: %w", err)
	}
	if strings.TrimSpace(selected) == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("解析所选 Git 仓库目录失败: %w", err)
	}
	return filepath.Clean(absolute), nil
}

func (a *App) BindGitRepository(
	request gitrepo.BindRequest,
) (storage.GitBindingRecord, error) {
	if a.startupErr != nil {
		return storage.GitBindingRecord{}, a.startupErr
	}
	if a.gitService == nil {
		return storage.GitBindingRecord{}, errors.New("Git worktree 服务未初始化")
	}
	return a.gitService.Bind(a.appContext(), request)
}

func (a *App) GetTaskGitStatus(taskID string) (gitrepo.StatusView, error) {
	if a.startupErr != nil {
		return gitrepo.StatusView{}, a.startupErr
	}
	if a.gitService == nil {
		return gitrepo.StatusView{}, errors.New("Git worktree 服务未初始化")
	}
	return a.gitService.Status(a.appContext(), taskID)
}

func (a *App) GetTaskFileDiff(taskID string, path string) (gitrepo.FileDiffView, error) {
	if a.startupErr != nil {
		return gitrepo.FileDiffView{}, a.startupErr
	}
	if a.gitService == nil {
		return gitrepo.FileDiffView{}, errors.New("Git worktree 服务未初始化")
	}
	return a.gitService.Diff(a.appContext(), taskID, path)
}

func (a *App) CommitTaskGitChanges(
	request gitrepo.CommitRequest,
) (gitrepo.CommitResult, error) {
	if a.startupErr != nil {
		return gitrepo.CommitResult{}, a.startupErr
	}
	if a.gitService == nil {
		return gitrepo.CommitResult{}, errors.New("Git worktree 服务未初始化")
	}
	return a.gitService.Commit(a.appContext(), request)
}

func (a *App) CleanupTaskGitWorktree(
	request gitrepo.CleanupRequest,
) (storage.GitBindingRecord, error) {
	if a.startupErr != nil {
		return storage.GitBindingRecord{}, a.startupErr
	}
	if a.gitService == nil {
		return storage.GitBindingRecord{}, errors.New("Git worktree 服务未初始化")
	}
	if (a.agentService != nil && a.agentService.ActiveTask(request.TaskID)) ||
		(a.executionService != nil && a.executionService.ActiveTask(request.TaskID)) {
		return storage.GitBindingRecord{}, errors.New("当前任务仍有 PI 或 Shell 在运行，拒绝清理 worktree")
	}
	return a.gitService.Cleanup(a.appContext(), request)
}

func (a *App) RecoverTaskGitWorktree(
	request gitrepo.RecoverRequest,
) (storage.GitBindingRecord, error) {
	if a.startupErr != nil {
		return storage.GitBindingRecord{}, a.startupErr
	}
	if a.gitService == nil {
		return storage.GitBindingRecord{}, errors.New("Git worktree 服务未初始化")
	}
	return a.gitService.Recover(a.appContext(), request)
}

func (a *App) SelectTaskContextRoot() (string, error) {
	if a.ctx == nil {
		return "", errors.New("桌面客户端尚未初始化")
	}

	root, err := a.GetTaskContextRoot()
	if err != nil {
		return "", err
	}
	options := wailsruntime.OpenDialogOptions{
		Title:                "选择任务资料目录",
		CanCreateDirectories: true,
		ResolvesAliases:      true,
	}
	if root.Available {
		options.DefaultDirectory = root.Path
	}

	openDirectoryDialog := a.openDirectoryDialog
	if openDirectoryDialog == nil {
		openDirectoryDialog = wailsruntime.OpenDirectoryDialog
	}
	selected, err := openDirectoryDialog(a.ctx, options)
	if err != nil {
		return "", fmt.Errorf("选择任务资料目录失败: %w", err)
	}
	if selected == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("解析所选任务资料目录失败: %w", err)
	}
	return filepath.Clean(absolute), nil
}

func (a *App) SetTaskContextRoot(
	path string,
) (storage.TaskContextRootInfo, error) {
	if a.startupErr != nil {
		return storage.TaskContextRootInfo{}, a.startupErr
	}
	root, err := a.store.SetTaskContextRoot(path)
	if err != nil {
		return storage.TaskContextRootInfo{}, fmt.Errorf(
			"设置任务资料目录失败: %w",
			err,
		)
	}
	return root, nil
}

func (a *App) OpenTaskContextRoot() error {
	if a.ctx == nil {
		return errors.New("桌面客户端尚未初始化")
	}
	root, err := a.GetTaskContextRoot()
	if err != nil {
		return err
	}
	if !root.Available {
		return errors.New("任务资料目录当前不可用")
	}

	openPath := a.openPath
	if openPath == nil {
		openPath = openPathInFileManager
	}
	if err := openPath(a.ctx, root.Path); err != nil {
		return fmt.Errorf("打开任务资料目录失败: %w", err)
	}
	return nil
}

func openPathInFileManager(ctx context.Context, path string) error {
	var command *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", path)
	case "windows":
		command = exec.CommandContext(ctx, "explorer.exe", path)
	default:
		command = exec.CommandContext(ctx, "xdg-open", path)
	}
	return command.Run()
}

func (a *App) SelectDailyReportProjectDirectory() (string, error) {
	if a.ctx == nil {
		return "", errors.New("桌面客户端尚未初始化")
	}

	openDirectoryDialog := a.openDirectoryDialog
	if openDirectoryDialog == nil {
		openDirectoryDialog = wailsruntime.OpenDirectoryDialog
	}
	selected, err := openDirectoryDialog(
		a.ctx,
		wailsruntime.OpenDialogOptions{Title: "选择本地 Git 仓库"},
	)
	if err != nil {
		return "", fmt.Errorf("选择本地 Git 仓库目录失败: %w", err)
	}
	if selected == "" {
		return "", nil
	}

	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("解析所选目录失败: %w", err)
	}
	return filepath.Clean(absolute), nil
}

func dailyReportCredentialAccount(
	apiURL string,
	employeeID string,
) (string, error) {
	normalized, err := report.NormalizeAPIURL(apiURL)
	if err != nil {
		return "", err
	}
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return "", errors.New("请输入工号")
	}
	sum := sha256.Sum256([]byte(normalized + "\x00" + employeeID))
	return fmt.Sprintf("daily-report-token-%x", sum[:16]), nil
}

func (a *App) readDailyReportToken(
	apiURL string,
	employeeID string,
) (string, error) {
	account, err := dailyReportCredentialAccount(apiURL, employeeID)
	if err != nil {
		return "", err
	}
	token, err := a.credentials.Get(account)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", credentials.ErrNotFound
	}
	return token, nil
}

// SaveDailyReportToken keeps the employee-bound token in the platform
// credential store. It is never included in the persisted workspace snapshot.
func (a *App) SaveDailyReportToken(
	apiURL string,
	employeeID string,
	token string,
) error {
	account, err := dailyReportCredentialAccount(apiURL, employeeID)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("请输入日报 Token")
	}
	if err := a.credentials.Set(account, token); err != nil {
		return fmt.Errorf("保存到系统凭据库失败: %w", err)
	}
	return nil
}

func (a *App) DeleteDailyReportToken(apiURL string, employeeID string) error {
	account, err := dailyReportCredentialAccount(apiURL, employeeID)
	if err != nil {
		return err
	}
	if err := a.credentials.Delete(account); err != nil {
		return fmt.Errorf("从系统凭据库删除日报 Token 失败: %w", err)
	}
	return nil
}

func (a *App) HasDailyReportToken(
	apiURL string,
	employeeID string,
) (bool, error) {
	token, err := a.readDailyReportToken(apiURL, employeeID)
	if errors.Is(err, credentials.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取系统凭据库失败: %w", err)
	}
	return strings.TrimSpace(token) != "", nil
}

func (a *App) SubmitDailyReport(
	apiURL string,
	employeeID string,
	reportDate string,
	content string,
) (report.SubmitResult, error) {
	token, err := a.readDailyReportToken(apiURL, employeeID)
	if errors.Is(err, credentials.ErrNotFound) {
		return report.SubmitResult{}, errors.New("该工号的日报 Token 尚未保存")
	}
	if err != nil {
		return report.SubmitResult{}, fmt.Errorf("读取系统凭据库失败: %w", err)
	}
	client, err := report.NewClient(apiURL, token)
	if err != nil {
		return report.SubmitResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 35*time.Second)
	defer cancel()
	response, err := client.SubmitDaily(ctx, report.DailyReport{
		EmployeeID: employeeID,
		ReportDate: reportDate,
		Content:    content,
	})
	if err != nil {
		return report.SubmitResult{}, err
	}
	if response.Code != 0 {
		message := strings.TrimSpace(response.Msg)
		if message == "" {
			message = "未知错误"
		}
		return report.SubmitResult{}, fmt.Errorf(
			"日报提交失败（code %d）: %s",
			response.Code,
			message,
		)
	}
	return report.SubmitResult{
		ID:      response.Data.ID,
		Action:  response.Data.Action,
		Message: response.Msg,
	}, nil
}

func planeCredentialAccount(baseURL string) (string, error) {
	normalized, err := plane.NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("plane-pat-%x", sum[:16]), nil
}

func legacyPlaneCredentialAccount(
	baseURL string,
	workspaceSlug string,
) (string, error) {
	normalized, err := plane.NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	workspaceSlug = strings.TrimSpace(workspaceSlug)
	if workspaceSlug == "" {
		return "", errors.New("请输入 Plane workspace slug")
	}
	sum := sha256.Sum256([]byte(normalized + "\x00" + workspaceSlug))
	return fmt.Sprintf("plane-pat-%x", sum[:16]), nil
}

func (a *App) readPlaneToken(
	baseURL string,
	workspaceSlug string,
) (string, error) {
	account, err := planeCredentialAccount(baseURL)
	if err != nil {
		return "", err
	}
	token, err := a.credentials.Get(account)
	if err == nil {
		return strings.TrimSpace(token), nil
	}
	if !errors.Is(err, credentials.ErrNotFound) {
		return "", fmt.Errorf("读取系统凭据库失败: %w", err)
	}

	// v0.2.1 keyed PATs by both instance and workspace. Keep reading that
	// account and copy it forward so upgrades do not require re-entering the
	// secret.
	legacyAccount, legacyErr := legacyPlaneCredentialAccount(
		baseURL,
		workspaceSlug,
	)
	if legacyErr != nil {
		return "", credentials.ErrNotFound
	}
	token, err = a.credentials.Get(legacyAccount)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token != "" {
		_ = a.credentials.Set(account, token)
	}
	return token, nil
}

// SavePlaneToken writes the PAT directly to the platform credential store. It
// is never included in the persisted Zustand/SQLite workspace snapshot.
func (a *App) SavePlaneToken(
	baseURL string,
	_ string,
	token string,
) error {
	account, err := planeCredentialAccount(baseURL)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("请输入 Plane Personal Access Token")
	}
	if err := a.credentials.Set(account, token); err != nil {
		return fmt.Errorf("保存到系统凭据库失败: %w", err)
	}
	return nil
}

func (a *App) DeletePlaneToken(baseURL string, workspaceSlug string) error {
	account, err := planeCredentialAccount(baseURL)
	if err != nil {
		return err
	}
	if err := a.credentials.Delete(account); err != nil {
		return fmt.Errorf("从系统凭据库删除令牌失败: %w", err)
	}
	if legacyAccount, legacyErr := legacyPlaneCredentialAccount(
		baseURL,
		workspaceSlug,
	); legacyErr == nil {
		if err := a.credentials.Delete(legacyAccount); err != nil {
			return fmt.Errorf("从系统凭据库删除旧令牌失败: %w", err)
		}
	}
	return nil
}

func (a *App) HasPlaneToken(
	baseURL string,
	workspaceSlug string,
) (bool, error) {
	token, err := a.readPlaneToken(baseURL, workspaceSlug)
	if errors.Is(err, credentials.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取系统凭据库失败: %w", err)
	}
	return strings.TrimSpace(token) != "", nil
}

func (a *App) planeClient(
	baseURL string,
	workspaceSlug string,
) (*plane.Client, error) {
	token, err := a.readPlaneToken(baseURL, workspaceSlug)
	if errors.Is(err, credentials.ErrNotFound) {
		return nil, errors.New("Plane PAT 尚未保存")
	}
	if err != nil {
		return nil, fmt.Errorf("读取系统凭据库失败: %w", err)
	}
	return plane.NewClient(baseURL, token)
}

// SetupPlaneConnection is the simplified first-run path. The user supplies
// only a Plane workspace URL and PAT; the backend validates both, stores the
// secret only after authentication succeeds, and returns selectable projects.
func (a *App) SetupPlaneConnection(
	serviceAddress string,
	token string,
) (plane.ConnectionSetup, error) {
	setup, err := plane.ResolveWorkspaceURL(serviceAddress)
	if err != nil {
		return plane.ConnectionSetup{}, err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		token, err = a.readPlaneToken(setup.BaseURL, setup.WorkspaceSlug)
		if errors.Is(err, credentials.ErrNotFound) {
			return plane.ConnectionSetup{}, errors.New(
				"请输入 Plane Personal Access Token",
			)
		}
		if err != nil {
			return plane.ConnectionSetup{}, err
		}
	}
	client, err := plane.NewClient(setup.BaseURL, token)
	if err != nil {
		return plane.ConnectionSetup{}, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 35*time.Second)
	defer cancel()
	setup.Projects, err = client.ListProjects(ctx, setup.WorkspaceSlug)
	if err != nil {
		return plane.ConnectionSetup{}, err
	}
	if len(setup.Projects) == 0 {
		return plane.ConnectionSetup{}, errors.New(
			"连接成功，但这个工作区中没有可访问的项目",
		)
	}

	account, err := planeCredentialAccount(setup.BaseURL)
	if err != nil {
		return plane.ConnectionSetup{}, err
	}
	if err := a.credentials.Set(account, token); err != nil {
		return plane.ConnectionSetup{}, fmt.Errorf(
			"连接成功，但保存到系统凭据库失败: %w",
			err,
		)
	}
	return setup, nil
}

func (a *App) TestPlaneConnection(
	baseURL string,
	workspaceSlug string,
	projectID string,
) (plane.ConnectionStatus, error) {
	client, err := a.planeClient(baseURL, workspaceSlug)
	if err != nil {
		return plane.ConnectionStatus{}, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 35*time.Second)
	defer cancel()
	return client.Test(ctx, workspaceSlug, projectID)
}

func (a *App) ListPlaneProjects(
	baseURL string,
	workspaceSlug string,
) ([]plane.Project, error) {
	client, err := a.planeClient(baseURL, workspaceSlug)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 35*time.Second)
	defer cancel()
	return client.ListProjects(ctx, workspaceSlug)
}

func (a *App) CollectPlaneWorkItems(
	baseURL string,
	workspaceSlug string,
	projectID string,
	projectIdentifier string,
) ([]plane.Candidate, error) {
	client, err := a.planeClient(baseURL, workspaceSlug)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 2*time.Minute)
	defer cancel()
	return client.ListCandidates(
		ctx,
		workspaceSlug,
		projectID,
		projectIdentifier,
	)
}

func (a *App) LoadPlaneWorkItemDetails(
	baseURL string,
	workspaceSlug string,
	projectID string,
	projectIdentifier string,
	workItemID string,
) (plane.Candidate, error) {
	client, err := a.planeClient(baseURL, workspaceSlug)
	if err != nil {
		return plane.Candidate{}, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 2*time.Minute)
	defer cancel()
	return client.LoadCandidateDetails(
		ctx,
		workspaceSlug,
		projectID,
		projectIdentifier,
		workItemID,
	)
}

func (a *App) AnalyzePlaneCandidate(
	sourceMarkdown string,
	settings engine.PISettings,
) (engine.CandidateAnalysis, error) {
	if strings.TrimSpace(sourceMarkdown) == "" {
		return engine.CandidateAnalysis{}, errors.New("候选来源不能为空")
	}
	normalized, err := engine.NormalizePISettings(settings)
	if err != nil {
		return engine.CandidateAnalysis{}, err
	}
	settings = normalized
	ctx, cancel := context.WithTimeout(
		a.appContext(),
		time.Duration(settings.TimeoutMinutes)*time.Minute+15*time.Second,
	)
	defer cancel()
	return engine.AnalyzePlaneCandidateWithPI(ctx, sourceMarkdown, settings)
}

// GenerateDailyReport produces a structured candidate from read-only Git facts,
// selected workflow-task summaries, and a free-form user description. The caller
// must preview and explicitly confirm it before writing into the report form.
func (a *App) GenerateDailyReport(
	input engine.DailyReportGenerationInput,
	runtime engine.PISettings,
) (engine.DailyReportGenerationResult, error) {
	requestID := strings.TrimSpace(input.RequestID)
	progressEnabled := requestID != "" &&
		len(requestID) <= 120 &&
		!strings.ContainsAny(requestID, "\x00\r\n")
	reportProgress := func(progress engine.DailyReportGenerationProgress) {
		if !progressEnabled ||
			progress.RequestID != requestID ||
			a.ctx == nil ||
			a.emitDailyReportProgress == nil {
			return
		}
		a.emitDailyReportProgress(a.ctx, progress)
	}
	emitFailure := func() {
		reportProgress(engine.DailyReportGenerationProgress{
			RequestID: requestID,
			Stage:     "failed",
			Message:   "生成失败，已停止本次处理",
		})
	}
	runtime, err := engine.NormalizePISettings(runtime)
	if err != nil {
		emitFailure()
		return engine.DailyReportGenerationResult{}, err
	}
	ctx, cancel := context.WithTimeout(
		a.appContext(),
		time.Duration(runtime.TimeoutMinutes)*time.Minute+15*time.Second,
	)
	defer cancel()
	generator := a.dailyReportGenerator
	if generator == nil {
		generator = engine.DailyReportGenerator{}
	}
	result, err := generator.Generate(ctx, input, runtime, reportProgress)
	if err != nil {
		emitFailure()
		return engine.DailyReportGenerationResult{}, err
	}
	return result, nil
}

// AnalyzeRequirements runs a single structured requirement-interview round.
// The selected CLI is constrained to read-only project access; the result is
// still only a candidate and cannot advance or approve the task.
func (a *App) AnalyzeRequirements(
	input engine.RequirementAnalysisInput,
	settings engine.PISettings,
) (engine.RequirementAnalysisResult, error) {
	settings, err := engine.NormalizePISettings(settings)
	if err != nil {
		return engine.RequirementAnalysisResult{}, err
	}
	ctx, cancel := context.WithTimeout(
		a.appContext(),
		time.Duration(settings.TimeoutMinutes)*time.Minute+15*time.Second,
	)
	defer cancel()
	return (engine.RequirementAnalyzer{}).AnalyzeWithSettings(
		ctx,
		input,
		settings,
	)
}
