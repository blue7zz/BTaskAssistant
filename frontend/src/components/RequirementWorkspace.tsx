import {
  AlertTriangle,
  Bot,
  Check,
  CheckCircle2,
  ClipboardCopy,
  FileText,
  FolderGit2,
  LoaderCircle,
  Plus,
  RefreshCw,
  ShieldAlert,
  Sparkles,
  Trash2,
  Upload,
  UserCheck,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  isQuestionResolved,
  questionStatus,
  type EvidenceType,
  type ForceProceedDecision,
  type ForceProceedStrategy,
  type RequirementQuestion,
  type RequirementQuestionSeverity,
  type Task,
} from "../domain/task";
import { unresolvedQuestions } from "../domain/requirements";
import { copyText } from "../lib/clipboard";
import type { EngineStatus } from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";

interface RequirementWorkspaceProps {
  task: Task;
  engines: EngineStatus[];
  run(
    action: () => void | Promise<void>,
    successMessage?: string,
  ): void;
}

const SEVERITY_LABEL: Record<RequirementQuestionSeverity, string> = {
  BLOCKING: "阻塞",
  IMPORTANT: "重要",
  OPTIONAL: "可选",
};

const STATUS_LABEL = {
  preparing: "准备上下文",
  analyzing: "AI 正在分析",
  waiting_user_answer: "等待用户回答",
  reanalyzing: "AI 正在重新分析",
  ai_suggested_ready: "AI 建议结束访谈",
  force_proceed_confirmation: "确认强制推进风险",
  draft_ready: "需求草稿待审查",
  approved: "需求已人工批准",
  blocked: "需求分析受阻",
  cancelled: "本次访谈已取消",
} as const;

const FORCE_STRATEGIES: Array<{
  value: ForceProceedStrategy;
  label: string;
}> = [
  { value: "KEEP_UNCONFIRMED", label: "保持未确认" },
  { value: "KEEP_EXISTING", label: "沿用项目现有行为" },
  { value: "TEMPORARY_DECISION", label: "采用临时决策" },
  { value: "EXCLUDE_SCOPE", label: "排除相关范围" },
];

function WorkspacePanel({
  title,
  eyebrow,
  actions,
  children,
  className = "",
}: {
  title: string;
  eyebrow?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`requirement-panel ${className}`}>
      <header>
        <div>
          {eyebrow && <span className="eyebrow">{eyebrow}</span>}
          <h3>{title}</h3>
        </div>
        {actions}
      </header>
      {children}
    </section>
  );
}

