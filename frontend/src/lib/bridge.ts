import type { StateStorage } from "zustand/middleware";
import type {
  CandidateAnalysis,
  PlaneCandidatePayload,
  PlaneConnectionSetup,
  PlaneConnectionStatus,
  PlaneProject,
  PlaneSettings,
} from "../domain/collection";
import type { TaskStatus } from "../domain/task";
import type { PISettings } from "../domain/engine";
import type {
  DailyReportGenerationInput,
  DailyReportGenerationProgress,
  DailyReportGenerationResult,
  DailyReportSettings,
} from "../domain/report";
import type {
  RequirementAnalysisInput,
  RequirementAnalysisResult,
} from "../domain/requirements";
import type { TransitionGates } from "../domain/workflow";
import { validateTransition as validateInBrowser } from "../domain/workflow";

export interface EngineStatus {
  id: "pi" | "codex";
  label: string;
  configured: boolean;
  requirementAnalysis: boolean;
  development: boolean;
  description: string;
  commandPath: string;
  version: string;
}

export interface DailyReportSubmissionResult {
  id: number | string;
  action: "inserted" | "updated";
  message?: string;
}

export interface TaskContextRootInfo {
  path: string;
  defaultPath: string;
  custom: boolean;
  available: boolean;
  databaseSchemaVersion?: number;
  taskWorkspaceSchemaVersion?: number;
  workspaceCount?: number;
  workspaceErrorCount?: number;
}

export interface TaskWorkspaceInfo {
  taskId: string;
  workspaceId: string;
  rootPath: string;
  schemaVersion: number;
  manifestRevision: number;
  state: "ready" | "legacy" | "error" | "archived";
  legacyContextPath?: string;
  createdAt: string;
  updatedAt: string;
  lastReconciledAt?: string;
  errorMessage?: string;
}

export interface WorkspaceEntry {
  name: string;
  path: string;
  type: "file" | "directory" | "symlink" | "special";
  byteSize: number;
  modifiedAt: string;
  readable: boolean;
}

export interface FilePreview {
  path: string;
  name: string;
  mimeType: string;
  byteSize: number;
  sha256: string;
  kind: "text" | "image";
  content: string;
  truncated: boolean;
}

interface NativeApp {
  LoadState(): Promise<string>;
  SaveState(payload: string): Promise<void>;
  ClearState(): Promise<void>;
  ValidateTransition(
    from: TaskStatus,
    to: TaskStatus,
    requirementsConfirmed: boolean,
    developmentCompleted: boolean,
    reviewApproved: boolean,
  ): Promise<string>;
  EngineStatuses(): Promise<EngineStatus[]>;
  SetupPlaneConnection(
    serviceAddress: string,
    token: string,
  ): Promise<PlaneConnectionSetup>;
  SavePlaneToken(
    baseUrl: string,
    workspaceSlug: string,
    token: string,
  ): Promise<void>;
  DeletePlaneToken(baseUrl: string, workspaceSlug: string): Promise<void>;
  HasPlaneToken(
    baseUrl: string,
    workspaceSlug: string,
  ): Promise<boolean>;
  SaveDailyReportToken(
    apiUrl: string,
    employeeId: string,
    token: string,
  ): Promise<void>;
  DeleteDailyReportToken(apiUrl: string, employeeId: string): Promise<void>;
  HasDailyReportToken(apiUrl: string, employeeId: string): Promise<boolean>;
  SubmitDailyReport(
    apiUrl: string,
    employeeId: string,
    reportDate: string,
    content: string,
  ): Promise<DailyReportSubmissionResult>;
  GenerateDailyReport(
    input: DailyReportGenerationInput,
    runtime: PISettings,
  ): Promise<DailyReportGenerationResult>;
  SelectDailyReportProjectDirectory(): Promise<string>;
  GetTaskContextRoot(): Promise<TaskContextRootInfo>;
  SelectTaskContextRoot(): Promise<string>;
  SetTaskContextRoot(path: string): Promise<TaskContextRootInfo>;
  OpenTaskContextRoot(): Promise<void>;
  EnsureTaskWorkspace?(taskId: string): Promise<TaskWorkspaceInfo>;
  ListTaskWorkspaceFiles?(
    taskId: string,
    path: string,
  ): Promise<WorkspaceEntry[]>;
  ReadTaskWorkspaceFile?(
    taskId: string,
    path: string,
  ): Promise<FilePreview>;
  ListPlaneProjects(
    baseUrl: string,
    workspaceSlug: string,
  ): Promise<PlaneProject[]>;
  TestPlaneConnection(
    baseUrl: string,
    workspaceSlug: string,
    projectId: string,
  ): Promise<PlaneConnectionStatus>;
  CollectPlaneWorkItems(
    baseUrl: string,
    workspaceSlug: string,
    projectId: string,
    projectIdentifier: string,
  ): Promise<PlaneCandidatePayload[]>;
  LoadPlaneWorkItemDetails(
    baseUrl: string,
    workspaceSlug: string,
    projectId: string,
    projectIdentifier: string,
    workItemId: string,
  ): Promise<PlaneCandidatePayload>;
  AnalyzePlaneCandidate(
    sourceMarkdown: string,
    settings: PISettings,
  ): Promise<CandidateAnalysis>;
  AnalyzeRequirements(
    input: RequirementAnalysisInput,
    settings: PISettings,
  ): Promise<RequirementAnalysisResult>;
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: NativeApp;
      };
    };
    runtime?: {
      BrowserOpenURL?(url: string): void;
      EventsOn?(
        eventName: string,
        callback: (payload: unknown) => void,
      ): (() => void) | void;
    };
  }
}

