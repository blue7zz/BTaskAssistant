import { X } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import {
  STATUS_META,
  TASK_STATUSES,
  type Task,
  type TaskPriority,
  type TaskStatus,
} from "../domain/task";
import { useWorkspaceStore } from "../store/workspace";

interface TaskBasicInfoDialogProps {
  task: Task;
  onClose(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

export function TaskBasicInfoDialog({
  task,
  onClose,
  onSuccess,
  onError,
}: TaskBasicInfoDialogProps) {
  const updateTaskRecord = useWorkspaceStore((state) => state.updateTaskRecord);
  const [title, setTitle] = useState(task.title);
  const [projectName, setProjectName] = useState(task.projectName);
  const [status, setStatus] = useState<TaskStatus>(task.status);
  const [priority, setPriority] = useState<TaskPriority>(task.priority);

  useEffect(() => {
    setTitle(task.title);
    setProjectName(task.projectName);
    setStatus(task.status);
    setPriority(task.priority);
  }, [task]);

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    try {
      updateTaskRecord(task.id, { title, projectName, status, priority });
      onSuccess("任务基本信息已保存");
      onClose();
    } catch (error) {
      onError(error);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        className="dialog task-basic-info-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="task-basic-info-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="dialog-header">
          <div>
            <span className="eyebrow">任务操作</span>
            <h2 id="task-basic-info-title">编辑基本信息</h2>
          </div>
          <button
            type="button"
            className="icon-button"
            onClick={onClose}
            aria-label="关闭编辑任务"
          >
            <X size={18} />
          </button>
        </header>

        <form className="composer-form" onSubmit={submit}>
          <label>
            <span>任务标题</span>
            <input
              name="title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              autoFocus
            />
          </label>
          <label>
            <span>项目分类</span>
            <input
              name="projectName"
              value={projectName}
              onChange={(event) => setProjectName(event.target.value)}
              placeholder="例如：BTaskAssistant、Y16 App"
            />
          </label>
          <label>
            <span>当前状态</span>
            <select
              name="status"
              value={status}
              onChange={(event) =>
                setStatus(event.target.value as TaskStatus)
              }
            >
              {TASK_STATUSES.map((taskStatus) => (
                <option value={taskStatus} key={taskStatus}>
                  {STATUS_META[taskStatus].label}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>优先级</span>
            <select
              name="priority"
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
          <div className="dialog-actions">
            <button
              type="button"
              className="button secondary"
              onClick={onClose}
            >
              取消
            </button>
            <button type="submit" className="button primary">
              保存修改
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