function CommitField({
  label,
  value,
  placeholder,
  rows,
  disabled,
  onCommit,
}: {
  label: string;
  value: string;
  placeholder?: string;
  rows?: number;
  disabled?: boolean;
  onCommit(value: string): void;
}) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  const commit = () => {
    if (draft !== value) onCommit(draft);
  };
  return (
    <label className="field compact-field">
      <span>{label}</span>
      {rows ? (
        <textarea
          value={draft}
          rows={rows}
          placeholder={placeholder}
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

function readUploadedFile(file: File): Promise<string> {
  if (file.size > 4 * 1024 * 1024) {
    return Promise.reject(new Error("单个补充资料不能超过 4 MB"));
  }
  const supportedImages = new Set([
    "image/png",
    "image/jpeg",
    "image/webp",
    "image/gif",
  ]);
  if (file.type.startsWith("image/") && !supportedImages.has(file.type)) {
    return Promise.reject(new Error("图片仅支持 PNG、JPEG、WebP 或 GIF"));
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("读取补充资料失败"));
    reader.onload = () => {
      const content = String(reader.result ?? "");
      resolve(
        file.type.startsWith("image/")
          ? `![${file.name}](${content})`
          : content,
      );
    };
    if (file.type.startsWith("image/")) reader.readAsDataURL(file);
    else reader.readAsText(file);
  });
}

function ContextColumn({
  task,
  disabled,
  run,
}: {
  task: Task;
  disabled: boolean;
  run: RequirementWorkspaceProps["run"];
}) {
  const updateTaskDetails = useWorkspaceStore(
    (state) => state.updateTaskDetails,
  );
  const addEvidence = useWorkspaceStore((state) => state.addEvidence);
  const removeEvidence = useWorkspaceStore((state) => state.removeEvidence);
  const toggleEvidenceForAnalysis = useWorkspaceStore(
    (state) => state.toggleEvidenceForAnalysis,
  );
  const [adding, setAdding] = useState(false);
  const [type, setType] = useState<EvidenceType>("manual");
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");

  const selectedCount = task.evidence.filter(
    (source) => source.selectedForAnalysis !== false,
  ).length;

  const saveSource = () =>
    run(() => {
      addEvidence(task.id, { type, title, content });
      setTitle("");
      setContent("");
      setAdding(false);
    }, "补充资料已加入本轮分析上下文");

  return (
    <aside className="requirement-column context-column">
      <WorkspacePanel title="任务与项目" eyebrow="固定分析上下文">
        <div className="requirement-panel-body compact-stack">
          <CommitField
            label="任务标题"
            value={task.title}
            disabled={disabled}
            onCommit={(title) =>
              run(() => updateTaskDetails(task.id, { title }))
            }
          />
          <CommitField
            label="原始描述"
            value={task.summary}
            rows={4}
            disabled={disabled}
            onCommit={(summary) =>
              run(() => updateTaskDetails(task.id, { summary }))
            }
          />
          <CommitField
            label="项目名称"
            value={task.projectName}
            placeholder="例如 BTaskAssistant"
            disabled={disabled}
            onCommit={(projectName) =>
              run(() => updateTaskDetails(task.id, { projectName }))
            }
          />
          <CommitField
            label="本地项目目录"
            value={task.projectPath ?? ""}
            placeholder="留空时只分析已选资料"
            disabled={disabled}
            onCommit={(projectPath) =>
              run(() => updateTaskDetails(task.id, { projectPath }))
            }
          />
          <div className="read-only-policy">
            <ShieldAlert size={15} />
            <span>AI 只获得只读项目权限，不能修改代码或 Git 状态。</span>
          </div>
        </div>
      </WorkspacePanel>

      <WorkspacePanel
        title="参与分析的资料"
        eyebrow={`${selectedCount}/${task.evidence.length} 条已选择`}
        actions={
          !disabled && (
            <button
              className="icon-button"
              aria-label="添加资料"
              onClick={() => setAdding((value) => !value)}
            >
              {adding ? <X size={14} /> : <Plus size={14} />}
            </button>
          )
        }
      >
        {adding && (
          <div className="source-adder">
            <label className="field compact-field">
              <span>资料类型</span>
              <select
                value={type}
                onChange={(event) => setType(event.target.value as EvidenceType)}
              >
                <option value="manual">手工说明</option>
                <option value="chat">聊天记录</option>
                <option value="project">项目说明</option>
                <option value="file">文件或图片</option>
              </select>
            </label>
            <CommitField label="资料标题" value={title} onCommit={setTitle} />
            <label className="field compact-field">
              <span>原始内容</span>
              <textarea
                value={content}
                rows={5}
                onChange={(event) => setContent(event.target.value)}
                placeholder="保留原文，不在这里补写推断。"
              />
            </label>
            <label className="file-upload-button">
              <Upload size={13} />
              读取文件或图片
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp,image/gif,.txt,.md,.json,.yaml,.yml,.csv"
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (!file) return;
                  run(async () => {
                    const uploadedContent = await readUploadedFile(file);
                    setType("file");
                    setTitle(file.name);
                    setContent(uploadedContent);
                  }, "资料已读取，请确认后保存");
                  event.currentTarget.value = "";
                }}
              />
            </label>
            <button className="button compact primary" onClick={saveSource}>
              保存资料
            </button>
          </div>
        )}
        <div className="requirement-source-list">
          {task.evidence.length === 0 ? (
            <div className="requirement-empty">尚未添加资料</div>
          ) : (
            task.evidence.map((source, index) => (
              <article
                className={`requirement-source ${
                  source.selectedForAnalysis === false ? "excluded" : ""
                }`}
                key={source.id}
              >
                <label>
                  <input
                    type="checkbox"
                    checked={source.selectedForAnalysis !== false}
                    disabled={disabled}
                    onChange={() =>
                      run(() => toggleEvidenceForAnalysis(task.id, source.id))
                    }
                  />
                  <span>S{index + 1}</span>
                </label>
                <div>
                  <strong>{source.title}</strong>
                  <small>{source.type}</small>
                  <p>{source.content}</p>
                </div>
                {!disabled && (
                  <button
                    className="plain-icon danger"
                    aria-label={`删除资料 ${source.title}`}
                    onClick={() => {
                      if (window.confirm("删除这条资料？已有草稿会失效。")) {
                        run(() => removeEvidence(task.id, source.id));
                      }
                    }}
                  >
                    <Trash2 size={13} />
                  </button>
                )}
              </article>
            ))
          )}
        </div>
      </WorkspacePanel>

      <WorkspacePanel
        title="已确认事实"
        eyebrow={`${task.requirements.facts.length} 条有来源事实`}
      >
        <div className="fact-list">
          {task.requirements.facts.length === 0 ? (
            <div className="requirement-empty">等待首轮分析</div>
          ) : (
            task.requirements.facts.map((fact) => (
              <p key={fact.id}>
                <Check size={12} />
                <span>{fact.statement}</span>
              </p>
            ))
          )}
        </div>
      </WorkspacePanel>
    </aside>
  );
}

