import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  DEFAULT_DAILY_REPORT_AI_SETTINGS,
  DEFAULT_DAILY_REPORT_SETTINGS,
  createEmptyDailyReportDraft,
} from "../domain/report";
import { useWorkspaceStore } from "../store/workspace";
import { DailyReportAIDialog } from "./DailyReportAIDialog";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

function setTextareaValue(textarea: HTMLTextAreaElement, value: string) {
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(
    textarea,
    value,
  );
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

function findButton(container: HTMLElement, text: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.includes(text),
  )!;
}

describe("Daily report AI dialog", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    window.localStorage.clear();
    const draft = createEmptyDailyReportDraft("2026-07-30");
    useWorkspaceStore.setState({
      tasks: [],
      dailyReportSettings: {
        ...DEFAULT_DAILY_REPORT_SETTINGS,
        organization: "万象",
        submitter: "莫淡如",
        employeeId: "DN7360",
      },
      dailyReportAISettings: { ...DEFAULT_DAILY_REPORT_AI_SETTINGS },
      dailyReportProjectHistory: [],
      dailyReportDate: draft.date,
      dailyReportDrafts: { [draft.date]: draft },
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

  it("previews generated content before explicitly filling the form", async () => {
    const generate = vi.fn().mockResolvedValue({
      reportDate: "2026-07-30",
      results: [
        {
          projectNo: "会议",
          projectName: "需求评审",
          task: "确认验收边界",
          status: "已完成",
          progress: "100%",
          evidence: ["会议纪要"],
        },
      ],
      blockers: [],
      reviews: [],
      nextActions: [
        {
          projectNo: "y16",
          projectName: "Y16 App",
          goal: "完成联调",
          deadline: "18:00 前",
          inferred: false,
        },
      ],
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
    const onClose = vi.fn();
    const nativeConfirm = vi.spyOn(window, "confirm").mockReturnValue(false);

    await act(async () => {
      root.render(
        createElement(DailyReportAIDialog, {
          open: true,
          onClose,
          onSuccess,
          onError: vi.fn(),
        }),
      );
    });

    await act(async () => {
      setTextareaValue(
        container.querySelector(
          'textarea[aria-label="日报人工补充"]',
        ) as HTMLTextAreaElement,
        "参加需求评审",
      );
    });
    await act(async () => {
      findButton(container, "生成预览").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(generate).toHaveBeenCalledTimes(1);
    const input = generate.mock.calls[0][0];
    expect(input).toMatchObject({
      reportDate: "2026-07-30",
      organization: "万象",
      level: "L1",
      role: "FE",
      workflowTasks: [],
      manualDescription: "参加需求评审",
    });
    expect(JSON.stringify(input)).not.toContain("DN7360");
    expect(JSON.stringify(input)).not.toContain("report/submit");
    expect(
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-30"].results[0]
        .projectNo,
    ).toBe("");
    expect(container.textContent).toContain("确认生成结果");
    expect(container.textContent).toContain("需求评审");
    expect(container.textContent).toContain(
      "确认验收边界；已完成；100%；会议纪要",
    );
    expect(container.textContent).toContain("- 无");

    await act(async () => {
      useWorkspaceStore.getState().updateDailyReportDraft({
        results: [
          {
            id: "existing-result",
            projectNo: "OLD",
            projectName: "原有日报",
            description: "不能静默保留的旧正文",
          },
        ],
      });
    });
    expect(container.textContent).toContain(
      "当前日期已有日报正文，确认后将替换四个正文区块",
    );

    await act(async () => findButton(container, "确认填入表单").click());

    const filled =
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-30"];
    expect(filled.results[0]).toMatchObject({
      projectNo: "会议",
      projectName: "需求评审",
      description: "确认验收边界；已完成；100%；会议纪要",
    });
    expect(filled.nextActions).toEqual([
      expect.objectContaining({
        projectNo: "y16",
        goal: "Y16 App 完成联调",
        deadline: "18:00 前",
      }),
    ]);
    expect(onSuccess).toHaveBeenCalledWith(
      "AI 日报已填入表单，请核对后再上传或复制",
    );
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(nativeConfirm).not.toHaveBeenCalled();
  });

  it("remembers selected projects and refuses to fill a different date", async () => {
    const generate = vi.fn().mockResolvedValue({
      reportDate: "2026-07-30",
      results: [],
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
    useWorkspaceStore.setState({
      dailyReportProjectHistory: [
        {
          id: "project-y16",
          projectNo: "y16",
          projectName: "wx-y16-app",
          path: "/workspace/wx-y16-app",
          lastUsedAt: "2026-07-29T10:00:00Z",
        },
      ],
    });
    const onError = vi.fn();

    await act(async () => {
      root.render(
        createElement(DailyReportAIDialog, {
          open: true,
          onClose: vi.fn(),
          onSuccess: vi.fn(),
          onError,
        }),
      );
    });

    await act(async () => {
      (
        container.querySelector(
          '.daily-report-project-option input[type="checkbox"]',
        ) as HTMLInputElement
      ).click();
      findButton(container, "生成预览").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(generate.mock.calls[0][0].projects).toEqual([
      {
        projectNo: "y16",
        projectName: "wx-y16-app",
        path: "/workspace/wx-y16-app",
      },
    ]);
    expect(
      useWorkspaceStore.getState().dailyReportProjectHistory[0].lastUsedAt,
    ).not.toBe("2026-07-29T10:00:00Z");

    await act(async () => {
      useWorkspaceStore.getState().selectDailyReportDate("2026-07-31");
    });
    await act(async () => findButton(container, "确认填入表单").click());

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "报告日期已变化，请按当前日期重新生成" }),
    );
    expect(
      useWorkspaceStore.getState().dailyReportDrafts["2026-07-31"].results[0]
        .projectNo,
    ).toBe("");
  });

  it("explains missing organization before invoking AI", async () => {
    const generate = vi.fn();
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          GenerateDailyReport: generate,
        },
      },
    } as unknown as typeof window.go;
    useWorkspaceStore.setState({
      dailyReportSettings: { ...DEFAULT_DAILY_REPORT_SETTINGS },
    });
    const onError = vi.fn();

    await act(async () => {
      root.render(
        createElement(DailyReportAIDialog, {
          open: true,
          onClose: vi.fn(),
          onSuccess: vi.fn(),
          onError,
        }),
      );
    });
    await act(async () => {
      setTextareaValue(
        container.querySelector(
          'textarea[aria-label="日报人工补充"]',
        ) as HTMLTextAreaElement,
        "参加需求评审",
      );
      findButton(container, "生成预览").click();
    });

    expect(container.textContent).toContain("请先在“日报设置”中填写组织");
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "请先在日报设置中填写组织" }),
    );
    expect(generate).not.toHaveBeenCalled();
  });

  it("sends only explicitly selected report-day workflow tasks", async () => {
    const generate = vi.fn().mockResolvedValue({
      reportDate: "2026-07-30",
      results: [],
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
    const taskID = useWorkspaceStore.getState().createTask({
      title: "实现 AI 日报",
      summary: "完成生成弹窗",
      projectName: "BTaskAssistant",
      projectPath: "/workspace/BTaskAssistant",
    });
    const currentTask = useWorkspaceStore.getState().tasks[0];
    useWorkspaceStore.setState({
      tasks: [
        {
          ...currentTask,
          id: taskID,
          status: "development",
          development: {
            ...currentTask.development,
            state: "completed",
            resultNote: "前端测试通过",
          },
          updatedAt: "2026-07-30T08:00:00Z",
        },
        {
          ...currentTask,
          id: "older-task",
          title: "昨天的任务",
          updatedAt: "2026-07-29T08:00:00Z",
        },
      ],
    });

    await act(async () => {
      root.render(
        createElement(DailyReportAIDialog, {
          open: true,
          onClose: vi.fn(),
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
    });
    const taskLabel = Array.from(
      container.querySelectorAll<HTMLLabelElement>(
        ".daily-report-project-option",
      ),
    ).find((label) => label.textContent?.includes("实现 AI 日报"));
    expect(taskLabel).toBeTruthy();
    expect(container.textContent).not.toContain("昨天的任务");

    await act(async () => {
      taskLabel?.querySelector<HTMLInputElement>('input[type="checkbox"]')?.click();
      findButton(container, "生成预览").click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(generate.mock.calls[0][0].workflowTasks).toEqual([
      expect.objectContaining({
        taskId: taskID,
        title: "实现 AI 日报",
        projectName: "BTaskAssistant",
        status: "development",
        summary: "完成生成弹窗",
        developmentState: "completed",
        developmentResult: "前端测试通过",
      }),
    ]);
    expect(generate.mock.calls[0][0].manualDescription).toBe("");
    expect(JSON.stringify(generate.mock.calls[0][0].workflowTasks)).not.toContain(
      "/workspace/BTaskAssistant",
    );
  });

  it("limits a generation to eight selected projects", async () => {
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          GenerateDailyReport: vi.fn(),
        },
      },
    } as unknown as typeof window.go;
    useWorkspaceStore.setState({
      dailyReportProjectHistory: Array.from({ length: 9 }, (_, index) => ({
        id: `project-${index}`,
        projectNo: `P${index}`,
        projectName: `项目 ${index}`,
        path: `/workspace/project-${index}`,
        lastUsedAt: `2026-07-${String(20 + index).padStart(2, "0")}T10:00:00Z`,
      })),
    });
    const onError = vi.fn();

    await act(async () => {
      root.render(
        createElement(DailyReportAIDialog, {
          open: true,
          onClose: vi.fn(),
          onSuccess: vi.fn(),
          onError,
        }),
      );
    });
    const checkboxes = Array.from(
      container.querySelectorAll<HTMLInputElement>(
        '.daily-report-project-option input[type="checkbox"]',
      ),
    );
    await act(async () => {
      checkboxes.forEach((checkbox) => checkbox.click());
    });

    expect(container.textContent).toContain("已选择 8/8 个");
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "每次最多选择 8 个项目" }),
    );
  });

  it("allows a slow generation to be closed and ignores its late result", async () => {
    let resolveGeneration!: (value: {
      reportDate: string;
      results: never[];
      blockers: never[];
      reviews: never[];
      nextActions: never[];
    }) => void;
    const pendingGeneration = new Promise<{
      reportDate: string;
      results: never[];
      blockers: never[];
      reviews: never[];
      nextActions: never[];
    }>((resolve) => {
      resolveGeneration = resolve;
    });
    window.go = {
      main: {
        App: {
          SaveState: vi.fn().mockResolvedValue(undefined),
          GenerateDailyReport: vi.fn().mockReturnValue(pendingGeneration),
        },
      },
    } as unknown as typeof window.go;
    const onClose = vi.fn();

    await act(async () => {
      root.render(
        createElement(DailyReportAIDialog, {
          open: true,
          onClose,
          onSuccess: vi.fn(),
          onError: vi.fn(),
        }),
      );
    });
    await act(async () => {
      setTextareaValue(
        container.querySelector(
          'textarea[aria-label="日报人工补充"]',
        ) as HTMLTextAreaElement,
        "今天完成 Y06 登录页联调",
      );
      findButton(container, "生成预览").click();
    });

    const closeButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="关闭并忽略本次 AI 生成"]',
    );
    expect(closeButton).toBeTruthy();
    await act(async () => closeButton?.click());
    expect(onClose).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveGeneration({
        reportDate: "2026-07-30",
        results: [],
        blockers: [],
        reviews: [],
        nextActions: [],
      });
      await pendingGeneration;
    });
    expect(container.textContent).not.toContain("确认生成结果");
  });
});
