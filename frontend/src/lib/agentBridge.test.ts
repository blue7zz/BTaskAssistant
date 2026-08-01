import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentEvent } from "../domain/agent";
import {
  agentClient,
  parseAgentEvent,
  resetBrowserAgentMockForTests,
} from "./agentBridge";

const VALID_EVENT: AgentEvent = {
  version: 1,
  eventId: "event_1",
  sequence: 1,
  kind: "message.delta",
  taskId: "task_a",
  sessionId: "session_a",
  runId: "run_a",
  occurredAt: "2026-08-01T00:00:00Z",
  payload: { messageId: "message_a", delta: "内容" },
};

describe("PI agent bridge", () => {
  beforeEach(() => {
    delete window.go;
    delete window.runtime;
    resetBrowserAgentMockForTests();
  });

  afterEach(() => {
    vi.useRealTimers();
    resetBrowserAgentMockForTests();
    delete window.go;
    delete window.runtime;
    vi.restoreAllMocks();
  });

  it("accepts only complete v1 event envelopes", () => {
    expect(parseAgentEvent(VALID_EVENT)).toEqual(VALID_EVENT);
    expect(parseAgentEvent({ ...VALID_EVENT, version: 2 })).toBeUndefined();
    expect(parseAgentEvent({ ...VALID_EVENT, sequence: 1.5 })).toBeUndefined();
    expect(parseAgentEvent({ ...VALID_EVENT, payload: [] })).toBeUndefined();
    expect(parseAgentEvent({ ...VALID_EVENT, taskId: "" })).toBeUndefined();
  });

  it("keeps deterministic browser sessions and messages isolated by task", async () => {
    vi.useFakeTimers();
    const events: AgentEvent[] = [];
    const unsubscribe = agentClient.subscribe((event) => events.push(event));
    const first = await agentClient.createSession({
      taskId: "task_a",
      title: "任务 A · PI",
      mode: "ask",
      model: "",
      thinkingLevel: "xhigh",
    });
    const second = await agentClient.createSession({
      taskId: "task_b",
      title: "任务 B · PI",
      mode: "ask",
      model: "",
      thinkingLevel: "xhigh",
    });

    await agentClient.sendPrompt({
      taskId: "task_a",
      sessionId: first.id,
      message: "第一行\u2028第二行",
      resourceIds: [],
    });
    await vi.runAllTimersAsync();

    const firstMessages = await agentClient.listMessages("task_a", first.id);
    const secondMessages = await agentClient.listMessages("task_b", second.id);
    expect(firstMessages).toHaveLength(2);
    expect(firstMessages[0].content).toBe("第一行\u2028第二行");
    expect(firstMessages[1]).toMatchObject({
      role: "assistant",
      status: "complete",
      content: "浏览器模拟回复：第一行\u2028第二行",
    });
    expect(secondMessages).toEqual([]);
    expect(events.some((event) => event.kind === "agent.settled")).toBe(true);
    expect(events.every((event) => event.version === 1)).toBe(true);
    unsubscribe();
  });

  it("aborts only the matching browser run", async () => {
    vi.useFakeTimers();
    const session = await agentClient.createSession({
      taskId: "task_abort",
      title: "中止测试",
      mode: "ask",
      model: "",
      thinkingLevel: "medium",
    });
    const run = await agentClient.sendPrompt({
      taskId: "task_abort",
      sessionId: session.id,
      message: "请停止",
      resourceIds: [],
    });

    await expect(
      agentClient.abortRun({
        taskId: "task_other",
        sessionId: session.id,
        runId: run.id,
      }),
    ).rejects.toThrow("没有匹配的活动 PI 运行");
    await agentClient.abortRun({
      taskId: "task_abort",
      sessionId: session.id,
      runId: run.id,
    });
    await vi.runAllTimersAsync();

    const messages = await agentClient.listMessages(
      "task_abort",
      session.id,
    );
    expect(messages.at(-1)?.status).toBe("cancelled");
  });

  it("pages browser history and keeps Steer separate from Follow-up", async () => {
    vi.useFakeTimers();
    const events: AgentEvent[] = [];
    const unsubscribe = agentClient.subscribe((event) => events.push(event));
    const session = await agentClient.createSession({
      taskId: "task_queue",
      title: "队列测试",
      mode: "agent",
      model: "",
      thinkingLevel: "high",
    });
    await agentClient.sendPrompt({
      taskId: session.taskId,
      sessionId: session.id,
      message: "先开始",
      resourceIds: [],
    });
    const steering = await agentClient.steerPrompt?.({
      taskId: session.taskId,
      sessionId: session.id,
      message: "先检查错误",
      resourceIds: [],
    });
    const followUp = await agentClient.followUpPrompt?.({
      taskId: session.taskId,
      sessionId: session.id,
      message: "最后总结",
      resourceIds: [],
    });
    expect(steering?.status).toBe("pending");
    expect(followUp?.status).toBe("pending");

    const latest = await agentClient.listHistoryPage?.({
      taskId: session.taskId,
      sessionId: session.id,
      cursor: "",
      limit: 2,
    });
    expect(latest).toMatchObject({
      hasMore: true,
      messages: [
        { content: "先检查错误" },
        { content: "最后总结" },
      ],
    });

    await vi.runAllTimersAsync();
    const messages = await agentClient.listMessages(session.taskId, session.id);
    expect(messages.some((message) => message.content === "浏览器模拟引导回复：先检查错误")).toBe(true);
    expect(messages.some((message) => message.content === "浏览器模拟后续回复：最后总结")).toBe(true);
    expect(events.some((event) =>
      event.kind === "queue.updated" && event.payload.steeringCount === 1)).toBe(true);
    expect(events.some((event) =>
      event.kind === "queue.updated" && event.payload.followUpCount === 1)).toBe(true);
    unsubscribe();
  });

  it("keeps browser attachments and message references task scoped", async () => {
    vi.useFakeTimers();
    const sessionA = await agentClient.createSession({
      taskId: "task_resource_a",
      title: "A",
      mode: "ask",
      model: "",
      thinkingLevel: "medium",
    });
    await agentClient.createSession({
      taskId: "task_resource_b",
      title: "B",
      mode: "ask",
      model: "",
      thinkingLevel: "medium",
    });
    const importedA = await agentClient.importAttachments?.({
      taskId: "task_resource_a",
      files: [{
        name: "same.md",
        mimeType: "text/markdown",
        dataBase64: window.btoa("task A"),
      }],
    });
    const importedB = await agentClient.importAttachments?.({
      taskId: "task_resource_b",
      files: [{
        name: "same.md",
        mimeType: "text/markdown",
        dataBase64: window.btoa("task B"),
      }],
    });
    expect(importedA).toHaveLength(1);
    expect(importedB).toHaveLength(1);
    expect(importedA?.[0].id).not.toBe(importedB?.[0].id);

    await agentClient.sendPrompt({
      taskId: "task_resource_a",
      sessionId: sessionA.id,
      message: "",
      resourceIds: [importedA![0].id],
    });
    await vi.runAllTimersAsync();
    let messages = await agentClient.listMessages("task_resource_a", sessionA.id);
    expect(messages[0].content).toBe("请查看所附的当前任务资源。");
    expect(messages[0].references).toMatchObject([{
      resourceId: importedA![0].id,
      method: "attachment",
      logicalPath: expect.stringContaining("same.md"),
    }]);
    await expect(
      agentClient.previewResource?.({
        taskId: "task_resource_b",
        resourceId: importedA![0].id,
      }),
    ).rejects.toThrow("当前任务资源不存在");
    await agentClient.removeReference?.({
      taskId: "task_resource_a",
      sessionId: sessionA.id,
      messageId: messages[0].id,
      resourceId: importedA![0].id,
    });
    messages = await agentClient.listMessages("task_resource_a", sessionA.id);
    expect(messages[0].references).toEqual([]);
    await expect(
      agentClient.previewResource?.({
        taskId: "task_resource_a",
        resourceId: importedA![0].id,
      }),
    ).resolves.toMatchObject({ content: "task A", kind: "text" });
  });

  it("delegates to Wails and drops malformed runtime events", async () => {
    const listSessions = vi.fn().mockResolvedValue([]);
    let runtimeListener: ((payload: unknown) => void) | undefined;
    const runtimeUnsubscribe = vi.fn();
    window.go = {
      main: {
        App: {
          ListAgentSessions: listSessions,
          ListAgentMessages: vi.fn().mockResolvedValue([]),
          GetAgentHistoryPage: vi.fn().mockResolvedValue({
            messages: [],
            hasMore: false,
          }),
          ListExecutionRuns: vi.fn().mockResolvedValue([]),
          ListAgentToolCalls: vi.fn().mockResolvedValue([]),
          ListAgentPermissionRequests: vi.fn().mockResolvedValue([]),
          ListAgentPermissionGrants: vi.fn().mockResolvedValue([]),
          ResolveAgentPermission: vi.fn(),
          RevokeAgentPermissionGrant: vi.fn(),
          ReadAgentToolOutput: vi.fn(),
          CreateAgentSession: vi.fn(),
          ResumeAgentSession: vi.fn(),
          SendAgentPrompt: vi.fn(),
          SteerAgent: vi.fn(),
          FollowUpAgent: vi.fn(),
          AbortAgentRun: vi.fn(),
          ListAgentResources: vi.fn().mockResolvedValue([]),
          ListAgentArtifacts: vi.fn().mockResolvedValue([]),
          ImportAgentAttachments: vi.fn().mockResolvedValue([]),
          PreviewAgentResource: vi.fn(),
          RemoveAgentMessageReference: vi.fn(),
          OpenAgentArtifact: vi.fn(),
        },
      },
    } as unknown as typeof window.go;
    window.runtime = {
      EventsOn: vi.fn((_name, listener) => {
        runtimeListener = listener;
        return runtimeUnsubscribe;
      }),
    };
    const listener = vi.fn();
    const unsubscribe = agentClient.subscribe(listener);

    await agentClient.listSessions("task_native");
    runtimeListener?.({ ...VALID_EVENT, version: 3 });
    runtimeListener?.(VALID_EVENT);
    unsubscribe();

    expect(agentClient.runtimeMode()).toBe("native");
    expect(listSessions).toHaveBeenCalledWith("task_native");
    expect(window.runtime.EventsOn).toHaveBeenCalledWith(
      "agent:event",
      expect.any(Function),
    );
    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener).toHaveBeenCalledWith(VALID_EVENT);
    expect(runtimeUnsubscribe).toHaveBeenCalledTimes(1);
  });
});
