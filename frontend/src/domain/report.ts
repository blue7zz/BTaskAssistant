import { createID } from "../lib/id";
import type { DevelopmentState, TaskStatus } from "./task";

export const DAILY_REPORT_LEVELS = ["L1", "L2", "L3D", "L3"] as const;
export type DailyReportLevel = (typeof DAILY_REPORT_LEVELS)[number];

export const DAILY_REPORT_ROLES = [
  "FE",
  "BE",
  "QA",
  "PM",
  "UI",
  "OPS",
  "TL",
] as const;
export type DailyReportRole = (typeof DAILY_REPORT_ROLES)[number];

export type DailyReportAIEngine = "pi" | "codex";

export interface DailyReportAISettings {
  engine: DailyReportAIEngine;
  customInstructions: string;
  gitAuthor: string;
  includeUncommitted: boolean;
}

export interface DailyReportProjectHistoryItem {
  id: string;
  projectNo: string;
  projectName: string;
  path: string;
  lastUsedAt: string;
}

export interface DailyReportGenerationProject {
  projectNo: string;
  projectName: string;
  path: string;
}

export interface DailyReportWorkflowTask {
  taskId: string;
  title: string;
  projectName: string;
  status: TaskStatus;
  summary: string;
  developmentState: DevelopmentState;
  developmentResult: string;
  reviewNote: string;
  updatedAt: string;
}

export interface DailyReportGenerationInput {
  reportDate: string;
  organization: string;
  level: string;
  role: string;
  engine: DailyReportAIEngine;
  customInstructions: string;
  gitAuthor: string;
  includeUncommitted: boolean;
  projects: DailyReportGenerationProject[];
  workflowTasks: DailyReportWorkflowTask[];
  manualDescription: string;
}

export type DailyReportGeneratedStatus =
  | "已完成"
  | "进行中"
  | "阻塞"
  | "已延期"
  | "待确认";

export type DailyReportGeneratedBlockerLevel =
  | "P0"
  | "P1"
  | "P2"
  | "一般"
  | "待确认";

export type DailyReportGeneratedEscalation = "Y" | "N" | "待确认";

export interface DailyReportGeneratedResultRow {
  projectNo: string;
  projectName: string;
  task: string;
  status: DailyReportGeneratedStatus;
  progress: string;
  evidence: string[];
}

export interface DailyReportGeneratedBlockerRow {
  projectNo: string;
  projectName: string;
  issue: string;
  level: DailyReportGeneratedBlockerLevel;
  impact: string;
  helpTarget: string;
  waitDuration: string;
  escalate: DailyReportGeneratedEscalation;
}

export interface DailyReportGeneratedReviewRow {
  scene: string;
  cause: string;
  action: string;
  validation: string;
  teamRisk: boolean;
}

export interface DailyReportGeneratedNextActionRow {
  projectNo: string;
  projectName: string;
  goal: string;
  deadline: string;
  inferred: boolean;
}

export interface DailyReportGenerationResult {
  reportDate: string;
  results: DailyReportGeneratedResultRow[];
  blockers: DailyReportGeneratedBlockerRow[];
  reviews: DailyReportGeneratedReviewRow[];
  nextActions: DailyReportGeneratedNextActionRow[];
}

export interface DailyReportSettings {
  organization: string;
  submitter: string;
  employeeId: string;
  level: DailyReportLevel;
  role: DailyReportRole;
  apiUrl: string;
}

export interface DailyReportProjectRow {
  id: string;
  projectNo: string;
  projectName: string;
  description: string;
}

export interface DailyReportReviewRow {
  id: string;
  topic: string;
  detail: string;
  outcome: string;
}

export interface DailyReportNextActionRow {
  id: string;
  projectNo: string;
  goal: string;
  deadline: string;
}

export interface DailyReportDraft {
  date: string;
  results: DailyReportProjectRow[];
  blockers: DailyReportProjectRow[];
  reviews: DailyReportReviewRow[];
  nextActions: DailyReportNextActionRow[];
}

