# 任务级原生 PI Agent 工作台实施计划

## 1. 执行合同

本计划基于：

- BTaskAssistant HEAD 540aade。
- 原生 PI @earendil-works/pi-coding-agent 0.82.1。
- 参考 Reasonix MIT 源码，但不复制其业务架构或 fail-open 权限语义。

阶段依赖严格为：

~~~text
0 → 1 → 2 → 3 → 4 → 5 → 6 → 7
~~~

每阶段：

1. 重新确认 HEAD、branch、remote、git status。
2. 只修改该阶段范围。
3. 执行阶段专项测试和全量 Go/前端检查。
4. 检查 diff、敏感信息和无关文件。
5. 验证成功后创建一个中文 Conventional Commit。
6. 按用户总指令推送当前功能分支。
7. 汇报证据并停止，获得人工确认后进入下一阶段。

需求包内各阶段提示词写有“不要 push”，但当前用户明确要求每阶段验证后提交并推送；以当前用户指令为交付方式。产品本身仍不得自动 push。

验证失败时不创建“阶段完成”提交、不 push，不掩盖失败。

## 2. 开发工作区

当前 /Users/blue/job/BTaskAssistant 有无关未提交 UI 修改。阶段 0 获得确认后执行：

~~~text
git worktree add \
  /Users/blue/job/BTaskAssistant-pi-agent-workbench \
  -b fet/task-scoped-pi-agent-workbench \
  <CONFIRMED_BASELINE>
~~~

建议 CONFIRMED_BASELINE=540aade，但必须由用户确认。

规则：

- 不复制源工作区未提交内容。
- 不 reset、stash、clean 或暂存源工作区。
- 所有本任务提交在独立 worktree。
- 每次 push 前 fetch 远端引用，检查 local/remote SHA 和 ahead/behind。
- 不创建 PR、不合并，除非用户另行要求。

## 3. 目标架构

### 3.1 后端模块

~~~text
internal/taskspace/
  service.go
  manifest.go
  resolver.go
  context.go
  resources.go
  artifacts.go

internal/agent/
  supervisor.go
  session.go
  pi_process.go
  rpc_client.go
  rpc_protocol.go
  lf_decoder.go
  event_mapper.go
  recovery.go
  extensions/btask-gate.ts

internal/permissions/
  policy.go
  resolver.go
  path.go
  command.go
  audit.go

internal/gitrepo/
  binding.go
  worktree.go
  status.go
  diff.go

internal/execution/
  service.go
  process.go
  output.go
  recovery.go

internal/storage/
  migrations.go
  task_workspace_repository.go
  agent_repository.go
  permission_repository.go
  git_repository.go
~~~

只在实际阶段需要时创建文件。一次性小逻辑不额外抽象。

### 3.2 App 生命周期

App 新增依赖：

- taskspace.Service
- agent.Supervisor
- permissions.Service
- gitrepo.Service
- execution.Service

startup：

1. 打开 DB 和运行 migration。
2. reconcile Task Workspace。
3. 把遗留 running/pending 状态改为 interrupted/expired。
4. 不自动启动 PI；用户打开/恢复 Session 时启动。

shutdown：

1. 停止接收新 prompt。
2. 取消 pending permission。
3. abort 活动 run。
4. 关闭 stdin，等待后按 SIGTERM/SIGKILL 升级。
5. flush event/message/run logs。
6. 关闭 DB。

### 3.3 Wails Bridge

最终能力：

- EnsureTaskWorkspace
- CreateAgentSession / StartAgentSession / ResumeAgentSession
- SubmitAgentPrompt / SteerAgent / FollowUpAgent / StopAgent
- ResolveAgentPermission / RevokePermissionGrant
- ListAgentSessions / GetAgentHistoryPage
- ListTaskWorkspaceFiles / ReadTaskWorkspaceFile
- ListTaskArtifacts
- BindGitRepository / GetTaskGitStatus / GetTaskFileDiff
- ListExecutionRuns / ReadExecutionOutput

所有方法先验证 task/session/run 关系。返回稳定 DTO，不返回 sql row、os.File 或 PI raw JSON。

Wails 事件统一为 agent:event，合同见 EVENT_PROTOCOL.md。

### 3.4 前端

~~~text
frontend/src/
├── components/agent/
│   ├── TaskAgentWorkbench.tsx
│   ├── AgentHeader.tsx
│   ├── AgentTimeline.tsx
│   ├── AgentComposer.tsx
│   ├── AgentToolCard.tsx
│   ├── AgentPermissionCard.tsx
│   ├── AgentContextPanel.tsx
│   ├── AgentFilesPanel.tsx
│   ├── AgentChangesPanel.tsx
│   └── AgentRunsPanel.tsx
├── domain/agent.ts
├── store/agent.ts
└── lib/agentBridge.ts
~~~

