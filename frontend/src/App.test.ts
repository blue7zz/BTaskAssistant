import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { DEFAULT_PI_SETTINGS } from "./domain/engine";
import {
  DEFAULT_DAILY_REPORT_SETTINGS,
  createEmptyDailyReportDraft,
} from "./domain/report";
import { useWorkspaceStore } from "./store/workspace";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("App smoke test", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
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
      piSettings: { ...DEFAULT_PI_SETTINGS },
      dailyReportSettings: { ...DEFAULT_DAILY_REPORT_SETTINGS },
      dailyReportDate: dailyReportDraft.date,
      dailyReportDrafts: { [dailyReportDraft.date]: dailyReportDraft },
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
    vi.restoreAllMocks();
    delete window.go;
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
    expect(container.textContent).toContain("纯手工记录");
    expect(container.textContent).toContain("优先级：中");
    expect(container.textContent).toContain("文本记录");
    expect(container.textContent).toContain("测试来源");
    expect(
      container.querySelector('button[aria-label="编辑任务基本信息"]'),
    ).not.toBeNull();
    expect(container.textContent).not.toContain("当前只保存手工记录");
    expect(container.textContent).not.toContain("分类与状态");
    expect(container.querySelector(".topbar")).toBeNull();
    expect(container.textContent).not.toContain("本地任务工作台");
    expect(container.textContent).not.toContain("按项目和状态整理记录");
    expect(container.textContent).not.toContain("导入聊天");
    expect(container.textContent).not.toContain("添加任务");
  });

  it("hides all AI feature entry points", async () => {
    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    expect(container.textContent).not.toContain("PI 详细设置");
    expect(container.textContent).not.toContain("引擎边界");
    expect(container.textContent).not.toContain("开始第一轮分析");
    expect(container.textContent).not.toContain("Codex");
  });

  it("opens settings as the full main workspace and returns to tasks", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "设置返回目标",
      summary: "设置页返回后仍然选中。",
      projectName: "BTaskAssistant",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="打开设置"]',
        ) as HTMLButtonElement
      ).click();
      await Promise.resolve();
    });

    const settingsPage = container.querySelector(
      ".main-shell > .settings-page",
    ) as HTMLElement;
    const backButton = container.querySelector(
      'button[aria-label="返回上一个界面"]',
    ) as HTMLButtonElement;
    expect(settingsPage).not.toBeNull();
    expect(settingsPage.contains(backButton)).toBe(true);
    expect(container.querySelector(".workspace-grid")).toBeNull();
    expect(container.querySelector(".task-list-panel")).toBeNull();
    expect(container.querySelector(".manual-task-detail")).toBeNull();
    expect(
      container.querySelector('button[role="tab"][aria-selected="true"]')
        ?.textContent,
    ).toContain("PI 设置");

    const planeSettingsTab = Array.from(
      container.querySelectorAll<HTMLButtonElement>('button[role="tab"]'),
    ).find((button) => button.textContent?.includes("Plane 连接"))!;
    await act(async () => {
      planeSettingsTab.click();
      await Promise.resolve();
    });
    expect(container.querySelector(".plane-settings-page")).not.toBeNull();
    expect(container.querySelector(".pi-settings-page")).toBeNull();

    await act(async () => backButton.click());

    expect(container.querySelector(".settings-page")).toBeNull();
    expect(container.querySelector(".workspace-grid")).not.toBeNull();
    expect(useWorkspaceStore.getState().selectedTaskId).toBe(taskID);
    expect(container.textContent).toContain("设置返回目标");
  });

  it("opens the daily report below task sources and returns from its settings", async () => {
    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    const dailyReportButton = container.querySelector(
      'button[aria-label="打开日报"]',
    ) as HTMLButtonElement;
    expect(dailyReportButton).not.toBeNull();
    expect(dailyReportButton.textContent).toContain("日报");

    await act(async () => dailyReportButton.click());
    expect(container.querySelector(".daily-report-page")).not.toBeNull();
    expect(container.textContent).toContain("【今日结果】");
    expect(container.textContent).toContain("导出预览（Markdown）");

    const reportSettingsButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("日报设置"))!;
    await act(async () => {
      reportSettingsButton.click();
      await Promise.resolve();
    });

    expect(
      container.querySelector(".daily-report-settings-page"),
    ).not.toBeNull();
    expect(
      container.querySelector('button[role="tab"][aria-selected="true"]')
        ?.textContent,
    ).toContain("日报设置");

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="返回上一个界面"]',
        ) as HTMLButtonElement
      ).click();
    });
    expect(container.querySelector(".settings-page")).toBeNull();
    expect(container.querySelector(".daily-report-page")).not.toBeNull();
  });

  it("returns from settings to the previously open trash view", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "回收站设置返回目标",
      summary: "验证返回之前的界面。",
      projectName: "BTaskAssistant",
    });
    useWorkspaceStore.getState().moveTaskToTrash(taskID);

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="打开回收站"]',
        ) as HTMLButtonElement
      ).click();
    });
    expect(container.querySelector(".trash-view")).not.toBeNull();

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="打开设置"]',
        ) as HTMLButtonElement
      ).click();
    });
    expect(container.querySelector(".settings-page")).not.toBeNull();

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="返回上一个界面"]',
        ) as HTMLButtonElement
      ).click();
    });

    expect(container.querySelector(".settings-page")).toBeNull();
    expect(container.querySelector(".trash-view")).not.toBeNull();
    expect(container.textContent).toContain("回收站设置返回目标");
  });

  it("hides the Plane source when configuration has no stored credential", async () => {
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
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          EngineStatuses: vi.fn().mockResolvedValue([]),
          HasPlaneToken: vi.fn().mockResolvedValue(false),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).not.toContain("任务来源");
    expect(container.textContent).not.toContain("Plane 收集箱");
  });

  it("shows a connected Plane source and returns to tasks after hiding it", async () => {
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
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          EngineStatuses: vi.fn().mockResolvedValue([]),
          HasPlaneToken: vi.fn().mockResolvedValue(true),
          TestPlaneConnection: vi.fn().mockResolvedValue({
            connected: true,
            itemCount: 1,
            message: "Plane 连接正常",
          }),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
      await Promise.resolve();
    });

    const planeSourceButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".nav-item"),
    ).find((button) => button.textContent?.includes("Plane 收集箱"))!;
    expect(planeSourceButton).toBeTruthy();
    await act(async () => planeSourceButton.click());
    expect(container.querySelector(".collector-page")).not.toBeNull();
    expect(container.textContent).not.toContain("先生成候选，再由人决定");

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="打开设置"]',
        ) as HTMLButtonElement
      ).click();
    });
    const planeSettingsTab = Array.from(
      container.querySelectorAll<HTMLButtonElement>('button[role="tab"]'),
    ).find((button) => button.textContent?.includes("Plane 连接"))!;
    await act(async () => {
      planeSettingsTab.click();
      await Promise.resolve();
    });

    const sourceSwitch = container.querySelector(
      'input[aria-label="在任务来源中显示 Plane 收集箱"]',
    ) as HTMLInputElement;
    expect(sourceSwitch.disabled).toBe(false);
    expect(sourceSwitch.checked).toBe(true);
    await act(async () => sourceSwitch.click());
    expect(useWorkspaceStore.getState().planeSettings.showInTaskSources).toBe(
      false,
    );

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="返回上一个界面"]',
        ) as HTMLButtonElement
      ).click();
    });

    expect(container.querySelector(".collector-page")).toBeNull();
    expect(container.textContent).toContain("任务池还是空的");
    expect(container.textContent).not.toContain("任务来源");
    expect(container.textContent).not.toContain("Plane 收集箱");
  });

  it("collapses and expands the task list", async () => {
    useWorkspaceStore.getState().createTask({
      title: "可收起的任务",
      summary: "验证任务列表缩放按钮。",
      projectName: "BTaskAssistant",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    const workspace = container.querySelector(".workspace-grid");
    const collapseButton = container.querySelector(
      'button[aria-label="收起任务列表"]',
    ) as HTMLButtonElement;

    expect(workspace?.classList.contains("task-list-collapsed")).toBe(false);

    await act(async () => collapseButton.click());

    expect(workspace?.classList.contains("task-list-collapsed")).toBe(true);
    expect(
      container.querySelector('.task-list[aria-hidden="true"]'),
    ).not.toBeNull();

    const expandButton = container.querySelector(
      'button[aria-label="展开任务列表"]',
    ) as HTMLButtonElement;
    await act(async () => expandButton.click());

    expect(workspace?.classList.contains("task-list-collapsed")).toBe(false);
    expect(
      container.querySelector('.task-list[aria-hidden="false"]'),
    ).not.toBeNull();
  });

  it("resizes both left panels without squeezing the detail below its limit", async () => {
    useWorkspaceStore.getState().createTask({
      title: "可调整宽度的任务",
      summary: "验证两条竖向分隔线。",
      projectName: "BTaskAssistant",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    const appShell = container.querySelector(".app-shell") as HTMLDivElement;
    const sidebarResizer = container.querySelector(
      '[aria-label="调整导航栏宽度"]',
    ) as HTMLDivElement;
    const initialSidebarWidth = Number.parseInt(
      appShell.style.getPropertyValue("--sidebar-width"),
      10,
    );

    await act(async () => {
      sidebarResizer.dispatchEvent(
        new MouseEvent("pointerdown", {
          bubbles: true,
          button: 0,
          clientX: initialSidebarWidth,
        }),
      );
    });
    await act(async () => {
      window.dispatchEvent(
        new MouseEvent("pointermove", {
          clientX: initialSidebarWidth + 32,
        }),
      );
      window.dispatchEvent(new MouseEvent("pointerup"));
    });

    const resizedSidebarWidth = Number.parseInt(
      appShell.style.getPropertyValue("--sidebar-width"),
      10,
    );
    expect(resizedSidebarWidth).toBe(initialSidebarWidth + 32);

    const taskListResizer = container.querySelector(
      '[aria-label="调整任务列表宽度"]',
    ) as HTMLDivElement;
    const initialTaskListWidth = Number.parseInt(
      appShell.style.getPropertyValue("--task-list-width"),
      10,
    );

    await act(async () => {
      taskListResizer.dispatchEvent(
        new MouseEvent("pointerdown", {
          bubbles: true,
          button: 0,
          clientX: initialTaskListWidth,
        }),
      );
    });
    await act(async () => {
      window.dispatchEvent(
        new MouseEvent("pointermove", {
          clientX: initialTaskListWidth + window.innerWidth,
        }),
      );
      window.dispatchEvent(new MouseEvent("pointerup"));
    });

    const resizedTaskListWidth = Number.parseInt(
      appShell.style.getPropertyValue("--task-list-width"),
      10,
    );
    expect(resizedTaskListWidth).toBe(
      Number(taskListResizer.getAttribute("aria-valuemax")),
    );
    expect(
      window.innerWidth - resizedSidebarWidth - resizedTaskListWidth,
    ).toBeGreaterThanOrEqual(420);
    expect(document.body.classList.contains("panel-resizing")).toBe(false);
  });

  it("keeps task search inside the expanded task list", async () => {
    useWorkspaceStore.getState().createTask({
      title: "列表内搜索目标",
      summary: "应当保留",
      projectName: "BTaskAssistant",
    });
    useWorkspaceStore.getState().createTask({
      title: "另一个任务",
      summary: "应当过滤",
      projectName: "其他项目",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    const searchInput = container.querySelector(
      '.task-list-panel input[placeholder="搜索任务或项目"]',
    ) as HTMLInputElement;
    const inputSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;

    expect(searchInput).not.toBeNull();
    expect(
      container.querySelector('.topbar input[placeholder="搜索任务或项目"]'),
    ).toBeNull();

    await act(async () => {
      inputSetter?.call(searchInput, "列表内搜索目标");
      searchInput.dispatchEvent(new Event("input", { bubbles: true }));
    });

    const taskList = container.querySelector(".task-list");
    expect(taskList?.textContent).toContain("列表内搜索目标");
    expect(taskList?.textContent).not.toContain("另一个任务");
    expect(taskList?.querySelectorAll(".task-card")).toHaveLength(1);

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="收起任务列表"]',
        ) as HTMLButtonElement
      ).click();
    });
    expect(container.querySelector(".task-list-search")).toBeNull();

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="展开任务列表"]',
        ) as HTMLButtonElement
      ).click();
    });
    expect(
      (
        container.querySelector(
          '.task-list-search input',
        ) as HTMLInputElement
    ).value,
    ).toBe("列表内搜索目标");
  });

  it("opens the clicked task menu and edits its basic information", async () => {
    const firstTaskID = useWorkspaceStore.getState().createTask({
      title: "需要右键编辑",
      summary: "只修改基础信息。",
      projectName: "旧项目",
    });
    useWorkspaceStore.getState().createTask({
      title: "保持不变",
      summary: "验证菜单操作目标。",
      projectName: "其他项目",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    const targetCard = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".task-card"),
    ).find((card) => card.textContent?.includes("需要右键编辑"))!;

    await act(async () => {
      targetCard.dispatchEvent(
        new MouseEvent("contextmenu", {
          bubbles: true,
          clientX: 240,
          clientY: 180,
        }),
      );
    });

    expect(useWorkspaceStore.getState().selectedTaskId).toBe(firstTaskID);
    expect(container.querySelector('[role="menu"]')?.textContent).toContain(
      "编辑基本信息",
    );
    expect(container.querySelector('[role="menu"]')?.textContent).toContain(
      "移入回收站",
    );

    const editButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'),
    ).find((button) => button.textContent?.includes("编辑基本信息"))!;
    await act(async () => editButton.click());

    const titleInput = container.querySelector(
      'input[name="title"]',
    ) as HTMLInputElement;
    const projectInput = container.querySelector(
      'input[name="projectName"]',
    ) as HTMLInputElement;
    const statusSelect = container.querySelector(
      'select[name="status"]',
    ) as HTMLSelectElement;
    const prioritySelect = container.querySelector(
      'select[name="priority"]',
    ) as HTMLSelectElement;
    const inputSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;

    await act(async () => {
      inputSetter?.call(titleInput, "已编辑的任务");
      titleInput.dispatchEvent(new Event("input", { bubbles: true }));
      inputSetter?.call(projectInput, "新项目");
      projectInput.dispatchEvent(new Event("input", { bubbles: true }));
      statusSelect.value = "review";
      statusSelect.dispatchEvent(new Event("change", { bubbles: true }));
      prioritySelect.value = "high";
      prioritySelect.dispatchEvent(new Event("change", { bubbles: true }));
    });

    await act(async () => {
      (
        container.querySelector(
          '.task-basic-info-dialog button[type="submit"]',
        ) as HTMLButtonElement
      ).click();
    });

    const editedTask = useWorkspaceStore
      .getState()
      .tasks.find((task) => task.id === firstTaskID);
    expect(editedTask?.title).toBe("已编辑的任务");
    expect(editedTask?.projectName).toBe("新项目");
    expect(editedTask?.status).toBe("review");
    expect(editedTask?.priority).toBe("high");
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it("moves a task to the trash and restores it", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "等待恢复",
      summary: "软删除后仍可找回。",
      projectName: "BTaskAssistant",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    const taskCard = container.querySelector(".task-card") as HTMLButtonElement;
    await act(async () => {
      taskCard.dispatchEvent(
        new MouseEvent("contextmenu", {
          bubbles: true,
          clientX: 220,
          clientY: 160,
        }),
      );
    });
    const trashButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'),
    ).find((button) => button.textContent?.includes("移入回收站"))!;
    await act(async () => trashButton.click());

    let state = useWorkspaceStore.getState();
    expect(state.tasks).toHaveLength(0);
    expect(state.trashedTasks).toHaveLength(1);
    expect(container.textContent).toContain("任务池还是空的");

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="打开回收站"]',
        ) as HTMLButtonElement
      ).click();
    });

    expect(container.textContent).toContain("回收站");
    expect(container.textContent).toContain("等待恢复");
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="恢复任务 等待恢复"]',
        ) as HTMLButtonElement
      ).click();
    });

    state = useWorkspaceStore.getState();
    expect(state.tasks.map((task) => task.id)).toContain(taskID);
    expect(state.trashedTasks).toHaveLength(0);
    expect(container.textContent).toContain("等待恢复");
    expect(container.textContent).toContain("文本记录");
  });

  it("requires confirmation before permanently deleting a trashed task", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "永久删除目标",
      summary: "确认后才真正移除。",
      projectName: "BTaskAssistant",
    });
    useWorkspaceStore.getState().moveTaskToTrash(taskID);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="打开回收站"]',
        ) as HTMLButtonElement
      ).click();
    });

    const deleteButton = container.querySelector(
      'button[aria-label="永久删除任务 永久删除目标"]',
    ) as HTMLButtonElement;
    await act(async () => deleteButton.click());
    expect(useWorkspaceStore.getState().trashedTasks).toHaveLength(1);

    confirm.mockReturnValue(true);
    await act(async () => deleteButton.click());
    expect(useWorkspaceStore.getState().trashedTasks).toHaveLength(0);
  });

  it("opens basic information editing from the detail header", async () => {
    const taskID = useWorkspaceStore.getState().createTask({
      title: "手工状态记录",
      summary: "由用户填写进展。",
      projectName: "未分类",
    });

    await act(async () => {
      root.render(createElement(App));
      await Promise.resolve();
    });

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="编辑任务基本信息"]',
        ) as HTMLButtonElement
      ).click();
    });

    expect(container.querySelector('[role="dialog"]')?.textContent).toContain(
      "编辑基本信息",
    );

    const statusSelect = container.querySelector(
      'select[name="status"]',
    ) as HTMLSelectElement;
    const inputSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;

    await act(async () => {
      statusSelect.value = "development";
      statusSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });

    const projectInput = container.querySelector(
      'input[name="projectName"]',
    ) as HTMLInputElement;
    await act(async () => {
      inputSetter?.call(projectInput, "BTaskAssistant");
      projectInput.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      (
        container.querySelector(
          '.task-basic-info-dialog button[type="submit"]',
        ) as HTMLButtonElement
      ).click();
    });

    const task = useWorkspaceStore
      .getState()
      .tasks.find((candidate) => candidate.id === taskID);
    expect(task?.status).toBe("development");
    expect(task?.projectName).toBe("BTaskAssistant");
  });
});
