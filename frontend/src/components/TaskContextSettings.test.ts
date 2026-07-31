import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { workspaceStorage } from "../lib/bridge";
import { SettingsPage } from "./SettingsPage";
import { TaskContextSettings } from "./TaskContextSettings";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function findButton(container: HTMLElement, text: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.includes(text),
  )!;
}

describe("Task context settings", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.go = undefined;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    delete window.go;
    vi.restoreAllMocks();
  });

  it("makes the lack of a physical directory explicit in browser preview", async () => {
    await act(async () => {
      root.render(
        createElement(TaskContextSettings, {
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
    });

    expect(container.textContent).toContain("浏览器预览 · 无物理目录");
    expect(container.textContent).toContain(
      "浏览器预览不会模拟或保存本机目录",
    );
    expect(
      (container.querySelector(
        'input[aria-label="任务资料根目录"]',
      ) as HTMLInputElement).value,
    ).toBe("浏览器预览模式不创建物理目录");
    expect(findButton(container, "更改目录").disabled).toBe(true);
    expect(findButton(container, "打开目录").disabled).toBe(true);
  });

  it("shows the effective default directory and opens it", async () => {
    const getRoot = vi.fn().mockResolvedValue({
      path: "/Users/test/Library/Application Support/BTaskAssistant/tasks",
      defaultPath:
        "/Users/test/Library/Application Support/BTaskAssistant/tasks",
      custom: false,
      available: true,
      databaseSchemaVersion: 3,
      taskWorkspaceSchemaVersion: 1,
      workspaceCount: 4,
      workspaceErrorCount: 1,
    });
    const openRoot = vi.fn().mockResolvedValue(undefined);
    window.go = {
      main: {
        App: {
          GetTaskContextRoot: getRoot,
          SelectTaskContextRoot: vi.fn(),
          SetTaskContextRoot: vi.fn(),
          OpenTaskContextRoot: openRoot,
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(TaskContextSettings, {
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(getRoot).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("应用默认目录");
    expect(container.textContent).toContain("目录可用");
    expect(container.textContent).toContain("SQLite v3 · Task Workspace v1");
    expect(container.textContent).toContain("已建立 4 个任务空间");
    expect(container.textContent).toContain("1 个任务空间需要修复");
    expect(
      (container.querySelector(
        'input[aria-label="任务资料根目录"]',
      ) as HTMLInputElement).value,
    ).toBe("/Users/test/Library/Application Support/BTaskAssistant/tasks");

    await act(async () => findButton(container, "打开目录").click());
    expect(openRoot).toHaveBeenCalledTimes(1);
  });

  it("refreshes availability when opening the directory fails", async () => {
    const openError = new Error("目录已移除");
    const onError = vi.fn();
    const getRoot = vi
      .fn()
      .mockResolvedValueOnce({
        path: "/current/tasks",
        defaultPath: "/current/tasks",
        custom: false,
        available: true,
      })
      .mockResolvedValueOnce({
        path: "/current/tasks",
        defaultPath: "/current/tasks",
        custom: false,
        available: false,
      });
    window.go = {
      main: {
        App: {
          GetTaskContextRoot: getRoot,
          SelectTaskContextRoot: vi.fn(),
          SetTaskContextRoot: vi.fn(),
          OpenTaskContextRoot: vi.fn().mockRejectedValue(openError),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(TaskContextSettings, {
          onSuccess: vi.fn(),
          onError,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => findButton(container, "打开目录").click());

    expect(onError).toHaveBeenCalledWith(openError);
    expect(getRoot).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain("目录不可用");
    expect(findButton(container, "打开目录").disabled).toBe(true);
  });

  it("selects before setting a custom directory and refreshes the display", async () => {
    const calls: string[] = [];
    const selectRoot = vi.fn().mockImplementation(async () => {
      calls.push("select");
      return "/Volumes/Work/task-contexts";
    });
    const setRoot = vi.fn().mockImplementation(async (path: string) => {
      calls.push(`set:${path}`);
      return {
        path,
        defaultPath:
          "/Users/test/Library/Application Support/BTaskAssistant/tasks",
        custom: true,
        available: true,
      };
    });
    const onSuccess = vi.fn();
    window.go = {
      main: {
        App: {
          GetTaskContextRoot: vi.fn().mockResolvedValue({
            path: "/Users/test/Library/Application Support/BTaskAssistant/tasks",
            defaultPath:
              "/Users/test/Library/Application Support/BTaskAssistant/tasks",
            custom: false,
            available: true,
          }),
          SelectTaskContextRoot: selectRoot,
          SetTaskContextRoot: setRoot,
          OpenTaskContextRoot: vi.fn(),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(TaskContextSettings, {
          onSuccess,
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => findButton(container, "更改目录").click());

    expect(calls).toEqual(["select", "set:/Volumes/Work/task-contexts"]);
    expect(setRoot).toHaveBeenCalledWith("/Volumes/Work/task-contexts");
    expect(onSuccess).toHaveBeenCalledWith("任务资料根目录已更新");
    expect(container.textContent).toContain("自定义目录");
    expect(container.textContent).toContain("应用默认目录：");
    expect(
      (container.querySelector(
        'input[aria-label="任务资料根目录"]',
      ) as HTMLInputElement).value,
    ).toBe("/Volumes/Work/task-contexts");
  });

  it("keeps the current directory when selection is cancelled", async () => {
    const setRoot = vi.fn();
    const onSuccess = vi.fn();
    window.go = {
      main: {
        App: {
          GetTaskContextRoot: vi.fn().mockResolvedValue({
            path: "/current/tasks",
            defaultPath: "/current/tasks",
            custom: false,
            available: true,
          }),
          SelectTaskContextRoot: vi.fn().mockResolvedValue(""),
          SetTaskContextRoot: setRoot,
          OpenTaskContextRoot: vi.fn(),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(TaskContextSettings, {
          onSuccess,
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => findButton(container, "更改目录").click());

    expect(setRoot).not.toHaveBeenCalled();
    expect(onSuccess).not.toHaveBeenCalled();
    expect(
      (container.querySelector(
        'input[aria-label="任务资料根目录"]',
      ) as HTMLInputElement).value,
    ).toBe("/current/tasks");
  });

  it("keeps the current directory when migration fails", async () => {
    const migrationError = new Error("目标目录不是空目录");
    const onError = vi.fn();
    const onSuccess = vi.fn();
    window.go = {
      main: {
        App: {
          GetTaskContextRoot: vi.fn().mockResolvedValue({
            path: "/current/tasks",
            defaultPath: "/current/tasks",
            custom: false,
            available: true,
          }),
          SelectTaskContextRoot: vi.fn().mockResolvedValue("/target/tasks"),
          SetTaskContextRoot: vi.fn().mockRejectedValue(migrationError),
          OpenTaskContextRoot: vi.fn(),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(TaskContextSettings, {
          onSuccess,
          onError,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => findButton(container, "更改目录").click());

    expect(onError).toHaveBeenCalledWith(migrationError);
    expect(onSuccess).not.toHaveBeenCalled();
    expect(
      (container.querySelector(
        'input[aria-label="任务资料根目录"]',
      ) as HTMLInputElement).value,
    ).toBe("/current/tasks");
  });

  it("is available as its own settings category", async () => {
    window.go = {
      main: {
        App: {
          GetTaskContextRoot: vi.fn().mockResolvedValue({
            path: "/application/tasks",
            defaultPath: "/application/tasks",
            custom: false,
            available: true,
          }),
          SelectTaskContextRoot: vi.fn(),
          SetTaskContextRoot: vi.fn(),
          OpenTaskContextRoot: vi.fn(),
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(SettingsPage, {
          initialCategory: "storage",
          planeConnected: false,
          onBack: vi.fn(),
          onPlaneConnectionChange: vi.fn(),
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    const storageTab = container.querySelector(
      '#settings-tab-storage[aria-selected="true"]',
    );
    expect(storageTab?.textContent).toContain("任务资料");
    expect(container.querySelector(".task-context-settings-page")).not.toBeNull();
    expect(container.querySelectorAll('button[role="tab"]')).toHaveLength(4);
  });
});

describe("Native workspace persistence", () => {
  afterEach(() => {
    delete window.go;
    vi.restoreAllMocks();
  });

  it("serializes SaveState calls so an older write cannot finish last", async () => {
    let finishFirst!: () => void;
    const firstSave = new Promise<void>((resolve) => {
      finishFirst = resolve;
    });
    const payloads: string[] = [];
    const saveState = vi.fn().mockImplementation((payload: string) => {
      payloads.push(payload);
      return payload === "old" ? firstSave : Promise.resolve();
    });
    window.go = {
      main: { App: { SaveState: saveState } },
    } as unknown as typeof window.go;

    const oldWrite = workspaceStorage.setItem("workspace", "old");
    const newWrite = workspaceStorage.setItem("workspace", "new");
    await Promise.resolve();

    expect(payloads).toEqual(["old"]);
    finishFirst();
    await Promise.all([oldWrite, newWrite]);

    expect(payloads).toEqual(["old", "new"]);
  });

  it("continues the native write queue after a failed save", async () => {
    const saveError = new Error("disk full");
    const payloads: string[] = [];
    const saveState = vi.fn().mockImplementation((payload: string) => {
      payloads.push(payload);
      return payload === "broken"
        ? Promise.reject(saveError)
        : Promise.resolve();
    });
    window.go = {
      main: { App: { SaveState: saveState } },
    } as unknown as typeof window.go;

    const brokenWrite = workspaceStorage.setItem("workspace", "broken");
    const recoveredWrite = workspaceStorage.setItem("workspace", "recovered");

    await expect(brokenWrite).rejects.toBe(saveError);
    await expect(recoveredWrite).resolves.toBeUndefined();
    expect(payloads).toEqual(["broken", "recovered"]);
  });
});
