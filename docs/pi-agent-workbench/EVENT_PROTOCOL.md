# BTask Agent 稳定事件协议

## 1. 目标

React 不直接消费 PI 原始 RPC。internal/agent/event_mapper.go 把当前 PI 版本映射为版本化 BTask 事件，SQLite 先保存稳定事件，Wails 再发送。

第一版只使用一个 Wails channel：

~~~text
agent:event
~~~

单 channel 避免为每种事件建立和清理多组监听；kind 完成判别。Workspace/Git/Permission 等需求中的能力仍以 kind 暴露。

## 2. Envelope v1

~~~ts
interface AgentEventV1 {
  version: 1;
  eventId: string;
  sequence: number;
  kind: AgentEventKind;
  taskId: string;
  sessionId: string;
  runId?: string;
  toolCallId?: string;
  occurredAt: string;
  payload: unknown;
}
~~~

约束：

- version 必须是整数 1；未知 major version 由前端拒绝并触发重新 hydrate。
- eventId 全局唯一，用于去重。
- sequence 在 session 内单调递增且落库后不复用。
- taskId、sessionId 始终存在。
- run-scoped 事件必须有 runId。
- tool-scoped 事件必须同时有 runId、toolCallId。
- occurredAt 使用 UTC RFC3339Nano。
- payload 只含稳定 BTask DTO，不透传任意 PI object。

## 3. EventKind v1

~~~text
session.state
run.state
message.start
message.delta
message.end
reasoning.delta
queue.updated
tool.start
tool.update
tool.end
permission.requested
permission.resolved
workspace.changed
git.changed
compaction.state
retry.state
agent.settled
error
~~~

### 3.1 session.state

~~~ts
interface SessionStatePayload {
  state:
    | "created"
    | "starting"
    | "idle"
    | "running"
    | "stopping"
    | "interrupted"
    | "failed"
    | "closed";
  model?: string;
  thinkingLevel?: string;
  piVersion?: string;
  sessionName?: string;
  error?: AgentErrorPayload;
}
~~~

### 3.2 run.state

~~~ts
interface RunStatePayload {
  state:
    | "queued"
    | "running"
    | "waiting_permission"
    | "stopping"
    | "succeeded"
    | "failed"
    | "cancelled"
    | "interrupted";
  reason?: string;
}
~~~

### 3.3 message events

~~~ts
interface MessageStartPayload {
  messageId: string;
  role: "user" | "assistant" | "system";
  kind: "text" | "notice";
}

interface MessageDeltaPayload {
  messageId: string;
  delta: string;
  accumulatedChars: number;
}

interface MessageEndPayload {
  messageId: string;
  status: "complete" | "error" | "cancelled";
  content?: string;
  contentRef?: string;
  error?: AgentErrorPayload;
}
~~~

reasoning.delta 与 message.delta 结构相同，但 UI 可独立折叠。不得持久化或展示 provider 未提供的“隐藏思维链”；只处理 PI 明确发出的 thinking content。

### 3.4 queue.updated

~~~ts
interface QueueUpdatedPayload {
  steeringCount: number;
  followUpCount: number;
}
~~~

默认不把待处理消息全文放入事件；历史页从数据库读取用户已提交内容。

### 3.5 tool events

~~~ts
interface ToolStartPayload {
  toolName: string;
  capability: string;
  subject: string;
  readOnly: boolean;
  riskLevel: "low" | "medium" | "high" | "critical";
  argsPreview?: string;
  argsRef?: string;
}

interface ToolUpdatePayload {
  outputPreview?: string;
  outputRef?: string;
  accumulated: true;
  truncated?: boolean;
}

interface ToolEndPayload {
  state: "succeeded" | "failed" | "denied" | "cancelled";
  outputSummary?: string;
  outputRef?: string;
  durationMs?: number;
  error?: AgentErrorPayload;
}
~~~

PI tool_execution_update.partialResult 是累计值，因此 accumulated 固定为 true；前端按 toolCallId 替换 preview，不拼接。

### 3.6 permission events

~~~ts
interface PermissionRequestedPayload {
  requestId: string;
  capability: string;
  subject: string;
  target: string;
  cwd?: string;
  riskLevel: "low" | "medium" | "high" | "critical";
  allowedScopes: Array<"once" | "session" | "task" | "permanent">;
  expiresAt: string;
}

interface PermissionResolvedPayload {
  requestId: string;
  decision: "allow" | "deny" | "expired" | "cancelled";
  scope?: "once" | "session" | "task" | "permanent";
}
~~~

高风险请求的 allowedScopes 只能包含 once。前端不能自己扩展 scope。

### 3.7 workspace.changed / git.changed

~~~ts
interface WorkspaceChangedPayload {
  paths: string[];
  reason: "artifact" | "attachment" | "context" | "external";
}

interface GitChangedPayload {
  bindingId: string;
  branch: string;
  dirty: boolean;
  changedPaths?: string[];
}
~~~

changedPaths 有数量和字符上限；超出时省略，前端主动刷新列表。

### 3.8 error

~~~ts
interface AgentErrorPayload {
  code:
    | "pi_not_installed"
    | "unsupported_pi_version"
    | "startup_timeout"
    | "gate_unavailable"
    | "protocol_error"
    | "frame_too_large"
    | "truncated_frame"
    | "process_exit"
    | "session_missing"
    | "permission_timeout"
    | "path_denied"
    | "storage_error"
    | "unknown";
  message: string;
  retryable: boolean;
  detailRef?: string;
}
~~~

message 经过路径和凭据清洗。详细 stderr 通过 detailRef 按需读取。

## 4. PI 0.82.1 映射

