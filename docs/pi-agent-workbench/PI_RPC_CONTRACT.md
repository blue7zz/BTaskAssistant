# 原生 PI 0.82.1 RPC 合同

## 1. 身份与证据

本合同只针对当前机器实际安装的原生 PI：

| 项目 | 值 |
|---|---|
| executable | /opt/homebrew/bin/pi |
| symlink target | ../lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js |
| package | @earendil-works/pi-coding-agent |
| version | 0.82.1 |
| license | MIT |
| repository | github.com/earendil-works/pi，packages/coding-agent |
| Node requirement | >=22.19.0 |

本机权威证据：

- /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/rpc.md
- /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/security.md
- /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md
- /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/jsonl.js
- /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-mode.js
- /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-types.d.ts

不得用 OMP 文档、omp --help 或历史调用参数补齐本合同。

## 2. 启动合同

每个活动 Agent Session 使用一个受 Supervisor 管理的长寿命进程。同一任务同一时刻最多一个活动运行。

默认隔离启动形态：

~~~text
PI_CODING_AGENT_DIR=<task-workspace>/.btask/pi-agent
PI_CODING_AGENT_SESSION_DIR=<task-workspace>/.btask/pi-sessions
PI_OFFLINE=1

pi \
  --mode rpc \
  --session-dir <task-workspace>/.btask/pi-sessions \
  --no-extensions \
  -e <task-workspace>/.btask/pi-agent/btask-gate.ts \
  --no-skills \
  --no-prompt-templates \
  --no-themes \
  --no-context-files \
  --no-approve \
  --offline \
  --no-builtin-tools
~~~

说明：

- --no-extensions 禁止全局和项目自动发现；PI 0.82.1 仍允许显式 `-e/--extension` 路径。
- btask-gate.ts 由 Go 内嵌模板原子物化到当前任务，配置中固化 task、session、mode、version 和一次性 nonce；启动前再次校验 regular-file、0600 mode 和 SHA-256，不从项目或全局目录发现 Extension。
- --no-builtin-tools 禁止 read/bash/edit/write 等内建工具，但保留 BTask Extension 注册的自有工具。
- Ask、Plan、Agent 三种模式通过 BTask 注册工具集合和策略决定，不通过 OMP approval mode。
- cwd 是当前任务空间；进入开发模式后仓库写工具只接受当前任务 repos/<repo> worktree 内目标。
- Supervisor 构造最小环境变量集合。默认不透传所有 provider API key、SSH agent、云凭据或 ~/.pi/agent 配置。
- 用户明确选择的模型凭据由现有 OS credential store 或后续专用设置提供；不得复制或记录全局 auth.json。
- 需要继承某个全局 PI 资源时，必须成为可见、可撤销的任务设置，并重新启动 Session。

阶段 2 若尚未启用任何工具，可以进一步使用 --no-tools。阶段 3 起需加载 BTask custom tools，因此使用 --no-builtin-tools。

## 3. 可用性判定

PI 0.82.1 RPC 没有 ready 帧，也没有协议版本协商命令。BTask 不得虚构 ready 或 protocol-v2。

启动状态机：

1. spawned：进程成功启动，stdin/stdout/stderr 已连接。
2. probing：发送带唯一 id 的 get_state。
3. ready：在启动超时内收到同 id、command=get_state、success=true 的 response；只要 Session 开放任何 custom tool，还必须验证 gate handshake。
4. failed：进程提前退出、probe 超时、帧非法、gate 未加载或版本不兼容。

gate handshake：

- BTask 启动时生成一次性 nonce，并与 task/session/mode/version 一起写入当前任务的内嵌 Extension 配置。
- Extension 在 session_start 后发出 RPC Extension UI setStatus 请求，statusKey 固定为 btask-gate，statusText 包含协议版本和 nonce。
- Supervisor 只在 nonce、版本和显式 Extension 路径全部匹配后开放 custom writer/shell 工具。
- 超时、extension_error、重载、Session 切换后缺少新 handshake 都失败关闭。
- 阶段 2 的 no-tools Session 和固定 utility Session 不加载工具，可在 get_state 成功后 ready；阶段 3 起只要加载 read/artifact 等 custom tool，握手就是 ready 的必要条件。
- 该握手用于检测配置和实现错误，不是密码学或操作系统安全边界。

