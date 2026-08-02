import { useEffect, useState, type FormEvent } from "react";
import { KeyRound, LoaderCircle, Server, Trash2 } from "lucide-react";
import { rxSettingsApp, type ReasonixSettingsView } from "../lib/bridge";

const rxApp = (method: string): ((...args: unknown[]) => Promise<unknown>) => {
  const fn = rxSettingsApp()[method];
  if (typeof fn !== "function") {
    return async () => {
      throw new Error(`Reasonix 设置方法不可用: ${method}`);
    };
  }
  return fn;
};

interface ReasonixSettingsProps {
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

/**
 * Reasonix 设置（阶段 4）：Reasonix 全局配置的唯一入口。
 * 凭据只显示"已配置/未配置"状态，明文只在本页输入时出现。
 */
export function ReasonixSettings({ onSuccess, onError }: ReasonixSettingsProps) {
  const [view, setView] = useState<ReasonixSettingsView | null>(null);
  const [loading, setLoading] = useState(true);
  const [homeIsolated, setHomeIsolated] = useState(false);
  const [defaultModel, setDefaultModel] = useState("");
  const [plannerModel, setPlannerModel] = useState("");
  const [approvalMode, setApprovalMode] = useState("ask");
  // 新增 Provider 表单
  const [newName, setNewName] = useState("");
  const [newKind, setNewKind] = useState("openai");
  const [newBaseUrl, setNewBaseUrl] = useState("");
  const [newEnvName, setNewEnvName] = useState("");
  const [newKey, setNewKey] = useState("");

  const refresh = async () => {
    setLoading(true);
    try {
      const settings = (await (await rxApp("ReasonixSettings"))()) as ReasonixSettingsView;
      setView(settings);
      setHomeIsolated(settings.isolated);
      setDefaultModel(settings.defaultModel);
      setPlannerModel(settings.plannerModel);
      setApprovalMode(settings.defaultToolApproval);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  // 挂载时立即加载配置（此前缺失该调用导致一直显示"正在读取……"）
  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const applyHome = async () => {
    try {
      await (await rxApp("ReasonixSetHomeIsolated"))(homeIsolated);
      onSuccess(homeIsolated ? "已切换到隔离 Reasonix Home（新会话生效）" : "已切换到共享 Reasonix Home");
      await refresh();
    } catch (error) {
      onError(error);
    }
  };

  const saveModel = async () => {
    try {
      if (defaultModel) await (await rxApp("ReasonixSetDefaultModel"))(defaultModel);
      if (plannerModel) await (await rxApp("ReasonixSetPlannerModel"))(plannerModel);
      await (await rxApp("ReasonixSetApprovalMode"))(approvalMode);
      onSuccess("Reasonix 模型与审批设置已保存");
    } catch (error) {
      onError(error);
    }
  };

  const addProvider = async (event: FormEvent) => {
    event.preventDefault();
    try {
      await (await rxApp("ReasonixSaveProvider"))(
        newName,
        newKind,
        newBaseUrl,
        newEnvName,
        newKey,
      );
      setNewName("");
      setNewKind("openai");
      setNewBaseUrl("");
      setNewEnvName("");
      setNewKey("");
      onSuccess("Provider 已保存");
      await refresh();
    } catch (error) {
      onError(error);
    }
  };

  const removeProvider = async (name: string) => {
    try {
      await (await rxApp("ReasonixRemoveProvider"))(name);
      onSuccess(`Provider ${name} 已删除`);
      await refresh();
    } catch (error) {
      onError(error);
    }
  };

  if (loading && !view) {
    return (
      <div className="settings-section">
        <span className="spin-row">
          <LoaderCircle className="spin" size={14} />
          正在读取 Reasonix 配置……
        </span>
      </div>
    );
  }

  if (!view) {
    return (
      <div className="settings-section">
        <p>读取 Reasonix 配置失败，请检查内核状态。</p>
      </div>
    );
  }

  return (
    <div className="settings-section">
      <p className="settings-section-hint">
        Reasonix 全局配置的唯一入口。API Key 只保存到系统凭据存储，本页只显示
        已配置状态。
      </p>

      <section className="settings-block">
        <h3>Reasonix Home</h3>
        <label className="settings-row">
          <input
            type="checkbox"
            checked={homeIsolated}
            onChange={(event) => {
              setHomeIsolated(event.target.checked);
              void applyHome();
            }}
          />
          <span>
            使用 BTask 隔离目录（<code>{view.homePath}</code>）
          </span>
        </label>
        <p className="settings-note">
          当前配置：<code>{view.configPath}</code>
        </p>
      </section>

      <section className="settings-block">
        <h3>模型与审批</h3>
        <label className="settings-row">
          <span>默认模型</span>
          <select
            value={defaultModel}
            onChange={(event) => setDefaultModel(event.target.value)}
          >
            {view.models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </label>
        <label className="settings-row">
          <span>Planner 模型</span>
          <select
            value={plannerModel}
            onChange={(event) => setPlannerModel(event.target.value)}
          >
            <option value="">（不指定）</option>
            {view.models.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </label>
        <label className="settings-row">
          <span>默认工具审批</span>
          <select
            value={approvalMode}
            onChange={(event) => setApprovalMode(event.target.value)}
          >
            <option value="ask">ask（每次询问）</option>
            <option value="auto">auto（自动批准）</option>
            <option value="yolo">yolo（全自动）</option>
          </select>
        </label>
        <button type="button" className="button secondary compact" onClick={() => void saveModel()}>
          保存模型与审批设置
        </button>
      </section>

      <section className="settings-block">
        <h3>Provider</h3>
        <ul className="settings-provider-list">
          {view.providers.map((provider) => (
            <li key={provider.name} className="settings-row">
              <Server size={14} />
              <span>
                <strong>{provider.name}</strong>
                <small>
                  {provider.kind} · {provider.baseUrl || "默认地址"}
                </small>
              </span>
              <span className={provider.keySet ? "badge badge-ok" : "badge"}>
                {provider.keySet ? "已配置" : "未配置 Key"}
              </span>
              <button
                type="button"
                className="button compact danger-ghost"
                aria-label={`删除 Provider ${provider.name}`}
                onClick={() => void removeProvider(provider.name)}
              >
                <Trash2 size={13} />
              </button>
            </li>
          ))}
        </ul>
        <form className="settings-row" onSubmit={(event) => void addProvider(event)}>
          <KeyRound size={14} />
          <input
            type="text"
            placeholder="名称（如 my-openai）"
            value={newName}
            onChange={(event) => setNewName(event.target.value)}
          />
          <select
            value={newKind}
            onChange={(event) => setNewKind(event.target.value)}
            aria-label="Provider 类型"
          >
            <option value="openai">openai</option>
            <option value="anthropic">anthropic</option>
            <option value="deepseek">deepseek</option>
          </select>
          <input
            type="text"
            placeholder="Base URL（可选）"
            value={newBaseUrl}
            onChange={(event) => setNewBaseUrl(event.target.value)}
          />
          <input
            type="text"
            placeholder="凭据环境变量名（如 MY_API_KEY）"
            value={newEnvName}
            onChange={(event) => setNewEnvName(event.target.value)}
          />
          <input
            type="password"
            placeholder="API Key（可选，仅本次输入）"
            value={newKey}
            onChange={(event) => setNewKey(event.target.value)}
          />
          <button type="submit" className="button secondary compact">
            添加
          </button>
        </form>
      </section>

      <section className="settings-block">
        <h3>会话保留</h3>
        <p className="settings-note">
          每任务最多保留 {view.maxSessionsPerTask} 个会话文件；后台最多保留{" "}
          {view.maxIdleRuntimes} 个空闲运行时（运行中/等待审批的会话不会被回收）。
        </p>
      </section>
    </div>
  );
}
