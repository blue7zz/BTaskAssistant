import { useEffect, useState } from "react";
import {
  BrainCircuit,
  CheckCircle2,
  Clock3,
  Cpu,
  RotateCcw,
  Save,
  ShieldCheck,
  TerminalSquare,
} from "lucide-react";
import {
  DEFAULT_PI_SETTINGS,
  normalizePISettings,
  type PISettings,
  type PIThinkingEffort,
} from "../domain/engine";
import type { EngineStatus } from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";

const THINKING_OPTIONS: Array<{
  value: PIThinkingEffort;
  label: string;
  description: string;
}> = [
  { value: "low", label: "低", description: "更快，适合简单整理" },
  { value: "medium", label: "中", description: "速度与分析深度均衡" },
  { value: "high", label: "高", description: "适合多数需求访谈" },
  { value: "xhigh", label: "极高", description: "用于需要更深分析的任务" },
];

interface PISettingsPageProps {
  engine?: EngineStatus;
  onSuccess(message: string): void;
}

export function PISettingsPage({
  engine,
  onSuccess,
}: PISettingsPageProps) {
  const settings = useWorkspaceStore((state) => state.piSettings);
  const updateSettings = useWorkspaceStore((state) => state.updatePISettings);
  const [draft, setDraft] = useState<PISettings>(() => ({ ...settings }));

  useEffect(() => setDraft({ ...settings }), [settings]);

  const save = () => {
    const next = normalizePISettings(draft);
    updateSettings(next);
    setDraft(next);
    onSuccess("PI 设置已保存，将在新建 PI 会话时应用");
  };

  const reset = () => {
    updateSettings(DEFAULT_PI_SETTINGS);
    setDraft({ ...DEFAULT_PI_SETTINGS });
    onSuccess("PI 设置已恢复为隔离默认值");
  };

  return (
    <section className="pi-settings-page">
      <div className="pi-settings-hero">
        <div className="pi-settings-hero-icon">
          <BrainCircuit size={28} />
        </div>
        <div>
          <span className="eyebrow">NATIVE PI</span>
          <h2>配置原生 PI 的模型与推理偏好</h2>
          <p>
            默认使用任务隔离的 PI 配置目录；需要调用真实模型时，可以显式使用
            本机 PI 已登录的 provider。模型、凭据来源与思考强度会在新建
            Session 时应用。
          </p>
        </div>
        <div className={`pi-runtime-state ${engine?.configured ? "online" : ""}`}>
          <span className={`status-dot ${engine?.configured ? "online" : ""}`} />
          <div>
            <strong>{engine?.configured ? "已检测到原生 PI" : "PI 未检测到"}</strong>
            <small>{engine?.version || "等待桌面客户端检测"}</small>
          </div>
        </div>
      </div>

      <div className="pi-settings-layout">
        <div className="pi-settings-main">
          <section className="pi-settings-card">
            <header>
              <div>
                <Cpu size={18} />
                <div>
                  <h3>模型与推理</h3>
                  <p>启动后通过原生 PI RPC 校验，不转换成旧 CLI 参数。</p>
                </div>
              </div>
            </header>

            <div className="pi-settings-form">
              <label className="field">
                <span>
                  模型标识
                  <small>可选</small>
                </span>
                <input
                  value={draft.model}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      model: event.target.value,
                    }))
                  }
                  placeholder="例如 provider/model；留空则由原生 PI 会话选择"
                />
                <small className="pi-field-help">
                  使用 provider/model 格式；留空时由所选凭据来源的原生 PI 会话选择模型。
                </small>
              </label>

              <fieldset className="pi-thinking-fieldset">
                <legend>模型凭据来源</legend>
                <div className="pi-thinking-options pi-resource-policy-options">
                  <label
                    className={
                      draft.resourcePolicy === "isolated" ? "selected" : ""
                    }
                  >
                    <input
                      type="radio"
                      name="pi-resource-policy"
                      value="isolated"
                      checked={draft.resourcePolicy === "isolated"}
                      onChange={() =>
                        setDraft((current) => ({
                          ...current,
                          resourcePolicy: "isolated",
                        }))
                      }
                    />
                    <span>任务隔离</span>
                    <small>不读取本机 PI 登录配置，适合无模型调用的隔离检查。</small>
                  </label>
                  <label
                    className={
                      draft.resourcePolicy === "explicit-inherit"
                        ? "selected"
                        : ""
                    }
                  >
                    <input
                      type="radio"
                      name="pi-resource-policy"
                      value="explicit-inherit"
                      checked={draft.resourcePolicy === "explicit-inherit"}
                      onChange={() =>
                        setDraft((current) => ({
                          ...current,
                          resourcePolicy: "explicit-inherit",
                        }))
                      }
                    />
                    <span>使用本机 PI 登录配置</span>
                    <small>
                      读取 PI_CODING_AGENT_DIR（默认 ~/.pi/agent）的模型与认证，不复制密钥。
                    </small>
                  </label>
                </div>
              </fieldset>

              <fieldset className="pi-thinking-fieldset">
                <legend>思考强度</legend>
                <div className="pi-thinking-options">
                  {THINKING_OPTIONS.map((option) => (
                    <label
                      key={option.value}
                      className={
                        draft.thinkingEffort === option.value ? "selected" : ""
                      }
                    >
                      <input
                        type="radio"
                        name="pi-thinking-effort"
                        value={option.value}
                        checked={draft.thinkingEffort === option.value}
                        onChange={() =>
                          setDraft((current) => ({
                            ...current,
                            thinkingEffort: option.value,
                          }))
                        }
                      />
                      <span>{option.label}</span>
                      <small>{option.description}</small>
                    </label>
                  ))}
                </div>
              </fieldset>

              <label className="field pi-timeout-field">
                <span>
                  单次执行时限
                  <small>超时后自动停止</small>
                </span>
                <div>
                  <Clock3 size={15} />
                  <select
                    value={draft.timeoutMinutes}
                    onChange={(event) =>
                      setDraft((current) => ({
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
            </div>

            <footer className="pi-settings-actions">
              <button className="button secondary" type="button" onClick={reset}>
                <RotateCcw size={15} />
                恢复默认
              </button>
              <button className="button primary" type="button" onClick={save}>
                <Save size={15} />
                保存 PI 设置
              </button>
            </footer>
          </section>

          <section className="pi-compatibility-card">
            <CheckCircle2 size={20} />
            <div>
              <strong>
                {draft.resourcePolicy === "explicit-inherit"
                  ? "已显式使用本机 PI 登录配置"
                  : "已启用每任务隔离策略"}
              </strong>
              {draft.resourcePolicy === "explicit-inherit" ? (
                <p>
                  仅使用本机 provider 认证与模型配置；不会复制密钥，任务 Session
                  与运行记录仍保持隔离。
                </p>
              ) : (
                <p>
                  默认资源策略为 isolated，不继承 ~/.pi/agent。旧的 max
                  思考强度会读时迁移为 xhigh，实际可用档位由原生 PI RPC 校验。
                </p>
              )}
            </div>
          </section>
        </div>

        <aside className="pi-settings-aside">
          <section className="pi-settings-card pi-runtime-card">
            <header>
              <div>
                <TerminalSquare size={18} />
                <div>
                  <h3>运行环境</h3>
                  <p>桌面客户端本次检测到的 PI CLI。</p>
                </div>
              </div>
            </header>
            <dl>
              <div>
                <dt>状态</dt>
                <dd>{engine?.configured ? "已安装，RPC 可用" : "未安装或不在 PATH"}</dd>
              </div>
              <div>
                <dt>版本</dt>
                <dd>{engine?.version || "—"}</dd>
              </div>
              <div>
                <dt>命令位置</dt>
                <dd title={engine?.commandPath}>{engine?.commandPath || "—"}</dd>
              </div>
            </dl>
          </section>

          <section className="pi-settings-card pi-boundary-card">
            <header>
              <div>
                <ShieldCheck size={18} />
                <div>
                  <h3>固定安全边界</h3>
                  <p>阶段 2 原生 RPC 的固定资源与执行边界。</p>
                </div>
              </div>
            </header>
            <ul>
              <li>
                {draft.resourcePolicy === "explicit-inherit"
                  ? "仅显式读取本机 PI 的模型配置与 provider 凭据"
                  : "不继承本机 PI 的模型配置或 provider 凭据"}
              </li>
              <li>全局扩展、技能、Prompt、主题与内建工具仍不自动加载</li>
              <li>任务资料、Session 与运行目录按任务隔离</li>
              <li>分析结果仍需用户采纳和人工批准</li>
            </ul>
          </section>
        </aside>
      </div>
    </section>
  );
}
