# 任务级原生 PI Agent 工作台最终验收

> 验收日期：2026-08-01
>
> 基线 commit：`0622cb7`（阶段 6）
>
> 验收 commit：本文件所在的阶段 7 提交（最终哈希以 Git 记录为准）

## 一、最终实现范围

阶段 0–7 已落地以下纵向切片：

- 原生 PI 身份、版本探针、严格 LF JSONL RPC 和隔离 utility 分析。
- 每任务 Task Workspace、旧上下文/图片/附件 copy-first 迁移和 schema v5。
- 任务详情右侧 PI Agent 工作台，支持 Ask/Plan/Agent、多 Session、分页历史、流式回答、
  Steer、Follow-up、停止和显式恢复。
- 当前任务资源引用、图片/文件附件、artifact 和 requirement proposal。
- 应用内嵌 BTask gate、权限作用域、审计和失败关闭。
- 显式 Git 仓库绑定、每任务 worktree、文件工具、Shell、状态和有界 Diff。
- Context、Files、Changes、Runs 辅助面板与 Browser Mock。
- 异常恢复、安全回归、性能门、当前架构/权限/迁移文档。

未加入多 Agent、自动提交、自动 push、自动 PR、自动合并、自动发布、浏览器自动操作或
跨任务全局记忆。

## 二、架构落地情况

| 边界 | 落地结论 |
| --- | --- |
| 页面上下文 | `taskId` 为最外层身份；切换任务递增 epoch、取消订阅并清空 Session/消息/权限/工具/资源/运行状态 |
| 后端 scope | 所有 Agent API 携带 task/session/run/tool 身份；SQLite 复合外键拒绝跨任务组合 |
| PI 运行时 | 只启动原生 `pi --mode rpc`；不探测、不执行、不回退 OMP |
| 资源加载 | 关闭全局 extensions/skills/prompts/themes/context、内建工具与在线发现；只显式加载 BTask gate |
| 状态真相 | SQLite 保存稳定状态与事件；Task Workspace 保存可读上下文、Session、日志、大输出和产物 |
| 权限 | Go policy + gate + receipt 对账；React 不拥有授权权威 |
| Git | Agent 只写独立任务 worktree；原始 source workspace 不作为写目标 |
| 工作流 | Agent 结束不批准需求、不推进 Task 状态 |

## 三、自动测试

最终命令结果：

| 命令 | 结果 |
| --- | --- |
| `go test ./... -count=1` | 通过；沙箱外复跑，允许 `httptest` 绑定本机回环端口 |
| `go test -race ./internal/agent ./internal/gitrepo ./internal/execution ./internal/permissions . -count=1` | 通过 |
| `go vet ./...` | 通过 |
| `pnpm typecheck` | 通过 |
| `pnpm test` | 22 个测试文件、121 个测试通过 |
| `pnpm build` | 通过；仅保留既有大 chunk 警告 |
| `BTASK_TEST_NATIVE_PI=1 go test ./internal/agent -run TestNativePI -count=1 -v` | 原生 PI probe 与 gate Extension 集成通过 |
| `wails build -m -v 1` | darwin/arm64 编译、打包和自签名通过 |

沙箱内 `go test ./...` 的 root/Plane/report `httptest` 因禁止监听 `[::1]:0` 失败；同一 commit
在允许本机回环的环境完整通过，因此记录为环境限制，不记为产品回归。

重点自动化证据：

- RPC：非法 JSON、截断 EOF、16 MiB 超大帧、raw U+2028、启动超时、stderr 上限、缺少 PI。
- 恢复：应用关闭、PI crash、缺失 Session、显式恢复、权限超时、取消与进程退出竞态。
- 任务隔离：跨任务 message/event/resource/reference/grant/tool 访问与复用被拒绝。
- 路径：`../`、绝对路径、Windows volume/UNC/保留名、大小写别名和 symlink 逃逸被拒绝。
- Shell/Git：凭据目标、外部重定向、`rm -rf`、隐藏 add/commit/push/PR/publish 被分类或硬拒绝；
  worktree 外部删除和 source repo 移动可诊断，原始仓库 fixture 保持不变。
- Gate：错误 nonce、Extension error、缺少 permission receipt、拒绝/超时/取消均失败关闭。
- 存储：v2→v5、v4→v5、空库、重开幂等、失败回滚、未来 schema 拒绝、数据库锁与释放后重试。
- 旧数据：生产形态 JSON、data URL、旧文档/图片保留、部分迁移重试、unknown engine 兼容。
- 文件失败：模拟 rename `ENOSPC` 后正式 artifact 不可见且临时文件被清理。
- 性能：10,000 消息首屏按 cursor 分页；2000 个 delta 只产生 1 次消息写和 1 条 delta 事件；
  定时器在 message_end 前刷新流；10 MiB 工具输出完整落盘、只懒读 2 MiB；事件队列有界；
  2 MiB Diff 截断并返回诊断。
