# Reasonix 嵌入兼容矩阵

> 冻结范围：**任务内 Reasonix 工作台完整可用**（对话、流式输出、工具调用、审批/提问、
> 历史、恢复、新建、重命名、删除、分叉、回退、压缩、模型/effort/token、文件与 Diff、
> 必要的 Memory/Skills/MCP）。
>
> **不包含**（独立 Reasonix 桌面功能）：机器人、远程 SSH、自动更新、独立窗口、
> 交付工作树、终端、插件、遥测/崩溃上报。这些功能**必须从嵌入 UI 隐藏或明确禁用**，
> 不得返回假成功。
>
> 统计口径：reasonix 前端 `AppBindings` 契约（bridge.ts）共 **358 个方法**；
> 宿主绑定 `rx_bindings.go`（真实现）+ `rx_bindings_stubs.go`（假实现）。

## 汇总

| 分类 | 总数 | ✅ 真实现 | ⚠️ 假实现（空返回） | ❌ 未绑定 |
|---|---|---|---|---|
| 核心（任务工作台） | 133 | 53 | 57 | 23 |
| 桌面（嵌入不适用） | 27 | 8 | 18 | 1 |
| 其他 | 198 | 59 | 86 | 53 |
| **合计** | **358** | **120** | **161** | **77** |

处置原则：
- 核心方法：逐一清零（阶段 6）——真实现或明确禁用，禁止空返回。
- 桌面方法：从嵌入 UI 隐藏（阶段 4/6），绑定可保留为明确错误（非假成功）。
- 其他：按可见性归类（阶段 6 前逐项确认）。

## 核心方法（任务工作台，133）

