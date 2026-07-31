# 任务级 PI Agent 工作台基线审计

## 1. 审计范围与快照

- 审计日期：2026-08-01（Asia/Shanghai）。
- 仓库：/Users/blue/job/BTaskAssistant。
- 当前分支：fet/agent。
- 当前 HEAD：540aade（feat(任务资料): 为每个任务建立独立上下文目录）。
- 远端基线：origin/agent/runnable-mvp = 10838c1。
- 祖先关系：origin/agent/runnable-mvp 是当前 HEAD 的祖先；当前 HEAD 相对该远端基线 ahead 4、behind 0。
- 本地 agent/runnable-mvp = cc0dd61，相对远端 ahead 3。
- 远端：origin = https://github.com/blue7zz/BTaskAssistant.git。

审计命令包括 pwd、git status --short --branch、git branch -vv、git rev-parse HEAD、git remote -v、git log -5 --oneline、git rev-list --left-right --count。

当前工作区存在与本阶段计划文档无关的未提交任务管理 UI 修改，阶段 0 不覆盖、不暂存、不格式化这些文件：

~~~text
 D frontend/dist/.gitkeep
 M frontend/src/App.test.ts
 M frontend/src/App.tsx
 M frontend/src/components/ManualTaskDetail.tsx
 M frontend/src/components/TaskBasicInfoDialog.tsx
 M frontend/src/components/TaskComposer.tsx
 M frontend/src/lib/bridge.ts
 M frontend/src/store/workspace.ts
 M frontend/src/styles.css
?? frontend/package.json.md5
?? frontend/src/components/TaskBasicInfoDialog.test.ts
?? frontend/src/components/TaskComposer.test.ts
?? frontend/src/components/TaskManagementPage.tsx
?? frontend/src/components/TaskProjectPicker.tsx
?? frontend/wailsjs/
~~~

因此，阶段 0 文档先在独立临时目录生成。落入仓库、建立功能分支和发布前必须重新确认基线 commit，并使用独立 Git worktree。

## 2. 工具链事实

| 项目 | 当前值 | 证据 |
|---|---|---|
| Go | go1.25.6 darwin/arm64 | go version |
| Node.js | v23.7.0 | node --version |
| pnpm | 11.1.2 | pnpm --version；frontend/package.json 期望 11.7.0 |
| Wails | v2.13.0 | wails version；go.mod 使用 Wails v2 |
| SQLite | modernc.org/sqlite v1.37.1 | go.mod |
| 原生 PI | 0.82.1 | /opt/homebrew/bin/pi；pi --version |
| PI 包 | @earendil-works/pi-coding-agent | 安装包 package.json |

pnpm 的实际版本低于仓库声明版本，但本次 typecheck、测试和构建均通过。后续 CI 应继续以仓库约定的 Node 20+、pnpm 11.7 为准。

## 3. 当前架构与能力

### 3.1 桌面边界

- main.go 和 app.go 是 Wails 入口。
- App.startup 打开 SQLite，并调用 SQLiteStore.ReconcileTaskContexts。
- App.shutdown 目前只关闭 SQLite；没有 Agent 进程注册表、取消或退出等待。
- App.LoadState、App.SaveState、App.ClearState 暴露整个 Zustand 快照的持久化。
- App.ValidateTransition 调用 internal/workflow.ValidateTransition，保留单阶段流转和人工门禁。
- 当前唯一流式 Wails 事件是 daily-report:generation-progress；frontend/src/lib/bridge.ts 的订阅函数返回清理回调。

### 3.2 数据持久化

- internal/storage/sqlite_store.go 的 workspace_state 仍保存完整 Zustand JSON。
- initialSchema 以一段 CREATE TABLE IF NOT EXISTS SQL 同时写入 schema_migrations 版本 1、2；它不是按版本逐步执行的迁移 runner。
- 540aade 新增 task_context_settings，并在 Save 时把活动任务和回收站任务物化到任务资料根目录。
- internal/storage/task_contexts.go 当前目录形态是：

~~~text
<root>/<task-id>/
├── .btaskassistant-task.json
├── context.json
├── files/
└── images/
~~~

- task_contexts.go 使用 os.Root、Lstat、任务 ownership marker、临时同目录文件和 Rename，已有任务 ID、目录碰撞、符号链接和 data URL 去重测试。
- 当前形态不是已锁定的 Task Workspace 目标树；没有 .btask/manifest.json、context/、sources/、attachments/、artifacts/、repos/、runs/。
- 现有 Save 会先同步文件系统，再更新 workspace_state；旧目录清理、归档和不可逆迁移策略仍需明确。
- Agent 消息、事件、运行、工具、授权和 Git 绑定均没有独立表。

### 3.3 工作流

