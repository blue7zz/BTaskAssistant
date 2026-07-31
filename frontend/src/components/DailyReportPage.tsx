import {
  CalendarDays,
  ClipboardCopy,
  CloudUpload,
  Download,
  FileCode2,
  FileText,
  Plus,
  RotateCcw,
  Settings2,
  Sparkles,
  Trash2,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  DAILY_REPORT_LEVEL_LABELS,
  DAILY_REPORT_ROLE_HINTS,
  DAILY_REPORT_ROLE_LABELS,
  buildDailyReportHTML,
  buildDailyReportMarkdown,
  createEmptyDailyReportDraft,
  createDailyReportNextActionRow,
  createDailyReportProjectRow,
  createDailyReportReviewRow,
  dailyReportFilename,
  dailyReportPlaceholders,
  type DailyReportNextActionRow,
  type DailyReportProjectRow,
  type DailyReportReviewRow,
} from "../domain/report";
import { copyText } from "../lib/clipboard";
import {
  dailyReportCloudAvailable,
  hasDailyReportToken,
  submitDailyReport,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";
import { DailyReportAIDialog } from "./DailyReportAIDialog";

interface DailyReportPageProps {
  onOpenSettings(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

type UploadStatus = { kind: "success" | "error"; message: string };

function replaceRow<T extends { id: string }>(
  rows: T[],
  rowID: string,
  patch: Partial<T>,
): T[] {
  return rows.map((row) => (row.id === rowID ? { ...row, ...patch } : row));
}

function removeRow<T extends { id: string }>(rows: T[], rowID: string): T[] {
  return rows.filter((row) => row.id !== rowID);
}

function downloadText(content: string, mimeType: string, filename: string) {
  const anchor = document.createElement("a");
  anchor.href = `data:${mimeType};charset=utf-8,${encodeURIComponent(content)}`;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
}

export function DailyReportPage({
  onOpenSettings,
  onSuccess,
  onError,
}: DailyReportPageProps) {
  const settings = useWorkspaceStore((state) => state.dailyReportSettings);
  const reportDate = useWorkspaceStore((state) => state.dailyReportDate);
  const storedDraft = useWorkspaceStore(
    (state) => state.dailyReportDrafts[state.dailyReportDate],
  );
  const selectReportDate = useWorkspaceStore(
    (state) => state.selectDailyReportDate,
  );
  const draft = storedDraft ?? createEmptyDailyReportDraft(reportDate);
  const updateDraft = useWorkspaceStore(
    (state) => state.updateDailyReportDraft,
  );
  const resetDraft = useWorkspaceStore((state) => state.resetDailyReportDraft);
  const [uploading, setUploading] = useState(false);
  const [uploadStatus, setUploadStatus] = useState<UploadStatus>();
  const [tokenStored, setTokenStored] = useState(false);
  const [checkingToken, setCheckingToken] = useState(false);
  const [aiDialogOpen, setAIDialogOpen] = useState(false);
  const markdown = useMemo(
    () => buildDailyReportMarkdown(settings, draft),
    [draft, settings],
  );
  const latestMarkdown = useRef(markdown);
  latestMarkdown.current = markdown;
  const placeholders = dailyReportPlaceholders(settings.role);
  const cloudAvailable = dailyReportCloudAvailable();
  const submissionReady = Boolean(
    cloudAvailable &&
      settings.employeeId.trim() &&
      settings.apiUrl.trim() &&
      tokenStored,
  );

  useEffect(() => {
    setUploadStatus(undefined);
  }, [markdown]);

  useEffect(() => {
    let active = true;
    if (
      !cloudAvailable ||
      !settings.apiUrl.trim() ||
      !settings.employeeId.trim()
    ) {
      setTokenStored(false);
      setCheckingToken(false);
      return;
    }
    setCheckingToken(true);
    hasDailyReportToken(settings.apiUrl, settings.employeeId)
      .then((stored) => {
        if (active) setTokenStored(stored);
      })
      .catch(() => {
        if (active) setTokenStored(false);
      })
      .finally(() => {
        if (active) setCheckingToken(false);
      });
    return () => {
      active = false;
    };
  }, [cloudAvailable, settings.apiUrl, settings.employeeId]);

  const updateProjectRow = (
    section: "results" | "blockers",
    rowID: string,
    patch: Partial<DailyReportProjectRow>,
  ) => {
    updateDraft({ [section]: replaceRow(draft[section], rowID, patch) });
  };

  const deleteProjectRow = (
    section: "results" | "blockers",
    rowID: string,
  ) => {
    updateDraft({ [section]: removeRow(draft[section], rowID) });
  };

  const handleCopy = async () => {
    try {
      await copyText(markdown);
      onSuccess("日报 Markdown 已复制，可直接粘贴到 TG");
    } catch (error) {
      onError(error);
    }
  };

  const handleUpload = async () => {
    if (!settings.apiUrl.trim() || !settings.employeeId.trim()) {
      onError(new Error("请先在日报设置中补全工号和 API 地址"));
      return;
    }
    if (!tokenStored) {
      onError(new Error("请先在日报设置中保存当前工号的 Token"));
      return;
    }
    const submittedMarkdown = markdown;
    setUploading(true);
    setUploadStatus(undefined);
    try {
      const result = await submitDailyReport(
        settings,
        draft.date,
        markdown,
      );
      const action = result.action === "updated" ? "已覆盖旧版本" : "首次提交";
      const changed = latestMarkdown.current !== submittedMarkdown;
      const message = `上传成功 #${result.id} · ${action}${
        changed ? "；当前内容已修改，请重新上传" : ""
      }`;
      setUploadStatus({ kind: "success", message });
      onSuccess(message);
    } catch (error) {
      setUploadStatus({
        kind: "error",
        message: error instanceof Error ? error.message : "上传失败",
      });
      onError(error);
    } finally {
      setUploading(false);
    }
  };

  return (
    <section className="daily-report-page">
      <header className="daily-report-toolbar">
        <div>
          <span className="eyebrow">工作报告</span>
          <h1>日报填报</h1>
          <p>按今日结果、阻碍、复盘和明日动作整理，实时生成 Markdown。</p>
        </div>
        <label className="daily-report-date">
          <CalendarDays size={16} />
          <span>报告日期</span>
          <input
            type="date"
            value={reportDate}
            onChange={(event) => selectReportDate(event.target.value)}
            aria-label="日报日期"
          />
        </label>
      </header>

      <div className="daily-report-scroll">
        <div className="daily-report-content">
          <section
            className={`daily-report-profile ${settings.employeeId.trim() ? "" : "incomplete"}`}
          >
            <div className="daily-report-profile-icon">
              <FileText size={20} />
            </div>
            <div className="daily-report-profile-copy">
              <strong>
                {settings.submitter.trim() || "尚未配置提交人"}
                {settings.employeeId.trim() && ` · ${settings.employeeId.trim()}`}
              </strong>
              <span>
                {settings.organization.trim() || "尚未配置组织"} · {DAILY_REPORT_LEVEL_LABELS[settings.level]} · {DAILY_REPORT_ROLE_LABELS[settings.role]}
              </span>
              <small>
                {DAILY_REPORT_ROLE_HINTS[settings.role]} · {checkingToken
                  ? "正在检查云端配置"
                  : submissionReady
                    ? "云端提交已配置"
                    : "云端提交未配置"}
              </small>
            </div>
            <button
              type="button"
              className="button secondary compact"
              onClick={onOpenSettings}
            >
              <Settings2 size={15} />
              日报设置
            </button>
          </section>

          <section className="daily-report-card section-results">
            <header className="daily-report-card-header">
              <div>
                <span className="report-section-dot" />
                <strong>【今日结果】</strong>
              </div>
              <small>当天完成量与交付证据（版本、PR、链接或缺陷单）</small>
            </header>
            <div className="daily-report-rows">
              {draft.results.map((row) => (
                <div className="daily-report-row project-row" key={row.id}>
                  <input
                    value={row.projectNo}
                    placeholder="项目编号"
                    aria-label="今日结果项目编号"
                    onChange={(event) =>
                      updateProjectRow("results", row.id, {
                        projectNo: event.target.value,
                      })
                    }
                  />
                  <input
                    value={row.projectName}
                    placeholder="项目名"
                    aria-label="今日结果项目名"
                    onChange={(event) =>
                      updateProjectRow("results", row.id, {
                        projectName: event.target.value,
                      })
                    }
                  />
                  <textarea
                    value={row.description}
                    placeholder={placeholders.result}
                    aria-label="今日结果描述"
                    onChange={(event) =>
                      updateProjectRow("results", row.id, {
                        description: event.target.value,
                      })
                    }
                  />
                  <button
                    type="button"
                    className="report-delete-button"
                    aria-label="删除今日结果"
                    onClick={() => deleteProjectRow("results", row.id)}
                  >
                    <Trash2 size={15} />
                    删除
                  </button>
                </div>
              ))}
            </div>
            <button
              type="button"
              className="report-add-button"
              onClick={() =>
                updateDraft({
                  results: [...draft.results, createDailyReportProjectRow()],
                })
              }
            >
              <Plus size={15} />
              添加任务
            </button>
          </section>

          <section className="daily-report-card section-blockers">
            <header className="daily-report-card-header">
              <div>
                <span className="report-section-dot" />
                <strong>【死锁阻碍】</strong>
              </div>
              <small>写清影响范围、求助对象、等待时长与是否升级</small>
            </header>
            <div className="daily-report-rows">
              {draft.blockers.map((row) => (
                <div className="daily-report-row project-row" key={row.id}>
                  <input
                    value={row.projectNo}
                    placeholder="项目编号"
                    aria-label="死锁阻碍项目编号"
                    onChange={(event) =>
                      updateProjectRow("blockers", row.id, {
                        projectNo: event.target.value,
                      })
                    }
                  />
                  <input
                    value={row.projectName}
                    placeholder="项目名"
                    aria-label="死锁阻碍项目名"
                    onChange={(event) =>
                      updateProjectRow("blockers", row.id, {
                        projectName: event.target.value,
                      })
                    }
                  />
                  <textarea
                    value={row.description}
                    placeholder={placeholders.blocker}
                    aria-label="死锁阻碍描述"
                    onChange={(event) =>
                      updateProjectRow("blockers", row.id, {
                        description: event.target.value,
                      })
                    }
                  />
                  <button
                    type="button"
                    className="report-delete-button"
                    aria-label="删除死锁阻碍"
                    onClick={() => deleteProjectRow("blockers", row.id)}
                  >
                    <Trash2 size={15} />
                    删除
                  </button>
                </div>
              ))}
            </div>
            <button
              type="button"
              className="report-add-button"
              onClick={() =>
                updateDraft({
                  blockers: [
                    ...draft.blockers,
                    createDailyReportProjectRow(),
                  ],
                })
              }
            >
              <Plus size={15} />
              添加异常
            </button>
          </section>

          <section className="daily-report-card section-reviews">
            <header className="daily-report-card-header">
              <div>
                <span className="report-section-dot" />
                <strong>【专项复盘】</strong>
              </div>
              <small>记录根因、处置动作和当日验证结果</small>
            </header>
            <div className="daily-report-rows">
              {draft.reviews.map((row) => (
                <div className="daily-report-row review-row" key={row.id}>
                  <input
                    value={row.topic}
                    placeholder={placeholders.reviewTopic}
                    aria-label="专项复盘主题"
                    onChange={(event) =>
                      updateDraft({
                        reviews: replaceRow<DailyReportReviewRow>(
                          draft.reviews,
                          row.id,
                          { topic: event.target.value },
                        ),
                      })
                    }
                  />
                  <input
                    value={row.detail}
                    placeholder={placeholders.reviewDetail}
                    aria-label="专项复盘内容"
                    onChange={(event) =>
                      updateDraft({
                        reviews: replaceRow<DailyReportReviewRow>(
                          draft.reviews,
                          row.id,
                          { detail: event.target.value },
                        ),
                      })
                    }
                  />
                  <input
                    value={row.outcome}
                    placeholder={placeholders.reviewOutcome}
                    aria-label="专项复盘收益"
                    onChange={(event) =>
                      updateDraft({
                        reviews: replaceRow<DailyReportReviewRow>(
                          draft.reviews,
                          row.id,
                          { outcome: event.target.value },
                        ),
                      })
                    }
                  />
                  <button
                    type="button"
                    className="report-delete-button"
                    aria-label="删除专项复盘"
                    onClick={() =>
                      updateDraft({
                        reviews: removeRow(draft.reviews, row.id),
                      })
                    }
                  >
                    <Trash2 size={15} />
                    删除
                  </button>
                </div>
              ))}
            </div>
            <button
              type="button"
              className="report-add-button"
              onClick={() =>
                updateDraft({
                  reviews: [...draft.reviews, createDailyReportReviewRow()],
                })
              }
            >
              <Plus size={15} />
              添加复盘记录
            </button>
          </section>

          <section className="daily-report-card section-next">
            <header className="daily-report-card-header">
              <div>
                <span className="report-section-dot" />
                <strong>【明日动作】</strong>
              </div>
              <small>按录入顺序导出 TOP1–3，目标需量化并带截止时间</small>
            </header>
            <div className="daily-report-rows">
              {draft.nextActions.map((row) => (
                <div className="daily-report-row next-row" key={row.id}>
                  <input
                    value={row.projectNo}
                    placeholder="项目编号"
                    aria-label="明日动作项目编号"
                    onChange={(event) =>
                      updateDraft({
                        nextActions: replaceRow<DailyReportNextActionRow>(
                          draft.nextActions,
                          row.id,
                          { projectNo: event.target.value },
                        ),
                      })
                    }
                  />
                  <input
                    value={row.goal}
                    placeholder={placeholders.nextGoal}
                    aria-label="明日动作目标"
                    onChange={(event) =>
                      updateDraft({
                        nextActions: replaceRow<DailyReportNextActionRow>(
                          draft.nextActions,
                          row.id,
                          { goal: event.target.value },
                        ),
                      })
                    }
                  />
                  <input
                    value={row.deadline}
                    placeholder={placeholders.nextDeadline}
                    aria-label="明日动作截止时间"
                    onChange={(event) =>
                      updateDraft({
                        nextActions: replaceRow<DailyReportNextActionRow>(
                          draft.nextActions,
                          row.id,
                          { deadline: event.target.value },
                        ),
                      })
                    }
                  />
                  <button
                    type="button"
                    className="report-delete-button"
                    aria-label="删除明日动作"
                    onClick={() =>
                      updateDraft({
                        nextActions: removeRow(draft.nextActions, row.id),
                      })
                    }
                  >
                    <Trash2 size={15} />
                    删除
                  </button>
                </div>
              ))}
            </div>
            <button
              type="button"
              className="report-add-button"
              onClick={() =>
                updateDraft({
                  nextActions: [
                    ...draft.nextActions,
                    createDailyReportNextActionRow(),
                  ],
                })
              }
            >
              <Plus size={15} />
              添加计划
            </button>
          </section>

          <section className="daily-report-preview">
            <header>
              <div>
                <FileCode2 size={17} />
                <strong>导出预览（Markdown）</strong>
              </div>
              <span>内容随表单实时更新</span>
            </header>
            <pre>{markdown}</pre>
            {uploadStatus && (
              <div className={`report-upload-status ${uploadStatus.kind}`}>
                {uploadStatus.message}
              </div>
            )}
          </section>
        </div>
      </div>

      <footer className="daily-report-actions">
        <button
          type="button"
          className="button primary report-ai-button"
          onClick={() => setAIDialogOpen(true)}
        >
          <Sparkles size={15} />
          AI 生成日报
        </button>
        <button
          type="button"
          className="button secondary"
          onClick={() => {
            if (!window.confirm("确定重置当前日期的日报正文吗？此操作无法撤销。")) {
              return;
            }
            resetDraft();
            setUploadStatus(undefined);
            onSuccess("日报正文已重置，个人与接口设置已保留");
          }}
        >
          <RotateCcw size={15} />
          重置正文
        </button>
        <button type="button" className="button secondary" onClick={handleCopy}>
          <ClipboardCopy size={15} />
          复制到 TG
        </button>
        <button
          type="button"
          className="button primary"
          disabled={uploading || checkingToken || !submissionReady}
          title={
            submissionReady
              ? "上传当前日报"
              : "请先在日报设置中配置工号、API 地址和 Token"
          }
          onClick={handleUpload}
        >
          <CloudUpload className={uploading ? "spin" : ""} size={15} />
          {uploading ? "上传中" : "上传云端"}
        </button>
        <button
          type="button"
          className="button secondary"
          onClick={() =>
            downloadText(
              markdown,
              "text/markdown",
              dailyReportFilename(settings, draft, "md"),
            )
          }
        >
          <Download size={15} />
          下载 MD
        </button>
        <button
          type="button"
          className="button secondary"
          onClick={() =>
            downloadText(
              buildDailyReportHTML(markdown),
              "text/html",
              dailyReportFilename(settings, draft, "html"),
            )
          }
        >
          <Download size={15} />
          下载 HTML
        </button>
      </footer>

      <DailyReportAIDialog
        open={aiDialogOpen}
        onClose={() => setAIDialogOpen(false)}
        onSuccess={onSuccess}
        onError={onError}
      />
    </section>
  );
}
