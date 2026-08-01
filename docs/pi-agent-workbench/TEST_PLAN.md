# 分阶段测试与验收计划

## 1. 原则

- 每个阶段只验证该阶段范围和既有回归。
- 自动测试、静态检查、真实本机 PI 探针和人工 UI 验收分层报告。
- fake RPC 通过不代表真实 provider、网络、凭据或模型可用。
- source/static check 不代表路径、Git 或进程运行时安全。
- 没有设备/凭据/网络或人工确认的结果标记 external_validation_pending，不伪造通过。
- 测试不得修改用户原始工作区、全局 PI 配置或凭据。

每阶段完成门：

1. 阶段专项测试通过。
2. go test ./... 通过。
3. frontend pnpm typecheck、pnpm test、pnpm build 通过。
4. git status 仅包含本阶段预期文件。
5. 人工确认点完成。
6. 按用户要求提交并推送当前阶段后停止，等待下一阶段确认。

## 2. 测试层

### 2.1 Go 单元测试

- taskspace path resolver、manifest、atomic write。
- SQLite migration/repository。
- LF JSONL decoder、RPC correlator、event mapper。
- permission pure policy、command classifier。
- Git worktree/status/diff。
- execution logs、output cap、recovery。

### 2.2 Go 集成测试

- 使用临时 SQLite 和临时文件系统。
- 使用真实 git binary 在 temp repo 创建 worktree。
- 使用受控 fake pi child process，通过 stdin/stdout 严格 JSONL。
- 使用 process group 测试 abort/exit。
- 不访问生产 TaskDataRoot、原始仓库或 ~/.pi。

### 2.3 React/Vitest

- bridge contract 与 deterministic browser Mock。
- TaskAgentWorkbench reducer。
- 任务/Session 切换和 stale event。
- 权限卡、工具卡、附件、文件和 Diff 面板。
- 长历史分页/虚拟化。
- 订阅和 timer 清理。

### 2.4 本机 PI 合同探针

探针使用：

~~~text
PI_CODING_AGENT_DIR=<temp>
PI_CODING_AGENT_SESSION_DIR=<temp>
PI_OFFLINE=1
pi --mode rpc ...
~~~

可自动验证 get_state、get_entries、U+2028、错误帧、EOF 和 gate handshake。真实模型 prompt、图片理解、工具执行需用户配置 provider 凭据，单列 external_validation_pending。

### 2.5 人工 UI

Wails 桌面运行时验证 WebView、文件选择器、系统打开、流式交互、窗口宽度和进程退出。浏览器 Mock 截图不能替代桌面验收。

## 3. Fake PI harness

新增 test helper，不命名为 omp。支持脚本化 fixture：

- 启动无首帧，收到 get_state 后回复。
- response 乱序但 id 正确。
- text/thinking delta。
- tool start/update/end。
- extension_ui_request/response。
- agent_end willRetry + agent_settled。
- get_entries cursor。
- stderr burst。
- invalid JSON、truncated EOF、16 MiB+ frame。
- abrupt exit、ignore abort、ignore SIGTERM。

fixture 使用 LF 字节 framing，并包含：

- JSON 字符串中的 U+2028/U+2029。
- UTF-8 多字节字符跨 pipe read。
- CRLF record 兼容。
- 大于 bufio.Scanner 默认 64 KiB 的合法帧。

## 4. 阶段 0

验证：

- 当前仓库四条基线命令和祖先关系。
- OMP 负向清单完整。
- pi path/version/help 与本机 rpc.md 一致。
- 隔离 PI probe。
- Reasonix license 和复用文件。
- 8 份计划文档链接/标题/必需章节存在。
- 文档没有声称 PI 有 ready/protocol negotiation/strong sandbox。

当前基线结果：

- pnpm typecheck：通过。
- pnpm test：独立功能 worktree 通过，15 files / 87 tests；源脏工作区初始审计为 17/96，差异来自未提交 UI 测试。
- pnpm build：通过，有既有大 chunk warning。
- go test ./...：受限 sandbox 的 httptest bind ::1 返回 EPERM；获准非沙箱执行全部通过。

人工门：确认阶段 0 文档、功能分支基线和 PI 凭据策略。

## 5. 阶段 1：Task Workspace 与数据

Go：

