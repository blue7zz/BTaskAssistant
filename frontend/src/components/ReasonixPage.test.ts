import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
  let mountReasonixEmbed: ReturnType<typeof vi.fn>;

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
    mountReasonixEmbed = mod.mountReasonixEmbed as unknown as ReturnType<typeof vi.fn>;
    mountReasonixEmbed.mockClear();
    mountReasonixEmbed.mockImplementation(() => () => undefined);
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

  it("任务切换时重挂载 embed（避免显示旧任务）", async () => {
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
    // 任务切换 → ensureReasonixTab 再次调用 + embed 重新挂载
    expect(bridge.ensureReasonixTab).toHaveBeenCalledTimes(2);
    expect(mountReasonixEmbed).toHaveBeenCalledTimes(2);
  });

  it("卸载时释放会话运行时（closeReasonixTab）", async () => {
    await renderPage();
    expect(bridge.closeReasonixTab).not.toHaveBeenCalled();
    act(() => root.unmount());
    expect(bridge.closeReasonixTab).toHaveBeenCalledWith("task_1");
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