export const DEFAULT_DAILY_REPORT_SETTINGS: DailyReportSettings = {
  organization: "",
  submitter: "",
  employeeId: "",
  level: "L1",
  role: "FE",
  apiUrl: "https://ep.jsyyds.com/api/v1/report/submit",
};

export const DEFAULT_DAILY_REPORT_AI_SETTINGS: DailyReportAISettings = {
  engine: "pi",
  customInstructions: "",
  gitAuthor: "",
  includeUncommitted: true,
};

export const DAILY_REPORT_LEVEL_LABELS: Record<DailyReportLevel, string> = {
  L1: "L1｜组员",
  L2: "L2｜组长",
  L3D: "L3｜副主管",
  L3: "L3｜主管",
};

export const DAILY_REPORT_ROLE_LABELS: Record<DailyReportRole, string> = {
  FE: "前端 FE",
  BE: "后端 BE",
  QA: "测试 QA",
  PM: "产品 PM",
  UI: "设计 UI",
  OPS: "运维 OPS",
  TL: "技术负责人",
};

export const DAILY_REPORT_ROLE_HINTS: Record<DailyReportRole, string> = {
  FE: "前端口径：页面/组件交付、联调进度、兼容与性能。结果应落到版本号、PR 或截图。",
  BE: "后端口径：接口交付、稳定性、性能、发布结果与根因。结果应落到接口、错误率或时延。",
  QA: "测试口径：执行率、通过率、缺陷发现/关闭效率、放行条件。结果应落到用例与缺陷单。",
  PM: "产品口径：需求推进状态、评审结论、上线节奏、指标变化与决策项。",
  UI: "设计口径：设计交付、还原验收、返工轮次与联调支撑。重点写视觉/交互风险。",
  OPS: "运维口径：服务可用性、告警处理、发布变更、巡检与备份状态。",
  TL: "技术负责人口径：聚焦各端进度、关键交付、质量稳定性与资源风险。",
};

interface DailyReportPlaceholders {
  result: string;
  blocker: string;
  nextGoal: string;
  nextDeadline: string;
  reviewTopic: string;
  reviewDetail: string;
  reviewOutcome: string;
}

const DEFAULT_PLACEHOLDERS: DailyReportPlaceholders = {
  result: "任务内容 | 状态 | 进度 xx% | 交付证据",
  blocker: "问题描述 | 等级 | 影响范围 | 求助对象 | 等待时长 | 是否升级",
  nextGoal: "项目名 + 可量化目标",
  nextDeadline: "截止时间，如 18:00 前",
  reviewTopic: "工具或复盘主题",
  reviewDetail: "场景、根因与验证结果",
  reviewOutcome: "量化收益或后续动作",
};

const ROLE_PLACEHOLDERS: Partial<
  Record<DailyReportRole, DailyReportPlaceholders>
> = {
  PM: {
    result: "需求/项目推进结果 | 状态 | 关键结论 | 指标变化",
    blocker: "卡点描述 | 责任方 | 业务影响 | 决策时限 | 是否升级",
    nextGoal: "事项 + 可量化业务目标",
    nextDeadline: "决策或上线截止时间",
    reviewTopic: "复盘主题",
    reviewDetail: "用户反馈、竞品变化或决策偏差",
    reviewOutcome: "修正动作与预期收益",
  },
  UI: {
    result: "设计交付结果 | 状态 | 还原进度 | 稿件链接或版本",
    blocker: "设计卡点 | 依赖方 | 影响页面/节点 | 等待时长 | 是否升级",
    nextGoal: "事项 + 可量化交付目标",
    nextDeadline: "评审或联调截止时间",
    reviewTopic: "返工、验收或协作主题",
    reviewDetail: "返工原因、沟通偏差或规范缺口",
    reviewOutcome: "修正动作与复用规范",
  },
  OPS: {
    result: "服务/系统/发布结果 | 状态 | 可用性或告警数据 | 证据",
    blocker: "故障/隐患 | 影响范围 | 求助对象 | 等待时长 | 升级判断",
    nextGoal: "事项 + 可量化目标",
    nextDeadline: "完成或上线时间点",
    reviewTopic: "事故复盘主题",
    reviewDetail: "事故根因、处置过程或 SOP 缺口",
    reviewOutcome: "预防措施与门禁阈值",
  },
  TL: {
    result: "部门汇总项 | 各端状态 | 关键里程碑达成率 | 证据",
    blocker: "部门级阻碍 | 影响范围 | 责任端/协作方 | 等待时长 | 升级判断",
    nextGoal: "部门 TOP 事项 + 可量化目标",
    nextDeadline: "里程碑截止时间",
    reviewTopic: "汇总复盘主题",
    reviewDetail: "跨端共性问题与根因",
    reviewOutcome: "治理动作与预期收益",
  },
};

