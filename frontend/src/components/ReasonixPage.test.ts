import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReasonixPage } from "./ReasonixPage";
import * as bridge from "../lib/bridge";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("ReasonixPage bridge", () => {
  let container: HTMLDivElement;
  let root: Root;
  let postMessages: Array<{ source: string; type: string; id?: number; value?: unknown; error?: string }>;

  beforeEach(() => {
    vi.resetModules();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    postMessages = [];
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
    // mock wails 桥与事件通道
    (window as unknown as { go?: unknown }).go = {
      main: {
        App: {
          ListTabs: vi.fn().mockResolvedValue([]),
          SubmitToTab: vi.fn().mockResolvedValue(undefined),
        },
      },
    };
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

  const frameWindow = () => {
    const frame = container.querySelector("iframe") as HTMLIFrameElement | null;
    if (!frame?.contentWindow) return null;
    frame.contentWindow.postMessage = (message: unknown) => {
      postMessages.push(message as { source: string; type: string });
    };
    return frame;
  };

  it("初始化会话后渲染 iframe", async () => {
    await renderPage();
    const frame = container.querySelector("iframe");
    expect(frame).not.toBeNull();
    expect(frame?.getAttribute("src")).toContain("/reasonix/");
    expect(frame?.getAttribute("src")).toContain("host=1");
    expect(bridge.ensureReasonixTab).toHaveBeenCalledWith("task_1", "", "");
  });

  it("忽略伪造来源的消息（不调用绑定）", async () => {
    await renderPage();
    const frame = frameWindow();
    if (!frame) throw new Error("iframe 未渲染");
    const appMock = (window as unknown as { go: { main: { App: { ListTabs: ReturnType<typeof vi.fn> } } } })
      .go.main.App;
    // 伪造来源：event.source 不是 iframe 的 contentWindow
    const forged = new MessageEvent("message", {
      data: { source: "reasonix", type: "call", id: 1, method: "ListTabs", args: [] },
      source: window,
    });
    window.dispatchEvent(forged);
    expect(appMock.ListTabs).not.toHaveBeenCalled();
  });

  it("转发 iframe 调用到 wails 绑定并回传结果", async () => {
    await renderPage();
    const frame = frameWindow();
    if (!frame) throw new Error("iframe 未渲染");
    const appMock = (window as unknown as { go: { main: { App: { ListTabs: ReturnType<typeof vi.fn> } } } })
      .go.main.App;
    const real = new MessageEvent("message", {
      data: { source: "reasonix", type: "call", id: 42, method: "ListTabs", args: [] },
      source: frame.contentWindow,
      origin: window.location.origin,
    });
    await act(async () => {
      window.dispatchEvent(real);
    });
    expect(appMock.ListTabs).toHaveBeenCalledTimes(1);
    // 回传应答
    const reply = postMessages.find((m) => m.type === "result" && m.id === 42);
    expect(reply).toBeDefined();
  });

  it("未绑定方法回传错误而非崩溃", async () => {
    await renderPage();
    const frame = frameWindow();
    if (!frame) throw new Error("iframe 未渲染");
    const real = new MessageEvent("message", {
      data: { source: "reasonix", type: "call", id: 7, method: "DoesNotExist", args: [] },
      source: frame.contentWindow,
      origin: window.location.origin,
    });
    await act(async () => {
      window.dispatchEvent(real);
    });
    const reply = postMessages.find((m) => m.type === "result" && m.id === 7);
    expect(reply?.error).toContain("未绑定的方法");
  });

  it("订阅 reasonix:event 并转发进 iframe", async () => {
    await renderPage();
    const frame = frameWindow();
    if (!frame) throw new Error("iframe 未渲染");
    const runtimeMock = (window as unknown as { runtime: { EventsOn: ReturnType<typeof vi.fn> } })
      .runtime;
    expect(runtimeMock.EventsOn).toHaveBeenCalledWith("reasonix:event", expect.any(Function));
    const callback = runtimeMock.EventsOn.mock.calls[0][1] as (payload: unknown) => void;
    await act(async () => {
      callback({ kind: "turn_done", tabId: "task_abc" });
    });
    const event = postMessages.find((m) => m.type === "event");
    expect(event).toBeDefined();
    expect((event as { payload?: unknown }).payload).toEqual({
      kind: "turn_done",
      tabId: "task_abc",
    });
  });

  it("卸载时释放会话运行时", async () => {
    await renderPage();
    await act(async () => {
      root.unmount();
    });
    expect(bridge.closeReasonixTab).toHaveBeenCalledWith("task_1");
  });

  it("转发 open-external 消息到 wails 浏览器打开", async () => {
    await renderPage();
    const frame = frameWindow();
    if (!frame) throw new Error("iframe 未渲染");
    const runtimeMock = (window as unknown as { runtime: { BrowserOpenURL: ReturnType<typeof vi.fn> } })
      .runtime;
    const real = new MessageEvent("message", {
      data: { source: "reasonix", type: "open-external", url: "https://example.com/docs" },
      source: frame.contentWindow,
      origin: window.location.origin,
    });
    await act(async () => {
      window.dispatchEvent(real);
    });
    expect(runtimeMock.BrowserOpenURL).toHaveBeenCalledWith("https://example.com/docs");
  });

  it("任务切换时重载 iframe（避免显示旧任务）", async () => {
    await renderPage();
    const frameBefore = container.querySelector("iframe") as HTMLIFrameElement;
    const keyBefore = frameBefore?.getAttribute("key") ?? "";
    // 切换任务（模拟 ManualTaskDetail 无 key 时 prop 变化）
    await act(async () => {
      root.render(
        createElement(ReasonixPage, { taskId: "task_2", workspaceRoot: "" }),
      );
    });
    expect(bridge.ensureReasonixTab).toHaveBeenCalledWith("task_2", "", "");
    const frameAfter = container.querySelector("iframe") as HTMLIFrameElement;
    expect(frameAfter).not.toBe(frameBefore);
    expect(frameAfter?.getAttribute("src")).toContain("host=1");
  });
});
