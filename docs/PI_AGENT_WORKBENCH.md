# 任务级原生 PI Agent 工作台

## 1. 当前范围

任务详情右侧区域已经接入原生 PI Agent 工作台。每个任务拥有独立的文件空间、
PI Session、消息、事件、运行、工具、权限、附件、产物和 Git worktree 绑定。

工作台支持：

- Ask、Plan、Agent 三种模式。
- 新建、切换、恢复任务内 Session。
- 分页历史、流式文本、Steer、Follow-up 和停止。
- 当前任务资源的 `@` 引用、图片和文档附件。
- Context、Files、Changes、Runs 面板。
- 工具、权限、错误和恢复卡片。
- 浏览器 Mock 与 Wails 桌面端相同的交互合同。

Agent 完成、工具成功或测试通过都不会批准需求、提交代码或推进 Task 状态。

## 2. 页面上下文隔离

`TaskAgentWorkbench` 的最外层身份是当前 `taskId`。加载和实时事件还必须匹配
当前 `sessionId`；运行和工具操作继续校验 `runId`、`toolCallId`。

切换任务时前端会：

1. 增加页面 epoch，使上一任务尚未返回的异步请求失效。
2. 取消上一任务的事件订阅。
3. 清空 Session、消息、权限、工具、引用、预览、运行和 Git 面板状态。
4. 只接受 `event.taskId === currentTaskId` 的事件。
5. 选择 Session 后，再拒绝其他 Session 的消息和运行事件。

Go 和 SQLite 不依赖前端过滤作为安全边界。所有查询和写入都携带 task scope，
复合外键拒绝跨任务的 session/run/message/tool/resource 组合。

## 3. 原生 PI 运行时

当前支持已验证的原生 PI `0.82.x`，不调用或回退到 OMP。Supervisor 启动形态为：

```text
pi --mode rpc \
  --session-dir <task>/.btask/pi-sessions \
  --no-extensions \
  --no-skills \
  --no-prompt-templates \
  --no-themes \
  --no-context-files \
  --no-approve \
  --offline \
  -e <task>/.btask/pi-agent/btask-gate-v2-<session>.ts \
  --no-builtin-tools
```

应用只显式加载内嵌的 BTask gate。启动前校验其任务身份、协议版本、一次性 nonce、
SHA-256、常规文件类型和 `0600` 权限；`get_state` 与 gate 心跳都成功后才认为 Session
可用。固定 utility 分析不加载 gate，使用 `--no-tools`。

环境变量使用最小 allowlist。默认 `isolated` 不读取本机 PI 配置；用户在 PI 设置中选择
`explicit-inherit` 后，新 Session 会读取 `PI_CODING_AGENT_DIR`（默认 `~/.pi/agent`）中的
模型与 provider 登录，但不会复制或持久化凭据。两种策略都不会自动加载全局 extensions、
skills、prompts、packages 或 context，任务 Session 目录保持隔离。

## 4. Task Workspace

```text
<task-id>/
├── .btask/
│   ├── manifest.json
│   ├── resources.json
│   ├── pi-agent/
│   └── pi-sessions/
├── context/
├── sources/
├── attachments/
├── artifacts/
├── repos/
└── runs/
```

- `context/` 是当前 Task、需求和验收标准的系统投影。
- `sources/` 与 `attachments/` 是不可由 Agent 覆盖的原始输入。
- `artifacts/` 允许 Plan/Agent 通过受控工具写入。
- `repos/` 只包含该任务显式创建的 Git worktree。
- `runs/` 保存事件、stdout、stderr、结果和工具大输出。
- `.btask/` 由应用管理，不对 Agent 写工具开放。

目录默认 `0700`、文件默认 `0600`。逻辑路径拒绝 NUL、绝对路径、`..`、UNC、
Windows volume/device path、保留名、大小写别名和符号链接逃逸。

## 5. 模式与工具

| 模式 | 默认能力 |
| --- | --- |
| Ask | 读取当前任务资源和已绑定 worktree；不写文件，不运行 Shell |
| Plan | Ask + 写 `artifacts/`；不修改 worktree，不运行 Shell |
| Agent | Plan + 在 development 状态下受控修改 worktree；Shell 仍受策略和审批约束 |

