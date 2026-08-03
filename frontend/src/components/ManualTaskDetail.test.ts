import { act, createElement, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createEmptyDevelopment,
  createEmptyRequirements,
  createEmptyReview,
  type Task,
} from "../domain/task";
import { useWorkspaceStore } from "../store/workspace";
import { ManualTaskDetail } from "./ManualTaskDetail";

const reasonixLifecycle = vi.hoisted(() => ({ mounts: 0, unmounts: 0 }));

vi.mock("./ReasonixPage", () => ({
  ReasonixPage: ({ taskId }: { taskId: string }) => {
    useEffect(() => {
      reasonixLifecycle.mounts += 1;
      return () => {
        reasonixLifecycle.unmounts += 1;
      };
    }, []);
    return createElement("div", { "data-testid": "reasonix-page" }, taskId);
  },
}));

vi.mock("./TaskAgentWorkbench", () => ({
  TaskAgentWorkbench: ({ taskId }: { taskId: string }) =>
    createElement("div", { "data-testid": "pi-page" }, taskId),
}));

vi.mock("./LazyRichMarkdownEditor", () => ({
  LazyRichMarkdownEditor: () => createElement("div"),
}));

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function makeTask(id: string): Task {
  return {
    id,
    title: `任务 ${id}`,
    summary: "任务说明",
    projectName: "",
    projectPath: "",
    priority: "medium",
    status: "inbox",
    evidence: [],
    requirements: createEmptyRequirements(),
    development: createEmptyDevelopment(),
    review: createEmptyReview(),
    revision: 1,
    createdAt: "2026-08-02T00:00:00Z",
    updatedAt: "2026-08-02T00:00:00Z",
  };
}

describe("ManualTaskDetail Reasonix singleton", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    reasonixLifecycle.mounts = 0;
    reasonixLifecycle.unmounts = 0;
    window.localStorage.clear();
    useWorkspaceStore.setState({ hydrated: true });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  const renderTask = async (task: Task) => {
    await act(async () => {
      root.render(
        createElement(ManualTaskDetail, {
          task,
          onEdit: vi.fn(),
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
    });
  };

  it("应用进入任务详情时即在隐藏状态启动 RX", async () => {
    await renderTask(makeTask("task_1"));

    expect(reasonixLifecycle.mounts).toBe(1);
    expect(reasonixLifecycle.unmounts).toBe(0);
    expect(container.querySelector(".manual-rx-pane")?.hasAttribute("hidden"))
      .toBe(true);
    expect(container.querySelector("[data-testid='reasonix-page']")?.textContent)
      .toBe("task_1");
  });

  it("切换任务和打开 RX 只更新会话，不重挂载 Reasonix", async () => {
    await renderTask(makeTask("task_1"));
    const firstEmbed = container.querySelector("[data-testid='reasonix-page']");

    await renderTask(makeTask("task_2"));

    expect(reasonixLifecycle.mounts).toBe(1);
    expect(reasonixLifecycle.unmounts).toBe(0);
    expect(container.querySelector("[data-testid='reasonix-page']")).toBe(firstEmbed);
    expect(firstEmbed?.textContent).toBe("task_2");

    const rxButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "RX",
    );
    await act(async () => rxButton?.click());

    expect(container.querySelector(".manual-rx-pane")?.hasAttribute("hidden"))
      .toBe(false);
    expect(reasonixLifecycle.mounts).toBe(1);
    expect(reasonixLifecycle.unmounts).toBe(0);
  });
});
