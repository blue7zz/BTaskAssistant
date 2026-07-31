import { useEffect, useState } from "react";
import {
  BrainCircuit,
  Building2,
  Clock3,
  FolderGit2,
  GitBranch,
  Info,
  IdCard,
  KeyRound,
  LoaderCircle,
  Save,
  ShieldCheck,
  Trash2,
  UserRound,
} from "lucide-react";
import {
  normalizePISettings,
  type PISettings,
  type PIThinkingEffort,
} from "../domain/engine";
import {
  DAILY_REPORT_LEVEL_LABELS,
  DAILY_REPORT_LEVELS,
  DAILY_REPORT_ROLE_HINTS,
  DAILY_REPORT_ROLE_LABELS,
  DAILY_REPORT_ROLES,
  normalizeDailyReportAPIURL,
  normalizeDailyReportAISettings,
  normalizeDailyReportSettings,
  type DailyReportAISettings,
  type DailyReportLevel,
  type DailyReportRole,
  type DailyReportSettings as DailyReportSettingsValue,
} from "../domain/report";
import {
  dailyReportCloudAvailable,
  deleteDailyReportToken,
  getEngineStatuses,
  hasDailyReportToken,
  saveDailyReportToken,
  type EngineStatus,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";

type BusyAction = "save" | "delete-token";

interface DailyReportSettingsProps {
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

export function DailyReportSettings({
  onSuccess,
  onError,
}: DailyReportSettingsProps) {
  const settings = useWorkspaceStore((state) => state.dailyReportSettings);
  const updateSettings = useWorkspaceStore(
    (state) => state.updateDailyReportSettings,
  );
  const aiSettings = useWorkspaceStore(
    (state) => state.dailyReportAISettings,
  );
  const updateAISettings = useWorkspaceStore(
    (state) => state.updateDailyReportAISettings,
  );
  const piSettings = useWorkspaceStore((state) => state.piSettings);
  const updatePISettings = useWorkspaceStore((state) => state.updatePISettings);
  const projectHistory = useWorkspaceStore(
    (state) => state.dailyReportProjectHistory,
  );
  const removeProject = useWorkspaceStore(
    (state) => state.removeDailyReportProject,
  );
  const [draft, setDraft] = useState<DailyReportSettingsValue>(() => ({
    ...settings,
  }));
  const [aiDraft, setAIDraft] = useState<DailyReportAISettings>(() => ({
    ...aiSettings,
  }));
  const [piDraft, setPIDraft] = useState<PISettings>(() => ({ ...piSettings }));
  const [engines, setEngines] = useState<EngineStatus[]>([]);
  const [token, setToken] = useState("");
  const [tokenStored, setTokenStored] = useState(false);
  const [checkingToken, setCheckingToken] = useState(false);
  const [busy, setBusy] = useState<BusyAction>();
  const cloudAvailable = dailyReportCloudAvailable();

  useEffect(() => setDraft({ ...settings }), [settings]);
  useEffect(() => setAIDraft({ ...aiSettings }), [aiSettings]);
  useEffect(() => setPIDraft({ ...piSettings }), [piSettings]);

  useEffect(() => {
    let active = true;
    getEngineStatuses()
      .then((statuses) => {
        if (active) setEngines(statuses);
      })
      .catch(() => {
        if (active) setEngines([]);
      });
    return () => {
      active = false;
    };
  }, []);

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

  const tokenIdentityMatches =
    draft.apiUrl.trim() === settings.apiUrl.trim() &&
    draft.employeeId.trim() === settings.employeeId.trim();
  const currentTokenStored = tokenIdentityMatches && tokenStored;

  const updateLevel = (level: DailyReportLevel) => {
    setDraft((current) => ({
      ...current,
      level,
      role:
        level === "L3"
          ? "TL"
          : current.level === "L3" && current.role === "TL"
            ? "FE"
            : current.role,
    }));
  };

  const save = async () => {
    setBusy("save");
    try {
      const next = normalizeDailyReportSettings({
        ...draft,
        organization: draft.organization.trim(),
        submitter: draft.submitter.trim(),
        employeeId: draft.employeeId.trim(),
        apiUrl: normalizeDailyReportAPIURL(draft.apiUrl),
      });
      const nextAI = normalizeDailyReportAISettings({
        ...aiDraft,
        customInstructions: aiDraft.customInstructions.trim(),
        gitAuthor: aiDraft.gitAuthor.trim(),
      });
      const nextPI = normalizePISettings(piDraft);
      if (token.trim()) {
        await saveDailyReportToken(next.apiUrl, next.employeeId, token);
      }
      updateSettings(next);
      updateAISettings(nextAI);
      updatePISettings(nextPI);
      setDraft(next);
      setAIDraft(nextAI);
      setPIDraft(nextPI);
      const stored = token.trim()
        ? true
        : await hasDailyReportToken(next.apiUrl, next.employeeId);
      setTokenStored(stored);
      setToken("");
      onSuccess(token.trim() ? "日报设置与 Token 已保存" : "日报设置已保存");
    } catch (error) {
      onError(error);
    } finally {
      setBusy(undefined);
    }
  };

  const removeToken = async () => {
    setBusy("delete-token");
    try {
      await deleteDailyReportToken(settings.apiUrl, settings.employeeId);
      setTokenStored(false);
      setToken("");
      onSuccess("日报 Token 已从系统凭据库删除");
    } catch (error) {
      onError(error);
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <section className="daily-report-settings-page">
      <section className="collector-settings">
        <div className="settings-heading">
          <div className="settings-icon">
            <UserRound size={18} />
          </div>
          <div>
            <strong>填报人信息</strong>
            <span>保存后会自动用于日报标题和提交内容</span>
          </div>
        </div>

        <div className="collector-settings-grid">
          <label className="field">
            <span>组织</span>
            <div className="credential-input">
              <Building2 size={16} />
              <input
                aria-label="日报组织"
                value={draft.organization}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    organization: event.target.value,
                  }))
                }
                placeholder="例如：技术中心"
              />
            </div>
          </label>

          <label className="field">
            <span>提交人</span>
            <input
              aria-label="日报提交人"
              value={draft.submitter}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  submitter: event.target.value,
                }))
              }
              placeholder="例如：张三"
            />
          </label>

          <label className="field">
            <span>工号</span>
            <div className="credential-input">
              <IdCard size={16} />
              <input
                aria-label="日报工号"
                value={draft.employeeId}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    employeeId: event.target.value,
                  }))
                }
                placeholder="例如：DN1111"
              />
            </div>
          </label>

          <label className="field">
            <span>层级</span>
            <select
              aria-label="日报层级"
              value={draft.level}
              onChange={(event) =>
                updateLevel(event.target.value as DailyReportLevel)
              }
            >
              {DAILY_REPORT_LEVELS.map((level) => (
                <option key={level} value={level}>
                  {DAILY_REPORT_LEVEL_LABELS[level]}
                </option>
              ))}
            </select>
          </label>

          <label className="field">
            <span>
              岗位
              {draft.level === "L3" && <small>L3 固定为技术负责人</small>}
            </span>
            <select
              aria-label="日报岗位"
              value={draft.role}
              disabled={draft.level === "L3"}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  role: event.target.value as DailyReportRole,
                }))
              }
            >
              {DAILY_REPORT_ROLES.map((role) => (
                <option key={role} value={role}>
                  {DAILY_REPORT_ROLE_LABELS[role]}
                </option>
              ))}
            </select>
            <small className="pi-field-help">
              {DAILY_REPORT_ROLE_HINTS[draft.role]}
            </small>
          </label>
        </div>
      </section>

      <section className="collector-settings">
        <div className="settings-heading">
          <div className="settings-icon">
            <BrainCircuit size={18} />
          </div>
          <div>
            <strong>AI 生成</strong>
            <span>控制日报生成引擎、Git 采集范围和附加写作偏好</span>
          </div>
        </div>

        <div className="collector-settings-grid daily-report-ai-settings-grid">
          <label className="field">
            <span>
              AI 引擎
              <small>
                {engines.find((engine) => engine.id === aiDraft.engine)
                  ?.configured
                  ? "已就绪"
                  : "未检测到"}
              </small>
            </span>
            <select
              aria-label="日报 AI 引擎"
              value={aiDraft.engine}
              onChange={(event) =>
                setAIDraft((current) => ({
                  ...current,
                  engine: event.target.value === "codex" ? "codex" : "pi",
                }))
              }
            >
              <option value="pi">PI / oh-my-pi</option>
              <option value="codex">Codex</option>
            </select>
          </label>

          <label className="field">
            <span>
              模型标识
              <small>可选，与 PI 设置共享</small>
            </span>
            <input
              aria-label="日报 AI 模型"
              value={piDraft.model}
              onChange={(event) =>
                setPIDraft((current) => ({
                  ...current,
                  model: event.target.value,
                }))
              }
              placeholder="留空使用所选 CLI 的默认模型"
            />
          </label>

          <label className="field">
            <span>思考强度</span>
            <select
              aria-label="日报 AI 思考强度"
              value={piDraft.thinkingEffort}
              onChange={(event) =>
                setPIDraft((current) => ({
                  ...current,
                  thinkingEffort: event.target.value as PIThinkingEffort,
                }))
              }
            >
              <option value="low">低</option>
              <option value="medium">中</option>
              <option value="high">高</option>
              <option value="xhigh">极高</option>
            </select>
          </label>

          <label className="field">
            <span>
              执行时限
              <small>超时自动停止</small>
            </span>
            <div className="credential-input">
              <Clock3 size={16} />
              <select
                aria-label="日报 AI 执行时限"
                value={piDraft.timeoutMinutes}
                onChange={(event) =>
                  setPIDraft((current) => ({
                    ...current,
                    timeoutMinutes: Number(event.target.value),
                  }))
                }
              >
                <option value={1}>1 分钟</option>
                <option value={3}>3 分钟</option>
                <option value={5}>5 分钟</option>
                <option value={10}>10 分钟</option>
              </select>
            </div>
          </label>

          <label className="field">
            <span>
              Git 作者
              <small>留空使用仓库当前用户</small>
            </span>
            <div className="credential-input">
              <GitBranch size={16} />
              <input
                aria-label="日报 Git 作者"
                value={aiDraft.gitAuthor}
                onChange={(event) =>
                  setAIDraft((current) => ({
                    ...current,
                    gitAuthor: event.target.value,
                  }))
                }
                placeholder="例如：blue"
              />
            </div>
          </label>

          <label className="setting-switch-row">
            <div>
              <strong>包含未提交变更</strong>
              <span>仅采集状态和文件统计，不读取 diff 正文</span>
            </div>
            <input
              aria-label="日报包含未提交变更"
              type="checkbox"
              checked={aiDraft.includeUncommitted}
              onChange={(event) =>
                setAIDraft((current) => ({
                  ...current,
                  includeUncommitted: event.target.checked,
                }))
              }
            />
          </label>
        </div>

        <label className="field daily-report-ai-instructions">
          <span>
            附加写作指令
            <small>可选，不能覆盖固定公司格式与事实边界</small>
          </span>
          <textarea
            aria-label="日报 AI 附加写作指令"
            value={aiDraft.customInstructions}
            onChange={(event) =>
              setAIDraft((current) => ({
                ...current,
                customInstructions: event.target.value,
              }))
            }
            placeholder="例如：结果按业务意图合并，表述更精炼；不要写无法从材料证明的完成状态。"
          />
        </label>

        <div className="daily-report-fixed-rules" role="note">
          <Info size={16} />
          <span>
            固定规则已内置：锚点与四段顺序不可修改；Git 事实不得编造；空区块写“无”；明日动作最多 TOP1–3；生成结果必须预览并人工确认。
          </span>
        </div>
      </section>

      <section className="collector-settings">
        <div className="settings-heading">
          <div className="settings-icon">
            <FolderGit2 size={18} />
          </div>
          <div>
            <strong>项目历史</strong>
            <span>在 AI 生成弹窗中使用过的仓库会保存在本机</span>
          </div>
        </div>

        {projectHistory.length > 0 ? (
          <div className="daily-report-history-list">
            {projectHistory.map((project) => (
              <div className="daily-report-history-item" key={project.id}>
                <div>
                  <strong>
                    {project.projectNo ? `${project.projectNo} · ` : ""}
                    {project.projectName || "未命名项目"}
                  </strong>
                  <span>{project.path}</span>
                </div>
                <button
                  type="button"
                  className="button secondary compact"
                  aria-label={`删除日报项目历史 ${project.projectName || project.path}`}
                  onClick={() => removeProject(project.id)}
                >
                  <Trash2 size={14} />
                  删除
                </button>
              </div>
            ))}
          </div>
        ) : (
          <div className="daily-report-ai-note">
            尚无项目历史。首次在“AI 生成日报”中选择仓库并生成后，会出现在这里。
          </div>
        )}
      </section>

      <section className="collector-settings">
        <div className="settings-heading">
          <div className="settings-icon">
            <ShieldCheck size={18} />
          </div>
          <div>
            <strong>云端提交</strong>
            <span>Token 仅保存在系统凭据库，不会写入任务数据库</span>
          </div>
        </div>

        <div className="collector-settings-grid">
          <label className="field">
            <span>API 地址</span>
            <input
              aria-label="日报 API 地址"
              value={draft.apiUrl}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  apiUrl: event.target.value,
                }))
              }
              placeholder="https://ep.jsyyds.com/api/v1/report/submit"
            />
          </label>

          <label className="field">
            <span>
              Token
              <small>
                {checkingToken
                  ? "正在检查"
                  : currentTokenStored
                    ? "已保存"
                    : "尚未保存"}
              </small>
            </span>
            <div className="credential-input">
              <KeyRound size={16} />
              <input
                aria-label="日报 Token"
                type="password"
                autoComplete="off"
                disabled={!cloudAvailable}
                value={token}
                onChange={(event) => setToken(event.target.value)}
                placeholder={
                  currentTokenStored
                    ? "已有 Token，需要替换时再输入"
                    : "在 Telegram bot 绑定工号后获取"
                }
              />
            </div>
          </label>
        </div>

        {!cloudAvailable && (
          <div className="plane-connection-message" role="note">
            浏览器预览模式仅支持保存非敏感设置；Token 保存和云端上传需要在
            Wails 桌面客户端中使用。
          </div>
        )}

        <div className="connection-action-row">
          <span>
            Token 与当前 API 地址和工号绑定。更换任一项后，需要重新保存 Token。
          </span>
          <div className="panel-actions">
            <button
              type="button"
              className="button secondary"
              disabled={Boolean(busy) || !currentTokenStored}
              onClick={removeToken}
            >
              {busy === "delete-token" ? (
                <LoaderCircle className="spin" size={15} />
              ) : (
                <Trash2 size={15} />
              )}
              删除 Token
            </button>
            <button
              type="button"
              className="button primary"
              disabled={Boolean(busy)}
              onClick={save}
            >
              {busy === "save" ? (
                <LoaderCircle className="spin" size={15} />
              ) : (
                <Save size={15} />
              )}
              保存日报设置
            </button>
          </div>
        </div>
      </section>
    </section>
  );
}
