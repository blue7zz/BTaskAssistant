import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentEvent,
  AgentMessage,
  AgentRun,
  AgentSession,
} from "../domain/agent";
import { DEFAULT_PI_SETTINGS } from "../domain/engine";
import type { AgentClient } from "../lib/agentBridge";
import { useWorkspaceStore } from "../store/workspace";
import { TaskAgentWorkbench } from "./TaskAgentWorkbench";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function session(taskId: string, id = `session_${taskId}`): AgentSession {
  return {
    id,
    taskId,
    engine: "pi",
    title: `${taskId} · PI`,
    mode: "ask",
    resourcePolicy: "isolated",
    state: "idle",
    lastSequence: 0,
    createdAt: "2026-08-01T08:00:00Z",
    updatedAt: "2026-08-01T08:00:00Z",
    lastActiveAt: "2026-08-01T08:00:00Z",
  };
}

function run(taskId: string, sessionId: string): AgentRun {
  return {
    id: `run_${taskId}`,
    taskId,
    sessionId,
    mode: "ask",
    state: "queued",
    eventsPath: "runs/run/events.jsonl",
    stdoutPath: "runs/run/stdout.jsonl",
    stderrPath: "runs/run/stderr.log",
    resultPath: "runs/run/result.md",
    startedAt: "2026-08-01T08:01:00Z",
  };
}

function event(
  taskId: string,
  sessionId: string,
  sequence: number,
  kind: string,
  payload: Record<string, unknown>,
  runId = "",
): AgentEvent {
  return {
    version: 1,
    eventId: `event_${taskId}_${sequence}`,
    sequence,
    kind,
    taskId,
    sessionId,
    runId: runId || undefined,
    occurredAt: `2026-08-01T08:01:0${sequence}Z`,
    payload,
  };
}

function button(container: HTMLElement, text: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (candidate) => candidate.textContent?.includes(text),
  )!;
}