- internal/workflow/machine.go 定义 inbox → requirements → approved → development → review → done。
- ValidateTransition 阻止跨阶段跳转，并检查 requirementsConfirmed、developmentCompleted、reviewApproved。
- 任务状态和 development.state 已经是两个概念，可作为“任务状态与 Agent 状态分离”的基础。
- 新工作台不得自动调用状态迁移，也不得把 agent_settled 等同于任务完成或审核通过。

### 3.4 现有 AI 接入

- internal/engine.Adapter 只有一次性 Run 抽象，没有会话、事件或工具模型。
- internal/engine/adapter.go 的 PI 状态实际检查 omp，UI 标签是 PI / oh-my-pi。
- internal/engine/omp.go 的 OMPAnalyzer 直接执行 omp。
- internal/engine/requirements.go 的 analystCommand("pi") 返回 omp，并使用 --no-rules、--no-session、--no-lsp、--no-pty、--print 等 OMP 假设。
- internal/engine/daily_report.go 复用同一路径。
- internal/engine/pi_settings.go 的 --thinking 和 --max-time 参数属于现有 OMP 调用合同，不能直接传给原生 PI RPC。
- app.go 的 AnalyzePlaneCandidate 直接构造 engine.OMPAnalyzer。
- README.md、docs/SOFTWARE_ARCHITECTURE.md、测试 fake binary 和浏览器 Mock 也包含 OMP 历史引用。

现有需求分析和日报属于一次性固定输入任务。迁移时可继续保留其产品能力，但底层必须改为原生 PI 或明确保留 Codex；不能只改显示名称。

### 3.5 前端

- frontend/src/store/workspace.ts 把任务、候选项、PI 设置、日报和选择状态保存在一个 persist store。
- frontend/src/lib/bridge.ts 在桌面端调用 App，在浏览器端使用 localStorage/Mock。
- frontend/src/components/ManualTaskDetail.tsx 是当前右侧详情区，没有 TaskAgentWorkbench。
- 当前没有 Agent session 列表、消息分页、流式 reducer、权限卡、工具卡、运行面板或 Git diff 面板。
- 当前未提交 UI 修改正在拆分 TaskManagementPage、TaskBasicInfoDialog 和 TaskProjectPicker；本计划不假设这些修改已完成。

## 4. 当前能力与目标差距

| 目标能力 | 当前状态 | 缺口 |
|---|---|---|
| 任务独立物理空间 | 部分具备 | 目录合同、manifest、只读系统区、artifacts、runs、repos 未实现 |
| 原生 PI RPC 长会话 | 不具备 | 当前调用 omp 单次命令 |
| 稳定内部事件协议 | 不具备 | 只有日报进度事件 |
| Session / Run 分离 | 不具备 | 无相关领域模型和表 |
| 增量消息持久化 | 不具备 | 仍是完整 Zustand JSON |
| 权限与审计 | 不具备 | 无 BTask 持有的策略、授权、撤销和审计 |
| Task worktree | 不具备 | 只可选择项目目录；无隔离 worktree |
| 文件与 Diff | 不具备 | 只有任务资料物化，无安全浏览和 Git diff |
| Ask / Plan / Agent | 不具备 | 只有 Requirement analyst 与 DevelopmentRecord |
| 恢复 | 很有限 | SQLite 和 task context 可恢复；无 PI/run/tool 恢复 |
| 浏览器 Mock | 部分具备 | 现有 bridge 有浏览器回退；无 Agent Mock |
| 人工状态门禁 | 已具备 | 必须保持，不与 Agent 状态合并 |

## 5. 原生 PI 基线结论

- 可执行文件：/opt/homebrew/bin/pi。
- 实际包：@earendil-works/pi-coding-agent 0.82.1，MIT。
- 目标启动模式存在：pi --mode rpc。
- 官方本机文档：/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/rpc.md。
- 严格 JSONL 只以 LF 分帧；U+2028/U+2029 可合法出现在 JSON 字符串中。
- 没有 ready 帧或协议协商命令。可用性应由“进程仍存活 + 带 id 的 get_state 成功响应”确定。
- get_entries 支持稳定 entry id 增量游标；get_messages 是当前投影，不替代持久增量历史。
- agent_settled 是本轮无自动重试、压缩重试或队列续跑的最终信号。
- Extension tool_call 可在工具执行前阻止；RPC Extension UI 的 select、confirm、input、editor 会阻塞等待匹配 response。
- PI 没有内置沙箱；工具和 Extension 以启动 PI 的用户权限运行。
- AGENTS.md 和 CLAUDE.md 不受项目 trust 拒绝保护，除非显式 --no-context-files。
- 默认会发现全局/项目 extensions、skills、prompts、themes、settings 和 packages；任务级运行必须显式隔离。

完整合同见 PI_RPC_CONTRACT.md。

