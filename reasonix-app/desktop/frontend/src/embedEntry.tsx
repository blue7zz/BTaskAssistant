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
import stylesRaw from "./styles.css?raw";

/**
 * 把 styles.css 改写为 :host 作用域版本（shadow DOM 注入用）。
 * - html / body / :root → :host（含 `html.xxx` / `body.xxx` 形式）
 * - `html body` / `html > body` 等组合 → :host
 * - 其余选择器保持（shadow 内天然隔离）
 */
function scopeStyles(css: string): string {
  // 注释先行保护（选择器段可能跨行吞掉注释），替换完成后恢复。
  const comments: string[] = [];
  const protectedCss = css.replace(/\/\*[\s\S]*?\*\//g, (comment) => {
    comments.push(comment);
    return `/*__RX_C${comments.length - 1}__*/`;
  });
  const scopedCss = protectedCss.replace(/(^|\n)([^{}]+)\{/g, (_whole, lead: string, sel: string) => {
    // 选择器段内替换（注释已保护，无副作用）：
    //   :root / html / body（元素选择器，前导为选择器边界）→ :host
    const scoped = sel
      .replace(/:root(?=[\s,:[]|$)/g, ":host")
      .replace(/(^|[\s,>+~])(html|body)(?=[\s.,:>[]|$)/g, "$1:host")
      .replace(/(^|[\s,>+~])(html|body)(?=\s)/g, "$1:host");
    return `${lead}${scoped}{`;
  });
  return scopedCss.replace(/\/\*__RX_C(\d+)__\*\//g, (_whole, index: string) => comments[Number(index)] ?? "");
}

const scopedStyles = scopeStyles(stylesRaw);

export interface ReasonixEmbedOptions {
  /** 挂载完成回调（首帧渲染后）。 */
  onMounted?: () => void;
}

/**
 * 把 Reasonix App 挂载到 host 元素（shadow root 内）。
 * 返回卸载函数：卸载 React 树并清理 embed 上下文。
 */
export function mountReasonixEmbed(host: HTMLElement, options: ReasonixEmbedOptions = {}): () => void {
  const shadow = host.attachShadow({ mode: "open" });

  const style = document.createElement("style");
  style.textContent = scopedStyles;
  shadow.appendChild(style);

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
