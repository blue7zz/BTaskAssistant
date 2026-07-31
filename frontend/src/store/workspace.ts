import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import {
  makePlaneCandidate,
  type CandidateAnalysis,
  type CollectionCandidate,
  type PlaneCandidatePayload,
  type PlaneSettings,
} from "../domain/collection";
import { buildRequirementDraft } from "../domain/templates";
import {
  DEFAULT_PI_SETTINGS,
  normalizePISettings,
  type PISettings,
} from "../domain/engine";
import {
  DEFAULT_DAILY_REPORT_AI_SETTINGS,
  createEmptyDailyReportDraft,
  localDateString,
  normalizeDailyReportAISettings,
  normalizeDailyReportDraft,
  normalizeDailyReportSettings,
  type DailyReportAISettings,
  type DailyReportDraft,
  type DailyReportGenerationProject,
  type DailyReportProjectHistoryItem,
  type DailyReportSettings,
} from "../domain/report";
import {
  buildRequirementAnalysisInput,
  isApprovalBlockingQuestion,
  mergeUnique,
  normalizeRequirements,
  unresolvedQuestions,
  type RequirementAnalysisResult,
  type RunRequirementAnalysisOptions,
} from "../domain/requirements";
import {
  createEmptyDevelopment,
  createEmptyRequirements,
  createEmptyReview,
  type CreateTaskInput,
  type DevelopmentEngine,
  type Evidence,
  type EvidenceType,
  type ForceProceedDecision,
  type RequirementAnalyst,
  type Requirements,
  type ReviewMode,
  type Task,
  type TaskPriority,
  type TaskStatus,
  type TrashedTask,
} from "../domain/task";
import { createID } from "../lib/id";
import {
  analyzeRequirements,
  validateTransition,
  workspaceStorage,
} from "../lib/bridge";

export type StatusFilter = TaskStatus | "all";

export interface PlaneCandidateFilters {
  state: string;
  assignees: string[];
}

export const DEFAULT_PLANE_CANDIDATE_FILTERS: PlaneCandidateFilters = {
  state: "all",
  assignees: [],
};

export type DailyReportDrafts = Record<string, DailyReportDraft>;

const MAX_DAILY_REPORT_PROJECT_HISTORY = 20;

const INITIAL_DAILY_REPORT_DATE = localDateString();

function normalizeDailyReportDrafts(
  drafts?: DailyReportDrafts,
  legacyDraft?: DailyReportDraft,
): DailyReportDrafts {
  const normalized: DailyReportDrafts = {};
  if (drafts && typeof drafts === "object") {
    for (const [date, draft] of Object.entries(drafts)) {
      if (!draft || typeof draft !== "object") continue;
      const normalizedDraft = normalizeDailyReportDraft({
        ...draft,
        date: draft.date || date,
      });
      normalized[normalizedDraft.date] = normalizedDraft;
    }
  }
  if (legacyDraft) {
    const normalizedLegacy = normalizeDailyReportDraft(legacyDraft);
    if (!normalized[normalizedLegacy.date]) {
      normalized[normalizedLegacy.date] = normalizedLegacy;
    }
  }
  return normalized;
}

function normalizeDailyReportProjectPath(path: string): string {
  const trimmed = path.trim();
  if (trimmed === "/" || /^[A-Za-z]:[\\/]$/.test(trimmed)) return trimmed;
  return trimmed.replace(/[\\/]+$/, "");
}

function normalizeDailyReportProjectHistory(
  items?: Array<Partial<DailyReportProjectHistoryItem>>,
): DailyReportProjectHistoryItem[] {
  if (!Array.isArray(items)) return [];
  const byPath = new Map<string, DailyReportProjectHistoryItem>();
  for (const item of items) {
    const path = normalizeDailyReportProjectPath(
      typeof item?.path === "string" ? item.path : "",
    );
    if (!path || byPath.has(path)) continue;
    byPath.set(path, {
      id:
        typeof item.id === "string" && item.id.trim()
          ? item.id
          : createID("report-project-history"),
      projectNo:
        typeof item.projectNo === "string" ? item.projectNo.trim() : "",
      projectName:
        typeof item.projectName === "string" ? item.projectName.trim() : "",
      path,
      lastUsedAt:
        typeof item.lastUsedAt === "string" ? item.lastUsedAt.trim() : "",
    });
  }
  return Array.from(byPath.values())
    .sort((left, right) => right.lastUsedAt.localeCompare(left.lastUsedAt))
    .slice(0, MAX_DAILY_REPORT_PROJECT_HISTORY);
}

export const DEFAULT_PLANE_SETTINGS: PlaneSettings = {
  baseUrl: "https://plane.fymyriad.com",
  workspaceSlug: "myriad",
  projectId: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
  projectName: "myriad",
  showInTaskSources: true,
  projectIdentifier: "MYRIA",
};

function migratePlaneSettings(
  settings?: Partial<PlaneSettings>,
): PlaneSettings {
  if (
    !settings?.baseUrl?.trim() &&
    !settings?.workspaceSlug?.trim() &&
    !settings?.projectId?.trim()
  ) {
    return { ...DEFAULT_PLANE_SETTINGS };
  }
  const merged = {
    ...DEFAULT_PLANE_SETTINGS,
    ...settings,
  };
  if (
    merged.projectId === DEFAULT_PLANE_SETTINGS.projectId &&
    !merged.projectIdentifier?.trim()
  ) {
    merged.projectIdentifier =
      DEFAULT_PLANE_SETTINGS.projectIdentifier;
  }
  return merged;
}

function normalizePlaneCandidateFilters(
  filters?: Partial<PlaneCandidateFilters>,
): PlaneCandidateFilters {
  const state = filters?.state?.trim() || DEFAULT_PLANE_CANDIDATE_FILTERS.state;
  const assignees = Array.isArray(filters?.assignees)
    ? Array.from(
        new Set(filters.assignees.map((value) => value.trim()).filter(Boolean)),
      )
    : [];
  return { state, assignees };
}

function migrateCandidateDraftEdited(
  candidate: Partial<CollectionCandidate>,
): boolean {
  if (typeof candidate.draftEdited === "boolean") {
    return candidate.draftEdited;
  }
  if (!candidate.analysis) return false;
  const defaultSummary =
    candidate.descriptionMarkdown ||
    `来自 Plane 的工作项 ${candidate.externalKey ?? ""}，详情尚未加载。`;
  return (
    candidate.analysis.mode === "pi" ||
    candidate.analysis.title !== (candidate.title ?? "") ||
    candidate.analysis.summaryMarkdown !== defaultSummary
  );
}

