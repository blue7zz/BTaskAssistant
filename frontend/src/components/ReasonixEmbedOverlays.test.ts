import { afterEach, beforeEach, describe, expect, it } from "vitest";
import anchoredPopoverSource from "../../../reasonix-app/desktop/frontend/src/components/AnchoredPopover.tsx?raw";
import {
  registerAuxiliaryEmbedShadowRoot,
  rxQuerySelector,
  setEmbedShadowRoot,
} from "../../../reasonix-app/desktop/frontend/src/lib/embedHost";

describe("Reasonix embed overlays", () => {
  let host: HTMLDivElement;
  let shadow: ShadowRoot;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
    shadow = host.attachShadow({ mode: "open" });
    const appRoot = document.createElement("div");
    appRoot.className = "rx-app-root";
    const chatPane = document.createElement("div");
    chatPane.className = "chat-pane";
    appRoot.appendChild(chatPane);
    shadow.appendChild(appRoot);
    setEmbedShadowRoot(shadow);
  });

  afterEach(() => {
    setEmbedShadowRoot(null);
    host.remove();
  });

  it("为嵌入宿主补齐基础主题与嵌入标记", () => {
    expect(host.getAttribute("data-rx-embed")).toBe("true");
    expect(host.getAttribute("data-theme-style")).toBe("graphite");
  });

  it("将底部和设置页的锚点弹层留在 RX Shadow DOM 内", () => {
    expect(anchoredPopoverSource).toContain("rxPortalTarget()");
    expect(anchoredPopoverSource).not.toMatch(/createPortal\([\s\S]*document\.body,\s*\);/);
  });

  it("关闭内嵌设置面板后恢复工作区 Shadow DOM 上下文", () => {
    const workspaceMarker = document.createElement("div");
    workspaceMarker.className = "workspace-marker";
    shadow.appendChild(workspaceMarker);

    const settingsHost = document.createElement("div");
    document.body.appendChild(settingsHost);
    const settingsShadow = settingsHost.attachShadow({ mode: "open" });
    const settingsMarker = document.createElement("div");
    settingsMarker.className = "settings-marker";
    settingsShadow.appendChild(settingsMarker);
    const unregister = registerAuxiliaryEmbedShadowRoot(settingsShadow);

    expect(rxQuerySelector(".settings-marker")).toBe(settingsMarker);
    expect(rxQuerySelector(".workspace-marker")).toBeNull();

    unregister();
    expect(rxQuerySelector(".workspace-marker")).toBe(workspaceMarker);
    settingsHost.remove();
  });
});
