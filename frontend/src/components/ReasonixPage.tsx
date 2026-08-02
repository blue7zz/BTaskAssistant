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
import { closeReasonixTab, ensureReasonixTab } from "../lib/bridge";

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

export function ReasonixPage({ taskId, workspaceRoot, taskTitle = "" }: RxBridgeProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const unmountRef = useRef<(() => void) | null>(null);
  const requestSeqRef = useRef(0);
  const mountedRef = useRef(false);
  const [ready, setReady] = useState(false);
  const [readyError, setReadyError] = useState("");

  // 打开时（或任务切换时）为任务建立 Reasonix 会话（复用已存在的运行时）。
  useEffect(() => {
    let active = true;
    setReady(false);
    setReadyError("");
    // 激活序号单调递增：后端只让最新请求成为活动任务（防快速切换竞态）
    const requestSeq = ++requestSeqRef.current;
    ensureReasonixTab(taskId, workspaceRoot, taskTitle, requestSeq)
      .then(() => {
        if (active) setReady(true);
      })
      .catch((error: unknown) => {
        if (!active) return;
        setReadyError(error instanceof Error ? error.message : String(error));
      });
    return () => {
      active = false;
    };
  }, [taskId, workspaceRoot, taskTitle]);

  // 挂载 reasonix App（动态 import：embed 模块在挂载时才加载，且 compat.ts
  // 只在 ?host=1 / ?browser=1 时删除 window.go——embed 无该参数，直连可用）。
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
      const mod = (await import("../../../reasonix-app/desktop/frontend/src/embedEntry")) as EmbedModule;
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
          if (!disposed) setReady(true);
        },
      });
      unmountRef.current = unmount;
    })().catch((error: unknown) => {
      if (!disposed) return;
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

  return (
    <section className="reasonix-page">
      <div className="reasonix-frame-wrap">
        {readyError && (
          <div className="reasonix-frame-error">
            <p>Reasonix 会话初始化失败。</p>
            <p>
              <code>{readyError}</code>
            </p>
            <p>请确认任务工作区已就绪，或重新打开本页重试。</p>
          </div>
        )}
        {!ready && !readyError && (
          <div className="reasonix-frame-error">
            <LoaderCircle className="spin" size={14} />
            <p>正在初始化任务的 Reasonix 会话……</p>
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
