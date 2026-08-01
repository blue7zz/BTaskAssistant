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
  riskLevel: "low" | "medium" | "high" | "critical";
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

function worktreePath(value: unknown): string | undefined {
  let path = String(value || "").trim();
  if (
    !path ||
    path.includes("\0") ||
    path.startsWith("/") ||
    path.startsWith("\\\\") ||
    path.startsWith("//") ||
    /^[A-Za-z]:/.test(path)
  ) {
    return undefined;
  }
  path = path.replaceAll("\\", "/");
  const reserved = /^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$/i;
  const credential = /^(?:\.ssh|\.aws|\.gnupg|\.kube|credentials?|id_rsa|id_ed25519|\.env)$/i;
  const segments = path.split("/");
  if (
    segments.some(
      (segment) =>
        !segment ||
        segment === "." ||
        segment === ".." ||
        segment.includes(":") ||
        segment.endsWith(" ") ||
        segment.endsWith(".") ||
        /[\u0000-\u001f\u007f-\u009f]/u.test(segment) ||
        reserved.test(segment) ||
        credential.test(segment) ||
        segment.toLowerCase() === ".git",
    )
  ) {
    return undefined;
  }
  return segments.join("/");
}

function shellPermission(params: Record<string, unknown>): PermissionDescription | undefined {
  const command = String(params.command || "").trim();
  let cwd = String(params.cwd || "").trim() || ".";
  if (!command || command.includes("\0")) return undefined;
  if (cwd !== ".") {
    const normalized = worktreePath(cwd);
    if (!normalized) return undefined;
    cwd = normalized;
  }
  const cwdTarget = normalizedTarget("task-worktree", cwd, "execute");
  const commandDigest = argsDigest({ command, cwd: cwdTarget });
  let target = `${command}\n[cwd: ${cwd}]`;
  let normalized = `${cwdTarget}\0${command}`;
  let subject = "在当前任务 worktree 执行命令";
  let riskLevel: PermissionDescription["riskLevel"] = "medium";
  const credential = /(?:\.ssh|id_rsa|id_ed25519|security\s+find-(?:generic|internet)-password|(?:printenv|env)\s+.*(?:token|secret|password)|(?:authorization|token|pat|password|secret|api[_-]?key|cookie|credential)\s*[:=])/i;
  const critical = /(^|[;&|\s])(?:git\s+(?:push|rebase|reset|filter-branch|filter-repo)|gh\s+(?:pr\s+create|release)|(?:npm|pnpm|yarn|cargo)\s+publish)(?:\s|$)/i;
  const destructive = /(^|[;&|\s])(?:rm\s+(?:-[a-z]*r[a-z]*f|-rf|-fr)|git\s+clean\s+[^;&|]*(?:-[a-z]*f)|gh\s+repo\s+delete)(?:\s|$)/i;
  const remoteDelete = /(^|[;&|\s])curl\b[^;&|]*(?:-X\s*|--request(?:=|\s+))DELETE(?:\s|$)/i;
  const dependency = /(^|[;&|\s])(?:(?:npm|pnpm|yarn)\s+(?:install|add|update)|cargo\s+(?:add|install)|go\s+get)(?:\s|$)/i;
  const indirect = /(?:[|<>`]|\$\(|&&|\|\||(^|[;\s])(?:eval|source|xargs|sudo|ssh|curl|wget)([;\s]|$)|find\s+.*-exec|(?:npm|pnpm|yarn)\s+(?:run|exec)|(?:sh|bash|zsh)\s+-c)/i;
  if (credential.test(command)) {
    riskLevel = "critical";
    target = "[REDACTED credential-bearing command]";
    normalized = `${cwdTarget}\0[REDACTED:${commandDigest}]`;
  } else if (critical.test(command) || destructive.test(command) || remoteDelete.test(command)) {
    riskLevel = "critical";
    subject = "执行关键 Git、发布、远端或大量删除操作";
  } else {
    if (dependency.test(command)) {
      riskLevel = "high";
      subject = "安装或更新项目依赖";
    }
    if (indirect.test(command) || command.includes("(") || command.includes(")") || command.includes("\n")) {
      riskLevel = "high";
      subject = "执行含重定向、管道或间接调用的命令";
    }
  }
  return {
    capability: "shell.execute",
    subject,
    target,
    normalizedTarget: normalized,
    riskLevel,
  };
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
  if (toolName === "btask_list_worktree_files") {
    return {
      capability: "task.worktree.list",
      subject: "列出当前任务 worktree 文件",
      target: "当前任务 worktree 文件索引",
      normalizedTarget: normalizedTarget("task-worktree", ".", "read"),
      riskLevel: "low",
    };
  }
  if (toolName === "btask_read_worktree_file") {
    const path = worktreePath(params.path);
    if (!path) return undefined;
    return {
      capability: "task.worktree.read",
      subject: "读取当前任务 worktree 文件",
      target: path,
      normalizedTarget: normalizedTarget("task-worktree", path, "read"),
      riskLevel: "low",
    };
  }
  if (toolName === "btask_write_worktree_file") {
    const path = worktreePath(params.path);
    if (!path) return undefined;
    return {
      capability: "task.worktree.write",
      subject: "写入当前任务 worktree 文件",
      target: path,
      normalizedTarget: normalizedTarget("task-worktree", path, "modify"),
      riskLevel: "medium",
    };
  }
  if (toolName === "btask_edit_worktree_file") {
    const path = worktreePath(params.path);
    if (!path) return undefined;
    return {
      capability: "task.worktree.write",
      subject: "精确编辑当前任务 worktree 文件",
      target: path,
      normalizedTarget: normalizedTarget("task-worktree", path, "modify"),
      riskLevel: "medium",
    };
  }
  if (toolName === "btask_delete_worktree_file") {
    const path = worktreePath(params.path);
    if (!path) return undefined;
    return {
      capability: "task.worktree.delete",
      subject: "删除当前任务 worktree 文件",
      target: path,
      normalizedTarget: normalizedTarget("task-worktree", path, "delete"),
      riskLevel: "medium",
    };
  }
  if (toolName === "btask_shell") return shellPermission(params);
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
    const modeGuidance = config.mode === "agent"
      ? "Agent mode may use BTask worktree list/read/write/edit/delete tools. Shell commands must use btask_shell and always pass BTask approval. Never run Git commit, push, merge, PR, release, credential access, or arbitrary filesystem operations."
      : "Ask/Plan may list and read the bound task worktree but must not modify it or execute Shell.";
    return { systemPrompt:
      event.systemPrompt +
      `\n\nBTask task scope: ${config.taskId}. Use BTask tools only. Use btask_list_resources and ` +
      "btask_read_resource for task context and references. " + modeGuidance + " " +
      "context/ and sources/ are immutable. Requirement changes must be " +
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
      name: "btask_list_worktree_files",
      label: "List task worktree files",
      description: "List tracked and untracked files only in the Git worktree bound to this BTask task.",
      parameters: Type.Object(
        {
          query: Type.Optional(Type.String({ maxLength: 200 })),
          limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 500 })),
        },
        { additionalProperties: false },
      ),
      async execute(toolCallId, params, _signal, _onUpdate, ctx) {
        const data = await bridge(ctx, toolCallId, "list_worktree_files", params);
        return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
      },
    }),
  );

  pi.registerTool(
    defineTool({
      name: "btask_read_worktree_file",
      label: "Read task worktree file",
      description: "Read one UTF-8 file from the Git worktree bound only to this BTask task.",
      parameters: Type.Object(
        { path: Type.String({ minLength: 1, maxLength: 1000 }) },
        { additionalProperties: false },
      ),
      async execute(toolCallId, params, _signal, _onUpdate, ctx) {
        const data = await bridge(ctx, toolCallId, "read_worktree_file", params);
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

  if (config.mode === "agent") {
    pi.registerTool(
      defineTool({
        name: "btask_write_worktree_file",
        label: "Write task worktree file",
        description: "Create or replace one UTF-8 file in the current task Git worktree.",
        parameters: Type.Object(
          {
            path: Type.String({ minLength: 1, maxLength: 1000 }),
            content: Type.String({ maxLength: 2097152 }),
          },
          { additionalProperties: false },
        ),
        async execute(toolCallId, params, _signal, _onUpdate, ctx) {
          const data = await bridge(ctx, toolCallId, "write_worktree_file", params);
          return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
        },
      }),
    );

    pi.registerTool(
      defineTool({
        name: "btask_edit_worktree_file",
        label: "Edit task worktree file",
        description: "Apply an exact old-text replacement to one UTF-8 file in the current task Git worktree.",
        parameters: Type.Object(
          {
            path: Type.String({ minLength: 1, maxLength: 1000 }),
            oldText: Type.String({ minLength: 1, maxLength: 2097152 }),
            newText: Type.String({ maxLength: 2097152 }),
            replaceAll: Type.Optional(Type.Boolean()),
          },
          { additionalProperties: false },
        ),
        async execute(toolCallId, params, _signal, _onUpdate, ctx) {
          const data = await bridge(ctx, toolCallId, "edit_worktree_file", params);
          return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
        },
      }),
    );

    pi.registerTool(
      defineTool({
        name: "btask_delete_worktree_file",
        label: "Delete task worktree file",
        description: "Delete one regular file from the current task Git worktree.",
        parameters: Type.Object(
          { path: Type.String({ minLength: 1, maxLength: 1000 }) },
          { additionalProperties: false },
        ),
        async execute(toolCallId, params, _signal, _onUpdate, ctx) {
          const data = await bridge(ctx, toolCallId, "delete_worktree_file", params);
          return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
        },
      }),
    );

    pi.registerTool(
      defineTool({
        name: "btask_shell",
        label: "Run task worktree command",
        description: "Run one approved build, test, check, or ordinary Shell command in the current task worktree.",
        parameters: Type.Object(
          {
            command: Type.String({ minLength: 1, maxLength: 65536 }),
            cwd: Type.Optional(Type.String({ maxLength: 1000 })),
            timeoutSeconds: Type.Optional(Type.Integer({ minimum: 1, maximum: 3600 })),
          },
          { additionalProperties: false },
        ),
        async execute(toolCallId, params, _signal, onUpdate, ctx) {
          onUpdate?.({ content: [{ type: "text", text: `Running in ${String(params.cwd || ".")}: ${params.command}` }] });
          const data = await bridge(ctx, toolCallId, "shell", params) as any;
          if (!data?.success) throw new Error(`Shell command failed: ${JSON.stringify(data)}`);
          return { content: [{ type: "text", text: JSON.stringify(data, null, 2) }], details: data };
        },
      }),
    );
  }
}
