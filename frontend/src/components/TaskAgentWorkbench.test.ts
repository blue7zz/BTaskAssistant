import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentEvent,
  AgentMessage,
  AgentPermissionRequest,
  AgentResource,
  AgentRun,
  AgentSession,
} from "../domain/agent";
import { DEFAULT_PI_SETTINGS } from "../domain/engine";
import type { AgentClient } from "../lib/agentBridge";
import { useWorkspaceStore } from "../store/workspace";
import { TaskAgentWorkbench } from "./TaskAgentWorkbench";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean; })
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

  it("selects only current-task @ resources and sends their ids", async () => {
    const activeSession = session("task_mentions");
    const requirement: AgentResource = {
      id: "resource_requirement",
      taskId: "task_mentions",
      targetType: "resource",
      kind: "context",
      sourceType: "requirements",
      logicalPath: "context/requirements/current.md",
      mimeType: "text/markdown",
      byteSize: 32,
      sha256: "hash",
      immutable: false,
      readable: true,
      createdAt: "2026-08-01T08:00:00Z",
    };
    const sendPrompt = vi.fn().mockResolvedValue(run("task_mentions", activeSession.id));
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      listResources: vi.fn().mockResolvedValue([requirement]),
      listArtifacts: vi.fn().mockResolvedValue([]),
      createSession: vi.fn(),
      sendPrompt,
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: "task_mentions", taskTitle: "引用任务", client,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
    await act(async () => {
      setter?.call(textarea, "请检查 @require");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
    });
    const option = container.querySelector<HTMLButtonElement>('[role="option"]');
    expect(option?.textContent).toContain("current.md");
    await act(async () => option?.click());
    expect(container.textContent).toContain("current.md");
    await act(async () => {
      button(container, "发送").click();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(sendPrompt).toHaveBeenCalledWith({
      taskId: "task_mentions",
      sessionId: activeSession.id,
      message: "请检查 @current.md",
      resourceIds: [requirement.id],
    });
  });

  it("imports pasted files and permits an attachment-only message", async () => {
    const activeSession = session("task_paste");
    const attachment: AgentResource = {
      id: "resource_pasted",
      taskId: "task_paste",
      targetType: "resource",
      kind: "attachment",
      sourceType: "message_attachment",
      logicalPath: "attachments/documents/pasted.md",
      mimeType: "text/markdown",
      byteSize: 6,
      sha256: "hash",
      immutable: true,
      readable: true,
      createdAt: "2026-08-01T08:00:00Z",
    };
    const importAttachments = vi.fn().mockResolvedValue([attachment]);
    const sendPrompt = vi.fn().mockResolvedValue(run("task_paste", activeSession.id));
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      listResources: vi.fn().mockResolvedValue([]),
      listArtifacts: vi.fn().mockResolvedValue([]),
      importAttachments,
      createSession: vi.fn(),
      sendPrompt,
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: "task_paste", taskTitle: "粘贴任务", client,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    const pasted = new File(["pasted"], "pasted.md", { type: "text/markdown" });
    const paste = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(paste, "clipboardData", { value: { files: [pasted] } });
    await act(async () => {
      textarea.dispatchEvent(paste);
    });
    await act(async () => {
      await vi.waitFor(() => {
        expect(importAttachments).toHaveBeenCalledWith({
          taskId: "task_paste",
          files: [expect.objectContaining({ name: "pasted.md", mimeType: "text/markdown" })],
        });
      });
    });
    expect(container.textContent).toContain("pasted.md");
    await act(async () => {
      button(container, "发送").click();
      await Promise.resolve();
    });
    expect(sendPrompt).toHaveBeenCalledWith({
      taskId: "task_paste",
      sessionId: activeSession.id,
      message: "请查看所附任务资源。",
      resourceIds: [attachment.id],
    });
  });

  it("refreshes and previews artifacts when the native gate reports a workspace change", async () => {
    const activeSession = session("task_artifacts");
    const artifact: AgentResource = {
      id: "artifact_report",
      taskId: "task_artifacts",
      targetType: "artifact",
      kind: "report",
      sourceType: "generated_artifact",
      logicalPath: "artifacts/reports/result.md",
      mimeType: "text/markdown",
      byteSize: 20,
      sha256: "hash",
      immutable: false,
      readable: true,
      createdAt: "2026-08-01T08:00:00Z",
    };
    let available = false;
    const listeners = new Set<(value: AgentEvent) => void>();
    const openArtifact = vi.fn().mockResolvedValue(undefined);
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      listResources: vi.fn().mockResolvedValue([]),
      listArtifacts: vi.fn().mockImplementation(async () => available ? [artifact] : []),
      previewResource: vi.fn().mockResolvedValue({
        path: artifact.logicalPath,
        name: "result.md",
        mimeType: "text/markdown",
        byteSize: artifact.byteSize,
        sha256: artifact.sha256,
        kind: "text",
        content: "# Gate result",
        truncated: false,
      }),
      openArtifact,
      createSession: vi.fn(),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: (listener) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    };
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: "task_artifacts", taskTitle: "Artifact 任务", client,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    available = true;
    await act(async () => {
      for (const listener of listeners) {
        listener(event(
          "task_artifacts",
          activeSession.id,
          1,
          "workspace.changed",
          { reason: "artifact", paths: [artifact.logicalPath] },
        ));
      }
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => button(container, "文件 1").click());
    const artifactButton = Array.from(container.querySelectorAll<HTMLButtonElement>("button"))
      .find((candidate) => candidate.textContent?.includes("result.md"));
    await act(async () => {
      artifactButton?.click();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("Gate result");
    await act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="在系统中打开 artifact"]')?.click();
      await Promise.resolve();
    });
    expect(openArtifact).toHaveBeenCalledWith("task_artifacts", artifact.id);
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

  it("loads a 10,000-message fixture by bounded cursor pages", async () => {
    const activeSession = session("task_long_history");
    const fixture: AgentMessage[] = Array.from({ length: 10_000 }, (_, index) => ({
      id: `message_${index + 1}`,
      taskId: activeSession.taskId,
      sessionId: activeSession.id,
      role: "assistant" as const,
      kind: "text" as const,
      status: "complete" as const,
      content: `历史消息 ${index + 1}`,
      sequence: index + 1,
      createdAt: "2026-08-01T08:00:00Z",
    }));
    const listHistoryPage = vi.fn(async (request: {
      cursor: string;
      limit: number;
    }) => {
      const cursor = request.cursor ? Number(request.cursor) : Number.POSITIVE_INFINITY;
      const eligible = fixture.filter((message) => message.sequence < cursor);
      const messages = eligible.slice(-request.limit);
      return {
        messages,
        hasMore: eligible.length > messages.length,
        nextCursor: eligible.length > messages.length
          ? String(messages[0].sequence)
          : undefined,
      };
    });
    const listMessages = vi.fn().mockRejectedValue(new Error("不得读取完整历史"));
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages,
      listHistoryPage,
      listRuns: vi.fn().mockResolvedValue([]),
      createSession: vi.fn(),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };

    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: activeSession.taskId,
        taskTitle: "长历史任务",
        client,
      }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(listMessages).not.toHaveBeenCalled();
    expect(listHistoryPage).toHaveBeenCalledWith({
      taskId: activeSession.taskId,
      sessionId: activeSession.id,
      cursor: "",
      limit: 60,
    });
    expect(container.querySelectorAll(".agent-message")).toHaveLength(60);
    expect(container.textContent).toContain("历史消息 10000");
    expect(Array.from(container.querySelectorAll(".agent-message p")).some(
      (message) => message.textContent === "历史消息 1",
    )).toBe(false);

    await act(async () => {
      button(container, "加载更早消息").click();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(listHistoryPage).toHaveBeenLastCalledWith({
      taskId: activeSession.taskId,
      sessionId: activeSession.id,
      cursor: "9941",
      limit: 60,
    });
    expect(container.querySelectorAll(".agent-message")).toHaveLength(120);
  });

  it("uses distinct active-run actions for Steer and Follow-up", async () => {
    const activeSession = { ...session("task_steer"), state: "running" as const };
    const activeRun = { ...run(activeSession.taskId, activeSession.id), state: "running" };
    const steerPrompt = vi.fn().mockImplementation(async (request) => ({
      id: "message_steer",
      taskId: request.taskId,
      sessionId: request.sessionId,
      runId: activeRun.id,
      role: "user" as const,
      kind: "text" as const,
      status: "pending" as const,
      content: request.message,
      sequence: 10,
      createdAt: "2026-08-01T08:02:00Z",
    }));
    const followUpPrompt = vi.fn().mockImplementation(async (request) => ({
      id: "message_follow_up",
      taskId: request.taskId,
      sessionId: request.sessionId,
      runId: activeRun.id,
      role: "user" as const,
      kind: "text" as const,
      status: "pending" as const,
      content: request.message,
      sequence: 11,
      createdAt: "2026-08-01T08:03:00Z",
    }));
    const sendPrompt = vi.fn();
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([activeRun]),
      createSession: vi.fn(),
      sendPrompt,
      steerPrompt,
      followUpPrompt,
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: activeSession.taskId,
        taskTitle: "运行中任务",
        client,
      }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )?.set;
    await act(async () => {
      setter?.call(textarea, "先处理错误路径");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      button(container, "Steer 引导").click();
      await Promise.resolve();
    });
    await act(async () => {
      setter?.call(textarea, "结束后补充总结");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      button(container, "Follow-up 后续").click();
      await Promise.resolve();
    });

    expect(steerPrompt).toHaveBeenCalledWith({
      taskId: activeSession.taskId,
      sessionId: activeSession.id,
      message: "先处理错误路径",
      resourceIds: [],
    });
    expect(followUpPrompt).toHaveBeenCalledWith({
      taskId: activeSession.taskId,
      sessionId: activeSession.id,
      message: "结束后补充总结",
      resourceIds: [],
    });
    expect(sendPrompt).not.toHaveBeenCalled();
    expect(container.textContent).toContain("已排队");
  });

  it("recovers a failed PI session only through the scoped recovery action", async () => {
    const failedSession: AgentSession = {
      ...session("task_recovery"),
      state: "failed",
      errorMessage: "PI 进程异常退出",
    };
    const resumeSession = vi.fn().mockResolvedValue({
      ...failedSession,
      state: "idle",
      errorMessage: undefined,
    });
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([failedSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      createSession: vi.fn(),
      resumeSession,
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: failedSession.taskId,
        taskTitle: "恢复任务",
        client,
      }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("当前会话需要恢复");
    await act(async () => {
      button(container, "恢复会话").click();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(resumeSession).toHaveBeenCalledWith({
      taskId: failedSession.taskId,
      sessionId: failedSession.id,
    });
    expect(container.textContent).not.toContain("当前会话需要恢复");
  });

  it("drops a late context response after switching task pages", async () => {
    let resolveOldContext: ((value: AgentResource[]) => void) | undefined;
    const oldContext = new Promise<AgentResource[]>((resolve) => {
      resolveOldContext = resolve;
    });
    const resource = (taskId: string, name: string): AgentResource => ({
      id: `resource_${taskId}`,
      taskId,
      targetType: "resource",
      kind: "context",
      sourceType: "task",
      logicalPath: `context/${name}.md`,
      mimeType: "text/markdown",
      byteSize: 10,
      sha256: "hash",
      immutable: false,
      readable: true,
      createdAt: "2026-08-01T08:00:00Z",
    });
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockImplementation(async (taskId) => [session(taskId)]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      listResources: vi.fn()
        .mockReturnValueOnce(oldContext)
        .mockResolvedValueOnce([resource("task_context_new", "new-context")]),
      listArtifacts: vi.fn().mockResolvedValue([]),
      createSession: vi.fn(),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: () => () => undefined,
    };
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: "task_context_old",
        taskTitle: "旧上下文",
        client,
      }));
      await Promise.resolve();
    });
    await act(async () => {
      root.render(createElement(TaskAgentWorkbench, {
        taskId: "task_context_new",
        taskTitle: "新上下文",
        client,
      }));
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      resolveOldContext?.([resource("task_context_old", "old-context")]);
      await Promise.resolve();
    });
    expect(container.textContent).toContain("new-context.md");
    expect(container.textContent).not.toContain("old-context.md");
  });

  it("renders task-scoped permission requests and submits a decision only once", async () => {
    const activeSession = session("task_permission");
    let currentRequest: AgentPermissionRequest = {
      id: "permission_task",
      taskId: "task_permission",
      sessionId: activeSession.id,
      runId: "run_permission",
      toolCallId: "tool_permission",
      toolName: "btask_write_artifact",
      capability: "artifact.write",
      target: "artifacts/plan.md",
      normalizedTarget: "artifacts/plan.md",
      subject: "pi",
      riskLevel: "medium",
      state: "pending",
      requestedAt: "2026-08-01T08:00:00Z",
      expiresAt: "2026-08-01T08:05:00Z",
      allowedScopes: ["once", "task"],
    };
    let listener: ((value: AgentEvent) => void) | undefined;
    let finishResolution: (() => void) | undefined;
    const listPermissionRequests = vi.fn(
      async () => [{ ...currentRequest }],
    );
    const resolvePermission = vi.fn(
      () => new Promise<AgentPermissionRequest>((resolve) => {
        finishResolution = () => {
          currentRequest = {
            ...currentRequest,
            state: "allowed",
            decisionScope: "once",
            allowedScopes: [],
          };
          resolve({ ...currentRequest });
        };
      }),
    );
    const client: AgentClient = {
      runtimeMode: () => "native",
      listSessions: vi.fn().mockResolvedValue([activeSession]),
      listMessages: vi.fn().mockResolvedValue([]),
      listRuns: vi.fn().mockResolvedValue([]),
      listPermissionRequests,
      listPermissionGrants: vi.fn().mockResolvedValue([]),
      listToolCalls: vi.fn().mockResolvedValue([]),
      resolvePermission,
      createSession: vi.fn(),
      sendPrompt: vi.fn(),
      abortRun: vi.fn(),
      subscribe: (candidate) => {
        listener = candidate;
        return () => undefined;
      },
    };

    await act(async () => {
      root.render(
        createElement(TaskAgentWorkbench, {
          taskId: "task_permission",
          taskTitle: "权限任务",
          client,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("btask_write_artifact");
    expect(container.textContent).toContain("应用级软权限边界");

    const loadCount = listPermissionRequests.mock.calls.length;
    await act(async () => {
      listener?.(
        event(
          "task_other",
          "session_other",
          1,
          "permission.requested",
          {},
        ),
      );
      await Promise.resolve();
    });
    expect(listPermissionRequests).toHaveBeenCalledTimes(loadCount);

    act(() => {
      button(container, "仅本次允许").click();
      button(container, "仅本次允许").click();
    });
    expect(resolvePermission).toHaveBeenCalledTimes(1);
    expect(resolvePermission).toHaveBeenCalledWith({
      taskId: "task_permission",
      sessionId: activeSession.id,
      requestId: "permission_task",
      decision: "allow",
      scope: "once",
    });

    await act(async () => {
      finishResolution?.();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain("已允许 · 仅本次允许");
  });
});