Agent store 独立于 workspace persist store，使用按 task/session 分片的轻量投影。完整历史、工具大输出和文件内容按需从 Go 读取。

## 4. 阶段 0：审计与计划锁定

### 范围

只新增：

- docs/pi-agent-workbench/BASELINE_AUDIT.md
- PI_RPC_CONTRACT.md
- PI_MIGRATION_FROM_OMP.md
- IMPLEMENTATION_PLAN.md
- DATA_MODEL.md
- EVENT_PROTOCOL.md
- PERMISSION_MODEL.md
- TEST_PLAN.md

不修改业务代码。

### 验证

- 8 文件存在且相互一致。
- 引用真实仓库文件、函数和 PI 0.82.1 本机证据。
- 执行 go test ./...、pnpm typecheck、pnpm test、pnpm build。
- 检查只包含 docs/pi-agent-workbench。

### 提交

建议：

~~~text
docs(PI工作台): 锁定原生 PI 分阶段实施计划
~~~

验证后推送 fet/task-scoped-pi-agent-workbench。

### 人工确认

必须确认：

1. baseline 是否为 540aade。
2. 当前源工作区未提交 UI 修改不纳入本分支。
3. 每任务隔离 PI config，默认不继承 ~/.pi/agent。
4. 第一版为软权限边界。
5. 16 MiB RPC frame cap。
6. 8 份计划文档允许进入阶段 1。

### 回滚

只需 revert 文档 commit；无数据影响。

## 5. 阶段 1：Task Workspace 与数据基础

### 依赖

阶段 0 commit 已确认并推送。

### 实现

1. 把 SQLiteStore 改为真实 migration runner。
2. migration v3 创建基础表和索引。
3. 新增 taskspace Service 和 locked directory tree。
4. manifest、resources/sessions/permissions mirror 原子写入。
5. 生成 context/task.md、requirements/current.md、approved-vN.md、acceptance-criteria.md。
6. 导入 manual/plane/chat/project-observation 来源。
7. 导入 data URL 和文件 evidence 到 attachments，保留 hash。
8. 旧 context.json/files/images 做 copy-first 幂等迁移，不删除。
9. 增加安全文件 list/read backend。
10. App/bridge 暴露 Ensure/List/Read。
11. PI engine 状态改探测原生 pi，UI 标签和设置文案迁移。
12. 旧 engine 字符串兼容只做读时映射，不启动 PI。

### 预计修改

