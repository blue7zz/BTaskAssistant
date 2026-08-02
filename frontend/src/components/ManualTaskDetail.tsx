import {
  Bot,
  CalendarClock,
  FileText,
  Pencil,
  SquareTerminal,
  Tag,
} from "lucide-react";
import { useState } from "react";
import {
  STATUS_META,
  type Task,
  type TaskPriority,
} from "../domain/task";
import { useWorkspaceStore } from "../store/workspace";
import { LazyRichMarkdownEditor } from "./LazyRichMarkdownEditor";
import { ReasonixPage } from "./ReasonixPage";
import { TaskAgentWorkbench } from "./TaskAgentWorkbench";

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
  const [activeView, setActiveView] = useState<"record" | "pi" | "rx">("pi");
  const [rxOpened, setRXOpened] = useState(false);
  const [piOpened, setPIOpened] = useState(true);

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
      {activeView === "record" && <header className="manual-task-header">
        <div>
          <span className="eyebrow">
            {activeView === "record" ? "纯手工记录" : "原生 PI 会话"}
          </span>
          <h2>{task.title}</h2>
          <p>
            {activeView === "record"
              ? "按项目分类和状态整理，正文内容完全由你填写。"
              : "与当前任务绑定的独立 Session；本阶段不开放任何工具。"}
          </p>
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
      </header>}

      <nav className="manual-task-tabs" role="tablist" aria-label="任务工作台">
        <button
          type="button"
          role="tab"
          aria-selected={activeView === "record"}
          className={activeView === "record" ? "active" : ""}
          onClick={() => setActiveView("record")}
        >
          <FileText size={14} />
          记录
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeView === "pi"}
          className={activeView === "pi" ? "active" : ""}
          onClick={() => {
            setPIOpened(true);
            setActiveView("pi");
          }}
        >
          <Bot size={14} />
          PI
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeView === "rx"}
          className={activeView === "rx" ? "active" : ""}
          onClick={() => {
            setRXOpened(true);
            setActiveView("rx");
          }}
        >
          <SquareTerminal size={14} />
          RX
        </button>
      </nav>

      <div
        className="manual-rx-pane"
        role="tabpanel"
        hidden={activeView !== "rx"}
      >
        {rxOpened && <ReasonixPage taskId={task.id} workspaceRoot="" taskTitle={task.title} />}
      </div>

      <div
        className="manual-task-scroll"
        role="tabpanel"
        hidden={activeView !== "record"}
      >
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
      {piOpened && (
        <div
          className="manual-pi-pane"
          role="tabpanel"
          hidden={activeView !== "pi"}
        >
          <TaskAgentWorkbench
            key={task.id}
            taskId={task.id}
            taskTitle={task.title}
            onEditTask={onEdit}
          />
        </div>
      )}
    </section>
  );
}
