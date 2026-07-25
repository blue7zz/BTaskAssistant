# BTaskAssistant v0.2.0

这是在可运行 MVP 基础上的第二个预发布版本，新增富文本任务正文和 Plane
任务收集纵向切片，同时继续保留不可绕过的人工确认门禁。

## 新增

- Markdown 原生富文本编辑器：
  - 所见即所得与 Markdown 源码切换。
  - 标题、列表、引用、链接、表格和分隔线。
  - 粘贴或拖放图片，单张图片最大 4 MB。
- Plane 收集箱：
  - 支持 Plane Cloud 和 HTTPS 自托管实例。
  - 使用 Personal Access Token 和 `X-API-Key`。
  - 自动分页、工作项 ID 去重和原始来源保留。
  - 待确认、已转换、已忽略三个候选视图。
- PI / oh-my-pi 候选提炼：
  - 检测本机 `omp`。
  - 以无工具、无技能、无规则、无扩展、无会话模式运行。
  - 只输出标题、Markdown 正文、关键信息和待确认问题候选。
- 安全：
  - Plane PAT 保存到系统凭据库，不进入 SQLite 或前端状态。
  - 远端服务强制 HTTPS，HTTP 只允许 localhost。
  - AI 结果不能自动创建任务或推进状态。

## 人工确认规则

Plane 拉取或 PI 提炼完成后，内容仍处于候选状态。只有用户点击
“人工确认并转为任务”才会创建正式任务；Plane 原始工作项会作为独立需求来源保存。

## 验证

- Go 全包测试与 `go vet`
- 前端 TypeScript 类型检查
- 前端 13 项测试
- Vite 生产构建
- 浏览器真实交互：
  - 富文本/Markdown 切换并持久化
  - Plane 候选确认后只创建一条正式任务

## 首次启动

当前构建仍未使用 Apple Developer 或 Windows 代码签名证书：

- macOS：解压后右键 `BTaskAssistant.app`，选择“打开”。
- Windows：若 SmartScreen 提示，选择“更多信息 → 仍要运行”。
