import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_PI_SETTINGS } from "../domain/engine";
import {
  DEFAULT_DAILY_REPORT_AI_SETTINGS,
  DEFAULT_DAILY_REPORT_SETTINGS,
  type DailyReportSettings as DailyReportSettingsValue,
} from "../domain/report";
import { submitDailyReport } from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";
import { DailyReportSettings } from "./DailyReportSettings";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function setTextareaValue(textarea: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )?.set;
  setter?.call(textarea, value);
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

function setSelectValue(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLSelectElement.prototype,
    "value",
  )?.set;
  setter?.call(select, value);
  select.dispatchEvent(new Event("change", { bubbles: true }));
}

function findButton(container: HTMLElement, text: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.includes(text),
  )!;
}

describe("Daily report settings", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    window.go = undefined;
    useWorkspaceStore.setState({
      dailyReportSettings: { ...DEFAULT_DAILY_REPORT_SETTINGS },
      dailyReportAISettings: { ...DEFAULT_DAILY_REPORT_AI_SETTINGS },
      dailyReportProjectHistory: [],
      piSettings: { ...DEFAULT_PI_SETTINGS },
      hydrated: true,
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    delete window.go;
    vi.restoreAllMocks();
  });

  it("saves profile settings and forces the L3 role to TL", async () => {
    const onSuccess = vi.fn();
    await act(async () => {
      root.render(
        createElement(DailyReportSettings, {
          onSuccess,
          onError: vi.fn(),
        }),
      );
    });

    expect(container.textContent).toContain(
      "浏览器预览模式仅支持保存非敏感设置",
    );
    expect(
      (
        container.querySelector(
          'input[aria-label="日报 Token"]',
        ) as HTMLInputElement
      ).disabled,
    ).toBe(true);

    await act(async () => {
      setInputValue(
        container.querySelector(
          'input[aria-label="日报组织"]',
        ) as HTMLInputElement,
        " 技术中心 ",
      );
      setInputValue(
        container.querySelector(
          'input[aria-label="日报提交人"]',
        ) as HTMLInputElement,
        " 张三 ",
      );
      setInputValue(
        container.querySelector(
          'input[aria-label="日报工号"]',
        ) as HTMLInputElement,
        " DN1111 ",
      );
      setInputValue(
        container.querySelector(
          'input[aria-label="日报 API 地址"]',
        ) as HTMLInputElement,
        "",
      );
      setSelectValue(
        container.querySelector(
          'select[aria-label="日报层级"]',
        ) as HTMLSelectElement,
        "L3",
      );
    });

    const role = container.querySelector(
      'select[aria-label="日报岗位"]',
    ) as HTMLSelectElement;
    expect(role.disabled).toBe(true);
    expect(role.value).toBe("TL");

    await act(async () => {
      findButton(container, "保存日报设置").click();
      await Promise.resolve();
    });

    expect(useWorkspaceStore.getState().dailyReportSettings).toMatchObject({
      organization: "技术中心",
      submitter: "张三",
      employeeId: "DN1111",
      level: "L3",
      role: "TL",
      apiUrl: "",
    });
    expect(onSuccess).toHaveBeenCalledWith("日报设置已保存");
  });

  it("saves AI generation options and manages project history", async () => {
    useWorkspaceStore.setState({
      dailyReportProjectHistory: [
        {
          id: "history-y16",
          projectNo: "y16",
          projectName: "wx-y16-app",
          path: "/workspace/wx-y16-app",
          lastUsedAt: "2026-07-30T10:00:00Z",
        },
      ],
    });
    const onSuccess = vi.fn();
    await act(async () => {
      root.render(
        createElement(DailyReportSettings, {
          onSuccess,
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

    await act(async () => {
      setSelectValue(
        container.querySelector(
          'select[aria-label="日报 AI 引擎"]',
        ) as HTMLSelectElement,
        "codex",
      );
      setInputValue(
        container.querySelector(
          'input[aria-label="日报 AI 模型"]',
        ) as HTMLInputElement,
        " gpt-report ",
      );
      setSelectValue(
        container.querySelector(
          'select[aria-label="日报 AI 思考强度"]',
        ) as HTMLSelectElement,
        "high",
      );
      setSelectValue(
        container.querySelector(
          'select[aria-label="日报 AI 执行时限"]',
        ) as HTMLSelectElement,
        "5",
      );
      setInputValue(
        container.querySelector(
          'input[aria-label="日报 Git 作者"]',
        ) as HTMLInputElement,
        " blue ",
      );
      setTextareaValue(
        container.querySelector(
          'textarea[aria-label="日报 AI 附加写作指令"]',
        ) as HTMLTextAreaElement,
        " 表述精炼 ",
      );
      (
        container.querySelector(
          'input[aria-label="日报包含未提交变更"]',
        ) as HTMLInputElement
      ).click();
    });

    await act(async () => {
      findButton(container, "保存日报设置").click();
      await Promise.resolve();
    });

    expect(useWorkspaceStore.getState().dailyReportAISettings).toEqual({
      engine: "codex",
      customInstructions: "表述精炼",
      gitAuthor: "blue",
      includeUncommitted: false,
    });
    expect(useWorkspaceStore.getState().piSettings).toEqual({
      model: "gpt-report",
      thinkingEffort: "high",
      timeoutMinutes: 5,
    });
    expect(onSuccess).toHaveBeenCalledWith("日报设置已保存");

    await act(async () => {
      findButton(container, "删除").click();
    });
    expect(useWorkspaceStore.getState().dailyReportProjectHistory).toEqual([]);
  });

  it("stores and removes the token through the desktop credential bridge", async () => {
    useWorkspaceStore.setState({
      dailyReportSettings: {
        ...DEFAULT_DAILY_REPORT_SETTINGS,
        employeeId: "DN1111",
      },
    });
    const hasToken = vi.fn().mockResolvedValue(false);
    const saveToken = vi.fn().mockResolvedValue(undefined);
    const deleteToken = vi.fn().mockResolvedValue(undefined);
    const saveState = vi.fn().mockResolvedValue(undefined);
    window.go = {
      main: {
        App: {
          SaveState: saveState,
          HasDailyReportToken: hasToken,
          SaveDailyReportToken: saveToken,
          DeleteDailyReportToken: deleteToken,
        },
      },
    } as unknown as typeof window.go;
    const onSuccess = vi.fn();

    await act(async () => {
      root.render(
        createElement(DailyReportSettings, {
          onSuccess,
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
    });

    expect(hasToken).toHaveBeenCalledWith(
      DEFAULT_DAILY_REPORT_SETTINGS.apiUrl,
      "DN1111",
    );
    expect(container.textContent).not.toContain("浏览器预览模式仅支持");

    await act(async () => {
      setInputValue(
        container.querySelector(
          'input[aria-label="日报 Token"]',
        ) as HTMLInputElement,
        " report-token ",
      );
      findButton(container, "保存日报设置").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(saveToken).toHaveBeenCalledWith(
      DEFAULT_DAILY_REPORT_SETTINGS.apiUrl,
      "DN1111",
      "report-token",
    );
    expect(onSuccess).toHaveBeenCalledWith("日报设置与 Token 已保存");
    await vi.waitFor(() => expect(saveState).toHaveBeenCalled());
    expect(JSON.stringify(saveState.mock.calls)).not.toContain("report-token");

    const deleteButton = findButton(container, "删除 Token");
    expect(deleteButton.disabled).toBe(false);
    await act(async () => {
      deleteButton.click();
      await Promise.resolve();
    });

    expect(deleteToken).toHaveBeenCalledWith(
      DEFAULT_DAILY_REPORT_SETTINGS.apiUrl,
      "DN1111",
    );
    expect(onSuccess).toHaveBeenCalledWith("日报 Token 已从系统凭据库删除");
  });

  it("reports credential errors without persisting a secret", async () => {
    useWorkspaceStore.setState({
      dailyReportSettings: {
        ...DEFAULT_DAILY_REPORT_SETTINGS,
        employeeId: "DN1111",
      },
    });
    const saveError = new Error("系统凭据库不可用");
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          HasDailyReportToken: vi.fn().mockResolvedValue(false),
          SaveDailyReportToken: vi.fn().mockRejectedValue(saveError),
        },
      },
    } as unknown as typeof window.go;
    const onError = vi.fn();
    await act(async () => {
      root.render(
        createElement(DailyReportSettings, {
          onSuccess: vi.fn(),
          onError,
        }),
      );
    });

    await act(async () => {
      setInputValue(
        container.querySelector(
          'input[aria-label="日报 Token"]',
        ) as HTMLInputElement,
        "report-token",
      );
      findButton(container, "保存日报设置").click();
      await Promise.resolve();
    });

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({
        message: "系统凭据库不可用",
      }),
    );
    expect(
      JSON.stringify(useWorkspaceStore.getState().dailyReportSettings),
    ).not.toContain("report-token");
  });

  it("rejects unsafe API URLs before they enter workspace persistence", async () => {
    const onError = vi.fn();
    await act(async () => {
      root.render(
        createElement(DailyReportSettings, {
          onSuccess: vi.fn(),
          onError,
        }),
      );
    });

    await act(async () => {
      setInputValue(
        container.querySelector(
          'input[aria-label="日报 API 地址"]',
        ) as HTMLInputElement,
        "https://user:password@reports.example.com/submit",
      );
      findButton(container, "保存日报设置").click();
      await Promise.resolve();
    });

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "日报 API 地址不能包含用户名或密码" }),
    );
    expect(useWorkspaceStore.getState().dailyReportSettings.apiUrl).toBe(
      DEFAULT_DAILY_REPORT_SETTINGS.apiUrl,
    );
  });
});

describe("Daily report bridge", () => {
  afterEach(() => {
    delete window.go;
    vi.restoreAllMocks();
  });

  it("normalizes a successful native submission for the report page", async () => {
    const submit = vi.fn().mockResolvedValue({
      id: null,
      action: "inserted",
      message: "提交成功",
    });
    window.go = {
      main: { App: { SubmitDailyReport: submit } },
    } as unknown as typeof window.go;
    const settings: DailyReportSettingsValue = {
      ...DEFAULT_DAILY_REPORT_SETTINGS,
      employeeId: " DN1111 ",
    };

    await expect(
      submitDailyReport(settings, " 2026-07-30 ", "# 日报"),
    ).resolves.toEqual({
      id: "?",
      action: "inserted",
      message: "提交成功",
    });
    expect(submit).toHaveBeenCalledWith(
      DEFAULT_DAILY_REPORT_SETTINGS.apiUrl,
      "DN1111",
      "2026-07-30",
      "# 日报",
    );
  });
});
