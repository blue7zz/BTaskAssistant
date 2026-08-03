// BTask “设置 → Reasonix 设置”中的原生 RX 设置面板入口。
// 只挂载 SettingsPanel，不创建第二个工作区或会话控制器。

import "./lib/compat";
import { StrictMode } from "react";
import { flushSync } from "react-dom";
import { createRoot, type Root } from "react-dom/client";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { SettingsPanel } from "./components/SettingsPanel";
import {
  EMBEDDED_REASONIX_SETTINGS_CHANGED_EVENT,
  registerAuxiliaryEmbedShadowRoot,
} from "./lib/embedHost";
import { LocaleProvider } from "./lib/i18n";
import { ToastProvider } from "./lib/toast";
import type { SettingsTab, SettingsView } from "./lib/types";
import scopedStyles from "./generated/scoped-styles.css?raw";
import inlineStyles from "./settingsEmbed.css?raw";

interface ReasonixSettingsEmbedOptions {
  initialTab?: SettingsTab;
  onMounted?: () => void;
}

type DesktopPlatform = "darwin" | "windows" | "linux";

function desktopPlatform(): DesktopPlatform {
  const platform = navigator.platform.toLowerCase();
  if (platform.includes("win")) return "windows";
  if (platform.includes("mac")) return "darwin";
  return "linux";
}

export function mountReasonixSettingsEmbed(
  host: HTMLElement,
  options: ReasonixSettingsEmbedOptions = {},
): () => void {
  const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" });
  shadow.replaceChildren();

  const style = document.createElement("style");
  style.textContent = scopedStyles;
  shadow.appendChild(style);
  const inlineStyle = document.createElement("style");
  inlineStyle.textContent = inlineStyles;
  shadow.appendChild(inlineStyle);

  const rootElement = document.createElement("div");
  rootElement.className = "rx-app-root reasonix-settings-inline-root";
  shadow.appendChild(rootElement);

  const unregisterRoot = registerAuxiliaryEmbedShadowRoot(shadow);
  let changedSettings: SettingsView | null | undefined;
  let reactRoot: Root | null = createRoot(rootElement);
  flushSync(() => {
    reactRoot?.render(
      <StrictMode>
        <ErrorBoundary>
          <LocaleProvider>
            <ToastProvider>
              <SettingsPanel
                inline
                initialTab={options.initialTab ?? "general"}
                desktopPlatform={desktopPlatform()}
                agentRunning={false}
                onClose={() => undefined}
                onChanged={(settings) => {
                  changedSettings = settings;
                }}
                onUseSubagent={() => undefined}
              />
            </ToastProvider>
          </LocaleProvider>
        </ErrorBoundary>
      </StrictMode>,
    );
  });
  options.onMounted?.();

  return () => {
    try {
      reactRoot?.unmount();
    } finally {
      reactRoot = null;
      unregisterRoot();
      shadow.replaceChildren();
      if (changedSettings !== undefined) {
        window.dispatchEvent(
          new CustomEvent(EMBEDDED_REASONIX_SETTINGS_CHANGED_EVENT, {
            detail: changedSettings,
          }),
        );
      }
    }
  };
}

export default mountReasonixSettingsEmbed;
