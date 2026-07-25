# BTaskAssistant v0.2.1

这是 Plane 收集箱的连接体验修订版，已使用真实的自托管 Plane
接口验证项目发现、工作项统计和分页兼容性。

## 改进

- 为当前部署预填非敏感连接信息：
  - 服务地址 `https://plane.fymyriad.com`
  - Workspace slug `myriad`
  - 项目 `MYRIA · myriad`
- 保存 PAT 后自动读取工作区项目，不再要求用户手工填写 Project UUID。
- 项目改为下拉选择，支持刷新工作区中的项目。
- 显示当前项目标识、名称和 UUID，连接测试会显示实际工作项数量。
- Plane API 未返回 `project_identifier` 时，使用所选项目标识生成
  `MYRIA-序号` 形式的来源编号。
- 继续使用系统凭据库保存 PAT；令牌不会进入 SQLite、日志、Git 或发布包。

## 真实连接验证

- HTTPS 与 TLS 正常。
- Personal Access Token 鉴权成功。
- 工作区 `myriad` 可发现项目 `MYRIA · myriad`。
- 工作项接口返回 262 条记录，游标分页结构与客户端兼容。

以上统计是发布前测试时的快照，之后会随 Plane 数据变化。

## 人工确认规则

Plane 拉取与 PI 提炼仍然只产生候选。只有用户点击“人工确认并转为任务”，
才会创建正式任务；本版没有放宽任何状态机门禁。
