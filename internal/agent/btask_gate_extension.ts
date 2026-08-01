import { Type } from "@earendil-works/pi-ai";
import { defineTool, type ExtensionAPI } from "@earendil-works/pi-coding-agent";

const config = __BTASK_GATE_CONFIG__;

type BridgeResponse = {
  version: string;
  nonce: string;
  ok: boolean;
  data?: unknown;
  error?: string;
};

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

  pi.on("before_agent_start", (event) => ({
    systemPrompt:
      event.systemPrompt +
      `\n\nBTask task scope: ${config.taskId}. Use only btask_list_resources and ` +
      "btask_read_resource for task context and references. Never use shell, Git, or arbitrary " +
      "filesystem access. context/ and sources/ are immutable. Requirement changes must be " +
      "written as proposal artifacts. Do not restate the entire session history in prompts.",
  }));

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
