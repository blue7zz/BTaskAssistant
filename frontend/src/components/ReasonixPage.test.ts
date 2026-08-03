import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { ReasonixPage } from "./ReasonixPage";
import * as bridge from "../lib/bridge";

vi.mock("../../../reasonix-app/desktop/frontend/src/embedEntry", () => {
  const mount = vi.fn(() => () => undefined);
  return { default: mount, mountReasonixEmbed: mount };
});

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("ReasonixPage embed", () => {
  let container: HTMLDivElement;
  let root: Root;
  let mountReasonixEmbed: Mock;

  beforeEach(async () => {
    vi.resetModules();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    vi.spyOn(bridge, "ensureReasonixTab").mockResolvedValue({
      id: "task_abc",
      workspaceRoot: "/tmp/demo",
      topicId: "topic_x",
      topicTitle: "demo",
      label: "demo",
      ready: true,
      running: false,
      mode: "normal",
    } as bridge.ReasonixTabView);
    vi.spyOn(bridge, "closeReasonixTab").mockResolvedValue();
    // 动态 import 的 embed 模块：挂载为同步 fn
    const mod = await import("../../../reasonix-app/desktop/frontend/src/embedEntry");
    mountReasonixEmbed = mod.mountReasonixEmbed as unknown as Mock;
    mountReasonixEmbed.mockClear();
    mountReasonixEmbed.mockImplementation(
      (_host: HTMLElement, options?: { onMounted?: () => void }) => {
        options?.onMounted?.();
        return () => undefined;
      },
    );
    (window as unknown as { runtime?: unknown }).runtime = {
      EventsOn: vi.fn().mockReturnValue(() => undefined),
      BrowserOpenURL: vi.fn(),
    };
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    delete (window as unknown as { go?: unknown }).go;
    delete (window as unknown as { runtime?: unknown }).runtime;
    vi.restoreAllMocks();
  });

  const renderPage = async () => {
    await act(async () => {
      root.render(
        createElement(ReasonixPage, { taskId: "task_1", workspaceRoot: "" }),
      );
    });
  };

  it("首次打开时主动提示工作区正在启动", async () => {
    let resolveActivation!: (value: bridge.ReasonixTabView) => void;
    const activation = new Promise<bridge.ReasonixTabView>((resolve) => {
      resolveActivation = resolve;
    });
    vi.mocked(bridge.ensureReasonixTab).mockReturnValueOnce(activation);

    act(() => {
      root.render(
        createElement(ReasonixPage, { taskId: "task_1", workspaceRoot: "" }),
      );
    });

    const status = container.querySelector(
      "[data-testid='reasonix-workspace-starting']",
    );
    expect(status).not.toBeNull();
    expect(status?.getAttribute("role")).toBe("status");
    expect(status?.textContent).toContain("Reasonix 工作区正在启动");

    await act(async () => {
      resolveActivation({
        id: "task_1",
        workspaceRoot: "/tmp/task-1",
        topicId: "topic_1",
        topicTitle: "task 1",
        label: "task 1",
        ready: true,
        running: false,
        mode: "normal",
      });
      await activation;
    });
  });

  it("初始化会话后挂载 embed（shadow 容器 + mountReasonixEmbed）", async () => {
    await renderPage();
    const host = container.querySelector(
      "[data-testid='reasonix-embed-host']",
    ) as HTMLElement | null;
    expect(host).not.toBeNull();
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(1);
    expect(mountReasonixEmbed.mock.calls[0][0]).toBe(host);
    // embed 标志在挂载前设置
    expect(
      (window as unknown as { __RX_EMBED__?: boolean }).__RX_EMBED__,
    ).toBe(true);
  });

  it("任务切换只激活不重挂载（单实例保活）", async () => {
    await renderPage();
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(1);
    const firstRequestSeq = vi.mocked(bridge.ensureReasonixTab).mock.calls[0][3]!;
    await act(async () => {
      root.render(
        createElement(ReasonixPage, {
          taskId: "task_2",
          workspaceRoot: "",
        }),
      );
    });
    // 任务切换 → 仅 Activate（序号递增）；App 保持挂载（不重建）
    expect(bridge.ensureReasonixTab).toHaveBeenCalledTimes(2);
    expect(bridge.ensureReasonixTab).toHaveBeenLastCalledWith(
      "task_2",
      "",
      "",
      expect.any(Number),
    );
    const secondRequestSeq = vi.mocked(bridge.ensureReasonixTab).mock.calls[1][3]!;
    expect(secondRequestSeq).toBeGreaterThan(firstRequestSeq);
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(1);
  });

  it("任务切换等待激活期间保留已挂载的 embed", async () => {
    let resolveSecondActivation!: (value: bridge.ReasonixTabView) => void;
    const secondActivation = new Promise<bridge.ReasonixTabView>((resolve) => {
      resolveSecondActivation = resolve;
    });
    vi.mocked(bridge.ensureReasonixTab)
      .mockResolvedValueOnce({
        id: "task_1",
        workspaceRoot: "/tmp/task-1",
        topicId: "topic_1",
        topicTitle: "task 1",
        label: "task 1",
        ready: true,
        running: false,
        mode: "normal",
      })
      .mockReturnValueOnce(secondActivation);
    const unmountEmbed = vi.fn();
    mountReasonixEmbed.mockImplementationOnce(() => unmountEmbed);

    await renderPage();
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        createElement(ReasonixPage, {
          taskId: "task_2",
          workspaceRoot: "",
        }),
      );
    });

    expect(unmountEmbed).not.toHaveBeenCalled();
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(1);
    expect(
      container.querySelector("[data-testid='reasonix-workspace-starting']")
        ?.textContent,
    ).toContain("Reasonix 工作区正在启动");

    await act(async () => {
      resolveSecondActivation({
        id: "task_2",
        workspaceRoot: "/tmp/task-2",
        topicId: "topic_2",
        topicTitle: "task 2",
        label: "task 2",
        ready: true,
        running: false,
        mode: "normal",
      });
      await secondActivation;
    });

    expect(unmountEmbed).not.toHaveBeenCalled();
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(1);
    expect(
      container.querySelector("[data-testid='reasonix-workspace-starting']"),
    ).toBeNull();
  });

  it("组件重新进入后继续使用更大的激活序号", async () => {
    await renderPage();
    const firstRequestSeq = vi.mocked(bridge.ensureReasonixTab).mock.calls[0][3]!;

    act(() => root.unmount());
    root = createRoot(container);
    await act(async () => {
      root.render(
        createElement(ReasonixPage, {
          taskId: "task_2",
          workspaceRoot: "",
        }),
      );
    });

    const secondRequestSeq = vi.mocked(bridge.ensureReasonixTab).mock.calls[1][3]!;
    expect(secondRequestSeq).toBeGreaterThan(firstRequestSeq);
  });

  it("embed 挂载失败时结束加载并显示错误", async () => {
    mountReasonixEmbed.mockImplementationOnce(() => {
      throw new Error("embed mount failed");
    });

    await renderPage();

    expect(container.textContent).toContain("Reasonix 会话初始化失败");
    expect(container.textContent).toContain("embed mount failed");
  });

  it("卸载 RX 界面时保留任务运行时供再次进入继续会话", async () => {
    await renderPage();
    expect(bridge.closeReasonixTab).not.toHaveBeenCalled();
    act(() => root.unmount());
    expect(bridge.closeReasonixTab).not.toHaveBeenCalled();
  });

  it("过期激活请求（stale）不显示错误（已有更新的请求接管）", async () => {
    vi.mocked(bridge.ensureReasonixTab).mockRejectedValueOnce(
      new Error("stale reasonix activate request"),
    );
    await renderPage();
    // stale 被忽略：不显示"初始化失败"错误（仍在等待/初始化中即可）
    expect(container.textContent).not.toContain("初始化失败");
    expect(container.textContent).toContain("正在初始化");
    expect(bridge.ensureReasonixTab).toHaveBeenCalledTimes(1);
  });

  it("会话初始化失败时显示错误而非挂载", async () => {
    vi.mocked(bridge.ensureReasonixTab).mockRejectedValueOnce(
      new Error("工作区未就绪"),
    );
    await renderPage();
    expect(
      container.querySelector(".reasonix-frame-error"),
    ).not.toBeNull();
    expect(mountReasonixEmbed).not.toHaveBeenCalled();
  });
});
