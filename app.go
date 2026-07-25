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
	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/workflow"
)

// App exposes the deliberately small native boundary used by the React client.
// Product decisions stay in the workflow layer; AI engines stay behind adapters.
type App struct {
	ctx         context.Context
	store       *storage.SQLiteStore
	credentials credentials.Store
	startupErr  error
}

func NewApp() *App {
	return &App{
		store:       storage.NewSQLiteStore("BTaskAssistant"),
		credentials: credentials.NewSystemStore("BTaskAssistant"),
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

func planeCredentialAccount(baseURL string, workspaceSlug string) (string, error) {
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

// SavePlaneToken writes the PAT directly to the platform credential store. It
// is never included in the persisted Zustand/SQLite workspace snapshot.
func (a *App) SavePlaneToken(
	baseURL string,
	workspaceSlug string,
	token string,
) error {
	account, err := planeCredentialAccount(baseURL, workspaceSlug)
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
	account, err := planeCredentialAccount(baseURL, workspaceSlug)
	if err != nil {
		return err
	}
	if err := a.credentials.Delete(account); err != nil {
		return fmt.Errorf("从系统凭据库删除令牌失败: %w", err)
	}
	return nil
}

func (a *App) HasPlaneToken(
	baseURL string,
	workspaceSlug string,
) (bool, error) {
	account, err := planeCredentialAccount(baseURL, workspaceSlug)
	if err != nil {
		return false, err
	}
	token, err := a.credentials.Get(account)
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
	account, err := planeCredentialAccount(baseURL, workspaceSlug)
	if err != nil {
		return nil, err
	}
	token, err := a.credentials.Get(account)
	if errors.Is(err, credentials.ErrNotFound) {
		return nil, errors.New("Plane PAT 尚未保存")
	}
	if err != nil {
		return nil, fmt.Errorf("读取系统凭据库失败: %w", err)
	}
	return plane.NewClient(baseURL, token)
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

func (a *App) CollectPlaneWorkItems(
	baseURL string,
	workspaceSlug string,
	projectID string,
) ([]plane.Candidate, error) {
	client, err := a.planeClient(baseURL, workspaceSlug)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 2*time.Minute)
	defer cancel()
	return client.ListCandidates(ctx, workspaceSlug, projectID)
}

func (a *App) AnalyzePlaneCandidate(
	sourceMarkdown string,
) (engine.CandidateAnalysis, error) {
	ctx, cancel := context.WithTimeout(a.appContext(), 2*time.Minute)
	defer cancel()
	return (engine.OMPAnalyzer{}).AnalyzeCandidate(ctx, sourceMarkdown)
}
