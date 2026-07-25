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
    description: "只根据证据生成候选稿",
  },
  approved: {
    label: "待开发",
    shortLabel: "确认",
    description: "需求已由人工确认",
  },
  development: {
    label: "开发中",
    shortLabel: "开发",
    description: "委托 Codex 或 PI 执行",
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
}

export interface ConfirmedFact {
  id: string;
  statement: string;
  sourceId: string;
}

export interface RequirementQuestion {
  id: string;
  question: string;
  answer: string;
  resolvedAt?: string;
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

export interface CreateTaskInput {
  title: string;
  summary: string;
  projectName?: string;
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
  };
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
