# Task Workspace 与增量数据模型

## 1. 权威来源

第一版采用明确的分层权威：

| 数据 | 权威来源 | 说明 |
|---|---|---|
| 现有 Task 业务字段 | workspace_state JSON | 过渡期兼容现有 Zustand；本任务不顺带重写全部任务领域 |
| Task Workspace 文件字节 | 文件系统 | sources、attachments、artifacts、run logs 和 worktree |
| Workspace 身份和 schema | .btask/manifest.json + task_workspaces | 两者 revision 对账；冲突时失败关闭 |
| Session/message/event/run/tool | SQLite 增量表 | 不写回完整 Zustand snapshot |
| 权限决策 | SQLite permission 表 | permissions.json 只是可恢复镜像，绝不单独作为放行依据 |
| PI Session | .btask/pi-sessions 中的原生 PI 文件 | SQLite 保存 external path、PI session id 和 cursor |
| Git 内容 | task repos/ 下的 Git worktree | git_bindings 保存来源、baseline、branch 和状态 |

.btask/resources.json、permissions.json、sessions.json 是原子更新的诊断/恢复镜像。数据库不可用时可展示只读信息，但不能从这些镜像自动恢复授权或执行状态。

## 2. Task Workspace

固定路径：

~~~text
<TaskDataRoot>/tasks/<task-id>/
├── .btask/
│   ├── manifest.json
│   ├── permissions.json
│   ├── resources.json
│   ├── sessions.json
│   ├── pi-agent/
│   └── pi-sessions/
├── context/
│   ├── task.md
│   ├── requirements/
│   │   ├── current.md
│   │   └── approved-v<N>.md
│   └── acceptance-criteria.md
├── sources/
│   ├── manual/
│   ├── plane/
│   ├── chats/
│   └── project-observations/
├── attachments/
│   ├── images/
│   └── documents/
├── artifacts/
│   ├── plans/
│   ├── reports/
│   ├── proposals/
│   └── exports/
├── repos/
│   └── <repo-name>/
└── runs/
    └── <execution-run-id>/
        ├── events.jsonl
        ├── stdout.log
        ├── stderr.log
        └── result.json
~~~

.btask/pi-agent 和 .btask/pi-sessions 是原生 PI 隔离运行所需的实现目录；它们不改变锁定的用户文件区。

### 2.1 manifest.json

最低字段：

~~~json
{
  "schemaVersion": 1,
  "taskId": "task_xxx",
  "workspaceId": "uuid",
  "createdAt": "RFC3339",
  "updatedAt": "RFC3339",
  "revision": 1,
  "engine": "pi",
  "resourcePolicy": "isolated",
  "managedBy": "BTaskAssistant"
}
~~~

- taskId 必须与目录名和 task_workspaces.task_id 完全一致。
- workspaceId 创建后不变。
- revision 只由 BTask 在成功原子更新后增加。
- 不存 token、PAT、API key 或授权 secret。

### 2.2 系统区写入规则

- context、sources、attachments 仅由 Task Workspace 服务写。
- artifacts 只能通过受控 artifact tool 写。
- repos 只能通过任务 worktree 工具写。
- runs 只能由 execution service 写。
- 所有写入使用同目录临时文件、fsync、原子替换。
- 目录创建权限默认 0700，文件默认 0600；导出时再按用户动作调整。

## 3. SQLite migration runner

当前 internal/storage/sqlite_store.go 的 initialSchema 同时 CREATE TABLE 并 INSERT migration 1、2，不足以支持真实增量迁移。

阶段 1 改为：

1. 打开连接后先执行 PRAGMA journal_mode=WAL、foreign_keys=ON、busy_timeout=5000。
2. 单独 bootstrap schema_migrations。
3. 按版本升序执行 migrations 列表。
4. 每个版本使用 BEGIN IMMEDIATE，在同一 transaction 内执行 DDL、数据转换和 INSERT schema_migrations。
5. 已存在版本跳过；同一数据库重复 Open 不产生变化。
6. 失败回滚该版本，保留 workspace_state 和旧任务数据。

版本分配：

| 版本 | 阶段 | 内容 |
|---:|---|---|
| 1 | 既有 | workspace_state |
| 2 | 既有 | task_context_settings |
| 3 | 阶段 1 | task_workspaces、resources、agent/session/event/run/tool、permission、git、artifact 基础表，以及 copy-first 所需的 legacy_task_migrations |
| 4 | 阶段 3 | message_attachments、resource_references、requirement_proposals |
| 5 | 阶段 7 | 最终索引、状态约束和升级兼容修补 |

