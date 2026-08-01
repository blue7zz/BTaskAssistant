import type {
  AbortAgentRunRequest,
  AgentEvent,
  AgentMessage,
  AgentPromptRequest,
  AgentRun,
  AgentSession,
  CreateAgentSessionRequest,
} from "../domain/agent";

const AGENT_EVENT_NAME = "agent:event";

interface NativeAgentApp {
  ListAgentSessions(taskId: string): Promise<AgentSession[]>;
  ListAgentMessages(
    taskId: string,
    sessionId: string,
  ): Promise<AgentMessage[]>;
  ListExecutionRuns(taskId: string, sessionId: string): Promise<AgentRun[]>;
  CreateAgentSession(
    request: CreateAgentSessionRequest,
  ): Promise<AgentSession>;
  SendAgentPrompt(request: AgentPromptRequest): Promise<AgentRun>;
  AbortAgentRun(request: AbortAgentRunRequest): Promise<void>;
}

export interface AgentClient {
  runtimeMode(): "native" | "browser-mock";
  listSessions(taskId: string): Promise<AgentSession[]>;
  listMessages(taskId: string, sessionId: string): Promise<AgentMessage[]>;
  listRuns(taskId: string, sessionId: string): Promise<AgentRun[]>;
  createSession(request: CreateAgentSessionRequest): Promise<AgentSession>;
  sendPrompt(request: AgentPromptRequest): Promise<AgentRun>;
  abortRun(request: AbortAgentRunRequest): Promise<void>;
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
    typeof app.CreateAgentSession !== "function" ||
    typeof app.SendAgentPrompt !== "function" ||
    typeof app.AbortAgentRun !== "function"
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
const browserListeners = new Set<(event: AgentEvent) => void>();
const browserTimers = new Set<number>();
const browserRuns = new Map<
  string,
  { run: AgentRun; assistantId: string; timers: number[] }
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
  return { ...message };
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
    const content = request.message.trim();
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
    };
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
  createSession: (request) => nativeAgentApp().CreateAgentSession(request),
  sendPrompt: (request) => nativeAgentApp().SendAgentPrompt(request),
  abortRun: (request) => nativeAgentApp().AbortAgentRun(request),
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
  browserListeners.clear();
  browserRuns.clear();
  browserID = 0;
  browserTick = 0;
}
