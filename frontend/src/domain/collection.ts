import type { TaskPriority } from "./task";

export type CollectionProvider = "plane";
export type CandidateDecision = "pending" | "accepted" | "ignored";
export type CandidateAnalysisMode = "script" | "pi";

export interface PlaneSettings {
  baseUrl: string;
  workspaceSlug: string;
  projectId: string;
  projectName: string;
  projectIdentifier?: string;
  lastCollectedAt?: string;
}

export interface PlaneProject {
  id: string;
  name: string;
  identifier: string;
}

export interface PlaneConnectionSetup {
  baseUrl: string;
  workspaceSlug: string;
  projects: PlaneProject[];
}

export interface PlaneCandidatePayload {
  externalId: string;
  externalKey: string;
  title: string;
  descriptionMarkdown: string;
  sourceMarkdown: string;
  priority: TaskPriority;
  stateName: string;
  stateGroup: string;
  labels: string[];
  assignees: string[];
  createdAt?: string;
  updatedAt?: string;
}

export interface CandidateAnalysis {
  mode: CandidateAnalysisMode;
  title: string;
  summaryMarkdown: string;
  keyPoints: string[];
  openQuestions: string[];
  analyzedAt: string;
}

export interface CollectionCandidate extends PlaneCandidatePayload {
  id: string;
  provider: CollectionProvider;
  projectName: string;
  decision: CandidateDecision;
  fetchedAt: string;
  acceptedTaskId?: string;
  analysis: CandidateAnalysis;
}

export interface PlaneConnectionStatus {
  connected: boolean;
  itemCount: number;
  message: string;
}

export function makePlaneCandidate(
  payload: PlaneCandidatePayload,
  projectName: string,
  existing?: CollectionCandidate,
): CollectionCandidate {
  const fetchedAt = new Date().toISOString();
  const preserveDecision =
    existing?.decision === "accepted" || existing?.decision === "ignored";
  const baseAnalysis: CandidateAnalysis = {
    mode: "script",
    title: payload.title,
    summaryMarkdown:
      payload.descriptionMarkdown ||
      `来自 Plane 的工作项 ${payload.externalKey}，原始描述为空。`,
    keyPoints: [
      payload.stateName ? `当前状态：${payload.stateName}` : "",
      payload.labels.length > 0
        ? `标签：${payload.labels.join("、")}`
        : "",
      payload.assignees.length > 0
        ? `负责人：${payload.assignees.join("、")}`
        : "",
    ].filter(Boolean),
    openQuestions: [],
    analyzedAt: fetchedAt,
  };

  return {
    ...payload,
    id: `plane:${payload.externalId}`,
    provider: "plane",
    projectName,
    decision: preserveDecision ? existing.decision : "pending",
    acceptedTaskId: preserveDecision ? existing.acceptedTaskId : undefined,
    fetchedAt,
    analysis:
      existing?.analysis.mode === "pi" && existing.updatedAt === payload.updatedAt
        ? existing.analysis
        : baseAnalysis,
  };
}
