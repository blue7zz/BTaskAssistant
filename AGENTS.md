# BTaskAssistant 仓库执行规范

## 1. 项目目标

BTaskAssistant 是一个本地控制、人工审批、远端优先执行的桌面任务工作流客户端。它负责保存资料和事实、控制状态、调用 PI/OMP 或 Codex、收集执行结果，并阻止未经批准的内容进入下一阶段。

本项目不是“自动完成一切”的 Agent。任何实现都必须维护以下边界：

1. AI 不得批准需求或开发提示词。
2. AI 不得直接把任务标记为完成。
3. 系统状态机是唯一流程真相；前端、模型文本和外部任务状态都不能绕过它。
4. 批准版本不可原地修改；变化创建新版本并使下游授权失效。
5. 项目源码默认在 GitHub Work/Codex 的临时环境中处理，用户本地不长期保存源码。

## 2. 指令与文档优先级

执行任务时按以下顺序读取：

1. 当前目录及父目录适用的 `AGENTS.override.md` / `AGENTS.md`。
2. 用户本次明确批准的任务目录：`docs/codex-tasks/<task-slug>/`。
3. [PRODUCT_REQUIREMENTS.md](docs/PRODUCT_REQUIREMENTS.md) 的产品行为。
4. [WORKFLOW_STATE_MACHINE.md](docs/WORKFLOW_STATE_MACHINE.md) 的状态和守卫。
5. [DATA_MODEL.md](docs/DATA_MODEL.md)、[EXECUTOR_INTEGRATION.md](docs/EXECUTOR_INTEGRATION.md) 与 [SOFTWARE_ARCHITECTURE.md](docs/SOFTWARE_ARCHITECTURE.md) 的技术约束。
6. [DECISIONS.md](docs/DECISIONS.md) 的已确认决策。

文档冲突若会改变产品行为、数据契约、权限、验收标准或迁移方案，立即停止并列出冲突；不要自行选择解释。当前代码只能说明“现状”，不能反向改写已确认需求。

## 3. 仓库与本地源码策略

- GitHub 是源码、迁移、测试和正式文档的长期真相源。
- 日常开发主要在 GitHub 关联的 Work/Codex 云端或隔离工作区完成。
- 不要求用户电脑长期克隆本仓库或目标项目。
- BTaskAssistant 本地数据目录只能保存资料、文档、图片、索引、日志、差异、测试结果、导出物和必要配置，不得保存长期项目检出。
- 本地执行只能使用用户已存在的项目目录，或 OS 临时目录中的一次性检出。临时检出不得被移动到应用数据目录；清理失败必须可见并可重试。
- 不把完整源码、凭据或未获准资料嵌入日志、实施报告或提示词。

## 4. 计划目录结构

仓库当前可能只有文档。代码骨架必须由获批任务逐步建立，目标结构为：

```text
BTaskAssistant/
├── AGENTS.md
├── README.md
├── docs/
│   ├── PRODUCT_REQUIREMENTS.md
│   ├── REQUIREMENT_INTERVIEW_FLOW.md
│   ├── SOFTWARE_ARCHITECTURE.md
│   ├── DATA_MODEL.md
│   ├── EXECUTOR_INTEGRATION.md
│   ├── WORKFLOW_STATE_MACHINE.md
│   ├── CODEX_IMPLEMENTATION_PROMPT.md
│   ├── DECISIONS.md
│   └── codex-tasks/<task-slug>/
├── cmd/btaskassistant/
├── desktop/frontend/
├── internal/domain/
├── internal/application/
├── internal/infrastructure/
├── internal/migrations/
└── scripts/
```

不要为了匹配目标树一次性创建空目录或大而全骨架。只创建当前获批阶段需要的最小结构。

## 5. 手动阶段工作流

三个阶段必须由用户分别显式触发，禁止自动串联。

### 5.1 `$requirement-planning`

- 只读核验代码和证据。
- 只在 `docs/codex-tasks/<task-slug>/` 创建或更新 `REQUIREMENTS.md`、`EXEC_PLAN.md`、`DEVELOPMENT_PROMPT.md`。
- 有阻塞性待确认项时只更新 `REQUIREMENTS.md` 并停止。
- 不修改业务代码、测试、依赖、配置、数据库或生成文件。

### 5.2 `$scoped-development`

- 必须读取已批准且无阻塞项的 `REQUIREMENTS.md` 与 `EXEC_PLAN.md`。
- 只做可映射到需求或验证的最小改动。
- 完成直接验证并生成 `IMPLEMENTATION_REPORT.md` 与 `SELF_TEST_PROMPT.md` 后停止。
- 不重新定义需求，不自动进入自测。

### 5.3 `$targeted-self-test`

- 必须读取需求、计划、实施报告和当前 diff。
- 先复现和形成可验证根因，再允许最小修复。
- 自动修复最多两轮；通常不超过三个直接相关文件。
- 生成 `TEST_REPORT.md` 后停止，等待人工决定是否完成。