PI 内建 `read`、`write`、`edit`、`bash` 不开放。当前自有工具包括任务资源读取、
artifact 写入、worktree 列表/读取/写入/精确编辑/删除和 BTask Shell。未知工具、
模式不匹配、任务状态不匹配或缺少有效 gate 都失败关闭。

Git push、force push、PR、merge、rebase、reset、发布和远端删除未作为 PI 工具开放，
也不能藏在 BTask Shell 中执行。提交与推送仍是独立、明确的人工工作流动作。

## 6. Session、消息与事件

- 每个 Agent Session 对应当前任务 `.btask/pi-sessions/` 中已登记的 PI 文件。
- 同一任务最多一个活动 run；不同任务的状态互不复用。
- `prompt`、`steer`、`follow_up` 调用原生命令，不把运行中普通提交默认为某一种队列行为。
- `agent_settled` 才是本轮最终完成信号；`agent_end` 可能仍有重试或队列续跑。
- 消息历史默认每页 60 条，最大 200 条，按 Session sequence 游标向前加载。
- 10,000 条历史的首屏查询只读取所需页面，不读取所有正文。
- 稳定事件先写 SQLite，再进入 Wails 事件队列；UI 丢事件后重新查询 SQLite 对账。
- 文本 delta 在内存聚合，每 100 ms 或 32 KiB 刷新一次；结束、取消和崩溃前强制刷新。
- Wails 事件队列最多 1024 条或 8 MiB；背压超限会停止运行并显示错误。

## 7. 附件、产物与大输出

图片通过 PI `images` 参数发送，文档通过任务资源引用读取。附件会校验数量、大小、
MIME、magic bytes、逻辑路径和任务归属；消息只保存引用，不保存 base64 快照。

PI 写入需求相关文档时先创建 `requirement_proposals`。用户采纳才形成新需求版本；
旧批准版本继续保留。

工具输出处理规则：

- SQLite 和事件只保存不超过 32 KiB 的脱敏摘要。
- 完整 UTF-8 输出写入对应 `runs/<run-id>/`，当前上限 12 MiB。
- UI 展开工具卡片后才读取输出，单次最多 2 MiB，并明确显示是否截断及原文件大小。
- 工具大输出不进入 Zustand 的全量持久化快照。

## 8. Git worktree

绑定仓库时记录 source realpath、common git dir、基线 commit、原分支和绑定时脏状态。
任务 worktree 使用独立分支和目录；不复用、不嵌套于原始工作区。每次读写重新验证
realpath 和绑定身份。

状态页展示 staged/unstaged/untracked/renamed/deleted 变化。单文件 Diff 预览超过
2 MiB 时截断，同时返回完整字节数和诊断标记。worktree 被外部删除或 source repo
被移动时绑定进入 `missing`，不会静默重建或修改原始仓库。

## 9. 恢复

应用启动会把遗留的 running/stopping Session、run、message、tool 和 permission
原子标记为 interrupted/error/expired，同时保留日志和历史。恢复必须由用户点击：

1. 校验已登记的 PI Session 文件仍在当前任务目录。
2. 启动新的隔离 PI 进程并完成 gate 探针。
3. 调用 `switch_session`、`get_state` 和 `get_entries` 对账。
4. 只补齐尚未持久化的 PI entry，不覆盖 BTask 审计记录。

Session 文件丢失、非法或越界时保留 SQLite 消息并显示明确错误。PI crash、非法帧、
截断帧、超大帧、权限超时、取消与进程退出竞态都会形成终态，不自动重复执行工具。

## 10. 安全边界与非目标

BTask gate、路径解析、工具 allowlist、权限数据库和 worktree 提供应用级软边界，
不是容器、VM 或操作系统沙箱。PI、Extension、Git 和 Shell 仍以当前用户权限运行；
不适合把恶意代码交给无人监管的 Agent。

第一版不做多 Agent 并行、跨任务全局记忆、自动提交、自动 push、自动 PR、自动合并、
自动发布、浏览器自动操作或物理删除旧任务数据。

更细的协议、数据和权限合同见：

- `docs/pi-agent-workbench/PI_RPC_CONTRACT.md`
- `docs/pi-agent-workbench/DATA_MODEL.md`
- `docs/pi-agent-workbench/EVENT_PROTOCOL.md`
- `docs/pi-agent-workbench/PERMISSION_MODEL.md`
