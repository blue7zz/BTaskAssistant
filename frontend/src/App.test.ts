import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import App from "./App";
import { useWorkspaceStore } from "./store/workspace";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("App smoke test", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      tasks: [],
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
  });

  it("renders the first-use task entry points", async () => {
    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    expect(container.textContent).toContain("任务池还是空的");
    expect(container.textContent).toContain("添加第一个任务");
    expect(container.textContent).toContain("导入聊天记录");
  });

  it("renders a created task in the inbox workbench", async () => {
    useWorkspaceStore.getState().createTask({
      title: "验证任务工作台",
      summary: "保留真实来源并手动推进。",
      projectName: "blue7zz/BTaskAssistant",
      initialEvidence: {
        type: "manual",
        title: "测试来源",
        content: "这是明确提供的测试资料。",
      },
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    expect(container.textContent).toContain("验证任务工作台");
    expect(container.textContent).toContain("先收集，不分析");
    expect(container.textContent).toContain("测试来源");
  });
});