describe("TaskAgentWorkbench", () => {
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
    vi.restoreAllMocks();
  });

  it("creates a task-scoped session and shows durable history", async () => {
    const created = session("task_history");
    const history: AgentMessage[] = [
      {
        id: "message_history",
        taskId: "task_history",
        sessionId: created.id,
        role: "assistant",
        kind: "text",
        status: "complete",
        content: "已保存的历史回答",
        sequence: 1,
        createdAt: "2026-08-01T08:00:00Z",
      },
    ];
    const listeners = new Set<(value: AgentEvent) => void>();
    const client: AgentClient = {
      runtimeMode: () => "browser-mock",
      listSessions: vi.fn().mockResolvedValue([]),
      listMessages: vi.fn().mockResolvedValue(history),
      listRuns: vi.fn().mockResolvedValue([]),
      createSession: vi.fn().mockResolvedValue(created),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: (listener) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    };

    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_history",
          taskTitle: "历史任务",
          client,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      button(container, "新建会话").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(client.createSession).toHaveBeenCalledWith({
      taskId: "task_history",
      title: "历史任务 · PI",
      mode: "ask",
      model: "",
      thinkingLevel: "xhigh",
    });
    expect(container.textContent).toContain("已保存的历史回答");
    expect(container.textContent).toContain("浏览器模拟，不启动本机 PI");
  });

  it("renders streaming deltas and aborts the matching active run", async () => {
    const activeSession = session("task_stream");
    const listeners = new Set<(value: AgentEvent) => void>();
    const durableMessages: AgentMessage[] = [];
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi
        .fn()
        .mockImplementation(async () => durableMessages.map((item) => ({ ...item }))),
      listRuns: vi.fn().mockResolvedValue([]),
      createSession: vi.fn(),
      sendPrompt: vi.fn().mockImplementation(async () => {
        const activeRun = run("task_stream", activeSession.id);
        durableMessages.push({
          id: "assistant_stream",
          taskId: "task_stream",
          sessionId: activeSession.id,
          runId: activeRun.id,
          role: "assistant",
          kind: "text",
          status: "streaming",
          content: "正在回答",
          sequence: 1,
          createdAt: "2026-08-01T08:01:00Z",
        });
        for (const listener of listeners) {
          listener(
            event(
              "task_stream",
              activeSession.id,
              1,
              "message.start",
              { messageId: "assistant_stream", role: "assistant", kind: "text" },
              activeRun.id,
            ),
          );
          listener(
            event(
              "task_stream",
              activeSession.id,
              2,
              "message.delta",
              { messageId: "assistant_stream", delta: "正在回答" },
              activeRun.id,
            ),
          );
          listener(
            event(
              "task_stream",
              activeSession.id,
              3,
              "run.state",
              { state: "running" },
              activeRun.id,
            ),
          );
        }
        return activeRun;
      }),
      abortRun: vi.fn().mockImplementation(async (request) => {
        for (const listener of listeners) {
          listener(
            event(
              request.taskId,
              request.sessionId,
              4,
              "run.state",
              { state: "cancelled", reason: "用户已停止本次回答" },
              request.runId,
            ),
          );
        }
      }),
      subscribe: (listener) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    };

    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_stream",
          taskTitle: "流式任务",
          client,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )?.set;
    await act(async () => {
      setter?.call(textarea, "开始回答");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      button(container, "发送").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain("正在回答");
    expect(container.textContent).toContain("停止");
    await act(async () => {
      button(container, "停止").click();
      await Promise.resolve();
    });
    expect(client.abortRun).toHaveBeenCalledWith({
      taskId: "task_stream",
      sessionId: activeSession.id,
      runId: "run_task_stream",
    });
    expect(button(container, "发送").disabled).toBe(true);
  });

  it("shows native PI startup failures without inventing a session", async () => {
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([]),
      listMessages: vi.fn(),
      listRuns: vi.fn().mockResolvedValue([]),
      createSession: vi
        .fn()
        .mockRejectedValue(new Error("未找到原生 PI CLI 可执行文件")),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };

    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_missing",
          taskTitle: "缺少 PI",
          client,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      button(container, "新建会话").click();
      await Promise.resolve();
    });

    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      "未找到原生 PI CLI 可执行文件",
    );
    expect(container.textContent).toContain("还没有 PI 会话");
  });

  it("recovers the active run after returning to a task", async () => {
    const activeSession = session("task_return");
    const activeRun = {
      ...run("task_return", activeSession.id),
      state: "running",
    };
    const abortRun = vi.fn().mockResolvedValue(undefined);
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([activeRun]),
      createSession: vi.fn(),
      sendPrompt: vi.fn(),
      abortRun,
      subscribe: () => () => undefined,
    };

    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_return",
          taskTitle: "返回任务",
          client,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain("停止");
    await act(async () => {
      button(container, "停止").click();
      await Promise.resolve();
    });
    expect(abortRun).toHaveBeenCalledWith({
      taskId: "task_return",
      sessionId: activeSession.id,
      runId: activeRun.id,
    });
  });

  it("unsubscribes and ignores a late session load after task switching", async () => {
    let resolveFirst: ((value: AgentSession[]) => void) | undefined;
    const firstLoad = new Promise<AgentSession[]>((resolve) => {
      resolveFirst = resolve;
    });
    const unsubscribe = vi.fn();
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi
        .fn()
        .mockReturnValueOnce(firstLoad)
        .mockResolvedValueOnce([session("task_new")]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      createSession: vi.fn(),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: () => unsubscribe,
    };

    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_old",
          taskTitle: "旧任务",
          client,
        }),
      );
      await Promise.resolve();
    });
    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_new",
          taskTitle: "新任务",
          client,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      resolveFirst?.([session("task_old")]);
      await Promise.resolve();
    });

    expect(unsubscribe).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain("task_new · PI");
    expect(container.textContent).not.toContain("task_old · PI");
  });
});
