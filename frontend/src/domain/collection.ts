import type { TaskPriority } from "./task";

export type CollectionProvider = "plane";
export type CandidateDecision = "pending" | "accepted" | "ignored";
export type CandidateAnalysisMode = "script" | "pi";

export interface PlaneSettings {
  baseUrl: string;
  workspaceSlug: string;
  projectId: string;
  projectName: string;
  showInTaskSources: boolean;
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

export interface PlanePerson {
  id: string;
  name: string;
}

export interface PlaneComment {
  id: string;
  bodyMarkdown: string;
  actor: PlanePerson;
  createdAt?: string;
  updatedAt?: string;
  editedAt?: string;
}

export interface PlaneWorkItemReference {
  externalId: string;
  externalKey: string;
  title: string;
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
  assigneeDetails?: PlanePerson[];
  comments?: PlaneComment[];
  commentsSyncError?: string;
  parent?: PlaneWorkItemReference;
  detailsLoaded?: boolean;
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
  detailsLoaded: boolean;
  draftEdited: boolean;
  collectionRevision: string;
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
  collectionRevision?: string,
): CollectionCandidate {
  const fetchedAt = new Date().toISOString();
  const reuseLoadedDetails = Boolean(
    payload.detailsLoaded === false &&
      existing?.detailsLoaded &&
      (!payload.updatedAt ||
        !existing.updatedAt ||
        existing.updatedAt === payload.updatedAt),
  );
  const candidatePayload: PlaneCandidatePayload =
    reuseLoadedDetails && existing
      ? {
          ...payload,
          descriptionMarkdown: existing.descriptionMarkdown,
          sourceMarkdown: existing.sourceMarkdown,
          comments: existing.comments,
          commentsSyncError: existing.commentsSyncError,
          parent: existing.parent,
          detailsLoaded: true,
        }
      : payload;
  const comments = candidatePayload.comments ?? [];
  const detailsLoaded = candidatePayload.detailsLoaded ?? true;
  const preserveDecision =
    existing?.decision === "accepted" || existing?.decision === "ignored";
  const baseAnalysis: CandidateAnalysis = {
    mode: "script",
    title: candidatePayload.title,
    summaryMarkdown:
      candidatePayload.descriptionMarkdown ||
      `来自 Plane 的工作项 ${candidatePayload.externalKey}，详情尚未加载。`,
    keyPoints: [
      candidatePayload.stateName
        ? `当前状态：${candidatePayload.stateName}`
        : "",
      candidatePayload.labels.length > 0
        ? `标签：${candidatePayload.labels.join("、")}`
        : "",
      candidatePayload.assignees.length > 0
        ? `负责人：${candidatePayload.assignees.join("、")}`
        : "",
      comments.length > 0 ? `Plane 评论：${comments.length} 条` : "",
    ].filter(Boolean),
    openQuestions: candidatePayload.commentsSyncError
      ? ["Plane 评论没有完整同步，请点击重试后再确认。"]
      : [],
    analyzedAt: fetchedAt,
  };
  let analysis = baseAnalysis;
  if (existing && (reuseLoadedDetails || existing.draftEdited)) {
    analysis = existing.analysis;
  } else if (
    existing?.analysis.mode === "pi" &&
    existing.updatedAt === candidatePayload.updatedAt
  ) {
    analysis = existing.analysis;
  }

  return {
    ...candidatePayload,
    assigneeDetails: candidatePayload.assigneeDetails ?? [],
    comments,
    detailsLoaded,
    draftEdited: existing?.draftEdited ?? false,
    collectionRevision:
      collectionRevision ?? existing?.collectionRevision ?? fetchedAt,
    id: `plane:${candidatePayload.externalId}`,
    provider: "plane",
    projectName,
    decision: preserveDecision ? existing.decision : "pending",
    acceptedTaskId: preserveDecision ? existing.acceptedTaskId : undefined,
    fetchedAt,
    analysis,
  };
}

export function planeWorkspaceAddress(settings: PlaneSettings): string {
  const baseUrl = settings.baseUrl.trim().replace(/\/+$/, "");
  const workspaceSlug = settings.workspaceSlug.trim();
  if (!baseUrl || !workspaceSlug) return baseUrl;
  try {
    const parsed = new URL(baseUrl);
    if (parsed.hostname === "api.plane.so") {
      parsed.hostname = "app.plane.so";
    }
    return `${parsed.origin}/${encodeURIComponent(workspaceSlug)}/`;
  } catch {
    return `${baseUrl}/${encodeURIComponent(workspaceSlug)}/`;
  }
}

export function planeWorkItemURL(
  settings: PlaneSettings,
  externalKey: string,
): string {
  return `${planeWorkspaceAddress(settings)}browse/${encodeURIComponent(
    externalKey.trim(),
  )}`;
}
