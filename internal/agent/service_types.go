package agent

import (
	"context"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
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
	UpsertAgentMessageWithReferences(storage.AgentMessageRecord, storage.AgentSessionRecord, []storage.AgentReferenceRecord) error
	AddAgentMessageReference(storage.AgentReferenceRecord) error
	AgentMessageReferences(string, string, string) ([]storage.AgentReferenceRecord, error)
	RemoveAgentMessageReference(string, string, string, string) error
	AppendAgentEvent(storage.AgentEventRecord) error
	AppendAgentEventAndUpdateSession(storage.AgentEventRecord, storage.AgentSessionRecord) error
	UpsertToolCall(storage.ToolCallRecord) error
	ToolCall(string, string) (storage.ToolCallRecord, error)
	ToolCalls(string, string) ([]storage.ToolCallRecord, error)
	UpsertPermissionRequest(storage.PermissionRequestRecord) error
	PermissionRequest(string, string) (storage.PermissionRequestRecord, error)
	PermissionRequests(string, string) ([]storage.PermissionRequestRecord, error)
	ActivePermissionGrants(string, string, string) ([]storage.PermissionGrantRecord, error)
	ResolvePermissionRequest(storage.PermissionResolution, *storage.PermissionGrantRecord) (storage.PermissionRequestRecord, error)
	ConsumePermissionGrant(string, string, string, string) error
	RevokePermissionGrant(string, string, string) (storage.PermissionGrantRecord, error)
	ExpirePendingPermissionRequests(string, string) (int64, error)
	ExpireSessionPermissionGrants(string, string, string) (int64, error)
	TaskResources(string) ([]storage.TaskResourceRecord, error)
	TaskResource(string, string) (storage.TaskResourceRecord, error)
	WorkspaceArtifacts(string) ([]storage.WorkspaceArtifactRecord, error)
	WorkspaceArtifact(string, string) (storage.WorkspaceArtifactRecord, error)
	WorkspaceArtifactByPath(string, string) (storage.WorkspaceArtifactRecord, error)
	UpsertWorkspaceArtifact(storage.WorkspaceArtifactRecord) error
	TaskRevision(string) (int, error)
	TaskStatus(string) (string, error)
	UpsertRequirementProposal(storage.RequirementProposalRecord) error
	RequirementProposals(string) ([]storage.RequirementProposalRecord, error)
	InterruptActiveAgentActivity(string, string) error
}

type ServiceOptions struct {
	Executable        string
	RuntimeFactory    RuntimeFactory
	StartupTimeout    time.Duration
	RequestTimeout    time.Duration
	ShutdownGrace     time.Duration
	PermissionTimeout time.Duration
	Now               func() time.Time
	Emit              Emitter
}

type CreateSessionRequest struct {
	TaskID        string `json:"taskId"`
	Title         string `json:"title"`
	Mode          string `json:"mode"`
	Model         string `json:"model"`
	ThinkingLevel string `json:"thinkingLevel"`
}

type PromptRequest struct {
	TaskID      string   `json:"taskId"`
	SessionID   string   `json:"sessionId"`
	Message     string   `json:"message"`
	ResourceIDs []string `json:"resourceIds"`
}

type AttachmentUpload struct {
	Name       string `json:"name"`
	MIMEType   string `json:"mimeType"`
	DataBase64 string `json:"dataBase64"`
}

type ImportAttachmentsRequest struct {
	TaskID string             `json:"taskId"`
	Files  []AttachmentUpload `json:"files"`
}

type ResourceDescriptor struct {
	ID            string  `json:"id"`
	TaskID        string  `json:"taskId"`
	TargetType    string  `json:"targetType"`
	Kind          string  `json:"kind"`
	SourceType    string  `json:"sourceType,omitempty"`
	LogicalPath   string  `json:"logicalPath"`
	MIMEType      string  `json:"mimeType,omitempty"`
	ByteSize      int64   `json:"byteSize"`
	SHA256        string  `json:"sha256"`
	Immutable     bool    `json:"immutable"`
	Readable      bool    `json:"readable"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt,omitempty"`
	ProposalState *string `json:"proposalState,omitempty"`
}

type ResourceSearchRequest struct {
	TaskID string `json:"taskId"`
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
}

type ResourcePreviewRequest struct {
	TaskID     string `json:"taskId"`
	ResourceID string `json:"resourceId"`
}

type RemoveReferenceRequest struct {
	TaskID     string `json:"taskId"`
	SessionID  string `json:"sessionId"`
	MessageID  string `json:"messageId"`
	ResourceID string `json:"resourceId"`
}

type AbortRequest struct {
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

type PermissionRequest struct {
	ID               string   `json:"id"`
	TaskID           string   `json:"taskId"`
	SessionID        string   `json:"sessionId"`
	RunID            string   `json:"runId"`
	ToolCallID       string   `json:"toolCallId"`
	ToolName         string   `json:"toolName"`
	Capability       string   `json:"capability"`
	Target           string   `json:"target"`
	NormalizedTarget string   `json:"normalizedTarget,omitempty"`
	Subject          string   `json:"subject"`
	RiskLevel        string   `json:"riskLevel"`
	State            string   `json:"state"`
	RequestedAt      string   `json:"requestedAt"`
	ExpiresAt        string   `json:"expiresAt,omitempty"`
	ResolvedAt       string   `json:"resolvedAt,omitempty"`
	ResolvedBy       string   `json:"resolvedBy,omitempty"`
	DecisionScope    string   `json:"decisionScope,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	AllowedScopes    []string `json:"allowedScopes"`
}

type ResolvePermissionRequest struct {
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"`
	Scope     string `json:"scope"`
}

type RevokePermissionGrantRequest struct {
	TaskID  string `json:"taskId"`
	GrantID string `json:"grantId"`
}

type ToolOutputRequest struct {
	TaskID     string `json:"taskId"`
	ToolCallID string `json:"toolCallId"`
}

type ToolOutput struct {
	Reference string `json:"reference"`
	Content   string `json:"content"`
	ByteSize  int64  `json:"byteSize"`
	Truncated bool   `json:"truncated"`
}

type AgentAPI interface {
	RecoverInterrupted() error
	Sessions(string) ([]storage.AgentSessionRecord, error)
	Messages(string, string) ([]storage.AgentMessageRecord, error)
	Runs(string, string) ([]storage.ExecutionRunRecord, error)
	ToolCalls(string, string) ([]storage.ToolCallRecord, error)
	PermissionRequests(string, string) ([]PermissionRequest, error)
	PermissionGrants(string, string) ([]storage.PermissionGrantRecord, error)
	ResolvePermission(context.Context, ResolvePermissionRequest) (PermissionRequest, error)
	RevokePermissionGrant(RevokePermissionGrantRequest) (storage.PermissionGrantRecord, error)
	ReadToolOutput(ToolOutputRequest) (ToolOutput, error)
	Resources(ResourceSearchRequest) ([]ResourceDescriptor, error)
	Artifacts(string) ([]ResourceDescriptor, error)
	ImportAttachments(ImportAttachmentsRequest) ([]ResourceDescriptor, error)
	PreviewResource(ResourcePreviewRequest) (taskspace.FilePreview, error)
	RemoveReference(RemoveReferenceRequest) error
	CreateSession(context.Context, CreateSessionRequest) (storage.AgentSessionRecord, error)
	SendPrompt(context.Context, PromptRequest) (storage.ExecutionRunRecord, error)
	Abort(context.Context, AbortRequest) error
	Close(context.Context) error
}
