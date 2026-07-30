import { RotateCcw, Trash2 } from "lucide-react";
import {
  STATUS_META,
  type TaskPriority,
  type TrashedTask,
} from "../domain/task";

interface TrashViewProps {
  tasks: TrashedTask[];
  onRestore(taskID: string): void;
  onDeletePermanently(taskID: string): void;
}

const PRIORITY_LABEL: Record<TaskPriority, string> = {
  low: "低",
  medium: "中",
  high: "高",
};

function formatTrashedAt(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function TrashView({
  tasks,
  onRestore,
  onDeletePermanently,
}: TrashViewProps) {
  return (
    <section className="trash-view">
      <header className="trash-view-header">
        <div>
          <span className="eyebrow">任务管理</span>
          <h1>回收站</h1>
          <p>删除的任务会保留在这里，可以恢复或永久删除。</p>
        </div>
        <span className="trash-count">{tasks.length}</span>
      </header>

      {tasks.length === 0 ? (
        <div className="trash-empty">
          <Trash2 size={30} />
          <h2>回收站是空的</h2>
          <p>从任务列表右键选择“移入回收站”后，任务会出现在这里。</p>
        </div>
      ) : (
        <div className="trash-list" role="list">
          {tasks.map((task) => (
            <article className="trash-card" role="listitem" key={task.id}>
              <div className="trash-card-main">
                <div className="trash-card-heading">
                  <strong>{task.title}</strong>
                  <span className={`status-chip status-${task.status}`}>
                    {STATUS_META[task.status].label}
                  </span>
                </div>
                <p>{task.summary || "暂无任务说明"}</p>
                <div className="trash-card-meta">
                  <span>{task.projectName || "项目待确认"}</span>
                  <span>优先级：{PRIORITY_LABEL[task.priority]}</span>
                  <time dateTime={task.trashedAt}>
                    删除于 {formatTrashedAt(task.trashedAt)}
                  </time>
                </div>
              </div>
              <div className="trash-card-actions">
                <button
                  type="button"
                  className="button secondary compact"
                  onClick={() => onRestore(task.id)}
                  aria-label={`恢复任务 ${task.title}`}
                >
                  <RotateCcw size={14} />
                  恢复
                </button>
                <button
                  type="button"
                  className="button danger-button compact"
                  onClick={() => onDeletePermanently(task.id)}
                  aria-label={`永久删除任务 ${task.title}`}
                >
                  <Trash2 size={14} />
                  永久删除
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
