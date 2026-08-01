import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  GitBinding,
  TaskFileDiff,
  TaskGitStatus,
} from "../domain/agent";
import type { AgentClient } from "../lib/agentBridge";
import { AgentChangesPanel } from "./AgentChangesPanel";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function binding(taskId: string): GitBinding {
  return {
    id: `binding_${taskId}`,
    taskId,
    sourcePath: `/source/${taskId}`,
    sourceRealPath: `/source/${taskId}`,
    commonGitDir: `/source/${taskId}/.git`,
    worktreePath: `/tasks/${taskId}/repos/source`,
    branch: `btask/${taskId}-abc123`,
    baselineCommit: "0123456789abcdef",
    sourceBranch: "main",
    sourceDirtyAtBind: false,
    state: "ready",
    createdAt: "2026-08-01T08:00:00Z",
    updatedAt: "2026-08-01T08:00:00Z",
  };
}

function gitStatus(taskId: string): TaskGitStatus {
  return {
    bound: true,
    binding: binding(taskId),
    remoteUrl: "https://example.invalid/team/repo.git",
    head: "0123456789abcdef",
    snapshot: `snapshot_${taskId}`,
    files: [
      {
        path: "README.md",
        status: "modified",
        indexStatus: " ",
        worktreeStatus: "M",
        staged: false,
        unstaged: true,
      },
    ],
    aheadOfBaseline: 0,
    behindBaseline: 0,
  };
}

function fileDiff(taskId: string): TaskFileDiff {
  return {
    taskId,
    path: "README.md",
    status: "modified",
    staged: "",
    unstaged: "diff --git a/README.md b/README.md\n+task change\n",
    added: 1,
    removed: 0,
    binary: false,
    truncated: false,
    byteSize: 55,
  };
}

function button(container: HTMLElement, label: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (candidate) => candidate.textContent?.includes(label),
  )!;
}

