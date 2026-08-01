import {
  AlertTriangle,
  AtSign,
  Bot,
  ExternalLink,
  FileText,
  FolderOpen,
  Image as ImageIcon,
  LoaderCircle,
  MessageSquare,
  Paperclip,
  Plus,
  Send,
  ShieldAlert,
  Square,
  X,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type DragEvent,
  type FormEvent,
} from "react";
import type {
  AgentEvent,
  AgentMessage,
  AgentMessageStatus,
  AgentPermissionGrant,
  AgentPermissionRequest,
  AgentPermissionScope,
  AgentResource,
  AgentResourcePreview,
  AgentSession,
  AgentToolCall,
} from "../domain/agent";
import { agentClient, type AgentClient } from "../lib/agentBridge";
import { useWorkspaceStore } from "../store/workspace";
import { AgentPermissionCard } from "./AgentPermissionCard";
import { AgentToolCard } from "./AgentToolCard";

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
const GOVERNANCE_EVENT_KINDS = new Set([
  "permission.requested",
  "permission.resolved",
  "permission.revoked",
  "tool.start",
  "tool.update",
  "tool.end",
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

function resourceName(resource: Pick<AgentResource, "logicalPath">): string {
  return resource.logicalPath.split("/").at(-1) ?? resource.logicalPath;
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${Math.ceil(value / 1024)} KiB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MiB`;
}

function permissionScopeLabel(scope: AgentPermissionScope): string {
  switch (scope) {
    case "once":
      return "仅本次";
    case "session":
      return "本会话";
    case "task":
      return "当前任务";
    case "permanent":
      return "永久";
  }
}

function readFileBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("读取附件失败"));
    reader.onload = () => {
      const value = typeof reader.result === "string" ? reader.result : "";
      const separator = value.indexOf(",");
      if (separator < 0) reject(new Error(`附件 ${file.name} 编码失败`));
      else resolve(value.slice(separator + 1));
    };
    reader.readAsDataURL(file);
  });
}

function mentionQuery(value: string): string | undefined {
  const match = value.match(/(?:^|\s)@([^\s@]*)$/u);
  return match?.[1];
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
  const [permissionRequests, setPermissionRequests] = useState<AgentPermissionRequest[]>([]);
  const [permissionGrants, setPermissionGrants] = useState<AgentPermissionGrant[]>([]);
  const [toolCalls, setToolCalls] = useState<AgentToolCall[]>([]);
  const [resolvingPermissions, setResolvingPermissions] = useState<Set<string>>(new Set());
  const [revokingGrants, setRevokingGrants] = useState<Set<string>>(new Set());
  const [draft, setDraft] = useState("");
  const [newSessionMode, setNewSessionMode] = useState<AgentSession["mode"]>("ask");
  const [resources, setResources] = useState<AgentResource[]>([]);
  const [artifacts, setArtifacts] = useState<AgentResource[]>([]);
  const [mentionOptions, setMentionOptions] = useState<AgentResource[]>([]);
  const [selectedResources, setSelectedResources] = useState<AgentResource[]>([]);
  const [resourcePanel, setResourcePanel] = useState<"resources" | "artifacts">("resources");
  const [preview, setPreview] = useState<AgentResourcePreview>();
  const [previewResource, setPreviewResource] = useState<AgentResource>();
  const [previewLoading, setPreviewLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [dragActive, setDragActive] = useState(false);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [sending, setSending] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [activeRunId, setActiveRunId] = useState("");
  const [error, setError] = useState("");
  const epochRef = useRef(0);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const selectedSessionRef = useRef("");
  const resolvingPermissionsRef = useRef(new Set<string>());
  const revokingGrantsRef = useRef(new Set<string>());
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

  const reloadGovernance = useCallback(
    async (sessionId: string, epoch = epochRef.current) => {
      if (!sessionId) {
        if (epoch === epochRef.current) {
          setPermissionRequests([]);
          setPermissionGrants([]);
          setToolCalls([]);
        }
        return;
      }
      const [loadedRequests, loadedGrants, loadedTools] = await Promise.all([
        client.listPermissionRequests?.(taskId, sessionId) ?? Promise.resolve([]),
        client.listPermissionGrants?.(taskId, sessionId) ?? Promise.resolve([]),
        client.listToolCalls?.(taskId, sessionId) ?? Promise.resolve([]),
      ]);
      if (
        epoch !== epochRef.current ||
        selectedSessionRef.current !== sessionId
      ) {
        return;
      }
      setPermissionRequests(loadedRequests);
      setPermissionGrants(loadedGrants);
      setToolCalls(loadedTools);
    },
    [client, taskId],
  );

  const reloadResources = useCallback(
    async (epoch = epochRef.current) => {
      if (!client.listResources) {
        if (epoch === epochRef.current) {
          setResources([]);
          setArtifacts([]);
        }
        return;
      }
      const [loadedResources, loadedArtifacts] = await Promise.all([
        client.listResources({ taskId, query: "", limit: 100 }),
        client.listArtifacts?.(taskId) ?? Promise.resolve([]),
      ]);
      if (epoch !== epochRef.current) return;
      setResources(loadedResources.filter((resource) => resource.targetType === "resource"));
      setArtifacts(loadedArtifacts);
    },
    [client, taskId],
  );

  const showPreview = useCallback(
    async (resource: AgentResource) => {
      if (!client.previewResource) return;
      const epoch = epochRef.current;
      setPreviewResource(resource);
      setPreview(undefined);
      setPreviewLoading(true);
      setError("");
      try {
        const loaded = await client.previewResource({
          taskId,
          resourceId: resource.id,
        });
        if (epoch === epochRef.current) setPreview(loaded);
      } catch (reason) {
        if (epoch === epochRef.current) setError(errorText(reason));
      } finally {
        if (epoch === epochRef.current) setPreviewLoading(false);
      }
    },
    [client, taskId],
  );

  const importFiles = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return;
      if (!client.importAttachments) {
        setError("当前客户端不支持任务附件");
        return;
      }
      if (files.length > 10) {
        setError("每次最多选择 10 个附件");
        return;
      }
      const oversized = files.find((file) => file.size === 0 || file.size > 16 * 1024 * 1024);
      if (oversized) {
        setError(`附件 ${oversized.name} 必须大于 0 且不超过 16 MiB`);
        return;
      }
      const batchBytes = files.reduce((total, file) => total + file.size, 0);
      if (batchBytes > 32 * 1024 * 1024) {
        setError("单次附件总大小不能超过 32 MiB");
        return;
      }
      const epoch = epochRef.current;
      setImporting(true);
      setError("");
      try {
        const uploads = await Promise.all(
          files.map(async (file) => ({
            name: file.name,
            mimeType: file.type,
            dataBase64: await readFileBase64(file),
          })),
        );
        const imported = await client.importAttachments({ taskId, files: uploads });
        if (epoch !== epochRef.current) return;
        setSelectedResources((current) => [
          ...current,
          ...imported.filter((resource) => !current.some((item) => item.id === resource.id)),
        ]);
        await reloadResources(epoch);
      } catch (reason) {
        if (epoch === epochRef.current) setError(errorText(reason));
      } finally {
        if (epoch === epochRef.current) setImporting(false);
      }
    },
    [client, reloadResources, taskId],
  );

  useEffect(() => {
    const epoch = ++epochRef.current;
    terminalRunsRef.current.clear();
    lastEventSequenceRef.current.clear();
    resolvingPermissionsRef.current.clear();
    revokingGrantsRef.current.clear();
    selectedSessionRef.current = "";
    setSessions([]);
    setSelectedSessionId("");
    setMessages([]);
    setPermissionRequests([]);
    setPermissionGrants([]);
    setToolCalls([]);
    setResolvingPermissions(new Set());
    setRevokingGrants(new Set());
    setDraft("");
    setResources([]);
    setArtifacts([]);
    setMentionOptions([]);
    setSelectedResources([]);
    setPreview(undefined);
    setPreviewResource(undefined);
    setResourcePanel("resources");
    setDragActive(false);
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

      if (GOVERNANCE_EVENT_KINDS.has(event.kind)) {
        void reloadGovernance(event.sessionId, epoch).catch((reason) => {
          if (epoch === epochRef.current) setError(errorText(reason));
        });
      }

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
      } else if (event.kind === "workspace.changed") {
        void reloadResources(epoch).catch((reason) => {
          if (epoch === epochRef.current) setError(errorText(reason));
        });
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
    void reloadResources(epoch).catch((reason) => {
      if (epoch === epochRef.current) setError(errorText(reason));
    });

    return () => {
      epochRef.current += 1;
      unsubscribe();
    };
  }, [client, reloadGovernance, reloadMessages, reloadResources, taskId]);

  useEffect(() => {
    if (!selectedSessionId) {
      setMessages([]);
      setPermissionRequests([]);
      setPermissionGrants([]);
      setToolCalls([]);
      return;
    }
    const epoch = epochRef.current;
    setLoading(true);
    setError("");
    void Promise.all([
      reloadMessages(selectedSessionId, epoch),
      reloadGovernance(selectedSessionId, epoch),
    ])
      .catch((reason) => {
        if (epoch === epochRef.current) setError(errorText(reason));
      })
      .finally(() => {
        if (epoch === epochRef.current) setLoading(false);
      });
  }, [reloadGovernance, reloadMessages, selectedSessionId]);

  const activeMentionQuery = mentionQuery(draft);
  useEffect(() => {
    if (activeMentionQuery === undefined || !client.listResources || !selectedSessionId) {
      setMentionOptions([]);
      return;
    }
    const epoch = epochRef.current;
    void client
      .listResources({ taskId, query: activeMentionQuery, limit: 8 })
      .then((loaded) => {
        if (epoch === epochRef.current) setMentionOptions(loaded);
      })
      .catch((reason) => {
        if (epoch === epochRef.current) setError(errorText(reason));
      });
  }, [activeMentionQuery, client, selectedSessionId, taskId]);

  const selectMention = (resource: AgentResource) => {
    setSelectedResources((current) =>
      current.some((item) => item.id === resource.id) ? current : [...current, resource],
    );
    setDraft((current) => {
      const match = current.match(/(?:^|\s)@[^\s@]*$/u);
      if (!match || match.index === undefined) return current;
      const leadingSpace = match[0].startsWith(" ") ? " " : "";
      return `${current.slice(0, match.index)}${leadingSpace}@${resourceName(resource)} `;
    });
    setMentionOptions([]);
  };

  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(event.clipboardData.files);
    if (files.length === 0) return;
    event.preventDefault();
    void importFiles(files);
  };

  const handleDrop = (event: DragEvent<HTMLFormElement>) => {
    event.preventDefault();
    setDragActive(false);
    if (!selectedSessionId || activeRunId) return;
    void importFiles(Array.from(event.dataTransfer.files));
  };

  const removeMessageReference = async (
    message: AgentMessage,
    resourceId: string,
  ) => {
    if (!client.removeReference) return;
    const epoch = epochRef.current;
    try {
      await client.removeReference({
        taskId,
        sessionId: message.sessionId,
        messageId: message.id,
        resourceId,
      });
      if (epoch === epochRef.current) await reloadMessages(message.sessionId, epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    }
  };

  const createSession = async () => {
    const epoch = epochRef.current;
    setCreating(true);
    setError("");
    try {
      const session = await client.createSession({
        taskId,
        title: `${taskTitle} · PI`,
        mode: newSessionMode,
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
      setPermissionRequests([]);
      setPermissionGrants([]);
      setToolCalls([]);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setCreating(false);
    }
  };

  const sendPrompt = async (event: FormEvent) => {
    event.preventDefault();
    const originalDraft = draft;
    const message = draft.trim() || (selectedResources.length > 0 ? "请查看所附任务资源。" : "");
    const sessionId = selectedSessionRef.current;
    if (!message || !sessionId || activeRunId) return;
    const epoch = epochRef.current;
    setSending(true);
    setError("");
    setDraft("");
    try {
      const run = await client.sendPrompt({
        taskId,
        sessionId,
        message,
        resourceIds: selectedResources.map((resource) => resource.id),
      });
      if (epoch !== epochRef.current) return;
      setSelectedResources([]);
      if (!terminalRunsRef.current.has(run.id)) setActiveRunId(run.id);
      await reloadMessages(sessionId, epoch);
    } catch (reason) {
      if (epoch !== epochRef.current) return;
      setDraft(originalDraft);
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

  const resolvePermission = async (
    request: AgentPermissionRequest,
    decision: "allow" | "deny",
    scope: AgentPermissionScope | "",
  ) => {
    if (
      !client.resolvePermission ||
      request.taskId !== taskId ||
      request.sessionId !== selectedSessionRef.current ||
      resolvingPermissionsRef.current.has(request.id)
    ) {
      return;
    }
    const epoch = epochRef.current;
    resolvingPermissionsRef.current.add(request.id);
    setResolvingPermissions(new Set(resolvingPermissionsRef.current));
    setError("");
    try {
      await client.resolvePermission({
        taskId,
        sessionId: request.sessionId,
        requestId: request.id,
        decision,
        scope,
      });
      await reloadGovernance(request.sessionId, epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      resolvingPermissionsRef.current.delete(request.id);
      if (epoch === epochRef.current) {
        setResolvingPermissions(new Set(resolvingPermissionsRef.current));
      }
    }
  };

  const revokePermissionGrant = async (grant: AgentPermissionGrant) => {
    if (
      !client.revokePermissionGrant ||
      revokingGrantsRef.current.has(grant.id)
    ) {
      return;
    }
    const sessionId = selectedSessionRef.current;
    if (!sessionId) return;
    const epoch = epochRef.current;
    revokingGrantsRef.current.add(grant.id);
    setRevokingGrants(new Set(revokingGrantsRef.current));
    setError("");
    try {
      await client.revokePermissionGrant({ taskId, grantId: grant.id });
      await reloadGovernance(sessionId, epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      revokingGrantsRef.current.delete(grant.id);
      if (epoch === epochRef.current) {
        setRevokingGrants(new Set(revokingGrantsRef.current));
      }
    }
  };

  const activeSession = sessions.find(
    (session) => session.id === selectedSessionId,
  );
  const busy = Boolean(activeRunId);
  const hasGovernance = permissionRequests.length > 0 || permissionGrants.length > 0 || toolCalls.length > 0;

  return (
    <section className="task-agent-workbench" aria-label="PI 会话工作台">
      <aside className="agent-session-panel">
        <div className="agent-session-heading">
          <div>
            <span className="eyebrow">任务级 Session</span>
            <strong>PI 会话</strong>
          </div>
          <div className="agent-session-actions">
            <select
              aria-label="新会话模式"
              value={newSessionMode}
              disabled={creating || busy}
              onChange={(event) => setNewSessionMode(event.target.value as AgentSession["mode"])}
            >
              <option value="ask">Ask</option>
              <option value="plan">Plan</option>
              <option value="agent">Agent</option>
            </select>
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
        </div>
        <div className="agent-runtime-note">
          <Bot size={14} />
          <span>
            {client.runtimeMode() === "browser-mock"
              ? "浏览器模拟，不启动本机 PI"
              : "原生 PI RPC · BTask 权限门禁 · 无 Shell/Git"}
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
                  {session.mode} · {session.state} · {formatSessionTime(session.lastActiveAt)}
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
          {selectedSessionId && (
            <div className="agent-security-boundary" role="note">
              <ShieldAlert size={13} />
              <span>
                BTask 提供应用级软权限边界，并非操作系统沙箱；本阶段未开放 Shell 与 Git 执行。
              </span>
            </div>
          )}
          {permissionGrants.length > 0 && (
            <section className="agent-grant-list" aria-label="当前有效授权">
              <header>
                <strong>当前有效授权</strong>
                <span>{permissionGrants.length}</span>
              </header>
              {permissionGrants.map((grant) => (
                <div key={grant.id}>
                  <span>
                    <strong>{grant.capability}</strong>
                    <small>
                      {permissionScopeLabel(grant.scope)} · {grant.targetPattern}
                    </small>
                  </span>
                  {client.revokePermissionGrant && (
                    <button
                      type="button"
                      className="button secondary compact"
                      disabled={revokingGrants.has(grant.id)}
                      onClick={() => void revokePermissionGrant(grant)}
                    >
                      {revokingGrants.has(grant.id) ? "撤销中" : "撤销"}
                    </button>
                  )}
                </div>
              ))}
            </section>
          )}
          {permissionRequests.map((request) => (
            <AgentPermissionCard
              key={request.id}
              request={request}
              submitting={resolvingPermissions.has(request.id)}
              onResolve={(decision, scope) => {
                void resolvePermission(request, decision, scope);
              }}
            />
          ))}
          {toolCalls.map((tool) => (
            <AgentToolCard
              key={tool.id}
              tool={tool}
              loadOutput={
                tool.outputRef && client.readToolOutput
                  ? () => client.readToolOutput!({
                    taskId,
                    toolCallId: tool.id,
                  })
                  : undefined
              }
            />
          ))}
          {!loading && selectedSessionId && messages.length === 0 && !hasGovernance && (
            <div className="agent-chat-empty">
              <Bot size={26} />
              <strong>开始当前任务的第一轮对话</strong>
              <p>PI 可读取当前任务资源；Plan/Agent 仅可写入受控 artifacts。</p>
            </div>
          )}
          {!loading && !selectedSessionId && (
            <div className="agent-chat-empty">
              <MessageSquare size={25} />
              <strong>新建一个任务级 PI 会话</strong>
              <p>历史、附件和引用会持久化，并与其他任务严格隔离。</p>
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
              {message.references && message.references.length > 0 && (
                <div className="agent-message-references">
                  {message.references.map((reference) => {
                    const resource = [...resources, ...artifacts].find(
                      (candidate) => candidate.id === reference.resourceId,
                    ) ?? {
                      id: reference.resourceId,
                      taskId: reference.taskId,
                      targetType: reference.targetType,
                      kind: reference.kind,
                      sourceType: reference.sourceType,
                      logicalPath: reference.logicalPath,
                      mimeType: reference.mimeType,
                      byteSize: reference.byteSize ?? 0,
                      sha256: "",
                      immutable: reference.immutable,
                      readable: true,
                      createdAt: reference.createdAt,
                    };
                    return (
                      <span key={`${reference.resourceId}-${reference.method}`}>
                        <button type="button" onClick={() => void showPreview(resource)}>
                          {reference.mimeType?.startsWith("image/") ? <ImageIcon size={11} /> : <FileText size={11} />}
                          {resourceName(resource)}
                        </button>
                        {message.role === "user" && client.removeReference && (
                          <button
                            type="button"
                            aria-label={`删除引用 ${resourceName(resource)}`}
                            onClick={() => void removeMessageReference(message, reference.resourceId)}
                          >
                            <X size={10} />
                          </button>
                        )}
                      </span>
                    );
                  })}
                </div>
              )}
            </article>
          ))}
        </div>

        <form
          className={`agent-composer${dragActive ? " drag-active" : ""}`}
          onSubmit={(event) => void sendPrompt(event)}
          onDragEnter={(event) => {
            event.preventDefault();
            if (selectedSessionId && !busy) setDragActive(true);
          }}
          onDragOver={(event) => event.preventDefault()}
          onDragLeave={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragActive(false);
          }}
          onDrop={handleDrop}
        >
          <input
            ref={fileInputRef}
            type="file"
            multiple
            hidden
            onChange={(event) => {
              void importFiles(Array.from(event.target.files ?? []));
              event.currentTarget.value = "";
            }}
          />
          <div className="agent-composer-main">
            {selectedResources.length > 0 && (
              <div className="agent-selected-resources" aria-label="待发送引用">
                {selectedResources.map((resource) => (
                  <span key={resource.id}>
                    {resource.mimeType?.startsWith("image/") ? <ImageIcon size={11} /> : <FileText size={11} />}
                    {resourceName(resource)}
                    <button
                      type="button"
                      aria-label={`移除待发送引用 ${resourceName(resource)}`}
                      onClick={() => setSelectedResources((current) => current.filter((item) => item.id !== resource.id))}
                    >
                      <X size={10} />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <div className="agent-composer-input">
              <textarea
                value={draft}
                aria-label="发送给 PI 的消息"
                placeholder={
                  selectedSessionId
                    ? "输入消息，键入 @ 引用当前任务资源；可粘贴或拖放附件……"
                    : "请先新建 PI 会话"
                }
                disabled={!selectedSessionId || busy}
                onPaste={handlePaste}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    event.currentTarget.form?.requestSubmit();
                  }
                }}
              />
              {activeMentionQuery !== undefined && mentionOptions.length > 0 && (
                <div className="agent-mention-picker" role="listbox" aria-label="当前任务资源">
                  {mentionOptions.map((resource) => (
                    <button
                      type="button"
                      role="option"
                      aria-selected={selectedResources.some((item) => item.id === resource.id)}
                      key={resource.id}
                      onClick={() => selectMention(resource)}
                    >
                      <AtSign size={11} />
                      <span><strong>{resourceName(resource)}</strong><small>{resource.logicalPath}</small></span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
          <div className="agent-composer-actions">
            <button
              type="button"
              className="icon-button"
              aria-label="添加任务附件"
              title="添加任务附件"
              disabled={!selectedSessionId || busy || importing}
              onClick={() => fileInputRef.current?.click()}
            >
              {importing ? <LoaderCircle className="spin" size={14} /> : <Paperclip size={14} />}
            </button>
            <button
              type="submit"
              className="button primary compact"
              disabled={!selectedSessionId || (!draft.trim() && selectedResources.length === 0) || busy || sending}
            >
              {sending ? <LoaderCircle className="spin" size={14} /> : <Send size={14} />}
              发送
            </button>
          </div>
          {dragActive && <div className="agent-drop-overlay">松开以保存到当前任务附件</div>}
        </form>
      </div>

      <aside className="agent-resource-panel" aria-label="当前任务上下文和文件">
        <header>
          <div><FolderOpen size={15} /><span><strong>上下文与文件</strong><small>仅当前任务</small></span></div>
        </header>
        <div className="agent-resource-tabs">
          <button
            type="button"
            className={resourcePanel === "resources" ? "active" : ""}
            onClick={() => setResourcePanel("resources")}
          >资源 {resources.length}</button>
          <button
            type="button"
            className={resourcePanel === "artifacts" ? "active" : ""}
            onClick={() => setResourcePanel("artifacts")}
          >Artifacts {artifacts.length}</button>
        </div>
        <div className="agent-resource-list">
          {(resourcePanel === "resources" ? resources : artifacts).map((resource) => (
            <button
              type="button"
              key={resource.id}
              className={previewResource?.id === resource.id ? "active" : ""}
              onClick={() => void showPreview(resource)}
            >
              {resource.mimeType?.startsWith("image/") ? <ImageIcon size={13} /> : <FileText size={13} />}
              <span><strong>{resourceName(resource)}</strong><small>{resource.kind} · {formatBytes(resource.byteSize)}</small></span>
              {resource.proposalState && <em>{resource.proposalState}</em>}
            </button>
          ))}
          {(resourcePanel === "resources" ? resources : artifacts).length === 0 && (
            <p>当前任务暂无{resourcePanel === "resources" ? "可引用资源" : " artifacts"}。</p>
          )}
        </div>
        {previewResource && (
          <section className="agent-resource-preview">
            <header>
              <span><strong>{resourceName(previewResource)}</strong><small>{previewResource.logicalPath}</small></span>
              <div>
                {previewResource.targetType === "artifact" && client.openArtifact && (
                  <button
                    type="button"
                    aria-label="在系统中打开 artifact"
                    onClick={() => void client.openArtifact?.(taskId, previewResource.id).catch((reason) => setError(errorText(reason)))}
                  ><ExternalLink size={11} /></button>
                )}
                <button type="button" aria-label="关闭预览" onClick={() => { setPreview(undefined); setPreviewResource(undefined); }}><X size={11} /></button>
              </div>
            </header>
            {previewLoading && <div className="agent-preview-loading"><LoaderCircle className="spin" size={14} />正在预览…</div>}
            {preview?.kind === "image" && <img src={preview.content} alt={preview.name} />}
            {preview?.kind === "text" && <pre>{preview.content}</pre>}
          </section>
        )}
      </aside>
    </section>
  );
}
