# BTaskAssistant 决策记录

> 状态：v1.0  
> 日期：2026-07-28  
> 说明：本文件记录已确认决策和明确待决事项。待决项不能由实现 Agent 自行选择。

## 已确认决策

| ID | 决策 | 原因与影响 |
| --- | --- | --- |
| D-001 | 产品是“状态机控制的任务工作流”，不是自主 Agent | 流程由系统和用户控制，执行器可替换 |
| D-002 | AI 不能批准需求、提示词或任务完成 | 所有正式授权都有独立人工命令和审计记录 |
| D-003 | 资料、任务和需求版本是三个独立对象 | 支持一份资料多个任务、一个任务多份资料及来源追踪 |
| D-004 | 原始资料先保存，固定程序优先解析 | 解析失败不丢资料，减少 AI 成本与不确定性 |
| D-005 | AI 提取内容在人工确认前均为候选 | OCR、视觉理解和语义分类不能直接成为正式需求 |
| D-006 | 每条正式事实必须有来源 | 无来源内容只能进入建议或问题 |
| D-007 | 需求整理采用多轮 AI 访谈 | AI 基于资料和项目证据发现缺口，用户回答和决定结束时机 |
| D-008 | AI“没有疑问”只是一项建议 | 状态进入 `AI_SUGGESTED_READY`，仍需人工批准 |
| D-009 | 用户可以强制推进 | 必须逐项处理未决问题，之后独立批准为 `APPROVED_WITH_RISKS` |
| D-010 | 批准版本不可变 | 新资料或修改创建新版本并使旧提示词/授权失效 |
| D-011 | PI/OMP 是内置首选执行器，Codex 是可选执行器 | 两者实现统一能力、输入、事件和结果契约 |
| D-012 | OMP 使用正式 RPC，不模拟终端 | 采用 `omp --mode rpc` 的 stdio JSONL 协议和关联 ID |
| D-013 | Codex 深度客户端集成优先使用公开的 app-server；批处理可用 `codex exec` | 不解析交互 TUI；具体适配能力按运行时协商 |
| D-014 | 阶段使用独立会话与权限 | 需求只读、开发可写、审查默认只读、修复限于 finding |
| D-015 | 三个 Skill 阶段由用户手动触发 | `requirement-planning`、`scoped-development`、`targeted-self-test` 禁止自动串联 |
| D-016 | 开发结束与任务完成分离 | 成功运行进入 `WAITING_REVIEW`，最终完成只由用户触发 |
| D-017 | AI 审查独立于开发会话 | 审查只看批准需求、diff、必要代码和真实测试结果 |
| D-018 | 定向自动修复默认关闭，显式授权后最多两轮 | 不改变需求、不扩大范围、不能自动完成任务 |
| D-019 | 客户端采用 Wails v2 + Go + React/TypeScript/Vite | 参考 DeepSeek-Reasonix 的桌面壳、Go 内核、typed bridge 和事件模式 |
| D-020 | SQLite 是本地业务真相，文件系统保存内容寻址资料 | 状态/审批使用事务，资料用 SHA-256 去重和版本化 |
| D-021 | 前端不是审批真相源 | Zustand 只管理交互状态；Go 应用/领域层执行守卫 |
| D-022 | GitHub 是源码的长期真相源 | 开发主要在 GitHub Work/Codex 的临时环境完成 |
| D-023 | 用户本地不长期保存项目源码 | 应用目录只保留资料、文档、运行产物和必要配置 |
| D-024 | 本地代码执行是显式例外 | 只能引用已有目录或使用带 TTL 的 OS 临时检出并报告清理结果 |
| D-025 | 第一版外部任务集成以单向导入为主 | 避免双向状态冲突和批准内容被外部覆盖 |
| D-026 | 默认不启用多 Agent 并行开发 | 只有可证明互不冲突且用户明确授权时使用 |
| D-027 | 状态变化和事件写入必须原子化 | 支持审计、幂等、恢复和并发保护 |
| D-028 | “本地优先”是控制数据本地，不等于所有计算离线 | 远端 AI/Work 仅接收用户批准的最小输入包 |

## 参考实现事实

以下只是技术参考，不自动继承其业务行为：

- [DeepSeek-Reasonix `main-v2`](https://github.com/esengine/DeepSeek-Reasonix/tree/main-v2) 使用 Wails v2、Go、React、TypeScript、Vite、pnpm，并以 bridge 隔离 Wails 绑定与浏览器 Mock。
- [OMP RPC 文档](https://github.com/can1357/oh-my-pi/blob/main/docs/rpc.md) 定义 stdio JSONL、ready 帧、协议协商、关联 ID、事件和 host tool/URI 子协议。
- [Codex app-server](https://learn.chatgpt.com/docs/app-server) 面向富客户端集成，提供认证、会话、审批和流式事件。
- [Codex 非交互模式](https://learn.chatgpt.com/docs/non-interactive-mode) 支持 `codex exec`、JSONL、输出 schema、沙箱和会话恢复。
- [Codex 云端环境](https://learn.chatgpt.com/docs/environments/cloud-environment) 在临时容器检出指定分支或 SHA，并读取仓库 `AGENTS.md`。

## 明确待决事项

这些问题在实施对应阶段前需要单独任务和人工批准：

| ID | 待决事项 | 决策时点 |
| --- | --- | --- |
| O-001 | SQLite Go 驱动与迁移库 | 数据持久化阶段 |
| O-002 | 本地 OCR、远端视觉模型及格式支持的实际组合 | 资料解析阶段 |
| O-003 | 应用数据是否采用整库/逐文件加密及密钥恢复策略 | 安全设计阶段 |
| O-004 | BTaskAssistant 如何通过公开、稳定接口创建和跟踪 Codex 云端任务 | Codex 远端适配阶段；无稳定接口时保留人工 handoff |
| O-005 | PI/OMP 是随应用捆绑、按版本下载还是发现系统安装 | OMP 分发阶段 |
| O-006 | 远端运行产物的默认保留期和清理 SLA | 运行恢复阶段 |
| O-007 | GitHub OAuth/GitHub App 的权限模型 | GitHub 集成阶段 |
| O-008 | 外部任务面板的首个具体供应商 | 外部集成阶段 |
| O-009 | 发布签名、自动更新和遥测策略 | 桌面发布阶段 |
| O-010 | 是否支持无项目资料的纯需求任务 | 需求会话实现前；当前默认要求项目或显式资料模式 |

## 决策更新规则

- 已确认决策发生变化时，新增一条 superseding 记录，不静默改写历史语义。
- 实现 Agent 发现决策缺口时，在任务 `REQUIREMENTS.md` 标记阻塞并停止。
- “参考项目这样做”“行业通常这样做”或“模型认为更好”都不能替代决策。

