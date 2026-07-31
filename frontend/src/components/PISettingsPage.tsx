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
  { value: "xhigh", label: "极高", description: "保留旧设置并等待 PI 校验" },
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
    onSuccess("PI 设置已保存，将在原生 RPC 接入后使用");
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
            每个任务使用隔离的 PI 配置目录，默认不读取或复制
            ~/.pi/agent。原生 RPC 接入前，这些设置只会保存，不会启动 PI。
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
                  留空不会读取或复制全局 PI 配置；阶段 2 会根据可用模型列表显式匹配。
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
              <strong>已启用每任务隔离策略</strong>
              <p>
                默认资源策略为 isolated，不继承 ~/.pi/agent。旧的 max
                思考强度会读时迁移为 xhigh，实际可用档位由原生 PI RPC 校验。
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
                <dd>{engine?.configured ? "已安装，RPC 待接入" : "未安装或不在 PATH"}</dd>
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
                  <p>阶段 1 锁定的资源与执行边界。</p>
                </div>
              </div>
            </header>
            <ul>
              <li>默认不继承全局 PI 资源或凭据</li>
              <li>阶段 2 RPC 完成前不启动 PI</li>
              <li>任务资料、Session 与运行目录按任务隔离</li>
              <li>分析结果仍需用户采纳和人工批准</li>
            </ul>
          </section>
        </aside>
      </div>
    </section>
  );
}
