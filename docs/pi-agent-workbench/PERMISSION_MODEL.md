# BTask Agent 权限模型

## 1. 边界声明

权限系统由 BTaskAssistant 持有。原生 PI 0.82.1 的 Extension tool_call 和 RPC Extension UI 是门禁接入点，不是授权数据库。

PI 与 Extension 使用当前用户的操作系统权限运行。第一版提供应用级软边界，不是 OS 沙箱。对不受信任代码或无人监管执行，需要未来另行引入容器、VM 或策略沙箱；不在第一版范围。

## 2. 三个输入维度

每次工具请求的有效能力由 Go 服务根据三项共同计算：

1. Task 主状态。
2. Agent 模式（Ask、Plan、Agent）。
3. 用户授权和硬规则。

React 只能展示结果，不能通过隐藏/显示按钮决定权限。

### 2.1 Agent 模式

| 模式 | 默认能力 |
|---|---|
| Ask | 读取当前任务允许资源；不写文件；不执行任意 Shell |
| Plan | Ask + 写 artifacts/plans、artifacts/proposals；仓库只读 |
| Agent | Plan + 在允许任务状态下修改 task worktree；Shell 仍进入审批 |

### 2.2 Task 状态

| Task 状态 | 基础能力 |
|---|---|
| inbox | 整理资料、生成候选文档；仓库只读 |
| requirements | 需求分析和检索；仓库只读 |
| approved | 生成计划；仓库只读 |
| development | Agent 模式可申请写 worktree、构建和测试 |
| review | 默认审查 Diff；修复必须由用户显式触发一次 edit run |
| done | 默认只读；继续修改前必须先由人工退回状态 |

Agent 完成、tool success 或 test pass 都不改变 Task 状态。

## 3. Capability 与风险

动作类别：

~~~text
read
write
exec
external_side_effect
destructive
credential
~~~

UI 风险级别由类别、目标和参数推导：

| 级别 | 示例 |
|---|---|
| low | 读取当前任务 context、列出 task workspace |
| medium | 写 artifacts、读取明确绑定目录、精确的本地 test 命令 |
| high | 写外部目录、安装依赖、网络访问、本地 commit、大量文件改动 |
| critical | push/force push、发布上传、远端删除、历史重写、读取凭据、跨任务修改 |

风险分类不是授权本身。未知工具或无法解析的目标至少按 high；缺少可显示 target/cwd 时直接硬拒绝。

## 4. 默认策略

| 动作 | 默认决策 |
|---|---|
| 读取当前任务 context/sources/attachments/artifacts/runs | allow |
| 读取当前任务 repos | allow；仅已绑定 worktree |
| 写 context/sources/attachments/.btask/runs | deny；只能系统服务写 |
| 写 artifacts | Plan/Agent allow；Ask deny |
| 修改任务 worktree | development + Agent allow；其他 deny |
| 读取已绑定外部资源 | allow |
| 写已绑定外部资源 | ask |
| 未绑定外部路径 | deny |
| 任意 Shell | ask；Ask/Plan deny |
| 普通测试/构建 | development + Agent ask，可精确 session/task 授权 |
| 安装依赖 | ask，high |
| 网络访问 | ask，high |
| Git local commit | ask，high；可 task grant |
| Git push / force push | always ask once，critical |
| PR、发布、上传、远端删除 | always ask once，critical |
| Git reset/rebase/filter/history rewrite | always ask once 或 deny，critical |
| 大量删除 | always ask once，critical |
| 读取凭据或 credential store | deny；只允许专用系统服务按固定用途取用且不回传 Agent |
| 修改其他任务 | deny |
| 未知工具 | ask high；无法规范化则 deny |

因为第一版没有强 Shell 沙箱，完整命令和 cwd 必须在执行前展示。即使 cwd 在 worktree，脚本、重定向、子模块、构建工具和包管理器仍可能写出目录。

## 5. 决策优先级

固定顺序：

~~~text
硬拒绝
→ 高风险强制逐次询问
→ 明确用户拒绝
→ 精确 once 授权
→ session / task / permanent 授权
→ Task 状态和 Agent 模式默认
→ 询问
~~~

细则：

