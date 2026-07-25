package main

import (
	"context"

	"github.com/blue7zz/BTaskAssistant/internal/engine"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/workflow"
)

// App exposes the deliberately small native boundary used by the React client.
// Product decisions stay in the workflow layer; AI engines stay behind adapters.
type App struct {
	ctx        context.Context
	store      *storage.SQLiteStore
	startupErr error
}

func NewApp() *App {
	return &App{store: storage.NewSQLiteStore("BTaskAssistant")}
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
