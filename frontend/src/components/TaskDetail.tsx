import {
  ArrowLeft,
  ArrowRight,
  Bot,
  Check,
  ChevronDown,
  ClipboardCheck,
  Clock3,
  Code2,
  Copy,
  FileCheck2,
  FileText,
  FolderGit2,
  Link2,
  MessageSquareText,
  Plus,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Trash2,
  UserCheck,
} from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  STATUS_META,
  TASK_STATUSES,
  statusIndex,
  type EvidenceType,
  type ReviewMode,
  type Task,
  type TaskPriority,
  type TaskStatus,
} from "../domain/task";
import { copyText } from "../lib/clipboard";
import type { EngineStatus } from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";
import { RequirementWorkspace } from "./RequirementWorkspace";

interface TaskDetailProps {
  task: Task;
  engines: EngineStatus[];
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

type RunAction = (
  action: () => void | Promise<void>,
  successMessage?: string,
) => void;

function formatDate(value?: string): string {
  if (!value) return "尚未记录";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function DraftInput({
  label,
  value,
  placeholder,
  multiline = false,
  rows = 3,
  disabled = false,
  hint,
  onCommit,
}: {
  label: string;
  value: string;
  placeholder?: string;
  multiline?: boolean;
  rows?: number;
  disabled?: boolean;
  hint?: string;
  onCommit(value: string): void;
}) {
  const [draft, setDraft] = useState(value);

  useEffect(() => setDraft(value), [value]);

  const commit = () => {
    if (draft !== value) onCommit(draft);
  };

  return (
    <label className="field">
      <span>
        {label}
        {hint && <small>{hint}</small>}
      </span>
      {multiline ? (
        <textarea
          value={draft}
          placeholder={placeholder}
          rows={rows}
          disabled={disabled}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
        />
      ) : (
        <input
          value={draft}
          placeholder={placeholder}
          disabled={disabled}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
        />
      )}
    </label>
  );
}

function Panel({
  title,
  eyebrow,
  icon,
  actions,
  children,
  className = "",
}: {
  title: string;
  eyebrow?: string;
  icon?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`detail-panel ${className}`}>
      <header className="panel-header">
        <div>
          {eyebrow && <span className="eyebrow">{eyebrow}</span>}
          <h3>
            {icon}
            {title}
          </h3>
        </div>
        {actions && <div className="panel-actions">{actions}</div>}
      </header>
      {children}
    </section>
  );
}

function CopyButton({
  value,
  label = "复制",
  run,
}: {
  value: string;
  label?: string;
  run: RunAction;
}) {
  return (
    <button
      type="button"
      className="button compact secondary"
      onClick={() =>
        run(() => copyText(value), `${label === "复制" ? "内容" : label}已复制`)
      }
    >
      <Copy size={14} />
      {label}
    </button>
  );
}

function BasicsPanel({
  task,
  editable,
  run,
}: {
  task: Task;
  editable: boolean;
  run: RunAction;
}) {
  const updateTaskDetails = useWorkspaceStore(
    (state) => state.updateTaskDetails,
  );

  return (
    <Panel title="任务信息" eyebrow="基础资料" icon={<FileText size={17} />}>
      <div className="field-grid two-columns">
        <DraftInput
          label="任务标题"
          value={task.title}
          disabled={!editable}
          onCommit={(title) =>
            run(() => updateTaskDetails(task.id, { title }))
          }
        />
        <DraftInput
          label="对应项目或仓库"
          value={task.projectName}
          placeholder="需要人工明确"
          disabled={!editable}
          onCommit={(projectName) =>
            run(() => updateTaskDetails(task.id, { projectName }))
          }
        />
        <label className="field">
          <span>优先级</span>
          <select
            value={task.priority}
            disabled={!editable}
            onChange={(event) =>
              run(() =>
                updateTaskDetails(task.id, {
                  priority: event.target.value as TaskPriority,
                }),
              )
            }
          >
            <option value="low">低</option>
            <option value="medium">中</option>
            <option value="high">高</option>
          </select>
        </label>
        <div className="field rich-summary-field">
          <span>
            任务正文
            <small>富文本 / Markdown / 图文混排</small>
          </span>
          <LazyRichMarkdownEditor
            value={task.summary}
            disabled={!editable}
            onCommit={(summary) =>
              run(() => updateTaskDetails(task.id, { summary }))
            }
          />
        </div>
      </div>
    </Panel>
  );
}

function EvidencePanel({
  task,
  editable,
  run,
}: {
  task: Task;
  editable: boolean;
  run: RunAction;
}) {
  const addEvidence = useWorkspaceStore((state) => state.addEvidence);
  const removeEvidence = useWorkspaceStore((state) => state.removeEvidence);
  const [adding, setAdding] = useState(false);
  const [type, setType] = useState<EvidenceType>("manual");
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");

  const submit = () =>
    run(() => {
      addEvidence(task.id, { type, title, content });
      setTitle("");
      setContent("");
      setAdding(false);
    }, "需求来源已添加；已有生成稿需要重新生成");

  return (
    <Panel
      title="需求来源"
      eyebrow={`${task.evidence.length} 条可追溯资料`}
      icon={<Link2 size={17} />}
      actions={
        editable && (
          <button
            type="button"
            className="button compact secondary"
            onClick={() => setAdding((value) => !value)}
          >
            <Plus size={14} />
            添加来源
          </button>
        )
      }
    >
      {adding && (
        <div className="inline-composer">
          <div className="field-grid source-fields">
            <label className="field">
              <span>来源类型</span>
              <select
                value={type}
                onChange={(event) =>
                  setType(event.target.value as EvidenceType)
                }
              >
                <option value="manual">人工说明</option>
                <option value="chat">聊天记录</option>
                <option value="project">项目现状</option>
                <option value="file">文档 / 文件</option>
                <option value="plane">Plane 工作项</option>
              </select>
            </label>
            <label className="field">
              <span>来源标题</span>
              <input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="这条资料是什么"
              />
            </label>
          </div>
          <label className="field">
            <span>原始内容</span>
            <textarea
              value={content}
              onChange={(event) => setContent(event.target.value)}
              placeholder="保留原文；不要在这里改写成推断结论。"
              rows={5}
            />
          </label>
          <div className="inline-actions">
            <button
              className="button compact ghost"
              onClick={() => setAdding(false)}
            >
              取消
            </button>
            <button className="button compact primary" onClick={submit}>
              保存来源
            </button>
          </div>
        </div>
      )}

      {task.evidence.length === 0 ? (
        <div className="mini-empty">
          <ShieldAlert size={22} />
          <div>
            <strong>还没有真实来源</strong>
            <p>没有来源时不能人工确认需求。</p>
          </div>
        </div>
      ) : (
        <div className="source-list">
          {task.evidence.map((source, index) => (
            <article className="source-card" key={source.id}>
              <div className="source-index">S{index + 1}</div>
              <div className="source-content">
                <div>
                  <strong>{source.title}</strong>
                  <span>{source.type}</span>
                </div>
                <p>{source.content}</p>
              </div>
              {editable && (
                <button
                  className="icon-button subtle-danger"
                  aria-label={`删除来源 ${source.title}`}
                  onClick={() => {
                    if (window.confirm("删除这条需求来源？已有生成稿会失效。")) {
                      run(() => removeEvidence(task.id, source.id));
                    }
                  }}
                >
                  <Trash2 size={15} />
                </button>
              )}
            </article>
          ))}
        </div>
      )}
    </Panel>
  );
}

function InboxStage({ task, run }: { task: Task; run: RunAction }) {
  return (
    <div className="stage-stack">
      <div className="stage-callout neutral">
        <div className="callout-icon">
          <MessageSquareText size={18} />
        </div>
        <div>
          <strong>先收集，不分析</strong>
          <p>
            在任务池中保留原始说明和资料。进入需求整理后，系统才会生成有来源标记的结构化候选稿。
          </p>
        </div>
      </div>
      <BasicsPanel task={task} editable run={run} />
      <EvidencePanel task={task} editable run={run} />
    </div>
  );
}

function ApprovedStage({ task, run }: { task: Task; run: RunAction }) {
  return (
    <div className="stage-stack">
      <div className="stage-callout success">
        <div className="callout-icon">
          <FileCheck2 size={18} />
        </div>
        <div>
          <strong>需求已经确认，开发尚未开始</strong>
          <p>
            这是人工检查后的冻结版本。点击右上角“进入开发中”才会推进状态。
          </p>
        </div>
      </div>
      <div className="summary-grid">
        <div className="summary-stat">
          <span>确认版本</span>
          <strong>v{task.requirements.confirmedRevision}</strong>
        </div>
        <div className="summary-stat">
          <span>需求来源</span>
          <strong>{task.evidence.length}</strong>
        </div>
        <div className="summary-stat">
          <span>验收标准</span>
          <strong>{task.requirements.acceptanceCriteria.length}</strong>
        </div>
        <div className="summary-stat">
          <span>确认时间</span>
          <strong className="small-value">
            {formatDate(task.requirements.confirmedAt)}
          </strong>
        </div>
      </div>
      <Panel
        title="已确认需求文档"
        icon={<FileText size={17} />}
        actions={
          <CopyButton
            value={task.requirements.document}
            label="复制需求文档"
            run={run}
          />
        }
      >
        <pre className="document-preview">{task.requirements.document}</pre>
      </Panel>
      <Panel
        title="已确认执行提示词"
        icon={<Code2 size={17} />}
        actions={
          <CopyButton
            value={task.requirements.executionPrompt}
            label="复制提示词"
            run={run}
          />
        }
      >
        <pre className="document-preview prompt-preview">
          {task.requirements.executionPrompt}
        </pre>
      </Panel>
    </div>
  );
}

function DevelopmentStage({
  task,
  engines,
  run,
}: {
  task: Task;
  engines: EngineStatus[];
  run: RunAction;
}) {
  const setDevelopmentEngine = useWorkspaceStore(
    (state) => state.setDevelopmentEngine,
  );
  const markDelegated = useWorkspaceStore((state) => state.markDelegated);
  const setDevelopmentResult = useWorkspaceStore(
    (state) => state.setDevelopmentResult,
  );
  const markDevelopmentCompleted = useWorkspaceStore(
    (state) => state.markDevelopmentCompleted,
  );
  const currentEngine = engines.find(
    (engine) => engine.id === task.development.engine,
  );

  return (
    <div className="stage-stack">
      <div className="stage-callout neutral">
        <div className="callout-icon">
          <Bot size={18} />
        </div>
        <div>
          <strong>执行引擎与工作流隔离</strong>
          <p>
            当前可复制已确认提示词到 Codex 或 PI 外部执行。CLI
            协议未确认前，本版本不会猜测命令并自动运行。
          </p>
        </div>
      </div>

      <Panel
        title="开发委托"
        eyebrow="只使用已确认提示词"
        icon={<Code2 size={17} />}
      >
        <div className="engine-selection">
          <label className="field">
            <span>执行工具</span>
            <div className="select-wrap">
              <select
                value={task.development.engine}
                disabled={task.development.state !== "idle"}
                onChange={(event) =>
                  run(() =>
                    setDevelopmentEngine(
                      task.id,
                      event.target.value as "codex" | "pi",
                    ),
                  )
                }
              >
                <option value="codex">Codex</option>
                <option value="pi">PI</option>
              </select>
              <ChevronDown size={15} />
            </div>
          </label>
          <div className="engine-status-detail">
            <span
              className={`status-dot ${currentEngine?.development ? "online" : ""}`}
            />
            <div>
              <strong>
                {currentEngine?.development ? "自动接口可用" : "外部执行模式"}
              </strong>
              <p>
                {currentEngine?.description ??
                  "引擎状态暂时不可用，可先复制提示词。"}
              </p>
            </div>
          </div>
        </div>

        <div className="prompt-action">
          <div>
            <strong>已确认执行提示词</strong>
            <span>{task.requirements.executionPrompt.length} 个字符</span>
          </div>
          <CopyButton
            value={task.requirements.executionPrompt}
            label={`复制给 ${task.development.engine === "codex" ? "Codex" : "PI"}`}
            run={run}
          />
        </div>

        <div className="development-timeline">
          <div
            className={`development-step ${
              task.development.state !== "idle" ? "complete" : "current"
            }`}
          >
            <span>1</span>
            <div>
              <strong>复制并委托</strong>
              <small>{formatDate(task.development.delegatedAt)}</small>
            </div>
          </div>
          <div
            className={`development-step ${
              task.development.state === "completed" ? "complete" : ""
            }`}
          >
            <span>2</span>
            <div>
              <strong>记录实际结果</strong>
              <small>{formatDate(task.development.completedAt)}</small>
            </div>
          </div>
        </div>

        {task.development.state === "idle" && (
          <button
            className="button primary"
            onClick={() =>
              run(
                () => markDelegated(task.id),
                "已记录开发委托；这不会假装外部引擎已经运行",
              )
            }
          >
            <ArrowRight size={16} />
            我已在外部完成委托
          </button>
        )}

        {task.development.state !== "idle" && (
          <div className="result-recorder">
            <DraftInput
              label="开发结果与验证记录"
              value={task.development.resultNote}
              multiline
              rows={7}
              disabled={task.development.state === "completed"}
              placeholder="记录提交 SHA、修改文件、测试命令、实际结果和遗留问题。"
              onCommit={(resultNote) =>
                run(() => setDevelopmentResult(task.id, resultNote))
              }
            />
            {task.development.state === "delegated" && (
              <button
                className="button primary"
                onClick={() =>
                  run(
                    () => markDevelopmentCompleted(task.id),
                    "已记录开发完成；仍需手动推进到审核",
                  )
                }
              >
                <Check size={16} />
                记录开发已完成
              </button>
            )}
          </div>
        )}
      </Panel>

      <Panel
        title="执行依据"
        icon={<FileText size={17} />}
        actions={
          <CopyButton
            value={task.requirements.document}
            label="复制需求"
            run={run}
          />
        }
      >
        <pre className="document-preview collapsed-preview">
          {task.requirements.document}
        </pre>
      </Panel>
    </div>
  );
}

function ReviewStage({ task, run }: { task: Task; run: RunAction }) {
  const createReviewChecklist = useWorkspaceStore(
    (state) => state.createReviewChecklist,
  );
  const toggleReviewItem = useWorkspaceStore(
    (state) => state.toggleReviewItem,
  );
  const setReviewNote = useWorkspaceStore((state) => state.setReviewNote);
  const approveReview = useWorkspaceStore((state) => state.approveReview);
  const checked = task.review.checklist.filter((item) => item.checked).length;

  return (
    <div className="stage-stack">
      <div
        className={`stage-callout ${task.review.approvedAt ? "success" : "warning"}`}
      >
        <div className="callout-icon">
          {task.review.approvedAt ? (
            <ShieldCheck size={18} />
          ) : (
            <ClipboardCheck size={18} />
          )}
        </div>
        <div>
          <strong>
            {task.review.approvedAt
              ? "审核已人工通过"
              : "审核结果不能由 AI 自动批准"}
          </strong>
          <p>
            {task.review.approvedAt
              ? `通过时间：${formatDate(task.review.approvedAt)}`
              : "固定模板负责列项，实际检查、证据和最终确认仍由你完成。"}
          </p>
        </div>
      </div>

      <Panel
        title="开发结果"
        eyebrow={`${task.development.engine.toUpperCase()} · 已完成`}
        icon={<Code2 size={17} />}
      >
        <div className="result-note">{task.development.resultNote}</div>
      </Panel>

      <Panel
        title="审核清单"
        eyebrow={
          task.review.checklist.length
            ? `${checked}/${task.review.checklist.length} 已通过`
            : "尚未生成"
        }
        icon={<ClipboardCheck size={17} />}
        actions={
          !task.review.approvedAt && (
            <button
              className="button compact secondary"
              onClick={() =>
                run(
                  () =>
                    createReviewChecklist(
                      task.id,
                      "template" as ReviewMode,
                    ),
                  "已生成固定审核模板，尚未执行任何 AI 审查",
                )
              }
            >
              <Sparkles size={14} />
              {task.review.checklist.length ? "重置清单" : "生成固定清单"}
            </button>
          )
        }
      >
        {task.review.checklist.length === 0 ? (
          <div className="mini-empty">
            <ClipboardCheck size={22} />
            <div>
              <strong>先生成基础审核清单</strong>
              <p>PI / Codex 自动审查适配器将在 CLI 协议确认后接入。</p>
            </div>
          </div>
        ) : (
          <div className="checklist">
            {task.review.checklist.map((item) => (
              <label className="check-item" key={item.id}>
                <input
                  type="checkbox"
                  checked={item.checked}
                  disabled={Boolean(task.review.approvedAt)}
                  onChange={() =>
                    run(() => toggleReviewItem(task.id, item.id))
                  }
                />
                <span className="custom-check">
                  <Check size={13} />
                </span>
                <span>{item.label}</span>
              </label>
            ))}
          </div>
        )}

        {task.review.checklist.length > 0 && (
          <div className="review-note">
            <DraftInput
              label="审核结论与证据"
              value={task.review.note}
              multiline
              rows={6}
              disabled={Boolean(task.review.approvedAt)}
              placeholder="记录实际检查命令、测试结果、发现的问题和处理结论。"
              onCommit={(note) =>
                run(() => setReviewNote(task.id, note))
              }
            />
            {!task.review.approvedAt && (
              <button
                className="button primary"
                onClick={() =>
                  run(
                    () => approveReview(task.id),
                    "审核已人工通过；仍需手动完成任务",
                  )
                }
              >
                <UserCheck size={16} />
                人工确认审核通过
              </button>
            )}
          </div>
        )}
      </Panel>
    </div>
  );
}

function DoneStage({ task, run }: { task: Task; run: RunAction }) {
  return (
    <div className="stage-stack">
      <div className="completion-hero">
        <div className="completion-icon">
          <Check size={28} />
        </div>
        <span className="eyebrow">任务闭环</span>
        <h2>{task.title}</h2>
        <p>需求经过人工确认，开发结果已记录，审核也由人工通过。</p>
      </div>
      <div className="summary-grid">
        <div className="summary-stat">
          <span>需求版本</span>
          <strong>v{task.requirements.confirmedRevision}</strong>
        </div>
        <div className="summary-stat">
          <span>执行工具</span>
          <strong>{task.development.engine.toUpperCase()}</strong>
        </div>
        <div className="summary-stat">
          <span>来源数量</span>
          <strong>{task.evidence.length}</strong>
        </div>
        <div className="summary-stat">
          <span>完成时间</span>
          <strong className="small-value">{formatDate(task.updatedAt)}</strong>
        </div>
      </div>
      <Panel
        title="最终需求快照"
        icon={<FileCheck2 size={17} />}
        actions={
          <CopyButton
            value={task.requirements.document}
            label="复制需求"
            run={run}
          />
        }
      >
        <pre className="document-preview collapsed-preview">
          {task.requirements.document}
        </pre>
      </Panel>
      <Panel title="审核结论" icon={<ShieldCheck size={17} />}>
        <div className="result-note">{task.review.note}</div>
      </Panel>
    </div>
  );
}

function stageContent(
  task: Task,
  engines: EngineStatus[],
  run: RunAction,
) {
  switch (task.status) {
    case "inbox":
      return <InboxStage task={task} run={run} />;
    case "requirements":
      return <RequirementWorkspace task={task} engines={engines} run={run} />;
    case "approved":
      return <ApprovedStage task={task} run={run} />;
    case "development":
      return <DevelopmentStage task={task} engines={engines} run={run} />;
    case "review":
      return <ReviewStage task={task} run={run} />;
    case "done":
      return <DoneStage task={task} run={run} />;
  }
}

export function TaskDetail({
  task,
  engines,
  onSuccess,
  onError,
}: TaskDetailProps) {
  const transitionTask = useWorkspaceStore((state) => state.transitionTask);
  const currentIndex = statusIndex(task.status);
  const previousStatus = TASK_STATUSES[currentIndex - 1] as
    | TaskStatus
    | undefined;
  const nextStatus = TASK_STATUSES[currentIndex + 1] as TaskStatus | undefined;
  const nextBlocked =
    (nextStatus === "approved" && !task.requirements.confirmedAt) ||
    (nextStatus === "development" && !task.requirements.confirmedAt) ||
    (nextStatus === "review" && task.development.state !== "completed") ||
    (nextStatus === "done" && !task.review.approvedAt);
  const requirementAnalysisBusy =
    task.status === "requirements" &&
    (task.requirements.interview.status === "analyzing" ||
      task.requirements.interview.status === "reanalyzing");

  const run: RunAction = (action, successMessage) => {
    try {
      const result = action();
      if (result instanceof Promise) {
        result
          .then(() => {
            if (successMessage) onSuccess(successMessage);
          })
          .catch(onError);
      } else if (successMessage) {
        onSuccess(successMessage);
      }
    } catch (error) {
      onError(error);
    }
  };

  const progress = useMemo(
    () => Math.round((currentIndex / (TASK_STATUSES.length - 1)) * 100),
    [currentIndex],
  );

  return (
    <section className="task-detail">
      <header className="task-detail-header">
        <div className="task-title-row">
          <div>
            <div className="task-context">
              <span className={`status-chip status-${task.status}`}>
                {STATUS_META[task.status].label}
              </span>
              <span>
                <FolderGit2 size={13} />
                {task.projectName || "项目待确认"}
              </span>
              <span>
                <Clock3 size={13} />
                v{task.revision}
              </span>
            </div>
            <h2>{task.title}</h2>
            <p>{STATUS_META[task.status].description}</p>
          </div>
          <div className="transition-actions">
            {previousStatus && (
              <button
                className="button secondary"
                disabled={requirementAnalysisBusy}
                onClick={() =>
                  run(
                    () => transitionTask(task.id, previousStatus),
                    `已退回“${STATUS_META[previousStatus].label}”`,
                  )
                }
              >
                <ArrowLeft size={15} />
                退回
              </button>
            )}
            {nextStatus && (
              <button
                className="button primary"
                disabled={nextBlocked || requirementAnalysisBusy}
                onClick={() =>
                  run(
                    () => transitionTask(task.id, nextStatus),
                    `已手动推进到“${STATUS_META[nextStatus].label}”`,
                  )
                }
              >
                进入{STATUS_META[nextStatus].label}
                <ArrowRight size={15} />
              </button>
            )}
          </div>
        </div>

        <div className="workflow-progress">
          <div className="progress-track">
            <span style={{ width: `${progress}%` }} />
          </div>
          <div className="workflow-steps">
            {TASK_STATUSES.map((status, index) => (
              <div
                key={status}
                className={`workflow-step ${
                  index < currentIndex
                    ? "complete"
                    : index === currentIndex
                      ? "current"
                      : ""
                }`}
              >
                <span>{index < currentIndex ? <Check size={12} /> : index + 1}</span>
                <small>{STATUS_META[status].shortLabel}</small>
              </div>
            ))}
          </div>
        </div>
      </header>

      <div className="task-detail-scroll">
        {stageContent(task, engines, run)}
      </div>
    </section>
  );
}
