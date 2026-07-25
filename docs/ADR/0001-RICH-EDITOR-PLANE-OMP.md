# ADR 0001：富文本、Plane 收集与 OMP 接入

状态：已采用

## 背景

BTaskAssistant 需要同时满足：

- 任务正文有接近 macOS 备忘录的图文混排体验。
- 内容可作为 Markdown 保存和继续编辑。
- 从已部署的 Plane 实例收集工作项。
- 固定流程由脚本完成，语义提炼交给 PI，但任何候选都必须人工确认。
- 后续可以继续深入开发 OMP 集成。

## 决策

### 1. 使用 MDXEditor

采用 `@mdxeditor/editor`：

- React 18/19 兼容，MIT。
- 直接接收和输出 Markdown 字符串。
- 支持所见即所得、Markdown 源码、表格、链接、图片粘贴和拖放。
- 不需要把 HTML 当作持久化真相再反向转换。

未选择：

- BlockNote：默认体验成熟，但核心包采用 MPL-2.0，且本项目更需要 Markdown
  字符串作为稳定交换格式。
- Milkdown：Markdown 原生且扩展性强，但当前 UI 组装和二次定制成本高于
  MDXEditor。
- Tiptap：底层成熟，但 Markdown 往返和完整 UI 需要更多自行组装。

### 2. Plane API 由 Go 后端访问

前端不直接请求 Plane，避免 CORS 和 PAT 暴露。Go 客户端：

- 只允许 HTTPS；HTTP 仅允许 localhost/回环地址。
- 通过 `X-API-Key` 发送 PAT。
- 最多每页 100 条，按 cursor 拉取，设置 5000 条安全上限。
- 保留 Plane ID、编号、状态、优先级、标签、负责人、时间和原始描述。
- 以 Plane 工作项 ID 去重。

PAT 使用 `go-keyring` 存入系统凭据库，不进入 Zustand、SQLite 或日志。

### 3. OMP 使用受限的外部进程适配器

目标架构仍采用 OMP RPC。当前收集纵向切片先调用本机 `omp --print`，同时关闭：

- tools
- skills
- rules
- extensions
- title
- session

OMP 只返回结构化候选，不读项目、不写文件、不运行命令。输出必须解析为固定
JSON，解析失败则整次失败，不使用模糊兜底。正式任务转换仍由用户点击确认。

没有把整个 oh-my-pi 仓库复制进本仓库：

- 官方已经提供稳定 CLI、RPC 和 Node SDK 三种嵌入边界。
- 复制源码会让跨平台发布、原生依赖和上游同步成本无必要地扩大。
- oh-my-pi 为 MIT，未来若确实需要源码级二次开发，可在独立适配层固定上游
  commit，再决定使用 fork、subtree 或 SDK sidecar。

## 后果

- 现有纯文本 `summary` 自动成为合法 Markdown，不需要破坏性迁移。
- data URL 图片适合当前 MVP；大量图片需要迁移到内容寻址资料存储。
- 用户必须自行安装并登录 OMP，才能使用 PI 提炼；不影响脚本收集和人工确认。
- 开发执行仍保持外部委托，直到完整 OMP RPC 权限和恢复机制实现。

## 上游资料

- MDXEditor: <https://github.com/mdx-editor/editor>
- Plane API: <https://developers.plane.so/api-reference/introduction>
- oh-my-pi: <https://github.com/can1357/oh-my-pi>
- go-keyring: <https://github.com/zalando/go-keyring>
