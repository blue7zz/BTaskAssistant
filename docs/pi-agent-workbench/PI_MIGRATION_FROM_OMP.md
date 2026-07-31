# 从 OMP 迁移到原生 PI

## 1. 目标

目标运行时只能是本机原生 pi 0.82.1，通过 pi --mode rpc 接入。迁移不是把 OMP 文案改成 PI，也不是为 omp 增加兼容分支。

迁移完成后的硬性条件：

- 运行时不执行、探测或建议安装 omp。
- 不读取 .omp 或 oh-my-pi 配置。
- 不新增 @oh-my-pi/*。
- UI 只显示 PI，不显示 PI / oh-my-pi。
- 不保留 OMP ready、protocol-v2、chunk、host tool、approval mode 等假设。
- 旧数据仍可读取，不静默删除。

## 2. 当前 OMP 依赖清单

### 2.1 运行时代码

| 文件 | 当前行为 | 迁移动作 |
|---|---|---|
| internal/engine/adapter.go | Statuses 调用 commandDetails("omp")；标签 PI / oh-my-pi | 阶段 1 改为隔离环境下探测 pi；状态返回原生路径和版本 |
| internal/engine/omp.go | FindOMP、OMPAnalyzer、omp --print | 阶段 2 由 native PI RPC utility session 替换并删除运行时引用 |
| internal/engine/requirements.go | analystCommand("pi") 返回 omp；拼装 OMP flags | 阶段 2 改走统一 PI RPC client；固定输入仍保留 |
| internal/engine/daily_report.go | 复用 analystCommand；PI 分支执行 OMP flags | 阶段 2 改走受限、无工具的 PI RPC utility session |
| app.go | AnalyzePlaneCandidate 构造 OMPAnalyzer | 阶段 2 注入 NativePIAnalyzer/utility service |
| internal/engine/pi_settings.go | 生成 --model、--thinking、--max-time | 保留用户模型/思考/超时意图，改为 RPC set_model、set_thinking_level 和 Go context timeout |

### 2.2 前端与持久数据

| 文件/字段 | 当前含义 | 兼容方案 |
|---|---|---|
| frontend/src/domain/task.ts 的 DevelopmentEngine | 值已经是 pi/codex | 保持值 pi；只纠正实际运行时 |
| frontend/src/domain/task.ts 的 RequirementAnalyst | 值已经是 pi/codex | 保持值 pi |
| workspace.piSettings | OMP 模型、thinking、timeout 的 UI 设置 | JSON key 暂保留；normalize 后解释为原生 PI 设置 |
| frontend/src/lib/bridge.ts | 浏览器状态标签 PI / oh-my-pi | 阶段 1 改为 PI；Mock 返回原生运行时字段 |
| PISettingsPage.tsx | OMP 登录、默认模型和兼容性文案 | 阶段 1 改为 PI 路径、版本、资源继承和凭据隔离说明 |
| TaskDetail.tsx / DailyReportSettings.tsx | option 文案 PI / oh-my-pi | 改为 PI |
| TaskContext materialized JSON | 可能包含 analyst=pi、engine=pi | 无需值迁移；manifest 记录 engine=pi 和 schemaVersion |

没有发现持久化 engine 值为 omp 的正式领域字段。迁移器仍应对未来或手工导入数据中的 omp、oh-my-pi、PI / oh-my-pi 做显式兼容：

- 读取时映射为 pi，并写 migration warning。
- 不自动启动 Agent。
- 下次正常保存写回 pi。
- 未知值保留原始数据并降级只读，不能默认映射。

### 2.3 测试与 fake

- internal/engine/daily_report_test.go 创建名为 omp 的 fake executable。
- internal/engine/omp_test.go 覆盖 OMP 候选解析。
- frontend/src/components/PISettingsPage.test.ts 断言 omp 路径和版本。
- App、bridge 和相关 UI 测试包含 PI / oh-my-pi。

迁移动作：

- RPC 单元测试使用 Go 内存 pipe 或受控 fake pi process，输入输出为 JSONL。
- 不把 fake binary 命名为 omp。
- 候选 JSON 解析测试保留，但移到与运行时无关的 analyzer/parser 文件。
- 新增全仓库负向测试/检查，确保生产代码没有 exec.LookPath("omp")、exec.Command("omp") 或 OMP UI 标签。

### 2.4 文档

当前 OMP 内容位于：

- README.md
- docs/ARCHITECTURE.md
- docs/SOFTWARE_ARCHITECTURE.md
- docs/MVP_SCOPE.md
- docs/RELEASE_NOTES_v0.1.0.md
- docs/RELEASE_NOTES_v0.2.0.md
- docs/ADR/0001-RICH-EDITOR-PLANE-OMP.md

策略：

- README 和当前架构文档改为原生 PI。
- 历史 release notes 保留历史事实，但追加“已由任务级原生 PI 工作台取代”的说明。
- ADR 0001 不篡改当时决策；新增 ADR 记录 superseded 状态和新决策。
- 不把旧文档中的 OMP 计划当作可执行合同。

## 3. OMP 与 PI 0.82.1 差异

| 旧实现假设 | 原生 PI 事实 | 替代 |
|---|---|---|
| executable 是 omp | executable 是 pi | commandDetails("pi") |
| 单次 --print | 长寿命 --mode rpc | Supervisor + JSONL |
| --no-rules | PI 没有该 flag | --no-context-files、--no-skills、--no-prompt-templates |
| --no-session | PI RPC 使用 Session | 任务级 --session-dir |
| --no-lsp、--no-pty、--no-title | PI 0.82.1 合同未使用这些启动假设 | 删除 |
| --max-time | PI 0.82.1 help 没有该合同 | Go context 和进程状态机 |
| ready / 协议协商 | PI 无 ready / negotiation | get_state correlated probe |
| OMP approval mode | 原生 PI 不提供 BTask 所需授权作用域 | BTask policy + Extension tool_call + RPC UI |
| host tool / chunk | PI 使用 tool_execution_*，update 是累计结果 | 稳定事件 mapper |
| OMP 会话格式 | PI 使用自己的 append-only Session JSONL | external_session_path + get_entries cursor |
| OMP 全局登录/配置 | PI 使用 PI_CODING_AGENT_DIR 和 provider auth/settings | 每任务隔离目录 + 显式凭据策略 |

## 4. 分阶段迁移

### 阶段 1：标识和数据基础

- 引入 engine 标识常量 pi、codex；拒绝新写入 omp。
- EngineStatuses 探测 pi，而不是 omp；探测使用隔离临时 PI_CODING_AGENT_DIR，避免读取全局设置。
- PISettings 保留 JSON 字段，但停止生成 OMP CLI 参数。
- 新 Task Workspace manifest 写 engine=pi、piResourcePolicy=isolated。
- 迁移器识别遗留字符串并记录 migration warning。
- UI 标签和设置帮助改为原生 PI。
- 不启动 PI Agent session，不实现 RPC。

回滚：保留原始 workspace_state 备份/事务；新表可为空，现有任务仍通过旧快照打开。

### 阶段 2：运行时替换

- 新增 internal/agent 的 pi_process、rpc_client、rpc_protocol、supervisor 和 event_mapper。
- 任务工作台与固定 utility analysis 都使用 pi --mode rpc。
- 候选提炼、需求分析、日报保持固定输入和结构化解析；使用独立的无工具 utility Session，不与任务聊天 Session 混用。
- 删除 app.go 到 OMPAnalyzer 的引用。
- 删除或改名 internal/engine/omp.go；解析纯函数独立保留。
- 测试 fake 改为严格 RPC JSONL。
- 生产代码负向扫描不得出现可执行 omp 调用。

回滚：数据库保留旧字段；关闭 Agent feature flag 后仍可人工整理、复制提示词和使用 Codex，不回退执行 omp。

### 阶段 3：上下文迁移

- utility analysis 和 TaskAgent Session 都只从 Task Workspace 装配材料。
- 不再用 @<prompt-file> 这种 OMP CLI 拼接。
- 图片走 prompt.images；文档走受控引用/read 工具。

### 阶段 4：审批迁移

- 所有授权作用域由 permission_requests、permission_grants 和 BTask gate Extension 管理。
- 不读取或翻译 OMP tools.approvalMode。
- 若旧 settings 中出现 OMP approval 字段，只记录 deprecated，不映射为允许。

### 阶段 7：清理和最终验收

- 更新当前文档和新增 ADR。
- 全仓库生产代码执行负向扫描。
- 旧 release note/ADR 只保留历史上下文。
- 验证运行 pi 的路径、版本、Session 目录和资源策略在 UI 可见。

## 5. 固定 utility analysis 的迁移方式

Plane 候选提炼、需求分析和日报不是自由 Agent 工具任务，固定程序应继续负责：

- 收集和清洗输入。
- 构造固定 JSON 输入包。
- 截断、隐私清洗和 schema 校验。
- 解析结构化输出。

PI 只负责模型推理：

1. 创建临时或任务级 utility Session。
2. 禁用所有工具和自动资源。
3. 发送 prompt。
4. 聚合最终 assistant text。
5. 以 agent_settled 结束。
6. 严格解析原有 JSON schema。
7. 成功或失败后关闭进程；不把 utility Session 展示为任务聊天历史。

需求分析中的 projectRead 不应开放 PI 对整个项目的通用读取。现有 Go 固定扫描/项目观察结果继续作为输入材料；需要任意文件引用时在阶段 3 通过任务资源绑定实现。

## 6. 设置兼容

现有 PISettings：

~~~json
{
  "model": "",
  "thinkingEffort": "xhigh",
  "timeoutMinutes": 3
}
~~~

新解释：

- model：用户期望的 provider/model；启动后通过 get_available_models 匹配，再调用 set_model。匹配不到则显示错误，不猜 provider。
- thinkingEffort：启动后以 get_available_thinking_levels 为准，再调用 set_thinking_level。旧 max 仍规范化为 xhigh。
- timeoutMinutes：BTask 一次 utility operation 或 UI run 的外层超时策略，不转为 PI CLI flag。聊天 run 默认允许用户停止，不能在后台静默当作成功。

新增但不放入完整消息快照的设置：

- resourcePolicy：isolated 或 explicit-inherit。
- inheritedResources：明确路径和类型。
- credentialProfileId：OS credential store 引用。
- verifiedPiPath、verifiedPiVersion：诊断投影，不作为可信授权。

## 7. 数据迁移规则

- 迁移在 SQLite transaction 内幂等执行。
- 原始 workspace_state 在迁移成功前不修改。
- agent/data 新表从空状态开始，不从历史 DevelopmentRecord 伪造聊天消息。
- 历史 development.resultNote 保持原字段，不冒充 Agent 输出。
- 旧 task context 的 context.json、files、images 导入新 Task Workspace 后保留旧目录或备份标记，直到用户明确清理。
- 每项映射记录 migration key、source revision、completed_at 和 warning。
- 任何单任务迁移失败不删除原任务；该任务保持 legacy/read-only 并展示错误。

## 8. 删除与保留清单

最终删除：

- FindOMP 和运行时 OMPAnalyzer。
- analystCommand 返回 omp 的分支。
- OMP 专属 flag 生成。
- UI 的 PI / oh-my-pi 和 OMP 登录说明。
- production tests 对 omp fake executable 的依赖。

保留但迁移：

- CandidateAnalysis 的纯 JSON 解析和业务验证。
- 需求固定输入、来源 fragment、人工确认门禁。
- 日报输入清洗、输出上限和严格解析。
- PISettings 中用户可理解的 model、thinkingEffort、timeoutMinutes。
- 历史 ADR/release note 作为不可改写的历史记录。

不迁移：

- OMP Session id、ready frame、host tool、chunk、approval mode、rules、LSP、PTY、title 等专属合同。

## 9. 验收

自动检查至少包括：

~~~text
rg -n 'exec.LookPath\("omp"\)|exec.Command.*"omp"|PI / oh-my-pi|@oh-my-pi|\.omp' \
  --glob '!docs/ADR/**' \
  --glob '!docs/RELEASE_NOTES_*.md' \
  --glob '!docs/pi-agent-workbench/PI_MIGRATION_FROM_OMP.md'
~~~

预期生产代码无命中。另需：

- fake pi RPC 测试覆盖 utility analysis。
- EngineStatuses 显示 /opt/homebrew/bin/pi 和 0.82.1 或当前实际值。
- PI 缺失时明确降级，不查找 omp。
- 旧 workspace JSON 中 pi 设置可读取。
- 人工查看 UI 不再出现 OMP 目标运行时文案。