本机隔离探针证实：启动后 stdout 不主动发送首帧；get_state、get_messages、get_entries、get_commands 可响应。即使 --no-extensions，0.82.1 的 get_commands 仍可出现临时内置 llama command，不能把“命令列表非空”等同于用户资源泄漏或 gate 成功。

## 4. JSONL framing

原生合同是严格 LF-only JSONL：

- 输入：每个 JSON object 后写一个字节 LF。
- 输出：只按字节 LF 分帧。
- 帧尾可有一个 CR，解析前移除。
- 不能使用把 U+2028/U+2029 当换行的通用 line reader。
- UTF-8 多字节字符可能跨 read 边界，decoder 必须保留不完整尾部。
- 空行可忽略；非空非法 JSON 是协议错误。

BTask decoder：

1. 使用 bufio.Reader.ReadSlice('\n') 或等价的字节级 LF 累积器，不使用 Scanner 默认 64 KiB 限制。
2. 每个进程维护独立 buffer。
3. 单帧硬上限 16 MiB；超过上限立即终止进程，run 标记 protocol_error。
4. EOF 时若 buffer 仍有非空内容，记录 truncated_frame，不尝试猜测修复。
5. stdout 只允许 RPC JSON；任何非 JSON 文本都作为协议错误，而不是终端文本解析。
6. stderr 独立读取，不参与 framing。

本机探针把实际 U+2028 放入 set_session_name，响应和后续 get_state 均保持该字符，证明只能按 LF 分帧。

PI 自带 dist/modes/rpc/jsonl.js 使用 StringDecoder，并只搜索换行符。其源码未声明最大 RPC 帧，因此 16 MiB 是 BTask 自身的资源保护，不应宣称为 PI 限制。

## 5. 命令合同

所有请求都由 BTask 生成唯一 id，并以 id 关联 response。原始 Agent events 通常没有请求 id。

| 能力 | PI 0.82.1 命令 | BTask 用法 |
|---|---|---|
| 发送消息 | prompt | 空闲时提交；运行中必须明确 streamingBehavior |
| steer | steer | 当前工具调用完成后、下次模型调用前处理 |
| follow-up | follow_up | Agent 无工具和 steer 后处理 |
| 停止 | abort | 取消当前 Agent run，不等同于杀进程 |
| 新会话 | new_session | 成功后重新探测 state 和 gate |
| 状态 | get_state | 启动探针、恢复对账 |
| 当前消息 | get_messages | UI 对账，不作为完整历史游标 |
| 增量历史 | get_entries, since | 使用稳定 entry id 保存 durable cursor |
| 会话切换 | switch_session | 只允许任务 .btask/pi-sessions 内已登记文件 |
| 会话树 | get_tree | 历史分支展示所需 |
| 模型 | get_available_models, set_model | 只使用实际返回的 provider/model |
| thinking | get_available_thinking_levels, set_thinking_level | 不沿用 OMP --thinking 假设 |
| 队列模式 | set_steering_mode, set_follow_up_mode | 默认 one-at-a-time |
| 压缩 | compact, set_auto_compaction | 只映射实际事件 |
| 重试 | set_auto_retry, abort_retry | Agent settled 前可能仍继续 |
| 命令目录 | get_commands | 诊断资源加载，不作为工具目录 |
| session 元数据 | set_session_name, get_session_stats | 保存标题和用量投影 |
| 直接 Shell | bash, abort_bash | 第一版不由前端或模型调用 |

prompt 的 success=true 只表示请求已被接受、排队或立即处理。接受后的 provider/tool 失败通过事件和消息流报告，不会给同一 id 再发第二个失败 response。

运行中再次 prompt 若没有 streamingBehavior 会失败。BTask UI 的 Steer 和 Follow-up 必须分别调用真实命令，普通 Submit 在运行中不得默默选其中一种。

