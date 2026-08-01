import type {
  AbortAgentRunRequest,
  AgentEvent,
  AgentMessage,
  AgentPermissionGrant,
  AgentPermissionRequest,
  AgentPromptRequest,
  AgentResource,
  AgentResourcePreview,
  AgentResourcePreviewRequest,
  AgentResourceSearchRequest,
  AgentRun,
  AgentSession,
  AgentToolCall,
  AgentToolOutput,
  AgentToolOutputRequest,
  BindGitRepositoryRequest,
  CommitTaskGitChangesRequest,
  CommitTaskGitChangesResult,
  CreateAgentSessionRequest,
  GitBinding,
  GitWorktreeActionRequest,
  ImportAgentAttachmentsRequest,
  ResolveAgentPermissionRequest,
  RemoveAgentReferenceRequest,
  RevokeAgentPermissionGrantRequest,
  StopAgentToolExecutionRequest,
  TaskFileDiff,
  TaskGitStatus,
} from "../domain/agent";

const AGENT_EVENT_NAME = "agent:event";

interface NativeAgentApp {
  ListAgentSessions(taskId: string): Promise<AgentSession[]>;
  ListAgentMessages(
    taskId: string,
    sessionId: string,
  ): Promise<AgentMessage[]>;
  ListExecutionRuns(taskId: string, sessionId: string): Promise<AgentRun[]>;
  ListAgentToolCalls(taskId: string, sessionId: string): Promise<AgentToolCall[]>;
  ListAgentPermissionRequests(
    taskId: string,
    sessionId: string,
  ): Promise<AgentPermissionRequest[]>;
  ListAgentPermissionGrants(
    taskId: string,
    sessionId: string,
  ): Promise<AgentPermissionGrant[]>;
  ResolveAgentPermission(
    request: ResolveAgentPermissionRequest,
  ): Promise<AgentPermissionRequest>;
  RevokeAgentPermissionGrant(
    request: RevokeAgentPermissionGrantRequest,
  ): Promise<AgentPermissionGrant>;
  ReadAgentToolOutput(request: AgentToolOutputRequest): Promise<AgentToolOutput>;
  CreateAgentSession(
    request: CreateAgentSessionRequest,
  ): Promise<AgentSession>;
  SendAgentPrompt(request: AgentPromptRequest): Promise<AgentRun>;
  AbortAgentRun(request: AbortAgentRunRequest): Promise<void>;
  StopAgentToolExecution?(request: StopAgentToolExecutionRequest): Promise<void>;
  SelectGitRepository?(): Promise<string>;
  BindGitRepository?(request: BindGitRepositoryRequest): Promise<GitBinding>;
  GetTaskGitStatus?(taskId: string): Promise<TaskGitStatus>;
  GetTaskFileDiff?(taskId: string, path: string): Promise<TaskFileDiff>;
  CommitTaskGitChanges?(
    request: CommitTaskGitChangesRequest,
  ): Promise<CommitTaskGitChangesResult>;
  CleanupTaskGitWorktree?(request: GitWorktreeActionRequest): Promise<GitBinding>;
  RecoverTaskGitWorktree?(request: GitWorktreeActionRequest): Promise<GitBinding>;
  ListAgentResources(request: AgentResourceSearchRequest): Promise<AgentResource[]>;
  ListAgentArtifacts(taskId: string): Promise<AgentResource[]>;
  ImportAgentAttachments(request: ImportAgentAttachmentsRequest): Promise<AgentResource[]>;
  PreviewAgentResource(request: AgentResourcePreviewRequest): Promise<AgentResourcePreview>;
  RemoveAgentMessageReference(request: RemoveAgentReferenceRequest): Promise<void>;
  OpenAgentArtifact(taskId: string, artifactId: string): Promise<void>;
}

