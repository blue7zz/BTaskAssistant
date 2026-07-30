import {
  createEmptyRequirementInterview,
  questionStatus,
  type Evidence,
  type ForceProceedDecision,
  type ProjectEvidence,
  type RequirementAnalysisStatus,
  type RequirementAnalyst,
  type RequirementAnswerType,
  type RequirementConflict,
  type RequirementDraftUpdates,
  type RequirementInterview,
  type RequirementProjectObservation,
  type RequirementQuestion,
  type RequirementQuestionCategory,
  type RequirementQuestionSeverity,
  type Requirements,
  type Task,
} from "./task";

export interface AnalysisMaterial {
  materialId: string;
  type: Evidence["type"];
  relationship: "PRIMARY_SOURCE" | "SUPPORTING_SOURCE";
  title: string;
  fragments: Array<{
    fragmentId: string;
    content: string;
    sourceLocation: string;
  }>;
}

export interface RequirementAnalysisInput {
  task: {
    id: string;
    title: string;
    originalDescription: string;
    currentStatus: string;
  };
  project: {
    name: string;
    localPath: string;
  };
  materials: AnalysisMaterial[];
  previousAnswers: Array<{
    questionId: string;
    question: string;
    answer: string;
    status: string;
    answeredBy: "user";
  }>;
  openQuestions: RequirementQuestion[];
  existingRequirementRevision: {
    version: number;
    objective: string;
    scope: string[];
    outOfScope: string[];
    acceptanceCriteria: string[];
    risks: string[];
    content: string;
  };
  analysisPolicy: {
    allowAssumption: false;
    requireSourceForFact: true;
    allowCodeWrite: false;
    allowProjectRead: true;
    askWhenAmbiguous: true;
    forceProceedRequested: boolean;
  };
  analyst: RequirementAnalyst;
  round: number;
  mode: "analyze" | "review";
  focus: string;
}

export interface RequirementAnalysisFact {
  id: string;
  content: string;
  sourceFragmentIds: string[];
}

export interface RequirementAnalysisQuestion {
  id: string;
  category: RequirementQuestionCategory;
  severity: RequirementQuestionSeverity;
  question: string;
  reason: string;
  sourceFragmentIds: string[];
  projectEvidence: ProjectEvidence[];
  answerType: RequirementAnswerType;
  options: string[];
  allowCustomAnswer: boolean;
}

export interface RequirementAnalysisResult {
  analysisId: string;
  round: number;
  confirmedFacts: RequirementAnalysisFact[];
  projectObservations: RequirementProjectObservation[];
  questions: RequirementAnalysisQuestion[];
  conflicts: RequirementConflict[];
  draftUpdates: RequirementDraftUpdates;
  analysisStatus: RequirementAnalysisStatus;
  recommendedAction: string;
  reason: string;
  analyzedAt: string;
}

export interface RunRequirementAnalysisOptions {
  analyst?: RequirementAnalyst;
  mode?: "analyze" | "review";
  focus?: string;
}