type EditableRequirementFields = Pick<
  Requirements,
  "objective" | "scope" | "outOfScope" | "acceptanceCriteria" | "risks"
>;

interface WorkspaceState {
  tasks: Task[];
  trashedTasks: TrashedTask[];
  collectionCandidates: CollectionCandidate[];
  planeSettings: PlaneSettings;
  planeCandidateFilters: PlaneCandidateFilters;
  piSettings: PISettings;
  dailyReportSettings: DailyReportSettings;
  dailyReportAISettings: DailyReportAISettings;
  dailyReportProjectHistory: DailyReportProjectHistoryItem[];
  dailyReportDate: string;
  dailyReportDrafts: DailyReportDrafts;
  selectedTaskId?: string;
  statusFilter: StatusFilter;
  hydrated: boolean;

  setHydrated(hydrated: boolean): void;
  setStatusFilter(status: StatusFilter): void;
  selectTask(taskID?: string): void;
  updatePlaneSettings(patch: Partial<PlaneSettings>): void;
  updatePlaneCandidateFilters(patch: Partial<PlaneCandidateFilters>): void;
  updatePISettings(patch: Partial<PISettings>): void;
  updateDailyReportSettings(patch: Partial<DailyReportSettings>): void;
  updateDailyReportAISettings(patch: Partial<DailyReportAISettings>): void;
  rememberDailyReportProjects(projects: DailyReportGenerationProject[]): void;
  updateDailyReportProject(
    projectID: string,
    project: DailyReportGenerationProject,
  ): void;
  removeDailyReportProject(projectID: string): void;
  selectDailyReportDate(date: string): void;
  updateDailyReportDraft(patch: Partial<DailyReportDraft>): void;
  resetDailyReportDraft(): void;
  ingestPlaneCandidates(payloads: PlaneCandidatePayload[]): number;
  hydratePlaneCandidate(
    payload: PlaneCandidatePayload,
    expected: {
      candidateID: string;
      collectionRevision: string;
      projectID: string;
    },
  ): boolean;
  applyCandidateAnalysis(
    candidateID: string,
    analysis: CandidateAnalysis,
  ): void;
  updateCandidateDraft(
    candidateID: string,
    patch: Pick<CandidateAnalysis, "title" | "summaryMarkdown">,
  ): void;
  acceptCandidate(candidateID: string): string;
  setCandidateIgnored(candidateID: string, ignored: boolean): void;
  createTask(input: CreateTaskInput): string;
  importChat(input: {
    title?: string;
    content: string;
    projectName?: string;
  }): string;
  updateTaskDetails(
    taskID: string,
    patch: Partial<
      Pick<
        Task,
        "title" | "summary" | "projectName" | "projectPath" | "priority"
      >
    >,
  ): void;
  updateTaskRecord(
    taskID: string,
    patch: Partial<
      Pick<Task, "title" | "summary" | "projectName" | "priority" | "status">
    >,
  ): void;
  moveTaskToTrash(taskID: string): void;
  restoreTask(taskID: string): void;
  deleteTaskPermanently(taskID: string): void;
  setTaskStatus(taskID: string, status: TaskStatus): void;
  addEvidence(
    taskID: string,
    input: { type: EvidenceType; title: string; content: string },
  ): void;
  removeEvidence(taskID: string, evidenceID: string): void;
  toggleEvidenceForAnalysis(taskID: string, evidenceID: string): void;
  patchRequirements(
    taskID: string,
    patch: Partial<EditableRequirementFields>,
  ): void;
  generateDraft(taskID: string): void;
  answerQuestion(taskID: string, questionID: string, answer: string): void;
  answerQuestions(
    taskID: string,
    answers: Array<{ questionId: string; answer: string }>,
  ): void;
  confirmExistingBehavior(
    taskID: string,
    questionID: string,
    snapshot: string,
  ): void;
  skipQuestion(taskID: string, questionID: string): void;
  markQuestionOutOfScope(taskID: string, questionID: string): void;
  setRequirementAnalyst(taskID: string, analyst: RequirementAnalyst): void;
  runRequirementAnalysis(
    taskID: string,
    options?: RunRequirementAnalysisOptions,
  ): Promise<void>;
  acceptSuggestedDraft(taskID: string): void;
  requestForceProceed(taskID: string): void;
  cancelForceProceed(taskID: string): void;
  confirmForceProceed(
    taskID: string,
    decisions: ForceProceedDecision[],
  ): void;
  confirmRequirements(taskID: string): void;
  revokeRequirements(taskID: string): void;
  transitionTask(taskID: string, to: TaskStatus): Promise<void>;
  setDevelopmentEngine(taskID: string, engine: DevelopmentEngine): void;
  markDelegated(taskID: string): void;
  setDevelopmentResult(taskID: string, resultNote: string): void;
  markDevelopmentCompleted(taskID: string): void;
  createReviewChecklist(taskID: string, mode: ReviewMode): void;
  toggleReviewItem(taskID: string, itemID: string): void;
  setReviewNote(taskID: string, note: string): void;
  approveReview(taskID: string): void;
}

function now(): string {
  return new Date().toISOString();
}

function makeEvidence(input: {
  type: EvidenceType;
  title: string;
  content: string;
}): Evidence {
  return {
    id: createID("source"),
    type: input.type,
    title: input.title.trim(),
    content: input.content.trim(),
    createdAt: now(),
    selectedForAnalysis: true,
  };
}

function invalidateDraft(requirements: Requirements): Requirements {
  return {
    ...requirements,
    document: "",
    executionPrompt: "",
    generatedAt: undefined,
    confirmedAt: undefined,
    confirmedRevision: undefined,
    interview: {
      ...requirements.interview,
      status:
        requirements.interview.round > 0
          ? "waiting_user_answer"
          : "preparing",
      forceProceed: undefined,
    },
  };
}

function ensureRequirementEditable(task: Task): void {
  if (task.status !== "requirements") {
    throw new Error("只有在“需求整理”阶段才能修改需求");
  }
  if (task.requirements.confirmedAt) {
    throw new Error("需求已确认；请先手动撤销确认再修改");
  }
}

function updateTask(
  tasks: Task[],
  taskID: string,
  updater: (task: Task) => Task,
): Task[] {
  let found = false;
  const updatedTasks = tasks.map((task) => {
    if (task.id !== taskID) return task;
    found = true;
    const next = updater(task);
    return {
      ...next,
      revision: task.revision + 1,
      updatedAt: now(),
    };
  });
  if (!found) throw new Error("没有找到这个任务");
  return updatedTasks;
}

