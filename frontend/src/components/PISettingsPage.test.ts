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
            label: "PI",
            configured: true,
            requirementAnalysis: false,
            development: false,
            description: "PI 可用于只读需求分析。",
            commandPath: "/opt/homebrew/bin/pi",
            version: "0.82.1",
          },
          onSuccess,
        }),
      );
    });

    expect(container.textContent).toContain("已启用每任务隔离策略");
    expect(container.textContent).toContain("0.82.1");

    const modelInput = container.querySelector(
      'input[placeholder*="provider/model"]',
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
      "PI 设置已保存，将在原生 RPC 接入后使用",
    );
  });
});