- internal/storage/sqlite_store.go、migrations.go 和 repository files。
- internal/taskspace/**。
- app.go、app_test.go。
- frontend/src/domain/engine.ts。
- frontend/src/lib/bridge.ts 或新增 agentBridge 基础。
- PISettingsPage、TaskContextSettings 及聚焦测试。
- README 只做当前 PI 身份最小修正；完整文档留阶段 7。

不改聊天 UI、Shell、Git worktree 或权限卡。

### 数据

- schema 2 → 3。
- 旧 workspace_state 保留。
- 每任务写 legacy_task_migrations 记录。

### 验证

按 TEST_PLAN 阶段 1；特别断言 path/symlink、两任务隔离、旧 data URL 和 migration 幂等。

### 人工确认

- TaskDataRoot 和新目录布局。
- 旧 context 迁移只复制不删除。
- PI 标签/路径正确。

### 回滚

- revert 代码不会删除 v3 表；旧应用会忽略额外表。
- 旧 context 文件保留。
- 新 Task Workspace 标记未完成，不自动清理。

## 6. 阶段 2：原生 PI RPC 最小会话

### 依赖

Task Workspace、agent_sessions/messages/events/runs repository 可用。

### 实现

1. internal/agent Supervisor、process、strict LF decoder、request correlator。
2. get_state probe，无 ready/negotiation。
3. task .btask/pi-sessions。
4. prompt、abort、new_session、switch_session、get_entries。
5. PI raw → BTask v1 events。
6. bounded stderr/stdout 和 process shutdown。
7. Session/Run/message/event 增量持久化。
8. app.go 方法和 agent:event。
9. 最小 TaskAgentWorkbench：Session、消息流、发送、停止、错误。
10. browser Mock。
11. 固定 utility analysis 改为无工具 PI RPC。
12. 移除生产运行时 OMP 查找/执行和 OMP fake。

工具调用只显示 raw-safe 状态或禁用；不提供文件写、Shell 和审批。

### 预计修改

- internal/agent/**、internal/execution 基础。
- internal/engine analyzer/requirements/daily_report 适配。
- app.go/main lifecycle。
- frontend/src/domain/agent.ts、store/agent.ts、lib/agentBridge.ts。
- components/agent 最小组件。
- App.tsx 只接入右侧工作台入口，保留基本信息编辑。

### 数据

使用 v3 表，不新增 schema；如实现发现字段缺失，先更新 DATA_MODEL 并单独增加 v4 前置迁移，不能直接 ALTER 无记录。

### 验证

按 TEST_PLAN 阶段 2；真实 PI 无凭据 probe 必须通过。真实模型调用允许 external_validation_pending。

### 人工确认

- 最小聊天状态和错误显示。
- 确认已完全停止执行 omp。

### 回滚

- 关闭/移除 UI 入口后历史仍在增量表和 PI Session。
- 旧人工工作流可继续。
- 不回退到 omp。

## 7. 阶段 3：上下文、附件与 artifacts

### 依赖

稳定 Session、prompt images、taskspace resolver。

### 实现

1. 当前任务 context 装配。
2. @ resource index/search/select。
3. 粘贴图片、拖放文档、消息附件。
4. MIME/magic/size/count/hash。
5. 新增内嵌 btask-gate Extension 的最小版本：注册 BTask read/list/artifact custom tools，并完成 version/nonce heartbeat；只执行固定模式与路径策略，不提供通用审批。
6. Plan/Agent 写 artifacts 的 custom tool。
7. artifact list/preview/open。
8. 正式需求变更只写 artifacts/proposals。
9. 引用、附件和 proposal 增量存储。

不提供 Shell、Git write/diff 或任意外部 write。

### 数据

schema 3 → 4：

- message_attachments。
- resource_references。
- requirement_proposals。

### 前端

- AgentComposer 的 @、paste、drop。
- timeline attachment。
- Context/Files 面板第一版。

### 验证

按 TEST_PLAN 阶段 3；两个任务同名文件和 task switch 是阻断测试。

### 人工确认

- 文件/图片预览。
- artifacts 生成和 proposal 保护。

### 回滚

- v4 表和文件保留。
- 禁用引用/写工具不会破坏聊天读取。
- 不自动删除导入附件。

## 8. 阶段 4：权限与工具审批

### 依赖

custom tools 和稳定 tool event 已存在。

### 实现

1. internal/permissions pure policy、path/command/Git classifier。
2. 把阶段 3 的 BTask gate Extension 从固定安全工具升级为完整审批协议；继续由 Go embed、原子物化并校验 hash/version/nonce。
3. Extension tool_call → RPC UI → Go permission service。
4. once/session/task/permanent grant、deny、timeout、revoke。
5. critical always-once。
6. request/grant/tool receipt 审计。
7. tool start/update/end 稳定映射。
8. PermissionCard/ToolCard。
9. 大输出 ref 和懒加载。

不实际开放 Git writer、Shell 或 push。

### 数据

使用 v3 permission/tool 表；migration 可增加索引但不改变已锁定语义。

### 验证

按 TEST_PLAN 阶段 4；gate 缺失和拒绝后未执行是阻断测试。

### 人工确认

- 作用域文案和默认按钮。
- 软边界说明清楚。
- critical 只有 once。

### 回滚

- 禁用 gate 时所有工具失败关闭。
- grants 保留但不生效。
- revert UI 不改变审计数据。

## 9. 阶段 5：Git worktree 与开发能力

### 依赖

权限和 custom tool receipt 已验证。

### 实现

1. repo binding inspect。
2. 从 committed baseline 创建 task repos/<repo> durable worktree。
3. 记录 source dirty，但不复制或修改 source worktree。
4. branch/baseline/state 恢复。
5. worktree read/write/edit custom tools。
6. BTask shell tool、process group、stop、stdout/stderr。
7. status 和逐文件 diff。
8. 可选 local commit 专用动作。
9. worktree missing/cleanup failure 恢复。
10. Changes/Runs 面板。

Reasonix worktree 和 diff 只借鉴最小纯逻辑；若复制代码，附 MIT attribution。

不提供自动 push、PR、merge、remote delete。

### 数据

使用 v3 git_bindings、execution_runs、tool_calls。必要索引通过下一 migration 明确记录。

### 验证

按 TEST_PLAN 阶段 5；source worktree 前后 git status 完全一致为阻断项。

### 人工确认

- 两任务/同仓库两个 worktree。
- diff 和长命令停止。
- local commit 是明确动作。

### 回滚

- worktree 永不自动删除。
- revert 代码后 worktree/branch 仍可由 Git 手工访问。
- DB binding 保留诊断信息。

## 10. 阶段 6：完整 Task Agent Workbench

### 依赖

后端 Session、资源、权限、Git、Run 能力稳定。

### 实现

1. 用 TaskAgentWorkbench 替换右侧仅 ManualTaskDetail 的布局。
2. 顶栏：Task/Agent 状态、PI、模型、Session、模式、workspace。
3. Ask/Plan/Agent。
4. new/switch Session。
5. 完整 timeline 和所有 card。
6. multi-line composer、send/follow-up/steer/stop。
7. Context/Files/Changes/Runs 四面板。
8. history cursor pagination 或 virtualization。
9. empty/error/recovery。
10. 窄窗口。
11. 保留 TaskBasicInfoDialog。

不复制 Codex/Reasonix 无关功能，不改变 Task 状态机。

### 数据

无新业务表。若 UI 偏好需要持久化，只保存小型 panel/session selection，不存历史内容。

### 验证

按 TEST_PLAN 阶段 6；Wails 桌面人工 UI 验收是阶段完成条件。

### 人工确认

- 需求红框区域。
- 基本信息仍可编辑。
- 状态和实际执行一致。

### 回滚

- 保留 ManualTaskDetail 作为组件直到本阶段验收；回退入口不删除 Agent 数据。
- 验收后再清理仅由新布局取代的孤儿样式/import，不重构相邻 UI。

## 11. 阶段 7：恢复、安全、迁移与最终验收

### 依赖

功能阶段 1-6 均已确认。

### 实现

1. 故障注入发现的最小修复。
2. app/process/Session/run/tool 恢复。
3. invalid/truncated/oversize frame。
4. DB lock/disk failure。
5. legacy migration v5 和诊断。
6. worktree/repo missing。
7. security regression。
8. performance limits。
9. 当前文档、ADR 和用户手册。
10. FINAL_VALIDATION.md。

必须更新：

- README.md
- docs/ARCHITECTURE.md
- docs/SOFTWARE_ARCHITECTURE.md
- docs/PI_AGENT_WORKBENCH.md
- docs/PERMISSIONS.md
- docs/DATA_MIGRATION.md
- docs/pi-agent-workbench/FINAL_VALIDATION.md

### 数据

schema 4 → 5：

- legacy_task_migrations 完成状态。
- 已验证必要的约束/索引。
- 不删除旧 workspace_state 或 legacy files。

### 验证

- TEST_PLAN 阶段 7。
- 04-最终验收清单.md 全部可自动化项目。
- 全量 Go/前端命令。
- 当前源工作区和用户仓库不变。
- 敏感信息扫描。

### 人工确认

填写最终验收结论，包括功能、安全、迁移、Git 隔离、UI 和是否允许合并。

### 回滚

- 数据迁移全部 forward-only、copy-first。
- 功能代码可 revert，新增表/Task Workspace/PI Sessions/worktrees 保留。
- 没有用户确认不执行物理清理。

## 12. 阶段提交建议

| 阶段 | 建议 commit |
|---:|---|
| 0 | docs(PI工作台): 锁定原生 PI 分阶段实施计划 |
| 1 | feat(任务空间): 建立任务级资料目录与增量数据基础 |
| 2 | feat(PI工作台): 接入原生 PI RPC 最小会话 |
| 3 | feat(PI工作台): 支持任务上下文附件与文档产物 |
| 4 | feat(权限): 增加 PI 工具审批与审计 |
| 5 | feat(Git): 支持任务级 worktree 与开发运行 |
| 6 | feat(PI工作台): 完成任务级聊天工作台 |
| 7 | fix(PI工作台): 完成恢复迁移与安全验收 |

实际摘要必须以当阶段 diff 为准，不能照抄未实现内容。

## 13. 第一版明确不做

- 多 Agent 或子 Agent。
- 自动 PR、自动 merge、自动 push、自动发布。
- 修改用户原始工作区。
- 远程云端 Session 同步。
- 操作系统级 sandbox 承诺。
- 自动推进 Task 状态。
- 自动批准需求或覆盖 approved revision。
- 通用浏览器自动化。
- OMP 兼容运行时。
- 全量重写现有任务领域或 Zustand。
- 为未来可能性添加 plugin/factory 抽象。

## 14. 阻断条件

遇到以下情况停止该阶段并请求用户：

- baseline/dirty changes 归属不明且会影响本阶段。
- TaskDataRoot 或旧数据需要破坏性移动/删除。
- PI 0.82.1 实际协议与合同不一致。
- gate 无法 fail closed。
- 需要读取/复制用户全局 PI 凭据。
- 需要修改原始 Git worktree。
- 验证失败或环境缺失导致无法证明阶段完成。
- 新产品范围、自动外部副作用或强 sandbox 要求。