- 新任务创建完整目录树和 manifest。
- Ensure 幂等。
- 两任务同名附件不串。
- context task/requirements/acceptance 内容正确。
- approved-vN 不被覆盖。
- sources/attachments 原始内容 hash。
- data URL 图片导入、去重、MIME/magic/size。
- 旧 context.json/files/images 迁移成功且旧数据保留。
- 迁移中断后可重试。
- TaskDataRoot 不可用/磁盘错误。
- path traversal、absolute、symlink、case collision、Windows path table tests。
- migration v1/v2 → v3；空库 → v3；重复 Open 幂等。
- 所有 repository CRUD/foreign scope。

React：

- PI 标签迁移。
- Task Workspace 设置/错误状态。
- 旧 workspace JSON hydrate。

人工：

- 选择自定义 TaskDataRoot。
- Finder 打开目录。
- 检查真实文件权限和布局。

停止条件：目录根、清理或不可逆迁移仍有未确认项。

## 6. 阶段 2：原生 PI RPC 最小会话

Go：

- 启动无 ready，get_state probe 成功。
- startup timeout、pi missing、unsupported version。
- strict LF、U+2028/U+2029、UTF-8 split、CRLF、invalid/truncated/oversize frame。
- request id correlation、乱序 response、timeout。
- prompt accepted 与后续 failure 分离。
- multi-turn 使用同一 process/session。
- abort、agent_settled、agent_end willRetry。
- get_entries cursor 和 missing cursor fallback。
- abnormal exit、stderr cap、EOF、SIGTERM/SIGKILL escalation。
- 一 task 一个 active run。
- 两任务 event envelope 隔离。
- DB message/event 增量存储和 restart recovery。
- OMP production call 负向扫描。

React：

- 最小 user/assistant streaming。
- stop。
- task switch unsubscribe 和 stale epoch。
- PI missing/error。
- browser Mock deterministic sequence。

真实 PI：

- 无凭据 probe。
- 有凭据 multi-turn、abort 和 restart resume：external_validation_pending，直到用户配置。

## 7. 阶段 3：上下文、附件、artifacts

Go：

- context assembly 只含当前 task。
- @ reference lookup 只返回授权资源。
- 两任务相同 logical path 隔离。
- image MIME/magic/size/count。
- document attach/drop/import。
- 删除引用不删除 immutable source bytes。
- artifact create/modify、atomicity、list/preview。
- proposal 写入 artifacts/proposals，不改 approved requirement。
- restart 后引用、artifact 和 attachment 恢复。
- migration v4 幂等。

React：

- @ picker scope。
- paste image、drop file、remove attachment。
- artifact 即时出现。
- preview failure/unsupported type。

真实 PI 图片理解：external_validation_pending，需 provider 支持和凭据。

### 阶段 3 自动验证记录（2026-08-01）

- taskspace/storage/agent 专项测试覆盖同名跨任务文件、MIME/magic/数量/大小、路径穿越与 symlink、引用删除、重启恢复、v3→v4 幂等迁移、proposal 保护和未知工具失败关闭。
- React/Vitest 覆盖当前任务 `@` 选择、纯附件发送、浏览器 Mock 的任务隔离、引用移除、artifact 即时刷新、预览与系统打开桥。
- 本机 PI 0.82.1 隔离探针覆盖 get_state 和真实内嵌 gate Extension 加载/heartbeat；不需要 provider 凭据。
- 阶段提交前必须再执行 `go test ./...`、`go vet ./...`、前端 typecheck/test/build、真实 PI probe/gate，以及 Wails build。最终命令结果写入阶段交付汇报。
- 真实模型图片理解、模型主动调用 list/read/write、系统文件选择器与桌面 WebView 人工验收为 `external_validation_pending`。

## 8. 阶段 4：权限与工具

Go：

- policy precedence 全矩阵。
- allow、deny、timeout、cancel、revoke。
- once 并发原子消费。
- session/task/permanent 不泄漏。
- critical 始终 once。
- unknown tool。
- gate handshake、bad nonce、extension_error、missing gate。
- tool receipt 对账。
- path traversal/symlink/case/Windows。
- shell redirection、pipe、subshell、indirect command。
- audit 不含 secret。
- restart pending → expired。

React：

