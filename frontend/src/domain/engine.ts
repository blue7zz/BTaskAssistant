export type PIThinkingEffort = "low" | "medium" | "high" | "xhigh";

export const NATIVE_PI_ENGINE = "pi" as const;
export const CODEX_ENGINE = "codex" as const;

export function migrateEngineIdentity(
  value: unknown,
  fallback: typeof NATIVE_PI_ENGINE | typeof CODEX_ENGINE,
): string {
  if (typeof value !== "string" || value.trim() === "") return fallback;
  const normalized = value.trim().toLocaleLowerCase();
  if (normalized === CODEX_ENGINE) return CODEX_ENGINE;
  if (
    normalized === NATIVE_PI_ENGINE ||
    normalized === "omp" ||
    normalized === "oh-my-pi" ||
    normalized === "pi / oh-my-pi"
  ) {
    return NATIVE_PI_ENGINE;
  }
  return value;
}

export interface PISettings {
  model: string;
  thinkingEffort: PIThinkingEffort;
  timeoutMinutes: number;
}

export const DEFAULT_PI_SETTINGS: PISettings = {
  model: "",
  thinkingEffort: "xhigh",
  timeoutMinutes: 3,
};

const THINKING_EFFORTS: PIThinkingEffort[] = [
  "low",
  "medium",
  "high",
  "xhigh",
];

export function normalizePISettings(
  settings?: {
    model?: string;
    thinkingEffort?: string;
    timeoutMinutes?: number;
  },
): PISettings {
  const thinkingEffort =
    settings?.thinkingEffort === "max" ||
    !THINKING_EFFORTS.includes(
      settings?.thinkingEffort as PIThinkingEffort,
    )
      ? DEFAULT_PI_SETTINGS.thinkingEffort
      : (settings?.thinkingEffort as PIThinkingEffort);
  const timeoutMinutes = [1, 3, 5, 10].includes(
    Number(settings?.timeoutMinutes),
  )
    ? Number(settings?.timeoutMinutes)
    : DEFAULT_PI_SETTINGS.timeoutMinutes;
  return {
    model: settings?.model?.trim() ?? DEFAULT_PI_SETTINGS.model,
    thinkingEffort,
    timeoutMinutes,
  };
}