既有数据库已经记录 1、2 时，runner 不重放；新数据库从 1 依次执行。迁移测试必须覆盖“旧 initialSchema 数据库 → 最新”和“空数据库 → 最新”。

阶段 1 必须立即记录每个旧任务的迁移结果，因此 legacy_task_migrations 随 v3 创建；阶段 7 不再补建该表，只允许在不丢失 v3 记录的前提下加固索引和约束。

## 4. 表定义

以下为字段合同；具体 SQL 名称可保持 snake_case。

### 4.1 task_workspaces

| 字段 | 类型/约束 |
|---|---|
| task_id | TEXT PRIMARY KEY |
| workspace_id | TEXT NOT NULL UNIQUE |
| root_path | TEXT NOT NULL UNIQUE |
| schema_version | INTEGER NOT NULL |
| manifest_revision | INTEGER NOT NULL DEFAULT 1 |
| state | TEXT NOT NULL：ready、legacy、error、archived |
| legacy_context_path | TEXT NULL |
| created_at | TEXT NOT NULL |
| updated_at | TEXT NOT NULL |
| last_reconciled_at | TEXT NULL |
| error_message | TEXT NULL |

Task 表仍在 workspace_state，因此 task_workspaces 是新表的 task scope anchor。所有 Agent 相关表通过 task_id 外键引用它。

### 4.2 task_resources

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id | TEXT NOT NULL REFERENCES task_workspaces ON DELETE RESTRICT |
| kind | TEXT：source、attachment、external、context |
| source_type | TEXT：manual、plane、chat、project_observation、image、document 等 |
| logical_path | TEXT NOT NULL |
| storage_path | TEXT NULL |
| external_path | TEXT NULL |
| mime_type | TEXT NULL |
| byte_size | INTEGER NULL |
| sha256 | TEXT NULL |
| immutable | INTEGER NOT NULL |
| readable | INTEGER NOT NULL |
| created_at | TEXT NOT NULL |
| removed_at | TEXT NULL |

唯一约束为 task_id + logical_path + active 状态。external_path 只保存用户明确绑定资源，读取时仍重新 realpath 校验。

### 4.3 agent_sessions

最低字段：

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id | TEXT NOT NULL REFERENCES task_workspaces |
| engine | TEXT NOT NULL CHECK engine='pi' |
| external_session_path | TEXT NULL |
| external_session_id | TEXT NULL |
| title | TEXT NOT NULL |
| mode | TEXT NOT NULL：ask、plan、agent |
| model | TEXT NULL |
| thinking_level | TEXT NULL |
| resource_policy | TEXT NOT NULL |
| state | TEXT NOT NULL：created、starting、idle、running、stopping、interrupted、failed、closed |
| last_entry_id | TEXT NULL |
| last_sequence | INTEGER NOT NULL DEFAULT 0 |
| created_at | TEXT NOT NULL |
| updated_at | TEXT NOT NULL |
| last_active_at | TEXT NOT NULL |
| error_message | TEXT NULL |

同一 task_id 使用 partial unique index，保证 starting/running/stopping 最多一行。

### 4.4 agent_messages

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id | TEXT NOT NULL |
| session_id | TEXT NOT NULL REFERENCES agent_sessions |
| run_id | TEXT NULL REFERENCES execution_runs |
| role | TEXT NOT NULL：user、assistant、tool、system |
| kind | TEXT NOT NULL：text、reasoning、tool_call、tool_result、notice |
| status | TEXT NOT NULL：pending、streaming、complete、error、cancelled |
| content | TEXT NULL |
| content_ref | TEXT NULL |
| sequence | INTEGER NOT NULL |
| pi_entry_id | TEXT NULL |
| created_at | TEXT NOT NULL |
| completed_at | TEXT NULL |

唯一约束：session_id + sequence。大内容放 runs/，content 只存可显示摘要，content_ref 指向受控相对路径。

### 4.5 agent_events

| 字段 | 类型/约束 |
|---|---|
| event_id | TEXT PRIMARY KEY |
| version | INTEGER NOT NULL |
| task_id | TEXT NOT NULL |
| session_id | TEXT NOT NULL |
| run_id | TEXT NULL |
| tool_call_id | TEXT NULL |
| sequence | INTEGER NOT NULL |
| kind | TEXT NOT NULL |
| payload_json | TEXT NOT NULL |
| payload_ref | TEXT NULL |
| occurred_at | TEXT NOT NULL |