function QuestionCard({
  taskID,
  question,
  disabled,
  run,
}: {
  taskID: string;
  question: RequirementQuestion;
  disabled: boolean;
  run: RequirementWorkspaceProps["run"];
}) {
  const answerQuestion = useWorkspaceStore((state) => state.answerQuestion);
  const confirmExistingBehavior = useWorkspaceStore(
    (state) => state.confirmExistingBehavior,
  );
  const skipQuestion = useWorkspaceStore((state) => state.skipQuestion);
  const markQuestionOutOfScope = useWorkspaceStore(
    (state) => state.markQuestionOutOfScope,
  );
  const [answer, setAnswer] = useState(question.answer);
  useEffect(() => setAnswer(question.answer), [question.answer]);
  const severity = question.severity ?? "BLOCKING";
  const status = questionStatus(question);
  const resolved = isQuestionResolved(question);

  const chooseOption = (value: string) => {
    if (question.answerType !== "MULTI_SELECT") {
      setAnswer(value);
      return;
    }
    const values = answer
      .split("；")
      .map((item) => item.trim())
      .filter(Boolean);
    setAnswer(
      values.includes(value)
        ? values.filter((item) => item !== value).join("；")
        : [...values, value].join("；"),
    );
  };

  return (
    <article className={`interview-question severity-${severity.toLowerCase()}`}>
      <input type="hidden" name={`answer:${question.id}`} value={answer} />
      <header>
        <div>
          <span className={`severity-badge severity-${severity.toLowerCase()}`}>
            {SEVERITY_LABEL[severity]}问题
          </span>
          <span className="question-round">第 {question.round ?? 0} 轮</span>
        </div>
        {resolved && <CheckCircle2 size={16} />}
      </header>
      <h4>{question.question}</h4>
      <p className="question-reason">为什么要问：{question.reason}</p>
      {(question.projectEvidence ?? []).map((evidence) => (
        <div className="project-evidence" key={`${evidence.path}-${evidence.lineRange}`}>
          <FolderGit2 size={12} />
          <span>
            {evidence.path}
            {evidence.lineRange ? `:${evidence.lineRange}` : ""} · {evidence.summary}
          </span>
        </div>
      ))}

      {(question.options ?? []).length > 0 && (
        <div className="question-options">
          {question.options?.map((option) => {
            const selected =
              question.answerType === "MULTI_SELECT"
                ? answer.split("；").includes(option)
                : answer === option;
            return (
              <button
                type="button"
                className={selected ? "selected" : ""}
                disabled={disabled}
                key={option}
                onClick={() => chooseOption(option)}
              >
                <span>{selected ? <Check size={11} /> : null}</span>
                {option}
              </button>
            );
          })}
        </div>
      )}

      {(question.allowCustomAnswer !== false ||
        (question.options ?? []).length === 0) && (
        <textarea
          value={answer}
          rows={3}
          disabled={disabled}
          placeholder={
            question.answerType === "FILE_OR_IMAGE"
              ? "先在左侧补充资料，再说明资料名称。"
              : "由你确认答案，AI 不会代答。"
          }
          onChange={(event) => setAnswer(event.target.value)}
        />
      )}

      <footer>
        <span>{status === "SKIPPED" ? "已暂时跳过" : ""}</span>
        <div>
          {!resolved && (
            <button
              type="button"
              className="button compact ghost"
              disabled={disabled}
              onClick={() => run(() => skipQuestion(taskID, question.id))}
            >
              暂时跳过
            </button>
          )}
          <button
            type="button"
            className="button compact ghost"
            disabled={disabled}
            onClick={() =>
              run(
                () => markQuestionOutOfScope(taskID, question.id),
                "已标记为本次不需要考虑",
              )
            }
          >
            不在范围内
          </button>
          {question.answerType === "CONFIRM_EXISTING_BEHAVIOR" && (
            <button
              type="button"
              className="button compact ghost"
              disabled={disabled || !answer.trim()}
              onClick={() =>
                run(
                  () =>
                    confirmExistingBehavior(taskID, question.id, answer),
                  "当前行为快照已保存为用户确认答案",
                )
              }
            >
              按现有实现处理
            </button>
          )}
          <button
            type="button"
            className="button compact secondary"
            disabled={disabled || !answer.trim()}
            onClick={() =>
              run(
                () => answerQuestion(taskID, question.id, answer),
                "回答已保存，尚未触发重新分析",
              )
            }
          >
            保存回答
          </button>
        </div>
      </footer>
    </article>
  );
}

