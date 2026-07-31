import { afterEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_PI_SETTINGS } from "./engine";
import {
  dailyReportAIAvailable,
  generateDailyReport,
} from "../lib/bridge";
import {
  DEFAULT_DAILY_REPORT_AI_SETTINGS,
  DEFAULT_DAILY_REPORT_SETTINGS,
  buildDailyReportHTML,
  buildDailyReportMarkdown,
  createDailyReportNextActionRow,
  createDailyReportProjectRow,
  createDailyReportReviewRow,
  dailyReportDraftFromGenerationResult,
  isDailyReportDraftEmpty,
  localDateString,
  normalizeDailyReportAISettings,
  normalizeDailyReportAPIURL,
  normalizeDailyReportSettings,
  type DailyReportDraft,
  type DailyReportGenerationInput,
  type DailyReportGenerationResult,
} from "./report";

describe("daily report domain", () => {
  it("uses the local calendar date instead of UTC slicing", () => {
    expect(localDateString(new Date(2026, 6, 30, 0, 15))).toBe("2026-07-30");
  });

  it("builds the reference Markdown contract and exports only TOP3", () => {
    const result = {
      ...createDailyReportProjectRow(),
      projectNo: "BT-18",
      projectName: "日报接入",
      description: "完成填报页 | 已完成 | 100% | pnpm test",
    };
    const blocker = {
      ...createDailyReportProjectRow(),
      projectNo: "BT-19",
      projectName: "联调",
      description: "等待测试 Token | 一般 | 无线上影响",
    };
    const review = {
      ...createDailyReportReviewRow(),
      topic: "接口契约",
      detail: "Bearer 与 JSON 字段已核对",
      outcome: "增加自动测试",
    };
    const nextActions = ["设计验收", "接口联调", "补充测试", "不应导出"].map(
      (goal, index) => ({
        ...createDailyReportNextActionRow(),
        projectNo: `BT-${20 + index}`,
        goal,
        deadline: `${15 + index}:00 前`,
      }),
    );
    const draft: DailyReportDraft = {
      date: "2026-07-30",
      results: [result],
      blockers: [blocker],
      reviews: [review],
      nextActions,
    };

    const markdown = buildDailyReportMarkdown(
      {
        ...DEFAULT_DAILY_REPORT_SETTINGS,
        organization: "技术中心",
        submitter: "张三",
        employeeId: "DN1111",
        role: "FE",
      },
      draft,
    );

    expect(markdown).toContain("# 日报 · 张三·2026-07-30");
    expect(markdown).toContain(
      "[REPORT-ORG:技术中心] [LEVEL:L1] [TYPE:日报] [DATE:2026-07-30]",
    );
    expect(markdown).toContain("- [BT-18 | 日报接入] 完成填报页");
    expect(markdown).toContain("- 接口契约 | Bearer 与 JSON 字段已核对 | 增加自动测试");
    expect(markdown).toContain("- TOP3: [BT-22 | 补充测试]（截止：17:00 前）");
    expect(markdown).not.toContain("不应导出");
  });

  it("forces the department lead role for L3 and escapes HTML exports", () => {
    const settings = normalizeDailyReportSettings({ level: "L3", role: "FE" });
    expect(settings.role).toBe("TL");
    expect(buildDailyReportHTML("<script>alert('x')</script>")).not.toContain(
      "<script>",
    );
    expect(buildDailyReportHTML("<script>alert('x')</script>")).toContain(
      "&lt;script&gt;",
    );
  });

  it("turns generated rows into a dated draft with local ids and TOP3", () => {
    const draft = dailyReportDraftFromGenerationResult({
      reportDate: "2026-07-30",
      results: [
        {
          projectNo: "Y16",
          projectName: "Y16 App",
          task: "完成日报入口",
          status: "已完成",
          progress: "100%",
          evidence: ["commit abc123", "pnpm test"],
        },
      ],
      blockers: [],
      reviews: [
        {
          scene: "联调复盘",
          cause: "字段理解不一致",
          action: "补充契约测试",
          validation: "定向测试通过",
          teamRisk: true,
        },
      ],
      nextActions: ["一", "二", "三", "四"].map((goal) => ({
        projectNo: "Y16",
        projectName: "Y16 App",
        goal,
        deadline: "18:00 前",
        inferred: false,
      })),
    });

    expect(draft.date).toBe("2026-07-30");
    expect(draft.results[0]).toMatchObject({
      projectNo: "Y16",
      projectName: "Y16 App",
      description: "完成日报入口；已完成；100%；commit abc123 · pnpm test",
    });
    expect(draft.results[0].id).toMatch(/^report-project_/);
    expect(draft.reviews[0].id).toMatch(/^report-review_/);
    expect(draft.reviews[0]).toMatchObject({
      topic: "⚠ 风险 · 联调复盘",
      detail: "根因：字段理解不一致；处置：补充契约测试",
      outcome: "验证结果：定向测试通过",
    });
    expect(draft.nextActions).toHaveLength(3);
    expect(draft.nextActions[2]).toMatchObject({
      projectNo: "Y16",
      goal: "Y16 App 三",
      deadline: "18:00 前",
    });
    expect(isDailyReportDraftEmpty(draft)).toBe(false);
    expect(
      isDailyReportDraftEmpty({
        date: "2026-07-30",
        results: [],
        blockers: [],
        reviews: [],
        nextActions: [],
      }),
    ).toBe(true);
  });

  it("normalizes structured AI rows without exposing prompt field names", () => {
    const draft = dailyReportDraftFromGenerationResult({
      reportDate: "2026-07-30",
      results: [
        {
          projectNo: "",
          projectName: "",
          task: "修复日报\n预览",
          status: "done",
          progress: 47,
          evidence: [
            "manualDescription",
            "workflowTasks: task-1",
            "source=gitContext",
            "customInstructions",
            '"status": "done"',
            "status：inbox",
            "summary: 内部摘要",
            "summary：内部摘要",
            "taskId=task-1",
            "updatedAt: 2026-07-30T08:00:00Z",
            "接口返回 status 200",
            "待确认",
            "commit abc123",
            "commit abc123",
          ],
        },
        {
          projectNo: "Y06",
          projectName: "Y06 App",
          task: "核对生成结果",
          status: "阻塞",
          progress: "12",
          evidence: [],
        },
        {
          projectNo: "Y15",
          projectName: "Y15 App",
          task: "确认进度",
          status: "进行中",
          progress: "未知",
          evidence: [],
        },
      ],
      blockers: [
        {
          projectNo: "Y16",
          projectName: "Y16 App",
          issue: "等待接口字段确认",
          level: "p1",
          impact: "联调暂停",
          helpTarget: "后端",
          waitDuration: "2h",
          escalate: "y",
        },
        {
          projectNo: "",
          projectName: "",
          issue: "",
          level: "P3",
          impact: "",
          helpTarget: "",
          waitDuration: "",
          escalate: "yes",
        },
      ],
      reviews: [
        {
          scene: "",
          cause: "",
          action: "补充契约测试",
          validation: "",
          teamRisk: true,
        },
      ],
      nextActions: [
        {
          projectNo: "",
          projectName: "",
          goal: "",
          deadline: "",
          inferred: true,
        },
        {
          projectNo: "Y|06]",
          projectName: "[Y06",
          goal: "完成|联调]（待确认）",
          deadline: "18:00 前",
          inferred: true,
        },
      ],
    } as unknown as DailyReportGenerationResult);

    expect(draft.results[0]).toMatchObject({
      projectNo: "未编号",
      projectName: "未命名",
      description:
        "修复日报 预览；待确认；47%；接口返回 status 200 · commit abc123",
    });
    expect(draft.results[1].description).toBe(
      "核对生成结果；阻塞；12%；待确认",
    );
    expect(draft.results[2].description).toBe(
      "确认进度；进行中；待确认；待确认",
    );
    expect(draft.blockers[0].description).toBe(
      "等待接口字段确认 | P1 | 联调暂停 | 后端 | 2h | Y",
    );
    expect(draft.blockers[1]).toMatchObject({
      projectNo: "未编号",
      projectName: "未命名",
      description: "待确认 | 待确认 | 待确认 | 待确认 | 待确认 | 待确认",
    });
    expect(draft.reviews[0]).toMatchObject({
      topic: "⚠ 风险 · 待确认",
      detail: "根因：待确认；处置：补充契约测试",
      outcome: "验证结果：待确认",
    });
    expect(draft.nextActions[0]).toMatchObject({
      projectNo: "未编号",
      goal: "未命名 待确认",
      deadline: "待确认",
    });
    expect(draft.nextActions[1]).toMatchObject({
      projectNo: "Y／06］",
      goal: "［Y06 完成／联调］（待确认）",
    });

    const markdown = buildDailyReportMarkdown(
      DEFAULT_DAILY_REPORT_SETTINGS,
      draft,
    );
    expect(markdown).not.toMatch(
      /manualDescription|workflowTasks|gitContext|customInstructions/,
    );
    expect(markdown).not.toMatch(
      /(?:status|summary|taskId|updatedAt)\s*[:=：]/i,
    );
    expect(markdown).toContain("接口返回 status 200");
    expect(markdown).not.toMatch(/[\r\n]预览/);
  });

  it("renders rule-compliant empty sections and normalizes AI settings", () => {
    const markdown = buildDailyReportMarkdown(
      DEFAULT_DAILY_REPORT_SETTINGS,
      {
        date: "2026-07-30",
        results: [],
        blockers: [],
        reviews: [],
        nextActions: [],
      },
    );

    expect(markdown).toContain("## 【今日结果】\n- 无");
    expect(markdown).toContain("## 【死锁阻碍】\n- 无");
    expect(markdown).toContain("## 【专项复盘】\n- 无");
    expect(markdown).toContain("## 【明日动作】\n- 无");
    expect(normalizeDailyReportAISettings()).toEqual(
      DEFAULT_DAILY_REPORT_AI_SETTINGS,
    );
    expect(
      normalizeDailyReportAISettings({
        engine: "codex",
        customInstructions: "保留版本号",
        gitAuthor: "blue@example.com",
        includeUncommitted: false,
      }),
    ).toEqual({
      engine: "codex",
      customInstructions: "保留版本号",
      gitAuthor: "blue@example.com",
      includeUncommitted: false,
    });
  });

  it("normalizes safe API URLs and rejects credential-bearing targets", () => {
    expect(
      normalizeDailyReportAPIURL(
        " https://REPORT.example.com:443/api/./report/submit/#section ",
      ),
    ).toBe("https://report.example.com/api/report/submit");
    expect(normalizeDailyReportAPIURL("http://127.0.0.1:8080/report/"))
      .toBe("http://127.0.0.1:8080/report");
    expect(() =>
      normalizeDailyReportAPIURL("http://reports.example.com/submit"),
    ).toThrow("只能发送到 HTTPS");
    expect(() =>
      normalizeDailyReportAPIURL("https://user:pass@reports.example.com/submit"),
    ).toThrow("不能包含用户名或密码");
    expect(() =>
      normalizeDailyReportAPIURL("https://reports.example.com/submit?token=x"),
    ).toThrow("不能包含查询参数");
  });
});

