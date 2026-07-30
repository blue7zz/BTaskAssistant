import { createID } from "../lib/id";
import type {
  ConfirmedFact,
  RequirementQuestion,
  Requirements,
  Task,
} from "./task";
import { questionStatus } from "./task";

const cleanLines = (values: string[]) =>
  values.map((value) => value.trim()).filter(Boolean);

function firstMeaningfulLine(content: string): string {
  const line = content
    .split(/\r?\n/)
    .map((candidate) => candidate.trim())
    .find(Boolean);
  if (!line) return "（来源内容为空）";
  return line.length > 220 ? `${line.slice(0, 220)}…` : line;
}

function addQuestionIfMissing(
  questions: RequirementQuestion[],
  question: string,
): void {
  if (questions.some((item) => item.question === question)) return;
  questions.push({
    id: createID("question"),
    round: 0,
    category: "OTHER",
    severity: "BLOCKING",
    question,
    reason: "缺少这项信息会影响需求范围或验收结果。",
    sourceIds: [],
    projectEvidence: [],
    answerType: "TEXT",
    options: [],
    allowCustomAnswer: true,
    status: "OPEN",
    answer: "",
  });
}

function renderList(values: string[], emptyLabel = "等待人工确认"): string {
  if (values.length === 0) return `- [${emptyLabel}]`;
  return values.map((value) => `- ${value}`).join("\n");
}

export function buildRequirementDraft(task: Task): Requirements {
  const now = new Date().toISOString();
  const requirements = task.requirements;
  const questions = requirements.questions.map((question) => ({ ...question }));
  const analysisSources = task.evidence.filter(
    (source) => source.selectedForAnalysis !== false,
  );

  if (!task.projectName.trim()) {
    addQuestionIfMissing(questions, "这项任务对应哪个项目或代码仓库？");
  }
  if (cleanLines(requirements.acceptanceCriteria).length === 0) {
    addQuestionIfMissing(questions, "如何客观判定这项任务已经完成？");
  }
  if (analysisSources.length === 0) {
    addQuestionIfMissing(questions, "当前没有需求来源，真实需求依据是什么？");
  }

  const facts: ConfirmedFact[] = requirements.facts.filter((fact) =>
    analysisSources.some((source) => source.id === fact.sourceId),
  );
  for (const source of analysisSources) {
    if (facts.some((fact) => fact.sourceId === source.id)) continue;
    facts.push({
      id: createID("fact"),
      statement: firstMeaningfulLine(source.content),
      sourceId: source.id,
      sourceIds: [source.id],
    });
  }

  const objective = requirements.objective.trim() || task.summary.trim();
  const scope = cleanLines(requirements.scope);
  const outOfScope = cleanLines(requirements.outOfScope);
  const acceptanceCriteria = cleanLines(requirements.acceptanceCriteria);
  const risks = cleanLines(requirements.risks);

  const sourceLines = task.evidence.map(
    (source, index) =>
      `- [S${index + 1}] ${source.title}（${source.type}${
        source.selectedForAnalysis === false ? "，未参与本轮分析" : ""
      }）\n  ${firstMeaningfulLine(source.content)}`,
  );
  const factLines = facts.map((fact) => {
    const sourceIndex = task.evidence.findIndex(
      (source) => source.id === fact.sourceId,
    );
    return sourceIndex >= 0
      ? `- ${fact.statement} [S${sourceIndex + 1}]`
      : `- ${fact.statement}`;
  });
  const answeredQuestions = questions.filter((question) => {
    const status = questionStatus(question);
    return status === "ANSWERED" || status === "OUT_OF_SCOPE";
  });
  const unresolved = questions.filter((question) => {
    const status = questionStatus(question);
    return status === "OPEN" || status === "SKIPPED";
  });
  const answerLines = answeredQuestions.map(
    (question) =>
      `- ${question.question}\n  - 用户确认：${question.answer}`,
  );
  const unresolvedLines = unresolved.map((question) => {
    const decision = question.forceDecision
      ? `\n  - 强制推进策略：${question.forceDecision}`
      : "";
    const note = question.forceDecisionNote
      ? `\n  - 处理说明：${question.forceDecisionNote}`
      : "";
    return `- ${question.id} ${question.question}\n  - 状态：未确认\n  - 级别：${question.severity ?? "BLOCKING"}${decision}${note}`;
  });
  const observationLines = requirements.interview.projectObservations.map(
    (observation) =>
      `- ${observation.content}\n  - 项目位置：${observation.filePath}${
        observation.lineRange ? `:${observation.lineRange}` : ""
      }\n  - 类型：PROJECT_OBSERVATION`,
  );
  const conflictLines = requirements.interview.conflicts.map(
    (conflict) =>
      `- ${conflict.description}（${conflict.sourceA} ↔ ${conflict.sourceB}）`,
  );

  const document = [
    `# ${task.title} — 需求说明`,
    "",
    "## 1. 背景",
    task.summary.trim() || "[等待人工确认]",
    "",
    "### 来源索引",
    sourceLines.join("\n") || "- [缺少来源]",
    "",
    "## 2. 当前问题",
    objective || "[等待人工确认]",
    "",
    "## 3. 已确认需求",
    factLines.join("\n") || "- [等待添加有来源的事实]",
    "",
    "## 4. 修改范围",
    renderList(scope),
    "",
    "## 5. 明确不做",
    renderList(outOfScope, "尚未明确"),
    "",
    "## 6. 交互和业务规则",
    answerLines.join("\n") || "- [尚无用户确认记录]",
    "",
    "## 7. 技术约束",
    renderList(risks, "尚未记录"),
    "",
    "## 8. 验收标准",
    renderList(acceptanceCriteria),
    "",
    "## 9. 项目证据",
    observationLines.join("\n") || "- [尚无只读项目观察]",
    "",
    "## 10. 用户确认记录",
    answerLines.join("\n") || "- [尚无逐项回答]",
    "",
    "## 11. 未确认事项",
    unresolvedLines.join("\n") || "- 无",
    "",
    "## 12. 风险",
    [...risks.map((risk) => `- ${risk}`), ...conflictLines].join("\n") ||
      "- [尚未记录]",
  ].join("\n");

  const executionPrompt = [
    `你要处理任务：${task.title}`,
    "",
    "必须遵守：",
    "1. 只根据下方已经人工确认的需求工作，严禁推测、补写或扩大范围。",
    "2. 如发现任何歧义、缺失、冲突或无法验证的信息，立即停止并提出问题。",
    "3. 先检查现状和影响范围，再给出最小实现；不得自动进入下一工作阶段。",
    "4. 完成后列出修改文件、验证命令、实际结果和仍需人工确认的事项。",
    "5. “未确认事项”中的内容不得自行决定；若实现触及 KEEP_UNCONFIRMED 项，必须停止并请求确认。",
    "",
    "以下为待人工确认的需求文档：",
    "",
    document,
  ].join("\n");

  return {
    ...requirements,
    objective,
    scope,
    outOfScope,
    acceptanceCriteria,
    risks,
    facts,
    questions,
    document,
    executionPrompt,
    generatedAt: now,
    confirmedAt: undefined,
    confirmedRevision: undefined,
  };
}
