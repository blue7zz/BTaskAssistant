# Reasonix 完美嵌入验收状态

> 目标：任务内 Reasonix 工作台完整可用。以下场景对应目标文件中的"最终验收场景"，
> 标注自动化测试覆盖与当前状态（阶段 7 快照）。

| # | 验收场景 | 自动化覆盖 | 状态 |
|---|---|---|---|
| 1 | A 发消息后切到 B，再返回 A，消息/运行状态/审批仍在，且可立即继续操作 | TestTaskSessionIsolation + ReasonixPage 保活测试 + host tab 后台 hydration 解锁测试 | ✅ 已实现（阶段 3） |
| 2 | A、B 都能后台运行，各自写入自己的任务文件空间和会话 | TestTaskSessionIsolation（会话目录隔离） | ✅ 已实现 |
| 3 | 快速 A→B→A 循环 100 次，无错任务/错目录/持续内存增长 | TestConcurrentActivateNoCrossWrite（-race）+ TestPruneIdleRuntimesKeepsActive（LRU 上限） | ✅ 竞态修复 + 上限回收 |
| 4 | 退出并重启后每个任务恢复到原会话 | TestSessionRetainedAcrossRebuild + TestMigrateLegacyLastSession | ✅ 已实现（阶段 2/3/7） |
| 5 | Git 绑定状态变化不改变 RX 的任务文件空间 | TestRxWorkspaceForTask（ready/非 ready/无绑定三态）+ WorkspaceIdentity.Generation | ✅ 已实现（阶段 2） |
| 6 | 模型/设置更新失败时原会话继续可用 | TestSetModelFailureKeepsOldController（build-then-swap） | ✅ 已实现（阶段 1） |
| 7 | API Key 不出现在前端状态/SQLite/日志/诊断 | TestCredentialWriteIsolated（.env 0600 + 隔离） | ✅ 已实现（阶段 4） |
| 8 | 所有可见按钮有真实副作用；未支持功能不显示或明确禁用 | 契约校验 + 核心矩阵（90 真 / 42 显式错误 / 0 空返回） | ✅ 已实现（阶段 6） |
| 9 | Shadow 内弹窗/菜单/输入法/拖拽/快捷键正常且不污染 BTask | 生产产物实测（样式全入 shadow、零泄漏）+ rxKeyActive 快捷键守卫 | ✅ 已实现（阶段 5） |
| 10 | 自动化测试使用临时 REASONIX_HOME、假 Provider、禁止网络 | TestMain 隔离 + RequireKey=false | ✅ 已实现（阶段 1） |

## 跨平台构建

| 平台 | 状态 |
|---|---|
| macOS（本机 arm64） | ✅ `wails build` 产物 + 运行验证（PID 60997） |
| Windows / Linux | ⏳ 待 CI（代码无平台特定依赖；go test 全绿） |

## 剩余人工验收（真实 UI）

1. 未绑定 Git 工作树的任务打开 RX：界面加载、对话、工具调用、审批。
2. 设置页"Reasonix 设置"：Provider 增删/凭据状态/模型/审批/Home 切换。
3. 附件粘贴/文件拖入；历史/恢复/分叉/回退/压缩按钮。
4. 任务切换（A→B→A）：会话内容/运行状态保持。
5. 重启应用：各任务会话恢复。
