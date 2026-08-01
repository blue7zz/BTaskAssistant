# 数据迁移与恢复

## 1. 原则

任务级 PI 工作台使用 forward-only、copy-first、可重试的迁移策略：

- 先保留旧 `workspace_state` 和旧任务文件，再建立规范化索引和新目录。
- 单个 schema 版本在 `BEGIN IMMEDIATE` 中执行；DDL 或记录版本失败会整体回滚。
- 已记录版本在重复 `Open` 时跳过，不能重放。
- 任务文件迁移失败不会删除、截断或覆盖旧输入。
- 没有用户明确确认，不执行旧目录、Session、run、worktree 或 orphan 的物理清理。

SQLite 是状态真相，Task Workspace 是可读上下文和大对象真相。两者不一致时显示错误并
允许对账或重试，不把部分成功伪装成完成。

## 2. Schema 版本

| 版本 | 内容 |
| ---: | --- |
| 1 | 既有 `workspace_state` |
| 2 | 既有 `task_context_settings` |
| 3 | Task Workspace、resource、session/message/event/run/tool、permission、Git binding、artifact 和 `legacy_task_migrations` |
| 4 | message attachment、resource reference、requirement proposal |
| 5 | 消息分页索引、旧迁移完成索引和完成状态约束 |

v5 新增：

- `agent_messages(task_id, session_id, sequence DESC)`，支持按任务与 Session 读取最新一页。
- `legacy_task_migrations(state, completed_at, task_id)`，支持完成/失败诊断。
- INSERT/UPDATE trigger：`state = completed` 必须有 `completed_at`；其他状态不得带完成时间。

v5 不删除表、列、`workspace_state`、旧 migration 记录或文件。数据库版本高于客户端支持范围
时拒绝打开，避免旧程序误写新数据。

## 3. 旧任务文件迁移

旧任务可能只有以下形态：

```text
<task-id>/
├── context.json
├── files/
└── images/     # 也可能只在 JSON 中保存 data URL
```

首次 Ensure 会创建新 Task Workspace 目录与 manifest，并将旧内容复制到当前任务的
`sources/`、`attachments/` 和 `context/` 投影。规则如下：

- 旧 `context.json`、`files/`、`images/` 保留原位，不作为迁移清理对象。
- data URL 必须合法 base64，声明 MIME 必须和 magic bytes 匹配。
- 支持 PNG、JPEG、GIF、WebP；数量、单项与总大小均有上限。
- 不可变副本记录字节数和 SHA-256；重复 Ensure 不重复写相同内容。
- 部分迁移失败会写 `legacy_task_migrations.state = failed` 和可读错误；修复输入后可重试。
- 成功记录 source revision、旧路径、目标 workspace、warnings、started/completed time。
- 旧目录中的未知用户文件不自动扫描、覆盖或删除。

任务缺少可迁移输入时也会建立独立 workspace，不伪造历史 Agent 消息或执行记录。

## 4. PI/OMP 标识兼容

新写入的 Agent engine 只能是 `pi`。读取旧前端快照时，`omp`、`oh-my-pi` 和
`PI / oh-my-pi` 显式规范化为 `pi`；`max` thinking 规范化为 `xhigh`。

不迁移 OMP Session、ready/protocol-v2、host tool、chunk、approval mode 或 CLI flags，
也不调用 OMP 作为兼容回退。未知 engine 值保留原始数据并降级处理，不能猜成 PI。

历史 development result 仍是人工/外部委托记录，不冒充新 Agent Session 或消息。

## 5. Task Data Root 迁移

设置页迁移任务资料根目录时：

1. 目标必须是用户明确选择的空目录。
2. 复制整个旧目录，包括应用未知的用户文件。
3. 验证复制结果并重新 Ensure 当前任务投影。
4. 只有全部成功后，才在 SQLite 中切换 root 设置。
5. 失败继续使用旧 root；成功后旧 root 仍保留为备份。

该动作与 schema migration、任务回收站、永久删除和 worktree 清理互相独立。

## 6. 运行时恢复

启动时 `RecoverInterrupted` 在 SQLite 中对账未结束状态：

- active Session → interrupted。
- active run → interrupted 并写 finished/error。
- streaming/pending message → error。
- running/waiting tool → failed/cancelled。
- pending permission → expired。
- Session grant → 进程退出或重启时失效。

恢复不会自动重新执行 prompt、工具或 Shell。用户选择“恢复会话”后，应用重新启动隔离 PI，
校验登记的 Session 文件，调用 `switch_session`、`get_state` 和 `get_entries` 补齐投影。
Session 文件丢失时保留数据库历史并报告不可恢复。

## 7. 数据库和文件失败

- 数据库启用 WAL、foreign keys 和 5000 ms busy timeout。
- 写事务获取不到锁时返回明确错误；锁释放后可用同一输入重试，失败写不会增加 revision。
- 原子文件写使用同目录临时文件、`fsync` 和 rename；写入或 rename 失败会清理临时文件，
  不让未完成 artifact 以正式文件可见。
- 数据库事件是稳定事件的权威记录；补充 run 日志失败不会重复发送已提交事件。
- 文件写成功但后续数据库索引失败时，保留文件供诊断/对账，不删除旧输入。

真实磁盘耗尽、断电和网络文件系统语义仍依赖操作系统；发布验收应在目标平台补做人工故障测试。

## 8. 验证与回滚

自动化迁移测试覆盖：

- v2 → v5，保留 `workspace_state`。
- v4 → v5，保留权限和 legacy migration 记录。
- 空库 → v5，重复打开幂等。
- 单版本失败回滚 DDL 与版本号。
- 更新程序版本过旧时拒绝更高 schema。
- 完成状态触发器、数据库锁与释放后重试。
- 旧 JSON、data URL、部分迁移重试、symlink 和未知 engine。

代码回滚不能自动降级 schema。旧数据和新增 Task Workspace 保留；如需使用旧客户端，
应先备份数据库并使用与该 schema 兼容的版本，而不是删除 migration 记录。

字段级合同见 `docs/pi-agent-workbench/DATA_MODEL.md`，OMP 标识兼容见
`docs/pi-agent-workbench/PI_MIGRATION_FROM_OMP.md`。