唯一约束：session_id + sequence。payload_json 有大小上限，超出后存摘要和 payload_ref。

### 4.6 execution_runs

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id | TEXT NOT NULL |
| session_id | TEXT NOT NULL |
| requirement_revision | TEXT NULL |
| git_binding_id | TEXT NULL |
| baseline_commit | TEXT NULL |
| mode | TEXT NOT NULL |
| state | TEXT NOT NULL：queued、running、waiting_permission、stopping、succeeded、failed、cancelled、interrupted |
| events_path | TEXT NOT NULL |
| stdout_path | TEXT NOT NULL |
| stderr_path | TEXT NOT NULL |
| result_path | TEXT NOT NULL |
| started_at | TEXT NOT NULL |
| finished_at | TEXT NULL |
| result_summary | TEXT NULL |
| error_message | TEXT NULL |

Run 表示一次前台 prompt 到 agent_settled 的执行边界；Session 可有多个 Run。

### 4.7 tool_calls

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id/session_id/run_id | TEXT NOT NULL |
| external_tool_call_id | TEXT NOT NULL |
| tool_name | TEXT NOT NULL |
| capability | TEXT NOT NULL |
| target | TEXT NULL |
| risk_level | TEXT NOT NULL |
| state | TEXT NOT NULL：received、waiting_permission、running、succeeded、failed、denied、cancelled |
| args_json | TEXT NULL |
| args_ref | TEXT NULL |
| output_summary | TEXT NULL |
| output_ref | TEXT NULL |
| is_error | INTEGER NOT NULL DEFAULT 0 |
| started_at/finished_at | TEXT |

唯一约束：session_id + external_tool_call_id。update 对同一条记录更新 preview，不插入重复 tool card。

### 4.8 permission_requests

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id/session_id/run_id/tool_call_id | TEXT NOT NULL |
| capability | TEXT NOT NULL |
| target | TEXT NOT NULL |
| normalized_target | TEXT NULL |
| subject | TEXT NOT NULL |
| risk_level | TEXT NOT NULL |
| state | TEXT NOT NULL：pending、allowed、denied、expired、cancelled |
| requested_at | TEXT NOT NULL |
| resolved_at | TEXT NULL |
| resolved_by | TEXT NULL |
| decision_scope | TEXT NULL |
| reason | TEXT NULL |

pending request 在 app 重启时一律转 expired；不能恢复为已允许。

### 4.9 permission_grants

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id | TEXT NULL |
| session_id | TEXT NULL |
| request_id | TEXT NULL；once scope 必填并引用 permission_requests |
| capability | TEXT NOT NULL |
| target_pattern | TEXT NOT NULL |
| scope | TEXT NOT NULL：once、session、task、permanent |
| decision | TEXT NOT NULL：allow、deny |
| risk_ceiling | TEXT NOT NULL |
| created_at | TEXT NOT NULL |
| expires_at | TEXT NULL |
| consumed_at | TEXT NULL |
| revoked_at | TEXT NULL |
| created_by | TEXT NOT NULL |

作用域约束：

- once 必须关联原 permission request，并在一次匹配后 consumed。
- session 必须有 session_id 和 task_id。
- task 必须有 task_id、session_id 为空。
- permanent 是应用级规则，但高风险能力仍不匹配 allow。
- deny 优先于 allow。

### 4.10 git_bindings

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id | TEXT NOT NULL UNIQUE |
| source_path | TEXT NOT NULL |
| source_real_path | TEXT NOT NULL |
| common_git_dir | TEXT NOT NULL |
| worktree_path | TEXT NULL UNIQUE |
| branch | TEXT NULL |
| baseline_commit | TEXT NOT NULL |
| source_branch | TEXT NULL |
| source_dirty_at_bind | INTEGER NOT NULL |
| state | TEXT NOT NULL：bound、creating、ready、missing、cleanup_failed、archived |
| created_at/updated_at | TEXT NOT NULL |
| error_message | TEXT NULL |

不保存远端凭据。worktree 删除不会级联删除 Session、Run 或审计。

### 4.11 workspace_artifacts

