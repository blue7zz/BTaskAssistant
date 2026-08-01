export type AgentSessionState =
  | "created"
  | "starting"
  | "idle"
  | "running"
  | "stopping"
  | "interrupted"
  | "failed";

export type AgentMessageStatus =
  | "pending"
  | "streaming"
  | "complete"
  | "error"
  | "cancelled";

export interface AgentSession {
  id: string;
  taskId: string;
  engine: "pi";
  externalSessionPath?: string;
  externalSessionId?: string;
  title: string;
  mode: "ask" | "plan" | "agent";
  model?: string;
  thinkingLevel?: string;
  resourcePolicy: "isolated";
  state: AgentSessionState;
  lastEntryId?: string;
  lastSequence: number;
  createdAt: string;
  updatedAt: string;
  lastActiveAt: string;
  errorMessage?: string;
}

export interface AgentMessage {
  id: string;
  taskId: string;
  sessionId: string;
  runId?: string;
  role: "user" | "assistant" | "system";
  kind: "text";
  status: AgentMessageStatus;
  content?: string;
  contentRef?: string;
  sequence: number;
  piEntryId?: string;
  createdAt: string;
  completedAt?: string;
  references?: AgentReference[];
}

export interface AgentReference {
  taskId: string;
  sessionId: string;
  messageId: string;
  resourceId: string;
  targetType: "resource" | "artifact";
  method: "mention" | "attachment" | "generated";
  position: number;
  createdAt: string;
  kind: string;
  sourceType?: string;
  logicalPath: string;
  mimeType?: string;
  byteSize?: number;
  immutable: boolean;
}

export interface AgentResource {
  id: string;
  taskId: string;
  targetType: "resource" | "artifact";
  kind: string;
  sourceType?: string;
  logicalPath: string;
  mimeType?: string;
  byteSize: number;
  sha256: string;
  immutable: boolean;
  readable: boolean;
  createdAt: string;
  updatedAt?: string;
  proposalState?: "pending" | "accepted" | "rejected";
}

export interface AgentResourcePreview {
  path: string;
  name: string;
  mimeType: string;
  byteSize: number;
  sha256: string;
  kind: "text" | "image";
  content: string;
  truncated: boolean;
}

export interface AgentRun {
  id: string;
  taskId: string;
  sessionId: string;
  gitBindingId?: string;
  baselineCommit?: string;
  mode: string;
  state: string;
  eventsPath: string;
  stdoutPath: string;
  stderrPath: string;
  resultPath: string;
  startedAt: string;
  finishedAt?: string;
  resultSummary?: string;
  errorMessage?: string;
}

export type AgentRiskLevel = "low" | "medium" | "high" | "critical";
export type AgentPermissionScope = "once" | "session" | "task" | "permanent";

export interface AgentToolCall {
  id: string;
  taskId: string;
  sessionId: string;
  runId: string;
  externalToolCallId: string;
  toolName: string;
  capability: string;
  target?: string;
  riskLevel: AgentRiskLevel;
  state: "received" | "waiting_permission" | "running" | "succeeded" | "failed" | "denied" | "cancelled";
  argsJson?: string;
  argsRef?: string;
  outputSummary?: string;
  outputRef?: string;
  isError: boolean;
  startedAt?: string;
  finishedAt?: string;
}

export interface AgentPermissionRequest {
  id: string;
  taskId: string;
  sessionId: string;
  runId: string;
  toolCallId: string;
  toolName: string;
  capability: string;
  target: string;
  normalizedTarget?: string;
  subject: string;
  riskLevel: AgentRiskLevel;
  state: "pending" | "allowed" | "denied" | "expired" | "cancelled";
  requestedAt: string;
  expiresAt?: string;
  resolvedAt?: string;
  resolvedBy?: string;
  decisionScope?: AgentPermissionScope;
  reason?: string;
  allowedScopes: AgentPermissionScope[];
}