- deny 永远优先于 allow。
- permanent allow 不覆盖 critical 动作。
- 撤销后的 grant 不匹配。
- 过期 grant 不匹配。
- once grant 与 request/toolCallId 绑定并原子 consumed。
- 更窄 target 比宽 pattern 优先；同等 specificity 下 deny 优先。
- task grant 不跨 task；session grant 同时绑定 task 和 session。
- 用户拒绝可记录为 once deny 或显式更宽 deny，不能把一次拒绝默认为永久。

## 6. 硬拒绝

以下请求不弹出“允许”选项：

- 写其他 task workspace。
- 写当前任务 context、sources、attachments、.btask、runs。
- 读取或写未绑定外部绝对路径。
- 通过 ..、符号链接、junction、大小写别名或 UNC 逃逸。
- 读取 .ssh、credential store、浏览器 profile、cloud credentials、环境变量 secret 等凭据目标。
- tool gate nonce/task/session/run 不匹配。
- PI gate 未加载、extension_error 或版本不支持。
- Task 不在 development 却请求 worktree write。
- Ask/Plan 请求 Shell。
- worktree 路径与原始 source workspace 相同或位于其内部。
- 命令 target/cwd 不可解析或 payload 超限。

## 7. 授权作用域

### once

- 绑定 permission_request_id 和 tool_call_id。
- 只允许完全相同的 capability、normalized target、cwd 和参数摘要。
- 成功进入工具执行前 consumed；执行失败不自动恢复。

### session

- 绑定 task_id + session_id。
- 只匹配同 capability、规范化目标 pattern 和 risk ceiling。
- Session close、PI 重启或 gate epoch 变化后失效。

### task

- 绑定 task_id。
- 可跨该任务 Session，但 Task 状态/模式硬限制仍生效。
- 适合明确 worktree 中某个目录或精确 test 命令。

### permanent

- 应用级可撤销规则。
- 只适合低/中风险可稳定规范化能力，例如读取某个显式外部文档目录。
- 不能授权 credential 或 critical 动作。

## 8. 路径决策

路径判断必须使用 internal/taskspace Resolver，不使用字符串 HasPrefix。

### 8.1 输入规范化

- 拒绝 NUL。
- 逻辑路径必须相对，分隔符统一后逐 segment 检查。
- 拒绝 ..、Windows volume、UNC 和 device path。
- macOS/Windows 处理大小写不敏感碰撞。
- 现有路径使用 realpath；新文件解析最近存在父目录再拼接 basename。
- 对每个 segment Lstat，系统管理区不跟随 symlink。

### 8.2 symlink

- context/sources/attachments/artifacts/.btask/runs 禁止 Agent 创建 symlink。
- worktree 可显示 Git 已跟踪 symlink，但写工具不跟随到 worktree 外。
- symlink target 不存在或循环时拒绝写。
- 外部资源在绑定和每次访问时对账 realpath；被替换后失效。

### 8.3 路径授权 target

保存结构化 target：

~~~json
{
  "rootKind": "task-artifacts | task-worktree | bound-external",
  "rootId": "workspace-or-binding-id",
  "relativePath": "normalized/path",
  "operation": "read | create | modify | delete"
}
~~~

数据库可另存 human-readable target，但策略匹配使用结构化字段。

## 9. Shell 与命令

Shell 工具只在 development + Agent 可申请。

执行合同：

- 展示完整 command、cwd、环境变量名称列表和预计 capability。
- 不显示 secret 值。
- 使用参数数组执行已知固定命令时优先不经过 shell。
- 需要 shell 语法时保留原始命令并做 token/classification；解析结果只用于提高风险，不作为强安全证明。
- 检测 redirection、pipe、command substitution、subshell、eval、source、xargs、find -exec、git hooks、package scripts、sudo、ssh、curl/wget upload 等。
- 发现绝对输出路径、外部重定向或不明确间接执行时 high/critical。
- stdout/stderr 有界并落盘；后台进程登记 process group，可单独停止。
- 不允许 Agent 直接使用 PI RPC 的 bash 命令；只允许 BTask custom shell tool。

精确 grant key 至少包含：

- normalized cwd root id。
- executable realpath 或固定 command family。
- 参数摘要。
- environment profile。
- network flag。

