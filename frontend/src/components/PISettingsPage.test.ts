import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_PI_SETTINGS } from "../domain/engine";
import { useWorkspaceStore } from "../store/workspace";
import { PISettingsPage } from "./PISettingsPage";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("PI settings page", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      piSettings: { ...DEFAULT_PI_SETTINGS },
      hydrated: true,
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  it("saves a compatible per-run model and thinking configuration", async () => {
    const onSuccess = vi.fn();
    await act(async () => {
      root.render(
        createElement(PISettingsPage, {
          engine: {
            id: "pi",
            label: "PI / oh-my-pi",
            configured: true,
            requirementAnalysis: true,
            development: false,
            description: "PI 可用于只读需求分析。",
            commandPath: "/Users/blue/.local/bin/omp",
            version: "omp/16.5.2",
          },
          onSuccess,
        }),
      );
    });

    expect(container.textContent).toContain("已启用 gpt-5.5 兼容修复");
    expect(container.textContent).toContain("omp/16.5.2");

    const modelInput = container.querySelector(
      'input[placeholder*="openai-codex/gpt-5.5"]',
    ) as HTMLInputElement;
    const inputSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;
    await act(async () => {
      inputSetter?.call(modelInput, "openai-codex/gpt-5.5");
      modelInput.dispatchEvent(new Event("input", { bubbles: true }));
      (
        container.querySelector('input[value="high"]') as HTMLInputElement
      ).click();
      const timeout = container.querySelector("select") as HTMLSelectElement;
      timeout.value = "5";
      timeout.dispatchEvent(new Event("change", { bubbles: true }));
    });

    const saveButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("保存 PI 设置"),
    );
    await act(async () => saveButton?.click());

    expect(useWorkspaceStore.getState().piSettings).toEqual({
      model: "openai-codex/gpt-5.5",
      thinkingEffort: "high",
      timeoutMinutes: 5,
    });
    expect(onSuccess).toHaveBeenCalledWith(
      "PI 详细设置已保存，下一轮分析将使用新配置",
    );
  });
});
