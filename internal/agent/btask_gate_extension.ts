import { Type } from "@earendil-works/pi-ai";
import { defineTool, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createHash } from "node:crypto";

const config = __BTASK_GATE_CONFIG__;

type BridgeResponse = {
  version: string;
  nonce: string;
  ok: boolean;
  data?: unknown;
  error?: string;
};

type RunResponse = {
  version: string;
  nonce: string;
  runId: string;
};

type PermissionDescription = {
  capability: string;
  subject: string;
  target: string;
  normalizedTarget: string;
  riskLevel: "low" | "medium" | "high";
};

let activeRunId = "";

function stable(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
  if (value && typeof value === "object") {
    const record = value as Record<string, unknown>;
    return `{${Object.keys(record)
      .sort()
      .map((key) => `${stable(key)}:${stable(record[key])}`)
      .join(",")}}`;
  }
  return JSON.stringify(value)
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
}

function argsDigest(args: Record<string, unknown>): string {
  return createHash("sha256").update(stable(args)).digest("hex");
}

function normalizedTarget(
  rootKind: string,
  relativePath: string,
  operation: string,
): string {
  return JSON.stringify({
    rootKind,
    rootId: config.taskId,
    relativePath,
    operation,
  });
}

function artifactPath(params: Record<string, unknown>): string {
  const directories: Record<string, string> = {
    plan: "plans",
    report: "reports",
    proposal: "proposals",
    export: "exports",
  };
  const kind = String(params.kind || "").trim().toLowerCase();
  let name = String(params.name || "").trim();
  if (!name.includes(".")) name += ".md";
  return `artifacts/${directories[kind] || "invalid"}/${name}`;
}