构建/测试可能间接写出目录，仍是软边界。首次未知项目命令应逐次确认。

## 10. Git 动作

| 动作 | 策略 |
|---|---|
| status/diff/log | task worktree 内 allow |
| add | ask 或精确 task grant |
| local commit | ask；可 task grant；必须展示 diff 范围和 message |
| branch create | ask；检查冲突 |
| worktree create | 明确用户动作；系统固定实现 |
| worktree remove | ask；有修改时强制 once |
| fetch | network ask |
| push | critical always once |
| force push | critical always once，单独选项 |
| merge/rebase/reset/history rewrite | high/critical；不可由宽 grant 放行 |
| remote delete / PR / release | critical always once |

PI 工具默认不提供 push/PR/release。即使后续提供，也必须通过专用结构化工具，不能藏在普通 shell grant 中。

## 11. PI gate 流程

1. Supervisor 只显式加载 app-owned btask-gate Extension。
2. Extension 完成 nonce/version handshake。
3. PI 产生 tool call。
4. Extension 的 tool_call handler 规范化基础输入，构造 BTASK_PERMISSION_V1 envelope。
5. Extension 调用 ctx.ui.confirm；RPC stdout 发 extension_ui_request。
6. Supervisor 校验 nonce、task/session/run/toolCallId 和 payload 上限。
7. Go permission service 计算 allow、deny 或 ask，并先插入 permission_requests。
8. allow：记录决策后回复 confirmed=true。
9. ask：发送 permission.requested，等待用户 ResolveAgentPermission。
10. deny/timeout/cancel：记录后回复 confirmed=false。
11. Extension 收到 false 或异常时返回 block=true。
12. tool_execution_end 和 BTask tool record 对账；没有合法 permission receipt 的 mutating tool result 标记安全错误。

超时默认 5 分钟，应用关闭立即取消。超时不创建 grant。

Extension 本身运行于同用户进程，故仍为软边界。通过 no-builtin-tools、自有 tool allowlist、双层路径校验和 receipt 对账缩小误用面。

## 12. 权限 UI

权限卡必须显示：

- 工具和能力。
- 真实目标。
- 完整命令与 cwd（Shell）。
- 原因和风险。
- 受影响 task/session/run。
- 可用作用域；后端给什么就显示什么。
- Allow once、Allow session、Allow task、Allow permanent、Deny。

交互要求：

- critical 只显示 Allow once 和 Deny。
- 默认焦点不放在永久允许。
- 提交中禁用重复点击。
- 后端已解决/过期后卡片变只读。
- 切换任务不把卡片显示到新任务。
- 关闭窗口等同取消，不等同允许。

## 13. 撤销与审计

撤销 grant：

- 写 revoked_at、created_by=user。
- 立即从内存 policy cache 移除。
- 已运行工具不回滚；后续调用重新决策。
- 对 pending request 不追溯改变，用户仍需处理或取消。

每次调用保存：

- permission request。
- 决策、作用域、用户或系统来源。
- 匹配的 grant id。
- tool start/end、结果、失败原因。
- revoke 记录。
- PI Extension UI request id 与 BTask request id 的关联。

日志不得包含 token、PAT、API key、完整凭据文件内容或未清洗环境变量。

## 14. Reasonix 复用边界

可借鉴 internal/permission 的纯函数和 deny > ask > allow 结构、bash token/risk 测试。

不得复制其“Ask 但没有 interactive Approver 时可能 Allow”的行为。BTask 的 ask 在没有 UI/响应/approver 时必须 deny/expired。

## 15. 测试

- hard deny 覆盖所有系统区和其他任务。
- allow/deny/timeout/cancel。
- once 原子消费和并发重复请求。
- session/task/permanent 作用域。
- revoke。
- critical 不被宽 grant 覆盖。
- task 状态和 Ask/Plan/Agent 矩阵。
- ../、绝对路径、symlink、case alias、Windows drive/UNC/junction。
- shell redirection、pipe、subshell、indirect exec、build script。
- push/force push/publish/remote delete 始终 once。
- gate handshake 缺失、nonce 错误、extension_error、版本错误。
- 用户拒绝后 tool 未执行。
- app restart 后 pending 变 expired。
- 审计不含 secret fixture。