const nativeApp = (): NativeApp | undefined => window.go?.main?.App;

let nativeStateWriteQueue: Promise<void> = Promise.resolve();

function enqueueNativeStateWrite(operation: () => Promise<void>): Promise<void> {
  const pending = nativeStateWriteQueue.then(operation);
  nativeStateWriteQueue = pending.catch(() => undefined);
  return pending;
}

export const workspaceStorage: StateStorage = {
  async getItem(name) {
    const app = nativeApp();
    if (app) {
      const content = await app.LoadState();
      return content || null;
    }
    return window.localStorage.getItem(name);
  },
  async setItem(name, value) {
    const app = nativeApp();
    if (app) {
      await enqueueNativeStateWrite(() => app.SaveState(value));
      return;
    }
    window.localStorage.setItem(name, value);
  },
  async removeItem(name) {
    const app = nativeApp();
    if (app) {
      await enqueueNativeStateWrite(() => app.ClearState());
      return;
    }
    window.localStorage.removeItem(name);
  },
};

export async function validateTransition(
  from: TaskStatus,
  to: TaskStatus,
  gates: TransitionGates,
): Promise<void> {
  const app = nativeApp();
  const reason = app
    ? await app.ValidateTransition(
        from,
        to,
        gates.requirementsConfirmed,
        gates.developmentCompleted,
        gates.reviewApproved,
      )
    : validateInBrowser(from, to, gates);

  if (reason) throw new Error(reason);
}

export async function getEngineStatuses(): Promise<EngineStatus[]> {
  const app = nativeApp();
  if (app) return app.EngineStatuses();
  return [
    {
      id: "pi",
      label: "PI",
      configured: false,
      requirementAnalysis: false,
      development: false,
      description: "浏览器预览模式：原生 PI 运行时不可用。",
      commandPath: "",
      version: "",
    },
    {
      id: "codex",
      label: "Codex",
      configured: false,
      requirementAnalysis: false,
      development: false,
      description: "当前可复制已确认提示词，自动执行接口待配置。",
      commandPath: "",
      version: "",
    },
  ];
}

function requireNativeApp(): NativeApp {
  const app = nativeApp();
  if (!app) {
    throw new Error("AI 与 Plane 集成只能在 Wails 桌面客户端中使用");
  }
  return app;
}

function requireDailyReportNativeApp(): NativeApp {
  const app = nativeApp();
  if (!app) {
    throw new Error("日报云端功能只能在 Wails 桌面客户端中使用");
  }
  return app;
}

export function dailyReportCloudAvailable(): boolean {
  return Boolean(nativeApp());
}

export function dailyReportAIAvailable(): boolean {
  return Boolean(nativeApp());
}

export function dailyReportDirectoryPickerAvailable(): boolean {
  return typeof nativeApp()?.SelectDailyReportProjectDirectory === "function";
}

const DAILY_REPORT_GENERATION_PROGRESS_EVENT =
  "daily-report:generation-progress";

const DAILY_REPORT_GENERATION_PROGRESS_STAGES = new Set([
  "validating",
  "collecting_git",
  "building_prompt",
  "waiting_ai",
  "parsing_result",
  "completed",
  "failed",
]);

function isDailyReportGenerationProgress(
  payload: unknown,
): payload is DailyReportGenerationProgress {
  if (!payload || typeof payload !== "object") return false;
  const candidate = payload as Record<string, unknown>;
  return (
    typeof candidate.requestId === "string" &&
    candidate.requestId.length > 0 &&
    typeof candidate.stage === "string" &&
    DAILY_REPORT_GENERATION_PROGRESS_STAGES.has(candidate.stage) &&
    typeof candidate.message === "string"
  );
}

