import {
  FolderCog,
  FolderOpen,
  HardDrive,
  LoaderCircle,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import {
  getTaskContextRoot,
  openTaskContextRoot,
  selectTaskContextRoot,
  setTaskContextRoot,
  taskContextDirectoryAvailable,
  type TaskContextRootInfo,
} from "../lib/bridge";

interface TaskContextSettingsProps {
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

type BusyAction = "change" | "open";

export function TaskContextSettings({
  onSuccess,
  onError,
}: TaskContextSettingsProps) {
  const nativeAvailable = taskContextDirectoryAvailable();
  const callbacks = useRef({ onSuccess, onError });
  const [rootInfo, setRootInfo] = useState<TaskContextRootInfo>();
  const [loading, setLoading] = useState(nativeAvailable);
  const [busy, setBusy] = useState<BusyAction>();

  useEffect(() => {
    callbacks.current = { onSuccess, onError };
  }, [onError, onSuccess]);

  useEffect(() => {
    if (!nativeAvailable) {
      setLoading(false);
      return;
    }

    let active = true;
    getTaskContextRoot()
      .then((info) => {
        if (active) setRootInfo(info);
      })
      .catch((error) => {
        if (active) callbacks.current.onError(error);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [nativeAvailable]);

  const changeRoot = async () => {
    if (!nativeAvailable || busy) return;
    setBusy("change");
    try {
      const selected = await selectTaskContextRoot();
      if (!selected.trim()) return;
      const info = await setTaskContextRoot(selected);
      setRootInfo(info);
      callbacks.current.onSuccess("任务资料根目录已更新");
    } catch (error) {
      callbacks.current.onError(error);
    } finally {
      setBusy(undefined);
    }
  };

  const openRoot = async () => {
    if (!nativeAvailable || !rootInfo?.available || busy) return;
    setBusy("open");
    try {
      await openTaskContextRoot();
    } catch (error) {
      try {
        setRootInfo(await getTaskContextRoot());
      } catch {
        // Keep the original open error as the actionable message.
      }
      callbacks.current.onError(error);
    } finally {
      setBusy(undefined);
    }
  };

  const currentPath = nativeAvailable
    ? rootInfo?.path ?? (loading ? "正在读取任务资料目录…" : "目录信息加载失败")
    : "浏览器预览模式不创建物理目录";

  return (
    <section className="task-context-settings-page">
      <section className="collector-settings task-context-settings-card">
        <div className="settings-heading">
          <div className="settings-icon">
            <HardDrive size={18} />
          </div>
          <div>
            <strong>任务资料根目录</strong>
            <span>每个任务使用独立子目录，集中保存完整上下文</span>
          </div>
        </div>

        <div className="task-context-status-row" aria-live="polite">
          {!nativeAvailable ? (
            <span className="task-context-status unavailable">
              浏览器预览 · 无物理目录
            </span>
          ) : loading ? (
            <span className="task-context-status">
              <LoaderCircle className="spin" size={13} />
              正在读取目录
            </span>
          ) : rootInfo ? (
            <>
              <span className="task-context-status">
                {rootInfo.custom ? "自定义目录" : "应用默认目录"}
              </span>
              <span
                className={`task-context-status ${
                  rootInfo.available ? "available" : "unavailable"
                }`}
              >
                {rootInfo.available ? "目录可用" : "目录不可用"}
              </span>
            </>
          ) : (
            <span className="task-context-status unavailable">
              目录状态未知
            </span>
          )}
        </div>

        {rootInfo && (
          <div className="task-context-status-row" aria-live="polite">
            <span className="task-context-status">
              SQLite v{rootInfo.databaseSchemaVersion ?? 3} · Task Workspace v
              {rootInfo.taskWorkspaceSchemaVersion ?? 1}
            </span>
            <span className="task-context-status">
              已建立 {rootInfo.workspaceCount ?? 0} 个任务空间
            </span>
            {(rootInfo.workspaceErrorCount ?? 0) > 0 && (
              <span className="task-context-status unavailable" role="alert">
                {rootInfo.workspaceErrorCount} 个任务空间需要修复
              </span>
            )}
          </div>
        )}

        <label className="field task-context-path-field">
          <span>
            当前目录
            <small>任务子目录以任务 ID 命名</small>
          </span>
          <div className="task-context-path-control">
            <FolderOpen size={16} />
            <input
              aria-label="任务资料根目录"
              value={currentPath}
              readOnly
              disabled={!nativeAvailable || loading}
            />
          </div>
        </label>

        {rootInfo?.custom && (
          <p className="task-context-default-path">
            应用默认目录：<code>{rootInfo.defaultPath}</code>
          </p>
        )}

        <div className="task-context-actions">
          <p>
            系统会按任务隔离 context、sources、attachments、artifacts、repos 和 runs。更改时请选择空目录；迁移成功后旧目录与旧版资料都会保留。
          </p>
          <div>
            <button
              type="button"
              className="button secondary"
              onClick={changeRoot}
              disabled={!nativeAvailable || loading || Boolean(busy)}
            >
              {busy === "change" ? (
                <LoaderCircle className="spin" size={15} />
              ) : (
                <FolderCog size={15} />
              )}
              更改目录
            </button>
            <button
              type="button"
              className="button primary"
              onClick={openRoot}
              disabled={
                !nativeAvailable ||
                loading ||
                !rootInfo?.available ||
                Boolean(busy)
              }
            >
              {busy === "open" ? (
                <LoaderCircle className="spin" size={15} />
              ) : (
                <FolderOpen size={15} />
              )}
              打开目录
            </button>
          </div>
        </div>

        {!nativeAvailable && (
          <div className="task-context-preview-note">
            浏览器预览不会模拟或保存本机目录。请在 Wails 桌面客户端中查看、更改或打开任务资料目录。
          </div>
        )}
      </section>
    </section>
  );
}
