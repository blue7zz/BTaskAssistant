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
import type { TransitionGates } from "../domain/workflow";
import { validateTransition as validateInBrowser } from "../domain/workflow";

export interface EngineStatus {
  id: "pi" | "codex";
  label: string;
  configured: boolean;
  description: string;
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
  AnalyzePlaneCandidate(sourceMarkdown: string): Promise<CandidateAnalysis>;
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: NativeApp;
      };
    };
  }
}

const nativeApp = (): NativeApp | undefined => window.go?.main?.App;

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
      await app.SaveState(value);
      return;
    }
    window.localStorage.setItem(name, value);
  },
  async removeItem(name) {
    const app = nativeApp();
    if (app) {
      await app.ClearState();
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
      label: "PI / oh-my-pi",
      configured: false,
      description: "浏览器预览模式：自动执行接口未配置。",
    },
    {
      id: "codex",
      label: "Codex",
      configured: false,
      description: "当前可复制已确认提示词，自动执行接口待配置。",
    },
  ];
}

function requireNativeApp(): NativeApp {
  const app = nativeApp();
  if (!app) {
    throw new Error("Plane 和 PI 集成只能在 Wails 桌面客户端中使用");
  }
  return app;
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

export async function analyzePlaneCandidate(
  sourceMarkdown: string,
): Promise<CandidateAnalysis> {
  return requireNativeApp().AnalyzePlaneCandidate(sourceMarkdown);
}
