import {
  ArrowLeft,
  BrainCircuit,
  CloudDownload,
} from "lucide-react";
import { useState, type KeyboardEvent } from "react";
import type { EngineStatus } from "../lib/bridge";
import { PISettingsPage } from "./PISettingsPage";
import { PlaneConnectionSettings } from "./PlaneConnectionSettings";

type SettingsCategory = "pi" | "plane";

interface SettingsPageProps {
  engine?: EngineStatus;
  planeConnected: boolean;
  onBack(): void;
  onPlaneConnectionChange(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

export function SettingsPage({
  engine,
  planeConnected,
  onBack,
  onPlaneConnectionChange,
  onSuccess,
  onError,
}: SettingsPageProps) {
  const [category, setCategory] = useState<SettingsCategory>("pi");
  const selectAdjacentCategory = (
    event: KeyboardEvent<HTMLButtonElement>,
  ) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const nextCategory = category === "pi" ? "plane" : "pi";
    setCategory(nextCategory);
    const nextTab = event.currentTarget.parentElement?.querySelector(
      `[data-settings-category="${nextCategory}"]`,
    ) as HTMLButtonElement | null;
    nextTab?.focus();
  };

  return (
    <section className="settings-page">
      <header className="settings-page-toolbar">
        <div>
          <span className="eyebrow">应用设置</span>
          <h1>设置</h1>
          <p>按分类管理外部连接与本地执行参数。</p>
        </div>
        <button
          type="button"
          className="button compact secondary"
          onClick={onBack}
          aria-label="返回上一个界面"
        >
          <ArrowLeft size={15} />
          返回
        </button>
      </header>

      <div className="settings-page-body">
        <nav
          className="settings-category-nav"
          aria-label="设置分类"
          role="tablist"
        >
          <button
            type="button"
            role="tab"
            id="settings-tab-pi"
            aria-controls="settings-panel-pi"
            aria-selected={category === "pi"}
            tabIndex={category === "pi" ? 0 : -1}
            data-settings-category="pi"
            className={category === "pi" ? "active" : ""}
            onClick={() => setCategory("pi")}
            onKeyDown={selectAdjacentCategory}
          >
            <BrainCircuit size={17} />
            <span>
              <strong>PI 设置</strong>
              <small>模型与推理参数</small>
            </span>
          </button>
          <button
            type="button"
            role="tab"
            id="settings-tab-plane"
            aria-controls="settings-panel-plane"
            aria-selected={category === "plane"}
            tabIndex={category === "plane" ? 0 : -1}
            data-settings-category="plane"
            className={category === "plane" ? "active" : ""}
            onClick={() => setCategory("plane")}
            onKeyDown={selectAdjacentCategory}
          >
            <CloudDownload size={17} />
            <span>
              <strong>Plane 连接</strong>
              <small>连接与来源显示</small>
            </span>
          </button>
        </nav>

        <div
          className="settings-category-content"
          role="tabpanel"
          id={`settings-panel-${category}`}
          aria-labelledby={`settings-tab-${category}`}
        >
          {category === "pi" ? (
            <PISettingsPage engine={engine} onSuccess={onSuccess} />
          ) : (
            <PlaneConnectionSettings
              connected={planeConnected}
              onConnectionChange={onPlaneConnectionChange}
              onSuccess={onSuccess}
              onError={onError}
            />
          )}
        </div>
      </div>
    </section>
  );
}