export function subscribeDailyReportGenerationProgress(
  listener: (progress: DailyReportGenerationProgress) => void,
): () => void {
  const runtime = window.runtime;
  if (typeof runtime?.EventsOn !== "function") return () => undefined;
  const unsubscribe = runtime.EventsOn(
    DAILY_REPORT_GENERATION_PROGRESS_EVENT,
    (payload) => {
      if (isDailyReportGenerationProgress(payload)) listener(payload);
    },
  );
  return typeof unsubscribe === "function" ? unsubscribe : () => undefined;
}

export async function selectDailyReportProjectDirectory(): Promise<string> {
  const app = nativeApp();
  if (typeof app?.SelectDailyReportProjectDirectory !== "function") {
    throw new Error("选择本地目录只能在 Wails 桌面客户端中使用");
  }
  const path = await app.SelectDailyReportProjectDirectory();
  return typeof path === "string" ? path : "";
}

export function taskContextDirectoryAvailable(): boolean {
  const app = nativeApp();
  return Boolean(
    typeof app?.GetTaskContextRoot === "function" &&
      typeof app.SelectTaskContextRoot === "function" &&
      typeof app.SetTaskContextRoot === "function" &&
      typeof app.OpenTaskContextRoot === "function",
  );
}

function requireTaskContextNativeApp(): NativeApp {
  const app = nativeApp();
  if (!app || !taskContextDirectoryAvailable()) {
    throw new Error("任务资料目录只能在 Wails 桌面客户端中使用");
  }
  return app;
}

export async function getTaskContextRoot(): Promise<TaskContextRootInfo> {
  return requireTaskContextNativeApp().GetTaskContextRoot();
}

export async function selectTaskContextRoot(): Promise<string> {
  const path = await requireTaskContextNativeApp().SelectTaskContextRoot();
  return typeof path === "string" ? path : "";
}

export async function setTaskContextRoot(
  path: string,
): Promise<TaskContextRootInfo> {
  return requireTaskContextNativeApp().SetTaskContextRoot(path.trim());
}

export async function openTaskContextRoot(): Promise<void> {
  await requireTaskContextNativeApp().OpenTaskContextRoot();
}

function requireTaskWorkspaceNativeApp(): Required<
  Pick<
    NativeApp,
    | "EnsureTaskWorkspace"
    | "ListTaskWorkspaceFiles"
    | "ReadTaskWorkspaceFile"
  >
> {
  const app = nativeApp();
  if (
    typeof app?.EnsureTaskWorkspace !== "function" ||
    typeof app.ListTaskWorkspaceFiles !== "function" ||
    typeof app.ReadTaskWorkspaceFile !== "function"
  ) {
    throw new Error("任务工作区文件只能在 Wails 桌面客户端中使用");
  }
  return app as Required<
    Pick<
      NativeApp,
      | "EnsureTaskWorkspace"
      | "ListTaskWorkspaceFiles"
      | "ReadTaskWorkspaceFile"
    >
  >;
}

export async function ensureTaskWorkspace(
  taskId: string,
): Promise<TaskWorkspaceInfo> {
  return requireTaskWorkspaceNativeApp().EnsureTaskWorkspace(taskId);
}

export async function listTaskWorkspaceFiles(
  taskId: string,
  path = "",
): Promise<WorkspaceEntry[]> {
  return requireTaskWorkspaceNativeApp().ListTaskWorkspaceFiles(taskId, path);
}

export async function readTaskWorkspaceFile(
  taskId: string,
  path: string,
): Promise<FilePreview> {
  return requireTaskWorkspaceNativeApp().ReadTaskWorkspaceFile(taskId, path);
}

export async function generateDailyReport(
  input: DailyReportGenerationInput,
  piSettings: PISettings,
): Promise<DailyReportGenerationResult> {
  const app = nativeApp();
  if (!app) {
    throw new Error("AI 生成日报只能在 Wails 桌面客户端中使用");
  }
  return app.GenerateDailyReport(input, piSettings);
}

export async function saveDailyReportToken(
  apiUrl: string,
  employeeId: string,
  token: string,
): Promise<void> {
  const normalizedApiUrl = apiUrl.trim();
  const normalizedEmployeeId = employeeId.trim();
  const normalizedToken = token.trim();
  if (!normalizedApiUrl) throw new Error("请输入日报 API 地址");
  if (!normalizedEmployeeId) throw new Error("请输入工号");
  if (!normalizedToken) throw new Error("请输入日报 Token");
  await requireDailyReportNativeApp().SaveDailyReportToken(
    normalizedApiUrl,
    normalizedEmployeeId,
    normalizedToken,
  );
}

