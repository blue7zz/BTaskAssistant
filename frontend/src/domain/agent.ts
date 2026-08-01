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
