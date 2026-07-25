import { beforeEach, describe, expect, it } from "vitest";
import type { PlaneCandidatePayload } from "../domain/collection";
import { useWorkspaceStore } from "./workspace";

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
  updatedAt: "2026-07-24T10:00:00Z",
};

describe("workspace store", () => {
  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      tasks: [],
      collectionCandidates: [],
      planeSettings: {
        baseUrl: "",
        workspaceSlug: "",
        projectId: "",
        projectName: "",
      },
      selectedTaskId: undefined,
      statusFilter: "all",
      hydrated: true,
    });
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
    expect(state.tasks[0].title).toBe("确认 Plane 候选任务");
    expect(state.tasks[0].evidence[0].type).toBe("plane");
    expect(state.tasks[0].evidence[0].content).toContain("Plane ID");
    expect(state.tasks[0].evidence[0].content).toContain("评论");
    expect(() => state.acceptCandidate(candidate.id)).toThrow("已经处理");
  });
});