export interface AgentClient {
  runtimeMode(): "native" | "browser-mock";
  listSessions(taskId: string): Promise<AgentSession[]>;
  listMessages(taskId: string, sessionId: string): Promise<AgentMessage[]>;
  listRuns(taskId: string, sessionId: string): Promise<AgentRun[]>;
  listToolCalls?(taskId: string, sessionId: string): Promise<AgentToolCall[]>;
  listPermissionRequests?(
    taskId: string,
    sessionId: string,
  ): Promise<AgentPermissionRequest[]>;
  listPermissionGrants?(
    taskId: string,
    sessionId: string,
  ): Promise<AgentPermissionGrant[]>;
  resolvePermission?(
    request: ResolveAgentPermissionRequest,
  ): Promise<AgentPermissionRequest>;
  revokePermissionGrant?(
    request: RevokeAgentPermissionGrantRequest,
  ): Promise<AgentPermissionGrant>;
  readToolOutput?(request: AgentToolOutputRequest): Promise<AgentToolOutput>;
  createSession(request: CreateAgentSessionRequest): Promise<AgentSession>;
  sendPrompt(request: AgentPromptRequest): Promise<AgentRun>;
  abortRun(request: AbortAgentRunRequest): Promise<void>;
  stopToolExecution?(request: StopAgentToolExecutionRequest): Promise<void>;
  selectGitRepository?(): Promise<string>;
  bindGitRepository?(request: BindGitRepositoryRequest): Promise<GitBinding>;
  getTaskGitStatus?(taskId: string): Promise<TaskGitStatus>;
  getTaskFileDiff?(taskId: string, path: string): Promise<TaskFileDiff>;
  commitTaskGitChanges?(
    request: CommitTaskGitChangesRequest,
  ): Promise<CommitTaskGitChangesResult>;
  cleanupTaskGitWorktree?(request: GitWorktreeActionRequest): Promise<GitBinding>;
  recoverTaskGitWorktree?(request: GitWorktreeActionRequest): Promise<GitBinding>;
  listResources?(request: AgentResourceSearchRequest): Promise<AgentResource[]>;
  listArtifacts?(taskId: string): Promise<AgentResource[]>;
  importAttachments?(request: ImportAgentAttachmentsRequest): Promise<AgentResource[]>;
  previewResource?(request: AgentResourcePreviewRequest): Promise<AgentResourcePreview>;
  removeReference?(request: RemoveAgentReferenceRequest): Promise<void>;
  openArtifact?(taskId: string, artifactId: string): Promise<void>;
  subscribe(listener: (event: AgentEvent) => void): () => void;
}

function nativeAppPresent(): boolean {
  return Boolean(window.go?.main?.App);
}

