import { describe, expect, it } from "vitest";
import {
  planeWorkItemURL,
  planeWorkspaceAddress,
  type PlaneSettings,
} from "./collection";

function settings(
  baseUrl: string,
  workspaceSlug: string,
): PlaneSettings {
  return {
    baseUrl,
    workspaceSlug,
    projectId: "project-1",
    projectName: "Project One",
    projectIdentifier: "ONE",
    showInTaskSources: true,
  };
}

describe("Plane work item links", () => {
  it("builds self-hosted workspace and work item addresses", () => {
    const value = settings("https://plane.fymyriad.com", "myriad");

    expect(planeWorkspaceAddress(value)).toBe(
      "https://plane.fymyriad.com/myriad/",
    );
    expect(planeWorkItemURL(value, "MYRIA-465")).toBe(
      "https://plane.fymyriad.com/myriad/browse/MYRIA-465",
    );
  });

  it("uses the Plane app host for cloud work item addresses", () => {
    const value = settings("https://api.plane.so", "my team");

    expect(planeWorkspaceAddress(value)).toBe(
      "https://app.plane.so/my%20team/",
    );
    expect(planeWorkItemURL(value, "APP-8")).toBe(
      "https://app.plane.so/my%20team/browse/APP-8",
    );
  });
});