function setInputValue(input: HTMLInputElement, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(
    input,
    value,
  );
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("AgentChangesPanel", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("shows task Git state, loads its diff, and requires commit confirmation", async () => {
    const initial = gitStatus("task_alpha");
    const committedStatus = { ...initial, head: "fedcba9876543210", files: [] };
    const getTaskFileDiff = vi.fn().mockResolvedValue(fileDiff("task_alpha"));
    const commitTaskGitChanges = vi.fn().mockResolvedValue({
      commit: committedStatus.head,
      status: committedStatus,
    });
    const client = {
      getTaskGitStatus: vi.fn().mockResolvedValue(initial),
      getTaskFileDiff,
      commitTaskGitChanges,
    } as unknown as AgentClient;

    await act(async () => {
      root.render(createElement(AgentChangesPanel, {
        taskId: "task_alpha",
        taskStatus: "development",
        client,
        refreshVersion: 0,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain("btask/task_alpha-abc123");
    expect(container.textContent).toContain("https://example.invalid/team/repo.git");
    expect(button(container, "安全清理 worktree").disabled).toBe(true);

    await act(async () => {
      button(container, "README.md").click();
      await Promise.resolve();
    });
    expect(getTaskFileDiff).toHaveBeenCalledWith("task_alpha", "README.md");
    expect(container.textContent).toContain("task change");

    await act(async () => {
      setInputValue(
        container.querySelector<HTMLInputElement>("input")!,
        "fix(Git工作区): 验证本地提交",
      );
    });
    await act(async () => button(container, "预览本地 commit").click());
    expect(commitTaskGitChanges).not.toHaveBeenCalled();
    expect(container.textContent).toContain("不会推送远端");

    await act(async () => {
      button(container, "确认创建本地 commit").click();
      await Promise.resolve();
    });
    expect(commitTaskGitChanges).toHaveBeenCalledWith({
      taskId: "task_alpha",
      message: "fix(Git工作区): 验证本地提交",
      expectedSnapshot: "snapshot_task_alpha",
      confirmed: true,
    });
    expect(container.textContent).toContain("任务 worktree 当前干净");
  });

  it("discards a late Git response after switching tasks", async () => {
    let resolveAlpha: ((value: TaskGitStatus) => void) | undefined;
    const alpha = new Promise<TaskGitStatus>((resolve) => {
      resolveAlpha = resolve;
    });
    const getTaskGitStatus = vi.fn().mockImplementation((taskId: string) => (
      taskId === "task_alpha" ? alpha : Promise.resolve(gitStatus("task_beta"))
    ));
    const client = { getTaskGitStatus } as unknown as AgentClient;

    await act(async () => {
      root.render(createElement(AgentChangesPanel, {
        taskId: "task_alpha",
        taskStatus: "development",
        client,
        refreshVersion: 0,
      }));
    });
    await act(async () => {
      root.render(createElement(AgentChangesPanel, {
        taskId: "task_beta",
        taskStatus: "development",
        client,
        refreshVersion: 0,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("/source/task_beta");

    await act(async () => {
      resolveAlpha?.(gitStatus("task_alpha"));
      await Promise.resolve();
    });
    expect(container.textContent).toContain("/source/task_beta");
    expect(container.textContent).not.toContain("/source/task_alpha");
  });

  it("keeps the latest selected file when an earlier diff resolves late", async () => {
    const status = gitStatus("task_diff_race");
    status.files.push({
      path: "LATEST.md",
      status: "added",
      indexStatus: "?",
      worktreeStatus: "?",
      staged: false,
      unstaged: true,
    });
    let resolveReadme: ((value: TaskFileDiff) => void) | undefined;
    const readme = new Promise<TaskFileDiff>((resolve) => {
      resolveReadme = resolve;
    });
    const latest = {
      ...fileDiff("task_diff_race"),
      path: "LATEST.md",
      unstaged: "+latest change",
    };
    const getTaskFileDiff = vi.fn().mockImplementation(
      (_taskId: string, path: string) => (
        path === "README.md" ? readme : Promise.resolve(latest)
      ),
    );
    const client = {
      getTaskGitStatus: vi.fn().mockResolvedValue(status),
      getTaskFileDiff,
    } as unknown as AgentClient;

    await act(async () => {
      root.render(createElement(AgentChangesPanel, {
        taskId: "task_diff_race",
        taskStatus: "development",
        client,
        refreshVersion: 0,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => button(container, "README.md").click());
    await act(async () => {
      button(container, "LATEST.md").click();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("latest change");

    await act(async () => {
      resolveReadme?.(fileDiff("task_diff_race"));
      await Promise.resolve();
    });
    expect(container.textContent).toContain("latest change");
    expect(container.textContent).not.toContain("task change");
  });

  it("binds an unbound task only after a repository is selected", async () => {
    const ready = gitStatus("task_bind");
    const getTaskGitStatus = vi
      .fn()
      .mockResolvedValueOnce({
        bound: false,
        binding: {} as GitBinding,
        files: [],
        aheadOfBaseline: 0,
        behindBaseline: 0,
      })
      .mockResolvedValueOnce(ready);
    const selectGitRepository = vi.fn().mockResolvedValue("/source/task_bind");
    const bindGitRepository = vi.fn().mockResolvedValue(ready.binding);
    const client = {
      getTaskGitStatus,
      selectGitRepository,
      bindGitRepository,
    } as unknown as AgentClient;

    await act(async () => {
      root.render(createElement(AgentChangesPanel, {
        taskId: "task_bind",
        taskStatus: "development",
        client,
        refreshVersion: 0,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("尚未绑定 Git 仓库");

    await act(async () => {
      button(container, "选择并绑定").click();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(bindGitRepository).toHaveBeenCalledWith({
      taskId: "task_bind",
      sourcePath: "/source/task_bind",
    });
    expect(container.textContent).toContain("btask/task_bind-abc123");
  });
});
