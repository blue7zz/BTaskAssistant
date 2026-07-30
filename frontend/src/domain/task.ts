export const TASK_STATUSES = [
  "inbox",
  "requirements",
  "approved",
  "development",
  "review",
  "done",
] as const;

export type TaskStatus = (typeof TASK_STATUSES)[number];
export type TaskPriority = "low" | "medium" | "high";
export type EvidenceType = "manual" | "chat" | "project" | "file" | "plane";
export type DevelopmentEngine = "codex" | "pi";
export type DevelopmentState = "idle" | "delegated" | "completed";
export type ReviewMode = "manual" | "template";
export type RequirementAnalyst = "pi" | "codex";
export type RequirementInterviewStatus =
  | "preparing"
  | "analyzing"
  | "waiting_user_answer"
  | "reanalyzing"
  | "ai_suggested_ready"
  | "force_proceed_confirmation"
  | "draft_ready"
  | "approved"
  | "blocked"
  | "cancelled";
export type RequirementAnalysisStatus =
  | "NEEDS_USER_INPUT"
  | "NO_BLOCKING_QUESTIONS"
  | "READY_FOR_DRAFT"
  | "INSUFFICIENT_MATERIALS"
  | "PROJECT_UNAVAILABLE"
  | "ANALYSIS_FAILED";
export type RequirementQuestionSeverity = "BLOCKING" | "IMPORTANT" | "OPTIONAL";
export type RequirementQuestionCategory =
  | "SCOPE"
  | "BEHAVIOR"
  | "ACCEPTANCE"
  | "CONSTRAINT"
  | "CONFLICT"
  | "MATERIAL"
  | "OTHER";
export type RequirementAnswerType =
  | "TEXT"
  | "SINGLE_SELECT"
  | "MULTI_SELECT"
  | "BOOLEAN"
  | "FILE_OR_IMAGE"
  | "PROJECT_REFERENCE"
  | "CONFIRM_EXISTING_BEHAVIOR";
export type RequirementQuestionStatus =
  | "OPEN"
  | "ANSWERED"
  | "SKIPPED"
  | "OUT_OF_SCOPE";
export type ForceProceedStrategy =
  | "KEEP_UNCONFIRMED"
  | "KEEP_EXISTING"
  | "TEMPORARY_DECISION"
  | "EXCLUDE_SCOPE";

export const STATUS_META: Record<
  TaskStatus,
  { label: string; shortLabel: string; description: string }
> = {
  inbox: {
    label: "任务池",
    shortLabel: "收集",
    description: "整理任务与原始资料",
  },
  requirements: {
    label: "需求整理",
    shortLabel: "需求",
    description: "手工补充任务记录",
  },
  approved: {
    label: "待开发",
    shortLabel: "确认",
    description: "需求已由人工确认",
  },
  development: {
    label: "开发中",
    shortLabel: "开发",
    description: "记录开发进度与结果",
  },
  review: {
    label: "待审核",
    shortLabel: "审核",
    description: "人工审核与最小修复",
  },
  done: {
    label: "已完成",
    shortLabel: "完成",
    description: "审核通过并归档",
  },
};

export interface Evidence {
  id: string;
  type: EvidenceType;
  title: string;
  content: string;
  createdAt: string;
  selectedForAnalysis?: boolean;
}

export interface ConfirmedFact {
  id: string;
  statement: string;
  sourceId: string;
  sourceIds?: string[];
}

export interface ProjectEvidence {
  path: string;
  summary: string;
  lineRange?: string;
}

export interface RequirementQuestion {
  id: string;
  round?: number;
  category?: RequirementQuestionCategory;
  severity?: RequirementQuestionSeverity;
  question: string;
  reason?: string;
  sourceIds?: string[];
  projectEvidence?: ProjectEvidence[];
  answerType?: RequirementAnswerType;
  options?: string[];
  allowCustomAnswer?: boolean;
  status?: RequirementQuestionStatus;
  answer: string;
  answerSource?: "user" | "project_snapshot" | "force_proceed";
  resolvedAt?: string;
  forceDecision?: ForceProceedStrategy;
  forceDecisionNote?: string;
}

export interface RequirementProjectObservation {
  id: string;
  content: string;
  filePath: string;
  lineRange?: string;
}

