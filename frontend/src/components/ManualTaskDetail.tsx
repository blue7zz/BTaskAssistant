import {
  CalendarClock,
  FileText,
  Pencil,
  Tag,
} from "lucide-react";
import {
  STATUS_META,
  type Task,
  type TaskPriority,
} from "../domain/task";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";

interface ManualTaskDetailProps {
  task: Task;
  onEdit(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

const PRIORITY_LABEL: Record<TaskPriority, string> = {
  low: "低",
  medium: "中",
  high: "高",
};

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function ManualTaskDetail({
  task,
  onEdit,
  onSuccess,
  onError,
}: ManualTaskDetailProps) {
  const updateTaskRecord = useWorkspaceStore((state) => state.updateTaskRecord);

  const run = (action: () => void, message: string) => {
    try {
      action();
      onSuccess(message);
    } catch (error) {
      onError(error);
    }
  };

  return (
    <section className="manual-task-detail">
      <header className="manual-task-header">
        <div>
          <span className="eyebrow">纯手工记录</span>
          <h2>{task.title}</h2>
          <p>按项目分类和状态整理，正文内容完全由你填写。</p>
        </div>
        <div className="manual-task-meta">
          <div className="manual-task-meta-row">
            <span className={`status-chip status-${task.status}`}>
              {STATUS_META[task.status].label}
            </span>
            <span
              className={`manual-priority-chip priority-${task.priority}`}
            >
              <Tag size={12} />
              优先级：{PRIORITY_LABEL[task.priority]}
            </span>
            <button
              type="button"
              className="button secondary compact manual-edit-button"
              onClick={onEdit}
              aria-label="编辑任务基本信息"
            >
              <Pencil size={13} />
              编辑
            </button>
          </div>
          <span className="manual-updated-at">
            <CalendarClock size={13} />
            更新于 {formatDate(task.updatedAt)}
          </span>
        </div>
      </header>

      <div className="manual-task-scroll">
        <section className="manual-record-card manual-text-card">
          <header>
            <FileText size={18} />
            <div>
              <h3>文本记录</h3>
              <p>可以写需求、进展、结论、链接或后续待办。</p>
            </div>
          </header>
          <div className="manual-editor-wrap">
            <LazyRichMarkdownEditor
              key={task.id}
              value={task.summary}
              placeholder="在这里手动填写任务记录……"
              onCommit={(summary) =>
                run(
                  () => updateTaskRecord(task.id, { summary }),
                  "文本记录已保存",
                )
              }
            />
          </div>
        </section>

        {task.evidence.length > 0 && (
          <details className="manual-source-records">
            <summary>查看原始记录（{task.evidence.length}）</summary>
            <div>
              {task.evidence.map((source) => (
                <article key={source.id}>
                  <header>
                    <strong>{source.title}</strong>
                    <span>{source.type}</span>
                  </header>
                  <p>{source.content}</p>
                </article>
              ))}
            </div>
          </details>
        )}
      </div>
    </section>
  );
}