## 6. Reasonix 参考审计

参考源码：/Users/blue/Downloads/DeepSeek-Reasonix-main-v2，许可证 MIT。

| 参考文件 | 可借鉴内容 | 不直接复制的原因 |
|---|---|---|
| internal/event/event.go | typed event、toolCallId、approval、turn done | 事件字段需改为 task/session/run 作用域 |
| internal/eventwire/wire.go | 内部事件到稳定 wire DTO | 不能暴露 Reasonix 业务枚举 |
| internal/permission/permission.go | deny > ask > allow > fallback 的纯策略 | Ask 无交互 approver 的放行语义与 BTask fail-closed 冲突 |
| internal/permission/bash_approval.go | shell 分解、间接命令与重定向风险识别 | 范围过大；阶段 4 只抽取必要分类和测试 |
| internal/worktree/worktree.go | 从 committed HEAD 创建持久 worktree、记录 source dirty、从不自动删除 | 分支命名、managed root、任务状态合同不同 |
| internal/diff/diff.go | 文本/二进制识别、diff 大小上限、行数统计 | BTask 可优先使用 git diff，并只复用纯 diff 需要的最小部分 |
| internal/fileutil/atomicwrite.go | 同目录临时文件、fsync、原子替换 | 需与 os.Root 路径边界组合 |
| desktop/tabs.go | 非阻塞、有序 Wails 事件发送 | BTask 需要有界队列与落盘恢复，不能无限增长 |
| desktop/workspace_changes.go | Git status + session 变更合并、2 MiB bridge 限制 | BTask 只看任务 worktree，不混用原始工作区 |
| desktop/frontend/src/lib/useController.ts | 事件 reducer、epoch 防过期事件、订阅清理 | 状态模型和 Wails API 必须重新定义 |
| ApprovalModal.tsx / ToolCard.tsx | 权限选择、工具状态、懒加载大输出 | UI 样式和高风险授权规则不同 |

若实际复制非平凡代码，必须保留 Reasonix MIT 许可证和可追溯 attribution；优先重新实现小而明确的模块。

## 7. 基线验证

| 命令 | 结果 |
|---|---|
| cd frontend && pnpm typecheck | 通过 |
| cd frontend && pnpm test | 独立功能 worktree 通过，15 个测试文件、87 个测试；源脏工作区初始审计为 17/96，额外项目来自未提交 UI 测试 |
| cd frontend && pnpm build | 通过；仅有大 chunk 警告，RichMarkdownEditor 约 1.17 MB |
| go test ./... | 默认受限环境因 httptest 监听 ::1 返回 EPERM；在获准的非沙箱执行中全部通过 |

前端 build 删除了已跟踪的 frontend/dist/.gitkeep，已用补丁恢复；最终 git status 仅包含 8 份阶段 0 文档，没有将验证副作用写入索引。

## 8. 必须锁定的决策

以下决策进入实施合同：

1. 新功能分支使用 fet/task-scoped-pi-agent-workbench，并从用户确认的 committed HEAD 建独立 worktree。
2. TaskDataRoot 继续由现有任务资料设置提供，但阶段 1 迁移为锁定目录树；旧 context.json/files/images 只复制/导入，不原地删除。
3. 原生 PI Session 存入每任务 .btask/pi-sessions；SQLite 只保存索引、状态和外部 session path。
4. 默认不继承用户全局 PI 配置。每任务使用独立 PI_CODING_AGENT_DIR；模型凭据必须通过明确设置提供，不自动复制 ~/.pi/agent。
5. 第一版使用 BTask 自有 custom tools，禁用 PI built-in writer/bash；Extension 加载和版本握手失败时 writer 能力全部失败关闭。
6. 这是进程内软门禁，不是操作系统强沙箱。第一版不承诺容器/VM 隔离。
7. RPC 单帧上限由 BTask 设为 16 MiB；超过后终止该 PI 进程并记录可诊断错误。PI 0.82.1 自身未声明帧上限。
8. 高风险外部副作用始终逐次确认，不能被 session/task/permanent grant 覆盖。
9. Agent 完成只更新 agent/run 状态，不自动推进任务状态、不批准需求、不提交、不 push。
10. 阶段 0 文档经人工确认后才进入阶段 1。

## 9. 当前阻塞与人工确认点

- 必须确认功能分支基线：540aade 是否是阶段 0 和后续实现的起点。
- 必须确认当前未提交任务管理 UI 修改由谁继续维护；独立 worktree 不会带入这些修改。
- 必须确认默认 PI 凭据策略：推荐任务隔离配置目录 + 用户显式选择/录入模型凭据，不静默继承全局 auth.json。
- 必须确认阶段 0 的 8 份文档后才能开始阶段 1。

在这些确认前，可以完成文档草案和静态自检，但不应开始业务实现。
