import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { buildRequirementDraft } from "../domain/templates";
import {
  createEmptyDevelopment,
  createEmptyRequirements,
  createEmptyReview,
  type CreateTaskInput,
  type DevelopmentEngine,
  type Evidence,
  type EvidenceType,
  type Requirements,
  type ReviewMode,
  type Task,
  type TaskPriority,
  type TaskStatus,
} from "../domain/task";
import { createID } from "../lib/id";
import {
  validateTransition,
  workspaceStorage,
} from "../lib/bridge";

export type StatusFilter = TaskStatus | "all";

type EditableRequirementFields = Pick<
  Requirements,
  "objective" | "scope" | "outOfScope" | "acceptanceCriteria" | "risks"
>;

interface WorkspaceState {
  tasks: Task[];
  selectedTaskId?: string;
  statusFilter: StatusFilter;
  hydrated: boolean;

  setHydrated(hydrated: boolean): void;
  setStatusFilter(status: StatusFilter): void;
  selectTask(taskID?: string): void;
  createTask(input: CreateTaskInput): string;
  importChat(input: {
    title?: string;
    content: string;
    projectName?: string;
  }): string;
  updateTaskDetails(
    taskID: string,
    patch: Partial<
      Pick<Task, "title" | "summary" | "projectName" | "priority">
    >,
  ): void;
  addEvidence(
    taskID: string,
    input: { type: EvidenceType; title: string; content: string },
  ): void;
  removeEvidence(taskID: string, evidenceID: string): void;
  patchRequirements(
    taskID: string,
    patch: Partial<EditableRequirementFields>,
  ): void;
  generateDraft(taskID: string): void;
  answerQuestion(taskID: string, questionID: string, answer: string): void;
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

export const useWorkspaceStore = create<WorkspaceState>()(
  persist(
    (set, get) => ({
      tasks: [],
      selectedTaskId: undefined,
      statusFilter: "all",
      hydrated: false,

      setHydrated: (hydrated) => set({ hydrated }),
      setStatusFilter: (statusFilter) => set({ statusFilter }),
      selectTask: (selectedTaskId) => set({ selectedTaskId }),

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
            return {
              ...task,
              requirements: buildRequirementDraft(task),
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
                        resolvedAt: answer.trim() ? now() : undefined,
                      }
                    : question,
                ),
              }),
            };
          }),
        })),

      confirmRequirements: (taskID) =>
        set((state) => ({
          tasks: updateTask(state.tasks, taskID, (task) => {
            ensureRequirementEditable(task);
            const requirements = task.requirements;
            if (task.evidence.length === 0) {
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
            if (
              requirements.questions.some(
                (question) =>
                  !question.resolvedAt || !question.answer.trim(),
              )
            ) {
              throw new Error("仍有待确认问题，不能确认需求");
            }
            if (
              !requirements.generatedAt ||
              !requirements.document ||
              !requirements.executionPrompt
            ) {
              throw new Error("内容有变化，请重新生成结构化草稿");
            }
            return {
              ...task,
              requirements: {
                ...requirements,
                confirmedAt: now(),
                confirmedRevision: task.revision + 1,
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
              },
            };
          }),
        })),

      transitionTask: async (taskID, to) => {
        const task = get().tasks.find((candidate) => candidate.id === taskID);
        if (!task) throw new Error("没有找到这个任务");
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
      version: 1,
      partialize: (state) => ({
        tasks: state.tasks,
        selectedTaskId: state.selectedTaskId,
        statusFilter: state.statusFilter,
      }),
      onRehydrateStorage: () => (state) => {
        state?.setHydrated(true);
      },
    },
  ),
);