function isLevel(value: unknown): value is DailyReportLevel {
  return DAILY_REPORT_LEVELS.includes(value as DailyReportLevel);
}

function isRole(value: unknown): value is DailyReportRole {
  return DAILY_REPORT_ROLES.includes(value as DailyReportRole);
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function localDateString(date = new Date()): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function createDailyReportProjectRow(): DailyReportProjectRow {
  return {
    id: createID("report-project"),
    projectNo: "",
    projectName: "",
    description: "",
  };
}

export function createDailyReportReviewRow(): DailyReportReviewRow {
  return {
    id: createID("report-review"),
    topic: "",
    detail: "",
    outcome: "",
  };
}

export function createDailyReportNextActionRow(): DailyReportNextActionRow {
  return {
    id: createID("report-next"),
    projectNo: "",
    goal: "",
    deadline: "",
  };
}

export function createEmptyDailyReportDraft(
  date = localDateString(),
): DailyReportDraft {
  return {
    date,
    results: [createDailyReportProjectRow()],
    blockers: [createDailyReportProjectRow()],
    reviews: [createDailyReportReviewRow()],
    nextActions: [createDailyReportNextActionRow()],
  };
}

export function normalizeDailyReportSettings(
  value?: Partial<DailyReportSettings>,
): DailyReportSettings {
  const level = isLevel(value?.level)
    ? value.level
    : DEFAULT_DAILY_REPORT_SETTINGS.level;
  let role = isRole(value?.role)
    ? value.role
    : DEFAULT_DAILY_REPORT_SETTINGS.role;
  if (level === "L3") role = "TL";
  return {
    organization: stringValue(value?.organization),
    submitter: stringValue(value?.submitter),
    employeeId: stringValue(value?.employeeId),
    level,
    role,
    apiUrl:
      typeof value?.apiUrl === "string"
        ? value.apiUrl
        : DEFAULT_DAILY_REPORT_SETTINGS.apiUrl,
  };
}

export function normalizeDailyReportAISettings(
  value?: Partial<DailyReportAISettings>,
): DailyReportAISettings {
  return {
    engine: value?.engine === "codex" ? "codex" : "pi",
    customInstructions: stringValue(value?.customInstructions),
    gitAuthor: stringValue(value?.gitAuthor),
    includeUncommitted:
      typeof value?.includeUncommitted === "boolean"
        ? value.includeUncommitted
        : DEFAULT_DAILY_REPORT_AI_SETTINGS.includeUncommitted,
  };
}

export function normalizeDailyReportAPIURL(value: string): string {
  const raw = value.trim();
  if (!raw) return "";

  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    throw new Error("日报 API 地址格式不正确");
  }
  if (parsed.username || parsed.password) {
    throw new Error("日报 API 地址不能包含用户名或密码");
  }
  const withoutFragment = raw.split("#", 1)[0];
  if (withoutFragment.includes("?")) {
    throw new Error("日报 API 地址不能包含查询参数");
  }
  const hostname = parsed.hostname
    .replace(/^\[/, "")
    .replace(/\]$/, "")
    .toLowerCase();
  const loopback =
    hostname === "localhost" ||
    hostname === "::1" ||
    /^127(?:\.\d{1,3}){3}$/.test(hostname);
  if (parsed.protocol === "http:" && !loopback) {
    throw new Error(
      "日报 Token 只能发送到 HTTPS；HTTP 仅允许 localhost 或 loopback 地址",
    );
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    throw new Error("日报 API 地址必须使用 HTTPS");
  }

  parsed.hash = "";
  const pathname =
    parsed.pathname === "/" ? "" : parsed.pathname.replace(/\/+$/, "");
  return `${parsed.origin}${pathname}`;
}