| PI raw | BTask v1 | 说明 |
|---|---|---|
| agent_start | run.state=running | 对应已创建 run |
| message_start assistant | message.start | 创建 streaming message |
| message_update text_delta | message.delta | 100 ms/32 KiB 合并 |
| message_update thinking_delta | reasoning.delta | 独立折叠 |
| message_update toolcall_* | 不直接创建执行结果 | 可更新 args preview；完整 tool start 以 tool_execution_start 为准 |
| message_end | message.end | 与 get_entries 对账 |
| tool_execution_start | tool.start | toolCallId 原样保存为 external id |
| tool_execution_update | tool.update | 累计替换 |
| tool_execution_end | tool.end | 记录 result、isError 和 output ref |
| queue_update | queue.updated | 只投影计数 |
| compaction_start/end | compaction.state | start/succeeded/failed/aborted |
| auto_retry_* 和 summarization_retry_* | retry.state | attempt/state |
| extension_ui_request permission envelope | permission.requested | 先由 Go 验证 envelope |
| extension_error | error + run.failed | gate 相关错误立即失败关闭 |
| agent_end | 通常不发终态 | willRetry 可能继续；仅诊断 |
| agent_settled | agent.settled + run.state | 最终完成信号 |
| process exit | session.state/error/run.state | 未 settled 的 run 变 interrupted/failed |

PI 原始事件没有 BTask taskId/sessionId/runId。Supervisor 以进程注册记录包裹；任何找不到唯一 scope 的事件直接丢入诊断并停止该进程，不能广播成“当前任务事件”。

## 5. 顺序、持久化和重放

后端处理顺序：

1. 在单 Session event loop 中接收已解码 PI frame。
2. 映射稳定事件。
3. 在 SQLite transaction 中分配 sequence、更新投影并插入 agent_events。
4. 提交 transaction。
5. 放入有界 Wails emitter queue。

这保证 UI 看见的事件已有持久记录。Wails emitter 不持有 Agent/RPC 锁。

有界策略：

- queue 上限 1024 个 envelope 或 8 MiB，以先到者为准。
- message.delta、reasoning.delta、tool.update 可按相同 id 合并为最新累计状态。
- 终态、permission、error、workspace/git 事件不可丢弃。
- 无法腾出空间时停止该 run 并发出 storage/backpressure error，而不是无限占用内存。

重放：

- 前端首次选择任务时先 GetAgentHistoryPage/ListAgentSessions，再订阅或以 bridge 提供的 hydrate+cursor 原子流程完成。
- 事件 eventId 去重。
- sequence <= 已应用 sequence 的事件丢弃。
- 发现 sequence gap 时暂停实时 reducer，调用 history/state API 对账。
- 页面刷新、WebView 丢事件和应用重启都以 SQLite 为权威恢复。

## 6. 前端订阅合同

bridge 提供：

~~~ts
function subscribeAgentEvents(
  handler: (event: AgentEventV1) => void,
): () => void;
~~~

- Wails EventsOn 返回值如果可用，直接作为 unsubscribe。
- React effect 每次 taskId/sessionId 改变时先调用旧 unsubscribe，再建立新 subscription epoch。
- reducer 同时校验 taskId、sessionId 和本地 epoch。
- runId 不匹配当前 run 的实时事件可写入后台 session cache，但不能更新当前输入区/权限卡。
- 组件卸载后不得保留 callback。
- 同一页面只建一个底层 EventsOn，按 session 路由到 store；禁止每个 ToolCard 单独订阅。

Reasonix desktop/frontend/src/lib/useController.ts 的 runtime epoch 和 stale-event 测试可作为模式参考，但 BTask 使用本协议字段重新实现。

## 7. 浏览器 Mock

浏览器模式不启动 pi。Mock bridge：

- 使用与桌面完全相同的 AgentEventV1。
- 使用 deterministic fixture sequence，不用 setInterval 随机事件。
- 支持 start、delta、end、tool、permission、stop、error 和任务切换。
- permission 解析仍经过浏览器内纯策略 fixture，不能把 Mock 当安全验收。
- 测试结束必须清理 timer 和 listeners。

## 8. 大输出

- 事件 payload_json 默认最大 256 KiB。
- tool args/output preview 默认最大 32 KiB。
- 超出部分写入 runs/ 下受控文件，event 只包含 truncated=true 和 ref。
- ReadAgentOutput 按 task/session/run/tool scope 校验后分页读取；不返回任意绝对路径。
- React store 不保存完整大输出；ToolCard 展开时懒加载，关闭后允许释放。

## 9. 错误和未知事件

- 未知 PI event：保存 sanitized raw kind 诊断，映射 error=unknown_pi_event；若处于工具/权限边界则失败关闭，否则可继续但标记版本不完全兼容。
- 未知 BTask v1 kind：前端忽略 payload、记录诊断并 hydrate；不能崩溃。
- payload schema 不合法：后端不发送；前端收到时丢弃并对账。
- error 事件不等同于 run 终态，除非随后 run.state 明确。

## 10. 测试向量

至少覆盖：

- text delta 正常顺序和 batched flush。
- U+2028/U+2029 不分帧。
- tool update 累计替换，不重复拼接。
- agent_end(willRetry=true) 不结束 run。
- agent_settled 结束 run。
- 同 task 两 session 事件不串。
- 两 task 相同 session 名称不串。
- stale epoch 丢弃。
- sequence duplicate 和 gap。
- Wails unsubscribe。
- queue backpressure 和不可丢事件。
- 大 payload ref。
- process exit 产生 interrupted。
- permission request 在 app restart 后 expired。
