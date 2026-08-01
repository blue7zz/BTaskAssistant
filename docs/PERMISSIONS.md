# PI Agent 权限与安全边界

## 1. 权威边界

Go 后端是权限决策权威。PI gate 负责把工具请求送到后端并阻止未获允许的调用，
React 只展示请求和提交用户选择。隐藏按钮、前端状态或 PI 自身的 approval mode
都不能授予能力。

这是一层应用级软边界，不是操作系统沙箱。PI、Extension、Git 和 Shell 与应用使用
同一用户权限；恶意同用户进程仍可能绕过应用策略。强隔离需要容器、VM 或 OS 策略，
不属于第一版。

## 2. 决策输入

每次请求都同时校验：

- `taskId`、`sessionId`、`runId`、`toolCallId`。
- 当前 Task 状态和 Agent 模式。
- gate 版本、一次性 nonce、Extension 文件与握手状态。
- 工具 allowlist、结构化参数摘要、真实目标和 cwd。
- 路径规范化、realpath、符号链接和 worktree 绑定。
- 已存在的 allow/deny grant、风险上限、过期/消费/撤销状态。

任何身份不匹配、未知工具、不可解析目标或缺失回执都会失败关闭。

## 3. 模式硬限制

| 能力 | Ask | Plan | Agent |
| --- | --- | --- | --- |
| 读当前任务 context/source/attachment/artifact | 允许 | 允许 | 允许 |
| 读已绑定任务 worktree | 允许 | 允许 | 允许 |
| 写 artifacts | 拒绝 | 允许 | 允许 |
| 写/编辑/删除 worktree | 拒绝 | 拒绝 | 仅 development 状态允许 |
| BTask Shell | 拒绝 | 拒绝 | 仅 development 状态进入策略判断 |

Agent、工具或测试结果不会改变 Task 状态。`context/`、`sources/`、`attachments/`、
`.btask/` 和 `runs/` 不对 Agent 写工具开放。

## 4. 风险与授权作用域

风险分为 low、medium、high、critical。风险分类用于限制可选授权范围，并不自行授权。

- `once`：绑定一条 permission request、tool call 和参数摘要；使用前消费。
- `session`：只在同 task + session 生效；进程退出、Session 关闭或 gate epoch 变化后失效。
- `task`：可跨当前任务 Session，但不能跨任务，仍受模式和 Task 状态硬限制。
- `permanent`：可撤销的应用级规则，只适合可稳定规范化的低/中风险能力。

高风险未知工具只允许逐次确认；critical 动作不能用 session/task/permanent grant 放行。
拒绝优先于允许，过期、消费或撤销的 grant 不匹配。

## 5. 始终拒绝或未开放

当前 PI 工具始终拒绝或根本不暴露：

- 覆盖当前任务的 context、sources、attachments、`.btask` 或 runs。
- 读取或写入未绑定的绝对路径、其他任务目录或凭据位置。
- `..`、绝对路径、UNC、Windows device/volume、junction、符号链接和大小写别名逃逸。
- Ask/Plan 中的 Shell，或非 development Task 中的 worktree 修改。
- gate 缺失、版本/nonce/身份不符、Extension error 或工具结束缺少合法回执。
- 读取 `.ssh`、credential store、浏览器 profile、云凭据或敏感环境变量。
- 将 `git add/commit/push/merge/rebase/reset/worktree`、`gh pr`、发布动作藏入 Shell。
- 自动 push、force push、PR、合并、发布、远端删除或历史重写。

如未来开放 push、发布或远端删除，必须使用专用结构化工具并逐次确认；不能由宽授权继承。

## 6. 路径规则

所有逻辑路径都由 Go 解析，而不是字符串前缀判断：

1. 拒绝 NUL、空路径、绝对路径、`..`、重复分隔、volume、UNC 和 device path。
2. 逐 segment `Lstat`，拒绝系统管理区和写目标上的 symlink。
3. 现有路径对账 realpath；新文件对账最近存在父目录。
4. worktree 目标必须仍属于已登记 binding，且不能等于或嵌套于 source workspace。
5. macOS/Windows 的大小写碰撞、Windows 保留名和 alternate data stream 形式被拒绝。

Shell 的 cwd 必须是任务 worktree 中的已验证目录。命令分类会识别重定向、管道、
subshell、command substitution、`rm -rf`、网络命令、凭据读取和隐藏 Git/发布副作用。
分类用于提高风险；它不是强安全证明。

## 7. Gate 与审计流程

1. Supervisor 物化并校验应用内嵌 gate Extension。
2. gate 用版本和一次性 nonce 完成启动心跳。
3. PI 发出 allowlist 工具调用。
4. gate 生成 `BTASK_PERMISSION_V1` envelope。
5. Go 对账 task/session/run/tool、目标、参数摘要和策略。
6. 默认允许或硬拒绝先写审计；需要人工时写入 pending request 并发送权限卡片。
7. 用户选择写入 permission request/grant 后，Go 才回复 PI。
8. 工具结束必须和已允许且已开始执行的回执对账。

权限请求默认 5 分钟超时。拒绝、超时、取消、应用关闭或进程退出不会创建 grant；
所有决策和 reconcile 结果均保留 task/session/run/tool 范围。

## 8. 凭据与日志

Plane PAT 等凭据保存在操作系统凭据库，不写 SQLite、Task Workspace、前端状态、
fixture 或 Git。PI 环境采用最小 allowlist，不透传敏感名称。工具参数、事件、stderr、
URL 和 Git remote 在持久化或展示前按常见 token/password/header 模式脱敏。

脱敏是纵深防御，不应替代“不读取、不传递凭据”的硬规则。

详细 capability、grant 和 gate 合同见
`docs/pi-agent-workbench/PERMISSION_MODEL.md`。
