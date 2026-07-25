import { beforeEach, describe, expect, it } from "vitest";
import { useWorkspaceStore } from "./workspace";

describe("workspace store", () => {
  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      tasks: [],
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
});
