import type { StateStorage } from "zustand/middleware";
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