function ForceProceedPanel({
  task,
  run,
}: {
  task: Task;
  run: RequirementWorkspaceProps["run"];
}) {
  const cancelForceProceed = useWorkspaceStore(
    (state) => state.cancelForceProceed,
  );
  const confirmForceProceed = useWorkspaceStore(
    (state) => state.confirmForceProceed,
  );
  const unresolved = unresolvedQuestions(task.requirements);
  const [decisions, setDecisions] = useState<Record<string, ForceProceedDecision>>(
    () =>
      Object.fromEntries(
        unresolved.map((question) => [
          question.id,
          { questionId: question.id, strategy: "KEEP_UNCONFIRMED" },
        ]),
      ),
  );

  return (
    <WorkspacePanel
      title="强制推进确认"
      eyebrow="未解决问题不会被自动回答"
      className="force-proceed-panel"
    >
      <div className="force-warning">
        <AlertTriangle size={17} />
        <p>
          当前仍有 {unresolved.length} 个未解决问题。继续后，它们会写入正式需求；开发执行器遇到未确认内容必须停止。
        </p>
      </div>
      <div className="force-decision-list">
        {unresolved.map((question) => {
          const decision = decisions[question.id];
          const needsDetail =
            decision.strategy === "KEEP_EXISTING" ||
            decision.strategy === "TEMPORARY_DECISION";
          return (
            <div key={question.id}>
              <strong>{question.question}</strong>
              <select
                value={decision.strategy}
                onChange={(event) =>
                  setDecisions((current) => ({
                    ...current,
                    [question.id]: {
                      ...current[question.id],
                      strategy: event.target.value as ForceProceedStrategy,
                    },
                  }))
                }
              >
                {FORCE_STRATEGIES.map((strategy) => (
                  <option value={strategy.value} key={strategy.value}>
                    {strategy.label}
                  </option>
                ))}
              </select>
              {needsDetail && (
                <textarea
                  rows={2}
                  value={decision.detail ?? ""}
                  placeholder={
                    decision.strategy === "KEEP_EXISTING"
                      ? "记录当前行为快照及项目证据"
                      : "填写这次采用的临时决定"
                  }
                  onChange={(event) =>
                    setDecisions((current) => ({
                      ...current,
                      [question.id]: {
                        ...current[question.id],
                        detail: event.target.value,
                      },
                    }))
                  }
                />
              )}
            </div>
          );
        })}
      </div>
      <div className="force-actions">
        <button
          className="button secondary"
          onClick={() => run(() => cancelForceProceed(task.id))}
        >
          返回访谈
        </button>
        <button
          className="button primary"
          onClick={() =>
            run(
              () => confirmForceProceed(task.id, Object.values(decisions)),
              "未确认事项和风险已冻结到需求草稿",
            )
          }
        >
          确认策略并生成草稿
        </button>
      </div>
    </WorkspacePanel>
  );
}