function nativeAgentApp(): NativeAgentApp {
  const app = window.go?.main?.App as unknown as
    | Partial<NativeAgentApp>
    | undefined;
  if (
    typeof app?.ListAgentSessions !== "function" ||
    typeof app.ListAgentMessages !== "function" ||
    typeof app.ListExecutionRuns !== "function" ||
    typeof app.ListAgentToolCalls !== "function" ||
    typeof app.ListAgentPermissionRequests !== "function" ||
    typeof app.ListAgentPermissionGrants !== "function" ||
    typeof app.ResolveAgentPermission !== "function" ||
    typeof app.RevokeAgentPermissionGrant !== "function" ||
    typeof app.ReadAgentToolOutput !== "function" ||
    typeof app.CreateAgentSession !== "function" ||
    typeof app.SendAgentPrompt !== "function" ||
    typeof app.AbortAgentRun !== "function" ||
    typeof app.ListAgentResources !== "function" ||
    typeof app.ListAgentArtifacts !== "function" ||
    typeof app.ImportAgentAttachments !== "function" ||
    typeof app.PreviewAgentResource !== "function" ||
    typeof app.RemoveAgentMessageReference !== "function" ||
    typeof app.OpenAgentArtifact !== "function"
  ) {
    throw new Error("当前桌面客户端不包含 PI 会话服务，请更新后重试");
  }
  return app as NativeAgentApp;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

export function parseAgentEvent(payload: unknown): AgentEvent | undefined {
  if (!isRecord(payload) || !isRecord(payload.payload)) return undefined;
  if (
    payload.version !== 1 ||
    typeof payload.eventId !== "string" ||
    payload.eventId.length === 0 ||
    typeof payload.sequence !== "number" ||
    !Number.isSafeInteger(payload.sequence) ||
    payload.sequence < 0 ||
    typeof payload.kind !== "string" ||
    payload.kind.length === 0 ||
    typeof payload.taskId !== "string" ||
    payload.taskId.length === 0 ||
    typeof payload.sessionId !== "string" ||
    payload.sessionId.length === 0 ||
    typeof payload.occurredAt !== "string" ||
    payload.occurredAt.length === 0
  ) {
    return undefined;
  }
  if (
    (payload.runId !== undefined && typeof payload.runId !== "string") ||
    (payload.toolCallId !== undefined &&
      typeof payload.toolCallId !== "string")
  ) {
    return undefined;
  }
  return payload as unknown as AgentEvent;
}

const browserSessions = new Map<string, AgentSession[]>();
const browserMessages = new Map<string, AgentMessage[]>();
const browserRunHistory = new Map<string, AgentRun[]>();
const browserResources = new Map<string, AgentResource[]>();
const browserPreviews = new Map<string, AgentResourcePreview>();
const browserListeners = new Set<(event: AgentEvent) => void>();
const browserTimers = new Set<number>();
const browserRuns = new Map<
  string,
  { run: AgentRun; assistantId: string; timers: number[]; }
>();
let browserID = 0;
let browserTick = 0;

function setBrowserTimer(action: () => void, delay: number): number {
  const timer = window.setTimeout(() => {
    browserTimers.delete(timer);
    action();
  }, delay);
  browserTimers.add(timer);
  return timer;
}

function clearBrowserTimer(timer: number): void {
  window.clearTimeout(timer);
  browserTimers.delete(timer);
}

function nextBrowserID(prefix: string): string {
  browserID += 1;
  return `${prefix}_browser_${browserID}`;
}

function browserTimestamp(): string {
  const value = new Date(Date.UTC(2026, 7, 1, 0, 0, browserTick));
  browserTick += 1;
  return value.toISOString();
}

function messageKey(taskId: string, sessionId: string): string {
  return `${taskId}\u0000${sessionId}`;
}

function copySession(session: AgentSession): AgentSession {
  return { ...session };
}

function copyMessage(message: AgentMessage): AgentMessage {
  return {
    ...message,
    references: message.references?.map((reference) => ({ ...reference })),
  };
}

function resourceKey(taskId: string, resourceId: string): string {
  return `${taskId}\u0000${resourceId}`;
}

function ensureBrowserResources(taskId: string): AgentResource[] {
  const current = browserResources.get(taskId);
  if (current) return current;
  const createdAt = browserTimestamp();
  const resources: AgentResource[] = [
    ["task", "context/task.md", `# ${taskId}\n\n浏览器模拟任务上下文。`],
    ["requirements", "context/requirements/current.md", "# 当前需求\n\n浏览器模拟需求。"],
    ["acceptance_criteria", "context/acceptance-criteria.md", "# 验收标准\n\n- [ ] 浏览器模拟验收。"],
  ].map(([sourceType, logicalPath, content], index) => {
    const id = `resource_${taskId}_context_${index}`;
    browserPreviews.set(resourceKey(taskId, id), {
      path: logicalPath,
      name: logicalPath.split("/").at(-1) ?? logicalPath,
      mimeType: "text/markdown",
      byteSize: new TextEncoder().encode(content).byteLength,
      sha256: "browser-preview",
      kind: "text",
      content,
      truncated: false,
    });
    return {
      id,
      taskId,
      targetType: "resource" as const,
      kind: "context",
      sourceType,
      logicalPath,
      mimeType: "text/markdown",
      byteSize: new TextEncoder().encode(content).byteLength,
      sha256: "browser-preview",
      immutable: sourceType !== "task",
      readable: true,
      createdAt,
    };
  });
  browserResources.set(taskId, resources);
  return resources;
}

function decodeBrowserBase64(value: string): Uint8Array {
  let decoded: string;
  try {
    decoded = window.atob(value);
  } catch {
    throw new Error("附件不是有效的 Base64 数据");
  }
  const result = new Uint8Array(decoded.length);
  for (let index = 0; index < decoded.length; index += 1) {
    result[index] = decoded.charCodeAt(index);
  }
  return result;
}

function browserSession(taskId: string, sessionId: string): AgentSession {
  const session = browserSessions
    .get(taskId)
    ?.find((candidate) => candidate.id === sessionId);
  if (!session) throw new Error("没有找到当前任务的 PI 会话");
  return session;
}

function emitBrowserEvent(
  session: AgentSession,
  kind: string,
  payload: Record<string, unknown>,
  runId = "",
): void {
  session.lastSequence += 1;
  session.updatedAt = browserTimestamp();
  session.lastActiveAt = session.updatedAt;
  const event: AgentEvent = {
    version: 1,
    eventId: nextBrowserID("event"),
    sequence: session.lastSequence,
    kind,
    taskId: session.taskId,
    sessionId: session.id,
    occurredAt: browserTimestamp(),
    payload,
  };
  if (runId) event.runId = runId;
  for (const listener of browserListeners) listener({ ...event });
}

function scheduleBrowserRun(
  session: AgentSession,
  run: AgentRun,
  assistant: AgentMessage,
  reply: string,
): number[] {
  const midpoint = Math.max(1, Math.floor(reply.length / 2));
  const parts = [reply.slice(0, midpoint), reply.slice(midpoint)].filter(Boolean);
  const timers: number[] = [];
  const schedule = (delay: number, action: () => void) => {
    timers.push(setBrowserTimer(action, delay));
  };

  schedule(0, () => {
    session.state = "running";
    run.state = "running";
    emitBrowserEvent(session, "run.state", { state: "running" }, run.id);
  });
  parts.forEach((part, index) => {
    schedule(10 * (index + 1), () => {
      if (!browserRuns.has(run.id)) return;
      assistant.content = `${assistant.content ?? ""}${part}`;
      emitBrowserEvent(
        session,
        "message.delta",
        {
          messageId: assistant.id,
          delta: part,
          accumulatedChars: Array.from(assistant.content).length,
        },
        run.id,
      );
    });
  });
  schedule(10 * (parts.length + 1), () => {
    if (!browserRuns.has(run.id)) return;
    const completedAt = browserTimestamp();
    assistant.status = "complete";
    assistant.completedAt = completedAt;
    run.state = "succeeded";
    run.finishedAt = completedAt;
    run.resultSummary = assistant.content;
    session.state = "idle";
    emitBrowserEvent(
      session,
      "message.end",
      { messageId: assistant.id, status: "complete", content: assistant.content },
      run.id,
    );
    emitBrowserEvent(session, "agent.settled", {}, run.id);
    emitBrowserEvent(
      session,
      "run.state",
      { state: "succeeded", reason: "" },
      run.id,
    );
    emitBrowserEvent(session, "session.state", { state: "idle" });
    browserRuns.delete(run.id);
  });
  return timers;
}

const browserAgentClient: AgentClient = {
  runtimeMode: () => "browser-mock",
  async listSessions(taskId) {
    return (browserSessions.get(taskId) ?? [])
      .map(copySession)
      .sort((left, right) => right.lastActiveAt.localeCompare(left.lastActiveAt));
  },
  async listMessages(taskId, sessionId) {
    browserSession(taskId, sessionId);
    return (browserMessages.get(messageKey(taskId, sessionId)) ?? [])
      .map(copyMessage)
      .sort((left, right) => left.sequence - right.sequence);
  },
  async listRuns(taskId, sessionId) {
    browserSession(taskId, sessionId);
    return (browserRunHistory.get(messageKey(taskId, sessionId)) ?? []).map(
      (run) => ({ ...run }),
    );
  },
  async listToolCalls(taskId, sessionId) {
    browserSession(taskId, sessionId);
    return [];
  },
  async listPermissionRequests(taskId, sessionId) {
    browserSession(taskId, sessionId);
    return [];
  },
  async listPermissionGrants(taskId, sessionId) {
    browserSession(taskId, sessionId);
    return [];
  },
  async resolvePermission() {
    throw new Error("浏览器模拟没有待审批的权限请求");
  },
  async revokePermissionGrant() {
    throw new Error("浏览器模拟没有可撤销的权限授权");
  },
  async readToolOutput() {
    throw new Error("浏览器模拟没有可懒加载的工具输出");
  },
  async listResources(request) {
    const query = request.query.trim().toLowerCase();
    const limit = Math.min(Math.max(request.limit || 50, 1), 100);
    return ensureBrowserResources(request.taskId)
      .filter((resource) =>
        !query ||
        `${resource.logicalPath} ${resource.kind} ${resource.sourceType ?? ""}`
          .toLowerCase()
          .includes(query),
      )
      .slice(0, limit)
      .map((resource) => ({ ...resource }));
  },
  async listArtifacts(taskId) {
    return ensureBrowserResources(taskId)
      .filter((resource) => resource.targetType === "artifact")
      .map((resource) => ({ ...resource }));
  },
  async importAttachments(request) {
    if (request.files.length < 1 || request.files.length > 10) {
      throw new Error("每次只能导入 1 至 10 个附件");
    }
    const resources = ensureBrowserResources(request.taskId);
    const created: AgentResource[] = [];
    let batchBytes = 0;
    for (const file of request.files) {
      const bytes = decodeBrowserBase64(file.dataBase64);
      if (bytes.byteLength === 0 || bytes.byteLength > 16 * 1024 * 1024) {
        throw new Error(`附件 ${file.name} 必须小于 16 MiB`);
      }
      batchBytes += bytes.byteLength;
      if (batchBytes > 32 * 1024 * 1024) {
        throw new Error("单次附件总大小不能超过 32 MiB");
      }
      const isImage = ["image/png", "image/jpeg", "image/gif", "image/webp"].includes(
        file.mimeType,
      );
      const isText =
        file.mimeType.startsWith("text/") ||
        ["application/json", "application/xml"].includes(file.mimeType);
      if (!isImage && !isText) throw new Error(`浏览器模拟不支持 ${file.mimeType || "未知类型"}`);
      const id = nextBrowserID(`resource_${request.taskId}`);
      const directory = isImage ? "attachments/images" : "attachments/documents";
      const logicalPath = `${directory}/${id}-${file.name.replace(/[^\p{L}\p{N}._-]+/gu, "-")}`;
      const resource: AgentResource = {
        id,
        taskId: request.taskId,
        targetType: "resource",
        kind: "attachment",
        sourceType: "message_attachment",
        logicalPath,
        mimeType: file.mimeType,
        byteSize: bytes.byteLength,
        sha256: "browser-preview",
        immutable: true,
        readable: true,
        createdAt: browserTimestamp(),
      };
      const content = isImage
        ? `data:${file.mimeType};base64,${file.dataBase64}`
        : new TextDecoder("utf-8", { fatal: true }).decode(bytes);
      browserPreviews.set(resourceKey(request.taskId, id), {
        path: logicalPath,
        name: file.name,
        mimeType: file.mimeType,
        byteSize: bytes.byteLength,
        sha256: "browser-preview",
        kind: isImage ? "image" : "text",
        content,
        truncated: false,
      });
      resources.push(resource);
      created.push({ ...resource });
    }
    return created;
  },
  async previewResource(request) {
    const exists = ensureBrowserResources(request.taskId).some(
      (resource) => resource.id === request.resourceId,
    );
    const preview = browserPreviews.get(resourceKey(request.taskId, request.resourceId));
    if (!exists || !preview) throw new Error("当前任务资源不存在或不支持预览");
    return { ...preview };
  },
  async removeReference(request) {
    const message = browserMessages
      .get(messageKey(request.taskId, request.sessionId))
      ?.find((candidate) => candidate.id === request.messageId);
    if (!message) throw new Error("当前任务消息不存在");
    message.references = message.references?.filter(
      (reference) => reference.resourceId !== request.resourceId,
    );
  },
  async openArtifact(taskId, artifactId) {
    const artifact = ensureBrowserResources(taskId).find(
      (resource) => resource.id === artifactId && resource.targetType === "artifact",
    );
    if (!artifact) throw new Error("当前任务 artifact 不存在");
  },
  async createSession(request) {
    if (!request.taskId.trim()) throw new Error("任务 ID 不能为空");
    const createdAt = browserTimestamp();
    const session: AgentSession = {
      id: nextBrowserID("session"),
      taskId: request.taskId,
      engine: "pi",
      title: request.title.trim() || "PI 会话",
      mode: request.mode,
      model: request.model.trim() || undefined,
      thinkingLevel: request.thinkingLevel.trim() || undefined,
      resourcePolicy: "isolated",
      state: "idle",
      lastSequence: 0,
      createdAt,
      updatedAt: createdAt,
      lastActiveAt: createdAt,
    };
    const sessions = browserSessions.get(request.taskId) ?? [];
    sessions.push(session);
    browserSessions.set(request.taskId, sessions);
    browserMessages.set(messageKey(request.taskId, session.id), []);
    browserRunHistory.set(messageKey(request.taskId, session.id), []);
    setBrowserTimer(
      () => emitBrowserEvent(session, "session.state", { state: "idle" }),
      0,
    );
    return copySession(session);
  },
  async sendPrompt(request) {
    const content = request.message.trim() ||
      ((request.resourceIds?.length ?? 0) > 0 ? "请查看所附的当前任务资源。" : "");
    if (!content) throw new Error("消息不能为空");
    if (
      Array.from(browserRuns.values()).some(
        ({ run }) => run.taskId === request.taskId,
      )
    ) {
      throw new Error("该任务已有正在运行的 PI 请求");
    }
    const session = browserSession(request.taskId, request.sessionId);
    const startedAt = browserTimestamp();
    const runId = nextBrowserID("run");
    const messages = browserMessages.get(
      messageKey(request.taskId, request.sessionId),
    )!;
    const user: AgentMessage = {
      id: nextBrowserID("message"),
      taskId: request.taskId,
      sessionId: request.sessionId,
      runId,
      role: "user",
      kind: "text",
      status: "complete",
      content,
      sequence: session.lastSequence + 1,
      createdAt: startedAt,
      completedAt: startedAt,
      references: (request.resourceIds ?? []).map((resourceId, position) => {
        const resource = ensureBrowserResources(request.taskId).find(
          (candidate) => candidate.id === resourceId,
        );
        if (!resource) throw new Error("引用不属于当前任务");
        return {
          taskId: request.taskId,
          sessionId: request.sessionId,
          messageId: "",
          resourceId,
          targetType: resource.targetType,
          method: resource.kind === "attachment" ? "attachment" : "mention",
          position,
          createdAt: startedAt,
          kind: resource.kind,
          sourceType: resource.sourceType,
          logicalPath: resource.logicalPath,
          mimeType: resource.mimeType,
          byteSize: resource.byteSize,
          immutable: resource.immutable,
        };
      }),
    };
    user.references?.forEach((reference) => {
      reference.messageId = user.id;
    });
    const assistant: AgentMessage = {
      id: nextBrowserID("message"),
      taskId: request.taskId,
      sessionId: request.sessionId,
      runId,
      role: "assistant",
      kind: "text",
      status: "streaming",
      content: "",
      sequence: session.lastSequence + 2,
      createdAt: startedAt,
    };
    session.lastSequence += 2;
    session.state = "running";
    messages.push(user, assistant);
    const run: AgentRun = {
      id: runId,
      taskId: request.taskId,
      sessionId: request.sessionId,
      mode: session.mode,
      state: "queued",
      eventsPath: `runs/${runId}/events.jsonl`,
      stdoutPath: `runs/${runId}/stdout.jsonl`,
      stderrPath: `runs/${runId}/stderr.log`,
      resultPath: `runs/${runId}/result.md`,
      startedAt,
    };
    browserRunHistory
      .get(messageKey(request.taskId, request.sessionId))!
      .push(run);
    emitBrowserEvent(session, "run.state", { state: "queued" }, run.id);
    emitBrowserEvent(
      session,
      "message.start",
      { messageId: user.id, role: "user", kind: "text" },
      run.id,
    );
    emitBrowserEvent(
      session,
      "message.end",
      { messageId: user.id, status: "complete", content },
      run.id,
    );
    emitBrowserEvent(
      session,
      "message.start",
      { messageId: assistant.id, role: "assistant", kind: "text" },
      run.id,
    );
    const reply = `浏览器模拟回复：${content}`;
    const active = { run, assistantId: assistant.id, timers: [] as number[] };
    browserRuns.set(run.id, active);
    active.timers = scheduleBrowserRun(session, run, assistant, reply);
    return { ...run };
  },
  async abortRun(request) {
    const active = browserRuns.get(request.runId);
    if (
      !active ||
      active.run.taskId !== request.taskId ||
      active.run.sessionId !== request.sessionId
    ) {
      throw new Error("没有匹配的活动 PI 运行");
    }
    active.timers.forEach(clearBrowserTimer);
    const session = browserSession(request.taskId, request.sessionId);
    const assistant = browserMessages
      .get(messageKey(request.taskId, request.sessionId))
      ?.find((message) => message.id === active.assistantId);
    const completedAt = browserTimestamp();
    if (assistant) {
      assistant.status = "cancelled";
      assistant.completedAt = completedAt;
      emitBrowserEvent(
        session,
        "message.end",
        { messageId: assistant.id, status: "cancelled" },
        active.run.id,
      );
    }
    active.run.state = "cancelled";
    active.run.finishedAt = completedAt;
    session.state = "idle";
    emitBrowserEvent(
      session,
      "run.state",
      { state: "cancelled", reason: "用户已停止本次回答" },
      active.run.id,
    );
    emitBrowserEvent(session, "session.state", { state: "idle" });
    browserRuns.delete(active.run.id);
  },
  async stopToolExecution() {
    throw new Error("浏览器模拟不执行本机 Shell 命令");
  },
  async selectGitRepository() {
    return "";
  },
  async bindGitRepository() {
    throw new Error("浏览器模拟不能绑定本机 Git 仓库");
  },
  async getTaskGitStatus() {
    return {
      bound: false,
      binding: {} as GitBinding,
      files: [],
      aheadOfBaseline: 0,
      behindBaseline: 0,
    };
  },
  async getTaskFileDiff() {
    throw new Error("浏览器模拟没有本机 Git Diff");
  },
  async commitTaskGitChanges() {
    throw new Error("浏览器模拟不能创建本地 commit");
  },
  async cleanupTaskGitWorktree() {
    throw new Error("浏览器模拟没有可清理的 Git worktree");
  },
  async recoverTaskGitWorktree() {
    throw new Error("浏览器模拟没有可恢复的 Git worktree");
  },
  subscribe(listener) {
    browserListeners.add(listener);
    return () => browserListeners.delete(listener);
  },
};

const nativeAgentClient: AgentClient = {
  runtimeMode: () => "native",
  listSessions: (taskId) => nativeAgentApp().ListAgentSessions(taskId),
  listMessages: (taskId, sessionId) =>
    nativeAgentApp().ListAgentMessages(taskId, sessionId),
  listRuns: (taskId, sessionId) =>
    nativeAgentApp().ListExecutionRuns(taskId, sessionId),
  listToolCalls: (taskId, sessionId) =>
    nativeAgentApp().ListAgentToolCalls(taskId, sessionId),
  listPermissionRequests: (taskId, sessionId) =>
    nativeAgentApp().ListAgentPermissionRequests(taskId, sessionId),
  listPermissionGrants: (taskId, sessionId) =>
    nativeAgentApp().ListAgentPermissionGrants(taskId, sessionId),
  resolvePermission: (request) =>
    nativeAgentApp().ResolveAgentPermission(request),
  revokePermissionGrant: (request) =>
    nativeAgentApp().RevokeAgentPermissionGrant(request),
  readToolOutput: (request) => nativeAgentApp().ReadAgentToolOutput(request),
  createSession: (request) => nativeAgentApp().CreateAgentSession(request),
  sendPrompt: (request) => nativeAgentApp().SendAgentPrompt(request),
  abortRun: (request) => nativeAgentApp().AbortAgentRun(request),
  stopToolExecution: (request) => {
    const app = nativeAgentApp();
    if (!app.StopAgentToolExecution) throw new Error("当前客户端不支持停止 Shell 工具");
    return app.StopAgentToolExecution(request);
  },
  selectGitRepository: () => {
    const app = nativeAgentApp();
    if (!app.SelectGitRepository) throw new Error("当前客户端不支持 Git worktree");
    return app.SelectGitRepository();
  },
  bindGitRepository: (request) => {
    const app = nativeAgentApp();
    if (!app.BindGitRepository) throw new Error("当前客户端不支持 Git worktree");
    return app.BindGitRepository(request);
  },
  getTaskGitStatus: (taskId) => {
    const app = nativeAgentApp();
    if (!app.GetTaskGitStatus) throw new Error("当前客户端不支持 Git worktree");
    return app.GetTaskGitStatus(taskId);
  },
  getTaskFileDiff: (taskId, path) => {
    const app = nativeAgentApp();
    if (!app.GetTaskFileDiff) throw new Error("当前客户端不支持 Git Diff");
    return app.GetTaskFileDiff(taskId, path);
  },
  commitTaskGitChanges: (request) => {
    const app = nativeAgentApp();
    if (!app.CommitTaskGitChanges) throw new Error("当前客户端不支持本地 commit");
    return app.CommitTaskGitChanges(request);
  },
  cleanupTaskGitWorktree: (request) => {
    const app = nativeAgentApp();
    if (!app.CleanupTaskGitWorktree) throw new Error("当前客户端不支持清理 Git worktree");
    return app.CleanupTaskGitWorktree(request);
  },
  recoverTaskGitWorktree: (request) => {
    const app = nativeAgentApp();
    if (!app.RecoverTaskGitWorktree) throw new Error("当前客户端不支持恢复 Git worktree");
    return app.RecoverTaskGitWorktree(request);
  },
  listResources: (request) => nativeAgentApp().ListAgentResources(request),
  listArtifacts: (taskId) => nativeAgentApp().ListAgentArtifacts(taskId),
  importAttachments: (request) => nativeAgentApp().ImportAgentAttachments(request),
  previewResource: (request) => nativeAgentApp().PreviewAgentResource(request),
  removeReference: (request) => nativeAgentApp().RemoveAgentMessageReference(request),
  openArtifact: (taskId, artifactId) => nativeAgentApp().OpenAgentArtifact(taskId, artifactId),
  subscribe(listener) {
    if (typeof window.runtime?.EventsOn !== "function") return () => undefined;
    const unsubscribe = window.runtime.EventsOn(AGENT_EVENT_NAME, (payload) => {
      const event = parseAgentEvent(payload);
      if (event) listener(event);
    });
    return typeof unsubscribe === "function" ? unsubscribe : () => undefined;
  },
};

export const agentClient: AgentClient = {
  runtimeMode: () => (nativeAppPresent() ? "native" : "browser-mock"),
  listSessions: (taskId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).listSessions(
      taskId,
    ),
  listMessages: (taskId, sessionId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).listMessages(
      taskId,
      sessionId,
    ),
  listRuns: (taskId, sessionId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).listRuns(
      taskId,
      sessionId,
    ),
  listToolCalls: (taskId, sessionId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).listToolCalls!(
      taskId,
      sessionId,
    ),
  listPermissionRequests: (taskId, sessionId) =>
    (nativeAppPresent()
      ? nativeAgentClient
      : browserAgentClient).listPermissionRequests!(taskId, sessionId),
  listPermissionGrants: (taskId, sessionId) =>
    (nativeAppPresent()
      ? nativeAgentClient
      : browserAgentClient).listPermissionGrants!(taskId, sessionId),
  resolvePermission: (request) =>
    (nativeAppPresent()
      ? nativeAgentClient
      : browserAgentClient).resolvePermission!(request),
  revokePermissionGrant: (request) =>
    (nativeAppPresent()
      ? nativeAgentClient
      : browserAgentClient).revokePermissionGrant!(request),
  readToolOutput: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).readToolOutput!(
      request,
    ),
  createSession: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).createSession(
      request,
    ),
  sendPrompt: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).sendPrompt(
      request,
    ),
  abortRun: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).abortRun(
      request,
    ),
  stopToolExecution: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).stopToolExecution!(request),
  selectGitRepository: () =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).selectGitRepository!(),
  bindGitRepository: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).bindGitRepository!(request),
  getTaskGitStatus: (taskId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).getTaskGitStatus!(taskId),
  getTaskFileDiff: (taskId, path) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).getTaskFileDiff!(taskId, path),
  commitTaskGitChanges: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).commitTaskGitChanges!(request),
  cleanupTaskGitWorktree: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).cleanupTaskGitWorktree!(request),
  recoverTaskGitWorktree: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).recoverTaskGitWorktree!(request),
  listResources: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).listResources!(request),
  listArtifacts: (taskId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).listArtifacts!(taskId),
  importAttachments: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).importAttachments!(request),
  previewResource: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).previewResource!(request),
  removeReference: (request) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).removeReference!(request),
  openArtifact: (taskId, artifactId) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).openArtifact!(taskId, artifactId),
  subscribe: (listener) =>
    (nativeAppPresent() ? nativeAgentClient : browserAgentClient).subscribe(
      listener,
    ),
};

export function resetBrowserAgentMockForTests(): void {
  browserTimers.forEach((timer) => window.clearTimeout(timer));
  browserTimers.clear();
  browserSessions.clear();
  browserMessages.clear();
  browserRunHistory.clear();
  browserResources.clear();
  browserPreviews.clear();
  browserListeners.clear();
  browserRuns.clear();
  browserID = 0;
  browserTick = 0;
}
