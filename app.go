package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/credentials"
	"github.com/blue7zz/BTaskAssistant/internal/engine"
	"github.com/blue7zz/BTaskAssistant/internal/plane"
	"github.com/blue7zz/BTaskAssistant/internal/report"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/workflow"
)

// App exposes the deliberately small native boundary used by the React client.
// Product decisions stay in the workflow layer; AI engines stay behind adapters.
type App struct {
	ctx                  context.Context
	store                *storage.SQLiteStore
	credentials          credentials.Store
	dailyReportGenerator engine.DailyReportGenerating
	startupErr           error
}

func NewApp() *App {
	return &App{
		store:                storage.NewSQLiteStore("BTaskAssistant"),
		credentials:          credentials.NewSystemStore("BTaskAssistant"),
		dailyReportGenerator: engine.DailyReportGenerator{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startupErr = a.store.Open()
}

func (a *App) shutdown(_ context.Context) {
	_ = a.store.Close()
}

// LoadState returns the complete persisted Zustand snapshot.
func (a *App) LoadState() (string, error) {
	if a.startupErr != nil {
		return "", a.startupErr
	}
	return a.store.Load()
}

// SaveState atomically persists the complete Zustand snapshot.
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
	settings, err := engine.NormalizePISettings(settings)
	if err != nil {
		return engine.CandidateAnalysis{}, err
	}
	ctx, cancel := context.WithTimeout(
		a.appContext(),
		time.Duration(settings.TimeoutMinutes)*time.Minute+15*time.Second,
	)
	defer cancel()
	return (engine.OMPAnalyzer{Settings: settings}).AnalyzeCandidate(
		ctx,
		sourceMarkdown,
	)
}

// GenerateDailyReport produces a structured candidate from read-only Git facts,
// selected workflow-task summaries, and a free-form user description. The caller
// must preview and explicitly confirm it before writing into the report form.
func (a *App) GenerateDailyReport(
	input engine.DailyReportGenerationInput,
	runtime engine.PISettings,
) (engine.DailyReportGenerationResult, error) {
	runtime, err := engine.NormalizePISettings(runtime)
	if err != nil {
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
	return generator.Generate(ctx, input, runtime)
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
