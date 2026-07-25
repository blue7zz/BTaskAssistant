import {
  ArrowRight,
  Bot,
  Check,
  CheckCircle2,
  CloudDownload,
  EyeOff,
  FolderKanban,
  KeyRound,
  Link2,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  SearchCheck,
  Server,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type {
  CandidateDecision,
  CollectionCandidate,
  PlaneProject,
  PlaneSettings,
} from "../domain/collection";
import {
  analyzePlaneCandidate,
  collectPlaneWorkItems,
  hasPlaneToken,
  setupPlaneConnection,
  testPlaneConnection,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";

type BusyAction =
  | "connect"
  | "test"
  | "collect"
  | "analyze";

const DECISION_LABELS: Record<CandidateDecision, string> = {
  pending: "待确认",
  accepted: "已转换",
  ignored: "已忽略",
};

function formatDate(value?: string): string {
  if (!value) return "尚未同步";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function workspaceAddress(settings: PlaneSettings): string {
  const baseUrl = settings.baseUrl.trim().replace(/\/+$/, "");
  const workspaceSlug = settings.workspaceSlug.trim();
  if (!baseUrl || !workspaceSlug) return baseUrl;
  try {
    const parsed = new URL(baseUrl);
    const firstSegment = parsed.pathname.split("/").filter(Boolean)[0];
    if (firstSegment === workspaceSlug) return `${baseUrl}/`;
    if (parsed.hostname === "api.plane.so") {
      parsed.hostname = "app.plane.so";
      return `${parsed.origin}/${workspaceSlug}/`;
    }
  } catch {
    // The backend will return the authoritative validation error on connect.
  }
  return `${baseUrl}/${workspaceSlug}/`;
}

function CandidateCard({
  candidate,
  selected,
  onSelect,
}: {
  candidate: CollectionCandidate;
  selected: boolean;
  onSelect(): void;
}) {
  return (
    <button
      type="button"
      className={`candidate-card ${selected ? "selected" : ""}`}
      onClick={onSelect}
    >
      <div className="candidate-card-top">
        <span>{candidate.externalKey}</span>
        <small>{candidate.stateName || "未设置状态"}</small>
      </div>
      <strong>{candidate.title}</strong>
      <p>{candidate.descriptionMarkdown || "Plane 中没有填写描述"}</p>
      <div className="candidate-card-meta">
        <span className={`priority priority-${candidate.priority}`}>
          {candidate.priority}
        </span>
        <span>{candidate.analysis.mode === "pi" ? "PI 已提炼" : "规则整理"}</span>
      </div>
    </button>
  );
}

interface PlaneCollectorProps {
  onSuccess(message: string): void;
  onError(error: unknown): void;
  onOpenTask(taskID: string): void;
}

export function PlaneCollector({
  onSuccess,
  onError,
  onOpenTask,
}: PlaneCollectorProps) {
  const settings = useWorkspaceStore((state) => state.planeSettings);
  const candidates = useWorkspaceStore(
    (state) => state.collectionCandidates,
  );
  const updateSettings = useWorkspaceStore(
    (state) => state.updatePlaneSettings,
  );
  const ingestCandidates = useWorkspaceStore(
    (state) => state.ingestPlaneCandidates,
  );
  const applyAnalysis = useWorkspaceStore(
    (state) => state.applyCandidateAnalysis,
  );
  const updateDraft = useWorkspaceStore(
    (state) => state.updateCandidateDraft,
  );
  const acceptCandidate = useWorkspaceStore(
    (state) => state.acceptCandidate,
  );
  const setIgnored = useWorkspaceStore(
    (state) => state.setCandidateIgnored,
  );

  const [token, setToken] = useState("");
  const [tokenStored, setTokenStored] = useState(false);
  const [busy, setBusy] = useState<BusyAction>();
  const [projects, setProjects] = useState<PlaneProject[]>([]);
  const [serviceAddress, setServiceAddress] = useState(() =>
    workspaceAddress(settings),
  );
  const [connectionReady, setConnectionReady] = useState(
    Boolean(settings.baseUrl && settings.workspaceSlug),
  );
  const [filter, setFilter] = useState<CandidateDecision>("pending");
  const [selectedID, setSelectedID] = useState<string>();
  const [connectionMessage, setConnectionMessage] = useState("");

  const filteredCandidates = useMemo(
    () => candidates.filter((candidate) => candidate.decision === filter),
    [candidates, filter],
  );
  const selected =
    filteredCandidates.find((candidate) => candidate.id === selectedID) ??
    filteredCandidates[0];
  const canUseStoredToken =
    tokenStored &&
    serviceAddress.trim().replace(/\/+$/, "") ===
      workspaceAddress(settings).replace(/\/+$/, "");
  const projectOptions = useMemo(() => {
    if (
      !settings.projectId ||
      projects.some((project) => project.id === settings.projectId)
    ) {
      return projects;
    }
    return [
      {
        id: settings.projectId,
        name: settings.projectName || settings.projectId,
        identifier: settings.projectIdentifier ?? "",
      },
      ...projects,
    ];
  }, [
    projects,
    settings.projectId,
    settings.projectIdentifier,
    settings.projectName,
  ]);

  useEffect(() => {
    if (!settings.baseUrl || !settings.workspaceSlug) {
      setTokenStored(false);
      return;
    }
    hasPlaneToken(settings)
      .then(setTokenStored)
      .catch(() => setTokenStored(false));
  }, [settings.baseUrl, settings.workspaceSlug]);

  useEffect(() => {
    if (selected && selected.id !== selectedID) {
      setSelectedID(selected.id);
    }
  }, [selected, selectedID]);

  const run = async (
    action: BusyAction,
    operation: () => Promise<void>,
  ) => {
    setBusy(action);
    try {
      await operation();
    } catch (error) {
      onError(error);
    } finally {
      setBusy(undefined);
    }
  };

  const connect = () =>
    run("connect", async () => {
      const setup = await setupPlaneConnection(serviceAddress, token);
      const discovered = setup.projects;
      if (discovered.length === 0) {
        throw new Error("连接成功，但这个工作区中没有可访问的项目");
      }
      setProjects(discovered);
      const sameConnection =
        setup.baseUrl === settings.baseUrl &&
        setup.workspaceSlug === settings.workspaceSlug;
      const selected =
        (sameConnection
          ? discovered.find(
              (project) => project.id === settings.projectId,
            ) ??
            discovered.find(
              (project) =>
                project.name.toLocaleLowerCase() ===
                settings.projectName.trim().toLocaleLowerCase(),
            )
          : undefined) ??
        (discovered.length === 1 ? discovered[0] : undefined);
      updateSettings({
        baseUrl: setup.baseUrl,
        workspaceSlug: setup.workspaceSlug,
        projectId: selected?.id ?? "",
        projectName: selected?.name ?? "",
        projectIdentifier: selected?.identifier ?? "",
      });
      setServiceAddress(
        workspaceAddress({
          ...settings,
          baseUrl: setup.baseUrl,
          workspaceSlug: setup.workspaceSlug,
        }),
      );
      setToken("");
      setTokenStored(true);
      setConnectionReady(true);
      const message =
        discovered.length === 1
          ? `已发现项目 ${discovered[0].identifier || discovered[0].name}，已自动选中。`
          : `已连接工作区 ${setup.workspaceSlug}，发现 ${discovered.length} 个项目，请选择。`;
      setConnectionMessage(message);
      onSuccess(`Plane 已连接；${message}`);
    });

  const testConnection = () =>
    run("test", async () => {
      const result = await testPlaneConnection(settings);
      setConnectionMessage(result.message);
      onSuccess(result.message);
    });

  const collect = () =>
    run("collect", async () => {
      const payloads = await collectPlaneWorkItems(settings);
      const pendingCount = ingestCandidates(payloads);
      setFilter("pending");
      setConnectionMessage(
        `已读取 ${payloads.length} 条 Plane 工作项，其中 ${pendingCount} 条待确认。`,
      );
      onSuccess("Plane 收集完成；没有自动创建正式任务");
    });

  const analyze = (candidate: CollectionCandidate) =>
    run("analyze", async () => {
      const analysis = await analyzePlaneCandidate(
        candidate.sourceMarkdown,
      );
      applyAnalysis(candidate.id, analysis);
      onSuccess("PI 已生成提炼候选；仍需你确认后才能创建任务");
    });

  const accept = (candidate: CollectionCandidate) => {
    try {
      const taskID = acceptCandidate(candidate.id);
      onSuccess("候选已由你确认并转换为正式任务");
      onOpenTask(taskID);
    } catch (error) {
      onError(error);
    }
  };

  return (
    <section className="collector-page">
      <header className="collector-hero">
        <div>
          <span className="eyebrow">外部任务入口</span>
          <h2>Plane 收集箱</h2>
          <p>
            固定脚本负责读取、分页、去重和保留原文；PI
            只生成提炼候选。只有你点击确认，候选才会进入正式任务池。
          </p>
        </div>
        <div className="collector-trust">
          <ShieldCheck size={20} />
          <div>
            <strong>人工确认门禁</strong>
            <span>PAT 不写入 SQLite</span>
          </div>
        </div>
      </header>

      <div className="collector-settings">
        <div className="settings-heading">
          <div className="settings-icon">
            <Server size={18} />
          </div>
          <div>
            <strong>Plane 连接</strong>
            <span>支持 Plane Cloud 和启用 HTTPS 的自托管实例</span>
          </div>
        </div>
        <div className="collector-settings-grid">
          <label className="field">
            <span>
              服务地址
              <small>粘贴浏览器里的工作区地址</small>
            </span>
            <input
              value={serviceAddress}
              onChange={(event) => {
                setServiceAddress(event.target.value);
                setConnectionReady(false);
                setProjects([]);
                setConnectionMessage("");
              }}
              placeholder="https://plane.example.com/my-team/"
            />
          </label>
          <label className="field">
            <span>
              Access Token
              <small>
                {canUseStoredToken
                  ? "系统凭据库中已有令牌，可留空"
                  : "连接成功后保存到系统凭据库"}
              </small>
            </span>
            <div className="credential-input">
              <KeyRound size={16} />
              <input
                type="password"
                value={token}
                autoComplete="off"
                onChange={(event) => setToken(event.target.value)}
                placeholder={
                  canUseStoredToken
                    ? "已有令牌，需要替换时再输入"
                    : "plane_api_…"
                }
              />
            </div>
          </label>
        </div>
        <div className="connection-action-row">
          <span>
            工作区会从地址自动识别，项目会在连接成功后列出；无需填写 slug 或 UUID。
          </span>
          <button
            type="button"
            className="button secondary"
            disabled={
              Boolean(busy) ||
              !serviceAddress.trim() ||
              (!token.trim() && !canUseStoredToken)
            }
            onClick={connect}
          >
            {busy === "connect" ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <Link2 size={16} />
            )}
            {connectionReady ? "重新连接并加载选项" : "连接并加载选项"}
          </button>
        </div>
        {connectionReady && (
          <div className="connection-options">
            <div className="workspace-option">
              <FolderKanban size={17} />
              <div>
                <span>已识别工作区</span>
                <strong>{settings.workspaceSlug}</strong>
              </div>
            </div>
            <label className="field">
              <span>
                收集项目
                <small>从可访问项目中选择</small>
              </span>
              <select
                value={settings.projectId}
                onChange={(event) => {
                  const project = projectOptions.find(
                    (item) => item.id === event.target.value,
                  );
                  updateSettings({
                    projectId: project?.id ?? "",
                    projectName: project?.name ?? "",
                    projectIdentifier: project?.identifier ?? "",
                  });
                  setConnectionMessage("");
                }}
              >
                <option value="">请选择 Plane 项目</option>
                {projectOptions.map((project) => (
                  <option key={project.id} value={project.id}>
                    {project.identifier
                      ? `${project.identifier} · ${project.name}`
                      : project.name}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className="button secondary"
              disabled={
                Boolean(busy) || !tokenStored || !settings.projectId
              }
              onClick={testConnection}
            >
              {busy === "test" ? (
                <LoaderCircle className="spin" size={16} />
              ) : (
                <SearchCheck size={16} />
              )}
              测试连接
            </button>
            <button
              type="button"
              className="button primary"
              disabled={
                Boolean(busy) || !tokenStored || !settings.projectId
              }
              onClick={collect}
            >
              {busy === "collect" ? (
                <LoaderCircle className="spin" size={16} />
              ) : (
                <CloudDownload size={16} />
              )}
              从 Plane 收集
            </button>
          </div>
        )}
        <div className="sync-caption">
          <span>
            <RefreshCw size={13} />
            上次同步：{formatDate(settings.lastCollectedAt)}
          </span>
          {connectionMessage && <strong>{connectionMessage}</strong>}
        </div>
      </div>

      <div className="candidate-tabs">
        {(["pending", "accepted", "ignored"] as CandidateDecision[]).map(
          (decision) => (
            <button
              type="button"
              key={decision}
              className={filter === decision ? "active" : ""}
              onClick={() => {
                setFilter(decision);
                setSelectedID(undefined);
              }}
            >
              {DECISION_LABELS[decision]}
              <span>
                {
                  candidates.filter(
                    (candidate) => candidate.decision === decision,
                  ).length
                }
              </span>
            </button>
          ),
        )}
      </div>

      {filteredCandidates.length === 0 ? (
        <div className="collector-empty">
          <CloudDownload size={30} />
          <strong>
            {filter === "pending"
              ? "还没有待确认候选"
              : `没有${DECISION_LABELS[filter]}的候选`}
          </strong>
          <p>
            {filter === "pending"
              ? "配置 Plane 后点击“从 Plane 收集”。脚本不会自动创建任务。"
              : "切换上方分类查看其他候选。"}
          </p>
        </div>
      ) : (
        <div className="candidate-workspace">
          <aside className="candidate-list">
            {filteredCandidates.map((candidate) => (
              <CandidateCard
                key={candidate.id}
                candidate={candidate}
                selected={candidate.id === selected?.id}
                onSelect={() => setSelectedID(candidate.id)}
              />
            ))}
          </aside>

          {selected && (
            <article className="candidate-detail">
              <header>
                <div>
                  <span className="eyebrow">
                    {selected.externalKey} · {selected.stateName}
                  </span>
                  <h3>候选任务确认</h3>
                </div>
                <span className={`analysis-badge ${selected.analysis.mode}`}>
                  {selected.analysis.mode === "pi" ? (
                    <Bot size={14} />
                  ) : (
                    <Sparkles size={14} />
                  )}
                  {selected.analysis.mode === "pi" ? "PI 候选" : "规则整理"}
                </span>
              </header>

              <div className="candidate-source-strip">
                <Link2 size={15} />
                <span>
                  原始 Plane 数据会作为独立来源保留，下面的标题和正文只是待确认草稿。
                </span>
              </div>

              <label className="field">
                <span>正式任务标题</span>
                <input
                  value={selected.analysis.title}
                  disabled={selected.decision !== "pending"}
                  onChange={(event) =>
                    updateDraft(selected.id, {
                      title: event.target.value,
                      summaryMarkdown: selected.analysis.summaryMarkdown,
                    })
                  }
                />
              </label>

              <div className="field">
                <span>
                  正式任务正文
                  <small>你可以继续编辑，支持 Markdown 和图片</small>
                </span>
                <LazyRichMarkdownEditor
                  key={selected.id}
                  value={selected.analysis.summaryMarkdown}
                  disabled={selected.decision !== "pending"}
                  onCommit={(summaryMarkdown) =>
                    updateDraft(selected.id, {
                      title: selected.analysis.title,
                      summaryMarkdown,
                    })
                  }
                />
              </div>

              {(selected.analysis.keyPoints.length > 0 ||
                selected.analysis.openQuestions.length > 0) && (
                <div className="analysis-grid">
                  <div>
                    <strong>提取到的关键信息</strong>
                    <ul>
                      {selected.analysis.keyPoints.map((point) => (
                        <li key={point}>{point}</li>
                      ))}
                    </ul>
                  </div>
                  <div>
                    <strong>仍需人工判断</strong>
                    {selected.analysis.openQuestions.length > 0 ? (
                      <ul>
                        {selected.analysis.openQuestions.map((question) => (
                          <li key={question}>{question}</li>
                        ))}
                      </ul>
                    ) : (
                      <p>PI 没有提出额外问题；仍需你核对全部内容。</p>
                    )}
                  </div>
                </div>
              )}

              <details className="raw-source">
                <summary>查看脚本保留的 Plane 原始来源</summary>
                <pre>{selected.sourceMarkdown}</pre>
              </details>

              <footer className="candidate-actions">
                {selected.decision === "pending" ? (
                  <>
                    <button
                      type="button"
                      className="button ghost"
                      disabled={Boolean(busy)}
                      onClick={() => setIgnored(selected.id, true)}
                    >
                      <EyeOff size={16} />
                      忽略
                    </button>
                    <button
                      type="button"
                      className="button secondary"
                      disabled={Boolean(busy)}
                      onClick={() => analyze(selected)}
                    >
                      {busy === "analyze" ? (
                        <LoaderCircle className="spin" size={16} />
                      ) : (
                        <Bot size={16} />
                      )}
                      用 PI 提炼关键信息
                    </button>
                    <button
                      type="button"
                      className="button primary"
                      disabled={Boolean(busy)}
                      onClick={() => accept(selected)}
                    >
                      <Check size={16} />
                      人工确认并转为任务
                      <ArrowRight size={15} />
                    </button>
                  </>
                ) : selected.decision === "ignored" ? (
                  <button
                    type="button"
                    className="button secondary"
                    onClick={() => setIgnored(selected.id, false)}
                  >
                    <RotateCcw size={16} />
                    恢复为待确认
                  </button>
                ) : (
                  <button
                    type="button"
                    className="button secondary"
                    onClick={() =>
                      selected.acceptedTaskId &&
                      onOpenTask(selected.acceptedTaskId)
                    }
                  >
                    <CheckCircle2 size={16} />
                    打开已创建任务
                  </button>
                )}
              </footer>
            </article>
          )}
        </div>
      )}
    </section>
  );
}
