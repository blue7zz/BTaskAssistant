// Embed 入口（BTask RX 完全嵌入形态）。
//
// 宿主（BTask）在挂载前必须设置 window.__RX_EMBED__ = true，然后调用
// mountReasonixEmbed(hostElement)。reasonix App 渲染进 host 的 shadow root，
// 36k 行 styles.css 以 :host 改写后注入 shadow——与宿主 DOM/样式完全隔离。
// 绑定调用经 bridge.ts 的 embed 分支直连 window.go（同 document，无 postMessage）。

import "./lib/compat";
import { createRoot } from "react-dom/client";
import type { Root } from "react-dom/client";
import { StrictMode } from "react";
import App from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { LocaleProvider } from "./lib/i18n";
import { ToastProvider } from "./lib/toast";
import { setEmbedShadowRoot } from "./lib/embedHost";
// 预构建的 :host 作用域样式（scripts/scope-rx-css.mjs 用 PostCSS AST 生成，
// 含 seti.woff 字体内联；prebuild 时重新生成）。
import scopedStyles from "./generated/scoped-styles.css?raw";
// heartbeat 样式同入 shadow（不再经 vite 打进主 document 的 CSS chunk）
import heartbeatRaw from "./custom/features/heartbeat/heartbeat.css?raw";

export interface ReasonixEmbedOptions {
  /** 挂载完成回调（首帧渲染后）。 */
  onMounted?: () => void;
}

/**
 * 把 Reasonix App 挂载到 host 元素（shadow root 内）。
 * 返回卸载函数：卸载 React 树并清理 embed 上下文。
 */
export function mountReasonixEmbed(host: HTMLElement, options: ReasonixEmbedOptions = {}): () => void {
  // host 元素可能在任务切换时被复用（ReasonixPage 不重挂载）——已存在的
  // shadow root 必须复用，否则 attachShadow 会抛 "already hosts a shadow tree"。
  const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" });

  const style = document.createElement("style");
  style.textContent = scopedStyles;
  shadow.appendChild(style);
  const heartbeatStyle = document.createElement("style");
  heartbeatStyle.textContent = heartbeatRaw;
  shadow.appendChild(heartbeatStyle);

  const rootEl = document.createElement("div");
  rootEl.className = "rx-app-root";
  rootEl.style.cssText = "height:100%;width:100%;";
  shadow.appendChild(rootEl);

  setEmbedShadowRoot(shadow);

  let reactRoot: Root | null = null;
  try {
    reactRoot = createRoot(rootEl);
    reactRoot.render(
      <StrictMode>
        <ErrorBoundary>
          <LocaleProvider>
            <ToastProvider>
              <App />
            </ToastProvider>
          </LocaleProvider>
        </ErrorBoundary>
      </StrictMode>,
    );
  } catch (error) {
    console.error("reasonix embed mount failed", error);
  }

  options.onMounted?.();

  return () => {
    try {
      reactRoot?.unmount();
    } catch (error) {
      console.error("reasonix embed unmount failed", error);
    }
    setEmbedShadowRoot(null);
    shadow.replaceChildren();
  };
}

// 生产构建下动态 import chunk 的命名/default 导出在多入口共享时不可靠，
// 因此额外挂到 window 全局——宿主侧从 window.__RX_MOUNT_EMBED__ 获取。
if (typeof window !== "undefined") {
  (window as unknown as { __RX_MOUNT_EMBED__?: typeof mountReasonixEmbed }).__RX_MOUNT_EMBED__ =
    mountReasonixEmbed;
}
export default mountReasonixEmbed;