| 方法 | 参数 | 返回 | 状态 | 调用点 |
|---|---|---|---|---|
| `Platform` | `—` | `Promise<string>` | ✅ 真实现 | App.tsx, components/ProjectTree.tsx |
| `Submit` | `input` | `Promise<void>` | ❌ 未绑定 | — |
| `SubmitToTab` | `tabID, input` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SubmitDisplay` | `display, input` | `Promise<void>` | ❌ 未绑定 | — |
| `SubmitDisplayToTab` | `tabID, display, input` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `SubmitDeliveryRecoveryToTab` | `tabID, display, input` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `SubmitInvocationsToTab` | `tabID, display, input, invocations` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `SubmitInitialGoalToTab` | `tabID, goal, display, input, invocations, collaborationMode, toolApprovalMode, targetKind, targetIdentityGen, targetRequestSeq, ` | `Promise<string[]>` | ✅ 真实现 | lib/useController.ts |
| `SubmitEditedDisplayToTab` | `tabID, display, input, original` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `Cancel` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `CancelTab` | `tabID` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ResolveRecovery` | `id, action, feedback` | `Promise<void>` | ❌ 未绑定 | — |
| `ResolveRecoveryTab` | `tabID, id, action, feedback` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `RecoveryCheckpointEnabled` | `—` | `Promise<boolean>` | ❌ 未绑定 | — |
| `RecoveryCheckpointEnabledTab` | `tabID` | `Promise<boolean>` | ❌ 未绑定 | — |
| `AnswerQuestion` | `id, answers` | `Promise<void>` | ❌ 未绑定 | — |
| `AnswerQuestionForTab` | `tabID, id, answers` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ResumeGoalForTab` | `tabID` | `Promise<boolean>` | ✅ 真实现 | lib/useController.ts |
| `ClearGoal` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `ClearGoalForTab` | `tabID` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `NewSession` | `—` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `NewSessionForTab` | `tabID` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ClearSession` | `—` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ClearSessionForTab` | `tabID` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `History` | `—` | `Promise<HistoryMessage[]>` | ❌ 未绑定 | — |
| `HistoryForTab` | `tabID` | `Promise<HistoryMessage[]>` | ✅ 真实现 | lib/useController.ts |
| `HistoryPage` | `beforeTurn, limit` | `Promise<HistoryPage>` | ❌ 未绑定 | — |
| `HistoryPageForTab` | `tabID, beforeTurn, limit` | `Promise<HistoryPage>` | ✅ 真实现 | lib/useController.ts |
| `HistoryCheckpointTurnsForTab` | `tabID` | `Promise<number[]>` | ✅ 真实现 | lib/useController.ts |
| `Checkpoints` | `—` | `Promise<CheckpointMeta[]>` | ❌ 未绑定 | — |
| `CheckpointsForTab` | `tabID` | `Promise<CheckpointMeta[]>` | ✅ 真实现 | lib/useController.ts |
| `Rewind` | `turn, scope` | `Promise<void>` | ❌ 未绑定 | — |
| `RewindForTab` | `tabID, turn, scope` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `Fork` | `turn` | `Promise<TabMeta>` | ❌ 未绑定 | — |
| `ForkForTab` | `tabID, turn` | `Promise<TabMeta>` | ✅ 真实现 | lib/useController.ts |
| `SummarizeFrom` | `turn` | `Promise<void>` | ❌ 未绑定 | — |
| `SummarizeFromForTab` | `tabID, turn` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SummarizeUpTo` | `turn` | `Promise<void>` | ❌ 未绑定 | — |
| `SummarizeUpToForTab` | `tabID, turn` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ListSessions` | `—` | `Promise<SessionMeta[]>` | ✅ 真实现 | components/Composer.tsx, lib/useController.ts |
| `ListTrashedSessions` | `—` | `Promise<SessionMeta[]>` | ✅ 真实现 | lib/useController.ts |
| `ResumeSession` | `path` | `Promise<HistoryMessage[]>` | ✅ 真实现 | — |
| `ResumeSessionForTab` | `tabID, path` | `Promise<HistoryMessage[]>` | ✅ 真实现 | — |
| `ResumeSessionPage` | `path, limit` | `Promise<HistoryPage>` | ✅ 真实现 | lib/useController.ts |
| `ResumeSessionPageForTab` | `tabID, path, limit` | `Promise<HistoryPage>` | ✅ 真实现 | lib/useController.ts |
| `OpenChannelSessionForTab` | `tabID, path` | `Promise<HistoryMessage[]>` | ❌ 未绑定 | — |
| `OpenChannelSessionPageForTab` | `tabID, path, limit` | `Promise<HistoryPage>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `PreviewSession` | `path` | `Promise<HistoryMessage[]>` | ✅ 真实现 | components/Composer.tsx, lib/useController.ts |
| `DeleteSession` | `path` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `RestoreSession` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `PurgeTrashedSession` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `PurgeRecoveryCopy` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx |
| `RenameSession` | `path, title` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ListWorkspaces` | `—` | `Promise<WorkspaceView[]>` | ✅ 真实现 | custom/features/heartbeat/HeartbeatPanel.tsx |
| `PickWorkspace` | `—` | `Promise<string>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `SwitchWorkspace` | `path` | `Promise<string>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `Meta` | `—` | `Promise<Meta>` | ✅ 真实现 | components/CapabilitiesPanel.tsx |
| `MetaForTab` | `tabID` | `Promise<Meta>` | ✅ 真实现 | lib/useController.ts |
| `Commands` | `—` | `Promise<CommandInfo[]>` | ✅ 真实现 | components/Composer.tsx |
| `Capabilities` | `—` | `Promise<CapabilitiesView>` | ✅ 真实现 | components/CapabilitiesPanel.tsx |
| `MCPServers` | `—` | `Promise<ServerView[]>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `MCPMarketplace` | `query` | `Promise<MCPMarketplaceView>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `MCPMarketplaceResolve` | `registryName` | `Promise<MCPMarketplaceEntry>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `SkillsSettings` | `—` | `Promise<SkillsSettingsView>` | ✅ 真实现 | components/CapabilitiesPanel.tsx, components/SubagentsPanel.tsx |
| `PlanPluginInstall` | `source, options` | `Promise<string>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `ClearMCPServerAuthentication` | `name` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `CancelTrySubagentProfile` | `—` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `SlashArgs` | `input` | `Promise<SlashArgsResult>` | ✅ 真实现 | — |
| `ListDir` | `rel` | `Promise<DirEntry[]>` | ❌ 未绑定 | — |
| `ListDirForTab` | `tabID, rel` | `Promise<DirEntry[]>` | ✅ 真实现 | components/WorkspacePanel.tsx |
| `WorkspaceChanges` | `tabID` | `Promise<WorkspaceChangesView>` | ✅ 真实现 | components/WorkspacePanel.tsx |
| `WorkspaceChangeDetail` | `tabID, path` | `Promise<WorkspaceChangeDetailView>` | ⚠️ 假实现（空返回） | components/WorkspacePanel.tsx |
| `WorkspaceGitHistory` | `tabID, path` | `Promise<GitCommitView[]>` | ⚠️ 假实现（空返回） | components/WorkspacePanel.tsx |
| `WorkspaceGitCommitDetail` | `tabID, hash, path` | `Promise<GitCommitDetailView>` | ❌ 未绑定 | — |
| `RevealWorkspacePath` | `rel` | `Promise<void>` | ❌ 未绑定 | — |
| `RevealWorkspacePathForTab` | `tabID, rel` | `Promise<void>` | ⚠️ 假实现（空返回） | components/WorkspacePanel.tsx |
| `RevealPath` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ProjectTree.tsx |
| `SavePastedImage` | `dataUrl` | `Promise<string>` | ⚠️ 假实现（空返回） | components/Composer.tsx |
| `SaveClipboardImage` | `—` | `Promise<string>` | ⚠️ 假实现（空返回） | components/Composer.tsx |
| `SavePastedFile` | `name, dataUrl` | `Promise<string>` | ⚠️ 假实现（空返回） | components/Composer.tsx |
| `SaveExportFile` | `path, payload, base64Encoded` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx |
| `SaveExportImageFiles` | `path, payloads` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx |
| `AttachmentDataURL` | `path` | `Promise<string>` | ⚠️ 假实现（空返回） | components/Composer.tsx, components/Message.tsx |
| `Models` | `—` | `Promise<ModelInfo[]>` | ✅ 真实现 | components/ModelSwitcher.tsx |
| `ModelsForTab` | `tabID` | `Promise<ModelInfo[]>` | ✅ 真实现 | components/ModelSwitcher.tsx |
| `Effort` | `—` | `Promise<EffortInfo>` | ✅ 真实现 | — |
| `EffortForTab` | `tabID` | `Promise<EffortInfo>` | ✅ 真实现 | lib/useController.ts |
| `Memory` | `—` | `Promise<MemoryView>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx, lib/useController.ts |
| `MemorySuggestions` | `—` | `Promise<MemorySuggestionsView>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `MemoryForTab` | `tabID` | `Promise<MemoryView>` | ✅ 真实现 | components/MemoryPanel.tsx |
| `MemoryRevisions` | `ref` | `Promise<MemoryFact[]>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `MemoryRevisionsForTab` | `tabID, ref` | `Promise<MemoryFact[]>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `RestoreMemoryRevision` | `ref, revision` | `Promise<MemoryFact>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `RestoreMemoryRevisionForTab` | `tabID, ref, revision` | `Promise<MemoryFact>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `MemorySuggestionsForTab` | `tabID` | `Promise<MemorySuggestionsView>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `RestoreArchivedMemory` | `archivePath` | `Promise<MemoryFact>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `RestoreArchivedMemoryForTab` | `tabID, archivePath` | `Promise<MemoryFact>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `SaveDoc` | `path, body` | `Promise<string>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx, lib/useController.ts |
| `SaveDocForTab` | `tabID, path, body` | `Promise<string>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `HooksSettings` | `scope` | `Promise<HooksSettingsView>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SaveHooksSettings` | `scope, hooks` | `Promise<void>` | ❌ 未绑定 | — |
| `SaveHooksSettingsForRoot` | `scope, projectRoot, hooks` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SaveProvider` | `p` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SaveProviderWithKey` | `p, key` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SaveProviderKey` | `apiKeyEnv, value` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `ClearProviderKey` | `apiKeyEnv` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetPermissionMode` | `mode` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `ClearBotSecret` | `envName` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `DiagnoseBotConnection` | `id` | `Promise<BotConnectionDiagnostic>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `TestBotConnection` | `id, target?` | `Promise<BotConnectionDiagnostic>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetCloseBehavior` | `mode` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `ListThemePacks` | `—` | `Promise<import("./themePack").ThemePackView[]>` | ⚠️ 假实现（空返回） | components/ThemeGallery.tsx, components/ThemeLibrary.tsx |
| `GetActiveThemePack` | `—` | `Promise<import("./themePack").ThemeActiveView>` | ✅ 真实现 | App.tsx, components/ThemeLibrary.tsx, lib/themeExperience.ts |
| `RestoreGraphiteAppearance` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `CopyThemePack` | `sourceID, newID, newName` | `Promise<import("./themePack").ThemePackView>` | ⚠️ 假实现（空返回） | components/ThemeGallery.tsx, components/ThemeLibrary.tsx, components/AppearanceOverview.tsx |
| `ImportThemePack` | `sourcePath, replace` | `Promise<import("./themePack").ThemeImportResult>` | ⚠️ 假实现（空返回） | components/ThemeGallery.tsx, components/ThemeLibrary.tsx |
| `ExportThemePack` | `id, destPath` | `Promise<string>` | ⚠️ 假实现（空返回） | components/ThemeGallery.tsx, components/ThemeLibrary.tsx |
| `MigrateDesktopPreferences` | `language, theme, style` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx |
| `Version` | `—` | `Promise<string>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `ConnectKey` | `apiKey` | `Promise<string>` | ⚠️ 假实现（空返回） | components/OnboardingOverlay.tsx |
| `ListTabs` | `—` | `Promise<TabMeta[]>` | ✅ 真实现 | App.tsx, components/MemoryPanel.tsx, components/CapabilitiesPanel.tsx |
| `EnsureBlankTab` | `scope, workspaceRoot` | `Promise<TabMeta>` | ✅ 真实现 | lib/useController.ts |
| `EnsureBlankSurface` | `scope, workspaceRoot` | `Promise<TabMeta>` | ✅ 真实现 | lib/useController.ts |
| `RenameTerminalForTab` | `tabID, sessionID, title` | `Promise<void>` | ✅ 真实现 | store/terminal.ts |
| `ListProjectTree` | `—` | `Promise<ProjectNode[]>` | ✅ 真实现 | App.tsx, components/ProjectTree.tsx |
| `RenameProject` | `workspaceRoot, title` | `Promise<void>` | ✅ 真实现 | components/ProjectTree.tsx |
| `RenameTopic` | `topicID, title` | `Promise<void>` | ✅ 真实现 | App.tsx, components/ProjectTree.tsx |
| `TrashTopic` | `topicID` | `Promise<void>` | ✅ 真实现 | components/ProjectTree.tsx |
| `ScanSSHConfig` | `—` | `Promise<RemoteHostInput[]>` | ⚠️ 假实现（空返回） | components/RemoteHostsPage.tsx |
| `ConnectRemoteHost` | `id` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx, components/RemotePanel.tsx, components/RemoteHostsPage.tsx |
| `ListRemoteDir` | `hostId, path` | `Promise<RemoteDirEntry[]>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx |
| `RenameRemotePath` | `hostId, oldPath, newPath` | `Promise<void>` | ❌ 未绑定 | — |
| `EnsureRemoteServer` | `hostId, workspace` | `Promise<void>` | ❌ 未绑定 | — |

## 桌面功能（嵌入不适用，27——应从 UI 隐藏/禁用）

| 方法 | 参数 | 返回 | 状态 | 调用点 |
|---|---|---|---|---|
| `Plugins` | `—` | `Promise<PluginView[]>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `UpdatePlugin` | `name` | `Promise<string>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `PluginDoctor` | `name` | `Promise<PluginView>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `UpdateMCPServer` | `name, input` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `UpdateSubagentProfile` | `name, scope, input` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `DesktopStartupSettings` | `—` | `Promise<DesktopStartupSettingsView>` | ✅ 真实现 | App.tsx |
| `BotRuntimeStatus` | `—` | `Promise<BotRuntimeStatusView>` | ⚠️ 假实现（空返回） | App.tsx |
| `DownloadUpdate` | `channel` | `Promise<UpdateDownloadResult | null>` | ❌ 未绑定 | — |
| `DownloadUpdateRequest` | `channel, expectedVersion, requestId` | `Promise<UpdateDownloadResult | null>` | ⚠️ 假实现（空返回） | lib/useUpdater.ts |
| `DeliveryWorktreeAvailability` | `workspaceRoot` | `Promise<DeliveryWorktreeAvailability>` | ✅ 真实现 | components/ProjectTree.tsx |
| `TerminalWorkspaceForTab` | `tabID` | `Promise<TerminalWorkspaceView>` | ✅ 真实现 | store/terminal.ts |
| `TerminalOutputForTab` | `tabID, sessionID` | `Promise<string>` | ✅ 真实现 | App.tsx |
| `RemoteHosts` | `—` | `Promise<RemoteHostView[]>` | ✅ 真实现 | App.tsx, components/RemoteHostsPage.tsx |
| `UpdateRemoteHost` | `id, input` | `Promise<RemoteHostView>` | ⚠️ 假实现（空返回） | components/RemoteHostsPage.tsx |
| `RemoteConnectionStatuses` | `—` | `Promise<RemoteConnectionStatus[]>` | ✅ 真实现 | App.tsx, components/RemoteHostsPage.tsx |
| `RemoteForwards` | `hostId` | `Promise<RemoteForwardView[]>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx |
| `RemoteServerStatus` | `hostId` | `Promise<RemoteServerView>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx |
| `RemoteServerLogs` | `hostId, tailLines` | `Promise<string>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx |
| `RemoteLastWorkspace` | `hostId` | `Promise<string>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx, lib/workbenchTarget.ts |
| `WorkbenchActiveTarget` | `—` | `Promise<WorkbenchActiveTarget>` | ✅ 真实现 | App.tsx, lib/workbenchTarget.ts |
| `WorkbenchLastRemoteHint` | `—` | `Promise<WorkbenchRemoteHint>` | ⚠️ 假实现（空返回） | lib/workbenchTarget.ts |
| `WorkbenchSwitchLocal` | `—` | `Promise<WorkbenchActiveTarget>` | ⚠️ 假实现（空返回） | App.tsx, lib/workbenchTarget.ts |
| `WorkbenchConnectRemote` | `hostId, workspace` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/workbenchTarget.ts |
| `WorkbenchDisconnectRemote` | `—` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/workbenchTarget.ts |
| `WorkbenchRemoteRequest` | `method, paramsJSON` | `Promise<string>` | ⚠️ 假实现（空返回） | lib/workbenchTarget.ts |
| `WorkbenchResolveProviderTrust` | `accept` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ProviderTrustDialog.tsx, lib/workbenchTarget.ts |
| `WorkbenchPendingProviderTrust` | `—` | `Promise<ProviderTrustPrompt | null>` | ✅ 真实现 | components/ProviderTrustDialog.tsx, lib/workbenchTarget.ts |

