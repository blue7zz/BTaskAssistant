import {
  ArrowRight,
  AlertTriangle,
  Check,
  CheckCircle2,
  CloudDownload,
  EyeOff,
  ExternalLink,
  ListFilter,
  LoaderCircle,
  MessageSquareText,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type {
  CandidateDecision,
  CollectionCandidate,
  PlanePerson,
} from "../domain/collection";
import { planeWorkItemURL } from "../domain/collection";
import {
  collectPlaneWorkItems,
  loadPlaneWorkItemDetails,
  openExternalURL,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";

type BusyAction = "collect";

const DECISION_LABELS: Record<CandidateDecision, string> = {
  pending: "待确认",
  accepted: "已转换",
  ignored: "已忽略",
};

const ALL_STATES = "all";
const NO_STATE = "state:none";
const UNASSIGNED = "unassigned";
const MAX_CONCURRENT_DETAIL_REQUESTS = 3;
const ignoreReadonlyCommit = () => undefined;

function formatDate(value?: string): string {
  if (!value) return "尚未同步";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatCommentDate(value?: string): string {
  if (!value) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function initials(name: string): string {
  const value = name.trim();
  if (!value) return "?";
  const words = value.split(/\s+/).filter(Boolean);
  if (words.length > 1) {
    return `${words[0][0]}${words[words.length - 1][0]}`.toUpperCase();
  }
  return Array.from(value).slice(0, 2).join("").toUpperCase();
}

function assigneeKey(person: PlanePerson): string {
  return person.id.trim()
    ? `id:${person.id.trim()}`
    : `name:${person.name.trim().toLocaleLowerCase()}`;
}

function candidateAssignees(candidate: CollectionCandidate): PlanePerson[] {
  if ((candidate.assigneeDetails ?? []).length > 0) {
    return candidate.assigneeDetails ?? [];
  }
  return candidate.assignees.map((name) => ({ id: "", name }));
}

function candidateStateKey(candidate: CollectionCandidate): string {
  const stateName = candidate.stateName.trim();
  return stateName ? `state:${stateName}` : NO_STATE;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
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
  const assignees = candidateAssignees(candidate);
  const comments = candidate.comments ?? [];
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
      <p>
        {candidate.detailsLoaded
          ? candidate.descriptionMarkdown || "Plane 中没有填写描述"
          : "点击后加载最新详情和评论"}
      </p>
      <div className="candidate-card-context">
        <span title={assignees.map((person) => person.name).join("、")}>
          <UserRound size={11} />
          {assignees.length > 0
            ? assignees.map((person) => person.name).join("、")
            : "未分配"}
        </span>
        <span>
          <MessageSquareText size={11} />
          {candidate.detailsLoaded
            ? candidate.commentsSyncError
              ? "同步失败"
              : comments.length
            : "按需"}
        </span>
      </div>
      <div className="candidate-card-meta">
        <span className={`priority priority-${candidate.priority}`}>
          {candidate.priority}
        </span>
        <span>只读预览</span>
      </div>
    </button>
  );
}

interface PlaneCollectorProps {
  connected: boolean;
  onSuccess(message: string): void;
  onError(error: unknown): void;
  onOpenTask(taskID: string): void;
}

export function PlaneCollector({
  connected,
  onSuccess,
  onError,
  onOpenTask,
}: PlaneCollectorProps) {
  const settings = useWorkspaceStore((state) => state.planeSettings);
  const candidates = useWorkspaceStore(
    (state) => state.collectionCandidates,
  );
  const ingestCandidates = useWorkspaceStore(
    (state) => state.ingestPlaneCandidates,
  );
  const hydrateCandidate = useWorkspaceStore(
    (state) => state.hydratePlaneCandidate,
  );
  const acceptCandidate = useWorkspaceStore(
    (state) => state.acceptCandidate,
  );
  const setIgnored = useWorkspaceStore(
    (state) => state.setCandidateIgnored,
  );
  const candidateFilters = useWorkspaceStore(
    (state) => state.planeCandidateFilters,
  );
  const updateCandidateFilters = useWorkspaceStore(
    (state) => state.updatePlaneCandidateFilters,
  );

  const [busy, setBusy] = useState<BusyAction>();
  const [filter, setFilter] = useState<CandidateDecision>("pending");
  const stateFilter = candidateFilters.state;
  const assigneeFilters = candidateFilters.assignees;
  const [selectedID, setSelectedID] = useState<string>();
  const [syncMessage, setSyncMessage] = useState("");
  const [loadingDetailIDs, setLoadingDetailIDs] = useState<string[]>([]);
  const [detailErrors, setDetailErrors] = useState<Record<string, string>>({});
  const detailRequests = useRef(new Set<string>());
  const assigneeFilterRef = useRef<HTMLDetailsElement>(null);

  const stateOptions = useMemo(() => {
    const options = new Map<
      string,
      { value: string; name: string; count: number }
    >();
    for (const candidate of candidates) {
      const value = candidateStateKey(candidate);
      const existing = options.get(value);
      options.set(value, {
        value,
        name: candidate.stateName.trim() || "未设置状态",
        count: (existing?.count ?? 0) + 1,
      });
    }
    return Array.from(options.values()).sort((left, right) =>
      left.name.localeCompare(right.name, "zh-CN"),
    );
  }, [candidates]);
  const assigneeOptions = useMemo(() => {
    const options = new Map<
      string,
      { value: string; name: string; count: number }
    >();
    for (const candidate of candidates) {
      const seen = new Set<string>();
      for (const person of candidateAssignees(candidate)) {
        const value = assigneeKey(person);
        if (!person.name.trim() || seen.has(value)) continue;
        seen.add(value);
        const existing = options.get(value);
        options.set(value, {
          value,
          name: person.name.trim(),
          count: (existing?.count ?? 0) + 1,
        });
      }
    }
    return Array.from(options.values()).sort((left, right) =>
      left.name.localeCompare(right.name, "zh-CN"),
    );
  }, [candidates]);
  const unassignedCount = useMemo(
    () =>
      candidates.filter(
        (candidate) => candidateAssignees(candidate).length === 0,
      ).length,
    [candidates],
  );
  const selectionFilteredCandidates = useMemo(
    () =>
      candidates.filter((candidate) => {
        if (
          stateFilter !== ALL_STATES &&
          candidateStateKey(candidate) !== stateFilter
        ) {
          return false;
        }
        if (assigneeFilters.length === 0) return true;
        const assignees = candidateAssignees(candidate);
        return assigneeFilters.some(
          (value) =>
            (value === UNASSIGNED && assignees.length === 0) ||
            assignees.some((person) => assigneeKey(person) === value),
        );
      }),
    [assigneeFilters, candidates, stateFilter],
  );
  const decisionCounts = useMemo(
    () =>
      selectionFilteredCandidates.reduce<Record<CandidateDecision, number>>(
        (counts, candidate) => {
          counts[candidate.decision] += 1;
          return counts;
        },
        { pending: 0, accepted: 0, ignored: 0 },
      ),
    [selectionFilteredCandidates],
  );
  const filteredCandidates = useMemo(
    () =>
      selectionFilteredCandidates.filter(
        (candidate) => candidate.decision === filter,
      ),
    [filter, selectionFilteredCandidates],
  );
  const selected = filteredCandidates.find(
    (candidate) => candidate.id === selectedID,
  );
  const assigneeFilterLabel = useMemo(() => {
    if (assigneeFilters.length === 0) {
      return `全部负责人（${candidates.length}）`;
    }
    if (assigneeFilters.length > 1) {
      return `已选 ${assigneeFilters.length} 项`;
    }
    if (assigneeFilters[0] === UNASSIGNED) {
      return `未分配（${unassignedCount}）`;
    }
    const option = assigneeOptions.find(
      (candidate) => candidate.value === assigneeFilters[0],
    );
    return option ? `${option.name}（${option.count}）` : "已选 1 项";
  }, [assigneeFilters, assigneeOptions, candidates, unassignedCount]);
  const hasActiveFilters =
    stateFilter !== ALL_STATES || assigneeFilters.length > 0;
  const configured = Boolean(
    settings.baseUrl.trim() &&
      settings.workspaceSlug.trim() &&
      settings.projectId.trim(),
  );

  useEffect(() => {
    const closeAssigneeFilter = (event: MouseEvent) => {
      const element = assigneeFilterRef.current;
      if (
        element?.open &&
        event.target instanceof Node &&
        !element.contains(event.target)
      ) {
        element.open = false;
      }
    };
    document.addEventListener("mousedown", closeAssigneeFilter);
    return () => document.removeEventListener("mousedown", closeAssigneeFilter);
  }, []);

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

  const collect = () =>
    run("collect", async () => {
      const payloads = await collectPlaneWorkItems(settings);
      const pendingCount = ingestCandidates(payloads);
      setFilter("pending");
      setSelectedID(undefined);
      setSyncMessage(
        `已读取 ${payloads.length} 条工作项摘要，${pendingCount} 条待确认；详情和评论将在点击任务后读取。`,
      );
      onSuccess("Plane 摘要收集完成；没有自动创建正式任务");
    });

  const toggleAssigneeFilter = (value: string) => {
    updateCandidateFilters({
      assignees: assigneeFilters.includes(value)
        ? assigneeFilters.filter((candidate) => candidate !== value)
        : [...assigneeFilters, value],
    });
    setSelectedID(undefined);
  };

  const selectCandidate = async (candidate: CollectionCandidate) => {
    setSelectedID(candidate.id);
    if (
      (candidate.detailsLoaded && !candidate.commentsSyncError) ||
      detailRequests.current.has(candidate.id)
    ) {
      return;
    }
    if (detailRequests.current.size >= MAX_CONCURRENT_DETAIL_REQUESTS) {
      setDetailErrors((current) => ({
        ...current,
        [candidate.id]: "已有 3 条任务正在加载，请稍后重试。",
      }));
      return;
    }

    detailRequests.current.add(candidate.id);
    setLoadingDetailIDs((current) => [...current, candidate.id]);
    setDetailErrors((current) => {
      const next = { ...current };
      delete next[candidate.id];
      return next;
    });
    try {
      const payload = await loadPlaneWorkItemDetails(
        settings,
        candidate.externalId,
      );
      const applied = hydrateCandidate(payload, {
        candidateID: candidate.id,
        collectionRevision: candidate.collectionRevision,
        projectID: settings.projectId,
      });
      if (!applied) {
        setDetailErrors((current) => ({
          ...current,
          [candidate.id]: "Plane 摘要已更新，请重新点击任务读取最新详情。",
        }));
      }
    } catch (error) {
      setDetailErrors((current) => ({
        ...current,
        [candidate.id]: errorMessage(error),
      }));
      onError(error);
    } finally {
      detailRequests.current.delete(candidate.id);
      setLoadingDetailIDs((current) =>
        current.filter((candidateID) => candidateID !== candidate.id),
      );
    }
  };

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
            固定脚本只同步列表摘要；点击任务后再读取详情和评论。父任务、标题与正文只读展示，
            只有点击确认后才会进入正式任务池。
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

      <div className="collector-sync-bar">
        <div className="candidate-filters">
          <label className="candidate-filter status-filter">
            <ListFilter size={14} />
            <span>状态</span>
            <select
              aria-label="按任务状态筛选"
              value={stateFilter}
              onChange={(event) => {
                updateCandidateFilters({ state: event.target.value });
                setSelectedID(undefined);
              }}
            >
              <option value={ALL_STATES}>
                全部状态（{candidates.length}）
              </option>
              {stateOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.name}（{option.count}）
                </option>
              ))}
            </select>
          </label>

          <details
            ref={assigneeFilterRef}
            className="candidate-filter assignee-filter"
          >
            <summary aria-label="按负责人筛选">
              <UserRound size={14} />
              <span>负责人</span>
              <strong>{assigneeFilterLabel}</strong>
            </summary>
            <div className="candidate-filter-menu">
              <button
                type="button"
                className={assigneeFilters.length === 0 ? "selected" : ""}
                onClick={() => {
                  updateCandidateFilters({ assignees: [] });
                  setSelectedID(undefined);
                }}
              >
                <span>全部负责人</span>
                <small>{candidates.length}</small>
              </button>
              {unassignedCount > 0 && (
                <label>
                  <input
                    type="checkbox"
                    aria-label="筛选负责人 未分配"
                    checked={assigneeFilters.includes(UNASSIGNED)}
                    onChange={() => toggleAssigneeFilter(UNASSIGNED)}
                  />
                  <span>未分配</span>
                  <small>{unassignedCount}</small>
                </label>
              )}
              {assigneeOptions.map((option) => (
                <label key={option.value}>
                  <input
                    type="checkbox"
                    aria-label={`筛选负责人 ${option.name}`}
                    checked={assigneeFilters.includes(option.value)}
                    onChange={() => toggleAssigneeFilter(option.value)}
                  />
                  <span>{option.name}</span>
                  <small>{option.count}</small>
                </label>
              ))}
            </div>
          </details>
        </div>
        <div className="collector-sync-status">
          <span>
            <RefreshCw size={13} />
            上次同步：{formatDate(settings.lastCollectedAt)}
          </span>
          {syncMessage && <strong>{syncMessage}</strong>}
        </div>
        <button
          type="button"
          className="button primary"
          disabled={
            Boolean(busy) ||
            loadingDetailIDs.length > 0 ||
            !configured ||
            !connected
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

      <div className="candidate-toolbar">
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
                <span>{decisionCounts[decision]}</span>
              </button>
            ),
          )}
        </div>
      </div>

      {filteredCandidates.length === 0 ? (
        <div className="collector-empty">
          <CloudDownload size={30} />
          <strong>
            {hasActiveFilters
              ? "当前筛选条件下没有候选"
              : filter === "pending"
                ? "还没有待确认候选"
              : `没有${DECISION_LABELS[filter]}的候选`}
          </strong>
          <p>
            {filter === "pending"
              ? configured
                ? "点击“从 Plane 收集”读取候选；脚本不会自动创建任务。"
                : "请先前往设置完成 Plane 连接并选择收集项目。"
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
                onSelect={() => void selectCandidate(candidate)}
              />
            ))}
          </aside>

          {selected?.detailsLoaded ? (
            <article className="candidate-detail">
              {selected.parent && (
                <a
                  className="plane-parent-link"
                  href={planeWorkItemURL(
                    settings,
                    selected.parent.externalKey,
                  )}
                  target="_blank"
                  rel="noreferrer"
                  onClick={(event) => {
                    event.preventDefault();
                    openExternalURL(
                      planeWorkItemURL(
                        settings,
                        selected.parent?.externalKey ?? "",
                      ),
                    );
                  }}
                >
                  <span className="plane-parent-dot" aria-hidden="true" />
                  <strong>{selected.parent.externalKey}</strong>
                  <span>{selected.parent.title}</span>
                  <ExternalLink size={13} aria-hidden="true" />
                </a>
              )}

              <header className="plane-preview-header">
                <span>{selected.externalKey}</span>
                <a
                  href={planeWorkItemURL(settings, selected.externalKey)}
                  target="_blank"
                  rel="noreferrer"
                  onClick={(event) => {
                    event.preventDefault();
                    openExternalURL(
                      planeWorkItemURL(settings, selected.externalKey),
                    );
                  }}
                >
                  <h3>{selected.title}</h3>
                  <ExternalLink size={15} aria-hidden="true" />
                </a>
              </header>

              <section
                className="plane-description-preview"
                aria-label="Plane 工作项正文只读预览"
              >
                <LazyRichMarkdownEditor
                  key={selected.id}
                  value={selected.descriptionMarkdown}
                  disabled
                  onCommit={ignoreReadonlyCommit}
                />
              </section>

              <section className="plane-context-panel">
                <header>
                  <div>
                    <span className="plane-context-icon">
                      <MessageSquareText size={15} />
                    </span>
                    <div>
                      <strong>Plane 评论</strong>
                      <small>
                        与正文一起保留并只读展示
                      </small>
                    </div>
                  </div>
                  <span className="comment-count">
                    {(selected.comments ?? []).length}
                  </span>
                </header>
                <div className="plane-assignee-row">
                  <UserRound size={13} />
                  <span>负责人</span>
                  <div>
                    {candidateAssignees(selected).length > 0 ? (
                      candidateAssignees(selected).map((person) => (
                        <span className="assignee-chip" key={assigneeKey(person)}>
                          <i>{initials(person.name)}</i>
                          {person.name}
                        </span>
                      ))
                    ) : (
                      <span className="assignee-chip empty">未分配</span>
                    )}
                  </div>
                </div>
                {selected.commentsSyncError ? (
                  <div className="comment-sync-warning">
                    <AlertTriangle size={15} />
                    <div>
                      <strong>评论没有完整同步</strong>
                      <span>{selected.commentsSyncError}</span>
                    </div>
                    <button
                      type="button"
                      className="button secondary comment-retry-button"
                      disabled={loadingDetailIDs.includes(selected.id)}
                      onClick={() => void selectCandidate(selected)}
                    >
                      {loadingDetailIDs.includes(selected.id) ? (
                        <LoaderCircle className="spin" size={13} />
                      ) : (
                        <RefreshCw size={13} />
                      )}
                      重试
                    </button>
                  </div>
                ) : (selected.comments ?? []).length === 0 ? (
                  <div className="comments-empty">
                    <MessageSquareText size={18} />
                    <span>这条工作项还没有评论</span>
                  </div>
                ) : (
                  <div className="comment-timeline">
                    {(selected.comments ?? []).map((comment) => (
                      <article className="plane-comment" key={comment.id}>
                        <div className="comment-avatar">
                          {initials(comment.actor.name)}
                        </div>
                        <div className="comment-card">
                          <header>
                            <strong>{comment.actor.name || "Plane 用户"}</strong>
                            <time dateTime={comment.createdAt}>
                              {formatCommentDate(comment.createdAt)}
                              {comment.editedAt ? " · 已编辑" : ""}
                            </time>
                          </header>
                          <p>{comment.bodyMarkdown}</p>
                        </div>
                      </article>
                    ))}
                  </div>
                )}
              </section>

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
                      className="button primary"
                      disabled={
                        Boolean(busy) || Boolean(selected.commentsSyncError)
                      }
                      title={
                        selected.commentsSyncError
                          ? "请先重试并完整同步 Plane 评论"
                          : undefined
                      }
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
          ) : (
            <section className="candidate-detail-placeholder">
              {selected && loadingDetailIDs.includes(selected.id) ? (
                <LoaderCircle className="spin" size={28} />
              ) : selected && detailErrors[selected.id] ? (
                <AlertTriangle size={28} />
              ) : (
                <MessageSquareText size={28} />
              )}
              <strong>
                {selected && loadingDetailIDs.includes(selected.id)
                  ? "正在读取这条任务的详情和评论"
                  : selected && detailErrors[selected.id]
                    ? "详情加载失败"
                    : "选择候选任务后再加载详情"}
              </strong>
              <p>
                {selected && detailErrors[selected.id]
                  ? detailErrors[selected.id]
                  : "批量收集只同步 Plane 列表摘要，不会遍历全部任务的详情和评论。"}
              </p>
              {selected && !loadingDetailIDs.includes(selected.id) && (
                <button
                  type="button"
                  className="button secondary"
                  onClick={() => void selectCandidate(selected)}
                >
                  <RefreshCw size={15} />
                  {detailErrors[selected.id]
                    ? "重试加载详情和评论"
                    : "加载详情和评论"}
                </button>
              )}
            </section>
          )}
        </div>
      )}
    </section>
  );
}