function describePermission(
  toolName: string,
  params: Record<string, unknown>,
): PermissionDescription | undefined {
  if (toolName === "btask_list_resources") {
    return {
      capability: "task.resource.list",
      subject: "列出当前任务资源",
      target: "当前任务资源索引",
      normalizedTarget: normalizedTarget("task-resources", ".", "read"),
      riskLevel: "low",
    };
  }
  if (toolName === "btask_read_resource") {
    const resourceId = String(params.resourceId || "").trim();
    return {
      capability: "task.resource.read",
      subject: "读取当前任务资源",
      target: resourceId,
      normalizedTarget: normalizedTarget("task-resources", resourceId, "read"),
      riskLevel: "low",
    };
  }
  if (toolName === "btask_write_artifact") {
    const target = artifactPath(params);
    return {
      capability: "task.artifact.write",
      subject: "写入当前任务 artifact",
      target,
      normalizedTarget: normalizedTarget(
        "task-artifacts",
        target.replace(/^artifacts\//, ""),
        "modify",
      ),
      riskLevel: "medium",
    };
  }
  if (toolName === "btask_permission_probe") {
    const target = String(params.target || "").trim();
    return {
      capability: "diagnostic.permission.probe",
      subject: "验证 BTask 权限审批链路",
      target,
      normalizedTarget: normalizedTarget("permission-self-test", target, "read"),
      riskLevel: "high",
    };
  }
  return undefined;
}

async function bindActiveRun(ctx: any): Promise<string> {
  const request = JSON.stringify({
    version: config.version,
    nonce: config.nonce,
    taskId: config.taskId,
    sessionId: config.sessionId,
    mode: config.mode,
  });
  const value = await ctx.ui.input("btask-run", request);
  if (!value) throw new Error("BTask gate did not bind an active run");
  const response = JSON.parse(value) as RunResponse;
  if (
    response.version !== config.version ||
    response.nonce !== config.nonce ||
    !response.runId
  ) {
    throw new Error("BTask active run identity mismatch");
  }
  activeRunId = response.runId;
  return activeRunId;
}

async function bridge(
  ctx: any,
  toolCallId: string,
  operation: string,
  args: Record<string, unknown>,
): Promise<unknown> {
  const request = JSON.stringify({
    version: config.version,
    nonce: config.nonce,
    taskId: config.taskId,
    sessionId: config.sessionId,
    runId: activeRunId,
    mode: config.mode,
    toolCallId,
    operation,
    args,
  });
  const value = await ctx.ui.input("btask-gate", request);
  if (!value) throw new Error("BTask gate did not return a response");
  const response = JSON.parse(value) as BridgeResponse;
  if (response.version !== config.version || response.nonce !== config.nonce) {
    throw new Error("BTask gate response identity mismatch");
  }
  if (!response.ok) throw new Error(response.error || "BTask gate rejected the operation");
  return response.data;
}

export default function (pi: ExtensionAPI) {
  pi.on("session_start", (_event, ctx) => {
    ctx.ui.setStatus("btask-gate", `${config.version}:${config.nonce}`);
  });

  pi.on("before_agent_start", async (event, ctx) => {
    await bindActiveRun(ctx);
    return { systemPrompt:
      event.systemPrompt +
      `\n\nBTask task scope: ${config.taskId}. Use only btask_list_resources and ` +
      "btask_read_resource for task context and references. Never use shell, Git, or arbitrary " +
      "filesystem access. context/ and sources/ are immutable. Requirement changes must be " +
      "written as proposal artifacts. Do not restate the entire session history in prompts." };
  });

  pi.on("tool_call", async (event, ctx) => {
    const description = describePermission(
      event.toolName,
      event.input as Record<string, unknown>,
    );
    if (!description || !activeRunId) {
      return { block: true, reason: "BTask gate cannot normalize this tool call" };
    }
    const envelope = JSON.stringify({
      protocol: config.permissionProtocol,
      version: config.version,
      nonce: config.nonce,
      taskId: config.taskId,
      sessionId: config.sessionId,
      runId: activeRunId,
      mode: config.mode,
      toolCallId: event.toolCallId,
      toolName: event.toolName,
      ...description,
      argsDigest: argsDigest(event.input as Record<string, unknown>),
    });
    const confirmed = await ctx.ui.confirm("btask-permission", envelope, {
      timeout: 300000,
      signal: ctx.signal,
    });
    if (!confirmed) {
      return { block: true, reason: "BTask permission denied or expired" };
    }
  });

  if (config.selfTest) {
    pi.registerCommand("btask-gate-self-test", {
      description: "Verify the PI RPC confirm request/response path without a model",
      handler: async (_args, ctx) => {
        const digestFixture = {
          text: "line\u2028next\u2029end",
          nested: { b: 2, a: 1 },
        };
        const confirmed = await ctx.ui.confirm(
          "btask-permission-self-test",
          JSON.stringify({
            protocol: config.permissionProtocol,
            version: config.version,
            nonce: config.nonce,
            argsDigest: argsDigest(digestFixture),
          }),
          { timeout: 10000 },
        );
        if (!confirmed) throw new Error("BTask permission self-test was denied");
        ctx.ui.notify("BTask permission self-test passed", "info");
      },
    });

    pi.registerTool(
      defineTool({
        name: "btask_permission_probe",
        label: "Permission probe",
        description: "Test-only permission protocol probe.",
        parameters: Type.Object(
          { target: Type.String({ minLength: 1, maxLength: 200 }) },
          { additionalProperties: false },
        ),
        async execute(toolCallId, params, _signal, _onUpdate, ctx) {
          const data = await bridge(ctx, toolCallId, "permission_probe", params);
          return { content: [{ type: "text", text: JSON.stringify(data) }], details: data };
        },
      }),
    );
  }

  pi.registerTool(
    defineTool({
      name: "btask_list_resources",
      label: "List task resources",
      description: "Search resources that belong only to the current BTask task.",
      promptSnippet: "Search current-task context, sources, attachments, and artifacts",
      promptGuidelines: [
        "Use btask_list_resources before reading task resources when the exact resource id is unknown.",
      ],
      parameters: Type.Object(
        {
          query: Type.Optional(Type.String({ maxLength: 200 })),
          limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 100 })),
        },
        { additionalProperties: false },
      ),
      async execute(toolCallId, params, _signal, _onUpdate, ctx) {
        const data = await bridge(ctx, toolCallId, "list_resources", params);
        return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
      },
    }),
  );

  pi.registerTool(
    defineTool({
      name: "btask_read_resource",
      label: "Read task resource",
      description: "Read one resource selected from the current BTask task resource index.",
      promptSnippet: "Read one explicitly indexed current-task resource",
      promptGuidelines: [
        "Use btask_read_resource only with ids returned by btask_list_resources or attached to the user message.",
      ],
      parameters: Type.Object(
        { resourceId: Type.String({ minLength: 1, maxLength: 200 }) },
        { additionalProperties: false },
      ),
      async execute(toolCallId, params, _signal, _onUpdate, ctx) {
        const data = await bridge(ctx, toolCallId, "read_resource", params);
        const image = (data as any)?.image;
        const display = image
          ? { ...(data as any), image: { mimeType: image.mimeType, embedded: true } }
          : data;
        const content: any[] = [{ type: "text", text: JSON.stringify(display, null, 2) }];
        if (image?.data && image?.mimeType) {
          content.push({ type: "image", data: image.data, mimeType: image.mimeType });
        }
        return { content, details: data };
      },
    }),
  );

  if (config.mode !== "ask") {
    pi.registerTool(
      defineTool({
        name: "btask_write_artifact",
        label: "Write task artifact",
        description: "Create or update a UTF-8 document in a controlled current-task artifacts directory.",
        promptSnippet: "Write plans, reports, proposals, or exports under current-task artifacts",
        promptGuidelines: [
          "Use btask_write_artifact for generated documents; use kind proposal for any requirement change suggestion.",
        ],
        parameters: Type.Object(
          {
            kind: Type.Union([
              Type.Literal("plan"),
              Type.Literal("report"),
              Type.Literal("proposal"),
              Type.Literal("export"),
            ]),
            name: Type.String({ minLength: 1, maxLength: 160 }),
            content: Type.String({ minLength: 1, maxLength: 2097152 }),
          },
          { additionalProperties: false },
        ),
        async execute(toolCallId, params, _signal, _onUpdate, ctx) {
          const data = await bridge(ctx, toolCallId, "write_artifact", params);
          return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
        },
      }),
    );
  }
}
