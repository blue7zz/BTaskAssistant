package agent

import (
	"context"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

const EventName = "agent:event"

type Event struct {
	Version    int    `json:"version"`
	EventID    string `json:"eventId"`
	Sequence   int64  `json:"sequence"`
	Kind       string `json:"kind"`
	TaskID     string `json:"taskId"`
	SessionID  string `json:"sessionId"`
	RunID      string `json:"runId,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	OccurredAt string `json:"occurredAt"`
	Payload    any    `json:"payload"`
}

type Emitter func(Event)

type Store interface {
	EnsureTaskWorkspace(string) (storage.TaskWorkspaceRecord, error)
	TaskWorkspace(string) (storage.TaskWorkspaceRecord, error)
	UpsertAgentSession(storage.AgentSessionRecord) error
	AgentSession(string, string) (storage.AgentSessionRecord, error)
	AgentSessions(string) ([]storage.AgentSessionRecord, error)
	UpsertExecutionRun(storage.ExecutionRunRecord) error
	ExecutionRun(string, string) (storage.ExecutionRunRecord, error)
	ExecutionRuns(string, string) ([]storage.ExecutionRunRecord, error)
	UpsertAgentMessage(storage.AgentMessageRecord) error
	UpsertAgentMessageAndUpdateSession(storage.AgentMessageRecord, storage.AgentSessionRecord) error
	AgentMessages(string, string) ([]storage.AgentMessageRecord, error)
	AppendAgentEvent(storage.AgentEventRecord) error
	AppendAgentEventAndUpdateSession(storage.AgentEventRecord, storage.AgentSessionRecord) error
	UpsertToolCall(storage.ToolCallRecord) error
	InterruptActiveAgentActivity(string, string) error
}

type ServiceOptions struct {
	Executable     string
	RuntimeFactory RuntimeFactory
	StartupTimeout time.Duration
	RequestTimeout time.Duration
	ShutdownGrace  time.Duration
	Now            func() time.Time
	Emit           Emitter
}

type CreateSessionRequest struct {
	TaskID        string `json:"taskId"`
	Title         string `json:"title"`
	Mode          string `json:"mode"`
	Model         string `json:"model"`
	ThinkingLevel string `json:"thinkingLevel"`
}

type PromptRequest struct {
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

type AbortRequest struct {
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

type AgentAPI interface {
	RecoverInterrupted() error
	Sessions(string) ([]storage.AgentSessionRecord, error)
	Messages(string, string) ([]storage.AgentMessageRecord, error)
	Runs(string, string) ([]storage.ExecutionRunRecord, error)
	CreateSession(context.Context, CreateSessionRequest) (storage.AgentSessionRecord, error)
	SendPrompt(context.Context, PromptRequest) (storage.ExecutionRunRecord, error)
	Abort(context.Context, AbortRequest) error
	Close(context.Context) error
}