function taskGates(task: Task) {
  return {
    requirementsConfirmed: Boolean(task.requirements.confirmedAt),
    developmentCompleted: task.development.state === "completed",
    reviewApproved: Boolean(task.review.approvedAt),
  };
}

function firstLine(content: string): string {
  return (
    content
      .split(/\r?\n/)
      .map((line) => line.trim())
      .find(Boolean) ?? "导入的聊天任务"
  );
}

function analysisInterviewStatus(
  status: RequirementAnalysisResult["analysisStatus"],
) {
  if (status === "NEEDS_USER_INPUT") return "waiting_user_answer" as const;
  if (status === "NO_BLOCKING_QUESTIONS" || status === "READY_FOR_DRAFT") {
    return "ai_suggested_ready" as const;
  }
  return "blocked" as const;
}

function questionKey(value: string): string {
  return value.replace(/[\s，。？！、,.?!:：;；]/g, "").toLocaleLowerCase();
}

function mergeAnalysisResult(
  task: Task,
  result: RequirementAnalysisResult,
  analyst: RequirementAnalyst,
  mode: "analyze" | "review",
  focus: string,
): Task {
  const questionIds: string[] = [];
  const questions = [...task.requirements.questions];
  for (const candidate of result.questions) {
    const existing = questions.find(
      (question) => questionKey(question.question) === questionKey(candidate.question),
    );
    if (existing) {
      questionIds.push(existing.id);
      continue;
    }
    const id = createID("question");
    questionIds.push(id);
    questions.push({
      id,
      round: result.round,
      category: candidate.category,
      severity: candidate.severity,
      question: candidate.question,
      reason: candidate.reason,
      sourceIds: candidate.sourceFragmentIds,
      projectEvidence: candidate.projectEvidence,
      answerType: candidate.answerType,
      options: candidate.options,
      allowCustomAnswer: candidate.allowCustomAnswer,
      status: "OPEN",
      answer: "",
    });
  }

  const facts = [...task.requirements.facts];
  for (const candidate of result.confirmedFacts) {
    if (
      facts.some(
        (fact) =>
          questionKey(fact.statement) === questionKey(candidate.content),
      )
    ) {
      continue;
    }
    facts.push({
      id: candidate.id || createID("fact"),
      statement: candidate.content,
      sourceId: candidate.sourceFragmentIds[0],
      sourceIds: candidate.sourceFragmentIds,
    });
  }

  const projectObservations = [
    ...task.requirements.interview.projectObservations,
  ];
  for (const observation of result.projectObservations) {
    if (
      projectObservations.some(
        (existing) =>
          existing.filePath === observation.filePath &&
          questionKey(existing.content) === questionKey(observation.content),
      )
    ) {
      continue;
    }
    projectObservations.push(observation);
  }

  const conflicts = [...task.requirements.interview.conflicts];
  for (const conflict of result.conflicts) {
    if (
      conflicts.some(
        (existing) =>
          questionKey(existing.description) === questionKey(conflict.description),
      )
    ) {
      continue;
    }
    conflicts.push(conflict);
  }

  return {
    ...task,
    requirements: {
      ...task.requirements,
      facts,
      questions,
      interview: {
        ...task.requirements.interview,
        status: analysisInterviewStatus(result.analysisStatus),
        round: result.round,
        analyses: [
          ...task.requirements.interview.analyses,
          {
            id: result.analysisId,
            round: result.round,
            analyst,
            mode,
            status: result.analysisStatus,
            reason: result.reason,
            questionIds,
            createdAt: result.analyzedAt,
          },
        ],
        projectObservations,
        conflicts,
        suggestedDraft: {
          objective:
            result.draftUpdates.objective?.trim() ||
            task.requirements.interview.suggestedDraft.objective,
          scope: mergeUnique(
            task.requirements.interview.suggestedDraft.scope,
            result.draftUpdates.scope,
          ),
          outOfScope: mergeUnique(
            task.requirements.interview.suggestedDraft.outOfScope,
            result.draftUpdates.outOfScope,
          ),
          acceptanceCriteria: mergeUnique(
            task.requirements.interview.suggestedDraft.acceptanceCriteria,
            result.draftUpdates.acceptanceCriteria,
          ),
          constraints: mergeUnique(
            task.requirements.interview.suggestedDraft.constraints,
            result.draftUpdates.constraints,
          ),
        },
        lastAnalysisStatus: result.analysisStatus,
        lastReason: result.reason,
        lastFocus: focus,
        forceProceed: undefined,
        blockedReason:
          analysisInterviewStatus(result.analysisStatus) === "blocked"
            ? result.reason
            : undefined,
      },
    },
  };
}

