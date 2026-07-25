import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
      collectionCandidates: [],
      planeSettings: {
        baseUrl: "https://plane.fymyriad.com",
        workspaceSlug: "myriad",
        projectId: "",
        projectName: "",
        projectIdentifier: "",
      },
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
  });

  it("connects with only the workspace URL and PAT, then selects the discovered project", async () => {
    const setupPlaneConnection = vi.fn().mockResolvedValue({
      baseUrl: "https://plane.fymyriad.com",
      workspaceSlug: "myriad",
      projects: [
        {
          id: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
          name: "myriad",
          identifier: "MYRIA",
        },
      ],
    });
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          HasPlaneToken: vi.fn().mockResolvedValue(false),
          SetupPlaneConnection: setupPlaneConnection,
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(PlaneCollector, {
          onSuccess: vi.fn(),
          onError: vi.fn(),
          onOpenTask: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

    expect(container.textContent).not.toContain("Workspace slug");
    expect(container.textContent).not.toContain("Project ID");
    const serviceInput = container.querySelector(
      'input:not([type="password"])',
    ) as HTMLInputElement;
    expect(serviceInput.value).toBe(
      "https://plane.fymyriad.com/myriad/",
    );

    const tokenInput = container.querySelector(
      'input[type="password"]',
    ) as HTMLInputElement;
    const valueSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;
    await act(async () => {
      valueSetter?.call(tokenInput, "plane_api_test");
      tokenInput.dispatchEvent(new Event("input", { bubbles: true }));
    });
    const saveButton = Array.from(
      container.querySelectorAll("button"),
    ).find((button) => button.textContent?.includes("连接并加载选项"));
    expect(saveButton).toBeTruthy();

    await act(async () => {
      saveButton?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(setupPlaneConnection).toHaveBeenCalledWith(
      "https://plane.fymyriad.com/myriad/",
      "plane_api_test",
    );
    expect(useWorkspaceStore.getState().planeSettings).toMatchObject({
      baseUrl: "https://plane.fymyriad.com",
      workspaceSlug: "myriad",
      projectId: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
      projectName: "myriad",
      projectIdentifier: "MYRIA",
    });
    expect(useWorkspaceStore.getState().tasks).toHaveLength(0);
    expect(container.textContent).toContain("MYRIA");
  });

  it("filters candidates by assignee and shows Plane comments", async () => {
    useWorkspaceStore.setState({
      planeSettings: {
        baseUrl: "",
        workspaceSlug: "",
        projectId: "",
        projectName: "myriad",
        projectIdentifier: "MYRIA",
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
        stateName: "待办",
        stateGroup: "backlog",
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
      },
      {
        externalId: "item-bob",
        externalKey: "MYRIA-2",
        title: "Bob 的任务",
        descriptionMarkdown: "需要 Bob 确认。",
        sourceMarkdown: "# Bob 的任务",
        priority: "low",
        stateName: "进行中",
        stateGroup: "started",
        labels: [],
        assignees: ["Bob"],
        assigneeDetails: [{ id: "user-bob", name: "Bob" }],
        comments: [],
      },
    ]);

    await act(async () => {
      root.render(
        createElement(PlaneCollector, {
          onSuccess: vi.fn(),
          onError: vi.fn(),
          onOpenTask: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

    expect(container.querySelectorAll(".candidate-card")).toHaveLength(2);
    expect(container.querySelector(".plane-comment")?.textContent).toContain(
      "请把这个边界条件补充到需求里。",
    );
    expect(container.querySelector(".plane-comment")?.textContent).toContain(
      "Reviewer",
    );

    const assigneeSelect = container.querySelector(
      'select[aria-label="按负责人筛选"]',
    ) as HTMLSelectElement;
    await act(async () => {
      assigneeSelect.value = "id:user-bob";
      assigneeSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });

    const visibleCards = container.querySelectorAll(".candidate-card");
    expect(visibleCards).toHaveLength(1);
    expect(visibleCards[0].textContent).toContain("Bob 的任务");
  });
});