export function buildRequirementAnalysisInput(
  task: Task,
  options: RunRequirementAnalysisOptions = {},
): RequirementAnalysisInput {
  const analyst = options.analyst ?? task.requirements.interview.analyst;
  const materials = task.evidence
    .filter((source) => source.selectedForAnalysis !== false)
    .map((source, index) => ({
      materialId: source.id,
      type: source.type,
      relationship: (index === 0
        ? "PRIMARY_SOURCE"
        : "SUPPORTING_SOURCE") as AnalysisMaterial["relationship"],
      title: source.title,
      fragments: [
        {
          fragmentId: source.id,
          content: source.content,
          sourceLocation: source.title,
        },
      ],
    }));
  const previousAnswers = task.requirements.questions
    .filter((question) => {
      const status = questionStatus(question);
      return status === "ANSWERED" || status === "OUT_OF_SCOPE";
    })
    .map((question) => ({
      questionId: question.id,
      question: question.question,
      answer: question.answer,
      status: questionStatus(question),
      answeredBy: "user" as const,
    }));
  const openQuestions = task.requirements.questions.filter((question) => {
    const status = questionStatus(question);
    return status === "OPEN" || status === "SKIPPED";
  });

  return {
    task: {
      id: task.id,
      title: task.title,
      originalDescription: task.summary,
      currentStatus: task.requirements.interview.status,
    },
    project: {
      name: task.projectName,
      localPath: task.projectPath?.trim() ?? "",
    },
    materials,
    previousAnswers,
    openQuestions,
    existingRequirementRevision: {
      version: task.revision,
      objective: task.requirements.objective,
      scope: task.requirements.scope,
      outOfScope: task.requirements.outOfScope,
      acceptanceCriteria: task.requirements.acceptanceCriteria,
      risks: task.requirements.risks,
      content: task.requirements.document,
    },
    analysisPolicy: {
      allowAssumption: false,
      requireSourceForFact: true,
      allowCodeWrite: false,
      allowProjectRead: true,
      askWhenAmbiguous: true,
      forceProceedRequested: Boolean(
        task.requirements.interview.forceProceed?.requestedAt,
      ),
    },
    analyst,
    round: task.requirements.interview.round + 1,
    mode: options.mode ?? "analyze",
    focus: options.focus?.trim() ?? "",
  };
}

export function normalizeRequirementQuestion(
  question: RequirementQuestion,
): RequirementQuestion {
  return {
    ...question,
    category: question.category ?? "OTHER",
    severity: question.severity ?? "BLOCKING",
    reason: question.reason ?? "固定模板发现这项信息尚未由用户确认。",
    sourceIds: question.sourceIds ?? [],
    projectEvidence: question.projectEvidence ?? [],
    answerType: question.answerType ?? "TEXT",
    options: question.options ?? [],
    allowCustomAnswer: question.allowCustomAnswer ?? true,
    status: questionStatus(question),
  };
}

export function normalizeRequirementInterview(
  interview?: Partial<RequirementInterview>,
): RequirementInterview {
  const empty = createEmptyRequirementInterview();
  return {
    ...empty,
    ...interview,
    analyses: interview?.analyses ?? [],
    projectObservations: interview?.projectObservations ?? [],
    conflicts: interview?.conflicts ?? [],
    suggestedDraft: {
      ...empty.suggestedDraft,
      ...interview?.suggestedDraft,
    },
  };
}

export function normalizeRequirements(
  requirements?: Partial<Requirements>,
): Requirements {
  const empty: Requirements = {
    objective: "",
    scope: [],
    outOfScope: [],
    acceptanceCriteria: [],
    facts: [],
    questions: [],
    risks: [],
    document: "",
    executionPrompt: "",
    approvedRevisions: [],
    interview: createEmptyRequirementInterview(),
  };
  const merged = { ...empty, ...requirements };
  return {
    ...merged,
    scope: merged.scope ?? [],
    outOfScope: merged.outOfScope ?? [],
    acceptanceCriteria: merged.acceptanceCriteria ?? [],
    facts: merged.facts ?? [],
    questions: (merged.questions ?? []).map(normalizeRequirementQuestion),
    risks: merged.risks ?? [],
    approvedRevisions: merged.approvedRevisions ?? [],
    interview: normalizeRequirementInterview(requirements?.interview),
  };
}

export function unresolvedQuestions(
  requirements: Requirements,
): RequirementQuestion[] {
  return requirements.questions.filter((question) => {
    const status = questionStatus(question);
    return status === "OPEN" || status === "SKIPPED";
  });
}

export function isApprovalBlockingQuestion(
  question: RequirementQuestion,
  forceDecisions: ForceProceedDecision[] = [],
): boolean {
  const status = questionStatus(question);
  if (
    question.severity !== "BLOCKING" ||
    (status !== "OPEN" && status !== "SKIPPED")
  ) {
    return false;
  }
  return !forceDecisions.some(
    (decision) => decision.questionId === question.id,
  );
}

export function mergeUnique(values: string[], additions: string[]): string[] {
  const seen = new Set<string>();
  return [...values, ...additions].filter((value) => {
    const normalized = value.trim().toLocaleLowerCase();
    if (!normalized || seen.has(normalized)) return false;
    seen.add(normalized);
    return true;
  });
}
