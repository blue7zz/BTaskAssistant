import { useEffect, useState, type FormEvent } from "react";
import { FilePlus2, MessageSquareText, X } from "lucide-react";
import type { TaskPriority } from "../domain/task";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";

export type ComposerMode = "task" | "chat";

interface TaskComposerProps {
  open: boolean;
  initialMode: ComposerMode;
  onClose(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

export function TaskComposer({
  open,
  initialMode,
  onClose,
  onSuccess,
  onError,
}: TaskComposerProps) {
  const createTask = useWorkspaceStore((state) => state.createTask);
  const importChat = useWorkspaceStore((state) => state.importChat);
  const [mode, setMode] = useState<ComposerMode>(initialMode);
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [projectName, setProjectName] = useState("");
  const [priority, setPriority] = useState<TaskPriority>("medium");
  const [chatContent, setChatContent] = useState("");

  useEffect(() => {
    if (open) setMode(initialMode);
  }, [initialMode, open]);

  if (!open) return null;

  const reset = () => {
    setTitle("");
    setSummary("");
    setProjectName("");
    setPriority("medium");
    setChatContent("");
  };

  const close = () => {
    reset();
    onClose();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    try {
      if (mode === "task") {
        if (!title.trim()) throw new Error("请输入任务标题");
        if (!summary.trim()) throw new Error("请输入任务说明");
        createTask({
          title,
          summary,
          projectName,
          priority,
          initialEvidence: {
            type: "manual",
            title: "创建任务时的说明",
            content: summary,
          },
        });
        onSuccess("任务已加入任务池");
      } else {
        importChat({ title, content: chatContent, projectName });
        onSuccess("聊天记录已导入，原文已保留为需求来源");
      }
      close();
    } catch (error) {
      onError(error);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={close}>
      <section
        className="dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="composer-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="dialog-header">
          <div>
            <span className="eyebrow">任务入口</span>
            <h2 id="composer-title">
              {mode === "task" ? "添加任务" : "导入聊天记录"}
            </h2>
          </div>
          <button className="icon-button" onClick={close} aria-label="关闭">
            <X size={18} />
          </button>
        </header>

        <div className="mode-switch" aria-label="创建方式">
          <button
            className={mode === "task" ? "active" : ""}
            onClick={() => setMode("task")}
            type="button"
          >
            <FilePlus2 size={16} />
            手动添加
          </button>
          <button
            className={mode === "chat" ? "active" : ""}
            onClick={() => setMode("chat")}
            type="button"
          >
            <MessageSquareText size={16} />
            粘贴聊天
          </button>
        </div>

        <form className="composer-form" onSubmit={submit}>
          <label>
            <span>
              任务标题
              {mode === "chat" && <small>可留空，将取聊天首行</small>}
            </span>
            <input
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder={
                mode === "task" ? "例如：优化视频列表缓存" : "可选"
              }
              autoFocus
            />
          </label>

          <label>
            <span>对应项目或仓库 <small>可稍后确认</small></span>
            <input
              value={projectName}
              onChange={(event) => setProjectName(event.target.value)}
              placeholder="例如：blue7zz/Y16-app"
            />
          </label>

          {mode === "task" ? (
            <>
              <div className="composer-rich-field">
                <span>原始任务说明</span>
                <LazyRichMarkdownEditor
                  value={summary}
                  placeholder="填写任务说明、当前进展或后续待办，之后可以随时补充。"
                  compact
                  onCommit={setSummary}
                />
                <small>支持 Markdown、表格，以及粘贴或拖入图片</small>
              </div>
              <label>
                <span>优先级</span>
                <select
                  value={priority}
                  onChange={(event) =>
                    setPriority(event.target.value as TaskPriority)
                  }
                >
                  <option value="low">低</option>
                  <option value="medium">中</option>
                  <option value="high">高</option>
                </select>
              </label>
            </>
          ) : (
            <label>
              <span>原始聊天记录</span>
              <textarea
                value={chatContent}
                onChange={(event) => setChatContent(event.target.value)}
                placeholder="把与任务有关的聊天原文完整粘贴到这里。第一版不会擅自改写原文。"
                rows={11}
              />
            </label>
          )}

          <div className="dialog-actions">
            <button type="button" className="button ghost" onClick={close}>
              取消
            </button>
            <button type="submit" className="button primary">
              {mode === "task" ? "加入任务池" : "保留原文并导入"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