| 字段 | 类型/约束 |
|---|---|
| id | TEXT PRIMARY KEY |
| task_id/session_id/run_id | TEXT |
| logical_path | TEXT NOT NULL |
| kind | TEXT NOT NULL：plan、report、proposal、export |
| mime_type | TEXT NULL |
| byte_size | INTEGER NOT NULL |
| sha256 | TEXT NOT NULL |
| created_at | TEXT NOT NULL |
| updated_at | TEXT NOT NULL |
| deleted_at | TEXT NULL |

唯一约束为 task_id + logical_path + active 状态。文件写成功后才提交数据库行；数据库提交失败时保留可对账 orphan 标记，不假装成功。

### 4.12 阶段 3 扩展表

- message_attachments：task/session/message/resource 复合范围、position、created_at；只允许引用当前任务 task_resources。
- resource_references：task/session/message、resource_id、target_type（resource/artifact）、method（mention/attachment/generated）、position 和 created_at。多态目标在 repository transaction 内再次按 task_id 校验。
- requirement_proposals：task_id、artifact_id、base_revision、state、accepted_revision、created_at、resolved_at；阶段 3 只创建 pending，阶段 4 才能由人工动作决议。

v4 已由 migration runner 实现，空库和 v3 升级都会顺序执行；重复打开数据库不重放迁移。消息与引用、Session sequence 更新在同一 `BEGIN IMMEDIATE` transaction 中提交。

### 4.13 legacy_task_migrations

记录每个旧任务的 source_revision、legacy path、target workspace、state、warnings、started_at、completed_at。状态为 pending、running、completed、failed。失败后可重试，不删除旧数据。

## 5. 消息与事件写入

- user message 在 prompt 被 PI preflight 接受后标记 complete；预提交 UI bubble 可保持 pending。
- assistant message_start 创建 streaming row。
- 文本 delta 在内存聚合，每 100 ms 或累计 32 KiB 刷新一次，先到条件触发；message_end、abort、crash 前强制 flush。
- 每次 flush 更新同一 message content，不创建每 token 行。
- 稳定事件使用 append-only agent_events；message.delta 是同一 flush 粒度。
- tool output preview 有上限；完整输出写 runs/<run>/stdout.log 或独立 output 文件。
- SQLite transaction 内同时更新 message/tool/run 状态和 sequence，再由事件泵发送 Wails event。

## 6. 分页

GetAgentHistoryPage 使用 opaque cursor，内部包含 session_id、sequence、message_id 和方向，不使用 OFFSET。

- 首次默认返回最新一页。
- 加载更早内容用 exclusive before cursor。
- limit 默认 50，最大 200。
- 返回 nextCursor/hasOlder。
- UI 以 message id 去重。
- reasoning 和 tool 大输出按需读取，不随每页全量返回。

## 7. 路径和符号链接

所有入口统一经过 taskspace Resolver：

1. 拒绝空字节、绝对逻辑路径、卷名、UNC、.. segment 和非规范分隔。
2. 从已打开的 Task Workspace root 逐 segment Lstat。
3. 系统区不跟随符号链接。
4. worktree 读取可允许 Git 已跟踪 symlink 显示，但写入目标必须 realpath 后仍在 worktree；不存在的新文件校验最近存在父目录。
5. 外部资源使用创建绑定时和每次读取时的 realpath；移动/替换后失效并重新确认。
6. 路径授权存规范化 scope + 相对目标，不以字符串前缀判断。

Windows 需覆盖盘符大小写、反斜杠、UNC、保留名和 junction/reparse point；macOS/Linux 覆盖大小写差异和 symlink。

## 8. 生命周期

- 创建任务：保存现有 Task 成功后异步 EnsureTaskWorkspace；失败不丢任务，显示 workspace error。
- 回收站：workspace 只标 archived，不删除。
- 永久删除：必须人工确认；先检查活动 Session、Run、worktree 和未提交修改。失败保留全部数据并显示原因。
- 清理 worktree：独立动作，不等同于删除任务。
- 旧任务首次打开：幂等迁移；迁移完成前只读。
- TaskDataRoot 切换：复制、校验、切换指针三阶段；不原地搬移后立即删除旧根。

## 9. 不变量

- task_id、session_id、run_id 不可跨任务组合。
- 每个 task 至多一个活动 run。
- sequence 在 Session 内严格递增。
- permission allow 必须有审计来源。
- context/sources/attachments 不能被 Agent tool 写。
- agent_settled 不能更新 Task.status。
- Git worktree 路径不能等于或位于 source workspace 内。
- 凭据不能进入任何本表、manifest、事件 payload、run log 或前端 store。
