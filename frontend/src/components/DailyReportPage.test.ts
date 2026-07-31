import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  DEFAULT_DAILY_REPORT_SETTINGS,
  createEmptyDailyReportDraft,
} from "../domain/report";
import { useWorkspaceStore } from "../store/workspace";
import { DailyReportPage } from "./DailyReportPage";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function setInputValue(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const prototype =
    input instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function findButton(container: HTMLElement, text: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.includes(text),
  )!;
}

describe("Daily report page", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    window.go = undefined;
    const dailyReportDraft = createEmptyDailyReportDraft("2026-07-30");
    useWorkspaceStore.setState({
      dailyReportSettings: {
        ...DEFAULT_DAILY_REPORT_SETTINGS,
        organization: "技术中心",
        submitter: "张三",
        employeeId: "DN1111",
      },
      dailyReportDate: dailyReportDraft.date,
      dailyReportDrafts: { [dailyReportDraft.date]: dailyReportDraft },
      hydrated: true,
    });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    vi.spyOn(window, "confirm").mockReturnValue(true);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    window.go = undefined;
    vi.restoreAllMocks();
  });

  it("updates the live Markdown and resets only report content", async () => {
    const onSuccess = vi.fn();
    await act(async () => {
      root.render(
        createElement(DailyReportPage, {
          onOpenSettings: vi.fn(),
          onSuccess,
          onError: vi.fn(),
        }),
      );
    });

    const projectNo = container.querySelector(
      'input[aria-label="今日结果项目编号"]',
    ) as HTMLInputElement;
    const projectName = container.querySelector(
      'input[aria-label="今日结果项目名"]',
    ) as HTMLInputElement;
    const description = container.querySelector(
      'textarea[aria-label="今日结果描述"]',
    ) as HTMLTextAreaElement;

    await act(async () => {
      setInputValue(projectNo, "BT-18");
      setInputValue(projectName, "日报功能");
      setInputValue(description, "完成页面并通过测试");
    });

    expect(container.querySelector(".daily-report-preview")?.textContent).toContain(
      "- [BT-18 | 日报功能] 完成页面并通过测试",
    );

    const dateInput = container.querySelector(
      'input[aria-label="日报日期"]',
    ) as HTMLInputElement;
    await act(async () => setInputValue(dateInput, "2026-07-29"));
    expect(
      (
        container.querySelector(
          'input[aria-label="今日结果项目编号"]',
        ) as HTMLInputElement
      ).value,
    ).toBe("");
    await act(async () => setInputValue(dateInput, "2026-07-30"));
    expect(
      (
        container.querySelector(
          'input[aria-label="今日结果项目编号"]',
        ) as HTMLInputElement
      ).value,
    ).toBe("BT-18");

    const addTaskButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("添加任务"))!;
    await act(async () => addTaskButton.click());
    expect(
      container.querySelectorAll('input[aria-label="今日结果项目编号"]'),
    ).toHaveLength(2);

    const resetButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("重置正文"))!;
    await act(async () => resetButton.click());

    expect(useWorkspaceStore.getState().dailyReportSettings.submitter).toBe(
      "张三",
    );
    const resetDraft =
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-30"];
    expect(useWorkspaceStore.getState().dailyReportDate).toBe("2026-07-30");
    expect(resetDraft.results).toHaveLength(1);
    expect(resetDraft.results[0].projectNo).toBe("");
    expect(onSuccess).toHaveBeenCalledWith(
      "日报正文已重置，个人与接口设置已保留",
    );
  });

  it("opens AI generation from the left side of the action bar", async () => {
    await act(async () => {
      root.render(
        createElement(DailyReportPage, {
          onOpenSettings: vi.fn(),
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
    });

    await act(async () => {
      const button = findButton(container, "AI 生成日报");
      button.click();
    });

    expect(container.querySelector('[role="dialog"]')?.textContent).toContain(
      "AI 生成日报",
    );
    expect(container.textContent).toContain(
      "浏览器预览模式不能执行本机 AI",
    );
  });

  it("fills the visible form and closes the dialog after confirming an AI preview", async () => {
    const generate = vi.fn().mockResolvedValue({
      reportDate: "2026-07-30",
      results: [
        {
          projectNo: "Y06",
          projectName: "Y06 App",
          task: "完成登录页联调",
          status: "已完成",
          progress: "100%",
          evidence: ["回归记录"],
        },
      ],
      blockers: [],
      reviews: [],
      nextActions: [],
    });
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          GenerateDailyReport: generate,
        },
      },
    } as unknown as typeof window.go;
    const onSuccess = vi.fn();

    await act(async () => {
      root.render(
        createElement(DailyReportPage, {
          onOpenSettings: vi.fn(),
          onSuccess,
          onError: vi.fn(),
        }),
      );
    });

    await act(async () => {
      setInputValue(
        container.querySelector(
          'textarea[aria-label="今日结果描述"]',
        ) as HTMLTextAreaElement,
        "需要被替换的旧正文",
      );
      findButton(container, "AI 生成日报").click();
    });
    await act(async () => {
      setInputValue(
        container.querySelector(
          'textarea[aria-label="日报人工补充"]',
        ) as HTMLTextAreaElement,
        "今天完成 Y06 登录页联调",
      );
      findButton(container, "生成预览").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.querySelector('[role="dialog"]')?.textContent).toContain(
      "确认生成结果",
    );

    await act(async () => {
      findButton(container, "确认填入表单").click();
    });

    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(
      (
        container.querySelector(
          'textarea[aria-label="今日结果描述"]',
        ) as HTMLTextAreaElement
      ).value,
    ).toBe("完成登录页联调；已完成；100%；回归记录");
    expect(
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-30"].results[0],
    ).toMatchObject({
      projectNo: "Y06",
      projectName: "Y06 App",
      description: "完成登录页联调；已完成；100%；回归记录",
    });
    expect(onSuccess).toHaveBeenCalledWith(
      "AI 日报已填入表单，请核对后再上传或复制",
    );
  });

  it("submits the generated report through the native bridge", async () => {
    const submit = vi.fn().mockResolvedValue({
      id: 17,
      action: "inserted",
      message: "提交成功",
    });
    window.go = {
      main: {
        App: {
          HasDailyReportToken: vi.fn().mockResolvedValue(true),
          SubmitDailyReport: submit,
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(DailyReportPage, {
          onOpenSettings: vi.fn(),
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    const uploadButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("上传云端"))!;
    await act(async () => {
      uploadButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(submit).toHaveBeenCalledTimes(1);
    expect(submit.mock.calls[0][0]).toBe(
      "https://ep.jsyyds.com/api/v1/report/submit",
    );
    expect(submit.mock.calls[0][1]).toBe("DN1111");
    expect(submit.mock.calls[0][2]).toBe("2026-07-30");
    expect(submit.mock.calls[0][3]).toContain("# 日报 · 张三·2026-07-30");
    expect(container.textContent).toContain("上传成功 #17 · 首次提交");
  });

  it("marks a successful upload stale when the draft changes in flight", async () => {
    let resolveSubmit: (value: {
      id: number;
      action: "updated";
      message: string;
    }) => void = () => undefined;
    const submit = vi.fn(
      () =>
        new Promise<{
          id: number;
          action: "updated";
          message: string;
        }>((resolve) => {
          resolveSubmit = resolve;
        }),
    );
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          HasDailyReportToken: vi.fn().mockResolvedValue(true),
          SubmitDailyReport: submit,
        },
      },
    } as unknown as typeof window.go;

    await act(async () => {
      root.render(
        createElement(DailyReportPage, {
          onOpenSettings: vi.fn(),
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    const uploadButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("上传云端"))!;
    await act(async () => uploadButton.click());

    await act(async () => {
      setInputValue(
        container.querySelector(
          'textarea[aria-label="今日结果描述"]',
        ) as HTMLTextAreaElement,
        "上传过程中修改的内容",
      );
    });
    await act(async () => {
      resolveSubmit({ id: 18, action: "updated", message: "提交成功" });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain(
      "上传成功 #18 · 已覆盖旧版本；当前内容已修改，请重新上传",
    );
  });
});
