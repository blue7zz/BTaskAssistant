/*
 * Reasonix 任务工作台（RX 标签页）——完全嵌入形态。
 *
 * reasonix 前端源码经动态 import 纳入 BTask 同一构建（vite 独立 chunk），
 * 渲染在宿主 div 的 shadow root 内（36k 行 styles.css 以 :host 改写注入，
 * 与宿主 DOM/样式完全隔离）。绑定调用同 document 直连 window.go.main.App
 * （reasonix bridge.ts 的 embed 分支），内核事件直连 window.runtime。
 * 一个任务 = 一个 Reasonix 会话（按 taskId 隔离，任务切换保留运行时）。
 */

import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { ensureReasonixTab } from "../lib/bridge";

interface RxBridgeProps {
  taskId: string;
  workspaceRoot: string;
  taskTitle?: string;
}

type MountReasonixEmbed = (host: HTMLElement, options?: { onMounted?: () => void }) => () => void;

interface EmbedModule {
  mountReasonixEmbed?: MountReasonixEmbed;
  default?: MountReasonixEmbed;
}

let embedModulePromise: Promise<EmbedModule> | undefined;

// 由 BTask App 启动 effect 主动调用；保留单一 Promise，组件挂载和后续任务
// 切换都复用同一份 Reasonix 前端模块。
export function preloadReasonixEmbed(): Promise<EmbedModule> {
  embedModulePromise ??= import(
    "../../../reasonix-app/desktop/frontend/src/embedEntry"
  ) as Promise<EmbedModule>;
  return embedModulePromise;
}

let lastActivationRequestSeq = 0;

function nextActivationRequestSeq(): number {
  lastActivationRequestSeq = Math.max(lastActivationRequestSeq + 1, Date.now());
  return lastActivationRequestSeq;
}

export function ReasonixPage({ taskId, workspaceRoot, taskTitle = "" }: RxBridgeProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const unmountRef = useRef<(() => void) | null>(null);
  const mountedRef = useRef(false);
  const [ready, setReady] = useState(false);
  const [workspaceStarting, setWorkspaceStarting] = useState(true);
  const [readyError, setReadyError] = useState("");

  // 打开时（或任务切换时）为任务建立 Reasonix 会话（复用已存在的运行时）。
  useEffect(() => {
    let active = true;
    // embed 已挂载后必须在任务切换期间保持存活；否则 ready=false 会触发
    // 挂载 effect 的 cleanup，而 mountedRef 又会阻止后续重新挂载。
    if (!mountedRef.current) setReady(false);
    setWorkspaceStarting(true);
    setReadyError("");
    // 序号跨组件重新进入仍保持单调递增，避免后端把新页面的首次请求判旧。
    const requestSeq = nextActivationRequestSeq();
    ensureReasonixTab(taskId, workspaceRoot, taskTitle, requestSeq)
      .then(() => {
        if (!active) return;
        setReady(true);
        // 已挂载时属于任务切换，后端激活完成即可继续操作；首次进入还要
        // 等 embed 首帧挂载完成，再收起启动提示。
        if (mountedRef.current) setWorkspaceStarting(false);
      })
      .catch((error: unknown) => {
        if (!active) return;
        const message = error instanceof Error ? error.message : String(error);
        // 过期激活请求（已被更新的请求接管，如快速切换或 effect 重跑）不是
        // 失败——忽略它，避免覆盖最新请求的成功状态（stale 意味着已有
        // 更新的请求在途或已完成）。
        if (message.includes("stale")) return;
        setWorkspaceStarting(false);
        setReadyError(message);
      });
    return () => {
      active = false;
    };
  }, [taskId, workspaceRoot, taskTitle]);

  // 挂载 reasonix App（模块已在主应用启动时预加载；compat.ts 只在
  // ?host=1 / ?browser=1 时删除 window.go——embed 无该参数，直连可用）。
  // 单实例：仅首次 ready 时挂载一次，任务切换不再卸载/重挂载。
  useEffect(() => {
    const host = hostRef.current;
    if (!host || !ready || mountedRef.current) return undefined;
    mountedRef.current = true;
    let disposed = false;
    let unmount: (() => void) | null = null;

    (async () => {
      // 设置 embed 标志：embed 模式下 reasonix 直连 window.go（不被 compat 删除）
      (window as unknown as { __RX_EMBED__?: boolean }).__RX_EMBED__ = true;
      const mod = await preloadReasonixEmbed();
      if (disposed) return;
      host.replaceChildren();
      // 动态 import chunk 在多入口共享/生产 minify 下导出不可靠，
      // 优先取 embed 模块挂载时写入的 window 全局挂载函数。
      const globalMount = (window as unknown as { __RX_MOUNT_EMBED__?: (host: HTMLElement, options?: { onMounted?: () => void }) => () => void })
        .__RX_MOUNT_EMBED__;
      // 安全读取 default/命名导出（vitest mock 无 default 时直接访问会抛错）
      const modRecord = mod as unknown as Record<string, unknown>;
      const modMount = typeof modRecord.default === "function" ? (modRecord.default as MountReasonixEmbed) : (modRecord.mountReasonixEmbed as MountReasonixEmbed | undefined);
      const mount = globalMount ?? modMount;
      if (typeof mount !== "function") {
        throw new Error("Reasonix embed 模块导出缺失");
      }
      unmount = mount(host, {
        onMounted: () => {
          if (!disposed) {
            setReady(true);
            setWorkspaceStarting(false);
          }
        },
      });
      unmountRef.current = unmount;
    })().catch((error: unknown) => {
      if (disposed) return;
      mountedRef.current = false;
      setReady(false);
      setWorkspaceStarting(false);
      setReadyError(error instanceof Error ? error.message : String(error));
    });

    return () => {
      disposed = true;
      (window as unknown as { __RX_EMBED__?: boolean }).__RX_EMBED__ = false;
      try {
        unmount?.();
      } catch (error) {
        console.error("reasonix embed unmount failed", error);
      }
      unmountRef.current = null;
    };
  }, [ready]);

  // 任务切换：单实例保活——reasonix App 不重建。ActivateReasonixTask 成功后
  // 后端发 host:tab-activated 事件，reasonix 前端 syncActiveTab 切换会话。
  // （App 首次挂载后常驻；ManualTaskDetail 切换任务只更新 props。）

  // 离开 RX 或切换主界面也不关闭任务控制器。后台运行时由 Manager 的 idle
  // LRU 限额回收，并在应用退出时统一关闭；再次进入可直接续接原会话。

  return (
    <section className="reasonix-page">
      <div className="reasonix-frame-wrap">
        {readyError && (
          <div className="reasonix-frame-error">
            <p>Reasonix 会话初始化失败。</p>
            <p>
              <code>{readyError}</code>
            </p>
            <p>请确认任务文件空间已就绪，或重新打开本页重试。</p>
          </div>
        )}
        {workspaceStarting && !readyError && (
          <div
            className={`reasonix-workspace-starting${ready ? " reasonix-workspace-starting--switching" : ""}`}
            role="status"
            aria-live="polite"
            data-testid="reasonix-workspace-starting"
          >
            <LoaderCircle className="spin" size={14} />
            <div>
              <strong>Reasonix 工作区正在启动……</strong>
              <span>正在初始化任务会话与文件空间</span>
            </div>
          </div>
        )}
        <div
          ref={hostRef}
          className="reasonix-embed-host"
          style={{ height: "100%", width: "100%" }}
          data-testid="reasonix-embed-host"
        />
      </div>
    </section>
  );
}
