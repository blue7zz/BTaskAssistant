import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PlaneCandidatePayload } from "../domain/collection";
import { DEFAULT_PI_SETTINGS } from "../domain/engine";
import {
  DEFAULT_DAILY_REPORT_AI_SETTINGS,
  DEFAULT_DAILY_REPORT_SETTINGS,
  createEmptyDailyReportDraft,
  localDateString,
} from "../domain/report";
import {
  DEFAULT_PLANE_CANDIDATE_FILTERS,
  DEFAULT_PLANE_SETTINGS,
  useWorkspaceHydrationStore,
  useWorkspaceStore,
} from "./workspace";

const planePayload: PlaneCandidatePayload = {
  externalId: "plane-item-1",
  externalKey: "BT-18",
  title: "整理 Plane 收集流程",
  descriptionMarkdown: "只把候选交给用户确认。",
  sourceMarkdown:
    "# 整理 Plane 收集流程\n\n- Plane ID: `plane-item-1`\n\n只把候选交给用户确认。\n\n## Plane 评论\n\n评论也必须作为原始来源保存。",
  priority: "high",
  stateName: "待办",
  stateGroup: "backlog",
  labels: ["workflow"],
  assignees: ["Alice"],
  assigneeDetails: [{ id: "user-alice", name: "Alice" }],
  comments: [
    {
      id: "comment-1",
      bodyMarkdown: "评论也必须作为原始来源保存。",
      actor: { id: "user-reviewer", name: "Reviewer" },
      createdAt: "2026-07-24T09:30:00Z",
    },
  ],
  detailsLoaded: true,
  updatedAt: "2026-07-24T10:00:00Z",
};

