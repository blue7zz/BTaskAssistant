import {
  FileDiff,
  FolderGit2,
  GitBranch,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type {
  GitChangedFile,
  TaskFileDiff,
  TaskGitStatus,
} from "../domain/agent";
import type { TaskStatus } from "../domain/task";
import type { AgentClient } from "../lib/agentBridge";

interface AgentChangesPanelProps {
  taskId: string;
  taskStatus: TaskStatus;
  client: AgentClient;
  refreshVersion: number;
}

function errorText(error: unknown): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return "Git worktree 操作失败";
}

function shortCommit(value?: string): string {
  return value ? value.slice(0, 10) : "—";
}

function statusLabel(file: GitChangedFile): string {
  const labels: Record<string, string> = {
    modified: "修改",
    added: "新增",
    deleted: "删除",
    renamed: "重命名",
    copied: "复制",
    untracked: "未跟踪",
    conflicted: "冲突",
  };
  return labels[file.status] ?? file.status;
}

export function AgentChangesPanel({
  taskId,
  taskStatus,
  client,
  refreshVersion,
}: AgentChangesPanelProps) {
  const [status, setStatus] = useState<TaskGitStatus>();
  const [diff, setDiff] = useState<TaskFileDiff>();
  const [selectedPath, setSelectedPath] = useState("");
  const [commitMessage, setCommitMessage] = useState("");
  const [commitPreview, setCommitPreview] = useState(false);
  const [cleanupPreview, setCleanupPreview] = useState(false);
  const [recoverPreview, setRecoverPreview] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const epochRef = useRef(0);
  const diffRequestRef = useRef(0);

  const loadStatus = useCallback(async (epoch = epochRef.current) => {
    if (!client.getTaskGitStatus) {
      if (epoch === epochRef.current) {
        setLoading(false);
        setError("当前客户端不支持任务 Git worktree");
      }
      return;
    }
    const loaded = await client.getTaskGitStatus(taskId);
    if (epoch !== epochRef.current) return;
    setStatus(loaded);
    setError(loaded.errorMessage ?? "");
    setCommitPreview(false);
    setCleanupPreview(false);
    setRecoverPreview(false);
    diffRequestRef.current += 1;
    setSelectedPath("");
    setDiff(undefined);
  }, [client, taskId]);

  useEffect(() => {
    const epoch = ++epochRef.current;
    diffRequestRef.current += 1;
    setStatus(undefined);
    setDiff(undefined);
    setSelectedPath("");
    setCommitMessage("");
    setCommitPreview(false);
    setCleanupPreview(false);
    setRecoverPreview(false);
    setError("");
    setLoading(true);
    void loadStatus(epoch)
      .catch((reason) => {
        if (epoch === epochRef.current) setError(errorText(reason));
      })
      .finally(() => {
        if (epoch === epochRef.current) setLoading(false);
      });
    return () => {
      epochRef.current += 1;
    };
  }, [loadStatus, refreshVersion, taskId]);

  const bindRepository = async () => {
    if (!client.selectGitRepository || !client.bindGitRepository) {
      setError("当前客户端不支持绑定 Git 仓库");
      return;
    }
    const epoch = epochRef.current;
    setBusy(true);
    setError("");
    try {
      const sourcePath = await client.selectGitRepository();
      if (!sourcePath || epoch !== epochRef.current) return;
      await client.bindGitRepository({ taskId, sourcePath });
      await loadStatus(epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setBusy(false);
    }
  };

  const refresh = async () => {
    const epoch = epochRef.current;
    setLoading(true);
    setError("");
    try {
      await loadStatus(epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setLoading(false);
    }
  };

  const showDiff = async (file: GitChangedFile) => {
    if (!client.getTaskFileDiff) return;
    const epoch = epochRef.current;
    const request = ++diffRequestRef.current;
    setSelectedPath(file.path);
    setDiff(undefined);
    setError("");
    try {
      const loaded = await client.getTaskFileDiff(taskId, file.path);
      if (
        epoch === epochRef.current &&
        request === diffRequestRef.current &&
        loaded.taskId === taskId &&
        loaded.path === file.path
      ) {
        setDiff(loaded);
      }
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    }
  };

  const commitChanges = async () => {
    if (!status?.snapshot || !client.commitTaskGitChanges) return;
    if (!commitPreview) {
      setCommitPreview(true);
      return;
    }
    const epoch = epochRef.current;
    setBusy(true);
    setError("");
    try {
      const result = await client.commitTaskGitChanges({
        taskId,
        message: commitMessage,
        expectedSnapshot: status.snapshot,
        confirmed: true,
      });
      if (epoch !== epochRef.current) return;
      setStatus(result.status);
      setCommitMessage("");
      setCommitPreview(false);
      diffRequestRef.current += 1;
      setSelectedPath("");
      setDiff(undefined);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setBusy(false);
    }
  };

  const cleanup = async () => {
    if (!client.cleanupTaskGitWorktree) return;
    if (!cleanupPreview) {
      setCleanupPreview(true);
      return;
    }
    const epoch = epochRef.current;
    setBusy(true);
    setError("");
    try {
      await client.cleanupTaskGitWorktree({ taskId, confirmed: true });
      await loadStatus(epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setBusy(false);
    }
  };

  const recover = async () => {
    if (!client.recoverTaskGitWorktree) return;
    if (!recoverPreview) {
      setRecoverPreview(true);
      return;
    }
    const epoch = epochRef.current;
    setBusy(true);
    setError("");
    try {
      await client.recoverTaskGitWorktree({ taskId, confirmed: true });
      await loadStatus(epoch);
    } catch (reason) {
      if (epoch === epochRef.current) setError(errorText(reason));
    } finally {
      if (epoch === epochRef.current) setBusy(false);
    }
  };

  if (loading && !status) {
    return <div className="agent-side-loading"><LoaderCircle className="spin" size={14} />读取 Git 状态…</div>;
  }

  if (!status?.bound) {
    return (
      <div className="agent-git-empty">
        <FolderGit2 size={22} />
        <strong>尚未绑定 Git 仓库</strong>
        <p>选择来源仓库后，BTask 会在当前任务目录创建独立分支与 worktree，不修改来源工作区。</p>
        <button type="button" className="button primary compact" disabled={busy} onClick={() => void bindRepository()}>
          {busy ? <LoaderCircle className="spin" size={12} /> : <FolderGit2 size={12} />}
          选择并绑定
        </button>
        {error && <p className="agent-side-error">{error}</p>}
      </div>
    );
  }

  const binding = status.binding;
  const ready = binding.state === "ready" && !status.errorMessage;
  const dirty = status.files.length > 0;
  const canCommit = ready && dirty && taskStatus === "development";

  return (
    <div className="agent-git-panel">
      <section className="agent-git-binding">
        <header>
          <span><GitBranch size={13} /><strong>{binding.branch ?? "任务分支"}</strong></span>
          <button type="button" aria-label="刷新 Git 状态" disabled={loading || busy} onClick={() => void refresh()}>
            <RefreshCw className={loading ? "spin" : ""} size={11} />
          </button>
        </header>
        <dl>
          <div><dt>来源</dt><dd title={binding.sourceRealPath}>{binding.sourceRealPath}</dd></div>
          {status.remoteUrl && <div><dt>远端</dt><dd title={status.remoteUrl}>{status.remoteUrl}</dd></div>}
          <div><dt>Worktree</dt><dd title={binding.worktreePath}>{binding.worktreePath ?? "—"}</dd></div>
          <div><dt>基线</dt><dd>{shortCommit(binding.baselineCommit)}</dd></div>
          <div><dt>HEAD</dt><dd>{shortCommit(status.head)}</dd></div>
        </dl>
        {binding.sourceDirtyAtBind && <p className="agent-git-warning">绑定时来源工作区已有修改；这些修改未复制到任务 worktree。</p>}
        {!ready && (
          <div className="agent-git-recovery">
            <p>{status.errorMessage || binding.errorMessage || `worktree 状态：${binding.state}`}</p>
            <button type="button" className="button secondary compact" disabled={busy} onClick={() => void recover()}>
              <RotateCcw size={11} />{recoverPreview ? "确认恢复 worktree" : "恢复 worktree"}
            </button>
          </div>
        )}
      </section>

      {error && <p className="agent-side-error">{error}</p>}

      {ready && (
        <>
          <section className="agent-git-files">
            <header><strong>未提交变更</strong><span>{status.files.length}</span></header>
            {status.files.map((file) => (
              <button
                type="button"
                key={`${file.originalPath ?? ""}-${file.path}`}
                className={selectedPath === file.path ? "active" : ""}
                onClick={() => void showDiff(file)}
              >
                <FileDiff size={11} />
                <span><strong>{file.path}</strong><small>{file.originalPath ? `${file.originalPath} → ` : ""}{statusLabel(file)}</small></span>
                <em>{file.staged ? "已暂存" : ""}{file.staged && file.unstaged ? " + " : ""}{file.unstaged ? "未暂存" : ""}</em>
              </button>
            ))}
            {!dirty && <p>任务 worktree 当前干净。</p>}
          </section>

          {selectedPath && (
            <section className="agent-git-diff">
              <header><strong>{selectedPath}</strong>{diff && <span>+{diff.added} −{diff.removed}</span>}</header>
              {!diff && <div className="agent-side-loading"><LoaderCircle className="spin" size={12} />读取 Diff…</div>}
              {diff?.binary && <p>二进制文件：仅显示 Git 二进制变更标记。</p>}
              {diff?.truncated && <p>Diff 超过 2 MiB，当前预览已截断。</p>}
              {diff?.staged && <><small>已暂存</small><pre>{diff.staged}</pre></>}
              {diff?.unstaged && <><small>未暂存</small><pre>{diff.unstaged}</pre></>}
            </section>
          )}

          <section className="agent-git-actions">
            <label>
              <span>本地 commit message</span>
              <input
                value={commitMessage}
                disabled={!canCommit || busy}
                placeholder={taskStatus === "development" ? "例如：fix(UI): 修复任务详情展示" : "仅开发中任务可提交"}
                onChange={(event) => { setCommitMessage(event.target.value); setCommitPreview(false); }}
              />
            </label>
            {commitPreview && <p>将暂存并提交上方 {status.files.length} 个变更；不会推送远端。</p>}
            <button
              type="button"
              className="button primary compact"
              disabled={!canCommit || !commitMessage.trim() || busy}
              onClick={() => void commitChanges()}
            >
              {busy ? <LoaderCircle className="spin" size={11} /> : <GitBranch size={11} />}
              {commitPreview ? "确认创建本地 commit" : "预览本地 commit"}
            </button>
            <button
              type="button"
              className="button secondary compact agent-cleanup-button"
              disabled={dirty || busy}
              onClick={() => void cleanup()}
            >
              <Trash2 size={11} />{cleanupPreview ? "确认安全清理" : "安全清理 worktree"}
            </button>
            {cleanupPreview && <p>仅删除干净的任务 worktree；任务分支和本地 commit 会保留，不会推送。</p>}
          </section>
        </>
      )}
    </div>
  );
}