PI 的 RPC handler 可以并发处理输入。BTask 仍使用单写队列：

- prompt、new_session、switch_session、model/thinking 变更和恢复操作串行。
- abort 可越过普通队列发出，但只允许对应当前 run。
- read-only probe 可以有多个 pending request，但必须按 id 关联并有超时。
- Session 生命周期变更期间拒绝新的 prompt。

## 6. 原始事件与完成语义

PI 0.82.1 的核心事件：

- agent_start
- agent_end
- agent_settled
- turn_start / turn_end
- message_start / message_update / message_end
- tool_execution_start / tool_execution_update / tool_execution_end
- queue_update
- compaction_start / compaction_end
- auto_retry_start / auto_retry_end
- summarization_retry_scheduled / attempt_start / finished
- extension_error
- bash_execution_update（只对应直接 RPC bash）

完成规则：

- agent_end 只代表一次低层 run 完成；willRetry=true 时还会继续。
- agent_settled 才代表没有自动重试、压缩重试和队列续跑。
- BTask run 在 agent_settled 后结合最后一条 assistant message、tool errors 和 abort 状态判定 succeeded、failed 或 cancelled。
- Agent settled 不推进任务主状态。

message_update 的 assistantMessageEvent 可为 text/thinking/toolcall start、delta、end、done、error。BTask 只增量渲染 delta，message_end/turn_end/get_entries 用于最终对账。

tool_execution_update.partialResult 是截至当前的累计结果，不是增量。BTask event mapper 应替换同 toolCallId 的 preview，不重复拼接。

## 7. Session、历史和恢复

- 原生 Session 文件必须位于当前任务 .btask/pi-sessions。
- agent_sessions.external_session_path 保存规范化绝对路径；打开前同时校验 task_id、登记记录和 realpath 边界。
- SQLite 保存最后 durable entry id、消息 sequence 和 PI session id。
- 正常启动恢复：
  1. 查询 agent_sessions。
  2. 校验 Session 文件仍在任务目录。
  3. 启动隔离 PI 进程。
  4. switch_session。
  5. get_state 对账。
  6. get_entries(since=last_entry_id) 补齐。
  7. 若 cursor 不存在，退回 get_entries 全量重建该 Session 的 PI 投影，但不覆盖 BTask 审计记录。
- 应用崩溃时 running/pending/tool_running 状态在下次启动标记 interrupted；只有完成恢复对账后才能转为 idle 或 recovered。
- Session 文件丢失时保留数据库消息和 runs 日志，显示不可恢复，不自动创建同名替代文件。

## 8. 图片与附件

prompt、steer、follow_up 支持 images 数组，每项为：

~~~json
{"type":"image","data":"base64-data","mimeType":"image/png"}
~~~

BTask 发送前必须：

- 只从当前任务 attachments/images 或明确引用资源读取。
- MIME allowlist 与解码后 magic bytes 同时校验。
- 执行大小、数量和总请求上限。
- 图片原始字节总量当前上限为 10 MiB，以给 base64 膨胀、消息文本和 JSON envelope 留出空间，确保完整请求仍低于 16 MiB RPC 单帧上限。
- 数据库只保存 attachment id、路径、MIME、大小和 hash；不把 base64 放入 Agent message 快照。
- 非图片文档通过 BTask read 工具和引用装配，不伪装为 PI image。

## 9. Extension UI 与权限门禁

RPC Extension UI 请求：

- select、confirm、input、editor：阻塞并等待相同 id 的 extension_ui_response。
- notify、setStatus、setWidget、setTitle、set_editor_text：fire-and-forget。

阶段 3 的固定资源门禁使用 `input` 作为 Extension 与 Go 的结构化桥。请求 title 固定为 `btask-gate`，placeholder 是 JSON，至少包含：

- version、nonce
- taskId、sessionId、mode
- toolCallId
- operation：list_resources、read_resource 或 write_artifact
- args

Go 只在存在活动 run、toolCallId 已由同名 allowlist 工具启动、身份字段全部匹配时执行；回复使用同 id 的 `extension_ui_response`。Ask 只注册 list/read，Plan/Agent 才注册受控 artifact writer。未知工具、错 nonce、跨任务资源、非法路径或 writer 身份不匹配均失败关闭。