## 6. 状态机不变量

- 未人工确认候选任务：保持 `INBOX`。
- 未绑定 `Project` 或没有获准上下文：不得开始需求分析。
- AI 认为信息充分：只能进入 `AI_SUGGESTED_READY`，不得批准需求。
- 普通批准存在开放 `BLOCKING` 问题时必须失败。
- 强制推进需要用户逐项处理未决问题，再以独立动作批准为 `APPROVED_WITH_RISKS`。
- 未批准 `RequirementRevision`：不得生成可执行开发授权。
- 未批准 `PromptRevision`：不得启动开发。
- 执行器成功结束：只进入 `WAITING_REVIEW`。
- 审查意见未经用户接受：不得启动修复。
- 最终 `COMPLETED` 只允许用户命令触发。

任何状态改变都必须通过应用服务命令、后端守卫、事务和追加式事件完成。不要提供通用 `setStatus` API。

## 7. AI 与工具权限

| 阶段 | 项目读取 | 项目写入 | 命令 | 子 Agent | 说明 |
| --- | --- | --- | --- | --- | --- |
| 资料提取 | 仅获准资料 | 禁止 | 禁止 | 禁止 | 结果均为候选 |
| 需求规划/访谈 | 只读快照 | 禁止 | 只读检查 | 禁止 | 不得替用户回答 |
| 提示词生成 | 只读批准版本 | 禁止 | 禁止 | 禁止 | 只引用固定需求版本 |
| 开发 | 获准工作区 | 允许范围内写入 | 允许白名单验证 | 默认禁止 | 只能执行冻结输入包 |
| 审查 | 只读 diff/代码 | 默认禁止 | 允许测试和 Git 只读命令 | 可显式启用 reviewer | 必须独立于开发会话 |
| 定向修复 | 获准工作区 | 仅获准 finding 范围 | 允许直接验证 | 默认禁止 | 最多两轮，不自动完成 |

权限必须由宿主和执行环境强制，不得只依赖提示词。

## 8. 编码原则

- 先核验目标、现状和约束；遇到会改变结果的歧义立即停止提问。
- 采用满足当前需求的最小方案，不为未来假设建立抽象。
- 每一行改动都应能映射到获批需求、根因或验证。
- 保持现有风格，不重构、改名、格式化或清理无关代码。
- 未明确批准时，不新增生产依赖，不改公共 API、协议、数据库或构建基础设施。
- 不覆盖、回退或混入其他人的未提交改动。
- 不通过删除测试、跳过测试、放宽断言、吞异常或隐藏日志制造通过。
- 固定程序可可靠完成的解析、哈希、状态守卫和校验，不交给 AI 决策。

## 9. 测试要求

按风险选择最窄有效验证，并如实区分通过、失败、未执行和环境阻塞：

1. Go：领域状态机和守卫单元测试；应用服务事务与幂等测试；SQLite 迁移/仓库集成测试。
2. 前端：Bridge Mock 契约、人工确认按钮、状态恢复和关键交互测试。
3. 执行器：协议录制回放、无效帧、取消、超时、崩溃恢复和权限拒绝测试。
4. 端到端：至少证明 AI 无法批准需求、开发完成无法自动完成任务、强制推进保留风险。
5. 需要真实 GitHub、PI/OMP、Codex、设备或人工操作的验证必须单独标记，不得用 Mock 结果冒充。

## 10. Git 与提交规则

- 开始前检查目标分支、基准 SHA、工作区状态和适用规则。
- 默认使用任务分支或隔离工作区；只有用户明确要求时才直接修改 `main`。
- 提交只包含当前任务文件；不要使用会混入无关改动的宽泛暂存。
- 提交前审计完整 diff，检查秘密、临时日志、生成物和无关格式变化。
- 提交信息简洁并描述结果，例如 `docs: consolidate product workflow specification`。
- 推送或更新远端后，重新读取远端分支 SHA 并核对提交文件。
- 不自动合并、推送额外分支、删除分支、清理用户工作区或改变仓库设置。

## 11. 必须停止并请求人工决定的情况

- 正式文档之间存在实质冲突。
- 目标行为、范围、验收标准或异常策略仍有多种解释。
- 需要新增生产依赖或更改公共协议、数据库、认证、安全边界。
- 需要跨越当前已批准阶段或显著扩大文件范围。
- 远端基准已变化，无法证明提交是快进更新。
- 验证失败源自历史问题、环境问题或需求冲突，而不是当前改动。
- 需要使用未获准资料、发送敏感内容或提高执行器权限。

## 12. 完成定义

开发 Agent 的“完成”只表示当前运行产生了结构化结果、diff 和真实验证记录。任务只有在用户审查结果并明确执行最终验收命令后，才可进入 `COMPLETED`。