export interface AgentPermissionGrant {
  id: string;
  taskId?: string;
  sessionId?: string;
  requestId?: string;
  capability: string;
  targetPattern: string;
  scope: AgentPermissionScope;
  decision: "allow" | "deny";
  riskCeiling: AgentRiskLevel;
  createdAt: string;
  expiresAt?: string;
  consumedAt?: string;
  revokedAt?: string;
  createdBy: string;
}

export interface ResolveAgentPermissionRequest {
  taskId: string;
  sessionId: string;
  requestId: string;
  decision: "allow" | "deny";
  scope: AgentPermissionScope | "";
}

export interface RevokeAgentPermissionGrantRequest {
  taskId: string;
  grantId: string;
}

export interface AgentToolOutputRequest {
  taskId: string;
  toolCallId: string;
}

export interface AgentToolOutput {
  reference: string;
  content: string;
  byteSize: number;
  truncated: boolean;
}

export interface AgentEvent {
  version: 1;
  eventId: string;
  sequence: number;
  kind: string;
  taskId: string;
  sessionId: string;
  runId?: string;
  toolCallId?: string;
  occurredAt: string;
  payload: Record<string, unknown>;
}

export interface CreateAgentSessionRequest {
  taskId: string;
  title: string;
  mode: "ask" | "plan" | "agent";
  model: string;
  thinkingLevel: string;
}

export interface AgentPromptRequest {
  taskId: string;
  sessionId: string;
  message: string;
  resourceIds: string[];
}

export interface AgentResourceSearchRequest {
  taskId: string;
  query: string;
  limit: number;
}

export interface AgentAttachmentUpload {
  name: string;
  mimeType: string;
  dataBase64: string;
}

export interface ImportAgentAttachmentsRequest {
  taskId: string;
  files: AgentAttachmentUpload[];
}

export interface AgentResourcePreviewRequest {
  taskId: string;
  resourceId: string;
}

export interface RemoveAgentReferenceRequest {
  taskId: string;
  sessionId: string;
  messageId: string;
  resourceId: string;
}

export interface AbortAgentRunRequest {
  taskId: string;
  sessionId: string;
  runId: string;
}

export interface StopAgentToolExecutionRequest {
  taskId: string;
  sessionId: string;
  runId: string;
  toolCallId: string;
}

export interface GitBinding {
  id: string;
  taskId: string;
  sourcePath: string;
  sourceRealPath: string;
  commonGitDir: string;
  worktreePath?: string;
  branch?: string;
  baselineCommit: string;
  sourceBranch?: string;
  sourceDirtyAtBind: boolean;
  state:
    | "creating"
    | "ready"
    | "missing"
    | "cleanup_failed"
    | "archived"
    | string;
  createdAt: string;
  updatedAt: string;
  errorMessage?: string;
}

export interface GitChangedFile {
  path: string;
  originalPath?: string;
  status:
    | "modified"
    | "added"
    | "deleted"
    | "renamed"
    | "copied"
    | "untracked"
    | "conflicted"
    | string;
  indexStatus: string;
  worktreeStatus: string;
  staged: boolean;
  unstaged: boolean;
}

export interface TaskGitStatus {
  bound: boolean;
  binding: GitBinding;
  remoteUrl?: string;
  head?: string;
  snapshot?: string;
  files: GitChangedFile[];
  aheadOfBaseline: number;
  behindBaseline: number;
  errorMessage?: string;
}

export interface TaskFileDiff {
  taskId: string;
  path: string;
  status: string;
  staged: string;
  unstaged: string;
  added: number;
  removed: number;
  binary: boolean;
  truncated: boolean;
  byteSize: number;
}

export interface BindGitRepositoryRequest {
  taskId: string;
  sourcePath: string;
}

export interface CommitTaskGitChangesRequest {
  taskId: string;
  message: string;
  expectedSnapshot: string;
  confirmed: boolean;
}

export interface CommitTaskGitChangesResult {
  commit: string;
  status: TaskGitStatus;
}

export interface GitWorktreeActionRequest {
  taskId: string;
  confirmed: boolean;
}
