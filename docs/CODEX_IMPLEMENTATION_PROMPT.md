# Codex 实施交接提示词

> 用途：交给 Work/Codex 分阶段实现 BTaskAssistant。  
> 规则：一次只复制并执行一个阶段；每个阶段结束后等待人工审查，禁止自动串联。

## 第一次：需求规划（只读，不写业务代码）

```text
$requirement-planning

仓库：blue7zz/BTaskAssistant
目标分支基线：执行时读取 main 的最新 SHA，并固定到任务文档。
任务目录：docs/codex-tasks/bootstrap-domain-state-machine/

为 BTaskAssistant 的第一实现阶段制定可审查计划：仅建立最小 Go 项目骨架、Task/Requirement 核心领域枚举、命令式状态转换、普通与带风险批准守卫、事件契约及纯 Go 单元测试。

必须阅读并以以下文件为正式来源：
- AGENTS.md
- docs/PRODUCT_REQUIREMENTS.md
- docs/WORKFLOW_STATE_MACHINE.md
- docs/DATA_MODEL.md
- docs/SOFTWARE_ARCHITECTURE.md
- docs/EXECUTOR_INTEGRATION.md
- docs/DECISIONS.md

本阶段明确不做 SQLite、Wails/React UI、资料解析、GitHub 集成、真实 PI/OMP、真实 Codex、审查/修复实现和发布配置。发现需要新增生产依赖、改变状态名/守卫、扩大阶段或存在文档冲突时，记录为阻塞问题并停止。

只生成 REQUIREMENTS.md、EXEC_PLAN.md 和简短 DEVELOPMENT_PROMPT.md；存在阻塞问题时只更新 REQUIREMENTS.md。完成后停止，等待人工批准。
```

## 第二次：范围受控开发（仅在用户批准任务文档后）

```text
$scoped-development

任务目录：docs/codex-tasks/bootstrap-domain-state-machine/
仅根据已审查的 REQUIREMENTS.md 和 EXEC_PLAN.md 实施。
完成直接验证并生成 IMPLEMENTATION_REPORT.md 与 SELF_TEST_PROMPT.md 后停止。
不要自动进入自测，不要创建未批准的后续阶段。
```

## 第三次：定向自测（仅在用户审查 diff 后）

```text
$targeted-self-test

任务目录：docs/codex-tasks/bootstrap-domain-state-machine/
根据已批准需求、执行计划、实施报告和当前 diff 做定向验收。
先复现和证明根因，再允许最多两轮最小修复。
生成 TEST_REPORT.md 后停止；不得自动标记任务完成、合并或开始下一实施阶段。
```

## 后续实施阶段

每个阶段建立新的 `docs/codex-tasks/<task-slug>/` 并重新走上述三次手动调用。建议依赖顺序：

1. `bootstrap-domain-state-machine`
2. `sqlite-application-services`
3. `project-github-context`
4. `material-ingestion-parsing`
5. `requirement-interview-mock`
6. `wails-react-workspace`
7. `omp-rpc-adapter`
8. `codex-adapter-remote-handoff`
9. `development-review-fix-loop`
10. `recovery-security-packaging`

阶段名只是导航建议；每个阶段的产品范围和依赖仍需 `$requirement-planning` 核验并由用户批准。

## 始终不可违反

- AI 不能批准需求、提示词或任务完成。
- 开发结果只能进入待审查。
- 用户本地不长期保存源码；默认使用 GitHub 关联远端临时环境。
- 不根据参考项目或当前代码自行改写正式需求。
- 不做无关重构、格式化、依赖升级或跨阶段实现。
- 未执行的测试必须标为未执行，Mock 不能冒充真实 PI/Codex/GitHub 验证。