export const useWorkspaceStore = create<WorkspaceState>()(
  persist(
    (set, get) => ({
      tasks: [],
      trashedTasks: [],
      collectionCandidates: [],
      planeSettings: DEFAULT_PLANE_SETTINGS,
      planeCandidateFilters: DEFAULT_PLANE_CANDIDATE_FILTERS,
      piSettings: DEFAULT_PI_SETTINGS,
      dailyReportSettings: normalizeDailyReportSettings(),
      dailyReportAISettings: { ...DEFAULT_DAILY_REPORT_AI_SETTINGS },
      dailyReportProjectHistory: [],
      dailyReportDate: INITIAL_DAILY_REPORT_DATE,
      dailyReportDrafts: {
        [INITIAL_DAILY_REPORT_DATE]: createEmptyDailyReportDraft(
          INITIAL_DAILY_REPORT_DATE,
        ),
      },
      selectedTaskId: undefined,
      statusFilter: "all",
      hydrated: false,

      setHydrated: (hydrated) => set({ hydrated }),
      setStatusFilter: (statusFilter) => set({ statusFilter }),
      selectTask: (selectedTaskId) => set({ selectedTaskId }),
      updatePlaneSettings: (patch) =>
        set((state) => ({
          planeSettings: { ...state.planeSettings, ...patch },
        })),
      updatePlaneCandidateFilters: (patch) =>
        set((state) => ({
          planeCandidateFilters: normalizePlaneCandidateFilters({
            ...state.planeCandidateFilters,
            ...patch,
          }),
        })),
      updatePISettings: (patch) =>
        set((state) => ({
          piSettings: normalizePISettings({ ...state.piSettings, ...patch }),
        })),
      updateDailyReportSettings: (patch) =>
        set((state) => {
          const next = { ...state.dailyReportSettings, ...patch };
          if (
            patch.level &&
            patch.level !== "L3" &&
            state.dailyReportSettings.level === "L3" &&
            patch.role === undefined &&
            next.role === "TL"
          ) {
            next.role = "FE";
          }
          return {
            dailyReportSettings: normalizeDailyReportSettings(next),
          };
        }),
      updateDailyReportAISettings: (patch) =>
        set((state) => ({
          dailyReportAISettings: normalizeDailyReportAISettings({
            ...state.dailyReportAISettings,
            ...patch,
          }),
        })),
      rememberDailyReportProjects: (projects) =>
        set((state) => {
          const usedAt = now();
          const existingByPath = new Map(
            state.dailyReportProjectHistory.map((project) => [
              project.path,
              project,
            ]),
          );
          const remembered = projects.map((project) => {
            const path = normalizeDailyReportProjectPath(project.path);
            const existing = existingByPath.get(path);
            return {
              id: existing?.id ?? createID("report-project-history"),
              projectNo: project.projectNo.trim(),
              projectName: project.projectName.trim(),
              path,
              lastUsedAt: usedAt,
            };
          });
          return {
            dailyReportProjectHistory: normalizeDailyReportProjectHistory([
              ...remembered,
              ...state.dailyReportProjectHistory,
            ]),
          };
        }),
      updateDailyReportProject: (projectID, project) =>
        set((state) => {
          const existing = state.dailyReportProjectHistory.find(
            (item) => item.id === projectID,
          );
          const path = normalizeDailyReportProjectPath(project.path);
          if (!existing || !path) return {};
          return {
            dailyReportProjectHistory: normalizeDailyReportProjectHistory([
              {
                ...existing,
                projectNo: project.projectNo.trim(),
                projectName: project.projectName.trim(),
                path,
              },
              ...state.dailyReportProjectHistory.filter(
                (item) => item.id !== projectID,
              ),
            ]),
          };
        }),
      removeDailyReportProject: (projectID) =>
        set((state) => ({
          dailyReportProjectHistory: state.dailyReportProjectHistory.filter(
            (project) => project.id !== projectID,
          ),
        })),
      selectDailyReportDate: (dailyReportDate) =>
        set((state) => {
          if (state.dailyReportDrafts[dailyReportDate]) {
            return { dailyReportDate };
          }
          return {
            dailyReportDate,
            dailyReportDrafts: {
              ...state.dailyReportDrafts,
              [dailyReportDate]: createEmptyDailyReportDraft(dailyReportDate),
            },
          };
        }),
      updateDailyReportDraft: (patch) =>
        set((state) => {
          const date = state.dailyReportDate;
          const current =
            state.dailyReportDrafts[date] ?? createEmptyDailyReportDraft(date);
          const next = { ...current, ...patch, date };
          return {
            dailyReportDrafts: {
              ...state.dailyReportDrafts,
              [date]: next,
            },
          };
        }),
      resetDailyReportDraft: () =>
        set((state) => ({
          dailyReportDrafts: {
            ...state.dailyReportDrafts,
            [state.dailyReportDate]: createEmptyDailyReportDraft(
              state.dailyReportDate,
            ),
          },
        })),
      ingestPlaneCandidates: (payloads) => {
        const state = get();
        const existingByID = new Map(
          state.collectionCandidates.map((candidate) => [
            candidate.externalId,
            candidate,
          ]),
        );
        const projectName =
          state.planeSettings.projectName.trim() ||
          `Plane · ${state.planeSettings.projectId.trim()}`;
        const collectionRevision = createID("plane-collection");
        const incoming = payloads.map((payload) =>
          makePlaneCandidate(
            payload,
            projectName,
            existingByID.get(payload.externalId),
            collectionRevision,
          ),
        );
        const incomingIDs = new Set(incoming.map((item) => item.externalId));
        const retained = state.collectionCandidates.filter(
          (item) =>
            !incomingIDs.has(item.externalId) &&
            (item.decision === "accepted" || item.decision === "ignored"),
        );
        const lastCollectedAt = new Date().toISOString();
        set({
          collectionCandidates: [...incoming, ...retained],
          planeSettings: {
            ...state.planeSettings,
            lastCollectedAt,
          },
        });
        return incoming.filter((item) => item.decision === "pending").length;
      },
      hydratePlaneCandidate: (payload, expected) => {
        const state = get();
        const existing = state.collectionCandidates.find(
          (candidate) => candidate.externalId === payload.externalId,
        );
        if (
          !existing ||
          existing.id !== expected.candidateID ||
          existing.collectionRevision !== expected.collectionRevision ||
          state.planeSettings.projectId !== expected.projectID
        ) {
          return false;
        }
        const hydrated = makePlaneCandidate(
          { ...payload, detailsLoaded: true },
          existing.projectName,
          existing,
        );
        set({
          collectionCandidates: state.collectionCandidates.map((candidate) =>
            candidate.id === existing.id ? hydrated : candidate,
          ),
        });
        return true;
      },
      applyCandidateAnalysis: (candidateID, analysis) =>
        set((state) => ({
          collectionCandidates: state.collectionCandidates.map((candidate) =>
            candidate.id === candidateID
              ? {
                  ...candidate,
                  analysis: {
                    ...analysis,
                    title: analysis.title.trim() || candidate.title,
                    summaryMarkdown:
                      analysis.summaryMarkdown.trim() ||
                      candidate.descriptionMarkdown,
                  },
                }
              : candidate,
          ),
        })),
      updateCandidateDraft: (candidateID, patch) =>
        set((state) => ({
          collectionCandidates: state.collectionCandidates.map((candidate) =>
            candidate.id === candidateID
              ? {
                  ...candidate,
                  analysis: {
                    ...candidate.analysis,
                    title: patch.title,
                    summaryMarkdown: patch.summaryMarkdown,
                  },
                  draftEdited: true,
                }
              : candidate,
          ),
        })),
      acceptCandidate: (candidateID) => {
        const candidate = get().collectionCandidates.find(
          (item) => item.id === candidateID,
        );
        if (!candidate) throw new Error("没有找到这条收集候选");
        if (candidate.decision !== "pending") {
          throw new Error("这条候选已经处理");
        }
        if (!candidate.detailsLoaded) {
          throw new Error("请先加载 Plane 详情和评论再确认");
        }
        if (candidate.commentsSyncError) {
          throw new Error("Plane 评论尚未完整同步，请重试后再确认");
        }
        if (!candidate.title.trim()) {
          throw new Error("请先确认候选任务标题");
        }
        if (!candidate.descriptionMarkdown.trim()) {
          throw new Error("请先确认候选任务说明");
        }

        const taskID = get().createTask({
          title: candidate.title,
          summary: candidate.descriptionMarkdown,
          projectName: candidate.projectName,
          priority: candidate.priority,
          initialEvidence: {
            type: "plane",
            title: `Plane 原始工作项 ${candidate.externalKey}`,
            content: candidate.sourceMarkdown,
          },
        });
        set((state) => ({
          collectionCandidates: state.collectionCandidates.map((item) =>
            item.id === candidateID
              ? {
                  ...item,
                  decision: "accepted",
                  acceptedTaskId: taskID,
                }
              : item,
          ),
        }));
        return taskID;
      },
      setCandidateIgnored: (candidateID, ignored) =>
        set((state) => ({
          collectionCandidates: state.collectionCandidates.map((candidate) => {
            if (candidate.id !== candidateID) return candidate;
            if (candidate.decision === "accepted") {
              throw new Error("已转换成正式任务的候选不能忽略");
            }
            return {
              ...candidate,
              decision: ignored ? "ignored" : "pending",
            };
          }),
        })),

      createTask: (input) => {
        const timestamp = now();
        const id = createID("task");
        const evidence = input.initialEvidence
          ? [makeEvidence(input.initialEvidence)]
          : [];
        const task: Task = {
          id,
          title: input.title.trim(),
          summary: input.summary.trim(),
          projectName: input.projectName?.trim() ?? "",
          projectPath: input.projectPath?.trim() ?? "",
          priority: input.priority ?? "medium",
          status: "inbox",
          evidence,
          requirements: createEmptyRequirements(),
          development: createEmptyDevelopment(),
          review: createEmptyReview(),
          revision: 1,
          createdAt: timestamp,
          updatedAt: timestamp,
        };
        set((state) => ({
          tasks: [task, ...state.tasks],
          selectedTaskId: id,
          statusFilter: "all",
        }));
        return id;
      },

      importChat: (input) => {
        const content = input.content.trim();
        if (!content) throw new Error("请先粘贴聊天记录");
        const suggestedTitle = firstLine(content).slice(0, 48);
        return get().createTask({
          title: input.title?.trim() || suggestedTitle,
          summary: "从聊天记录导入，等待人工整理。",
          projectName: input.projectName,
          initialEvidence: {
            type: "chat",
            title: "原始聊天记录",
            content,
          },
        });
      },

      updateTaskDetails: (taskID, patch) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (
              task.status !== "inbox" &&
              task.status !== "requirements"
            ) {
              throw new Error("开发开始后不能直接修改任务基础信息");
            }
            if (task.requirements.confirmedAt) {
              throw new Error("需求已确认；请先撤销确认");
            }
            return {
              ...task,
              ...patch,
              requirements:
                task.status === "requirements"
                  ? invalidateDraft(task.requirements)
                  : task.requirements,
            };
          }),
        })),

      updateTaskRecord: (taskID, patch) => {
        if (patch.title !== undefined && !patch.title.trim()) {
          throw new Error("任务标题不能为空");
        }
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => ({
            ...task,
            ...patch,
            title: patch.title?.trim() ?? task.title,
            projectName: patch.projectName?.trim() ?? task.projectName,
          })),
        }));
      },

      moveTaskToTrash: (taskID) =>
        set((state) => {
          const task = state.tasks.find((candidate) => candidate.id === taskID);
          if (!task) throw new Error("没有找到这个任务");
          const trashedAt = now();
          return {
            tasks: state.tasks.filter((candidate) => candidate.id !== taskID),
            trashedTasks: [
              {
                ...task,
                revision: task.revision + 1,
                updatedAt: trashedAt,
                trashedAt,
              },
              ...state.trashedTasks,
            ],
            selectedTaskId:
              state.selectedTaskId === taskID ? undefined : state.selectedTaskId,
          };
        }),

      restoreTask: (taskID) =>
        set((state) => {
          const task = state.trashedTasks.find(
            (candidate) => candidate.id === taskID,
          );
          if (!task) throw new Error("回收站中没有这个任务");
          const { trashedAt: _trashedAt, ...restoredTask } = task;
          return {
            tasks: [
              {
                ...restoredTask,
                revision: task.revision + 1,
                updatedAt: now(),
              },
              ...state.tasks,
            ],
            trashedTasks: state.trashedTasks.filter(
              (candidate) => candidate.id !== taskID,
            ),
            selectedTaskId: taskID,
            statusFilter: "all",
          };
        }),

      deleteTaskPermanently: (taskID) =>
        set((state) => {
          if (!state.trashedTasks.some((task) => task.id === taskID)) {
            throw new Error("回收站中没有这个任务");
          }
          return {
            trashedTasks: state.trashedTasks.filter(
              (task) => task.id !== taskID,
            ),
            collectionCandidates: state.collectionCandidates.map((candidate) =>
              candidate.acceptedTaskId === taskID
                ? {
                    ...candidate,
                    decision: "pending",
                    acceptedTaskId: undefined,
                  }
                : candidate,
            ),
          };
        }),

      setTaskStatus: (taskID, status) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => ({
            ...task,
            status,
          })),
        })),

      addEvidence: (taskID, input) => {
        if (!input.title.trim() || !input.content.trim()) {
          throw new Error("来源标题和内容都不能为空");
        }
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (
              task.status !== "inbox" &&
              task.status !== "requirements"
            ) {
              throw new Error("只能在任务池或需求整理阶段补充资料");
            }
            if (task.requirements.confirmedAt) {
              throw new Error("需求已确认；请先撤销确认再补充资料");
            }
            return {
              ...task,
              evidence: [...task.evidence, makeEvidence(input)],
              requirements: invalidateDraft(task.requirements),
            };
          }),
        }));
      },

      removeEvidence: (taskID, evidenceID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (
              task.status !== "inbox" &&
              task.status !== "requirements"
            ) {
              throw new Error("只能在任务池或需求整理阶段删除资料");
            }
            if (task.requirements.confirmedAt) {
              throw new Error("需求已确认；请先撤销确认再删除资料");
            }
            return {
              ...task,
              evidence: task.evidence.filter(
                (source) => source.id !== evidenceID,
              ),
              requirements: invalidateDraft({
                ...task.requirements,
                facts: task.requirements.facts.filter(
                  (fact) => fact.sourceId !== evidenceID,
                ),
              }),
            };
          }),
        })),

      toggleEvidenceForAnalysis: (taskID, evidenceID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              evidence: task.evidence.map((source) =>
                source.id === evidenceID
                  ? {
                      ...source,
                      selectedForAnalysis: source.selectedForAnalysis === false,
                    }
                  : source,
              ),
              requirements: invalidateDraft(task.requirements),
            };
          }),
        })),

      patchRequirements: (taskID, patch) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: invalidateDraft({
                ...task.requirements,
                ...patch,
              }),
            };
          }),
        })),

      generateDraft: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            const requirements = buildRequirementDraft(task);
            return {
              ...task,
              requirements: {
                ...requirements,
                interview: {
                  ...requirements.interview,
                  status: "draft_ready",
                },
              },
            };
          }),
        })),

      answerQuestion: (taskID, questionID, answer) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: invalidateDraft({
                ...task.requirements,
                questions: task.requirements.questions.map((question) =>
                  question.id === questionID
                    ? {
                        ...question,
                        answer,
                        status: answer.trim() ? "ANSWERED" : "OPEN",
                        answerSource: answer.trim() ? "user" : undefined,
                        resolvedAt: answer.trim() ? now() : undefined,
                        forceDecision: undefined,
                        forceDecisionNote: undefined,
                      }
                    : question,
                ),
              }),
            };
          }),
        })),

      answerQuestions: (taskID, answers) => {
        const answerByQuestion = new Map(
          answers
            .filter((answer) => answer.answer.trim())
            .map((answer) => [answer.questionId, answer.answer]),
        );
        if (answerByQuestion.size === 0) {
          throw new Error("请至少填写一个问题答案");
        }
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: invalidateDraft({
                ...task.requirements,
                questions: task.requirements.questions.map((question) => {
                  const answer = answerByQuestion.get(question.id);
                  return answer === undefined
                    ? question
                    : {
                        ...question,
                        answer,
                        status: "ANSWERED",
                        answerSource: "user",
                        resolvedAt: now(),
                        forceDecision: undefined,
                        forceDecisionNote: undefined,
                      };
                }),
              }),
            };
          }),
        }));
      },

      confirmExistingBehavior: (taskID, questionID, snapshot) => {
        if (!snapshot.trim()) {
          throw new Error("请先记录当前行为快照或项目证据");
        }
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: invalidateDraft({
                ...task.requirements,
                questions: task.requirements.questions.map((question) =>
                  question.id === questionID
                    ? {
                        ...question,
                        answer: snapshot,
                        status: "ANSWERED",
                        answerSource: "project_snapshot",
                        resolvedAt: now(),
                        forceDecision: undefined,
                        forceDecisionNote: undefined,
                      }
                    : question,
                ),
              }),
            };
          }),
        }));
      },

      skipQuestion: (taskID, questionID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: invalidateDraft({
                ...task.requirements,
                questions: task.requirements.questions.map((question) =>
                  question.id === questionID
                    ? {
                        ...question,
                        status: "SKIPPED",
                        answer: "",
                        resolvedAt: undefined,
                      }
                    : question,
                ),
              }),
            };
          }),
        })),

      markQuestionOutOfScope: (taskID, questionID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: invalidateDraft({
                ...task.requirements,
                questions: task.requirements.questions.map((question) =>
                  question.id === questionID
                    ? {
                        ...question,
                        status: "OUT_OF_SCOPE",
                        answer: "用户明确标记为本次不需要考虑",
                        answerSource: "user",
                        resolvedAt: now(),
                      }
                    : question,
                ),
              }),
            };
          }),
        })),

      setRequirementAnalyst: (taskID, analyst) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: {
                ...task.requirements,
                interview: {
                  ...task.requirements.interview,
                  analyst,
                },
              },
            };
          }),
        })),

      runRequirementAnalysis: async (taskID, options = {}) => {
        const task = get().tasks.find((candidate) => candidate.id === taskID);
        if (!task) throw new Error("没有找到这个任务");
        ensureRequirementEditable(task);
        if (
          task.requirements.interview.status === "analyzing" ||
          task.requirements.interview.status === "reanalyzing"
        ) {
          throw new Error("需求分析正在进行，请等待本轮完成");
        }
        if (!task.projectName.trim()) {
          throw new Error("请先绑定项目");
        }
        if (
          !task.summary.trim() &&
          !task.evidence.some((source) => source.selectedForAnalysis !== false)
        ) {
          throw new Error("至少需要任务说明或一条参与分析的资料");
        }
        const analyst = options.analyst ?? task.requirements.interview.analyst;
        const mode = options.mode ?? "analyze";
        const focus = options.focus?.trim() ?? "";
        const input = buildRequirementAnalysisInput(task, {
          analyst,
          mode,
          focus,
        });

        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (current) => ({
            ...current,
            requirements: {
              ...current.requirements,
              interview: {
                ...current.requirements.interview,
                status:
                  current.requirements.interview.round === 0
                    ? "analyzing"
                    : "reanalyzing",
                round: input.round,
                blockedReason: undefined,
              },
            },
          })),
        }));

        try {
          const result = await analyzeRequirements(input, get().piSettings);
          const latest = get().tasks.find(
            (candidate) => candidate.id === taskID,
          );
          if (
            !latest ||
            latest.status !== "requirements" ||
            latest.requirements.confirmedAt ||
            latest.requirements.interview.round !== input.round
          ) {
            throw new Error("任务状态已变化，本轮分析结果未保存");
          }
          set((state) => ({
            tasks: updateTask(state.tasks, taskID, (current) =>
              mergeAnalysisResult(current, result, analyst, mode, focus),
            ),
          }));
        } catch (error) {
          const message =
            error instanceof Error ? error.message : "需求分析执行失败";
          const latest = get().tasks.find(
            (candidate) => candidate.id === taskID,
          );
          if (
            latest?.status === "requirements" &&
            latest.requirements.interview.round === input.round
          ) {
            set((state) => ({
              tasks: updateTask(state.tasks, taskID, (current) => ({
                ...current,
                requirements: {
                  ...current.requirements,
                  interview: {
                    ...current.requirements.interview,
                    status: "blocked",
                    lastAnalysisStatus: "ANALYSIS_FAILED",
                    blockedReason: message,
                  },
                },
              })),
            }));
          }
          throw error;
        }
      },

      acceptSuggestedDraft: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            const suggested = task.requirements.interview.suggestedDraft;
            const requirements = invalidateDraft({
              ...task.requirements,
              objective:
                suggested.objective?.trim() || task.requirements.objective,
              scope: mergeUnique(task.requirements.scope, suggested.scope),
              outOfScope: mergeUnique(
                task.requirements.outOfScope,
                suggested.outOfScope,
              ),
              acceptanceCriteria: mergeUnique(
                task.requirements.acceptanceCriteria,
                suggested.acceptanceCriteria,
              ),
              risks: mergeUnique(
                task.requirements.risks,
                suggested.constraints,
              ),
            });
            return {
              ...task,
              requirements: {
                ...requirements,
                interview: {
                  ...requirements.interview,
                  status: "ai_suggested_ready",
                  suggestedDraft: {
                    scope: [],
                    outOfScope: [],
                    acceptanceCriteria: [],
                    constraints: [],
                  },
                },
              },
            };
          }),
        })),

      requestForceProceed: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            if (unresolvedQuestions(task.requirements).length === 0) {
              throw new Error("当前没有未解决问题，无需强制推进");
            }
            return {
              ...task,
              requirements: {
                ...task.requirements,
                interview: {
                  ...task.requirements.interview,
                  status: "force_proceed_confirmation",
                  forceProceed: {
                    requestedAt: now(),
                    decisions: [],
                  },
                },
              },
            };
          }),
        })),

      cancelForceProceed: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            return {
              ...task,
              requirements: {
                ...task.requirements,
                interview: {
                  ...task.requirements.interview,
                  status: "waiting_user_answer",
                  forceProceed: undefined,
                },
              },
            };
          }),
        })),

      confirmForceProceed: (taskID, decisions) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            const unresolved = unresolvedQuestions(task.requirements);
            const decisionByQuestion = new Map(
              decisions.map((decision) => [decision.questionId, decision]),
            );
            if (
              unresolved.some(
                (question) => !decisionByQuestion.has(question.id),
              )
            ) {
              throw new Error("请为每个未解决问题选择处理策略");
            }
            const questions = task.requirements.questions.map((question) => {
              const decision = decisionByQuestion.get(question.id);
              if (!decision) return question;
              const detail = decision.detail?.trim() ?? "";
              if (
                (decision.strategy === "KEEP_EXISTING" ||
                  decision.strategy === "TEMPORARY_DECISION") &&
                !detail
              ) {
                throw new Error(
                  decision.strategy === "KEEP_EXISTING"
                    ? "沿用现有行为时必须填写当前行为快照"
                    : "临时决策不能为空",
                );
              }
              if (decision.strategy === "KEEP_EXISTING") {
                return {
                  ...question,
                  status: "ANSWERED" as const,
                  answer: detail,
                  answerSource: "project_snapshot" as const,
                  resolvedAt: now(),
                  forceDecision: decision.strategy,
                  forceDecisionNote: detail,
                };
              }
              if (decision.strategy === "TEMPORARY_DECISION") {
                return {
                  ...question,
                  status: "ANSWERED" as const,
                  answer: detail,
                  answerSource: "force_proceed" as const,
                  resolvedAt: now(),
                  forceDecision: decision.strategy,
                  forceDecisionNote: detail,
                };
              }
              if (decision.strategy === "EXCLUDE_SCOPE") {
                return {
                  ...question,
                  status: "OUT_OF_SCOPE" as const,
                  answer: "依赖该问题的内容排除在本次开发范围外",
                  answerSource: "force_proceed" as const,
                  resolvedAt: now(),
                  forceDecision: decision.strategy,
                  forceDecisionNote: detail,
                };
              }
              return {
                ...question,
                status: "OPEN" as const,
                answer: "",
                resolvedAt: undefined,
                forceDecision: decision.strategy,
                forceDecisionNote: detail,
              };
            });
            const confirmedAt = now();
            const forceProceed = {
              requestedAt:
                task.requirements.interview.forceProceed?.requestedAt ??
                confirmedAt,
              confirmedAt,
              decisions,
            };
            const workingTask: Task = {
              ...task,
              requirements: {
                ...task.requirements,
                questions,
                interview: {
                  ...task.requirements.interview,
                  status: "draft_ready",
                  forceProceed,
                },
              },
            };
            const requirements = buildRequirementDraft(workingTask);
            return {
              ...workingTask,
              requirements: {
                ...requirements,
                interview: {
                  ...requirements.interview,
                  status: "draft_ready",
                  forceProceed,
                },
              },
            };
          }),
        })),

      confirmRequirements: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            const requirements = task.requirements;
            if (
              !task.evidence.some(
                (source) => source.selectedForAnalysis !== false,
              )
            ) {
              throw new Error("至少需要一条真实需求来源");
            }
            if (!task.projectName.trim()) {
              throw new Error("请先确认对应项目或代码仓库");
            }
            if (!requirements.objective.trim()) {
              throw new Error("需求目标尚未明确");
            }
            if (requirements.acceptanceCriteria.length === 0) {
              throw new Error("至少需要一条可验证的验收标准");
            }
            const forceDecisions =
              requirements.interview.forceProceed?.confirmedAt
                ? requirements.interview.forceProceed.decisions
                : [];
            if (
              requirements.questions.some((question) =>
                isApprovalBlockingQuestion(question, forceDecisions),
              )
            ) {
              throw new Error("仍有未处理的阻塞问题，不能确认需求");
            }
            if (
              !requirements.generatedAt ||
              !requirements.document ||
              !requirements.executionPrompt
            ) {
              throw new Error("内容有变化，请重新生成结构化草稿");
            }
            const confirmedAt = now();
            const confirmedRevision =
              (requirements.approvedRevisions.at(-1)?.version ?? 0) + 1;
            return {
              ...task,
              requirements: {
                ...requirements,
                confirmedAt,
                confirmedRevision,
                approvedRevisions: [
                  ...requirements.approvedRevisions,
                  {
                    version: confirmedRevision,
                    document: requirements.document,
                    executionPrompt: requirements.executionPrompt,
                    confirmedAt,
                  },
                ],
                interview: {
                  ...requirements.interview,
                  status: "approved",
                },
              },
            };
          }),
        })),

      revokeRequirements: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.status !== "requirements") {
              throw new Error("请先退回需求整理阶段再撤销确认");
            }
            return {
              ...task,
              requirements: {
                ...task.requirements,
                confirmedAt: undefined,
                confirmedRevision: undefined,
                interview: {
                  ...task.requirements.interview,
                  status: "draft_ready",
                },
              },
            };
          }),
        })),

      transitionTask: async (taskID, to) => {
        const task = get().tasks.find((candidate) => candidate.id === taskID);
        if (!task) throw new Error("没有找到这个任务");
        if (
          task.status === "requirements" &&
          (task.requirements.interview.status === "analyzing" ||
            task.requirements.interview.status === "reanalyzing")
        ) {
          throw new Error("需求分析正在进行，不能切换任务阶段");
        }
        await validateTransition(task.status, to, taskGates(task));
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (current) => {
            if (current.status !== task.status) {
              throw new Error("任务状态已变化，请重试");
            }
            return { ...current, status: to };
          }),
        }));
      },

      setDevelopmentEngine: (taskID, engine) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.status !== "development") {
              throw new Error("只有开发阶段可以选择执行引擎");
            }
            if (task.development.state !== "idle") {
              throw new Error("已经记录委托，不能切换执行引擎");
            }
            return {
              ...task,
              development: { ...task.development, engine },
            };
          }),
        })),

      markDelegated: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.status !== "development") {
              throw new Error("任务尚未进入开发阶段");
            }
            if (!task.requirements.confirmedAt) {
              throw new Error("需求确认已失效，不能委托开发");
            }
            return {
              ...task,
              development: {
                ...task.development,
                state: "delegated",
                delegatedAt: now(),
              },
            };
          }),
        })),

      setDevelopmentResult: (taskID, resultNote) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (
              task.status !== "development" ||
              task.development.state === "idle"
            ) {
              throw new Error("请先进入开发阶段并记录委托");
            }
            if (task.development.state === "completed") {
              throw new Error("开发结果已经锁定");
            }
            return {
              ...task,
              development: { ...task.development, resultNote },
            };
          }),
        })),

      markDevelopmentCompleted: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.development.state !== "delegated") {
              throw new Error("请先记录任务已委托");
            }
            if (!task.development.resultNote.trim()) {
              throw new Error("请记录开发结果、提交或验证信息");
            }
            return {
              ...task,
              development: {
                ...task.development,
                state: "completed",
                completedAt: now(),
              },
            };
          }),
        })),

      createReviewChecklist: (taskID, mode) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.status !== "review") {
              throw new Error("任务尚未进入审核阶段");
            }
            const labels = [
              "改动范围与人工确认的需求一致",
              "每条验收标准都有实际验证证据",
              "定向测试和静态检查结果已记录",
              "没有未经确认的功能或架构扩张",
              "已确认遗留风险和后续事项",
            ];
            return {
              ...task,
              review: {
                ...task.review,
                mode,
                checklist: labels.map((label) => ({
                  id: createID("review"),
                  label,
                  checked: false,
                })),
                approvedAt: undefined,
              },
            };
          }),
        })),

      toggleReviewItem: (taskID, itemID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.review.approvedAt) {
              throw new Error("审核已经通过，不能再修改检查项");
            }
            return {
              ...task,
              review: {
                ...task.review,
                checklist: task.review.checklist.map((item) =>
                  item.id === itemID
                    ? { ...item, checked: !item.checked }
                    : item,
                ),
              },
            };
          }),
        })),

      setReviewNote: (taskID, note) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.status !== "review" || task.review.approvedAt) {
              throw new Error("当前不能修改审核记录");
            }
            return {
              ...task,
              review: { ...task.review, note },
            };
          }),
        })),

      approveReview: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            if (task.status !== "review") {
              throw new Error("任务尚未进入审核阶段");
            }
            if (task.review.checklist.length === 0) {
              throw new Error("请先生成并执行审核清单");
            }
            if (task.review.checklist.some((item) => !item.checked)) {
              throw new Error("仍有未通过的审核项");
            }
            if (!task.review.note.trim()) {
              throw new Error("请记录审核结论或验证证据");
            }
            return {
              ...task,
              review: { ...task.review, approvedAt: now() },
            };
          }),
        })),
    }),
    {
      name: "btaskassistant-workspace",
      storage: createJSONStorage(() => workspaceStorage),
      version: 13,
      migrate: (persistedState) => {
        const state = persistedState as Partial<WorkspaceState> & {
          dailyReportDraft?: DailyReportDraft;
        };
        return {
          ...state,
          tasks: (state.tasks ?? []).map((task) => ({
            ...task,
            projectPath: task.projectPath ?? "",
            evidence: (task.evidence ?? []).map((source) => ({
              ...source,
              selectedForAnalysis: source.selectedForAnalysis !== false,
            })),
            requirements: normalizeRequirements(task.requirements),
          })),
          trashedTasks: (state.trashedTasks ?? []).map((task) => ({
            ...task,
            projectPath: task.projectPath ?? "",
            evidence: (task.evidence ?? []).map((source) => ({
              ...source,
              selectedForAnalysis: source.selectedForAnalysis !== false,
            })),
            requirements: normalizeRequirements(task.requirements),
          })),
          collectionCandidates: (state.collectionCandidates ?? []).map(
            (candidate) => ({
              ...candidate,
              assigneeDetails: candidate.assigneeDetails ?? [],
              comments: candidate.comments ?? [],
              detailsLoaded: candidate.detailsLoaded ?? true,
              draftEdited: migrateCandidateDraftEdited(candidate),
              collectionRevision:
                candidate.collectionRevision ?? createID("plane-collection"),
            }),
          ),
          planeSettings: migratePlaneSettings(state.planeSettings),
          planeCandidateFilters: normalizePlaneCandidateFilters(
            state.planeCandidateFilters,
          ),
          piSettings: normalizePISettings(state.piSettings),
          dailyReportSettings: normalizeDailyReportSettings(
            state.dailyReportSettings,
          ),
          dailyReportAISettings: normalizeDailyReportAISettings(
            state.dailyReportAISettings,
          ),
          dailyReportProjectHistory: normalizeDailyReportProjectHistory(
            state.dailyReportProjectHistory,
          ),
          dailyReportDate: localDateString(),
          dailyReportDrafts: normalizeDailyReportDrafts(
            state.dailyReportDrafts,
            state.dailyReportDraft,
          ),
        } as WorkspaceState;
      },
      partialize: (state) => ({
        tasks: state.tasks,
        trashedTasks: state.trashedTasks,
        collectionCandidates: state.collectionCandidates,
        planeSettings: state.planeSettings,
        planeCandidateFilters: state.planeCandidateFilters,
        piSettings: state.piSettings,
        dailyReportSettings: state.dailyReportSettings,
        dailyReportAISettings: state.dailyReportAISettings,
        dailyReportProjectHistory: state.dailyReportProjectHistory,
        dailyReportDrafts: state.dailyReportDrafts,
        selectedTaskId: state.selectedTaskId,
        statusFilter: state.statusFilter,
      }),
      onRehydrateStorage: () => (state) => {
        if (!state) return;
        state.selectDailyReportDate(localDateString());
        state.setHydrated(true);
      },
    },
  ),
);
