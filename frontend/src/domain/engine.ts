export type PIThinkingEffort = "low" | "medium" | "high" | "xhigh";

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
