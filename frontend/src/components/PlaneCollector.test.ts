import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWorkspaceStore } from "../store/workspace";
import { PlaneCollector } from "./PlaneCollector";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("PlaneCollector", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    useWorkspaceStore.setState({
      tasks: [],
      collectionCandidates: [],
      planeSettings: {
        baseUrl: "https://plane.fymyriad.com",
        workspaceSlug: "myriad",
        projectId: "",
        projectName: "",
        projectIdentifier: "",
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
  });

  it("saves the PAT and automatically selects the only discovered project", async () => {
    const savePlaneToken = vi.fn().mockResolvedValue(undefined);
    const listPlaneProjects = vi.fn().mockResolvedValue([
      {
        id: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
        name: "myriad",
        identifier: "MYRIA",
      },
    ]);
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          HasPlaneToken: vi.fn().mockResolvedValue(false),
          SavePlaneToken: savePlaneToken,
          ListPlaneProjects: listPlaneProjects,
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(PlaneCollector, {
          onSuccess: vi.fn(),
          onError: vi.fn(),
          onOpenTask: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

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
    const saveButton = Array.from(
      container.querySelectorAll("button"),
    ).find((button) => button.textContent?.includes("保存并发现项目"));
    expect(saveButton).toBeTruthy();

    await act(async () => {
      saveButton?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(savePlaneToken).toHaveBeenCalledWith(
      "https://plane.fymyriad.com",
      "myriad",
      "plane_api_test",
    );
    expect(listPlaneProjects).toHaveBeenCalledWith(
      "https://plane.fymyriad.com",
      "myriad",
    );
    expect(useWorkspaceStore.getState().planeSettings).toMatchObject({
      projectId: "d4074079-8ce3-4cf2-8cd5-7e0af8c67f57",
      projectName: "myriad",
      projectIdentifier: "MYRIA",
    });
    expect(useWorkspaceStore.getState().tasks).toHaveLength(0);
    expect(container.textContent).toContain("MYRIA");
  });
});
