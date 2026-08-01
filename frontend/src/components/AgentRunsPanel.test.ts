import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentRun } from "../domain/agent";
import type { AgentClient } from "../lib/agentBridge";
import { AgentRunsPanel } from "./AgentRunsPanel";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function run(taskId: string, sessionId: string): AgentRun {
  return {
    id: `run_${taskId}`,
    taskId,
    sessionId,
    gitBindingId: `binding_${taskId}`,
    baselineCommit: "0123456789abcdef",
    mode: "agent",
    state: "succeeded",
    eventsPath: `runs/run_${taskId}/events.jsonl`,
    stdoutPath: `runs/run_${taskId}/stdout.jsonl`,
    stderrPath: `runs/run_${taskId}/stderr.log`,
    resultPath: `runs/run_${taskId}/result.md`,
    resultSummary: `result for ${taskId}`,
    startedAt: "2026-08-01T08:00:00.000Z",
    finishedAt: "2026-08-01T08:00:02.500Z",
  };
}

describe("AgentRunsPanel", () => {
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

  it("shows durable run state, baseline, duration, and result", async () => {
    const listRuns = vi.fn().mockResolvedValue([run("task_runs", "session_runs")]);
    const client = { listRuns } as unknown as AgentClient;
    await act(async () => {
      root.render(createElement(AgentRunsPanel, {
        taskId: "task_runs",
        sessionId: "session_runs",
        client,
        refreshVersion: 0,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(listRuns).toHaveBeenCalledWith("task_runs", "session_runs");
    expect(container.textContent).toContain("run_task_runs");
    expect(container.textContent).toContain("0123456789");
    expect(container.textContent).toContain("2.5 秒");
    expect(container.textContent).toContain("result for task_runs");
  });

  it("ignores a late run list after switching tasks", async () => {
    let resolveAlpha: ((value: AgentRun[]) => void) | undefined;
    const alpha = new Promise<AgentRun[]>((resolve) => {
      resolveAlpha = resolve;
    });
    const listRuns = vi.fn().mockImplementation((taskId: string, sessionId: string) => (
      taskId === "task_alpha"
        ? alpha
        : Promise.resolve([run("task_beta", sessionId)])
    ));
    const client = { listRuns } as unknown as AgentClient;

    await act(async () => {
      root.render(createElement(AgentRunsPanel, {
        taskId: "task_alpha",
        sessionId: "session_alpha",
        client,
        refreshVersion: 0,
      }));
    });
    await act(async () => {
      root.render(createElement(AgentRunsPanel, {
        taskId: "task_beta",
        sessionId: "session_beta",
        client,
        refreshVersion: 0,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("run_task_beta");

    await act(async () => {
      resolveAlpha?.([run("task_alpha", "session_alpha")]);
      await Promise.resolve();
    });
    expect(container.textContent).toContain("run_task_beta");
    expect(container.textContent).not.toContain("run_task_alpha");
  });
});
