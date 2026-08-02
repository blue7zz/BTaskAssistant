import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PlaneCandidatePayload } from "../domain/collection";
import { useWorkspaceStore } from "../store/workspace";
import { PlaneCollector } from "./PlaneCollector";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("PlaneCollector", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      tasks: [],
      trashedTasks: [],
      collectionCandidates: [],
      planeSettings: {
        baseUrl: "https://plane.fymyriad.com",
        workspaceSlug: "myriad",
        projectId: "",
        projectName: "",
        projectIdentifier: "",
        showInTaskSources: true,
      },
      planeCandidateFilters: { state: "all", assignees: [] },
      selectedTaskId: undefined,
      statusFilter: "all",
      hydrated: true,
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    delete window.go;
    delete window.runtime;
    vi.restoreAllMocks();
  });

  it("combines status and multi-assignee filters before lazily loading details", async () => {
    useWorkspaceStore.setState({
      planeSettings: {
        baseUrl: "https://plane.example.com",
        workspaceSlug: "team",
        projectId: "project-1",
        projectName: "myriad",
        projectIdentifier: "MYRIA",
        showInTaskSources: true,
      },
    });
    useWorkspaceStore.getState().ingestPlaneCandidates([
      {
        externalId: "item-alice",
        externalKey: "MYRIA-1",
        title: "Alice 的任务",
        descriptionMarkdown: "需要 Alice 确认。",
        sourceMarkdown: "# Alice 的任务",
        priority: "medium",
        stateName: "开发中",
        stateGroup: "started",
        labels: [],
        assignees: ["Alice"],
        assigneeDetails: [{ id: "user-alice", name: "Alice" }],
        comments: [],
        detailsLoaded: false,
        updatedAt: "2026-07-24T10:00:00Z",
      },
      {
        externalId: "item-bob",
        externalKey: "MYRIA-2",
        title: "Bob 的任务",
        descriptionMarkdown: "需要 Bob 确认。",
        sourceMarkdown: "# Bob 的任务",
        priority: "low",
        stateName: "测试中",
        stateGroup: "started",
        labels: [],
        assignees: ["Bob"],
        assigneeDetails: [{ id: "user-bob", name: "Bob" }],
        comments: [],
        detailsLoaded: false,
        updatedAt: "2026-07-24T10:01:00Z",
      },
      {
        externalId: "item-carol",
        externalKey: "MYRIA-3",
        title: "Carol 的任务",
        descriptionMarkdown: "",
        sourceMarkdown: "# Carol 的任务",
        priority: "low",
        stateName: "待办",
        stateGroup: "backlog",
        labels: [],
        assignees: ["Carol"],
        assigneeDetails: [{ id: "user-carol", name: "Carol" }],
        comments: [],
        detailsLoaded: false,
        updatedAt: "2026-07-24T10:02:00Z",
      },
    ]);
    const loadDetails = vi.fn().mockResolvedValue({
      externalId: "item-alice",
      externalKey: "MYRIA-1",
      title: "Alice 的任务",
      descriptionMarkdown:
        "需要 Alice 确认。\n\n![需求截图](https://plane.example.com/assets/spec.png)\n\n| 字段 | 必填 |\n| --- | --- |\n| keyword | 是 |",
      sourceMarkdown:
        "# Alice 的任务\n\n## Plane 原始描述\n\n需要 Alice 确认。\n\n## Plane 评论\n\n请把这个边界条件补充到需求里。",
      priority: "medium",
      stateName: "开发中",
      stateGroup: "started",
      labels: [],
      assignees: ["Alice"],
      assigneeDetails: [{ id: "user-alice", name: "Alice" }],
      comments: [
        {
          id: "comment-1",
          bodyMarkdown: "请把这个边界条件补充到需求里。",
          actor: { id: "reviewer", name: "Reviewer" },
          createdAt: "2026-07-24T10:00:00Z",
        },
      ],
      parent: {
        externalId: "item-parent",
        externalKey: "MYRIA-359",
        title: "Y15web｜子任务｜埋点更新与对齐",
      },
      detailsLoaded: true,
      updatedAt: "2026-07-24T10:00:00Z",
    });
    const browserOpenURL = vi.fn();
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          LoadPlaneWorkItemDetails: loadDetails,
        },
      },
    } as unknown as typeof window.go;
    window.runtime = { BrowserOpenURL: browserOpenURL } as unknown as Window["runtime"];

    await act(async () => {
      root.render(
        createElement(PlaneCollector, {
          connected: true,
          onSuccess: vi.fn(),
          onError: vi.fn(),
          onOpenTask: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

    expect(container.querySelectorAll(".candidate-card")).toHaveLength(3);
    expect(loadDetails).not.toHaveBeenCalled();
    expect(container.textContent).toContain("选择候选任务后再加载详情");
    expect(container.textContent).not.toContain("用 PI 提炼关键信息");
    expect(container.textContent).not.toContain("PI 候选");
    expect(container.textContent).not.toContain("Plane 连接");
    expect(container.querySelector('input[type="password"]')).toBeNull();
    expect(container.textContent).toContain("从 Plane 收集");

    const syncBar = container.querySelector(".collector-sync-bar")!;
    const candidateFilters = syncBar.querySelector(".candidate-filters")!;
    expect(syncBar.firstElementChild).toBe(candidateFilters);
    expect(
      container.querySelector(".candidate-toolbar .candidate-filters"),
    ).toBeNull();

    const statusSelect = container.querySelector(
      'select[aria-label="按任务状态筛选"]',
    ) as HTMLSelectElement;
    await act(async () => {
      statusSelect.value = "state:测试中";
      statusSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(container.querySelectorAll(".candidate-card")).toHaveLength(1);
    expect(container.querySelector(".candidate-card")?.textContent).toContain(
      "Bob 的任务",
    );

    await act(async () => {
      statusSelect.value = "all";
      statusSelect.dispatchEvent(new Event("change", { bubbles: true }));
      (
        container.querySelector(
          'input[aria-label="筛选负责人 Alice"]',
        ) as HTMLInputElement
      ).click();
    });
    await act(async () => {
      (
        container.querySelector(
          'input[aria-label="筛选负责人 Bob"]',
        ) as HTMLInputElement
      ).click();
    });

    expect(container.querySelectorAll(".candidate-card")).toHaveLength(2);
    expect(container.textContent).toContain("已选 2 项");

    await act(async () => {
      statusSelect.value = "state:开发中";
      statusSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(container.querySelectorAll(".candidate-card")).toHaveLength(1);
    expect(container.querySelector(".candidate-card")?.textContent).toContain(
      "Alice 的任务",
    );
    expect(loadDetails).not.toHaveBeenCalled();

    const aliceCard = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".candidate-card"),
    ).find((button) => button.textContent?.includes("Alice 的任务"))!;
    await act(async () => {
      aliceCard.click();
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 0));
    });

    expect(loadDetails).toHaveBeenCalledTimes(1);
    expect(loadDetails).toHaveBeenCalledWith(
      "https://plane.example.com",
      "team",
      "project-1",
      "MYRIA",
      "item-alice",
    );
    expect(container.querySelector(".plane-comment")?.textContent).toContain(
      "请把这个边界条件补充到需求里。",
    );
    expect(container.querySelector(".plane-comment")?.textContent).toContain(
      "Reviewer",
    );
    const detail = container.querySelector(".candidate-detail")!;
    expect(detail.querySelector("input, textarea")).toBeNull();
    expect(detail.querySelector('[contenteditable="true"]')).toBeNull();
    expect(detail.querySelector(".editor-mode-tabs")).toBeNull();
    expect(detail.querySelector(".plane-description-preview")).not.toBeNull();
    expect(detail.textContent).not.toContain("候选任务确认");
    expect(detail.textContent).not.toContain("正式任务标题");
    expect(detail.textContent).not.toContain("正式任务正文");

    const titleLink = detail.querySelector<HTMLAnchorElement>(
      ".plane-preview-header > a",
    )!;
    expect(titleLink.href).toBe(
      "https://plane.example.com/team/browse/MYRIA-1",
    );
    const parentLink = detail.querySelector<HTMLAnchorElement>(
      ".plane-parent-link",
    )!;
    expect(parentLink.href).toBe(
      "https://plane.example.com/team/browse/MYRIA-359",
    );
    expect(parentLink.textContent).toContain("Y15web｜子任务｜埋点更新与对齐");

    await act(async () => titleLink.click());
    expect(browserOpenURL).toHaveBeenCalledWith(
      "https://plane.example.com/team/browse/MYRIA-1",
    );

    const taskBody = detail.querySelector(".plane-description-preview")!;
    const comments = detail.querySelector(".plane-context-panel")!;
    expect(taskBody.compareDocumentPosition(comments)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(
      useWorkspaceStore.getState().collectionCandidates[0]
        .descriptionMarkdown,
    ).toContain("![需求截图]");
  });

  it("restores filters and counts every decision from the filtered result", async () => {
    useWorkspaceStore.setState({
      planeSettings: {
        baseUrl: "https://plane.example.com",
        workspaceSlug: "team",
        projectId: "project-1",
        projectName: "myriad",
        projectIdentifier: "MYRIA",
        showInTaskSources: true,
      },
    });
    const payload = (
      externalId: string,
      title: string,
      stateName: string,
      assigneeID: string,
      assigneeName: string,
    ): PlaneCandidatePayload => ({
      externalId,
      externalKey: externalId.toUpperCase(),
      title,
      descriptionMarkdown: title,
      sourceMarkdown: `# ${title}`,
      priority: "medium",
      stateName,
      stateGroup: "started",
      labels: [],
      assignees: [assigneeName],
      assigneeDetails: [{ id: assigneeID, name: assigneeName }],
      comments: [],
      detailsLoaded: false,
    });
    useWorkspaceStore.getState().ingestPlaneCandidates([
      payload("pending-alice", "Alice 待确认", "开发中", "user-alice", "Alice"),
      payload("pending-bob", "Bob 待确认", "开发中", "user-bob", "Bob"),
      payload("accepted-alice", "Alice 已转换", "开发中", "user-alice", "Alice"),
      payload("ignored-alice", "Alice 已忽略", "测试中", "user-alice", "Alice"),
      payload("ignored-bob", "Bob 已忽略", "开发中", "user-bob", "Bob"),
    ]);
    useWorkspaceStore.setState((state) => ({
      collectionCandidates: state.collectionCandidates.map((candidate) => ({
        ...candidate,
        decision: candidate.externalId.startsWith("accepted")
          ? "accepted"
          : candidate.externalId.startsWith("ignored")
            ? "ignored"
            : "pending",
      })),
      planeCandidateFilters: {
        state: "state:开发中",
        assignees: ["id:user-alice"],
      },
    }));

    await act(async () => {
      root.render(
        createElement(PlaneCollector, {
          connected: true,
          onSuccess: vi.fn(),
          onError: vi.fn(),
          onOpenTask: vi.fn(),
        }),
      );
    });

    const tab = (label: string) =>
      Array.from(
        container.querySelectorAll<HTMLButtonElement>(".candidate-tabs button"),
      ).find((button) => button.textContent?.includes(label))!;
    const statusSelect = container.querySelector(
      'select[aria-label="按任务状态筛选"]',
    ) as HTMLSelectElement;
    const aliceFilter = container.querySelector(
      'input[aria-label="筛选负责人 Alice"]',
    ) as HTMLInputElement;

    expect(statusSelect.value).toBe("state:开发中");
    expect(aliceFilter.checked).toBe(true);
    expect(tab("待确认").querySelector("span")?.textContent).toBe("1");
    expect(tab("已转换").querySelector("span")?.textContent).toBe("1");
    expect(tab("已忽略").querySelector("span")?.textContent).toBe("0");
    expect(container.querySelectorAll(".candidate-card")).toHaveLength(1);
    expect(container.querySelector(".candidate-card")?.textContent).toContain(
      "Alice 待确认",
    );

    await act(async () => tab("已转换").click());

    expect(statusSelect.value).toBe("state:开发中");
    expect(aliceFilter.checked).toBe(true);
    expect(container.querySelectorAll(".candidate-card")).toHaveLength(1);
    expect(container.querySelector(".candidate-card")?.textContent).toContain(
      "Alice 已转换",
    );
    expect(tab("待确认").querySelector("span")?.textContent).toBe("1");
    expect(tab("已转换").querySelector("span")?.textContent).toBe("1");
    expect(tab("已忽略").querySelector("span")?.textContent).toBe("0");

    await act(async () => tab("已忽略").click());

    expect(container.querySelectorAll(".candidate-card")).toHaveLength(0);
    expect(container.textContent).toContain("当前筛选条件下没有候选");
    expect(statusSelect.value).toBe("state:开发中");
    expect(aliceFilter.checked).toBe(true);
  });

  it("collects candidates from the compact collection action", async () => {
    useWorkspaceStore.setState({
      planeSettings: {
        baseUrl: "https://plane.example.com",
        workspaceSlug: "team",
        projectId: "project-1",
        projectName: "Project One",
        projectIdentifier: "ONE",
        showInTaskSources: true,
      },
    });
    const collectPlaneWorkItems = vi.fn().mockResolvedValue([
      {
        externalId: "plane-item-1",
        externalKey: "ONE-1",
        title: "新收集候选",
        descriptionMarkdown: "等待人工确认。",
        sourceMarkdown: "# 新收集候选",
        priority: "medium",
        stateName: "待办",
        stateGroup: "backlog",
        labels: [],
        assignees: [],
        comments: [],
        detailsLoaded: false,
      },
    ]);
    const loadDetails = vi.fn();
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          CollectPlaneWorkItems: collectPlaneWorkItems,
          LoadPlaneWorkItemDetails: loadDetails,
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(PlaneCollector, {
          connected: true,
          onSuccess: vi.fn(),
          onError: vi.fn(),
          onOpenTask: vi.fn(),
        }),
      );
    });

    const collectButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("从 Plane 收集"))!;
    await act(async () => {
      collectButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(collectPlaneWorkItems).toHaveBeenCalledWith(
      "https://plane.example.com",
      "team",
      "project-1",
      "ONE",
    );
    expect(container.textContent).toContain("新收集候选");
    expect(container.textContent).toContain(
      "详情和评论将在点击任务后读取",
    );
    expect(loadDetails).not.toHaveBeenCalled();
    expect(useWorkspaceStore.getState().tasks).toHaveLength(0);
  });
});
