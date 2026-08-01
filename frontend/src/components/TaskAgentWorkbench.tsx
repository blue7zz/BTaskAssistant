import {
  AlertTriangle,
  Bot,
  LoaderCircle,
  MessageSquare,
  Plus,
  Send,
  Square,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import type {
  AgentEvent,
  AgentMessage,
  AgentMessageStatus,
  AgentSession,
} from "../domain/agent";
import { agentClient, type AgentClient } from "../lib/agentBridge";
import { useWorkspaceStore } from "../store/workspace";

interface TaskAgentWorkbenchProps {
  taskId: string;
  taskTitle: string;
  client?: AgentClient;
}

const FINAL_RUN_STATES = new Set([
  "succeeded",
  "failed",
  "cancelled",
  "interrupted",
]);
const ACTIVE_RUN_STATES = new Set(["queued", "running", "waiting_permission", "stopping"]);
const MESSAGE_STATUSES = new Set<AgentMessageStatus>([
  "pending",
  "streaming",
  "complete",
  "error",
  "cancelled",
]);

function errorText(error: unknown): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return "PI 会话操作失败";
}

function payloadString(
  payload: Record<string, unknown>,
  key: string,
): string {
  return typeof payload[key] === "string" ? payload[key] : "";
}

function eventError(event: AgentEvent): string {
  const nested = event.payload.error;
  if (nested && typeof nested === "object" && !Array.isArray(nested)) {
    const message = (nested as Record<string, unknown>).message;
    if (typeof message === "string") return message;
  }
  return payloadString(event.payload, "message") || "PI 运行失败";
}

function sortMessages(messages: AgentMessage[]): AgentMessage[] {
  return [...messages].sort(
    (left, right) =>
      left.sequence - right.sequence || left.createdAt.localeCompare(right.createdAt),
  );
}

function formatSessionTime(value: string): string {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.valueOf())) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(timestamp);
}

