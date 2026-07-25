import { Check, CircleDot, FolderGit2 } from "lucide-react";
import {
  STATUS_META,
  type Task,
  type TaskPriority,
} from "../domain/task";

interface TaskListProps {
  tasks: Task[];
  selectedTaskID?: string;
  onSelect(taskID: string): void;
}

const PRIORITY_LABEL: Record<TaskPriority, string> = {
  low: "低",
  medium: "中",
  high: "高",
};

function formatUpdatedAt(value: string): string {
  const date = new Date(value);
  const now = new Date();
  const difference = now.getTime() - date.getTime();
  const minutes = Math.floor(difference / 60_000);
  if (minutes < 1) return "刚刚更新";
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "short",
    day: "numeric",
  }).format(date);
}

export function TaskList({
  tasks,
  selectedTaskID,
  onSelect,
}: TaskListProps) {
  return (
    <aside className="task-list-panel">
      <div className="task-list-heading">
        <div>
          <span className="eyebrow">当前视图</span>
          <h2>任务</h2>
        </div>
        <span className="count-pill">{tasks.length}</span>
      </div>

      <div className="task-list" role="list">
        {tasks.map((task) => (
          <button
            type="button"
            role="listitem"
            key={task.id}
            className={`task-card ${selectedTaskID === task.id ? "selected" : ""}`}
            onClick={() => onSelect(task.id)}
          >
            <div className="task-card-topline">
              <span className={`priority priority-${task.priority}`}>
                <CircleDot size={12} />
                {PRIORITY_LABEL[task.priority]}
              </span>
              <span className={`status-chip status-${task.status}`}>
                {task.status === "done" && <Check size={12} />}
                {STATUS_META[task.status].label}
              </span>
            </div>
            <strong>{task.title}</strong>
            <p>{task.summary || "暂无任务说明"}</p>
            <div className="task-card-meta">
              <span>
                <FolderGit2 size={13} />
                {task.projectName || "项目待确认"}
              </span>
              <time dateTime={task.updatedAt}>
                {formatUpdatedAt(task.updatedAt)}
              </time>
            </div>
          </button>
        ))}
      </div>
    </aside>
  );
}

