/*
 * Reasonix 任务工作台（RX 标签页）。
 *
 * 融合形态：Reasonix 前端源码与 BTask 同仓同构建，iframe 以 ?host=1 模式加载
 * 同源 /reasonix/ 产物。Reasonix 的每个绑定调用经 postMessage 转发到这里，
 * 由本组件携带任务上下文调用 BTask 的融合绑定（window.go.main.App.*，按
 * taskId 隔离会话），结果回传 iframe；Reasonix 内核事件（"reasonix:event"）
 * 同样经这里转发进 iframe。一个任务 = 一个 Reasonix 会话。
 */

import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { closeReasonixTab, ensureReasonixTab } from "../lib/bridge";

const REASONIX_URL = `${(import.meta.env.VITE_REASONIX_URL as string | undefined) ?? "/reasonix/"}?host=1`;

const HOST_REPLY_SOURCE = "btask-reasonix";

interface RxBridgeProps {
  taskId: string;
  workspaceRoot: string;
  taskTitle?: string;
}

export function ReasonixPage({ taskId, workspaceRoot, taskTitle = "" }: RxBridgeProps) {
  const frameRef = useRef<HTMLIFrameElement | null>(null);
  const [frameKey, setFrameKey] = useState(0);
  const [frameError, setFrameError] = useState(false);
  const [frameStalled, setFrameStalled] = useState(false);
  const [ready, setReady] = useState(false);
  const [readyError, setReadyError] = useState("");

  // 打开时（或任务切换时）为任务建立 Reasonix 会话（复用已存在的运行时）。
  useEffect(() => {
    let active = true;
    setReady(false);
    setReadyError("");
    ensureReasonixTab(taskId, workspaceRoot, taskTitle)
      .then((view) => {
        if (!active) return;
        setReady(true);
        void view;
      })
      .catch((error: unknown) => {
        if (!active) return;
        setReadyError(error instanceof Error ? error.message : String(error));
      });
    return () => {
      active = false;
    };
  }, [taskId, workspaceRoot, taskTitle, frameKey]);

  // 任务切换时重载 iframe：reasonix 前端按后端 tab 状态渲染，iframe 不重载
  // 会继续显示旧任务的内容（ManualTaskDetail 无 key，切换任务不重挂载）。
  const prevTaskIdRef = useRef(taskId);
  useEffect(() => {
    if (prevTaskIdRef.current !== taskId) {
      prevTaskIdRef.current = taskId;
      setFrameKey((key) => key + 1);
    }
  }, [taskId]);

  // 组件真正卸载（离开任务详情页）时释放会话运行时（会话文件保留）。
  // 任务切换（taskId 变化）不关闭控制器——切换回来秒开，会话状态保留。
  const latestTaskIdRef = useRef(taskId);
  useEffect(() => {
    latestTaskIdRef.current = taskId;
  }, [taskId]);
  useEffect(() => {
    return () => {
      void closeReasonixTab(latestTaskIdRef.current);
    };
  }, []);

  // 主 frame 桥：iframes 的绑定调用 → 融合绑定；内核事件 → iframe。
  useEffect(() => {
    const frame = frameRef.current;
    if (!frame) return undefined;
    const iframeWindow = frame.contentWindow;
    if (!iframeWindow) return undefined;

    const handleMessage = (event: MessageEvent) => {
      // 只接受来自本页面 Reasonix iframe 的调用，杜绝伪造源调用绑定方法
      if (event.source !== iframeWindow) return;
      if (event.origin !== window.location.origin) return;
      const data = event.data as
        | { source?: string; type?: string; id?: number; method?: string; args?: unknown[]; url?: string }
        | undefined;
      if (!data || data.source !== "reasonix") return;
      // 外部链接：交给 wails 原生浏览器打开（webview 无多窗口）
      if (data.type === "open-external" && typeof data.url === "string") {
        window.runtime?.BrowserOpenURL?.(data.url);
        return;
      }
      if (data.type !== "call") return;
      const { id, method, args } = data;
      if (!method || typeof id !== "number") return;
      const bound = window.go?.main?.App as Record<string, unknown> | undefined;
      const fn = bound?.[method];
      if (typeof fn !== "function") {
        iframeWindow.postMessage(
          { source: HOST_REPLY_SOURCE, type: "result", id, error: `未绑定的方法: ${method}` },
          "*",
        );
        return;
      }
      Promise.resolve(fn.apply(bound, args ?? []))
        .then((value) => {
          iframeWindow.postMessage({ source: HOST_REPLY_SOURCE, type: "result", id, value }, "*");
        })
        .catch((error: unknown) => {
          // 附加方法名，便于定位任何 wails 绑定错误（如参数解析失败）
          const detail = error instanceof Error ? error.message : String(error);
          console.error(`[reasonix-bridge] 调用失败 method=${method}: ${detail}`);
          iframeWindow.postMessage(
            {
              source: HOST_REPLY_SOURCE,
              type: "result",
              id,
              error: `[${method}] ${detail}`,
            },
            "*",
          );
        });
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [frameKey, ready]);

  // 加载诊断：iframe onLoad 后 15s 内未出现 Reasonix 标题则提示（白屏排查）。
  useEffect(() => {
    if (!ready) return undefined;
    const timer = window.setTimeout(() => {
      const frame = frameRef.current;
      const doc = frame?.contentDocument;
      if (doc && !doc.title && !doc.querySelector("#root > *")) {
        setFrameStalled(true);
      }
    }, 15000);
    return () => window.clearTimeout(timer);
  }, [ready, frameKey]);

  // Reasonix 内核事件：BTask 桥监听 reasonix:event 后转发进 iframe。
  useEffect(() => {
    const frame = frameRef.current;
    if (!frame) return undefined;
    const iframeWindow = frame.contentWindow;
    if (!iframeWindow) return undefined;
    const runtime = window.runtime;
    if (!runtime?.EventsOn) return undefined;
    const unsubscribe = runtime.EventsOn("reasonix:event", (payload) => {
      iframeWindow.postMessage(
        { source: HOST_REPLY_SOURCE, type: "event", payload },
        "*",
      );
    });
    return typeof unsubscribe === "function" ? unsubscribe : undefined;
  }, [frameKey, ready]);

  return (
    <section className="reasonix-page">
      <div className="reasonix-frame-wrap">
        {readyError && (
          <div className="reasonix-frame-error">
            <p>Reasonix 会话初始化失败。</p>
            <p>
              <code>{readyError}</code>
            </p>
            <p>请确认任务工作区已就绪，或刷新重试。</p>
          </div>
        )}
        {!ready && !readyError && (
          <div className="reasonix-frame-error">
            <LoaderCircle className="spin" size={14} />
            <p>正在初始化任务的 Reasonix 会话……</p>
          </div>
        )}
        {frameError && !readyError && (
          <div className="reasonix-frame-error">
            <p>Reasonix 前端未能加载。</p>
            <p>请确认 reasonix-app/desktop/frontend 已构建。</p>
          </div>
        )}
        {frameStalled && !frameError && !readyError && (
          <div className="reasonix-frame-error">
            <p>Reasonix 页面加载超时（15 秒无内容）。</p>
            <p>请点击“刷新”重试；若持续失败，检查应用日志中的 reasonix 资源诊断。</p>
          </div>
        )}
        {ready && (
          <iframe
            key={frameKey}
            ref={frameRef}
            className="reasonix-frame"
            src={REASONIX_URL}
            title="Reasonix 工作台"
            onLoad={() => setFrameError(false)}
            onError={() => setFrameError(true)}
          />
        )}
      </div>
    </section>
  );
}