export function TaskAgentWorkbench({
  taskId,
  taskTitle,
  client = agentClient,
}: TaskAgentWorkbenchProps) {
  const piSettings = useWorkspaceStore((state) => state.piSettings);
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [selectedSessionId, setSelectedSessionId] = useState("");
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [sending, setSending] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [activeRunId, setActiveRunId] = useState("");
  const [error, setError] = useState("");
  const epochRef = useRef(0);
  const selectedSessionRef = useRef("");
  const terminalRunsRef = useRef(new Set<string>());
  const lastEventSequenceRef = useRef(new Map<string, number>());

  useEffect(() => {
    selectedSessionRef.current = selectedSessionId;
  }, [selectedSessionId]);

  const reloadMessages = useCallback(
    async (sessionId: string, epoch = epochRef.current) => {
      if (!sessionId) {
        if (epoch === epochRef.current) setMessages([]);
        return;
      }
      const loaded = await client.listMessages(taskId, sessionId);
      if (
        epoch !== epochRef.current ||
        selectedSessionRef.current !== sessionId
      ) {
        return;
      }
      setMessages(sortMessages(loaded));
    },
    [client, taskId],
  );

  useEffect(() => {
    const epoch = ++epochRef.current;
    terminalRunsRef.current.clear();
    lastEventSequenceRef.current.clear();
    selectedSessionRef.current = "";
    setSessions([]);
    setSelectedSessionId("");
    setMessages([]);
    setDraft("");
    setActiveRunId("");
    setError("");
    setLoading(true);
    setCreating(false);
    setSending(false);
    setStopping(false);

    const unsubscribe = client.subscribe((event) => {
      if (epoch !== epochRef.current || event.taskId !== taskId) return;
      const previous = lastEventSequenceRef.current.get(event.sessionId) ?? -1;
      if (event.sequence <= previous) return;
      lastEventSequenceRef.current.set(event.sessionId, event.sequence);

      if (event.kind === "session.state") {
        const state = payloadString(event.payload, "state");
        setSessions((current) =>
          current.map((session) =>
            session.id === event.sessionId
              ? {
                  ...session,
                  state: (state || session.state) as AgentSession["state"],
                  errorMessage:
                    state === "failed" ? eventError(event) : undefined,
                }
              : session,
          ),
        );
      }

      if (
        selectedSessionRef.current &&
        event.sessionId !== selectedSessionRef.current
      ) {
        return;
      }
      if (!selectedSessionRef.current) return;

      const messageId = payloadString(event.payload, "messageId");
      if (event.kind === "message.start" && messageId) {
        const role = payloadString(event.payload, "role");
        setMessages((current) => {
          if (current.some((message) => message.id === messageId)) return current;
          return sortMessages([
            ...current,
            {
              id: messageId,
              taskId,
              sessionId: event.sessionId,
              runId: event.runId,
              role:
                role === "user" || role === "system" ? role : "assistant",
              kind: "text",
              status: role === "user" ? "pending" : "streaming",
              content: "",
              sequence: event.sequence,
              createdAt: event.occurredAt,
            },
          ]);
        });
      } else if (event.kind === "message.delta" && messageId) {
        const delta = payloadString(event.payload, "delta");
        setMessages((current) =>
          current.map((message) =>
            message.id === messageId
              ? {
                  ...message,
                  content: `${message.content ?? ""}${delta}`,
                  status: "streaming",
                }
              : message,
          ),
        );
      } else if (event.kind === "message.end" && messageId) {
        const rawStatus = payloadString(event.payload, "status");
        const status = MESSAGE_STATUSES.has(rawStatus as AgentMessageStatus)
          ? (rawStatus as AgentMessageStatus)
          : "complete";
        const content = event.payload.content;
        setMessages((current) =>
          current.map((message) =>
            message.id === messageId
              ? {
                  ...message,
                  status,
                  content:
                    typeof content === "string" ? content : message.content,
                  completedAt: event.occurredAt,
                }
              : message,
          ),
        );
        void reloadMessages(event.sessionId, epoch).catch(() => undefined);
      } else if (event.kind === "error") {
        setError(eventError(event));
      }

      if (event.kind === "run.state" && event.runId) {
        const state = payloadString(event.payload, "state");
        if (FINAL_RUN_STATES.has(state)) {
          terminalRunsRef.current.add(event.runId);
          setActiveRunId((current) =>
            current === event.runId ? "" : current,
          );
          setSending(false);
          setStopping(false);
          if (state === "failed" || state === "interrupted") {
            setError(
              payloadString(event.payload, "reason") || "PI 运行未完成",
            );
          }
        } else if (["queued", "running", "stopping"].includes(state)) {
          setActiveRunId(event.runId);
          setStopping(state === "stopping");
        }
      }
    });

    void client
      .listSessions(taskId)
      .then(async (loaded) => {
        const runs = (
          await Promise.all(
            loaded.map((session) => client.listRuns(taskId, session.id)),
          )
        ).flat();
        if (epoch !== epochRef.current) return;
        setSessions(loaded);
        const activeRun = runs.find((run) => ACTIVE_RUN_STATES.has(run.state));
        const selected = activeRun?.sessionId ?? loaded[0]?.id ?? "";
        selectedSessionRef.current = selected;
        setSelectedSessionId(selected);
        setActiveRunId(activeRun?.id ?? "");
        setStopping(activeRun?.state === "stopping");
      })
      .catch((reason) => {
        if (epoch === epochRef.current) setError(errorText(reason));
      })
      .finally(() => {
        if (epoch === epochRef.current) setLoading(false);
      });

    return () => {
      epochRef.current += 1;
      unsubscribe();
    };
  }, [client, reloadMessages, taskId]);

  useEffect(() => {
    if (!selectedSessionId) {
      setMessages([]);
      return;
    }
    const epoch = epochRef.current;
    setLoading(true);
    setError("");
    void reloadMessages(selectedSessionId, epoch)
      .catch((reason) => {
        if (epoch === epochRef.current) setError(errorText(reason));
      })
      .finally(() => {
        if (epoch === epochRef.current) setLoading(false);
      });
  }, [reloadMessages, selectedSessionId]);

  const createSession = async () => {
    const epoch = epochRef.current;
    setCreating(true);
    setError("");
    try {
      const session = await client.createSession({
        taskId,
        title: `${taskTitle} · PI`,
        mode: "ask",
        model: piSettings.model,
        thinkingLevel: piSettings.thinkingEffort,
      });
      if (epoch !== epochRef.current) return;
      selectedSessionRef.current = session.id;
      setSessions((current) => [
        session,
        ...current.filter((candidate) => candidate.id !== session.id),
      ]);
      setSelectedSessionId(session.id);
      setMessages([]);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setCreating(false);
    }
  };

  const sendPrompt = async (event: FormEvent) => {
    event.preventDefault();
    const message = draft.trim();
    const sessionId = selectedSessionRef.current;
    if (!message || !sessionId || activeRunId) return;
    const epoch = epochRef.current;
    setSending(true);
    setError("");
    setDraft("");
    try {
      const run = await client.sendPrompt({ taskId, sessionId, message });
      if (epoch !== epochRef.current) return;
      if (!terminalRunsRef.current.has(run.id)) setActiveRunId(run.id);
      await reloadMessages(sessionId, epoch);
    } catch (reason) {
      if (epoch !== epochRef.current) return;
      setDraft(message);
      setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setSending(false);
    }
  };

  const stopRun = async () => {
    const sessionId = selectedSessionRef.current;
    if (!activeRunId || !sessionId) return;
    const epoch = epochRef.current;
    const runId = activeRunId;
    setStopping(true);
    setError("");
    try {
      await client.abortRun({ taskId, sessionId, runId });
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setStopping(false);
    }
  };

  const activeSession = sessions.find(
    (session) => session.id === selectedSessionId,
  );
  const busy = Boolean(activeRunId);

  return (
    <section className="task-agent-workbench" aria-label="PI 会话工作台">
      <aside className="agent-session-panel">
        <div className="agent-session-heading">
          <div>
            <span className="eyebrow">任务级 Session</span>
            <strong>PI 会话</strong>
          </div>
          <button
            type="button"
            className="icon-button"
            aria-label="新建 PI 会话"
            title="新建 PI 会话"
            disabled={creating || busy}
            onClick={() => void createSession()}
          >
            {creating ? <LoaderCircle className="spin" size={15} /> : <Plus size={15} />}
          </button>
        </div>
        <div className="agent-runtime-note">
          <Bot size={14} />
          <span>
            {client.runtimeMode() === "browser-mock"
              ? "浏览器模拟，不启动本机 PI"
              : "原生 PI RPC · 无工具隔离"}
          </span>
        </div>
        <div className="agent-session-list">
          {sessions.map((session) => (
            <button
              type="button"
              key={session.id}
              className={session.id === selectedSessionId ? "active" : ""}
              disabled={busy && session.id !== selectedSessionId}
              onClick={() => {
                selectedSessionRef.current = session.id;
                setSelectedSessionId(session.id);
              }}
            >
              <MessageSquare size={14} />
              <span>
                <strong>{session.title}</strong>
                <small>
                  {session.state} · {formatSessionTime(session.lastActiveAt)}
                </small>
              </span>
            </button>
          ))}
          {!loading && sessions.length === 0 && (
            <div className="agent-session-empty">
              <p>还没有 PI 会话。</p>
              <button
                type="button"
                className="button secondary compact"
                onClick={() => void createSession()}
                disabled={creating}
              >
                <Plus size={13} />
                新建会话
              </button>
            </div>
          )}
        </div>
      </aside>

      <div className="agent-chat-panel">
        <header className="agent-chat-heading">
          <div>
            <strong>{activeSession?.title ?? "选择或新建会话"}</strong>
            <span>会话数据只属于当前任务，不会写入任务状态快照。</span>
          </div>
          {busy && (
            <button
              type="button"
              className="button secondary compact agent-stop-button"
              onClick={() => void stopRun()}
              disabled={stopping}
            >
              {stopping ? <LoaderCircle className="spin" size={13} /> : <Square size={12} />}
              {stopping ? "正在停止" : "停止"}
            </button>
          )}
        </header>

        {error && (
          <div className="agent-error" role="alert">
            <AlertTriangle size={15} />
            <span>{error}</span>
            <button type="button" onClick={() => setError("")}>
              关闭
            </button>
          </div>
        )}

        <div className="agent-message-list" aria-live="polite">
          {loading && (
            <div className="agent-loading">
              <LoaderCircle className="spin" size={17} />
              正在读取会话…
            </div>
          )}
          {!loading && selectedSessionId && messages.length === 0 && (
            <div className="agent-chat-empty">
              <Bot size={26} />
              <strong>开始当前任务的第一轮对话</strong>
              <p>Phase 2 会话处于无工具模式，PI 不能读写文件或执行命令。</p>
            </div>
          )}
          {!loading && !selectedSessionId && (
            <div className="agent-chat-empty">
              <MessageSquare size={25} />
              <strong>新建一个任务级 PI 会话</strong>
              <p>历史消息会增量保存在 SQLite，并与其他任务隔离。</p>
            </div>
          )}
          {messages.map((message) => (
            <article
              key={message.id}
              className={`agent-message agent-message-${message.role}`}
            >
              <header>
                <strong>{message.role === "user" ? "你" : "PI"}</strong>
                <span>{message.status}</span>
              </header>
              <p>{message.content || (message.status === "streaming" ? "…" : "")}</p>
            </article>
          ))}
        </div>

        <form className="agent-composer" onSubmit={(event) => void sendPrompt(event)}>
          <textarea
            value={draft}
            aria-label="发送给 PI 的消息"
            placeholder={
              selectedSessionId
                ? "输入消息；当前阶段 PI 不可调用工具……"
                : "请先新建 PI 会话"
            }
            disabled={!selectedSessionId || busy}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
          />
          <button
            type="submit"
            className="button primary compact"
            disabled={!selectedSessionId || !draft.trim() || busy || sending}
          >
            {sending ? <LoaderCircle className="spin" size={14} /> : <Send size={14} />}
            发送
          </button>
        </form>
      </div>
    </section>
  );
}