## 其他（198，阶段 6 前逐项归类）

| 方法 | 参数 | 返回 | 状态 | 调用点 |
|---|---|---|---|---|
| `MinimiseMainWindow` | `—` | `Promise<void>` | ✅ 真实现 | App.tsx |
| `ToggleMaximiseMainWindow` | `—` | `Promise<void>` | ✅ 真实现 | App.tsx |
| `IsMainWindowMaximised` | `—` | `Promise<boolean>` | ✅ 真实现 | App.tsx |
| `CloseMainWindow` | `—` | `Promise<void>` | ✅ 真实现 | App.tsx |
| `HeartbeatListTasks` | `—` | `Promise<unknown>` | ✅ 真实现 | — |
| `HeartbeatReloadTasks` | `—` | `Promise<unknown>` | ✅ 真实现 | custom/features/heartbeat/heartbeat.bridge.ts |
| `HeartbeatSaveTasks` | `tasks` | `Promise<void>` | ✅ 真实现 | custom/features/heartbeat/heartbeat.bridge.ts |
| `HeartbeatTriggerNow` | `id` | `Promise<void>` | ✅ 真实现 | custom/features/heartbeat/heartbeat.bridge.ts |
| `HeartbeatGenerateID` | `—` | `Promise<string>` | ✅ 真实现 | custom/features/heartbeat/heartbeat.bridge.ts |
| `RunShell` | `command` | `Promise<void>` | ❌ 未绑定 | — |
| `RunShellForTab` | `tabID, command` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `Steer` | `text` | `Promise<void>` | ❌ 未绑定 | — |
| `SteerForTab` | `tabID, text` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `Approve` | `id, allow, session, persist` | `Promise<void>` | ❌ 未绑定 | — |
| `ApproveTab` | `tabID, id, allow, session, persist` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SetRecoveryCheckpointEnabled` | `enabled` | `Promise<void>` | ❌ 未绑定 | — |
| `SetRecoveryCheckpointEnabledTab` | `tabID, enabled` | `Promise<void>` | ❌ 未绑定 | — |
| `ReplayPendingPrompts` | `—` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SetPlanMode` | `on` | `Promise<void>` | ❌ 未绑定 | — |
| `SetMode` | `mode` | `Promise<void>` | ❌ 未绑定 | — |
| `SetModeForTab` | `tabID, mode` | `Promise<string[] | void>` | ✅ 真实现 | lib/useController.ts |
| `SetAutoApproveTools` | `on` | `Promise<void>` | ❌ 未绑定 | — |
| `SetCollaborationMode` | `mode` | `Promise<void>` | ❌ 未绑定 | — |
| `SetCollaborationModeForTab` | `tabID, mode` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SetToolApprovalMode` | `mode` | `Promise<void>` | ❌ 未绑定 | — |
| `SetToolApprovalModeForTab` | `tabID, mode` | `Promise<string[] | void>` | ✅ 真实现 | lib/useController.ts |
| `SetComposerProfileForTab` | `tabID, collaborationMode, toolApprovalMode, goal` | `Promise<string[] | void>` | ✅ 真实现 | lib/useController.ts |
| `SetGoal` | `goal` | `Promise<void>` | ❌ 未绑定 | — |
| `SetGoalForTab` | `tabID, goal` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `Compact` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `CompactForTab` | `tabID` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `DeleteRecoveryCopy` | `path` | `Promise<void>` | ✅ 真实现 | App.tsx |
| `ScanPromptHistory` | `nonce` | `Promise<PromptHistoryResult>` | ✅ 真实现 | lib/composerHistory.ts |
| `RemoveWorkspace` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ProjectTree.tsx |
| `ContextUsage` | `—` | `Promise<ContextInfo>` | ❌ 未绑定 | — |
| `ContextUsageForTab` | `tabID` | `Promise<ContextInfo>` | ✅ 真实现 | lib/useController.ts |
| `Balance` | `—` | `Promise<BalanceInfo>` | ❌ 未绑定 | — |
| `BalanceForTab` | `tabID` | `Promise<BalanceInfo>` | ✅ 真实现 | lib/useController.ts |
| `Jobs` | `—` | `Promise<JobView[]>` | ❌ 未绑定 | — |
| `JobsForTab` | `tabID` | `Promise<JobView[]>` | ✅ 真实现 | lib/useController.ts |
| `ToolResultForTab` | `tabID, toolID` | `Promise<{ args: string` | ✅ 真实现 | components/ToolCard.tsx |
| `AutoResearchCurrent` | `—` | `Promise<AutoResearchStatusView>` | ❌ 未绑定 | — |
| `AutoResearchStatus` | `tabID` | `Promise<AutoResearchStatusView>` | ❌ 未绑定 | — |
| `AutoResearchList` | `tabID` | `Promise<AutoResearchStatusView[]>` | ❌ 未绑定 | — |
| `AutoResearchFindings` | `tabID, limit` | `Promise<AutoResearchFindingView[]>` | ❌ 未绑定 | — |
| `AutoResearchOpenTask` | `tabID` | `Promise<void>` | ❌ 未绑定 | — |
| `AutoResearchRecordEvidence` | `tabID, criterionID, input` | `Promise<void>` | ❌ 未绑定 | — |
| `CapabilityDiagnostics` | `includeSessionRuntime` | `Promise<CapabilityDiagnosticsReport>` | ⚠️ 假实现（空返回） | components/DiagnosticsSettingsPage.tsx |
| `InstallPlugin` | `source, options` | `Promise<string>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `RemovePlugin` | `name` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `SetPluginEnabled` | `name, enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `AddMCPServer` | `input` | `Promise<number>` | ❌ 未绑定 | — |
| `InstallMCPServer` | `input` | `Promise<MCPInstallResult>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `RemoveMCPServer` | `name` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `AuthorizeAndConnectMCPServer` | `name` | `Promise<void>` | ❌ 未绑定 | — |
| `ReconnectMCPServer` | `name` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `PickSkillFolder` | `—` | `Promise<string>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `PickPluginFolder` | `—` | `Promise<string>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `AddSkillPath` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `RemoveSkillPath` | `path` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `RefreshSkills` | `—` | `Promise<void>` | ✅ 真实现 | components/CapabilitiesPanel.tsx, components/SubagentsPanel.tsx |
| `ReloadCommands` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `SetSkillEnabled` | `name, enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `AvailableSubagentTools` | `—` | `Promise<MCPToolView[]>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `CreateSubagentProfile` | `input` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `DeleteSubagentProfile` | `name, scope` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `SetSubagentProfileModel` | `name, ref` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `SetSubagentProfileEffort` | `name, level` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `TrySubagentProfile` | `input, task` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SubagentsPanel.tsx |
| `SetMCPServerEnabled` | `name, enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/CapabilitiesPanel.tsx |
| `SetMCPServerTier` | `name, tier` | `Promise<void>` | ❌ 未绑定 | — |
| `SearchFileRefs` | `query` | `Promise<DirEntry[]>` | ❌ 未绑定 | — |
| `SearchFileRefsForTab` | `tabID, query` | `Promise<DirEntry[]>` | ✅ 真实现 | components/WorkspacePanel.tsx |
| `ReadFile` | `rel` | `Promise<FilePreview>` | ❌ 未绑定 | — |
| `ReadFileForTab` | `tabID, rel` | `Promise<FilePreview>` | ✅ 真实现 | components/WorkspacePanel.tsx |
| `GitBranches` | `—` | `Promise<string[]>` | ❌ 未绑定 | — |
| `GitCheckout` | `branch` | `Promise<void>` | ❌ 未绑定 | — |
| `OpenWorkspacePath` | `rel` | `Promise<void>` | ❌ 未绑定 | — |
| `OpenWorkspacePathForTab` | `tabID, rel` | `Promise<void>` | ✅ 真实现 | — |
| `ExternalOpeners` | `—` | `Promise<ExternalOpenersView>` | ✅ 真实现 | — |
| `SetPreferredExternalOpener` | `id` | `Promise<void>` | ❌ 未绑定 | — |
| `OpenWorkspaceInExternalOpener` | `id` | `Promise<void>` | ❌ 未绑定 | — |
| `OpenWorkspaceInExternalOpenerForTab` | `tabID, id` | `Promise<void>` | ❌ 未绑定 | — |
| `PickExportFile` | `defaultFilename, mimeType` | `Promise<string>` | ⚠️ 假实现（空返回） | App.tsx |
| `AttachDropped` | `path` | `Promise<DroppedItem>` | ⚠️ 假实现（空返回） | components/Composer.tsx |
| `SetModel` | `name` | `Promise<void>` | ❌ 未绑定 | — |
| `SetModelForTab` | `tabID, name` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SetEffort` | `level` | `Promise<void>` | ❌ 未绑定 | — |
| `SetEffortForTab` | `tabID, level` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `SetTokenMode` | `mode` | `Promise<void>` | ❌ 未绑定 | — |
| `SetTokenModeForTab` | `tabID, mode` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `AcceptMemorySuggestion` | `suggestion` | `Promise<string>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `AcceptSkillSuggestion` | `suggestion` | `Promise<string>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `AcceptMemorySuggestionForTab` | `tabID, suggestion` | `Promise<string>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `AcceptSkillSuggestionForTab` | `tabID, suggestion` | `Promise<string>` | ⚠️ 假实现（空返回） | components/MemoryPanel.tsx |
| `Remember` | `scope, note` | `Promise<string>` | ✅ 真实现 | components/MemoryPanel.tsx, lib/useController.ts |
| `RememberForTab` | `tabID, scope, note` | `Promise<string>` | ✅ 真实现 | components/MemoryPanel.tsx |
| `Forget` | `name` | `Promise<void>` | ✅ 真实现 | components/MemoryPanel.tsx, lib/useController.ts |
| `ForgetForTab` | `tabID, name` | `Promise<void>` | ✅ 真实现 | components/MemoryPanel.tsx |
| `Settings` | `—` | `Promise<SettingsView>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `TrustProjectHooks` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `TrustProjectHooksForRoot` | `projectRoot` | `Promise<void>` | ❌ 未绑定 | — |
| `SetDefaultModel` | `ref` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetPlannerModel` | `ref` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetSubagentModel` | `ref` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetSubagentEffort` | `level` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetMaxSubagentDepth` | `depth` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetMaxSubagentConcurrency` | `n` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetMaxParallelWriters` | `n` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetAutoPlan` | `mode` | `Promise<void>` | ❌ 未绑定 | — |
| `SetDefaultToolApprovalMode` | `mode` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDefaultAutoRecoveryCheckpoint` | `enabled` | `Promise<void>` | ❌ 未绑定 | — |
| `AddOfficialProviderAccess` | `kind, key` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `AddProviderPresetAccess` | `id, key` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `ResetProviderPresetAccess` | `id` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `FetchProviderModels` | `p` | `Promise<string[]>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `DeleteProvider` | `name` | `Promise<void>` | ❌ 未绑定 | — |
| `RemoveProviderAccess` | `name` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetProviderKey` | `apiKeyEnv, value` | `Promise<string>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `AddPermissionRule` | `list, rule` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `RemovePermissionRule` | `list, rule` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `ReloadSettings` | `—` | `Promise<void>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `SetSandbox` | `bash, network, workspaceRoot, allowWrite, shell` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetNetwork` | `n` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetBotSettings` | `b` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetBotConnectionToolApprovalMode` | `connID, mode` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetBotSecret` | `envName, value` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `StartBotConnectionInstall` | `provider, domain` | `Promise<BotInstallStartResult>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `PollBotConnectionInstall` | `installID` | `Promise<BotInstallPollResult>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `SetDisplayMode` | `mode` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetStatusBarStyle` | `style` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetStatusBarItems` | `items` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopLanguage` | `lang` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopCurrency` | `currency` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopAppearance` | `theme, style` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx, components/SettingsPanel.tsx, lib/themeExperience.ts |
| `GetThemeExperience` | `—` | `Promise<import("./themeExperience").ThemeExperienceView>` | ✅ 真实现 | — |
| `ActivateThemePack` | `id` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ThemeLibrary.tsx, lib/themeExperience.ts |
| `ActivateBaseStyle` | `style` | `Promise<void>` | ❌ 未绑定 | — |
| `DisableThemePack` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `ResetThemePack` | `—` | `Promise<void>` | ✅ 真实现 | App.tsx, components/ThemeLibrary.tsx, lib/themeExperience.ts |
| `DeleteThemePack` | `id` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ThemeGallery.tsx, components/ThemeLibrary.tsx |
| `PickThemeBackground` | `—` | `Promise<string>` | ⚠️ 假实现（空返回） | components/ThemeGallery.tsx, components/ThemeLibrary.tsx |
| `SetDesktopLayoutStyle` | `style` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopZoomFactor` | `factor` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `GetDesktopZoomFactor` | `—` | `Promise<number>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `RestartApplication` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `SetDesktopCheckUpdates` | `enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopUpdateChannel` | `channel` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopTelemetry` | `enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetDesktopMetrics` | `enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetExpandThinking` | `on` | `Promise<void>` | ❌ 未绑定 | — |
| `SetDesktopConversationWidth` | `width` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetAgentParams` | `temperature, maxSteps, plannerMaxSteps, systemPrompt` | `Promise<void>` | ❌ 未绑定 | — |
| `SetColdResumePrune` | `enabled` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetReasoningLanguage` | `lang` | `Promise<void>` | ⚠️ 假实现（空返回） | components/SettingsPanel.tsx |
| `SetTrayLocale` | `locale` | `Promise<void>` | ✅ 真实现 | App.tsx |
| `SetBypass` | `on` | `Promise<void>` | ❌ 未绑定 | — |
| `CheckUpdate` | `channel` | `Promise<UpdateInfo | null>` | ✅ 真实现 | lib/useUpdater.ts |
| `InstallUpdate` | `channel` | `Promise<void>` | ❌ 未绑定 | — |
| `InstallUpdateRequest` | `channel, expectedVersion, requestId` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useUpdater.ts |
| `ApplyUpdate` | `—` | `Promise<void>` | ❌ 未绑定 | — |
| `OpenDownloadPage` | `—` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useUpdater.ts |
| `NeedsOnboarding` | `—` | `Promise<boolean>` | ✅ 真实现 | App.tsx |
| `ReportCrash` | `kind, detail` | `Promise<void>` | ✅ 真实现 | components/SettingsPanel.tsx |
| `OpenProjectTab` | `workspaceRoot, topicID` | `Promise<TabMeta>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `CreateDeliveryWorktree` | `workspaceRoot` | `Promise<DeliveryWorktreeOpenResult>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `OpenGlobalTab` | `topicID` | `Promise<TabMeta>` | ✅ 真实现 | lib/useController.ts |
| `OpenTopicSession` | `scope, workspaceRoot, topicID, sessionPath` | `Promise<TabMeta>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `ActivateTopic` | `scope, workspaceRoot, topicID, sessionPath` | `Promise<TabMeta>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `SetActiveTab` | `tabID` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `ReorderTabs` | `tabIDs` | `Promise<void>` | ✅ 真实现 | lib/useController.ts |
| `CloseTab` | `tabID` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/useController.ts |
| `CreateTerminalForTab` | `tabID, relativePath, shellID` | `Promise<TerminalSessionView>` | ⚠️ 假实现（空返回） | store/terminal.ts |
| `WriteTerminalForTab` | `tabID, sessionID, data` | `Promise<void>` | ✅ 真实现 | store/terminal.ts |
| `ResizeTerminalForTab` | `tabID, sessionID, cols, rows` | `Promise<void>` | ✅ 真实现 | store/terminal.ts |
| `CloseTerminalForTab` | `tabID, sessionID` | `Promise<void>` | ✅ 真实现 | store/terminal.ts |
| `SetProjectColor` | `workspaceRoot, color` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ProjectTree.tsx |
| `SetProjectPinned` | `workspaceRoot, pinned` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ProjectTree.tsx |
| `ReorderProjects` | `workspaceRoots` | `Promise<void>` | ✅ 真实现 | components/ProjectTree.tsx |
| `CreateTopic` | `scope, workspaceRoot, title` | `Promise<TopicMeta>` | ✅ 真实现 | components/ProjectTree.tsx |
| `DeleteTopic` | `topicID` | `Promise<void>` | ❌ 未绑定 | — |
| `SetTopicPinned` | `topicID, pinned` | `Promise<void>` | ⚠️ 假实现（空返回） | components/ProjectTree.tsx |
| `ContextPanel` | `tabID` | `Promise<ContextPanelInfo>` | ✅ 真实现 | components/ContextPanel.tsx, components/ContextWindowRing.tsx |
| `ConfirmAction` | `req` | `Promise<boolean>` | ❌ 未绑定 | — |
| `SaveWindowState` | `state` | `Promise<void>` | ⚠️ 假实现（空返回） | lib/windowState.ts |
| `AddRemoteHost` | `input` | `Promise<RemoteHostView>` | ⚠️ 假实现（空返回） | components/RemoteHostsPage.tsx |
| `RemoveRemoteHost` | `id` | `Promise<void>` | ⚠️ 假实现（空返回） | components/RemoteHostsPage.tsx |
| `DisconnectRemoteHost` | `id` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx, components/RemotePanel.tsx, components/RemoteHostsPage.tsx |
| `ConfirmRemoteHostKey` | `hostId, accept` | `Promise<void>` | ⚠️ 假实现（空返回） | components/RemoteHostKeyDialog.tsx |
| `ConfirmRemoteSecret` | `hostId, promptId, secret, accept` | `Promise<void>` | ⚠️ 假实现（空返回） | components/RemoteSecretDialog.tsx |
| `ReadRemoteFile` | `hostId, path` | `Promise<RemoteFilePreview>` | ✅ 真实现 | components/RemotePanel.tsx |
| `WriteRemoteFile` | `hostId, path, body, expectMtimeUnix` | `Promise<RemoteWriteResult>` | ✅ 真实现 | components/RemotePanel.tsx |
| `MkdirRemote` | `hostId, path` | `Promise<void>` | ❌ 未绑定 | — |
| `DeleteRemotePath` | `hostId, path, recursive` | `Promise<void>` | ❌ 未绑定 | — |
| `AddRemoteForward` | `hostId, input` | `Promise<RemoteForwardView>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx |
| `RemoveRemoteForward` | `hostId, forwardId` | `Promise<void>` | ⚠️ 假实现（空返回） | components/RemotePanel.tsx |
| `OpenRemoteWorkspace` | `hostId, workspace` | `Promise<void>` | ⚠️ 假实现（空返回） | App.tsx, components/RemotePanel.tsx |
| `StopRemoteServer` | `hostId` | `Promise<void>` | ✅ 真实现 | components/RemotePanel.tsx |

## 持久化归属

| 数据 | 归属 | 说明 |
|---|---|---|
| 会话正文 | Reasonix JSONL（`<BTask 数据>/tasks/<taskId>/reasonix-sessions/`） | 唯一事实来源 |
| 任务→活动会话映射 | SQLite（BTask） | 仅元数据，正文不入库 |
| 配置 | BTask 数据目录隔离 `REASONIX_HOME`（阶段 4） | 提供"导入/共享 `~/.reasonix`"选项 |
| 凭据（API Key） | Reasonix .env 凭据帮助函数（系统钥匙串/文件） | 前端只接收"已配置/未配置"，不返回明文 |
| 工作区 | 任务 Git Binding 的 WorktreePath（阶段 2） | 未绑定工作树时禁止可写启动 |
| 最后会话 | `last-session.txt`（任务目录内相对会话 ID，原子写入，阶段 2） | 绝对路径 + 符号链接检查 |

## 验收用例

### 自动化（临时 REASONIX_HOME、假 Provider、禁止网络）

1. A 发消息 → 切 B → 回 A：消息/运行状态/审批仍在，且无 B 的事件泄漏（阶段 3）。
2. A、B 后台并行运行，各自写入自己的工作树与会话（阶段 3）。
3. 快速 A→B→A 循环 100 次：无错任务/错目录/持续内存增长（阶段 1/3，`go test -race`）。
4. 退出重启后每个任务恢复到原会话（阶段 3）。
5. 工作树重新绑定后不再操作旧路径（阶段 2）。
6. 模型/设置更新失败时旧控制器继续可用（阶段 1 build-then-swap）。
7. API Key 不出现在前端状态、SQLite、日志、诊断输出（阶段 4）。
8. 未支持功能不显示或明确禁用，可见按钮均有真实副作用（阶段 6）。

### 人工（真实 UI）

1. Shadow 内弹窗/菜单/输入法/拖拽/快捷键正常且不污染 BTask（阶段 5）。
2. 中文输入法、复制粘贴、文件拖放、Mermaid、图片、Diff、滚动、缩放（阶段 5）。
3. 设置面板"Reasonix 设置"为唯一入口，Provider 增删改/凭据状态/默认模型/effort/审批 可配置（阶段 4）。

## 签名不匹配（契约参数数 ≠ 宿主绑定参数数，92 个）

> 假兼容的直接证据：前端按契约参数调用，宿主绑定参数数不同——wails 解析要么
> 报错、要么吞掉参数（如 `SaveProviderWithKey(provider, key)` 宿主只收 1 参，
> 凭据实际未保存）。阶段 1 重建宿主契约时必须逐项对齐或显式禁用。

| 方法 | 契约参数 | 宿主参数 | 宿主实现 |
|---|---|---|---|
| `SubmitDisplayToTab` | 3 | 1 | ⚠️ stub |
| `SubmitDeliveryRecoveryToTab` | 3 | 1 | ⚠️ stub |
| `SubmitInvocationsToTab` | 4 | 1 | ⚠️ stub |
| `SubmitEditedDisplayToTab` | 4 | 1 | ⚠️ stub |
| `RunShellForTab` | 2 | 1 | ⚠️ stub |
| `SetComposerProfileForTab` | 4 | 5 | real |
| `OpenChannelSessionPageForTab` | 3 | 1 | ⚠️ stub |
| `PickWorkspace` | 0 | 1 | ⚠️ stub |
| `MCPServers` | 0 | 1 | ⚠️ stub |
| `Plugins` | 0 | 1 | ⚠️ stub |
| `PlanPluginInstall` | 2 | 1 | ⚠️ stub |
| `InstallPlugin` | 2 | 1 | ⚠️ stub |
| `SetPluginEnabled` | 2 | 1 | ⚠️ stub |
| `UpdateMCPServer` | 2 | 1 | ⚠️ stub |
| `PickSkillFolder` | 0 | 1 | ⚠️ stub |
| `PickPluginFolder` | 0 | 1 | ⚠️ stub |
| `SetSkillEnabled` | 2 | 1 | ⚠️ stub |
| `AvailableSubagentTools` | 0 | 1 | ⚠️ stub |
| `UpdateSubagentProfile` | 3 | 1 | ⚠️ stub |
| `DeleteSubagentProfile` | 2 | 1 | ⚠️ stub |
| `SetSubagentProfileModel` | 2 | 1 | ⚠️ stub |
| `SetSubagentProfileEffort` | 2 | 1 | ⚠️ stub |
| `TrySubagentProfile` | 2 | 1 | ⚠️ stub |
| `CancelTrySubagentProfile` | 0 | 1 | ⚠️ stub |
| `SetMCPServerEnabled` | 2 | 1 | ⚠️ stub |
| `WorkspaceChangeDetail` | 2 | 1 | ⚠️ stub |
| `WorkspaceGitHistory` | 2 | 1 | ⚠️ stub |
| `RevealWorkspacePathForTab` | 2 | 1 | ⚠️ stub |
| `SaveClipboardImage` | 0 | 1 | ⚠️ stub |
| `SavePastedFile` | 2 | 1 | ⚠️ stub |
| `PickExportFile` | 2 | 1 | ⚠️ stub |
| `SaveExportFile` | 3 | 1 | ⚠️ stub |
| `SaveExportImageFiles` | 2 | 1 | ⚠️ stub |
| `Memory` | 0 | 1 | ⚠️ stub |
| `MemorySuggestions` | 0 | 1 | ⚠️ stub |
| `MemoryRevisionsForTab` | 2 | 1 | ⚠️ stub |
| `RestoreMemoryRevision` | 2 | 1 | ⚠️ stub |
| `RestoreMemoryRevisionForTab` | 3 | 1 | ⚠️ stub |
| `AcceptMemorySuggestionForTab` | 2 | 1 | ⚠️ stub |
| `AcceptSkillSuggestionForTab` | 2 | 1 | ⚠️ stub |
| `RestoreArchivedMemoryForTab` | 2 | 1 | ⚠️ stub |
| `SaveDoc` | 2 | 1 | ⚠️ stub |
| `SaveDocForTab` | 3 | 1 | ⚠️ stub |
| `SaveHooksSettingsForRoot` | 3 | 1 | ⚠️ stub |
| `SaveProviderWithKey` | 2 | 1 | ⚠️ stub |
| `AddOfficialProviderAccess` | 2 | 1 | ⚠️ stub |
| `AddProviderPresetAccess` | 2 | 1 | ⚠️ stub |
| `SaveProviderKey` | 2 | 1 | ⚠️ stub |
| `SetProviderKey` | 2 | 1 | ⚠️ stub |
| `AddPermissionRule` | 2 | 1 | ⚠️ stub |
| `RemovePermissionRule` | 2 | 1 | ⚠️ stub |
| `SetSandbox` | 5 | 1 | ⚠️ stub |
| `SetBotConnectionToolApprovalMode` | 2 | 1 | ⚠️ stub |
| `SetBotSecret` | 2 | 1 | ⚠️ stub |
| `BotRuntimeStatus` | 0 | 1 | ⚠️ stub |
| `TestBotConnection` | 2 | 1 | ⚠️ stub |
| `SetDesktopAppearance` | 2 | 1 | ⚠️ stub |
| `ListThemePacks` | 0 | 1 | ⚠️ stub |
| `CopyThemePack` | 3 | 1 | ⚠️ stub |
| `ImportThemePack` | 2 | 1 | ⚠️ stub |
| `ExportThemePack` | 2 | 1 | ⚠️ stub |
| `PickThemeBackground` | 0 | 1 | ⚠️ stub |
| `MigrateDesktopPreferences` | 3 | 1 | ⚠️ stub |
| `DownloadUpdateRequest` | 3 | 1 | ⚠️ stub |
| `InstallUpdateRequest` | 3 | 1 | ⚠️ stub |
| `OpenDownloadPage` | 0 | 1 | ⚠️ stub |
| `ReportCrash` | 2 | 1 | real |
| `OpenProjectTab` | 2 | 1 | ⚠️ stub |
| `OpenTopicSession` | 4 | 1 | ⚠️ stub |
| `ActivateTopic` | 4 | 1 | ⚠️ stub |
| `EnsureBlankSurface` | 2 | 1 | real |
| `TerminalWorkspaceForTab` | 1 | 2 | real |
| `TerminalOutputForTab` | 2 | 3 | real |
| `CreateTerminalForTab` | 3 | 1 | ⚠️ stub |
| `SetProjectColor` | 2 | 1 | ⚠️ stub |
| `SetProjectPinned` | 2 | 1 | ⚠️ stub |
| `SetTopicPinned` | 2 | 1 | ⚠️ stub |
| `UpdateRemoteHost` | 2 | 1 | ⚠️ stub |
| `ScanSSHConfig` | 0 | 1 | ⚠️ stub |
| `ConfirmRemoteHostKey` | 2 | 1 | ⚠️ stub |
| `ConfirmRemoteSecret` | 4 | 1 | ⚠️ stub |
| `ListRemoteDir` | 2 | 1 | ⚠️ stub |
| `WriteRemoteFile` | 4 | 3 | real |
| `AddRemoteForward` | 2 | 1 | ⚠️ stub |
| `RemoveRemoteForward` | 2 | 1 | ⚠️ stub |
| `OpenRemoteWorkspace` | 2 | 1 | ⚠️ stub |
| `RemoteServerLogs` | 2 | 1 | ⚠️ stub |
| `WorkbenchLastRemoteHint` | 0 | 1 | ⚠️ stub |
| `WorkbenchSwitchLocal` | 0 | 1 | ⚠️ stub |
| `WorkbenchConnectRemote` | 2 | 1 | ⚠️ stub |
| `WorkbenchDisconnectRemote` | 0 | 1 | ⚠️ stub |
| `WorkbenchRemoteRequest` | 2 | 1 | ⚠️ stub |

## 阶段 6 清零结果（核心面）

核心工作台方法最终状态：**90 真实现 / 42 显式错误 / 0 空返回**。

本轮新增真实现：
- 附件：SavePastedImage / SavePastedFile / AttachmentDataURL（任务会话目录
  attachments/，路径归属 + 符号链接校验）；SaveClipboardImage 显式错误。
- Provider：SaveProvider / SaveProviderWithKey / SaveProviderKey /
  ClearProviderKey（映射内核 .env，凭据不落库）；SetPermissionMode 映射
  审批模式。
- 工作台无参版：Submit / Cancel / AnswerQuestion / ClearGoal / History /
  HistoryPage / Checkpoints / Rewind / Fork / SummarizeFrom / SummarizeUpTo
  （全部映射激活任务）。
- 文档：SaveDoc / SaveDocForTab（任务会话 docs/）；导出：
  SaveExportFile / SaveExportImageFiles（文件对话框）。
- RestoreSession（回收站恢复）。

显式错误（诚实禁用，不返回假成功）：恢复管理（ResolveRecovery /
RecoveryCheckpointEnabled）、显示提交（SubmitDisplay*）、频道会话、
MCP/插件、远程 SSH/机器人、主题包、Hook、迁移、切换工作区（嵌入单任务
工作树）——对应 UI 已在 host 模式隐藏（SettingsPanel tabs）或显示错误。

## 生成方式

本矩阵由脚本从 `bridge.ts`（契约）、`rx_bindings.go`/（`rx_bindings_stubs.go` 宿主实现）与
前端调用点自动提取；状态标注为快照（阶段 1 重建契约后需重新生成并人工复核）。
