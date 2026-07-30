import {
  Check,
  CircleDot,
  FolderGit2,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
} from "lucide-react";
import {
  STATUS_META,
  type Task,
  type TaskPriority,
} from "../domain/task";
import type {
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
} from "react";

interface TaskListProps {
  tasks: Task[];
  selectedTaskID?: string;
  collapsed: boolean;
  searchQuery: string;
  onSelect(taskID: string): void;
  onOpenContextMenu(taskID: string, x: number, y: number): void;
  onSearchQueryChange(query: string): void;
  onToggleCollapsed(): void;
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
  collapsed,
  searchQuery,
  onSelect,
  onOpenContextMenu,
  onSearchQueryChange,
  onToggleCollapsed,
}: TaskListProps) {
  const openContextMenu = (
    taskID: string,
    event: ReactMouseEvent<HTMLButtonElement>,
  ) => {
    event.preventDefault();
    onSelect(taskID);
    onOpenContextMenu(taskID, event.clientX, event.clientY);
  };

  const openContextMenuWithKeyboard = (
    taskID: string,
    event: ReactKeyboardEvent<HTMLButtonElement>,
  ) => {
    if (
      event.key !== "ContextMenu" &&
      !(event.shiftKey && event.key === "F10")
    ) {
      return;
    }
    event.preventDefault();
    const bounds = event.currentTarget.getBoundingClientRect();
    onSelect(taskID);
    onOpenContextMenu(taskID, bounds.left + 16, bounds.top + 16);
  };

  return (
    <aside className={`task-list-panel ${collapsed ? "collapsed" : ""}`}>
      <div className="task-list-heading">
        {!collapsed && (
          <div>
            <span className="eyebrow">当前视图</span>
            <h2>任务</h2>
          </div>
        )}
        <div className="task-list-heading-actions">
          {!collapsed && <span className="count-pill">{tasks.length}</span>}
          <button
            type="button"
            className="task-list-toggle"
            aria-label={collapsed ? "展开任务列表" : "收起任务列表"}
            title={collapsed ? "展开任务列表" : "收起任务列表"}
            onClick={onToggleCollapsed}
          >
            {collapsed ? (
              <PanelLeftOpen size={16} />
            ) : (
              <PanelLeftClose size={16} />
            )}
          </button>
        </div>
      </div>

      {!collapsed && (
        <label className="search-box task-list-search">
          <Search size={16} />
          <input
            value={searchQuery}
            onChange={(event) => onSearchQueryChange(event.target.value)}
            placeholder="搜索任务或项目"
          />
        </label>
      )}

      <div className="task-list" role="list" aria-hidden={collapsed}>
        {tasks.map((task) => (
          <button
            type="button"
            role="listitem"
            key={task.id}
            className={`task-card ${selectedTaskID === task.id ? "selected" : ""}`}
            onClick={() => onSelect(task.id)}
            onContextMenu={(event) => openContextMenu(task.id, event)}
            onKeyDown={(event) => openContextMenuWithKeyboard(task.id, event)}
            aria-haspopup="menu"
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
