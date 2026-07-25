# BTaskAssistant v0.2.2

这是 Plane 收集箱的简化配置版本。首次连接只需填写工作区页面地址和
Access Token，工作区与项目技术字段由客户端自动解析和加载。

## 改进

- 初始配置只显示“服务地址”和“Access Token”。
- 服务地址可直接粘贴浏览器中的 Plane 工作区页面地址，例如
  `https://plane.fymyriad.com/myriad/`。
- 鉴权成功后自动识别 workspace slug，并列出可访问项目供下拉选择。
- 不再要求用户填写 Workspace slug、Project UUID 或项目标识。
- Token 先完成真实鉴权，再保存到系统凭据库。
- 自动读取并迁移 v0.2.1 按“实例 + 工作区”保存的旧凭据，无需重新输入。
- 支持把 Plane Cloud 的 `app.plane.so` 工作区地址映射到 API 主机。

## 安全与人工确认

- PAT 不进入 Zustand、SQLite、日志、Git 或发布包。
- 远端实例只允许 HTTPS；HTTP 仍仅允许本机回环地址。
- Plane 拉取与 PI 提炼只产生候选。只有用户点击“人工确认并转为任务”，
  才会创建正式任务；本版没有放宽任何状态机门禁。

## 验证

- Go 全量测试及 `go vet ./...`
- 14 项前端测试
- TypeScript 类型检查
- Vite 生产构建