- permission card 所有风险和 scope。
- critical 只显示 once。
- stale task request 不显示。
- repeated submit 防护。
- ToolCard start/update/end、折叠、错误、大输出懒加载。

真实 PI custom tool 拦截与 RPC confirm：阶段 4 必须在本机 PI 0.82.1 做一次集成验证；没有模型凭据时可用 Extension 自测命令验证 gate，模型发起工具仍 external_validation_pending。

## 9. 阶段 5：Git worktree 与开发

Go：

- repo inspect：non-git、bare、unborn、subdirectory、dirty。
- 两 task 绑定同一 source repo 创建独立 worktree。
- baseline commit、branch 唯一。
- source worktree pre/post status byte-for-byte 不变。
- Agent write 只进入 task worktree。
- status：tracked/untracked/renamed/deleted。
- text diff、binary、large diff truncation。
- test/build command、stdout/stderr、abort background process。
- local commit 明确审批。
- push/force push 分类始终 once；测试不访问真实远端。
- worktree external delete、repo move、cleanup failure、uncommitted cleanup。
- app restart recovery。

人工：

- Finder/terminal 对比 source 和 task worktree。
- 查看 branch、baseline、逐文件 diff。
- 中断一个真实长运行测试命令。

## 10. 阶段 6：完整工作台

React：

- 顶栏所有状态。
- Ask/Plan/Agent 与后端能力一致。
- new/switch Session。
- follow_up、steer、stop 的真实命令区别。
- message/tool/permission/file/diff/test/artifact cards。
- context/file/change/run 四面板。
- task basic info 入口保留。
- no PI/no repo/no session/empty/error/recovery。
- 运行中切 task。
- 长历史分页或 virtualization：至少 10,000 条 fixture。
- 大 tool output 不留在 store。
- 1180 breakpoint 附近和更窄窗口。
- keyboard/focus/aria 基础检查。

Go/bridge：

- 所有 Wails method task/session scope。
- subscription cleanup。
- hydrate + live event gap。
- browser Mock 与 native DTO parity。

人工：

- Wails 桌面截图与需求红框区域对照。
- 流式滚动、卡片展开、窄窗口。
- UI 不宣称未执行动作成功。

## 11. 阶段 7：恢复、安全、迁移与性能

故障注入：

- streaming 中退出 app。
- tool 执行中退出。
- PI crash、invalid/truncated/oversize frame。
- database locked、disk full、artifact rename failure。
- PI Session 丢失。
- worktree 丢失、source repo 移动。
- permission 无响应。
- cancel 与 process exit race。

安全回归：

- traversal/symlink/junction/UNC。
- shell external write fixture。
- cross-task event/resource/grant。
- credential fixture 不出现在 DB/log/UI。
- gate missing fail closed。
- 原始 source repo 不变。

迁移：

- 生产形态旧 JSON fixture。
- 旧 data URL。
- 部分迁移 crash/retry。
- unknown engine value。
- 每阶段 schema 重开幂等。

性能门：

- 10,000 messages 分页首屏不读取全量正文。
- 10 MiB tool output 不进入 React store。
- 高频 delta 按批写；SQLite revision/statement 次数有断言。
- Wails event queue 有界。
- 2 MiB diff preview 截断并可诊断。

最终执行 04-最终验收清单.md 中可自动化项目，并生成 docs/pi-agent-workbench/FINAL_VALIDATION.md。

## 12. 常规命令

每阶段：

~~~bash
go test ./...
cd frontend
pnpm typecheck
pnpm test
pnpm build
~~~

改变 Go 文件后仅对改动文件 gofmt。改变 TypeScript 后遵守现有格式，不全仓库格式化。

高风险或慢测试可单独提供：

~~~bash
go test ./internal/agent/... -run TestRPC
go test ./internal/gitrepo/... -run TestWorktreeIntegration
~~~

但专项通过不能替代 go test ./...。

## 13. 阶段证据格式

每阶段提交前记录：

~~~text
基线 commit：
验收 commit：
修改范围：
数据库版本：
自动测试：
真实 PI 探针：
人工验证：
external_validation_pending：
原始工作区 git status 前后对比：
已知限制：
~~~

不得把 browser Mock、fake PI、临时 Git repo 或静态检查描述为生产验收。
