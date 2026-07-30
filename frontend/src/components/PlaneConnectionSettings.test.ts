import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWorkspaceStore } from "../store/workspace";
import { PlaneConnectionSettings } from "./PlaneConnectionSettings";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("Plane connection settings", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      tasks: [],
      trashedTasks: [],
      collectionCandidates: [],
      planeSettings: {
        baseUrl: "https://plane.fymyriad.com",
        workspaceSlug: "myriad",
        projectId: "",
        projectName: "",
        projectIdentifier: "",
        showInTaskSources: true,
      },
      selectedTaskId: undefined,
      statusFilter: "all",
      hydrated: true,
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    delete window.go;
    vi.restoreAllMocks();
  });

  it("connects Plane and enables the task-source visibility switch", async () => {
    let resolveTokenCheck: (stored: boolean) => void = () => undefined;
    const hasPlaneToken = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          resolveTokenCheck = resolve;
        }),
    );
    const setupPlaneConnection = vi.fn().mockResolvedValue({
      baseUrl: "https://plane.fymyriad.com",
      workspaceSlug: "myriad",
      projects: [
        {
          id: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
          name: "myriad",
          identifier: "MYRIA",
        },
      ],
    });
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          HasPlaneToken: hasPlaneToken,
          SetupPlaneConnection: setupPlaneConnection,
        },
      },
    } as unknown as typeof window.go;
    const onConnectionChange = vi.fn();

    await act(async () => {
      root.render(
        createElement(PlaneConnectionSettings, {
          connected: false,
          onConnectionChange,
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

    const sourceSwitch = container.querySelector(
      'input[aria-label="在任务来源中显示 Plane 收集箱"]',
    ) as HTMLInputElement;
    expect(sourceSwitch.disabled).toBe(true);

    const tokenInput = container.querySelector(
      'input[type="password"]',
    ) as HTMLInputElement;
    const valueSetter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;
    await act(async () => {
      valueSetter?.call(tokenInput, "plane_api_test");
      tokenInput.dispatchEvent(new Event("input", { bubbles: true }));
    });

    const connectButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("连接并加载选项"))!;
    await act(async () => {
      connectButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(setupPlaneConnection).toHaveBeenCalledWith(
      "https://plane.fymyriad.com/myriad/",
      "plane_api_test",
    );
    expect(useWorkspaceStore.getState().planeSettings).toMatchObject({
      projectId: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
      projectName: "myriad",
      projectIdentifier: "MYRIA",
      showInTaskSources: true,
    });

    await act(async () => {
      root.render(
        createElement(PlaneConnectionSettings, {
          connected: true,
          onConnectionChange,
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
    });
    expect(sourceSwitch.disabled).toBe(false);
    expect(sourceSwitch.checked).toBe(true);

    await act(async () => {
      resolveTokenCheck(false);
      await Promise.resolve();
    });
    expect(sourceSwitch.disabled).toBe(false);

    const addressInput = container.querySelector(
      'input[placeholder="https://plane.example.com/my-team/"]',
    ) as HTMLInputElement;
    await act(async () => {
      valueSetter?.call(addressInput, "https://plane.example.com/other/");
      addressInput.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(sourceSwitch.disabled).toBe(true);
    await act(async () => {
      valueSetter?.call(addressInput, "https://plane.fymyriad.com/myriad/");
      addressInput.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(sourceSwitch.disabled).toBe(false);

    const connectionChangeCalls = onConnectionChange.mock.calls.length;
    await act(async () => sourceSwitch.click());

    expect(useWorkspaceStore.getState().planeSettings.showInTaskSources).toBe(
      false,
    );
    expect(onConnectionChange).toHaveBeenCalledTimes(connectionChangeCalls);
  });
});
