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
  { value: "xhigh", label: "极高", description: "兼容 gpt-5.5 的最高档" },
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
    onSuccess("PI 详细设置已保存，下一轮分析将使用新配置");
  };

  const reset = () => {
    updateSettings(DEFAULT_PI_SETTINGS);
    setDraft({ ...DEFAULT_PI_SETTINGS });
    onSuccess("PI 设置已恢复为兼容默认值");
  };

  return (
    <section className="pi-settings-page">
      <div className="pi-settings-hero">
        <div className="pi-settings-hero-icon">
          <BrainCircuit size={28} />
        </div>
        <div>
          <span className="eyebrow">PI / OH-MY-PI</span>
          <h2>控制每一次 PI 分析实际使用的参数</h2>
          <p>
            此处配置会同时用于需求访谈和 Plane 候选提炼。API Key
            与登录信息仍由 OMP 自己管理，不会写入任务数据库。
          </p>
        </div>
        <div className={`pi-runtime-state ${engine?.configured ? "online" : ""}`}>
          <span className={`status-dot ${engine?.configured ? "online" : ""}`} />
          <div>
            <strong>{engine?.configured ? "PI 已就绪" : "PI 未检测到"}</strong>
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
                  <p>显式覆盖 OMP 全局参数，避免模型与思考档位不兼容。</p>
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
                  placeholder="例如 openai-codex/gpt-5.5；留空则使用 OMP 默认模型"
                />
                <small className="pi-field-help">
                  留空不会读取或复制 OMP 的全局配置，只在运行时沿用其默认模型。
                </small>
              </label>

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
              <strong>已启用 gpt-5.5 兼容修复</strong>
              <p>
                每次运行都会显式传入所选思考强度。默认使用 xhigh，不再继承可能为
                max 的 OMP 全局值；旧的 max 设置也会自动迁移为 xhigh。
              </p>
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
                <dd>{engine?.configured ? "可用" : "未安装或不在 PATH"}</dd>
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
                  <p>这些限制不能在设置页中关闭。</p>
                </div>
              </div>
            </header>
            <ul>
              <li>需求分析只开放 read、grep、glob</li>
              <li>不加载技能、规则、扩展或会话</li>
              <li>关闭 LSP、PTY 与代码写入能力</li>
              <li>分析结果仍需用户采纳和人工批准</li>
            </ul>
          </section>
        </aside>
      </div>
    </section>
  );
}