describe("workspace store", () => {
  beforeEach(() => {
    window.go = undefined;
    window.localStorage.clear();
    useWorkspaceHydrationStore.setState({
      hydrated: true,
      error: undefined,
    });
    const dailyReportDraft = createEmptyDailyReportDraft("2026-07-30");
    useWorkspaceStore.setState({
      tasks: [],
      trashedTasks: [],
      collectionCandidates: [],
      planeSettings: {
        baseUrl: "",
        workspaceSlug: "",
        projectId: "",
        projectName: "",
        showInTaskSources: true,
      },
      planeCandidateFilters: { ...DEFAULT_PLANE_CANDIDATE_FILTERS },
      piSettings: { ...DEFAULT_PI_SETTINGS },
      dailyReportSettings: { ...DEFAULT_DAILY_REPORT_SETTINGS },
      dailyReportAISettings: { ...DEFAULT_DAILY_REPORT_AI_SETTINGS },
      dailyReportProjectHistory: [],
      dailyReportDate: dailyReportDraft.date,
      dailyReportDrafts: { [dailyReportDraft.date]: dailyReportDraft },
      selectedTaskId: undefined,
      statusFilter: "all",
      hydrated: true,
    });
  });

  it("leaves the loading screen with a retryable error when migration persistence fails", async () => {
    const loadState = vi.fn().mockResolvedValue(
      JSON.stringify({
        version: 13,
        state: {
          tasks: [],
          trashedTasks: [],
          collectionCandidates: [],
          statusFilter: "all",
        },
      }),
    );
    const saveState = vi
      .fn()
      .mockRejectedValue(new Error("旧任务目录缺少所有权标记"));
    useWorkspaceStore.setState({
      hydrated: false,
    });
    useWorkspaceHydrationStore.getState().start();
    window.go = {
      main: {
        App: {
          LoadState: loadState,
          SaveState: saveState,
        } as never,
      },
    };

    await useWorkspaceStore.persist.rehydrate();

    expect(loadState).toHaveBeenCalledOnce();
    expect(saveState).toHaveBeenCalled();
    expect(useWorkspaceHydrationStore.getState().hydrated).toBe(true);
    expect(useWorkspaceHydrationStore.getState().error).toContain(
      "旧任务目录缺少所有权标记",
    );
  });

  it("defaults legacy Plane settings to a visible task source", async () => {
    expect(DEFAULT_PLANE_SETTINGS.showInTaskSources).toBe(true);
    window.localStorage.setItem(
      "btaskassistant-workspace",
      JSON.stringify({
        version: 7,
        state: {
          tasks: [],
          trashedTasks: [],
          collectionCandidates: [],
          planeSettings: {
            baseUrl: "https://plane.example.com",
            workspaceSlug: "team",
            projectId: "project-1",
            projectName: "Team Plane",
          },
          piSettings: DEFAULT_PI_SETTINGS,
          statusFilter: "all",
        },
      }),
    );

    await useWorkspaceStore.persist.rehydrate();

    expect(useWorkspaceStore.getState().planeSettings).toMatchObject({
      baseUrl: "https://plane.example.com",
      workspaceSlug: "team",
      projectId: "project-1",
      projectName: "Team Plane",
      showInTaskSources: true,
    });
    expect(useWorkspaceStore.getState().planeCandidateFilters).toEqual(
      DEFAULT_PLANE_CANDIDATE_FILTERS,
    );

    useWorkspaceStore.getState().updatePlaneSettings({
      showInTaskSources: false,
    });
    expect(
      useWorkspaceStore.getState().planeSettings.showInTaskSources,
    ).toBe(false);
  });

  it("maps legacy OMP engine strings to native PI during hydration", async () => {
    const task = {
      id: "task_legacy_engine",
      title: "旧引擎任务",
      summary: "",
      projectName: "",
      projectPath: "",
      priority: "medium",
      status: "requirements",
      evidence: [],
      requirements: {
        interview: {
          status: "preparing",
          analyst: "oh-my-pi",
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
        },
      },
      development: {
        engine: "omp",
        state: "idle",
        resultNote: "",
      },
      review: { mode: "manual", checklist: [], note: "" },
      revision: 1,
      createdAt: "2026-08-01T08:00:00Z",
      updatedAt: "2026-08-01T08:00:00Z",
    };
    window.localStorage.setItem(
      "btaskassistant-workspace",
      JSON.stringify({
        version: 13,
        state: {
          tasks: [task],
          trashedTasks: [
            {
              ...task,
              id: "task_legacy_trash",
              development: {
                ...task.development,
                engine: "PI / oh-my-pi",
              },
              trashedAt: "2026-08-01T09:00:00Z",
            },
          ],
          collectionCandidates: [],
          planeSettings: DEFAULT_PLANE_SETTINGS,
          planeCandidateFilters: DEFAULT_PLANE_CANDIDATE_FILTERS,
          piSettings: DEFAULT_PI_SETTINGS,
          dailyReportSettings: DEFAULT_DAILY_REPORT_SETTINGS,
          dailyReportAISettings: {
            ...DEFAULT_DAILY_REPORT_AI_SETTINGS,
            engine: "omp",
          },
          statusFilter: "all",
        },
      }),
    );

    await useWorkspaceStore.persist.rehydrate();

    const state = useWorkspaceStore.getState();
    expect(state.tasks[0].development.engine).toBe("pi");
    expect(state.tasks[0].requirements.interview.analyst).toBe("pi");
    expect(state.trashedTasks[0].development.engine).toBe("pi");
    expect(state.dailyReportAISettings.engine).toBe("pi");
  });

  it("persists Plane candidate filters and restores them after refresh", async () => {
    window.localStorage.setItem(
      "btaskassistant-workspace",
      JSON.stringify({
        version: 10,
        state: {
          tasks: [],
          trashedTasks: [],
          collectionCandidates: [],
          planeSettings: DEFAULT_PLANE_SETTINGS,
          planeCandidateFilters: {
            state: "state:开发中",
            assignees: ["id:user-alice"],
          },
          piSettings: DEFAULT_PI_SETTINGS,
          statusFilter: "all",
        },
      }),
    );

    await useWorkspaceStore.persist.rehydrate();

    expect(useWorkspaceStore.getState().planeCandidateFilters).toEqual({
      state: "state:开发中",
      assignees: ["id:user-alice"],
    });

    useWorkspaceStore.getState().updatePlaneCandidateFilters({
      state: "state:测试中",
      assignees: ["id:user-bob", "id:user-bob", ""],
    });

    await vi.waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem("btaskassistant-workspace") ?? "{}",
      );
      expect(stored.state.planeCandidateFilters).toEqual({
        state: "state:测试中",
        assignees: ["id:user-bob"],
      });
    });
  });

  it("migrates and persists date-scoped daily report drafts without a token", async () => {
    const legacyDraft = createEmptyDailyReportDraft("2026-07-29");
    legacyDraft.results[0] = {
      ...legacyDraft.results[0],
      projectNo: "BT-17",
      projectName: "旧日报",
      description: "迁移前正文",
    };
    window.localStorage.setItem(
      "btaskassistant-workspace",
      JSON.stringify({
        version: 11,
        state: {
          tasks: [],
          trashedTasks: [],
          collectionCandidates: [],
          planeSettings: DEFAULT_PLANE_SETTINGS,
          planeCandidateFilters: DEFAULT_PLANE_CANDIDATE_FILTERS,
          piSettings: DEFAULT_PI_SETTINGS,
          dailyReportSettings: DEFAULT_DAILY_REPORT_SETTINGS,
          dailyReportDraft: legacyDraft,
          statusFilter: "all",
        },
      }),
    );

    await useWorkspaceStore.persist.rehydrate();

    expect(useWorkspaceStore.getState().dailyReportSettings).toEqual(
      DEFAULT_DAILY_REPORT_SETTINGS,
    );
    expect(useWorkspaceStore.getState().dailyReportAISettings).toEqual(
      DEFAULT_DAILY_REPORT_AI_SETTINGS,
    );
    expect(useWorkspaceStore.getState().dailyReportProjectHistory).toEqual([]);
    expect(
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-29"].results[0]
        .projectNo,
    ).toBe("BT-17");
    expect(useWorkspaceStore.getState().dailyReportDate).toBe(
      localDateString(),
    );

    useWorkspaceStore.getState().updateDailyReportSettings({
      organization: "技术中心",
      submitter: "张三",
      employeeId: "DN1111",
      apiUrl: "https://report.example.com/submit",
    });
    const today = useWorkspaceStore.getState().dailyReportDate;
    const currentDraft = useWorkspaceStore.getState().dailyReportDrafts[today];
    useWorkspaceStore.getState().updateDailyReportDraft({
      results: [
        {
          ...currentDraft.results[0],
          projectNo: "BT-18",
          projectName: "日报",
          description: "已完成",
        },
      ],
    });
    useWorkspaceStore.getState().selectDailyReportDate("2026-07-28");
    const previousDraft =
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-28"];
    useWorkspaceStore.getState().updateDailyReportDraft({
      results: [
        {
          ...previousDraft.results[0],
          projectNo: "BT-16",
          projectName: "补交日报",
          description: "按日期独立保存",
        },
      ],
    });
    useWorkspaceStore.getState().selectDailyReportDate(today);
    expect(
      useWorkspaceStore.getState().dailyReportDrafts[today].results[0]
        .projectNo,
    ).toBe("BT-18");
    useWorkspaceStore.getState().selectDailyReportDate("2026-07-28");
    expect(
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-28"].results[0]
        .projectNo,
    ).toBe("BT-16");

    await vi.waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem("btaskassistant-workspace") ?? "{}",
      );
      expect(stored.version).toBe(14);
      expect(stored.state.dailyReportSettings.employeeId).toBe("DN1111");
      expect(stored.state.dailyReportDrafts[today].results[0].projectNo).toBe(
        "BT-18",
      );
      expect(
        stored.state.dailyReportDrafts["2026-07-28"].results[0].projectNo,
      ).toBe("BT-16");
      expect(stored.state.dailyReportDate).toBeUndefined();
      expect(JSON.stringify(stored)).not.toContain("apiToken");
    });

    useWorkspaceStore.getState().resetDailyReportDraft();
    expect(useWorkspaceStore.getState().dailyReportDate).toBe("2026-07-28");
    expect(
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-28"].results[0],
    ).toMatchObject({
        projectNo: "",
        projectName: "",
        description: "",
    });
    expect(useWorkspaceStore.getState().dailyReportSettings.organization).toBe(
      "技术中心",
    );
  });

  it("persists normalized AI settings and a bounded project history", async () => {
    useWorkspaceStore.getState().updateDailyReportAISettings({
      engine: "codex",
      customInstructions: "仅使用真实 Git 证据",
      gitAuthor: " blue@example.com ",
      includeUncommitted: false,
    });
    useWorkspaceStore.getState().rememberDailyReportProjects([
      {
        projectNo: "Y15",
        projectName: "Y15 App",
        path: " /workspace/y15/ ",
      },
      {
        projectNo: "duplicate",
        projectName: "重复路径",
        path: "/workspace/y15",
      },
    ]);

    const first = useWorkspaceStore.getState().dailyReportProjectHistory[0];
    expect(first).toMatchObject({
      projectNo: "Y15",
      projectName: "Y15 App",
      path: "/workspace/y15",
    });
    expect(first.id).toMatch(/^report-project-history_/);

    useWorkspaceStore.getState().rememberDailyReportProjects([
      {
        projectNo: "Y15-NEW",
        projectName: "Y15 App 新名称",
        path: "/workspace/y15///",
      },
      ...Array.from({ length: 21 }, (_, index) => ({
        projectNo: `P${index}`,
        projectName: `Project ${index}`,
        path: `/workspace/project-${index}/`,
      })),
    ]);

    const history = useWorkspaceStore.getState().dailyReportProjectHistory;
    expect(history).toHaveLength(20);
    expect(history[0]).toMatchObject({
      id: first.id,
      projectNo: "Y15-NEW",
      path: "/workspace/y15",
    });

    useWorkspaceStore.getState().removeDailyReportProject(first.id);
    expect(
      useWorkspaceStore
        .getState()
        .dailyReportProjectHistory.some((project) => project.id === first.id),
    ).toBe(false);

    await vi.waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem("btaskassistant-workspace") ?? "{}",
      );
      expect(stored.version).toBe(14);
      expect(stored.state.dailyReportAISettings).toEqual({
        engine: "codex",
        customInstructions: "仅使用真实 Git 证据",
        gitAuthor: " blue@example.com ",
        includeUncommitted: false,
      });
      expect(stored.state.dailyReportProjectHistory).toHaveLength(19);
      expect(stored.state.dailyReportProjectHistory[0].path).not.toMatch(/\/$/);
    });
  });

  it("updates a remembered project without changing its history identity", async () => {
    useWorkspaceStore.setState({
      dailyReportProjectHistory: [
        {
          id: "history-y16",
          projectNo: "Y16",
          projectName: "Y16 App",
          path: "/workspace/y16",
          lastUsedAt: "2026-07-30T10:00:00Z",
        },
      ],
    });

    useWorkspaceStore.getState().updateDailyReportProject("history-y16", {
      projectNo: " Y16-NEW ",
      projectName: " Y16 App New ",
      path: " C:\\workspace\\y16\\ ",
    });

    expect(useWorkspaceStore.getState().dailyReportProjectHistory).toEqual([
      {
        id: "history-y16",
        projectNo: "Y16-NEW",
        projectName: "Y16 App New",
        path: "C:\\workspace\\y16",
        lastUsedAt: "2026-07-30T10:00:00Z",
      },
    ]);

    await vi.waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem("btaskassistant-workspace") ?? "{}",
      );
      expect(stored.state.dailyReportProjectHistory).toEqual([
        {
          id: "history-y16",
          projectNo: "Y16-NEW",
          projectName: "Y16 App New",
          path: "C:\\workspace\\y16",
          lastUsedAt: "2026-07-30T10:00:00Z",
        },
      ]);
    });

    useWorkspaceStore.getState().updateDailyReportProject("history-y16", {
      projectNo: "ROOT",
      projectName: "Windows Root",
      path: " C:\\ ",
    });
    expect(
      useWorkspaceStore.getState().dailyReportProjectHistory[0].path,
    ).toBe("C:\\");

    useWorkspaceStore.getState().updateDailyReportProject("history-y16", {
      projectNo: "ROOT",
      projectName: "POSIX Root",
      path: " / ",
    });
    expect(
      useWorkspaceStore.getState().dailyReportProjectHistory[0].path,
    ).toBe("/");
  });

  it("updates manual record fields and status without workflow gates", () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "手工记录",
      summary: "初始文本",
      projectName: "未分类",
    });

    useWorkspaceStore.getState().updateTaskRecord(taskID, {
      summary: "人工补充后的文本",
      projectName: "BTaskAssistant",
      priority: "high",
      status: "development",
    });

    const task = useWorkspaceStore.getState().tasks[0];
    expect(task.summary).toBe("人工补充后的文本");
    expect(task.projectName).toBe("BTaskAssistant");
    expect(task.priority).toBe("high");
    expect(task.status).toBe("development");
  });

  it("moves tasks to the trash, restores them, and permanently deletes them", () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "可恢复任务",
      summary: "正文和来源在回收站中都必须保留。",
      projectName: "BTaskAssistant",
      initialEvidence: {
        type: "manual",
        title: "原始记录",
        content: "不要在软删除时丢失。",
      },
    });
    const originalTask = useWorkspaceStore.getState().tasks[0];

    useWorkspaceStore.getState().moveTaskToTrash(taskID);

    let state = useWorkspaceStore.getState();
    expect(state.tasks).toHaveLength(0);
    expect(state.selectedTaskId).toBeUndefined();
    expect(state.trashedTasks).toHaveLength(1);
    expect(state.trashedTasks[0].id).toBe(taskID);
    expect(state.trashedTasks[0].summary).toBe(originalTask.summary);
    expect(state.trashedTasks[0].evidence).toEqual(originalTask.evidence);
    expect(state.trashedTasks[0].trashedAt).toBeTruthy();

    useWorkspaceStore.getState().restoreTask(taskID);

    state = useWorkspaceStore.getState();
    expect(state.trashedTasks).toHaveLength(0);
    expect(state.tasks).toHaveLength(1);
    expect(state.tasks[0].id).toBe(taskID);
    expect(state.tasks[0].summary).toBe(originalTask.summary);
    expect(state.selectedTaskId).toBe(taskID);
    expect(state.statusFilter).toBe("all");

    useWorkspaceStore.getState().moveTaskToTrash(taskID);
    useWorkspaceStore.getState().deleteTaskPermanently(taskID);

    state = useWorkspaceStore.getState();
    expect(state.tasks).toHaveLength(0);
    expect(state.trashedTasks).toHaveLength(0);
  });

  it("walks the complete workflow only through explicit human gates", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "实现任务工作流",
      summary: "实现任务录入、需求确认、开发和审核主链路。",
      projectName: "blue7zz/BTaskAssistant",
      initialEvidence: {
        type: "manual",
        title: "用户明确需求",
        content: "状态必须手动推进，需求必须由人工确认。",
      },
    });

    await useWorkspaceStore.getState().transitionTask(taskID, "requirements");
    useWorkspaceStore.getState().patchRequirements(taskID, {
      objective: "实现可以运行的工作流第一版",
      scope: ["任务录入", "需求确认", "开发记录", "审核完成"],
      outOfScope: ["自动调用未配置的 AI CLI"],
      acceptanceCriteria: ["完整状态流可以通过人工操作走通"],
      risks: ["严禁自动越过人工确认"],
    });
    useWorkspaceStore.getState().generateDraft(taskID);
    useWorkspaceStore.getState().confirmRequirements(taskID);

    await useWorkspaceStore.getState().transitionTask(taskID, "approved");
    await useWorkspaceStore.getState().transitionTask(taskID, "development");
    useWorkspaceStore.getState().markDelegated(taskID);
    useWorkspaceStore
      .getState()
      .setDevelopmentResult(taskID, "commit abc123；pnpm test 通过。");
    useWorkspaceStore.getState().markDevelopmentCompleted(taskID);

    await useWorkspaceStore.getState().transitionTask(taskID, "review");
    useWorkspaceStore.getState().createReviewChecklist(taskID, "template");
    const reviewItems = useWorkspaceStore
      .getState()
      .tasks.find((task) => task.id === taskID)!.review.checklist;
    for (const item of reviewItems) {
      useWorkspaceStore.getState().toggleReviewItem(taskID, item.id);
    }
    useWorkspaceStore
      .getState()
      .setReviewNote(taskID, "已核对范围并执行全部测试。");
    useWorkspaceStore.getState().approveReview(taskID);

    await useWorkspaceStore.getState().transitionTask(taskID, "done");
    const finishedTask = useWorkspaceStore
      .getState()
      .tasks.find((task) => task.id === taskID);

    expect(finishedTask?.status).toBe("done");
    expect(finishedTask?.requirements.confirmedAt).toBeTruthy();
    expect(finishedTask?.development.state).toBe("completed");
    expect(finishedTask?.review.approvedAt).toBeTruthy();
  });

  it("refuses to confirm requirements without a project", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "缺少项目",
      summary: "这个任务还没有指定项目。",
      initialEvidence: {
        type: "manual",
        title: "原始说明",
        content: "需要先确认项目。",
      },
    });

    await useWorkspaceStore.getState().transitionTask(taskID, "requirements");
    useWorkspaceStore.getState().patchRequirements(taskID, {
      objective: "确认项目",
      acceptanceCriteria: ["项目名称已经记录"],
    });
    useWorkspaceStore.getState().generateDraft(taskID);

    expect(() =>
      useWorkspaceStore.getState().confirmRequirements(taskID),
    ).toThrow("项目");
  });

  it("deduplicates Plane items and creates a task only after human acceptance", () => {
    useWorkspaceStore.getState().updatePlaneSettings({
      baseUrl: "https://plane.example.com",
      workspaceSlug: "team",
      projectId: "project-1",
      projectName: "BTaskAssistant",
    });

    expect(
      useWorkspaceStore.getState().ingestPlaneCandidates([planePayload]),
    ).toBe(1);
    useWorkspaceStore.getState().ingestPlaneCandidates([planePayload]);

    let state = useWorkspaceStore.getState();
    expect(state.collectionCandidates).toHaveLength(1);
    expect(state.tasks).toHaveLength(0);
    const candidate = state.collectionCandidates[0];

    state.applyCandidateAnalysis(candidate.id, {
      mode: "pi",
      title: "确认 Plane 候选任务",
      summaryMarkdown: "保留来源，等待人工确认。",
      keyPoints: ["不得自动创建正式任务"],
      openQuestions: [],
      analyzedAt: "2026-07-24T10:01:00Z",
    });
    const taskID = useWorkspaceStore
      .getState()
      .acceptCandidate(candidate.id);

    state = useWorkspaceStore.getState();
    expect(state.collectionCandidates[0].decision).toBe("accepted");
    expect(state.collectionCandidates[0].acceptedTaskId).toBe(taskID);
    expect(state.tasks).toHaveLength(1);
    expect(state.tasks[0].title).toBe(planePayload.title);
    expect(state.tasks[0].summary).toBe(planePayload.descriptionMarkdown);
    expect(state.tasks[0].evidence[0].type).toBe("plane");
    expect(state.tasks[0].evidence[0].content).toContain("Plane ID");
    expect(state.tasks[0].evidence[0].content).toContain("评论");
    expect(() => state.acceptCandidate(candidate.id)).toThrow("已经处理");

    state.moveTaskToTrash(taskID);
    useWorkspaceStore.getState().deleteTaskPermanently(taskID);
    state = useWorkspaceStore.getState();
    expect(state.collectionCandidates[0].decision).toBe("pending");
    expect(state.collectionCandidates[0].acceptedTaskId).toBeUndefined();
  });

  it("hydrates only the selected Plane candidate before acceptance", () => {
    useWorkspaceStore.getState().updatePlaneSettings({
      projectId: "project-1",
      projectName: "BTaskAssistant",
    });
    const summary: PlaneCandidatePayload = {
      ...planePayload,
      descriptionMarkdown: "",
      sourceMarkdown: "# 整理 Plane 收集流程\n\n- Plane ID: `plane-item-1`",
      comments: [],
      detailsLoaded: false,
    };
    const secondSummary: PlaneCandidatePayload = {
      ...summary,
      externalId: "plane-item-2",
      externalKey: "BT-19",
      title: "另一条摘要",
      sourceMarkdown: "# 另一条摘要\n\n- Plane ID: `plane-item-2`",
      updatedAt: "2026-07-24T10:02:00Z",
    };

    useWorkspaceStore
      .getState()
      .ingestPlaneCandidates([summary, secondSummary]);
    let candidate = useWorkspaceStore.getState().collectionCandidates[0];
    expect(() =>
      useWorkspaceStore.getState().acceptCandidate(candidate.id),
    ).toThrow("先加载 Plane 详情和评论");

    expect(
      useWorkspaceStore.getState().hydratePlaneCandidate(planePayload, {
        candidateID: candidate.id,
        collectionRevision: candidate.collectionRevision,
        projectID: "project-1",
      }),
    ).toBe(true);
    let state = useWorkspaceStore.getState();
    expect(state.collectionCandidates).toHaveLength(2);
    expect(state.collectionCandidates[0].detailsLoaded).toBe(true);
    expect(state.collectionCandidates[0].comments).toHaveLength(1);
    expect(state.collectionCandidates[1].detailsLoaded).toBe(false);

    candidate = state.collectionCandidates[0];
    expect(
      useWorkspaceStore.getState().hydratePlaneCandidate(
        {
          ...planePayload,
          descriptionMarkdown: "Plane 中更新后的正文",
          sourceMarkdown: "# 整理 Plane 收集流程\n\nPlane 中更新后的正文",
        },
        {
          candidateID: candidate.id,
          collectionRevision: candidate.collectionRevision,
          projectID: "project-1",
        },
      ),
    ).toBe(true);
    expect(
      useWorkspaceStore.getState().collectionCandidates[0].analysis
        .summaryMarkdown,
    ).toBe("Plane 中更新后的正文");

    state = useWorkspaceStore.getState();
    state.updateCandidateDraft(candidate.id, {
      title: "人工整理后的标题",
      summaryMarkdown: "人工整理后的正文",
    });
    candidate = useWorkspaceStore.getState().collectionCandidates[0];
    expect(
      useWorkspaceStore.getState().hydratePlaneCandidate(planePayload, {
        candidateID: candidate.id,
        collectionRevision: candidate.collectionRevision,
        projectID: "project-1",
      }),
    ).toBe(true);
    expect(
      useWorkspaceStore.getState().collectionCandidates[0].analysis.title,
    ).toBe("人工整理后的标题");
    useWorkspaceStore
      .getState()
      .ingestPlaneCandidates([summary, secondSummary]);
    state = useWorkspaceStore.getState();
    expect(state.collectionCandidates[0].detailsLoaded).toBe(true);
    expect(state.collectionCandidates[0].comments).toHaveLength(1);
    expect(state.collectionCandidates[0].analysis.title).toBe(
      "人工整理后的标题",
    );

    const staleRequest = {
      candidateID: state.collectionCandidates[0].id,
      collectionRevision: state.collectionCandidates[0].collectionRevision,
      projectID: "project-1",
    };
    const changedSummary: PlaneCandidatePayload = {
      ...summary,
      stateName: "开发中",
      updatedAt: "2026-07-24T10:03:00Z",
    };
    useWorkspaceStore
      .getState()
      .ingestPlaneCandidates([changedSummary, secondSummary]);
    expect(
      useWorkspaceStore
        .getState()
        .hydratePlaneCandidate(planePayload, staleRequest),
    ).toBe(false);
    state = useWorkspaceStore.getState();
    expect(state.collectionCandidates[0].stateName).toBe("开发中");
    expect(state.collectionCandidates[0].detailsLoaded).toBe(false);
    expect(state.collectionCandidates[0].comments).toHaveLength(0);
    expect(state.collectionCandidates[0].analysis.title).toBe(
      "人工整理后的标题",
    );
    expect(state.collectionCandidates[0].analysis.summaryMarkdown).toBe(
      "人工整理后的正文",
    );

    candidate = state.collectionCandidates[0];
    expect(
      useWorkspaceStore.getState().hydratePlaneCandidate(
        {
          ...planePayload,
          stateName: "开发中",
          updatedAt: changedSummary.updatedAt,
        },
        {
          candidateID: candidate.id,
          collectionRevision: candidate.collectionRevision,
          projectID: "project-1",
        },
      ),
    ).toBe(true);
    state = useWorkspaceStore.getState();
    expect(state.collectionCandidates[0].analysis.title).toBe(
      "人工整理后的标题",
    );

    const taskID = state.acceptCandidate(candidate.id);
    const task = useWorkspaceStore
      .getState()
      .tasks.find((item) => item.id === taskID)!;
    expect(task.title).toBe(planePayload.title);
    expect(task.summary).toBe(planePayload.descriptionMarkdown);
    expect(task.evidence[0].content).toContain("评论也必须作为原始来源保存");
  });

  it("blocks acceptance until Plane comments finish syncing", () => {
    useWorkspaceStore.getState().updatePlaneSettings({
      projectId: "project-1",
      projectName: "BTaskAssistant",
    });
    useWorkspaceStore.getState().ingestPlaneCandidates([
      {
        ...planePayload,
        comments: [],
        commentsSyncError: "评论同步失败，可稍后重试",
      },
    ]);

    const candidate = useWorkspaceStore.getState().collectionCandidates[0];
    expect(() =>
      useWorkspaceStore.getState().acceptCandidate(candidate.id),
    ).toThrow("评论尚未完整同步");
    expect(useWorkspaceStore.getState().tasks).toHaveLength(0);
  });

  it("migrates legacy manual Plane drafts without losing them on recollection", async () => {
    useWorkspaceStore.getState().updatePlaneSettings({
      projectId: "project-1",
      projectName: "BTaskAssistant",
    });
    useWorkspaceStore.getState().ingestPlaneCandidates([planePayload]);
    const candidate = useWorkspaceStore.getState().collectionCandidates[0];
    useWorkspaceStore.getState().updateCandidateDraft(candidate.id, {
      title: "旧版本人工标题",
      summaryMarkdown: "旧版本人工正文",
    });
    const legacyCandidate = {
      ...useWorkspaceStore.getState().collectionCandidates[0],
    } as unknown as Record<string, unknown>;
    delete legacyCandidate.draftEdited;
    delete legacyCandidate.collectionRevision;
    window.localStorage.setItem(
      "btaskassistant-workspace",
      JSON.stringify({
        version: 8,
        state: {
          tasks: [],
          trashedTasks: [],
          collectionCandidates: [legacyCandidate],
          planeSettings: useWorkspaceStore.getState().planeSettings,
          piSettings: DEFAULT_PI_SETTINGS,
          statusFilter: "all",
        },
      }),
    );

    await useWorkspaceStore.persist.rehydrate();
    expect(
      useWorkspaceStore.getState().collectionCandidates[0].draftEdited,
    ).toBe(true);

    useWorkspaceStore.getState().ingestPlaneCandidates([
      {
        ...planePayload,
        descriptionMarkdown: "",
        sourceMarkdown: "# 更新后的摘要",
        comments: [],
        detailsLoaded: false,
        updatedAt: "2026-07-24T11:00:00Z",
      },
    ]);
    const migrated = useWorkspaceStore.getState().collectionCandidates[0];
    expect(migrated.detailsLoaded).toBe(false);
    expect(migrated.analysis.title).toBe("旧版本人工标题");
    expect(migrated.analysis.summaryMarkdown).toBe("旧版本人工正文");
  });

  it("stores AI output as interview candidates without auto-approving draft updates", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "补齐需求访谈",
      summary: "AI 应发现信息缺口，但不能代替用户作决定。",
      projectName: "BTaskAssistant",
      projectPath: "/workspace/BTaskAssistant",
      initialEvidence: {
        type: "manual",
        title: "用户说明",
        content: "需求必须经过人工批准。",
      },
    });
    await useWorkspaceStore.getState().transitionTask(taskID, "requirements");

    const analyze = vi.fn().mockResolvedValue({
      analysisId: "ANALYSIS-1",
      round: 1,
      confirmedFacts: [
        {
          id: "FACT-1",
          content: "需求必须经过人工批准。",
          sourceFragmentIds: [
            useWorkspaceStore.getState().tasks[0].evidence[0].id,
          ],
        },
      ],
      projectObservations: [
        {
          id: "OBS-1",
          content: "现有状态机只允许逐级推进。",
          filePath: "internal/workflow/machine.go",
          lineRange: "20-40",
        },
      ],
      questions: [
        {
          id: "QUESTION-1",
          category: "SCOPE",
          severity: "BLOCKING",
          question: "是否包含开发阶段？",
          reason: "不回答会影响实现范围。",
          sourceFragmentIds: [],
          projectEvidence: [],
          answerType: "SINGLE_SELECT",
          options: ["只做需求整理", "包含开发阶段"],
          allowCustomAnswer: true,
        },
      ],
      conflicts: [],
      draftUpdates: {
        objective: "建立多轮需求访谈",
        scope: ["需求整理阶段"],
        outOfScope: [],
        acceptanceCriteria: ["AI 不会自动批准需求"],
        constraints: ["项目只读"],
      },
      analysisStatus: "NEEDS_USER_INPUT",
      recommendedAction: "ASK_QUESTIONS",
      reason: "仍有范围问题需要用户回答。",
      analyzedAt: "2026-07-28T12:00:00Z",
    });
    window.go = {
      main: {
        App: {
          AnalyzeRequirements: analyze,
          SaveState: vi.fn().mockResolvedValue(undefined),
        } as never,
      },
    };

    await useWorkspaceStore.getState().runRequirementAnalysis(taskID);

    expect(analyze).toHaveBeenCalledWith(
      expect.any(Object),
      DEFAULT_PI_SETTINGS,
    );

    const task = useWorkspaceStore
      .getState()
      .tasks.find((candidate) => candidate.id === taskID)!;
    expect(task.requirements.interview.status).toBe("waiting_user_answer");
    expect(task.requirements.questions).toHaveLength(1);
    expect(task.requirements.questions[0].severity).toBe("BLOCKING");
    expect(task.requirements.facts[0].sourceId).toBe(task.evidence[0].id);
    expect(task.requirements.scope).toEqual([]);
    expect(task.requirements.interview.suggestedDraft.scope).toEqual([
      "需求整理阶段",
    ]);
    useWorkspaceStore.getState().answerQuestions(taskID, [
      {
        questionId: task.requirements.questions[0].id,
        answer: "只做需求整理",
      },
    ]);
    expect(
      useWorkspaceStore.getState().tasks[0].requirements.questions[0].status,
    ).toBe("ANSWERED");
  });

  it("freezes unresolved questions and risks when the user force-proceeds", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "强制推进需求",
      summary: "允许用户带着显式风险生成草稿。",
      projectName: "BTaskAssistant",
      initialEvidence: {
        type: "manual",
        title: "用户说明",
        content: "未确认事项不能被隐藏。",
      },
    });
    await useWorkspaceStore.getState().transitionTask(taskID, "requirements");
    useWorkspaceStore.getState().patchRequirements(taskID, {
      objective: "保留强制推进风险",
      scope: ["需求整理阶段"],
      acceptanceCriteria: ["正式文档包含未确认事项"],
    });
    const sourceID = useWorkspaceStore.getState().tasks[0].evidence[0].id;
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          AnalyzeRequirements: vi.fn().mockResolvedValue({
            analysisId: "ANALYSIS-2",
            round: 1,
            confirmedFacts: [
              {
                id: "FACT-2",
                content: "未确认事项不能被隐藏。",
                sourceFragmentIds: [sourceID],
              },
            ],
            projectObservations: [],
            questions: [
              {
                id: "QUESTION-2",
                category: "BEHAVIOR",
                severity: "BLOCKING",
                question: "失败时是否允许重试？",
                reason: "不回答会影响失败交互。",
                sourceFragmentIds: [sourceID],
                projectEvidence: [],
                answerType: "BOOLEAN",
                options: ["允许", "不允许"],
                allowCustomAnswer: true,
              },
            ],
            conflicts: [],
            draftUpdates: {
              scope: [],
              outOfScope: [],
              acceptanceCriteria: [],
              constraints: [],
            },
            analysisStatus: "NEEDS_USER_INPUT",
            recommendedAction: "ASK_QUESTIONS",
            reason: "仍有阻塞问题。",
            analyzedAt: "2026-07-28T12:05:00Z",
          }),
        } as never,
      },
    };
    await useWorkspaceStore.getState().runRequirementAnalysis(taskID);
    let task = useWorkspaceStore.getState().tasks[0];
    const questionID = task.requirements.questions[0].id;

    expect(() => useWorkspaceStore.getState().confirmRequirements(taskID)).toThrow(
      "阻塞问题",
    );
    useWorkspaceStore.getState().requestForceProceed(taskID);
    useWorkspaceStore.getState().confirmForceProceed(taskID, [
      { questionId: questionID, strategy: "KEEP_UNCONFIRMED" },
    ]);
    useWorkspaceStore.getState().confirmRequirements(taskID);

    task = useWorkspaceStore.getState().tasks[0];
    expect(task.requirements.questions[0].status).toBe("OPEN");
    expect(task.requirements.questions[0].forceDecision).toBe(
      "KEEP_UNCONFIRMED",
    );
    expect(task.requirements.document).toContain("## 11. 未确认事项");
    expect(task.requirements.document).toContain("失败时是否允许重试");
    expect(task.requirements.confirmedAt).toBeTruthy();
  });

  it("preserves an approved snapshot before a new requirement revision", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "需求版本",
      summary: "批准版本不能被后续修改静默覆盖。",
      projectName: "BTaskAssistant",
      initialEvidence: {
        type: "manual",
        title: "版本规则",
        content: "每次批准都保留不可变快照。",
      },
    });
    await useWorkspaceStore.getState().transitionTask(taskID, "requirements");
    useWorkspaceStore.getState().patchRequirements(taskID, {
      objective: "保留第一版",
      acceptanceCriteria: ["历史快照可追溯"],
    });
    useWorkspaceStore.getState().generateDraft(taskID);
    useWorkspaceStore.getState().confirmRequirements(taskID);
    const firstDocument = useWorkspaceStore.getState().tasks[0].requirements.document;

    useWorkspaceStore.getState().revokeRequirements(taskID);
    useWorkspaceStore.getState().patchRequirements(taskID, {
      objective: "保留第二版",
    });
    useWorkspaceStore.getState().generateDraft(taskID);
    useWorkspaceStore.getState().confirmRequirements(taskID);

    const requirements = useWorkspaceStore.getState().tasks[0].requirements;
    expect(requirements.confirmedRevision).toBe(2);
    expect(requirements.approvedRevisions).toHaveLength(2);
    expect(requirements.approvedRevisions[0].document).toBe(firstDocument);
    expect(requirements.approvedRevisions[1].document).toContain("保留第二版");
  });
});