function normalizeProjectRows(
  rows: DailyReportProjectRow[] | undefined,
): DailyReportProjectRow[] {
  if (!Array.isArray(rows)) return [createDailyReportProjectRow()];
  return rows.map((row) => ({
    id: stringValue(row?.id) || createID("report-project"),
    projectNo: stringValue(row?.projectNo),
    projectName: stringValue(row?.projectName),
    description: stringValue(row?.description),
  }));
}

function normalizeReviewRows(
  rows: DailyReportReviewRow[] | undefined,
): DailyReportReviewRow[] {
  if (!Array.isArray(rows)) return [createDailyReportReviewRow()];
  return rows.map((row) => ({
    id: stringValue(row?.id) || createID("report-review"),
    topic: stringValue(row?.topic),
    detail: stringValue(row?.detail),
    outcome: stringValue(row?.outcome),
  }));
}

function normalizeNextActionRows(
  rows: DailyReportNextActionRow[] | undefined,
): DailyReportNextActionRow[] {
  if (!Array.isArray(rows)) return [createDailyReportNextActionRow()];
  return rows.map((row) => ({
    id: stringValue(row?.id) || createID("report-next"),
    projectNo: stringValue(row?.projectNo),
    goal: stringValue(row?.goal),
    deadline: stringValue(row?.deadline),
  }));
}

export function normalizeDailyReportDraft(
  value?: Partial<DailyReportDraft>,
): DailyReportDraft {
  return {
    date: stringValue(value?.date) || localDateString(),
    results: normalizeProjectRows(value?.results),
    blockers: normalizeProjectRows(value?.blockers),
    reviews: normalizeReviewRows(value?.reviews),
    nextActions: normalizeNextActionRows(value?.nextActions),
  };
}

