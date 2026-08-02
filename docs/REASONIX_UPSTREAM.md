# Reasonix 上游维护记录

## 上游信息

| 项 | 值 |
|---|---|
| 上游项目 | DeepSeek-Reasonix（main-v2 快照） |
| 来源 | `/Users/blue/Downloads/DeepSeek-Reasonix-main-v2` |
| 版本 | 0.0.0（前端 package.json） |
| 许可证 | MIT（Reasonix Contributors, 2026） |
| LICENSE 校验值 | `dc024237821ac82056c37f8d82e3be919bd51e39a4529ec12a8ab3e2a346dc4c`（与本仓 reasonix-app/LICENSE 一致） |
| 集成形态 | 源码全量同仓（`reasonix-app/`），BTask 以 `replace reasonix => ./reasonix-app` 引用 Go 内核；前端经动态 import 纳入 BTask 同一 vite 构建 |

## 补丁清单（相对上游的改动）

目的：嵌入宿主（BTask）适配 + 安全/契约加固。全部改动保持独立应用
（/Applications/Reasonix.app）可独立构建运行。

### 前端（desktop/frontend/src，37 修改 + 3 新增）

- **嵌入入口（新增）**：`embedEntry.tsx`（shadow DOM 挂载）、
  `lib/embedHost.ts`（DOM adapter：rxQuerySelector/rxBody/rxActiveElement/
  rxPortalTarget/rxKeyActive/rxDirectCall）、`generated/scoped-styles.css`
  （PostCSS 预构建 :host 样式 + seti 字体内联）。
- **桥**（lib/bridge.ts）：host 模式 postMessage 桥 → embed 直连
  window.go；onHostTabActivated（单实例任务切换）；onRuntimeRebuilt 的
  embed 分支；openExternal embed 直连。
- **样式/资源**：styles.css 追加宿主提示样式；main.tsx 承接 heartbeat.css
  （App.tsx 移除——嵌入时进 shadow）。
- **DOM 操作适配**（18 个文件）：portal 落点 → rxPortalTarget；
  documentElement → rxRootElement；activeElement → rxActiveElement；
  querySelector → rxQuerySelector；body 样式 → rxBody。
- **快捷键隔离**：CommandPalette / keyboardShortcuts 全局 keydown 加
  rxKeyActive 守卫。
- **设置面板**：host 模式移除桌面功能 tabs（bots/mcp/remote/plugins/
  sandbox/network/hooks/updates）+ 顶部宿主提示。
- **更新逻辑移除**：useUpdater host 短路；App.tsx UpdateBanner host 条件
  渲染；SETTINGS_TABS 移除 updates。

### 后端（desktop）

- `updater_app.go`：CheckUpdate/openDownloadPage 禁用（不访问更新清单）。

### Go 内核（internal/）

未修改（原样引用）。宿主侧适配全部在 `reasonix-bridge/` 与 `rx_bindings*.go`。

## 差异报告

与上游快照的逐文件差异（40 个文件）：

| MOD | frontend/src/App.tsx |
| MOD | frontend/src/__tests__/context-window-ring.test.tsx |
| MOD | frontend/src/components/CommandPalette.tsx |
| MOD | frontend/src/components/Composer.tsx |
| MOD | frontend/src/components/ConfirmDialog.tsx |
| MOD | frontend/src/components/CopyButton.tsx |
| MOD | frontend/src/components/FloatingMenu.tsx |
| MOD | frontend/src/components/ImageViewer.tsx |
| MOD | frontend/src/components/MermaidDiagram.tsx |
| MOD | frontend/src/components/ProjectTree.tsx |
| MOD | frontend/src/components/RichComposerInput.tsx |
| MOD | frontend/src/components/SettingsPanel.tsx |
| MOD | frontend/src/components/ShortcutsCheatsheet.tsx |
| MOD | frontend/src/components/ThemeGallery.tsx |
| MOD | frontend/src/components/Tooltip.tsx |
| MOD | frontend/src/components/Transcript.tsx |
| MOD | frontend/src/components/TranscriptSelectionMenu.tsx |
| MOD | frontend/src/components/TypographySettings.tsx |
| MOD | frontend/src/custom/features/heartbeat/HeartbeatPanel.tsx |
| NEW | frontend/src/embedEntry.tsx |
| NEW | frontend/src/generated/scoped-styles.css |
| MOD | frontend/src/lib/bridge.ts |
| MOD | frontend/src/lib/clipboard.ts |
| MOD | frontend/src/lib/compat.ts |
| MOD | frontend/src/lib/conversationWidth.ts |
| NEW | frontend/src/lib/embedHost.ts |
| MOD | frontend/src/lib/fontAvailability.ts |
| MOD | frontend/src/lib/fontFamily.ts |
| MOD | frontend/src/lib/i18n.tsx |
| MOD | frontend/src/lib/keyboardShortcuts.ts |
| MOD | frontend/src/lib/textSize.ts |
| MOD | frontend/src/lib/theme.ts |
| MOD | frontend/src/lib/themePack.ts |
| MOD | frontend/src/lib/typographyPreferences.ts |
| MOD | frontend/src/lib/useUpdater.ts |
| MOD | frontend/src/lib/useWailsResizeFix.ts |
| MOD | frontend/src/lib/windowState.ts |
| MOD | frontend/src/main.tsx |
| MOD | frontend/src/styles.css |
| MOD | updater_app.go |

## 维护约定

- 上游内核升级：替换 `reasonix-app/internal/` 后运行 `scripts/test-all.sh`。
- 前端升级：替换 `reasonix-app/desktop/frontend/src` 后重新生成
  `generated/scoped-styles.css`（`node ../scripts/scope-rx-css.mjs`）。
- 新增宿主绑定必须过 `TestBridgeContractArgCounts`（契约参数数校验）。
