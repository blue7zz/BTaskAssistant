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
} from "../domain/collection";
import {
  analyzePlaneCandidate,
  collectPlaneWorkItems,
  hasPlaneToken,
  listPlaneProjects,
  savePlaneToken,
  testPlaneConnection,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";

type BusyAction =
  | "token"
  | "projects"
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

  const discoverProjects = async () => {
    const discovered = await listPlaneProjects(settings);
    if (discovered.length === 0) {
      throw new Error("连接成功，但这个工作区中没有可访问的项目");
    }
    setProjects(discovered);
    const current =
      discovered.find((project) => project.id === settings.projectId) ??
      discovered.find(
        (project) =>
          project.name.toLocaleLowerCase() ===
          settings.projectName.trim().toLocaleLowerCase(),
      ) ??
      (discovered.length === 1 ? discovered[0] : undefined);
    if (current) {
      updateSettings({
        projectId: current.id,
        projectName: current.name,
        projectIdentifier: current.identifier,
      });
    }
    const message =
      discovered.length === 1
        ? `已发现项目 ${discovered[0].identifier || discovered[0].name}，已自动选中。`
        : `已发现 ${discovered.length} 个项目，请选择要收集的项目。`;
    setConnectionMessage(message);
    return message;
  };

  const saveToken = () =>
    run("token", async () => {
      await savePlaneToken(settings, token);
      setToken("");
      setTokenStored(true);
      const message = await discoverProjects();
      onSuccess(`Plane 令牌已安全保存；${message}`);
    });

  const refreshProjects = () =>
    run("projects", async () => {
      const message = await discoverProjects();
      onSuccess(message);
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
            <span>服务地址</span>
            <input
              value={settings.baseUrl}
              onChange={(event) => {
                updateSettings({
                  baseUrl: event.target.value,
                  projectId: "",
                  projectName: "",
                  projectIdentifier: "",
                });
                setProjects([]);
                setConnectionMessage("");
              }}
              placeholder="https://plane.example.com"
            />
          </label>
          <label className="field">
            <span>Workspace slug</span>
            <input
              value={settings.workspaceSlug}
              onChange={(event) => {
                updateSettings({
                  workspaceSlug: event.target.value,
                  projectId: "",
                  projectName: "",
                  projectIdentifier: "",
                });
                setProjects([]);
                setConnectionMessage("");
              }}
              placeholder="my-team"
            />
          </label>
          <label className="field">
            <span>
              项目
              <small>保存令牌后自动发现</small>
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
        </div>
        {settings.projectId && (
          <div className="project-selection-meta">
            <FolderKanban size={14} />
            <span>
              当前项目：
              <strong>
                {settings.projectIdentifier || settings.projectName}
              </strong>
            </span>
            <code>{settings.projectId}</code>
          </div>
        )}
        <div className="token-row">
          <label className="field">
            <span>
              Personal Access Token
              <small>{tokenStored ? "系统凭据库中已有令牌" : "只在保存时使用"}</small>
            </span>
            <div className="credential-input">
              <KeyRound size={16} />
              <input
                type="password"
                value={token}
                autoComplete="off"
                onChange={(event) => setToken(event.target.value)}
                placeholder={tokenStored ? "输入新令牌可替换" : "plane_api_…"}
              />
            </div>
          </label>
          <button
            type="button"
            className="button secondary"
            disabled={Boolean(busy) || !token.trim()}
            onClick={saveToken}
          >
            {busy === "token" ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <KeyRound size={16} />
            )}
            保存并发现项目
          </button>
          <button
            type="button"
            className="button secondary"
            disabled={Boolean(busy) || !tokenStored}
            onClick={refreshProjects}
          >
            {busy === "projects" ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <RefreshCw size={16} />
            )}
            刷新项目
          </button>
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
