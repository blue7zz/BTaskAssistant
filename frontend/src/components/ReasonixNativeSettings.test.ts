import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReasonixNativeSettings } from "./ReasonixNativeSettings";

const mountSettings = vi.hoisted(() => vi.fn());

vi.mock(
  "../../../reasonix-app/desktop/frontend/src/settingsEmbedEntry",
  () => ({
    default: mountSettings,
    mountReasonixSettingsEmbed: mountSettings,
  }),
);

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("ReasonixNativeSettings", () => {
  let container: HTMLDivElement;
  let root: Root;
  let unmountSettings: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    unmountSettings = vi.fn();
    mountSettings.mockReset();
    mountSettings.mockImplementation(
      (_host: HTMLElement, options?: { onMounted?: () => void }) => {
        options?.onMounted?.();
        return unmountSettings;
      },
    );
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("在应用设置页挂载并卸载 RX 原生设置面板", async () => {
    await act(async () => {
      root.render(createElement(ReasonixNativeSettings));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const host = container.querySelector(
      "[data-testid='reasonix-native-settings-host']",
    );
    expect(mountSettings).toHaveBeenCalledTimes(1);
    expect(mountSettings.mock.calls[0][0]).toBe(host);
    expect(container.textContent).not.toContain("正在加载 RX 设置面板");

    act(() => root.unmount());
    root = createRoot(container);
    expect(unmountSettings).toHaveBeenCalledTimes(1);
  });
});