阶段 4 在此固定桥上增加人工审批。届时 BTask gate Extension 在 tool_call 中生成结构化 permission envelope，并使用 confirm 请求等待 BTask 决策。Envelope 至少包含：

- version
- nonce
- taskId
- sessionId
- runId
- toolCallId
- toolName
- capability
- normalized target / cwd
- risk
- human-readable subject

Go 权限服务是决策权威；React 只显示和提交用户选择。Go 落库后才向 PI 回复 confirmed=true。拒绝、超时、取消、窗口关闭、进程退出或 envelope 不合法都回复 false 或中止进程。

PI 文档说明 tool_call handler 抛错会 fail-safe 阻止工具。这一行为必须有版本锁定集成测试。

## 10. 进程、stderr 和退出

- stdout、stderr 必须并发持续读取，避免 pipe backpressure。
- stderr 先写 runs/<run-id>/stderr.log，UI 只保留有界尾部摘要。
- stdout 原始帧可写 events.jsonl，但敏感字段应在落盘前按合同处理；数据库存稳定事件和引用。
- graceful stop：先 abort，等待 agent_settled 或短超时。
- Session close / 应用退出：关闭 stdin，等待进程退出；随后发送 SIGTERM；仍未退出才 SIGKILL。
- PI 0.82.1 处理 SIGTERM；stdin EOF 会进入关闭流程。本机探针 Ctrl-D 正常退出码 0。
- 终止必须针对实际子进程/进程组，不能按命令名批量 kill。
- 任何异常退出都携带 taskId/sessionId/runId、exit code、signal 和 stderr 摘要。

## 11. 安全边界

PI 官方 security.md 明确：

- PI 使用启动用户权限运行。
- project trust 只是输入加载门，不是沙箱。
- built-in tools、Extension、包安装和 shell 都是普通本地进程。
- 强隔离需要 OS、容器、VM 或同等级边界。

因此第一版的保证是：

- 默认资源隔离。
- 自有工具 allowlist。
- 路径 canonicalization 和 symlink 检查。
- BTask 权限决策和审计。
- Git worktree 隔离原始工作区。
- 未加载门禁时失败关闭。

第一版不保证恶意本地 Extension、被攻陷的 PI 进程或同用户进程无法绕过策略。

## 12. 兼容性与降级

| 情况 | 行为 |
|---|---|
| 找不到 pi | 显示未安装、保留历史只读，不调用 omp |
| 版本不是已验证范围 | 只做 probe；协议或 gate 测试不通过则禁用运行 |
| get_state 超时 | startup_failed，终止进程 |
| gate 未 handshake | 所有 Agent 工具禁用，进程终止 |
| 非法 stdout frame | protocol_error，保留原始诊断后终止 |
| 超大 frame | frame_too_large，终止 |
| extension_error | 当前 run 失败；writer 能力立即关闭 |
| 模型/凭据缺失 | 明确 preflight 错误，不创建伪 assistant 成功消息 |
| Session 文件丢失 | 历史只读，标记 external_session_missing |
| app bridge 不可用 | 浏览器 Mock，不启动本地 PI |

## 13. 已验证探针

使用隔离 config/session 目录和 no-* 参数执行本机探针，结果：

- 启动后没有主动 ready 输出。
- get_state 成功，返回 sessionFile、sessionId、isStreaming 等。
- get_messages 返回空数组。
- get_entries 返回 append-only entry；since 不存在时 success=false。
- get_commands 在禁用扩展发现后仍有内置 inline llama command。
- 含实际 U+2028 的 set_session_name 和 get_state 保持内容。
- 未配置 API key 时 prompt 在 preflight 返回失败，没有伪造 run 事件。
- 未知命令和非法输入都有可诊断错误响应。
- stdin EOF 后进程正常退出。

该探针验证协议形状，不构成真实模型网络调用、图片输入、工具审批和崩溃恢复验收；这些属于后续阶段集成测试与人工验证。