- 前端：任务切换迟到请求/事件丢弃、历史分页、工具输出点击后只加载一次、权限卡作用域、
  Changes/Runs race 与错误态、窗口布局和编辑入口。

生产代码负向扫描未发现 OMP 可执行调用、`@oh-my-pi/*` 依赖、`.omp` 配置读取或硬编码凭据。
兼容迁移字符串、历史审计文档和兼容测试中的旧名称按设计保留。

## 四、手工验收

已完成 Browser Mock 人工检查：

- 创建任务 A/B；A 的 Session 与消息在 B 中不可见，切回 A 后恢复。
- 1180 px 视口和 Wails 最小 1120×720 布局无水平溢出。
- 红框详情区域展示 Agent 工作台；任务编辑和原始人工记录入口仍可用。

已完成本机桌面构建与应用进程启动。原生 macOS GUI 截图/点击自动化因辅助功能读取超时未完成，
因此桌面视觉、真实文件选择器、真实 provider 长对话和 Windows/Linux 包属于需要人工回归项。

## 五、安全边界

本版本提供默认资源隔离、自有工具 allowlist、路径/realpath/symlink 校验、权限审计、gate
失败关闭、最小环境和 Git worktree 隔离。它不是 OS 沙箱：PI、Extension、Shell 和 Git 仍以
当前登录用户权限运行。对于恶意仓库、恶意本地进程或无人监管执行，应另行使用容器、VM 或
操作系统策略。

Git push、force push、PR、merge、rebase、reset、发布和远端删除没有向 PI 开放。BTask
实施分支的 Git push 是用户对本次开发流程的明确授权，不属于产品内 Agent 自动推送能力。

## 六、数据迁移

当前 schema 为 v5。迁移保留 `workspace_state` 和全部 v3/v4 数据；v5 只增加分页/完成索引和
legacy migration 完成状态触发器。旧 `context.json`、`files/`、`images/` 和旧 root 采用
copy-first 策略保留。迁移失败可重试且不删除旧数据；代码回滚不自动降级数据库。

## 七、已知限制

- 只支持代码锁定的原生 PI `0.82.x`（当前至少 0.82.1）；其他版本需重新验证协议与 gate。
- 工具完整文本输出当前最多落盘 12 MiB，UI 单次懒加载最多 2 MiB。
- 单条 assistant 聚合正文上限 4 MiB，RPC 单帧上限 16 MiB。
- SQLite 单进程写模型和 5 秒 busy timeout 不等同于多实例协调协议。
- 跨进程重连、自动重放工具、连续崩溃安全模式和自动 orphan 清理未实现。
- Browser Mock 不代表物理 Session、OS 凭据库、真实 provider 或文件系统权限。
- 构建仍有既有 RichMarkdownEditor 大 chunk 警告，不影响本阶段正确性。

## 八、未完成项

没有已知的自动化测试失败或阶段 7 代码阻塞项。仍需人工完成：

1. 原生 Wails 窗口完整视觉与交互 smoke。
2. 使用真实 provider 凭据进行长对话、附件和权限确认回归。
3. Windows/Linux 的 junction、UNC、凭据库和安装包回归。
4. 目标平台真实磁盘耗尽/断电故障演练。

## 九、合并建议

功能、安全、迁移和 Git 隔离的自动化结论为通过；Browser Mock UI 结论为通过。建议先推送并
进入团队 Review，在合入主分支前补做上述原生桌面 smoke。原生 GUI 人工回归尚未完成，因此
最终 UI 体验结论为“有条件通过”，不应写成已完整验收。

```text
验收人：待团队指定
验收日期：2026-08-01
基线 commit：0622cb7
验收 commit：本文件所在阶段 7 提交

功能结论：通过（自动化与 Browser Mock）
安全结论：通过（应用级软边界；非 OS 沙箱）
数据迁移：通过
Git 隔离：通过
UI 体验：有条件通过（原生桌面人工 smoke 待补）

阻塞问题：无已知代码阻塞
非阻塞问题：原生桌面、真实 provider、Windows/Linux 和真实磁盘故障人工回归待补
是否允许合并：完成原生桌面 smoke 后是
```
