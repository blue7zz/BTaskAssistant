import { createID } from "../lib/id";
import type {
  ConfirmedFact,
  RequirementQuestion,
  Requirements,
  Task,
} from "./task";

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
    question,
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

  if (!task.projectName.trim()) {
    addQuestionIfMissing(questions, "这项任务对应哪个项目或代码仓库？");
  }
  if (cleanLines(requirements.acceptanceCriteria).length === 0) {
    addQuestionIfMissing(questions, "如何客观判定这项任务已经完成？");
  }
  if (task.evidence.length === 0) {
    addQuestionIfMissing(questions, "当前没有需求来源，真实需求依据是什么？");
  }

  const evidenceFacts: ConfirmedFact[] = task.evidence.map((source) => ({
    id: createID("fact"),
    statement: firstMeaningfulLine(source.content),
    sourceId: source.id,
  }));
  const existingManualFacts = requirements.facts.filter((fact) =>
    task.evidence.some((source) => source.id === fact.sourceId),
  );
  const facts = evidenceFacts.map((fact) => {
    const existing = existingManualFacts.find(
      (candidate) => candidate.sourceId === fact.sourceId,
    );
    return existing ?? fact;
  });

  const objective = requirements.objective.trim() || task.summary.trim();
  const scope = cleanLines(requirements.scope);
  const outOfScope = cleanLines(requirements.outOfScope);
  const acceptanceCriteria = cleanLines(requirements.acceptanceCriteria);
  const risks = cleanLines(requirements.risks);

  const sourceLines = task.evidence.map(
    (source, index) =>
      `- [S${index + 1}] ${source.title}（${source.type}）\n  ${firstMeaningfulLine(source.content)}`,
  );
  const factLines = facts.map((fact) => {
    const sourceIndex = task.evidence.findIndex(
      (source) => source.id === fact.sourceId,
    );
    return `- ${fact.statement} [S${sourceIndex + 1}]`;
  });
  const questionLines = questions.map((question) =>
    question.resolvedAt
      ? `- ${question.question}\n  - 已确认答案：${question.answer}`
      : `- [待确认] ${question.question}`,
  );

  const document = [
    `# ${task.title} — 需求说明`,
    "",
    "## 目标",
    objective || "[等待人工确认]",
    "",
    "## 已确认事实",
    factLines.join("\n") || "- [等待添加有来源的事实]",
    "",
    "## 范围内",
    renderList(scope),
    "",
    "## 范围外",
    renderList(outOfScope, "尚未明确"),
    "",
    "## 验收标准",
    renderList(acceptanceCriteria),
    "",
    "## 待确认问题",
    questionLines.join("\n") || "- 无",
    "",
    "## 风险与约束",
    renderList(risks, "尚未记录"),
    "",
    "## 来源",
    sourceLines.join("\n") || "- [缺少来源]",
  ].join("\n");

  const executionPrompt = [
    `你要处理任务：${task.title}`,
    "",
    "必须遵守：",
    "1. 只根据下方已经人工确认的需求工作，严禁推测、补写或扩大范围。",
    "2. 如发现任何歧义、缺失、冲突或无法验证的信息，立即停止并提出问题。",
    "3. 先检查现状和影响范围，再给出最小实现；不得自动进入下一工作阶段。",
    "4. 完成后列出修改文件、验证命令、实际结果和仍需人工确认的事项。",
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