describe("daily report AI bridge", () => {
  const input: DailyReportGenerationInput = {
    reportDate: "2026-07-30",
    organization: "万象",
    level: "L1",
    role: "FE",
    engine: "pi",
    customInstructions: "保留 commit 证据",
    gitAuthor: "blue@example.com",
    includeUncommitted: true,
    projects: [
      {
        projectNo: "Y16",
        projectName: "Y16 App",
        path: "/workspace/y16",
      },
    ],
    workflowTasks: [],
    manualDescription:
      "今天参加需求评审并确认验收范围，明天完成联调。",
  };

  afterEach(() => {
    delete window.go;
    vi.restoreAllMocks();
  });

  it("reports browser mode as unavailable", async () => {
    delete window.go;
    expect(dailyReportAIAvailable()).toBe(false);
    await expect(generateDailyReport(input, DEFAULT_PI_SETTINGS)).rejects.toThrow(
      "只能在 Wails 桌面客户端中使用",
    );
  });

  it("passes only the generation contract and PI runtime to native code", async () => {
    const result = {
      reportDate: "2026-07-30",
      results: [],
      blockers: [],
      reviews: [],
      nextActions: [],
    };
    const generate = vi.fn().mockResolvedValue(result);
    window.go = {
      main: { App: { GenerateDailyReport: generate } },
    } as unknown as typeof window.go;

    expect(dailyReportAIAvailable()).toBe(true);
    await expect(
      generateDailyReport(input, DEFAULT_PI_SETTINGS),
    ).resolves.toEqual(result);
    expect(generate).toHaveBeenCalledWith(input, DEFAULT_PI_SETTINGS);
    expect(JSON.stringify(generate.mock.calls)).not.toMatch(
      /apiUrl|employeeId|token/i,
    );
  });
});
