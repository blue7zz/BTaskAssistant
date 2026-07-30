import {
  Eye,
  FolderKanban,
  KeyRound,
  Link2,
  LoaderCircle,
  SearchCheck,
  Server,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  planeWorkspaceAddress,
  type PlaneProject,
} from "../domain/collection";
import {
  hasPlaneToken,
  setupPlaneConnection,
  testPlaneConnection,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";

type BusyAction = "connect" | "test";

interface PlaneConnectionSettingsProps {
  connected: boolean;
  onConnectionChange(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

export function PlaneConnectionSettings({
  connected,
  onConnectionChange,
  onSuccess,
  onError,
}: PlaneConnectionSettingsProps) {
  const settings = useWorkspaceStore((state) => state.planeSettings);
  const updateSettings = useWorkspaceStore(
    (state) => state.updatePlaneSettings,
  );
  const [token, setToken] = useState("");
  const [tokenStored, setTokenStored] = useState(false);
  const [busy, setBusy] = useState<BusyAction>();
  const [projects, setProjects] = useState<PlaneProject[]>([]);
  const [serviceAddress, setServiceAddress] = useState(() =>
    planeWorkspaceAddress(settings),
  );
  const [connectionReady, setConnectionReady] = useState(false);
  const [connectionMessage, setConnectionMessage] = useState("");
  const tokenCheckID = useRef(0);

  const serviceAddressMatchesSettings =
    serviceAddress.trim().replace(/\/+$/, "") ===
    planeWorkspaceAddress(settings).replace(/\/+$/, "");
  const canUseStoredToken =
    tokenStored && serviceAddressMatchesSettings;
  const configured = Boolean(
    settings.baseUrl.trim() &&
      settings.workspaceSlug.trim() &&
      settings.projectId.trim(),
  );
  const sourceToggleEnabled =
    configured && connected && serviceAddressMatchesSettings;
  const projectOptions = useMemo(() => {
    if (
      !settings.projectId ||
      projects.some((project) => project.id === settings.projectId)
    ) {
      return projects;
    }
    return [
      {
        id: settings.projectId,
        name: settings.projectName || settings.projectId,
        identifier: settings.projectIdentifier ?? "",
      },
      ...projects,
    ];
  }, [
    projects,
    settings.projectId,
    settings.projectIdentifier,
    settings.projectName,
  ]);

  useEffect(() => {
    let active = true;
    const checkID = ++tokenCheckID.current;
    if (!settings.baseUrl.trim() || !settings.workspaceSlug.trim()) {
      setTokenStored(false);
      setConnectionReady(false);
      return;
    }
    hasPlaneToken(settings)
      .then((stored) => {
        if (!active || checkID !== tokenCheckID.current) return;
        setTokenStored(stored);
        setConnectionReady(stored);
      })
      .catch(() => {
        if (!active || checkID !== tokenCheckID.current) return;
        setTokenStored(false);
        setConnectionReady(false);
      });
    return () => {
      active = false;
    };
  }, [settings.baseUrl, settings.workspaceSlug]);

  const run = async (
    action: BusyAction,
    operation: () => Promise<void>,
    onFailure?: () => void,
  ) => {
    setBusy(action);
    try {
      await operation();
    } catch (error) {
      onFailure?.();
      onError(error);
    } finally {
      setBusy(undefined);
    }
  };

  const connect = () => {
    tokenCheckID.current += 1;
    return run("connect", async () => {
      const setup = await setupPlaneConnection(serviceAddress, token);
      const discovered = setup.projects;
      if (discovered.length === 0) {
        throw new Error("连接成功，但这个工作区中没有可访问的项目");
      }
      setProjects(discovered);
      const sameConnection =
        setup.baseUrl === settings.baseUrl &&
        setup.workspaceSlug === settings.workspaceSlug;
      const selected =
        (sameConnection
          ? discovered.find(
              (project) => project.id === settings.projectId,
            ) ??
            discovered.find(
              (project) =>
                project.name.toLocaleLowerCase() ===
                settings.projectName.trim().toLocaleLowerCase(),
            )
          : undefined) ??
        (discovered.length === 1 ? discovered[0] : undefined);
      updateSettings({
        baseUrl: setup.baseUrl,
        workspaceSlug: setup.workspaceSlug,
        projectId: selected?.id ?? "",
        projectName: selected?.name ?? "",
        projectIdentifier: selected?.identifier ?? "",
      });
      setServiceAddress(
        planeWorkspaceAddress({
          ...settings,
          baseUrl: setup.baseUrl,
          workspaceSlug: setup.workspaceSlug,
        }),
      );
      setToken("");
      setTokenStored(true);
      setConnectionReady(true);
      onConnectionChange();
      const message =
        discovered.length === 1
          ? `已发现项目 ${discovered[0].identifier || discovered[0].name}，已自动选中。`
          : `已连接工作区 ${setup.workspaceSlug}，发现 ${discovered.length} 个项目，请选择。`;
      setConnectionMessage(message);
      onSuccess(`Plane 已连接；${message}`);
    }, onConnectionChange);
  };

  const testConnection = () =>
    run("test", async () => {
      const result = await testPlaneConnection(settings);
      setConnectionMessage(result.message);
      onConnectionChange();
      onSuccess(result.message);
    }, onConnectionChange);

  return (
    <section className="plane-settings-page">
      <div className="collector-settings plane-connection-card">
        <div className="settings-heading">
          <div className="settings-icon">
            <Server size={18} />
          </div>
          <div>
            <strong>Plane 连接</strong>
            <span>支持 Plane Cloud 和启用 HTTPS 的自托管实例</span>
          </div>
        </div>
        <div className="collector-settings-grid">
          <label className="field">
            <span>
              服务地址
              <small>粘贴浏览器里的工作区地址</small>
            </span>
            <input
              value={serviceAddress}
              onChange={(event) => {
                tokenCheckID.current += 1;
                setServiceAddress(event.target.value);
                setConnectionReady(false);
                setProjects([]);
                setConnectionMessage("");
              }}
              placeholder="https://plane.example.com/my-team/"
            />
          </label>
          <label className="field">
            <span>
              Access Token
              <small>
                {canUseStoredToken
                  ? "系统凭据库中已有令牌，可留空"
                  : "连接成功后保存到系统凭据库"}
              </small>
            </span>
            <div className="credential-input">
              <KeyRound size={16} />
              <input
                type="password"
                value={token}
                autoComplete="off"
                onChange={(event) => setToken(event.target.value)}
                placeholder={
                  canUseStoredToken
                    ? "已有令牌，需要替换时再输入"
                    : "plane_api_…"
                }
              />
            </div>
          </label>
        </div>
        <div className="connection-action-row">
          <span>
            工作区会从地址自动识别，项目会在连接成功后列出；无需填写 slug 或 UUID。
          </span>
          <button
            type="button"
            className="button secondary"
            disabled={
              Boolean(busy) ||
              !serviceAddress.trim() ||
              (!token.trim() && !canUseStoredToken)
            }
            onClick={connect}
          >
            {busy === "connect" ? (
              <LoaderCircle className="spin" size={16} />
            ) : (
              <Link2 size={16} />
            )}
            {connectionReady ? "重新连接并加载选项" : "连接并加载选项"}
          </button>
        </div>
        {connectionReady && (
          <div className="connection-options plane-connection-options">
            <div className="workspace-option">
              <FolderKanban size={17} />
              <div>
                <span>已识别工作区</span>
                <strong>{settings.workspaceSlug}</strong>
              </div>
            </div>
            <label className="field">
              <span>
                收集项目
                <small>从可访问项目中选择</small>
              </span>
              <select
                value={settings.projectId}
                onChange={(event) => {
                  const project = projectOptions.find(
                    (item) => item.id === event.target.value,
                  );
                  updateSettings({
                    projectId: project?.id ?? "",
                    projectName: project?.name ?? "",
                    projectIdentifier: project?.identifier ?? "",
                  });
                  setConnectionMessage("");
                  onConnectionChange();
                }}
              >
                <option value="">请选择 Plane 项目</option>
                {projectOptions.map((project) => (
                  <option key={project.id} value={project.id}>
                    {project.identifier
                      ? `${project.identifier} · ${project.name}`
                      : project.name}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className="button secondary"
              disabled={Boolean(busy) || !tokenStored || !settings.projectId}
              onClick={testConnection}
            >
              {busy === "test" ? (
                <LoaderCircle className="spin" size={16} />
              ) : (
                <SearchCheck size={16} />
              )}
              测试连接
            </button>
          </div>
        )}
        {connectionMessage && (
          <div className="plane-connection-message">{connectionMessage}</div>
        )}
      </div>

      <section className="plane-source-setting">
        <div className="settings-heading">
          <div className="settings-icon">
            <Eye size={18} />
          </div>
          <div>
            <strong>任务来源显示</strong>
            <span>仅控制左侧导航入口，不会删除配置或已收集候选</span>
          </div>
        </div>
        <label
          className={`setting-switch-row ${!sourceToggleEnabled ? "disabled" : ""}`}
        >
          <div>
            <strong>在任务来源中显示 Plane 收集箱</strong>
            <span>
              {sourceToggleEnabled
                ? "Plane 已配置并连接，可以随时显示或隐藏。"
                : "完成连接并选择收集项目后，才可以调整此选项。"}
            </span>
          </div>
          <input
            type="checkbox"
            role="switch"
            aria-label="在任务来源中显示 Plane 收集箱"
            checked={sourceToggleEnabled && settings.showInTaskSources}
            disabled={!sourceToggleEnabled}
            onChange={(event) => {
              updateSettings({ showInTaskSources: event.target.checked });
            }}
          />
        </label>
      </section>
    </section>
  );
}
