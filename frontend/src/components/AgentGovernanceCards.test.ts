import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentPermissionRequest,
  AgentToolCall,
} from "../domain/agent";
import { AgentPermissionCard } from "./AgentPermissionCard";
import { AgentToolCard } from "./AgentToolCard";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean; })
  .IS_REACT_ACT_ENVIRONMENT = true;

function permission(
  overrides: Partial<AgentPermissionRequest> = {},
): AgentPermissionRequest {
  return {
    id: "permission_1",
    taskId: "task_alpha",
    sessionId: "session_alpha",
    runId: "run_alpha",
    toolCallId: "tool_1",
    toolName: "btask_write_artifact",
    capability: "artifact.write",
    target: "artifacts/plan.md",
    normalizedTarget: "artifacts/plan.md",
    subject: "pi",
    riskLevel: "medium",
    state: "pending",
    requestedAt: "2026-08-01T08:00:00Z",
    expiresAt: "2026-08-01T08:05:00Z",
    allowedScopes: ["once", "session", "task", "permanent"],
    ...overrides,
  };
}

function tool(overrides: Partial<AgentToolCall> = {}): AgentToolCall {
  return {
    id: "tool_1",
    taskId: "task_alpha",
    sessionId: "session_alpha",
    runId: "run_alpha",
    externalToolCallId: "pi_tool_1",
    toolName: "btask_read_resource",
    capability: "resource.read",
    target: "context/task.md",
    riskLevel: "low",
    state: "running",
    argsJson: "{\"path\":\"context/task.md\"}",
    outputSummary: "摘要输出",
    outputRef: "runs/run_alpha/tool-output/tool_1.txt",
    isError: false,
    startedAt: "2026-08-01T08:00:00.000Z",
    ...overrides,
  };
}

function findButton(container: HTMLElement, label: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (candidate) => candidate.textContent?.includes(label),
  )!;
}

describe("Agent governance cards", () => {
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

  it("renders only backend-approved scopes and disables repeated approval", async () => {
    const onResolve = vi.fn();
    await act(async () => {
      root.render(
        createElement(AgentPermissionCard, {
          request: permission(),
          submitting: false,
          onResolve,
        }),
      );
    });

    expect(container.textContent).toContain("仅本次允许");
    expect(container.textContent).toContain("本会话允许");
    expect(container.textContent).toContain("当前任务允许");
    expect(container.textContent).toContain("永久允许");
    await act(async () => findButton(container, "当前任务允许").click());
    expect(onResolve).toHaveBeenCalledWith("allow", "task");

    await act(async () => {
      root.render(
        createElement(AgentPermissionCard, {
          request: permission(),
          submitting: true,
          onResolve,
        }),
      );
    });
    expect(
      Array.from(container.querySelectorAll<HTMLButtonElement>("button")).every(
        (candidate) => candidate.disabled,
      ),
    ).toBe(true);
  });

  it("defensively limits critical approvals to once and renders resolved state read-only", async () => {
    const onResolve = vi.fn();
    await act(async () => {
      root.render(
        createElement(AgentPermissionCard, {
          request: permission({ riskLevel: "critical" }),
          submitting: false,
          onResolve,
        }),
      );
    });
    expect(container.textContent).toContain("仅本次允许");
    expect(container.textContent).not.toContain("本会话允许");
    expect(container.textContent).not.toContain("永久允许");

    await act(async () => {
      root.render(
        createElement(AgentPermissionCard, {
          request: permission({
            state: "allowed",
            decisionScope: "once",
            allowedScopes: [],
          }),
          submitting: false,
          onResolve,
        }),
      );
    });
    expect(container.textContent).toContain("已允许 · 仅本次允许");
    expect(container.querySelector("button")).toBeNull();
  });

  it("shows tool lifecycle state and lazy-loads large output only once", async () => {
    const loadOutput = vi.fn().mockResolvedValue({
      reference: "runs/run_alpha/tool-output/tool_1.txt",
      content: "完整输出内容",
      byteSize: 40960,
      truncated: false,
    });
    await act(async () => {
      root.render(createElement(AgentToolCard, { tool: tool(), loadOutput }));
    });
    expect(container.textContent).toContain("执行中");
    expect(container.textContent).not.toContain("完整输出内容");
    expect(loadOutput).not.toHaveBeenCalled();

    await act(async () => {
      findButton(container, "btask_read_resource").click();
      await Promise.resolve();
    });
    expect(loadOutput).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("完整输出内容");

    await act(async () => findButton(container, "btask_read_resource").click());
    await act(async () => findButton(container, "btask_read_resource").click());
    expect(loadOutput).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        createElement(AgentToolCard, {
          tool: tool({
            state: "succeeded",
            isError: false,
            finishedAt: "2026-08-01T08:00:02.500Z",
          }),
          loadOutput,
        }),
      );
    });
    expect(container.textContent).toContain("已完成");
    expect(container.textContent).toContain("2.5 秒");
  });

  it("stops a running Shell tool without expanding its output", async () => {
    const onStop = vi.fn().mockResolvedValue(undefined);
    const loadOutput = vi.fn();
    await act(async () => {
      root.render(createElement(AgentToolCard, {
        tool: tool({ toolName: "btask_shell", state: "running" }),
        loadOutput,
        onStop,
      }));
    });

    await act(async () => {
      findButton(container, "停止").click();
      await Promise.resolve();
    });
    expect(onStop).toHaveBeenCalledTimes(1);
    expect(loadOutput).not.toHaveBeenCalled();
    expect(
      findButton(container, "btask_shell").getAttribute("aria-expanded"),
    ).toBe("false");
  });
});