const PENDING_CONFIRMATION = "待确认";
const GENERATED_STATUSES: readonly DailyReportGeneratedStatus[] = [
  "已完成",
  "进行中",
  "阻塞",
  "已延期",
  PENDING_CONFIRMATION,
];
const INTERNAL_EVIDENCE_FIELD_PATTERN =
  /(?:manualDescription|workflowTasks|gitContext|customInstructions|taskId|updatedAt|developmentState|developmentResult|reviewNote)|(?:^|[^A-Za-z0-9_])["']?(?:title|projectName|status|summary|taskId|updatedAt|developmentState|developmentResult|reviewNote)["']?\s*[:=：]/i;

function generatedText(value: unknown, fallback = PENDING_CONFIRMATION): string {
  if (typeof value !== "string") return fallback;
  const normalized = value
    .replace(/[\r\n]+/g, " ")
    .replace(/[ \t]{2,}/g, " ")
    .replaceAll("[", "［")
    .replaceAll("]", "］")
    .replaceAll("|", "／")
    .replace(/[;；]+/g, "，")
    .trim();
  return normalized || fallback;
}

function generatedStatus(value: unknown): DailyReportGeneratedStatus {
  const normalized = generatedText(value);
  return GENERATED_STATUSES.includes(normalized as DailyReportGeneratedStatus)
    ? (normalized as DailyReportGeneratedStatus)
    : PENDING_CONFIRMATION;
}

function generatedProgress(value: unknown): string {
  const normalized =
    typeof value === "number"
      ? String(value)
      : typeof value === "string"
        ? value.trim()
        : "";
  const match = /^(\d+(?:\.\d+)?)\s*%?$/.exec(normalized);
  if (!match) return PENDING_CONFIRMATION;
  const numeric = Number(match[1]);
  if (!Number.isFinite(numeric) || numeric < 0 || numeric > 100) {
    return PENDING_CONFIRMATION;
  }
  return `${numeric}%`;
}

function generatedEvidence(value: unknown): string[] {
  if (!Array.isArray(value)) return [PENDING_CONFIRMATION];
  const evidence = [
    ...new Set(
      value
        .filter((item): item is string => typeof item === "string")
        .map((item) => generatedText(item, ""))
        .filter(Boolean)
        .filter((item) => !INTERNAL_EVIDENCE_FIELD_PATTERN.test(item)),
    ),
  ].filter((item) => item !== PENDING_CONFIRMATION);
  return evidence.length > 0 ? evidence : [PENDING_CONFIRMATION];
}

function generatedBlockerLevel(
  value: unknown,
): DailyReportGeneratedBlockerLevel {
  const normalized = generatedText(value).toUpperCase();
  if (normalized === "P0" || normalized === "P1" || normalized === "P2") {
    return normalized;
  }
  return normalized === "一般" ? "一般" : PENDING_CONFIRMATION;
}

function generatedEscalation(
  value: unknown,
): DailyReportGeneratedEscalation {
  const normalized = generatedText(value).toUpperCase();
  return normalized === "Y" || normalized === "N"
    ? normalized
    : PENDING_CONFIRMATION;
}

export function dailyReportDraftFromGenerationResult(
  result: DailyReportGenerationResult,
): DailyReportDraft {
  return {
    date: stringValue(result.reportDate) || localDateString(),
    results: (Array.isArray(result.results) ? result.results : []).map((row) => ({
      id: createID("report-project"),
      projectNo: generatedText(row?.projectNo, "未编号"),
      projectName: generatedText(row?.projectName, "未命名"),
      description: [
        generatedText(row?.task),
        generatedStatus(row?.status),
        generatedProgress(row?.progress),
        generatedEvidence(row?.evidence).join(" · "),
      ].join("；"),
    })),
    blockers: (Array.isArray(result.blockers) ? result.blockers : []).map(
      (row) => ({
        id: createID("report-project"),
        projectNo: generatedText(row?.projectNo, "未编号"),
        projectName: generatedText(row?.projectName, "未命名"),
        description: [
          generatedText(row?.issue),
          generatedBlockerLevel(row?.level),
          generatedText(row?.impact),
          generatedText(row?.helpTarget),
          generatedText(row?.waitDuration),
          generatedEscalation(row?.escalate),
        ].join(" | "),
      }),
    ),
    reviews: (Array.isArray(result.reviews) ? result.reviews : []).map((row) => ({
      id: createID("report-review"),
      topic: `${row?.teamRisk === true ? "⚠ 风险 · " : ""}${generatedText(row?.scene)}`,
      detail: `根因：${generatedText(row?.cause)}；处置：${generatedText(row?.action)}`,
      outcome: `验证结果：${generatedText(row?.validation)}`,
    })),
    nextActions: (
      Array.isArray(result.nextActions) ? result.nextActions : []
    )
      .slice(0, 3)
      .map((row) => {
        const goal = generatedText(row?.goal);
        const inferredMarker =
          row?.inferred === true && !goal.includes(PENDING_CONFIRMATION)
            ? "（待确认）"
            : "";
        return {
          id: createID("report-next"),
          projectNo: generatedText(row?.projectNo, "未编号"),
          goal: `${generatedText(row?.projectName, "未命名")} ${goal}${inferredMarker}`,
          deadline: generatedText(row?.deadline),
        };
      }),
  };
}

export function dailyReportGenerationToDraft(
  reportDate: string,
  result: DailyReportGenerationResult,
): DailyReportDraft {
  return dailyReportDraftFromGenerationResult({
    ...result,
    reportDate: reportDate || result.reportDate,
  });
}

export function isDailyReportDraftEmpty(draft: DailyReportDraft): boolean {
  return ![
    ...draft.results.flatMap((row) => [
      row.projectNo,
      row.projectName,
      row.description,
    ]),
    ...draft.blockers.flatMap((row) => [
      row.projectNo,
      row.projectName,
      row.description,
    ]),
    ...draft.reviews.flatMap((row) => [row.topic, row.detail, row.outcome]),
    ...draft.nextActions.flatMap((row) => [
      row.projectNo,
      row.goal,
      row.deadline,
    ]),
  ].some((value) => value.trim());
}

export function dailyReportPlaceholders(
  role: DailyReportRole,
): DailyReportPlaceholders {
  return ROLE_PLACEHOLDERS[role] ?? DEFAULT_PLACEHOLDERS;
}

function nonEmpty(values: string[]): boolean {
  return values.some((value) => value.trim());
}

function lines(values: string[]): string {
  return values.length > 0 ? values.join("\n") : "- 无";
}

export function buildDailyReportMarkdown(
  settings: DailyReportSettings,
  draft: DailyReportDraft,
): string {
  const organization = settings.organization.trim() || "未填写组织";
  const submitter = settings.submitter.trim();
  const employeeId = settings.employeeId.trim();
  const date = draft.date || localDateString();
  const results = draft.results
    .filter((row) =>
      nonEmpty([row.projectNo, row.projectName, row.description]),
    )
    .map(
      (row) =>
        `- [${row.projectNo.trim() || "未编号"} | ${row.projectName.trim() || "未命名"}] ${row.description.trim()}`,
    );
  const blockers = draft.blockers
    .filter((row) =>
      nonEmpty([row.projectNo, row.projectName, row.description]),
    )
    .map(
      (row) =>
        `- [${row.projectNo.trim() || "未编号"} | ${row.projectName.trim() || "未命名"}] ${row.description.trim()}`,
    );
  const reviews = draft.reviews
    .filter((row) => nonEmpty([row.topic, row.detail, row.outcome]))
    .map(
      (row) =>
        `- ${[row.topic, row.detail, row.outcome]
          .map((value) => value.trim())
          .filter(Boolean)
          .join(" | ")}`,
    );
  const nextActions = draft.nextActions
    .filter((row) => nonEmpty([row.projectNo, row.goal, row.deadline]))
    .slice(0, 3)
    .map(
      (row, index) =>
        `- TOP${index + 1}: [${row.projectNo.trim() || "未编号"} | ${row.goal.trim() || "未命名"}]${
          row.deadline.trim() ? `（截止：${row.deadline.trim()}）` : ""
        }`,
    );

  return `# 日报 · ${submitter || "提交人"}·${date}\n\n[REPORT-ORG:${organization}] [LEVEL:${settings.level}] [TYPE:日报] [DATE:${date}]\n> 提交人: ${submitter}\n> 工号: ${employeeId}\n> 岗位: ${DAILY_REPORT_ROLE_LABELS[settings.role]}\n> 层级: ${settings.level}\n> 日期: ${date}\n\n## 【今日结果】\n${lines(results)}\n\n## 【死锁阻碍】\n${lines(blockers)}\n\n## 【专项复盘】\n${lines(reviews)}\n\n## 【明日动作】\n${lines(nextActions)}`;
}

export function dailyReportFilename(
  settings: DailyReportSettings,
  draft: DailyReportDraft,
  extension: "md" | "html",
): string {
  return `工作报告_${settings.level}_日报_${draft.date || localDateString()}.${extension}`;
}

export function buildDailyReportHTML(markdown: string): string {
  const escaped = markdown
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>技术工作日报</title><style>body{font-family:PingFang SC,Microsoft YaHei,sans-serif;background:#0d1520;color:#f0f8ff;max-width:760px;margin:30px auto;padding:0 18px;white-space:pre-wrap;line-height:1.75}</style></head><body>${escaped}</body></html>`;
}
