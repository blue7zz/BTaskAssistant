import { useEffect, useMemo, useState } from "react";
import {
  Archive,
  Bot,
  CheckCircle2,
  ChevronRight,
  ClipboardList,
  FilePlus2,
  Inbox,
  MessageSquareText,
  Plus,
  Search,
  ShieldCheck,
  Sparkles,
  Wrench,
  X,
} from "lucide-react";
import { TaskComposer, type ComposerMode } from "./components/TaskComposer";
import { TaskDetail } from "./components/TaskDetail";
import { TaskList } from "./components/TaskList";
import { STATUS_META, TASK_STATUSES, type TaskStatus } from "./domain/task";
import { getEngineStatuses, type EngineStatus } from "./lib/bridge";
import {
  useWorkspaceStore,
  type StatusFilter,
} from "./store/workspace";

const NAV_ICONS: Record<TaskStatus, typeof Inbox> = {
  inbox: Inbox,
  requirements: Sparkles,
  approved: ShieldCheck,
  development: Wrench,
  review: ClipboardList,
  done: CheckCircle2,
};

type Notice = { kind: "success" | "error"; message: string };

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export default function App() {
  const tasks = useWorkspaceStore((state) => state.tasks);
  const selectedTaskID = useWorkspaceStore((state) => state.selectedTaskId);
  const statusFilter = useWorkspaceStore((state) => state.statusFilter);
  const hydrated = useWorkspaceStore((state) => state.hydrated);
  const selectTask = useWorkspaceStore((state) => state.selectTask);
  const setStatusFilter = useWorkspaceStore((state) => state.setStatusFilter);
  const [composerOpen, setComposerOpen] = useState(false);
  const [composerMode, setComposerMode] = useState<ComposerMode>("task");
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState<Notice>();
  const [engines, setEngines] = useState<EngineStatus[]>([]);

  useEffect(() => {
    getEngineStatuses().then(setEngines).catch(() => setEngines([]));
  }, []);

  useEffect(() => {
    if (!notice) return;
    const timeout = window.setTimeout(() => setNotice(undefined), 3600);
    return () => window.clearTimeout(timeout);
  }, [notice]);

  useEffect(() => {
    if (!hydrated || tasks.length === 0) return;
    if (!selectedTaskID || !tasks.some((task) => task.id === selectedTaskID)) {
      selectTask(tasks[0].id);
    }
  }, [hydrated, selectTask, selectedTaskID, tasks]);

  const selectedTask = tasks.find((task) => task.id === selectedTaskID);
  const filteredTasks = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    return tasks.filter((task) => {
      const matchesStatus =
        statusFilter === "all" || task.status === statusFilter;
      const matchesQuery =
        !normalizedQuery ||
        [task.title, task.summary, task.projectName].some((value) =>
          value.toLocaleLowerCase().includes(normalizedQuery),
        );
      return matchesStatus && matchesQuery;
    });
  }, [query, statusFilter, tasks]);

  const openComposer = (mode: ComposerMode) => {
    setComposerMode(mode);
    setComposerOpen(true);
  };

  const reportError = (error: unknown) =>
    setNotice({ kind: "error", message: errorMessage(error) });
  const reportSuccess = (message: string) =>
    setNotice({ kind: "success", message });

  if (!hydrated) {
    return (
      <div className="loading-screen">
        <div className="brand-mark large">
          <Archive size={26} />
        </div>
        <strong>正在加载本地任务…</strong>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <nav className="sidebar">
        <div className="brand">
          <div className="brand-mark">
            <Archive size={20} />
          </div>
          <div>
            <strong>BTask</strong>
            <span>Assistant</span>
          </div>
        </div>

        <button
          type="button"
          className="new-task-button"
          onClick={() => openComposer("task")}
        >
          <Plus size={17} />
          新建任务
          <ChevronRight size={15} />
        </button>

        <div className="nav-section">
          <span className="nav-label">工作流</span>
          <button
            className={`nav-item ${statusFilter === "all" ? "active" : ""}`}
            onClick={() => setStatusFilter("all")}
          >
            <Archive size={16} />
            全部任务
            <span>{tasks.length}</span>
          </button>
          {TASK_STATUSES.map((status) => {
            const Icon = NAV_ICONS[status];
            const count = tasks.filter((task) => task.status === status).length;
            return (
              <button
                key={status}
                className={`nav-item ${statusFilter === status ? "active" : ""}`}
                onClick={() => setStatusFilter(status)}
              >
                <Icon size={16} />
                {STATUS_META[status].label}
                <span>{count}</span>
              </button>
            );
          })}
        </div>

        <div className="engine-card">
          <div className="engine-card-title">
            <Bot size={16} />
            引擎边界
          </div>
          {engines.map((engine) => (
            <div className="engine-row" key={engine.id}>
              <span
                className={`status-dot ${engine.configured ? "online" : ""}`}
              />
              <div>
                <strong>{engine.label}</strong>
                <small>{engine.configured ? "已配置" : "待配置"}</small>
              </div>
            </div>
          ))}
          <p>当前版本不会自行调用或替你做产品决策。</p>
        </div>
      </nav>

      <main className="main-shell">
        <header className="topbar">
          <div>
            <span className="eyebrow">本地任务工作台</span>
            <h1>把需求确认权留在人手里</h1>
          </div>
          <div className="topbar-actions">
            <label className="search-box">
              <Search size={16} />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="搜索任务或项目"
              />
            </label>
            <button
              type="button"
              className="button secondary"
              onClick={() => openComposer("chat")}
            >
              <MessageSquareText size={16} />
              导入聊天
            </button>
            <button
              type="button"
              className="button primary"
              onClick={() => openComposer("task")}
            >
              <FilePlus2 size={16} />
              添加任务
            </button>
          </div>
        </header>

        {tasks.length === 0 ? (
          <section className="empty-workspace">
            <div className="empty-illustration">
              <ClipboardList size={34} />
              <span />
              <span />
              <span />
            </div>
            <span className="eyebrow">从一个真实任务开始</span>
            <h2>任务池还是空的</h2>
            <p>
              手动添加任务，或把已有聊天记录完整导入。系统只整理你提供的事实，
              不会自动补写需求。
            </p>
            <div>
              <button
                className="button primary"
                onClick={() => openComposer("task")}
              >
                <Plus size={16} />
                添加第一个任务
              </button>
              <button
                className="button secondary"
                onClick={() => openComposer("chat")}
              >
                <MessageSquareText size={16} />
                导入聊天记录
              </button>
            </div>
          </section>
        ) : (
          <div className="workspace-grid">
            <TaskList
              tasks={filteredTasks}
              selectedTaskID={selectedTaskID}
              onSelect={selectTask}
            />
            {selectedTask ? (
              <TaskDetail
                task={selectedTask}
                engines={engines}
                onError={reportError}
                onSuccess={reportSuccess}
              />
            ) : (
              <section className="no-selection">
                <ClipboardList size={28} />
                <h2>选择一个任务</h2>
                <p>从左侧任务列表进入工作台。</p>
              </section>
            )}
          </div>
        )}
      </main>

      <TaskComposer
        open={composerOpen}
        initialMode={composerMode}
        onClose={() => setComposerOpen(false)}
        onSuccess={reportSuccess}
        onError={reportError}
      />

      {notice && (
        <div className={`notice notice-${notice.kind}`} role="status">
          {notice.kind === "success" ? (
            <CheckCircle2 size={17} />
          ) : (
            <X size={17} />
          )}
          <span>{notice.message}</span>
          <button onClick={() => setNotice(undefined)} aria-label="关闭提示">
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  );
}

