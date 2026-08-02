import {
  AlertTriangle,
  Activity,
  ArrowRight,
  AtSign,
  Bot,
  Boxes,
  Check,
  ChevronsUpDown,
  Clock3,
  Equal,
  ExternalLink,
  File,
  FileText,
  Flag,
  FolderOpen,
  Gauge,
  GitBranch,
  Image as ImageIcon,
  Info,
  List,
  LoaderCircle,
  MessageSquare,
  Paperclip,
  Plus,
  RotateCcw,
  Send,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Square,
  Tag,
  Target,
  X,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
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
import { STATUS_META, type TaskPriority } from "../domain/task";
import { useWorkspaceStore } from "../store/workspace";
import type { PIThinkingEffort } from "../domain/engine";
import { AgentChangesPanel } from "./AgentChangesPanel";
import { AgentPermissionCard } from "./AgentPermissionCard";
import { AgentTerminalPanel } from "./AgentTerminalPanel";
import { AgentRunsPanel } from "./AgentRunsPanel";
import { AgentToolCard } from "./AgentToolCard";

interface TaskAgentWorkbenchProps {
  taskId: string;
  taskTitle: string;
  client?: AgentClient;
  onEditTask?(): void;
}

type ResourcePanel = "context" | "files" | "changes" | "runs" | "terminal";

type TimelineItem =
  | { type: "message"; id: string; at: string; sequence: number; message: AgentMessage }
  | { type: "permission"; id: string; at: string; sequence: number; request: AgentPermissionRequest }
  | { type: "tool"; id: string; at: string; sequence: number; tool: AgentToolCall };

const HISTORY_PAGE_SIZE = 60;

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

const SESSION_STATE_LABELS: Record<AgentSession["state"], string> = {
  created: "待启动",
  starting: "正在启动",
  idle: "空闲",
  running: "运行中",
  stopping: "正在停止",
  interrupted: "已中断",
  failed: "失败",
};

const MODE_LABELS: Record<AgentSession["mode"], string> = {
  ask: "Ask",
  plan: "Plan",
  agent: "Agent",
};

const PRIORITY_LABEL: Record<TaskPriority, string> = {
  low: "低",
  medium: "中",
  high: "高",
};

const MESSAGE_STATUS_LABELS: Record<AgentMessageStatus, string> = {
  pending: "已排队",
  streaming: "输出中",
  complete: "完成",
  error: "失败",
  cancelled: "已取消",
};

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

function mergeMessages(...groups: AgentMessage[][]): AgentMessage[] {
  const byID = new Map<string, AgentMessage>();
  for (const group of groups) {
    for (const message of group) byID.set(message.id, message);
  }
  return sortMessages(Array.from(byID.values()));
}

function isContextResource(resource: AgentResource): boolean {
  return resource.kind === "context" || resource.logicalPath.startsWith("context/");
}

function timelineTimestamp(value: string): number {
  const parsed = new Date(value).valueOf();
  return Number.isFinite(parsed) ? parsed : 0;
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
  onEditTask,
}: TaskAgentWorkbenchProps) {
  const piSettings = useWorkspaceStore((state) => state.piSettings);
  const updatePISettings = useWorkspaceStore((state) => state.updatePISettings);
  const taskStatus = useWorkspaceStore(
    (state) => state.tasks.find((task) => task.id === taskId)?.status ?? "inbox",
  );
  const taskPriority = useWorkspaceStore(
    (state) =>
      state.tasks.find((task) => task.id === taskId)?.priority ?? "medium",
  );
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [selectedSessionId, setSelectedSessionId] = useState("");
  const [collaborationMode, setCollaborationMode] = useState<
    "normal" | "plan" | "goal"
  >("normal");
  const [tokenMode, setTokenMode] = useState<"economy" | "full" | "delivery">(
    "full",
  );
  const [toolApprovalMode, setToolApprovalMode] = useState<
    "ask" | "auto" | "yolo"
  >("ask");
  const [intentMenuOpen, setIntentMenuOpen] = useState(false);
  const [profileMenuOpen, setProfileMenuOpen] = useState(false);
  const [goalDraft, setGoalDraft] = useState("");
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [historyCursor, setHistoryCursor] = useState("");
  const [hasOlderMessages, setHasOlderMessages] = useState(false);
  const [loadingOlderMessages, setLoadingOlderMessages] = useState(false);
  const [permissionRequests, setPermissionRequests] = useState<AgentPermissionRequest[]>([]);
  const [permissionGrants, setPermissionGrants] = useState<AgentPermissionGrant[]>([]);
  const [toolCalls, setToolCalls] = useState<AgentToolCall[]>([]);
  const [resolvingPermissions, setResolvingPermissions] = useState<Set<string>>(new Set());
  const [revokingGrants, setRevokingGrants] = useState<Set<string>>(new Set());
  const [draft, setDraft] = useState("");
  const [resources, setResources] = useState<AgentResource[]>([]);
  const [artifacts, setArtifacts] = useState<AgentResource[]>([]);
  const [mentionOptions, setMentionOptions] = useState<AgentResource[]>([]);
  const [selectedResources, setSelectedResources] = useState<AgentResource[]>([]);
  const [resourcePanel, setResourcePanel] = useState<ResourcePanel>("context");
  const [slashCommands, setSlashCommands] = useState<
    Array<{ name: string; description?: string }>
  >([]);
  const intentMenuRef = useRef<HTMLDivElement>(null);
  const profileMenuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!intentMenuOpen && !profileMenuOpen) return;
    const close = (event: PointerEvent) => {
      const target = event.target as Node;
      if (intentMenuRef.current?.contains(target)) return;
      if (profileMenuRef.current?.contains(target)) return;
      setIntentMenuOpen(false);
      setProfileMenuOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [intentMenuOpen, profileMenuOpen]);
  const [gitRefreshVersion, setGitRefreshVersion] = useState(0);
  const [runRefreshVersion, setRunRefreshVersion] = useState(0);
  const [preview, setPreview] = useState<AgentResourcePreview>();
  const [previewResource, setPreviewResource] = useState<AgentResource>();
  const [previewLoading, setPreviewLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [dragActive, setDragActive] = useState(false);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [submittingAction, setSubmittingAction] = useState<
    "" | "send" | "steer" | "follow_up"
  >("");
  const [stopping, setStopping] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [activeRunId, setActiveRunId] = useState("");
  const [queueCounts, setQueueCounts] = useState({ steering: 0, followUp: 0 });
  const [workspaceState, setWorkspaceState] = useState("读取中");
  const [error, setError] = useState("");
  const epochRef = useRef(0);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const messageListRef = useRef<HTMLDivElement>(null);
  const stickToBottomRef = useRef(true);
  const selectedSessionRef = useRef("");
  const resolvingPermissionsRef = useRef(new Set<string>());
  const revokingGrantsRef = useRef(new Set<string>());
  const terminalRunsRef = useRef(new Set<string>());
  const lastEventSequenceRef = useRef(new Map<string, number>());

  useEffect(() => {
    selectedSessionRef.current = selectedSessionId;
  }, [selectedSessionId]);

  const reloadMessages = useCallback(
    async (
      sessionId: string,
      epoch = epochRef.current,
      replace = false,
    ) => {
      if (!sessionId) {
        if (epoch === epochRef.current) {
          setMessages([]);
          setHistoryCursor("");
          setHasOlderMessages(false);
        }
        return;
      }
      const page = client.listHistoryPage
        ? await client.listHistoryPage({
          taskId,
          sessionId,
          cursor: "",
          limit: HISTORY_PAGE_SIZE,
        })
        : {
          messages: await client.listMessages(taskId, sessionId),
          nextCursor: undefined,
          hasMore: false,
        };
      if (
        epoch !== epochRef.current ||
        selectedSessionRef.current !== sessionId
      ) {
        return;
      }
      setMessages((current) =>
        replace ? sortMessages(page.messages) : mergeMessages(current, page.messages),
      );
      setHistoryCursor(page.nextCursor ?? "");
      setHasOlderMessages(page.hasMore);
    },
    [client, taskId],
  );

  const loadOlderMessages = useCallback(async () => {
    const sessionId = selectedSessionRef.current;
    if (
      !sessionId ||
      !historyCursor ||
      !hasOlderMessages ||
      loadingOlderMessages ||
      !client.listHistoryPage
    ) {
      return;
    }
    const epoch = epochRef.current;
    setLoadingOlderMessages(true);
    setError("");
    try {
      const page = await client.listHistoryPage({
        taskId,
        sessionId,
        cursor: historyCursor,
        limit: HISTORY_PAGE_SIZE,
      });
      if (
        epoch !== epochRef.current ||
        selectedSessionRef.current !== sessionId
      ) {
        return;
      }
      stickToBottomRef.current = false;
      setMessages((current) => mergeMessages(page.messages, current));
      setHistoryCursor(page.nextCursor ?? "");
      setHasOlderMessages(page.hasMore);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setLoadingOlderMessages(false);
    }
  }, [
    client,
    hasOlderMessages,
    historyCursor,
    loadingOlderMessages,
    taskId,
  ]);

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

  const reloadWorkspaceState = useCallback(
    async (epoch = epochRef.current) => {
      if (!client.getTaskGitStatus) {
        if (epoch === epochRef.current) setWorkspaceState("仅任务上下文");
        return;
      }
      try {
        const status = await client.getTaskGitStatus(taskId);
        if (epoch !== epochRef.current) return;
        if (!status.bound) {
          setWorkspaceState("仓库未绑定");
        } else if (status.binding.state === "ready" && !status.errorMessage) {
          setWorkspaceState(status.binding.branch ?? "worktree 就绪");
        } else {
          setWorkspaceState("worktree 异常");
        }
      } catch {
        if (epoch === epochRef.current) setWorkspaceState("worktree 不可用");
      }
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
    setHistoryCursor("");
    setHasOlderMessages(false);
    setLoadingOlderMessages(false);
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
    setResourcePanel("context");
    setGitRefreshVersion(0);
    setRunRefreshVersion(0);
    setDragActive(false);
    setActiveRunId("");
    setQueueCounts({ steering: 0, followUp: 0 });
    setWorkspaceState("读取中");
    setError("");
    setLoading(true);
    setCreating(false);
    setSubmittingAction("");
    setStopping(false);
    setRecovering(false);

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
      if (event.kind === "git.changed") {
        setGitRefreshVersion((current) => current + 1);
        void reloadWorkspaceState(epoch);
      }
      if (event.kind === "run.state") {
        setRunRefreshVersion((current) => current + 1);
      }
      if (event.kind === "queue.updated") {
        const steering = event.payload.steeringCount;
        const followUp = event.payload.followUpCount;
        setQueueCounts({
          steering: typeof steering === "number" ? steering : 0,
          followUp: typeof followUp === "number" ? followUp : 0,
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
              content: payloadString(event.payload, "content"),
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
          setSubmittingAction("");
          setStopping(false);
          setQueueCounts({ steering: 0, followUp: 0 });
          if (state === "failed" || state === "interrupted") {
            setError(
              payloadString(event.payload, "reason") || "PI 运行未完成",
            );
          }
        } else if (ACTIVE_RUN_STATES.has(state)) {
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
    void reloadWorkspaceState(epoch);

    return () => {
      epochRef.current += 1;
      unsubscribe();
    };
  }, [
    client,
    reloadGovernance,
    reloadMessages,
    reloadResources,
    reloadWorkspaceState,
    taskId,
  ]);

  useEffect(() => {
    if (!selectedSessionId) {
      setMessages([]);
      setHistoryCursor("");
      setHasOlderMessages(false);
      setPermissionRequests([]);
      setPermissionGrants([]);
      setToolCalls([]);
      return;
    }
    const epoch = epochRef.current;
    setMessages([]);
    setHistoryCursor("");
    setHasOlderMessages(false);
    setQueueCounts({ steering: 0, followUp: 0 });
    setLoading(true);
    setError("");
    void Promise.all([
      reloadMessages(selectedSessionId, epoch, true),
      reloadGovernance(selectedSessionId, epoch),
    ])
      .catch((reason) => {
        if (epoch === epochRef.current) setError(errorText(reason));
      })
      .finally(() => {
        if (epoch === epochRef.current) {
          setLoading(false);
          window.setTimeout(() => textareaRef.current?.focus(), 0);
        }
      });
  }, [reloadGovernance, reloadMessages, selectedSessionId]);

  useEffect(() => {
    const element = messageListRef.current;
    if (!element || !stickToBottomRef.current) return;
    element.scrollTop = element.scrollHeight;
  }, [messages, permissionRequests, toolCalls]);

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
    if (!selectedSessionId || stopping) return;
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
  const loadSlashCommands = async () => {
    if (!selectedSessionId || slashCommands.length > 0 || !client.sessionCommand) {
      return;
    }
    try {
      const result = await client.sessionCommand({
        taskId,
        sessionId: selectedSessionId,
        type: "get_commands",
        payload: {},
      });
      if (Array.isArray(result.commands)) {
        setSlashCommands(result.commands as Array<{ name: string; description?: string }>);
      }
    } catch {
      // 命令列表不可用时静默
    }
  };

  const slashQuery = draft.startsWith("/") ? draft.slice(1).trim() : "";
  const slashMatches = slashQuery
    ? slashCommands.filter(
        (command) =>
          command.name.includes(slashQuery) ||
          (command.description ?? "").includes(slashQuery),
      )
    : slashCommands;

  const insertSlashCommand = (name: string) => {
    const tokenEnd = draft.indexOf(" ") < 0 ? draft.length : draft.indexOf(" ");
    const remainder = draft.slice(tokenEnd).replace(/^\s+/, "");
    setDraft(remainder ? `/${name} ${remainder}` : `/${name} `);
    window.requestAnimationFrame(() => textareaRef.current?.focus());
  };

  const chooseTaskMode = (mode: "normal" | "plan" | "goal") => {
    setCollaborationMode(mode);
    setIntentMenuOpen(false);
    window.requestAnimationFrame(() => textareaRef.current?.focus());
  };

  const chooseTokenMode = (mode: "economy" | "full" | "delivery") => {
    setTokenMode(mode);
    setProfileMenuOpen(false);
    window.requestAnimationFrame(() => textareaRef.current?.focus());
  };

  const chooseApprovalMode = (mode: "ask" | "auto" | "yolo") => {
    setToolApprovalMode(mode);
    window.requestAnimationFrame(() => textareaRef.current?.focus());
  };

  const TaskModeIcon =
    collaborationMode === "plan"
      ? List
      : collaborationMode === "goal"
        ? Target
        : ArrowRight;
  const taskModeShortKey =
    collaborationMode === "plan"
      ? "计划"
      : collaborationMode === "goal"
        ? "目标"
        : "常规";
  const RuntimeProfileIcon =
    tokenMode === "economy" ? Gauge : tokenMode === "delivery" ? Flag : Equal;
  const runtimeProfileShortKey =
    tokenMode === "economy"
      ? "轻量"
      : tokenMode === "delivery"
        ? "交付"
        : "均衡";

  const createSession = async () => {
    const epoch = epochRef.current;
    setCreating(true);
    setError("");
    try {
      const session = await client.createSession({
        taskId,
        title: `${taskTitle} · PI`,
        mode:
          collaborationMode === "plan"
            ? "plan"
            : collaborationMode === "goal"
              ? "agent"
              : "ask",
        model: piSettings.model,
        thinkingLevel: piSettings.thinkingEffort,
        resourcePolicy: piSettings.resourcePolicy,
      });
      if (epoch !== epochRef.current) return;
      selectedSessionRef.current = session.id;
      setSessions((current) => [
        session,
        ...current.filter((candidate) => candidate.id !== session.id),
      ]);
      setSelectedSessionId(session.id);
      setMessages([]);
      setHistoryCursor("");
      setHasOlderMessages(false);
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
    setSubmittingAction("send");
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
      if (epoch === epochRef.current) setSubmittingAction("");
    }
  };

  const queuePrompt = async (behavior: "steer" | "follow_up") => {
    const originalDraft = draft;
    const message = draft.trim() ||
      (selectedResources.length > 0 ? "请查看所附任务资源。" : "");
    const sessionId = selectedSessionRef.current;
    const queue = behavior === "steer"
      ? client.steerPrompt
      : client.followUpPrompt;
    if (!message || !sessionId || !activeRunId) return;
    if (!queue) {
      setError(
        behavior === "steer"
          ? "当前客户端不支持 PI Steer"
          : "当前客户端不支持 PI Follow-up",
      );
      return;
    }
    const epoch = epochRef.current;
    setSubmittingAction(behavior);
    setError("");
    setDraft("");
    try {
      const queued = await queue({
        taskId,
        sessionId,
        message,
        resourceIds: selectedResources.map((resource) => resource.id),
      });
      if (epoch !== epochRef.current) return;
      setMessages((current) => mergeMessages(current, [queued]));
      setSelectedResources([]);
    } catch (reason) {
      if (epoch !== epochRef.current) return;
      setDraft(originalDraft);
      setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setSubmittingAction("");
    }
  };

  const resumeSession = async () => {
    const sessionId = selectedSessionRef.current;
    if (!sessionId || !client.resumeSession || activeRunId) return;
    const epoch = epochRef.current;
    setRecovering(true);
    setError("");
    try {
      const resumed = await client.resumeSession({ taskId, sessionId });
      if (epoch !== epochRef.current) return;
      setSessions((current) =>
        current.map((session) => session.id === resumed.id ? resumed : session),
      );
      await Promise.all([
        reloadMessages(sessionId, epoch),
        reloadGovernance(sessionId, epoch),
      ]);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setRecovering(false);
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

  const stopToolExecution = async (tool: AgentToolCall) => {
    if (!client.stopToolExecution) {
      setError("当前客户端不支持单独停止 Shell 工具");
      return;
    }
    try {
      await client.stopToolExecution({
        taskId,
        sessionId: tool.sessionId,
        runId: tool.runId,
        toolCallId: tool.id,
      });
    } catch (reason) {
      setError(errorText(reason));
    }
  };

  const activeSession = sessions.find(
    (session) => session.id === selectedSessionId,
  );
  const busy = Boolean(activeRunId);
  const canRecover = Boolean(
    activeSession &&
    ["created", "interrupted", "failed"].includes(activeSession.state),
  );
  const contextResources = resources.filter(isContextResource);
  const fileResources = [
    ...resources.filter((resource) => !isContextResource(resource)),
    ...artifacts,
  ];
  const visibleResources = resourcePanel === "context"
    ? contextResources
    : fileResources;
  const hasGovernance = permissionRequests.length > 0 ||
    permissionGrants.length > 0 ||
    toolCalls.length > 0;
  const timelineItems = useMemo<TimelineItem[]>(() => {
    const items: TimelineItem[] = [
      ...messages.map((message) => ({
        type: "message" as const,
        id: message.id,
        at: message.createdAt,
        sequence: message.sequence,
        message,
      })),
      ...permissionRequests.map((request, index) => ({
        type: "permission" as const,
        id: request.id,
        at: request.requestedAt,
        sequence: Number.MAX_SAFE_INTEGER - 20_000 + index,
        request,
      })),
      ...toolCalls.map((tool, index) => ({
        type: "tool" as const,
        id: tool.id,
        at: tool.startedAt ?? tool.finishedAt ?? "",
        sequence: Number.MAX_SAFE_INTEGER - 10_000 + index,
        tool,
      })),
    ];
    return items.sort((left, right) => {
      const time = timelineTimestamp(left.at) - timelineTimestamp(right.at);
      return time || left.sequence - right.sequence || left.id.localeCompare(right.id);
    });
  }, [messages, permissionRequests, toolCalls]);
  return (
    <section className="task-agent-workbench" aria-label="PI 会话工作台">
      <header className="agent-workbench-topbar">
        <div className="agent-workbench-identity">
          <span className="eyebrow">任务 Agent 工作台</span>
          <div className="agent-workbench-title-row">
            <span className={`status-chip status-${taskStatus}`}>
              {STATUS_META[taskStatus].label}
            </span>
            <span className={`manual-priority-chip priority-${taskPriority}`}>
              <Tag size={12} />
              {PRIORITY_LABEL[taskPriority]}
            </span>
            <strong title={taskTitle}>{taskTitle}</strong>
          </div>
        </div>
        <div className="agent-workbench-status" aria-label="会话状态">
          <span>
            <small>模式</small>
            <strong>
              {MODE_LABELS[
                activeSession?.mode ??
                  (collaborationMode === "plan"
                    ? "plan"
                    : collaborationMode === "goal"
                      ? "agent"
                      : "ask")
              ]}
            </strong>
          </span>
          <span title={workspaceState}>
            <small>工作区</small>
            <strong>{workspaceState}</strong>
          </span>
        </div>
        <div className="agent-workbench-actions">
          <label>
            <span>会话</span>
            <select
              aria-label="切换 PI 会话"
              value={selectedSessionId}
              disabled={sessions.length === 0 || busy}
              onChange={(event) => {
                selectedSessionRef.current = event.target.value;
                stickToBottomRef.current = true;
                setSelectedSessionId(event.target.value);
              }}
            >
              {sessions.length === 0 && <option value="">暂无会话</option>}
              {sessions.map((session) => (
                <option key={session.id} value={session.id}>{session.title}</option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="button secondary compact"
            disabled={creating || busy}
            onClick={() => void createSession()}
          >
            {creating ? <LoaderCircle className="spin" size={13} /> : <Plus size={13} />}
            新会话
          </button>
          {canRecover && (
            <button
              type="button"
              className="button secondary compact"
              disabled={recovering || busy || !client.resumeSession}
              onClick={() => void resumeSession()}
            >
              {recovering
                ? <LoaderCircle className="spin" size={13} />
                : <RotateCcw size={13} />}
              {recovering ? "恢复中" : "恢复"}
            </button>
          )}
          {busy && (
            <button
              type="button"
              className="button secondary compact agent-stop-button"
              onClick={() => void stopRun()}
              disabled={stopping}
            >
              {stopping
                ? <LoaderCircle className="spin" size={13} />
                : <Square size={12} />}
              {stopping ? "正在停止" : "停止"}
            </button>
          )}
          {onEditTask && (
            <button
              type="button"
              className="icon-button"
              aria-label="编辑任务基本信息"
              title="编辑任务基本信息"
              onClick={onEditTask}
            >
              <Info size={14} />
            </button>
          )}
          <div className="composer-modebar composer-modebar--approval" data-mode={toolApprovalMode}>
            <span className="composer-modebar__thumb" aria-hidden="true" />
            <button
              type="button"
              className={`composer-modebar__item composer-modebar__item--ask${toolApprovalMode === "ask" ? " composer-modebar__item--active" : ""}`}
              onClick={() => chooseApprovalMode("ask")}
              disabled={creating || busy}
              aria-pressed={toolApprovalMode === "ask"}
              title="需审批的工具调用会先询问；询问不是只读模式"
            >
              <Shield size={14} />
              <span>询问</span>
            </button>
            <button
              type="button"
              className={`composer-modebar__item composer-modebar__item--auto${toolApprovalMode === "auto" ? " composer-modebar__item--active" : ""}`}
              onClick={() => chooseApprovalMode("auto")}
              disabled={creating || busy}
              aria-pressed={toolApprovalMode === "auto"}
              title="自动执行，只在需要用户决定计划时询问"
            >
              <ShieldCheck size={14} />
              <span>自动</span>
            </button>
            <button
              type="button"
              className={`composer-modebar__item composer-modebar__item--yolo${toolApprovalMode === "yolo" ? " composer-modebar__item--active" : ""}`}
              onClick={() => chooseApprovalMode("yolo")}
              disabled={creating || busy}
              aria-pressed={toolApprovalMode === "yolo"}
              title="Yolo 批准会跳过普通工具权限提示"
            >
              <ShieldAlert size={14} />
              <span>Yolo</span>
            </button>
          </div>
          {collaborationMode === "goal" && (
            <label className="agent-goal-input">
              <span>目标</span>
              <input
                value={goalDraft}
                placeholder="请输入目标…"
                aria-label="目标说明"
                disabled={creating || busy}
                onChange={(event) => setGoalDraft(event.target.value)}
              />
            </label>
          )}
          {intentMenuOpen && (
            <div
              className="composer-access-menu composer-intent-menu"
              role="menu"
              aria-label="执行方式"
              ref={intentMenuRef}
            >
              <div className="composer-access-menu__label">执行方式</div>
              <button
                type="button"
                role="menuitemradio"
                aria-checked={collaborationMode === "normal"}
                className={`composer-access-menu__item composer-intent-menu__item${collaborationMode === "normal" ? " composer-access-menu__item--active" : ""}`}
                onClick={() => chooseTaskMode("normal")}
                disabled={creating || busy}
              >
                <ArrowRight size={16} />
                <span className="composer-access-menu__copy">
                  <span className="composer-access-menu__title">常规 · 边做边推进</span>
                  <span className="composer-access-menu__desc">边分析边执行，适合明确的日常任务。</span>
                </span>
                {collaborationMode === "normal" && <Check size={16} />}
              </button>
              <button
                type="button"
                role="menuitemradio"
                aria-checked={collaborationMode === "plan"}
                className={`composer-access-menu__item composer-intent-menu__item${collaborationMode === "plan" ? " composer-access-menu__item--active" : ""}`}
                onClick={() => chooseTaskMode("plan")}
                disabled={creating || busy}
              >
                <List size={16} />
                <span className="composer-access-menu__copy">
                  <span className="composer-access-menu__title">计划 · 确认后执行</span>
                  <span className="composer-access-menu__desc">先产出计划；工具是否执行仍由当前权限与沙箱决定。</span>
                </span>
                {collaborationMode === "plan" && <Check size={16} />}
              </button>
              <button
                type="button"
                role="menuitemradio"
                aria-checked={collaborationMode === "goal"}
                className={`composer-access-menu__item composer-intent-menu__item${collaborationMode === "goal" ? " composer-access-menu__item--active" : ""}`}
                onClick={() => chooseTaskMode("goal")}
                disabled={creating || busy}
              >
                <Target size={16} />
                <span className="composer-access-menu__copy">
                  <span className="composer-access-menu__title">目标 · 持续推进</span>
                  <span className="composer-access-menu__desc">
                    {goalDraft || "请输入目标…"}
                  </span>
                </span>
                {collaborationMode === "goal" && <Check size={16} />}
              </button>
              {collaborationMode === "goal" && goalDraft && (
                <button
                  type="button"
                  className="composer-intent-menu__stop"
                  onClick={() => {
                    setGoalDraft("");
                    setIntentMenuOpen(false);
                  }}
                  disabled={creating || busy}
                >
                  结束目标
                </button>
              )}
            </div>
          )}
          {profileMenuOpen && (
            <div className="composer-access-menu composer-profile-menu" role="menu" aria-label="工作模式" ref={profileMenuRef}>
              <div className="composer-access-menu__label">工作模式</div>
              {([
                ["economy", Gauge, "轻量 · 快速省用量", "少上下文 · 工具按需启用"],
                ["full", Equal, "均衡 · 日常通用", "完整工具 · 模型自主执行"],
                ["delivery", Flag, "交付 · 完整验证", "强制验收 · 复查验证"],
              ] as const).map(([profile, Icon, title, desc]) => (
                <button
                  key={profile}
                  type="button"
                  role="menuitemradio"
                  className={`composer-access-menu__item composer-profile-menu__item${tokenMode === profile ? " composer-access-menu__item--active" : ""}`}
                  onClick={() => chooseTokenMode(profile)}
                  disabled={creating || busy}
                  title={desc}
                  aria-checked={tokenMode === profile}
                >
                  <Icon size={16} strokeWidth={1.75} />
                  <span className="composer-access-menu__copy">
                    <span className="composer-access-menu__title">{title}</span>
                    <span className="composer-access-menu__desc">{desc}</span>
                  </span>
                  {tokenMode === profile && <Check size={15} />}
                </button>
              ))}
            </div>
          )}
        </div>
      </header>

      <div className="agent-workbench-body">
        <aside className="agent-session-panel">
        <div className="agent-session-heading">
          <div>
            <span className="eyebrow">当前任务独立</span>
            <strong>会话历史</strong>
          </div>
          <span className="agent-session-count">{sessions.length}</span>
        </div>
        <div className="agent-runtime-note">
          <Bot size={14} />
          <span>
            {client.runtimeMode() === "browser-mock"
              ? "浏览器模拟，不启动本机 PI"
              : "当前 taskId 独立 Session、资源、worktree 与权限"}
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
                stickToBottomRef.current = true;
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
            <span>
              {activeSession
                ? `${MODE_LABELS[activeSession.mode]} · ${activeSession.thinkingLevel || "默认思考级别"} · ${SESSION_STATE_LABELS[activeSession.state]}`
                : "会话数据只属于当前任务，不会写入任务状态快照。"}
            </span>
          </div>
          {(queueCounts.steering > 0 || queueCounts.followUp > 0) && (
            <span className="agent-queue-counts" role="status">
              引导 {queueCounts.steering} · 后续 {queueCounts.followUp}
            </span>
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

        <div
          ref={messageListRef}
          className="agent-message-list"
          aria-live="polite"
          onScroll={(event) => {
            const element = event.currentTarget;
            stickToBottomRef.current =
              element.scrollHeight - element.scrollTop - element.clientHeight < 48;
          }}
        >
          {loading && (
            <div className="agent-loading">
              <LoaderCircle className="spin" size={17} />
              正在读取会话…
            </div>
          )}
          {!loading && hasOlderMessages && (
            <button
              type="button"
              className="agent-load-history"
              disabled={loadingOlderMessages}
              onClick={() => void loadOlderMessages()}
            >
              {loadingOlderMessages
                ? <LoaderCircle className="spin" size={12} />
                : <Clock3 size={12} />}
              {loadingOlderMessages ? "正在读取更早消息" : "加载更早消息"}
            </button>
          )}
          {selectedSessionId && (
            <div className="agent-security-boundary" role="note">
              <ShieldAlert size={13} />
              <span>
                BTask 提供应用级软权限边界，并非操作系统沙箱；仅 Agent + development 可修改任务 worktree 或申请 Shell，Git 推送、合并与 PR 未开放。
              </span>
            </div>
          )}
          {canRecover && activeSession?.errorMessage && (
            <div className="agent-recovery-card" role="status">
              <RotateCcw size={14} />
              <span>
                <strong>当前会话需要恢复</strong>
                <small>{activeSession.errorMessage}</small>
              </span>
              <button
                type="button"
                className="button secondary compact"
                disabled={recovering || !client.resumeSession}
                onClick={() => void resumeSession()}
              >
                {recovering ? "恢复中" : "恢复会话"}
              </button>
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
          {!loading && selectedSessionId && messages.length === 0 && !hasGovernance && (
            <div className="agent-chat-empty">
              <Bot size={26} />
              <strong>开始当前任务的第一轮对话</strong>
              <p>PI 可读取当前任务资源与已绑定 worktree；Plan 可写 artifacts，Agent 在开发中任务可申请受控开发工具。</p>
            </div>
          )}
          {!loading && !selectedSessionId && (
            <div className="agent-chat-empty">
              <MessageSquare size={25} />
              <strong>新建一个任务级 PI 会话</strong>
              <p>历史、附件和引用会持久化，并与其他任务严格隔离。</p>
            </div>
          )}
          {timelineItems.map((item) => {
            if (item.type === "permission") {
              return (
                <AgentPermissionCard
                  key={`permission-${item.id}`}
                  request={item.request}
                  submitting={resolvingPermissions.has(item.request.id)}
                  onResolve={(decision, scope) => {
                    void resolvePermission(item.request, decision, scope);
                  }}
                />
              );
            }
            if (item.type === "tool") {
              return (
                <AgentToolCard
                  key={`tool-${item.id}`}
                  tool={item.tool}
                  loadOutput={
                    item.tool.outputRef && client.readToolOutput
                      ? () => client.readToolOutput!({
                        taskId,
                        toolCallId: item.tool.id,
                      })
                      : undefined
                  }
                  onStop={
                    item.tool.toolName === "btask_shell" && item.tool.state === "running"
                      ? () => stopToolExecution(item.tool)
                      : undefined
                  }
                />
              );
            }
            const message = item.message;
            return (
              <article
                key={`message-${message.id}`}
                className={`agent-message agent-message-${message.role}`}
              >
                <header>
                  <strong>
                    {message.role === "user"
                      ? "你"
                      : message.role === "system"
                        ? "系统"
                        : "PI"}
                  </strong>
                  <span>{MESSAGE_STATUS_LABELS[message.status]}</span>
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
                            {reference.mimeType?.startsWith("image/")
                              ? <ImageIcon size={11} />
                              : <FileText size={11} />}
                            {resourceName(resource)}
                          </button>
                          {message.role === "user" && client.removeReference && (
                            <button
                              type="button"
                              aria-label={`删除引用 ${resourceName(resource)}`}
                              onClick={() =>
                                void removeMessageReference(message, reference.resourceId)}
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
            );
          })}
        </div>

        <form
          className={`agent-composer${dragActive ? " drag-active" : ""}`}
          onSubmit={(event) => void sendPrompt(event)}
          onDragEnter={(event) => {
            event.preventDefault();
            if (selectedSessionId && !stopping) setDragActive(true);
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
                    <Paperclip size={11} />
                    {resourceName(resource)}
                    <button
                      type="button"
                      aria-label={`移除引用 ${resourceName(resource)}`}
                      onClick={() =>
                        setSelectedResources((current) =>
                          current.filter((item) => item.id !== resource.id),
                        )
                      }
                    >
                      <X size={10} />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <div className="agent-composer-input">
              <textarea
                ref={textareaRef}
                value={draft}
                aria-label="发送给 PI 的消息"
                aria-describedby="agent-composer-shortcuts"
                placeholder={
                  selectedSessionId
                    ? busy
                      ? "运行中：输入明确的 Steer 引导或 Follow-up 后续消息……"
                      : "输入消息，键入 @ 引用当前任务资源；键入 / 打开会话命令……"
                    : "请先新建 PI 会话"
                }
                disabled={!selectedSessionId || stopping || recovering}
                onPaste={handlePaste}
                onChange={(event) => {
                  const value = event.target.value;
                  setDraft(value);
                  if (value.startsWith("/")) void loadSlashCommands();
                }}
                onKeyDown={(event) => {
                  if (
                    busy &&
                    event.key === "Enter" &&
                    (event.metaKey || event.ctrlKey)
                  ) {
                    event.preventDefault();
                    void queuePrompt("steer");
                  } else if (busy && event.key === "Enter" && event.altKey) {
                    event.preventDefault();
                    void queuePrompt("follow_up");
                  } else if (!busy && event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    event.currentTarget.form?.requestSubmit();
                  }
                }}
              />
              {draft.startsWith("/") && slashMatches.length > 0 && (
                <div className="agent-slash-picker" role="listbox" aria-label="会话命令">
                  {slashMatches.map((command) => (
                    <button
                      type="button"
                      role="option"
                      key={command.name}
                      onClick={() => insertSlashCommand(command.name)}
                    >
                      <span>
                        <strong>/{command.name}</strong>
                        {command.description && <small>{command.description}</small>}
                      </span>
                    </button>
                  ))}
                </div>
              )}
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
              disabled={
                !selectedSessionId ||
                importing ||
                stopping ||
                Boolean(submittingAction)
              }
              onClick={() => fileInputRef.current?.click()}
            >
              {importing ? <LoaderCircle className="spin" size={14} /> : <Paperclip size={14} />}
            </button>
            {busy ? (
              <>
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={
                    (!draft.trim() && selectedResources.length === 0) ||
                    Boolean(submittingAction) ||
                    stopping ||
                    !client.steerPrompt
                  }
                  onClick={() => void queuePrompt("steer")}
                >
                  {submittingAction === "steer"
                    ? <LoaderCircle className="spin" size={13} />
                    : <Send size={13} />}
                  Steer 引导
                </button>
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={
                    (!draft.trim() && selectedResources.length === 0) ||
                    Boolean(submittingAction) ||
                    stopping ||
                    !client.followUpPrompt
                  }
                  onClick={() => void queuePrompt("follow_up")}
                >
                  {submittingAction === "follow_up"
                    ? <LoaderCircle className="spin" size={13} />
                    : <Clock3 size={13} />}
                  Follow-up 后续
                </button>
              </>
            ) : (
              <button
                type="submit"
                className="button primary compact"
                disabled={
                  !selectedSessionId ||
                  (!draft.trim() && selectedResources.length === 0) ||
                  Boolean(submittingAction)
                }
              >
                {submittingAction === "send"
                  ? <LoaderCircle className="spin" size={14} />
                  : <Send size={14} />}
                发送
              </button>
            )}
          </div>
          <small id="agent-composer-shortcuts" className="agent-composer-shortcuts">
            {busy
              ? "⌘/Ctrl + Enter：Steer · Alt + Enter：Follow-up · Enter：换行"
              : "Enter：发送 · Shift + Enter：换行"}
          </small>
          {dragActive && <div className="agent-drop-overlay">松开以保存到当前任务附件</div>}
        </form>
      </div>

      <aside className="agent-resource-panel" aria-label="当前任务上下文和文件">
        <header>
          <div>
            {resourcePanel === "changes" ? (
              <GitBranch size={15} />
            ) : resourcePanel === "runs" ? (
              <Activity size={15} />
            ) : resourcePanel === "context" ? (
              <Boxes size={15} />
            ) : (
              <FolderOpen size={15} />
            )}
            <span>
              <strong>
                {resourcePanel === "changes"
                  ? "Git 变更"
                  : resourcePanel === "runs"
                    ? "运行历史"
                    : resourcePanel === "context"
                      ? "任务上下文"
                      : "任务文件"}
              </strong>
              <small>仅当前任务</small>
            </span>
          </div>
        </header>
        <div className="agent-resource-tabs">
          <button
            type="button"
            className={resourcePanel === "context" ? "active" : ""}
            onClick={() => setResourcePanel("context")}
          >
            上下文 {contextResources.length}
          </button>
          <button
            type="button"
            className={resourcePanel === "files" ? "active" : ""}
            onClick={() => setResourcePanel("files")}
          >
            文件 {fileResources.length}
          </button>
          <button
            type="button"
            className={resourcePanel === "changes" ? "active" : ""}
            onClick={() => setResourcePanel("changes")}
          >
            变更
          </button>
          <button
            type="button"
            className={resourcePanel === "runs" ? "active" : ""}
            onClick={() => setResourcePanel("runs")}
          >
            运行
          </button>
          <button
            type="button"
            className={resourcePanel === "terminal" ? "active" : ""}
            onClick={() => setResourcePanel("terminal")}
          >
            终端
          </button>
        </div>
        {(resourcePanel === "context" || resourcePanel === "files") && (
          <div className="agent-resource-list">
            {visibleResources.map((resource) => (
              <button
                type="button"
                key={resource.id}
                className={previewResource?.id === resource.id ? "active" : ""}
                onClick={() => void showPreview(resource)}
              >
                {resource.mimeType?.startsWith("image/") ? (
                  <ImageIcon size={13} />
                ) : (
                  <File size={13} />
                )}
                <span>
                  <strong>{resourceName(resource)}</strong>
                  <small>{resource.kind} · {formatBytes(resource.byteSize)}</small>
                </span>
                {resource.proposalState && <em>{resource.proposalState}</em>}
              </button>
            ))}
            {visibleResources.length === 0 && (
              <p>
                当前任务暂无{resourcePanel === "context" ? "上下文资源" : "附件或 artifacts"}。
              </p>
            )}
          </div>
        )}
        {resourcePanel === "changes" && (
          <AgentChangesPanel
            taskId={taskId}
            taskStatus={taskStatus}
            client={client}
            refreshVersion={gitRefreshVersion}
          />
        )}
        {resourcePanel === "runs" && (
          <AgentRunsPanel
            taskId={taskId}
            sessionId={selectedSessionId}
            client={client}
            refreshVersion={runRefreshVersion}
          />
        )}
        {resourcePanel === "terminal" && (
          <AgentTerminalPanel
            taskId={taskId}
            sessionId={selectedSessionId}
            client={client}
            onError={(message) => setError(message)}
          />
        )}
        {(resourcePanel === "context" || resourcePanel === "files") && previewResource && (
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
      </div>
    </section>
  );
}