function InterviewColumn({
  task,
  engines,
  disabled,
  run,
}: {
  task: Task;
  engines: EngineStatus[];
  disabled: boolean;
  run: RequirementWorkspaceProps["run"];
}) {
  const setRequirementAnalyst = useWorkspaceStore(
    (state) => state.setRequirementAnalyst,
  );
  const runRequirementAnalysis = useWorkspaceStore(
    (state) => state.runRequirementAnalysis,
  );
  const acceptSuggestedDraft = useWorkspaceStore(
    (state) => state.acceptSuggestedDraft,
  );
  const answerQuestions = useWorkspaceStore((state) => state.answerQuestions);
  const requestForceProceed = useWorkspaceStore(
    (state) => state.requestForceProceed,
  );
  const generateDraft = useWorkspaceStore((state) => state.generateDraft);
  const [focus, setFocus] = useState("");
  const interview = task.requirements.interview;
  const unresolved = unresolvedQuestions(task.requirements);
  const currentEngine = engines.find(
    (engine) => engine.id === interview.analyst,
  );
  const alternativeEngine = engines.find(
    (engine) => engine.id !== interview.analyst && engine.requirementAnalysis,
  );
  const busy = interview.status === "analyzing" || interview.status === "reanalyzing";
  const suggested = interview.suggestedDraft;
  const suggestionCount =
    (suggested.objective ? 1 : 0) +
    suggested.scope.length +
    suggested.outOfScope.length +
    suggested.acceptanceCriteria.length +
    suggested.constraints.length;
  const sortedQuestions = useMemo(
    () =>
      [...task.requirements.questions].sort((a, b) => {
        const order = { BLOCKING: 0, IMPORTANT: 1, OPTIONAL: 2 };
        return order[a.severity ?? "BLOCKING"] - order[b.severity ?? "BLOCKING"];
      }),
    [task.requirements.questions],
  );

  if (interview.status === "force_proceed_confirmation") {
    return (
      <main className="requirement-column interview-column">
        <ForceProceedPanel task={task} run={run} />
      </main>
    );
  }

  return (
    <main className="requirement-column interview-column">
      <div className={`interview-status status-${interview.status}`}>
        <div>
          {busy ? <LoaderCircle className="spin" size={18} /> : <Bot size={18} />}
        </div>
        <div>
          <span className="eyebrow">第 {interview.round} 轮</span>
          <strong>{STATUS_LABEL[interview.status]}</strong>
          <p>
            {interview.blockedReason ||
              interview.lastReason ||
              "AI 负责发现缺口，是否结束访谈和批准需求始终由你决定。"}
          </p>
        </div>
      </div>

      <WorkspacePanel title="访谈控制" eyebrow="可随时切换或交叉复查">
        <div className="analysis-controls">
          <label className="field compact-field">
            <span>主分析器</span>
            <select
              value={interview.analyst}
              disabled={disabled || busy}
              onChange={(event) =>
                run(() =>
                  setRequirementAnalyst(
                    task.id,
                    event.target.value as "pi" | "codex",
                  ),
                )
              }
            >
              {engines.map((engine) => (
                <option value={engine.id} key={engine.id}>
                  {engine.label}{engine.requirementAnalysis ? "" : "（不可用）"}
                </option>
              ))}
            </select>
          </label>
          <button
            className="button primary"
            disabled={disabled || busy || !currentEngine?.requirementAnalysis}
            onClick={() =>
              run(
                () => runRequirementAnalysis(task.id),
                "本轮需求分析已完成，结果仍需人工确认",
              )
            }
          >
            {interview.round === 0 ? <Sparkles size={15} /> : <RefreshCw size={15} />}
            {interview.round === 0 ? "开始第一轮分析" : "根据回答继续分析"}
          </button>
        </div>
        {!currentEngine?.requirementAnalysis && (
          <p className="inline-warning">
            当前运行环境没有可用的 {currentEngine?.label ?? "AI"} CLI；仍可在右侧人工整理并生成草稿。
          </p>
        )}
        {interview.analyses.length > 0 && (
          <div className="analysis-history">
            {interview.analyses.slice(-4).map((analysis) => (
              <span key={`${analysis.id}-${analysis.analyst}`}>
                R{analysis.round} · {analysis.analyst.toUpperCase()} · {analysis.status}
              </span>
            ))}
          </div>
        )}
      </WorkspacePanel>

      {interview.status === "ai_suggested_ready" && (
        <div className="ai-ready-card">
          <CheckCircle2 size={20} />
          <div>
            <strong>AI 认为当前信息足够生成需求草稿</strong>
            <p>
              阻塞问题 {unresolved.filter((item) => item.severity === "BLOCKING").length} 个，
              重要问题 {unresolved.filter((item) => item.severity === "IMPORTANT").length} 个，
              可选问题 {unresolved.filter((item) => item.severity === "OPTIONAL").length} 个。
            </p>
          </div>
          <button
            className="button compact primary"
            onClick={() =>
              run(() => generateDraft(task.id), "需求草稿已生成，等待人工审查")
            }
          >
            生成需求草稿
          </button>
        </div>
      )}

      {suggestionCount > 0 && (
        <WorkspacePanel
          title="AI 草稿建议"
          eyebrow={`${suggestionCount} 项候选，尚未成为需求`}
          actions={
            <button
              className="button compact secondary"
              disabled={disabled || busy}
              onClick={() =>
                run(
                  () => acceptSuggestedDraft(task.id),
                  "候选内容已由你采纳，请在右侧继续核对",
                )
              }
            >
              <UserCheck size={13} />
              采纳到实时草稿
            </button>
          }
        >
          <div className="suggestion-grid">
            {suggested.objective && <p><strong>目标</strong>{suggested.objective}</p>}
            {suggested.scope.map((item) => <p key={`scope-${item}`}><strong>范围内</strong>{item}</p>)}
            {suggested.outOfScope.map((item) => <p key={`out-${item}`}><strong>不做</strong>{item}</p>)}
            {suggested.acceptanceCriteria.map((item) => <p key={`accept-${item}`}><strong>验收</strong>{item}</p>)}
            {suggested.constraints.map((item) => <p key={`risk-${item}`}><strong>约束</strong>{item}</p>)}
          </div>
        </WorkspacePanel>
      )}

      <WorkspacePanel
        title="AI 访谈问题"
        eyebrow={`${unresolved.length} 项仍未解决`}
      >
        {sortedQuestions.length === 0 ? (
          <div className="interview-empty">
            <Bot size={24} />
            <strong>尚未生成访谈问题</strong>
            <p>绑定项目、选择资料，再启动第一轮只读分析。</p>
          </div>
        ) : (
          <form
            className="interview-question-list"
            onSubmit={(event) => {
              event.preventDefault();
              const form = new FormData(event.currentTarget);
              const answers = unresolved.map((question) => ({
                questionId: question.id,
                answer: String(form.get(`answer:${question.id}`) ?? ""),
              }));
              run(
                () => answerQuestions(task.id, answers),
                "本轮已填写答案已批量保存",
              );
            }}
          >
            {sortedQuestions.map((question) => (
              <QuestionCard
                key={question.id}
                taskID={task.id}
                question={question}
                disabled={disabled || busy}
                run={run}
              />
            ))}
            {unresolved.length > 1 && (
              <div className="batch-answer-bar">
                <span>可以先填写多个问题，再一次保存本轮答案。</span>
                <button
                  type="submit"
                  className="button compact secondary"
                  disabled={disabled || busy}
                >
                  批量保存已填答案
                </button>
              </div>
            )}
          </form>
        )}
      </WorkspacePanel>

      {(interview.projectObservations.length > 0 || interview.conflicts.length > 0) && (
        <WorkspacePanel title="项目观察与冲突" eyebrow="不会自动转成用户需求">
          <div className="observation-list">
            {interview.projectObservations.map((observation) => (
              <p key={observation.id}>
                <FolderGit2 size={13} />
                <span>
                  {observation.content}
                  <small>{observation.filePath}{observation.lineRange ? `:${observation.lineRange}` : ""}</small>
                </span>
              </p>
            ))}
            {interview.conflicts.map((conflict) => (
              <p className="conflict" key={conflict.id}>
                <AlertTriangle size={13} />
                <span>{conflict.description}<small>{conflict.sourceA} ↔ {conflict.sourceB}</small></span>
              </p>
            ))}
          </div>
        </WorkspacePanel>
      )}

      {(interview.round > 0 || unresolved.length > 0) && !disabled && (
        <WorkspacePanel title="继续检查" eyebrow="只针对新增关注点追问">
          <div className="review-controls">
            <textarea
              rows={3}
              value={focus}
              onChange={(event) => setFocus(event.target.value)}
              placeholder="例如：再次检查验收标准是否都可以客观验证"
            />
            <div>
              <button
                className="button compact secondary"
                disabled={busy || !focus.trim() || !currentEngine?.requirementAnalysis}
                onClick={() =>
                  run(
                    () => runRequirementAnalysis(task.id, { mode: "review", focus }),
                    "已按指定角度复查",
                  )
                }
              >
                从这个角度复查
              </button>
              {alternativeEngine && (
                <button
                  className="button compact secondary"
                  disabled={busy}
                  onClick={() =>
                    run(
                      () =>
                        runRequirementAnalysis(task.id, {
                          analyst: alternativeEngine.id,
                          mode: "review",
                          focus: focus || "独立检查遗漏、冲突、无来源内容和不可验证的验收标准",
                        }),
                      `${alternativeEngine.label} 独立复查已完成`,
                    )
                  }
                >
                  用 {alternativeEngine.label} 复查
                </button>
              )}
              {unresolved.length > 0 && (
                <button
                  className="button compact danger-button"
                  disabled={busy}
                  onClick={() => run(() => requestForceProceed(task.id))}
                >
                  <AlertTriangle size={13} />
                  强制进入下一步
                </button>
              )}
            </div>
          </div>
        </WorkspacePanel>
      )}
    </main>
  );
}