export async function deleteDailyReportToken(
  apiUrl: string,
  employeeId: string,
): Promise<void> {
  const normalizedApiUrl = apiUrl.trim();
  const normalizedEmployeeId = employeeId.trim();
  if (!normalizedApiUrl) throw new Error("请输入日报 API 地址");
  if (!normalizedEmployeeId) throw new Error("请输入工号");
  await requireDailyReportNativeApp().DeleteDailyReportToken(
    normalizedApiUrl,
    normalizedEmployeeId,
  );
}

export async function hasDailyReportToken(
  apiUrl: string,
  employeeId: string,
): Promise<boolean> {
  const normalizedApiUrl = apiUrl.trim();
  const normalizedEmployeeId = employeeId.trim();
  const app = nativeApp();
  if (!app || !normalizedApiUrl || !normalizedEmployeeId) return false;
  return app.HasDailyReportToken(normalizedApiUrl, normalizedEmployeeId);
}

export async function submitDailyReport(
  settings: DailyReportSettings,
  reportDate: string,
  content: string,
): Promise<DailyReportSubmissionResult> {
  const normalizedApiUrl = settings.apiUrl.trim();
  const normalizedEmployeeId = settings.employeeId.trim();
  const normalizedReportDate = reportDate.trim();
  if (!normalizedApiUrl) throw new Error("请输入日报 API 地址");
  if (!normalizedEmployeeId) throw new Error("请输入工号");
  if (!normalizedReportDate) throw new Error("请选择日报日期");
  if (!content.trim()) throw new Error("日报内容不能为空");

  const response = await requireDailyReportNativeApp().SubmitDailyReport(
    normalizedApiUrl,
    normalizedEmployeeId,
    normalizedReportDate,
    content,
  );
  if (response.action !== "inserted" && response.action !== "updated") {
    throw new Error("日报 API 返回了未知的提交结果");
  }
  return {
    id:
      typeof response.id === "string" || typeof response.id === "number"
        ? response.id
        : "?",
    action: response.action,
    message: response.message,
  };
}

export async function savePlaneToken(
  settings: PlaneSettings,
  token: string,
): Promise<void> {
  if (!token.trim()) throw new Error("请输入 Plane Personal Access Token");
  await requireNativeApp().SavePlaneToken(
    settings.baseUrl,
    settings.workspaceSlug,
    token.trim(),
  );
}

export async function setupPlaneConnection(
  serviceAddress: string,
  token: string,
): Promise<PlaneConnectionSetup> {
  if (!serviceAddress.trim()) throw new Error("请输入 Plane 服务地址");
  return requireNativeApp().SetupPlaneConnection(
    serviceAddress.trim(),
    token.trim(),
  );
}

export async function deletePlaneToken(
  settings: PlaneSettings,
): Promise<void> {
  await requireNativeApp().DeletePlaneToken(
    settings.baseUrl,
    settings.workspaceSlug,
  );
}

export async function hasPlaneToken(
  settings: PlaneSettings,
): Promise<boolean> {
  const app = nativeApp();
  if (!app) return false;
  return app.HasPlaneToken(settings.baseUrl, settings.workspaceSlug);
}

export async function testPlaneConnection(
  settings: PlaneSettings,
): Promise<PlaneConnectionStatus> {
  return requireNativeApp().TestPlaneConnection(
    settings.baseUrl,
    settings.workspaceSlug,
    settings.projectId,
  );
}

export async function listPlaneProjects(
  settings: PlaneSettings,
): Promise<PlaneProject[]> {
  return requireNativeApp().ListPlaneProjects(
    settings.baseUrl,
    settings.workspaceSlug,
  );
}

export async function collectPlaneWorkItems(
  settings: PlaneSettings,
): Promise<PlaneCandidatePayload[]> {
  return requireNativeApp().CollectPlaneWorkItems(
    settings.baseUrl,
    settings.workspaceSlug,
    settings.projectId,
    settings.projectIdentifier ?? "",
  );
}

export async function loadPlaneWorkItemDetails(
  settings: PlaneSettings,
  workItemId: string,
): Promise<PlaneCandidatePayload> {
  return requireNativeApp().LoadPlaneWorkItemDetails(
    settings.baseUrl,
    settings.workspaceSlug,
    settings.projectId,
    settings.projectIdentifier ?? "",
    workItemId,
  );
}

export function openExternalURL(url: string): void {
  if (window.runtime?.BrowserOpenURL) {
    window.runtime.BrowserOpenURL(url);
    return;
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

export async function analyzePlaneCandidate(
  sourceMarkdown: string,
  settings: PISettings,
): Promise<CandidateAnalysis> {
  return requireNativeApp().AnalyzePlaneCandidate(sourceMarkdown, settings);
}

export async function analyzeRequirements(
  input: RequirementAnalysisInput,
  settings: PISettings,
): Promise<RequirementAnalysisResult> {
  return requireNativeApp().AnalyzeRequirements(input, settings);
}