export interface RequirementConflict {
  id: string;
  description: string;
  sourceA: string;
  sourceB: string;
}

export interface RequirementDraftUpdates {
  objective?: string;
  scope: string[];
  outOfScope: string[];
  acceptanceCriteria: string[];
  constraints: string[];
}

export interface RequirementAnalysisRecord {
  id: string;
  round: number;
  analyst: RequirementAnalyst;
  mode: "analyze" | "review";
  status: RequirementAnalysisStatus;
  reason: string;
  questionIds: string[];
  createdAt: string;
}

export interface ForceProceedDecision {
  questionId: string;
  strategy: ForceProceedStrategy;
  detail?: string;
}

export interface RequirementForceProceed {
  requestedAt: string;
  confirmedAt?: string;
  decisions: ForceProceedDecision[];
}

export interface RequirementInterview {
  id?: string;
  status: RequirementInterviewStatus;
  analyst: RequirementAnalyst;
  round: number;
  analyses: RequirementAnalysisRecord[];
  projectObservations: RequirementProjectObservation[];
  conflicts: RequirementConflict[];
  suggestedDraft: RequirementDraftUpdates;
  lastAnalysisStatus?: RequirementAnalysisStatus;
  lastReason?: string;
  lastFocus?: string;
  forceProceed?: RequirementForceProceed;
  blockedReason?: string;
}

export interface ApprovedRequirementRevision {
  version: number;
  document: string;
  executionPrompt: string;
  confirmedAt: string;
}

export interface Requirements {
  objective: string;
  scope: string[];
  outOfScope: string[];
  acceptanceCriteria: string[];
  facts: ConfirmedFact[];
  questions: RequirementQuestion[];
  risks: string[];
  document: string;
  executionPrompt: string;
  generatedAt?: string;
  confirmedAt?: string;
  confirmedRevision?: number;
  approvedRevisions: ApprovedRequirementRevision[];
  interview: RequirementInterview;
}

export interface DevelopmentRecord {
  engine: DevelopmentEngine;
  state: DevelopmentState;
  delegatedAt?: string;
  completedAt?: string;
  resultNote: string;
}

export interface ReviewItem {
  id: string;
  label: string;
  checked: boolean;
}

export interface ReviewRecord {
  mode: ReviewMode;
  checklist: ReviewItem[];
  note: string;
  approvedAt?: string;
}

export interface Task {
  id: string;
  title: string;
  summary: string;
  projectName: string;
  projectPath?: string;
  priority: TaskPriority;
  status: TaskStatus;
  evidence: Evidence[];
  requirements: Requirements;
  development: DevelopmentRecord;
  review: ReviewRecord;
  revision: number;
  createdAt: string;
  updatedAt: string;
}

export interface TrashedTask extends Task {
  trashedAt: string;
}

export interface CreateTaskInput {
  title: string;
  summary: string;
  projectName?: string;
  projectPath?: string;
  priority?: TaskPriority;
  initialEvidence?: {
    type: EvidenceType;
    title: string;
    content: string;
  };
}

export function createEmptyRequirements(): Requirements {
  return {
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
}

export function createEmptyRequirementInterview(): RequirementInterview {
  return {
    status: "preparing",
    analyst: "pi",
    round: 0,
    analyses: [],
    projectObservations: [],
    conflicts: [],
    suggestedDraft: {
      scope: [],
      outOfScope: [],
      acceptanceCriteria: [],
      constraints: [],
    },
  };
}

export function questionStatus(
  question: RequirementQuestion,
): RequirementQuestionStatus {
  if (question.status) return question.status;
  return question.resolvedAt && question.answer.trim() ? "ANSWERED" : "OPEN";
}

export function isQuestionResolved(question: RequirementQuestion): boolean {
  const status = questionStatus(question);
  return status === "ANSWERED" || status === "OUT_OF_SCOPE";
}

export function createEmptyDevelopment(): DevelopmentRecord {
  return {
    engine: "codex",
    state: "idle",
    resultNote: "",
  };
}

export function createEmptyReview(): ReviewRecord {
  return {
    mode: "manual",
    checklist: [],
    note: "",
  };
}

export function statusIndex(status: TaskStatus): number {
  return TASK_STATUSES.indexOf(status);
}