function DraftColumn({
  task,
  disabled,
  run,
}: {
  task: Task;
  disabled: boolean;
  run: RequirementWorkspaceProps["run"];
}) {
  const patchRequirements = useWorkspaceStore((state) => state.patchRequirements);
  const generateDraft = useWorkspaceStore((state) => state.generateDraft);
  const confirmRequirements = useWorkspaceStore((state) => state.confirmRequirements);
  const revokeRequirements = useWorkspaceStore((state) => state.revokeRequirements);
  const requirements = task.requirements;
  const unresolved = unresolvedQuestions(requirements);
  const toLines = (value: string) =>
    value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);

  return (
    <aside className="requirement-column draft-column">
      <WorkspacePanel title="实时需求草稿" eyebrow="预览不是批准版本">
        <div className="draft-progress">
          <div><span>事实</span><strong>{requirements.facts.length}</strong></div>
          <div><span>验收</span><strong>{requirements.acceptanceCriteria.length}</strong></div>
          <div><span>待确认</span><strong>{unresolved.length}</strong></div>
        </div>
        <div className="requirement-panel-body compact-stack">
          <CommitField
            label="目标"
            value={requirements.objective}
            rows={3}
            disabled={disabled}
            onCommit={(objective) => run(() => patchRequirements(task.id, { objective }))}
          />
          <CommitField
            label="修改范围（每行一项）"
            value={requirements.scope.join("\n")}
            rows={4}
            disabled={disabled}
            onCommit={(value) => run(() => patchRequirements(task.id, { scope: toLines(value) }))}
          />
          <CommitField
            label="明确不做（每行一项）"
            value={requirements.outOfScope.join("\n")}
            rows={3}
            disabled={disabled}
            onCommit={(value) => run(() => patchRequirements(task.id, { outOfScope: toLines(value) }))}
          />
          <CommitField
            label="验收标准（每行一项）"
            value={requirements.acceptanceCriteria.join("\n")}
            rows={4}
            disabled={disabled}
            onCommit={(value) => run(() => patchRequirements(task.id, { acceptanceCriteria: toLines(value) }))}
          />
          <CommitField
            label="技术约束与风险（每行一项）"
            value={requirements.risks.join("\n")}
            rows={3}
            disabled={disabled}
            onCommit={(value) => run(() => patchRequirements(task.id, { risks: toLines(value) }))}
          />
          {!disabled && (
            <button
              className="button secondary"
              onClick={() => run(() => generateDraft(task.id), "结构化需求草稿已生成")}
            >
              <FileText size={14} />
              {requirements.generatedAt ? "重新生成草稿" : "生成结构化草稿"}
            </button>
          )}
        </div>
      </WorkspacePanel>

      {requirements.document ? (
        <WorkspacePanel
          title="正式结构预览"
          eyebrow="包含来源、确认记录和未确认事项"
          actions={
            <button
              className="icon-button"
              aria-label="复制需求文档"
              onClick={() => run(() => copyText(requirements.document), "需求文档已复制")}
            >
              <ClipboardCopy size={14} />
            </button>
          }
        >
          <pre className="requirement-document-preview">{requirements.document}</pre>
        </WorkspacePanel>
      ) : (
        <WorkspacePanel title="正式结构预览" eyebrow="尚未生成">
          <div className="interview-empty small">
            <FileText size={22} />
            <p>完成访谈或选择强制推进策略后生成需求草稿。</p>
          </div>
        </WorkspacePanel>
      )}

      <div className={`requirement-approval ${requirements.confirmedAt ? "approved" : ""}`}>
        {requirements.confirmedAt ? <CheckCircle2 size={22} /> : <UserCheck size={22} />}
        <div>
          <strong>{requirements.confirmedAt ? "需求版本已冻结" : "最终批准必须由你完成"}</strong>
          <p>
            {requirements.confirmedAt
              ? `确认版本 v${requirements.confirmedRevision} · 已保留 ${requirements.approvedRevisions.length} 个历史快照`
              : "批准前请核对来源、范围、验收标准和所有未确认事项。"}
          </p>
        </div>
        {!requirements.confirmedAt && (
          <button
            className="button compact primary"
            disabled={disabled || !requirements.document}
            onClick={() =>
              run(
                () => confirmRequirements(task.id),
                "需求已人工批准并冻结；任务仍需手动推进",
              )
            }
          >
            人工批准这版需求
          </button>
        )}
        {requirements.confirmedAt && (
          <button
            className="button compact secondary"
            onClick={() =>
              run(
                () => revokeRequirements(task.id),
                "已撤销当前批准，可以创建新的需求草稿",
              )
            }
          >
            撤销批准并继续修改
          </button>
        )}
      </div>
    </aside>
  );
}

export function RequirementWorkspace({
  task,
  engines,
  run,
}: RequirementWorkspaceProps) {
  const locked = Boolean(task.requirements.confirmedAt);
  const busy =
    task.requirements.interview.status === "analyzing" ||
    task.requirements.interview.status === "reanalyzing";

  return (
    <div className="requirement-workspace">
      <ContextColumn task={task} disabled={locked || busy} run={run} />
      <InterviewColumn
        task={task}
        engines={engines}
        disabled={locked}
        run={run}
      />
      <DraftColumn task={task} disabled={locked || busy} run={run} />
    </div>
  );
}
