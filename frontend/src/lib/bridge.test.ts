import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ensureTaskWorkspace,
  listTaskWorkspaceFiles,
  readTaskWorkspaceFile,
} from "./bridge";

describe("Task Workspace bridge", () => {
  afterEach(() => {
    delete window.go;
  });

  it("delegates scoped workspace operations to the Wails backend", async () => {
    const ensure = vi.fn().mockResolvedValue({
      taskId: "task_bridge",
      workspaceId: "workspace-bridge",
      rootPath: "/tasks/task_bridge",
      schemaVersion: 1,
      manifestRevision: 1,
      state: "ready",
      createdAt: "2026-08-01T08:00:00Z",
      updatedAt: "2026-08-01T08:00:00Z",
    });
    const list = vi.fn().mockResolvedValue([]);
    const read = vi.fn().mockResolvedValue({
      path: "context/task.md",
      name: "task.md",
      mimeType: "text/markdown",
      byteSize: 4,
      sha256: "hash",
      kind: "text",
      content: "task",
      truncated: false,
    });
    window.go = {
      main: {
        App: {
          EnsureTaskWorkspace: ensure,
          ListTaskWorkspaceFiles: list,
          ReadTaskWorkspaceFile: read,
        },
      },
    } as unknown as typeof window.go;

    await expect(ensureTaskWorkspace("task_bridge")).resolves.toMatchObject({
      state: "ready",
    });
    await expect(
      listTaskWorkspaceFiles("task_bridge", "context"),
    ).resolves.toEqual([]);
    await expect(
      readTaskWorkspaceFile("task_bridge", "context/task.md"),
    ).resolves.toMatchObject({ content: "task" });
    expect(ensure).toHaveBeenCalledWith("task_bridge");
    expect(list).toHaveBeenCalledWith("task_bridge", "context");
    expect(read).toHaveBeenCalledWith("task_bridge", "context/task.md");
  });

  it("does not simulate physical workspace files in browser preview", async () => {
    window.go = undefined;
    await expect(ensureTaskWorkspace("task_browser")).rejects.toThrow(
      "任务工作区文件只能在 Wails 桌面客户端中使用",
    );
  });
});
