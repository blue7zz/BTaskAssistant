import {
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  Archive,
  CheckCircle2,
  ChevronRight,
  ClipboardList,
  CloudDownload,
  Inbox,
  MessageSquareText,
  Plus,
  Settings,
  ShieldCheck,
  Sparkles,
  Trash2,
  Wrench,
  X,
} from "lucide-react";
import { TaskComposer, type ComposerMode } from "./components/TaskComposer";
import { TaskBasicInfoDialog } from "./components/TaskBasicInfoDialog";
import { TaskContextMenu } from "./components/TaskContextMenu";
import { ManualTaskDetail } from "./components/ManualTaskDetail";
import { TaskList } from "./components/TaskList";
import { PlaneCollector } from "./components/PlaneCollector";
import { SettingsPage } from "./components/SettingsPage";
import { TrashView } from "./components/TrashView";
import { STATUS_META, TASK_STATUSES, type TaskStatus } from "./domain/task";
import {
  getEngineStatuses,
  hasPlaneToken,
  testPlaneConnection,
  type EngineStatus,
} from "./lib/bridge";
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
type WorkspaceView = "tasks" | "plane" | "trash" | "settings";
type ContentView = Exclude<WorkspaceView, "settings">;
type ResizablePanel = "sidebar" | "task-list";

interface TaskMenuState {
  taskID: string;
  x: number;
  y: number;
}

interface ActiveResize {
  panel: ResizablePanel;
  startX: number;
  startWidth: number;
  minWidth: number;
  maxWidth: number;
}

const PANEL_WIDTHS = {
  sidebar: { default: 228, compact: 204, min: 180, max: 360 },
  "task-list": { default: 306, compact: 278, min: 240, max: 520 },
} as const;
const COMPACT_LAYOUT_BREAKPOINT = 1180;
const COLLAPSED_TASK_LIST_WIDTH = 58;
const MIN_DETAIL_WIDTH = 420;

function initialPanelWidth(panel: ResizablePanel): number {
  const widths = PANEL_WIDTHS[panel];
  if (
    typeof window !== "undefined" &&
    window.innerWidth <= COMPACT_LAYOUT_BREAKPOINT
  ) {
    return widths.compact;
  }
  return widths.default;
}

function clampWidth(value: number, minimum: number, maximum: number): number {
  return Math.min(Math.max(value, minimum), maximum);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export default function App() {
  const tasks = useWorkspaceStore((state) => state.tasks);
  const trashedTasks = useWorkspaceStore((state) => state.trashedTasks);
  const collectionCandidates = useWorkspaceStore(
    (state) => state.collectionCandidates,
  );
  const planeSettings = useWorkspaceStore((state) => state.planeSettings);
  const selectedTaskID = useWorkspaceStore((state) => state.selectedTaskId);
  const statusFilter = useWorkspaceStore((state) => state.statusFilter);
  const hydrated = useWorkspaceStore((state) => state.hydrated);
  const selectTask = useWorkspaceStore((state) => state.selectTask);
  const setStatusFilter = useWorkspaceStore((state) => state.setStatusFilter);
  const moveTaskToTrash = useWorkspaceStore(
    (state) => state.moveTaskToTrash,
  );
  const restoreTask = useWorkspaceStore((state) => state.restoreTask);
  const deleteTaskPermanently = useWorkspaceStore(
    (state) => state.deleteTaskPermanently,
  );
  const [composerOpen, setComposerOpen] = useState(false);
  const [composerMode, setComposerMode] = useState<ComposerMode>("task");
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState<Notice>();
  const [engines, setEngines] = useState<EngineStatus[]>([]);
  const [taskListCollapsed, setTaskListCollapsed] = useState(false);
  const [activeView, setActiveView] = useState<WorkspaceView>("tasks");
  const [settingsReturnView, setSettingsReturnView] =
    useState<ContentView>("tasks");
  const [planeAvailable, setPlaneAvailable] = useState(false);
  const [planeConnectionChecked, setPlaneConnectionChecked] = useState(false);
  const [planeConnectionRevision, setPlaneConnectionRevision] = useState(0);
  const [sidebarWidth, setSidebarWidth] = useState(() =>
    initialPanelWidth("sidebar"),
  );
  const [taskListWidth, setTaskListWidth] = useState(() =>
    initialPanelWidth("task-list"),
  );
  const [activeResize, setActiveResize] = useState<ActiveResize>();
  const [taskMenu, setTaskMenu] = useState<TaskMenuState>();
  const [editingTaskID, setEditingTaskID] = useState<string>();

  useEffect(() => {
    let active = true;
    getEngineStatuses()
      .then((statuses) => {
        if (active) setEngines(statuses);
      })
      .catch(() => {
        if (active) setEngines([]);
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    setPlaneAvailable(false);
    setPlaneConnectionChecked(false);
    const configured = Boolean(
      planeSettings.baseUrl.trim() &&
        planeSettings.workspaceSlug.trim() &&
        planeSettings.projectId.trim(),
    );
    if (!configured) {
      setPlaneAvailable(false);
      setPlaneConnectionChecked(true);
      return;
    }
    const checkConnection = async () => {
      try {
        const stored = await hasPlaneToken(planeSettings);
        if (!stored) {
          if (active) setPlaneAvailable(false);
          return;
        }
        const result = await testPlaneConnection(planeSettings);
        if (active) setPlaneAvailable(result.connected);
      } catch {
        if (active) setPlaneAvailable(false);
      } finally {
        if (active) setPlaneConnectionChecked(true);
      }
    };
    void checkConnection();
    return () => {
      active = false;
    };
  }, [
    planeConnectionRevision,
    planeSettings.baseUrl,
    planeSettings.projectId,
    planeSettings.workspaceSlug,
  ]);

  useEffect(() => {
    if (
      activeView === "plane" &&
      planeConnectionChecked &&
      !planeAvailable
    ) {
      setActiveView("tasks");
    }
  }, [activeView, planeAvailable, planeConnectionChecked]);

  useEffect(() => {
    if (!notice) return;
    const timeout = window.setTimeout(() => setNotice(undefined), 3600);
    return () => window.clearTimeout(timeout);
  }, [notice]);

  useEffect(() => {
    setTaskMenu(undefined);
  }, [activeResize, activeView, taskListCollapsed]);

  useEffect(() => {
    if (!activeResize) return;

    const handlePointerMove = (event: PointerEvent) => {
      const nextWidth = clampWidth(
        activeResize.startWidth + event.clientX - activeResize.startX,
        activeResize.minWidth,
        activeResize.maxWidth,
      );
      if (activeResize.panel === "sidebar") {
        setSidebarWidth(nextWidth);
      } else {
        setTaskListWidth(nextWidth);
      }
    };
    const stopResize = () => setActiveResize(undefined);

    document.body.classList.add("panel-resizing");
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", stopResize);
    window.addEventListener("pointercancel", stopResize);
    return () => {
      document.body.classList.remove("panel-resizing");
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", stopResize);
      window.removeEventListener("pointercancel", stopResize);
    };
  }, [activeResize]);

  useEffect(() => {
    const adaptPanelWidths = () => {
      let nextSidebarWidth = clampWidth(
        sidebarWidth,
        PANEL_WIDTHS.sidebar.min,
        PANEL_WIDTHS.sidebar.max,
      );
      let nextTaskListWidth = clampWidth(
        taskListWidth,
        PANEL_WIDTHS["task-list"].min,
        PANEL_WIDTHS["task-list"].max,
      );
      const taskListVisible = activeView === "tasks" && tasks.length > 0;

      if (!taskListVisible) {
        nextSidebarWidth = Math.min(
          nextSidebarWidth,
          Math.max(
            PANEL_WIDTHS.sidebar.min,
            window.innerWidth - MIN_DETAIL_WIDTH,
          ),
        );
      } else if (taskListCollapsed) {
        nextSidebarWidth = Math.min(
          nextSidebarWidth,
          Math.max(
            PANEL_WIDTHS.sidebar.min,
            window.innerWidth -
              COLLAPSED_TASK_LIST_WIDTH -
              MIN_DETAIL_WIDTH,
          ),
        );
      } else {
        let overflow =
          nextSidebarWidth +
          nextTaskListWidth -
          (window.innerWidth - MIN_DETAIL_WIDTH);
        if (overflow > 0) {
          const taskListReduction = Math.min(
            overflow,
            nextTaskListWidth - PANEL_WIDTHS["task-list"].min,
          );
          nextTaskListWidth -= taskListReduction;
          overflow -= taskListReduction;
          nextSidebarWidth = Math.max(
            PANEL_WIDTHS.sidebar.min,
            nextSidebarWidth - overflow,
          );
        }
      }

      if (nextSidebarWidth !== sidebarWidth) {
        setSidebarWidth(nextSidebarWidth);
      }
      if (nextTaskListWidth !== taskListWidth) {
        setTaskListWidth(nextTaskListWidth);
      }
    };

    window.addEventListener("resize", adaptPanelWidths);
    return () => window.removeEventListener("resize", adaptPanelWidths);
  }, [
    activeView,
    sidebarWidth,
    taskListCollapsed,
    taskListWidth,
    tasks.length,
  ]);

  useEffect(() => {
    if (!hydrated || tasks.length === 0) return;
    if (!selectedTaskID || !tasks.some((task) => task.id === selectedTaskID)) {
      selectTask(tasks[0].id);
    }
  }, [hydrated, selectTask, selectedTaskID, tasks]);

  const selectedTask = tasks.find((task) => task.id === selectedTaskID);
  const menuTask = tasks.find((task) => task.id === taskMenu?.taskID);
  const editingTask = tasks.find((task) => task.id === editingTaskID);
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
    setActiveView("tasks");
    setComposerMode(mode);
    setComposerOpen(true);
  };

  const openSettings = () => {
    if (activeView !== "settings") setSettingsReturnView(activeView);
    setActiveView("settings");
  };

  const planeConfigured = Boolean(
    planeSettings.baseUrl.trim() &&
      planeSettings.workspaceSlug.trim() &&
      planeSettings.projectId.trim(),
  );
  const showPlaneSource =
    planeConfigured &&
    planeAvailable &&
    planeSettings.showInTaskSources;

  const closeSettings = () => {
    setActiveView(
      settingsReturnView === "plane" && !showPlaneSource
        ? "tasks"
        : settingsReturnView,
    );
  };

  const reportError = (error: unknown) =>
    setNotice({ kind: "error", message: errorMessage(error) });
  const reportSuccess = (message: string) =>
    setNotice({ kind: "success", message });

  const runTaskAction = (action: () => void, message: string): boolean => {
    try {
      action();
      reportSuccess(message);
      return true;
    } catch (error) {
      reportError(error);
      return false;
    }
  };

  const handleMoveToTrash = (taskID: string) => {
    setTaskMenu(undefined);
    runTaskAction(() => moveTaskToTrash(taskID), "任务已移入回收站");
  };

  const handleRestoreTask = (taskID: string) => {
    if (runTaskAction(() => restoreTask(taskID), "任务已恢复")) {
      setActiveView("tasks");
    }
  };

  const handleDeletePermanently = (taskID: string) => {
    const task = trashedTasks.find((candidate) => candidate.id === taskID);
    if (!task) return;
    if (!window.confirm(`确定永久删除“${task.title}”吗？此操作无法撤销。`)) {
      return;
    }
    runTaskAction(() => deleteTaskPermanently(taskID), "任务已永久删除");
  };

  const maxPanelWidth = (panel: ResizablePanel) => {
    const visibleTaskListWidth =
      activeView === "tasks" && tasks.length > 0
        ? taskListCollapsed
          ? COLLAPSED_TASK_LIST_WIDTH
          : taskListWidth
        : 0;
    const occupiedWidth =
      panel === "sidebar" ? visibleTaskListWidth : sidebarWidth;
    return Math.max(
      PANEL_WIDTHS[panel].min,
      Math.min(
        PANEL_WIDTHS[panel].max,
        window.innerWidth - occupiedWidth - MIN_DETAIL_WIDTH,
      ),
    );
  };

  const startPanelResize = (
    panel: ResizablePanel,
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
    if (event.button !== 0) return;
    event.preventDefault();
    setActiveResize({
      panel,
      startX: event.clientX,
      startWidth: panel === "sidebar" ? sidebarWidth : taskListWidth,
      minWidth: PANEL_WIDTHS[panel].min,
      maxWidth: maxPanelWidth(panel),
    });
  };

  const resizePanelWithKeyboard = (
    panel: ResizablePanel,
    event: ReactKeyboardEvent<HTMLDivElement>,
  ) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const change = event.key === "ArrowRight" ? 8 : -8;
    const limits = PANEL_WIDTHS[panel];
    const maximum = maxPanelWidth(panel);
    if (panel === "sidebar") {
      setSidebarWidth((width) =>
        clampWidth(width + change, limits.min, maximum),
      );
    } else {
      setTaskListWidth((width) =>
        clampWidth(width + change, limits.min, maximum),
      );
    }
  };

  const layoutStyle = {
    "--sidebar-width": `${sidebarWidth}px`,
    "--task-list-width": `${taskListWidth}px`,
  } as CSSProperties;

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
    <div className="app-shell" style={layoutStyle}>
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
            className={`nav-item ${
              activeView === "tasks" && statusFilter === "all" ? "active" : ""
            }`}
            onClick={() => {
              setActiveView("tasks");
              setStatusFilter("all");
            }}
          >
            <Archive size={16} />
            <span className="nav-item-label">全部任务</span>
            <span className="nav-count">{tasks.length}</span>
          </button>
          {TASK_STATUSES.map((status) => {
            const Icon = NAV_ICONS[status];
            const count = tasks.filter((task) => task.status === status).length;
            return (
              <button
                key={status}
                className={`nav-item ${
                  activeView === "tasks" && statusFilter === status
                    ? "active"
                    : ""
                }`}
                onClick={() => {
                  setActiveView("tasks");
                  setStatusFilter(status);
                }}
              >
                <Icon size={16} />
                <span className="nav-item-label">
                  {STATUS_META[status].label}
                </span>
                <span className="nav-count">{count}</span>
              </button>
            );
          })}
          <button
            type="button"
            className={`nav-item ${activeView === "trash" ? "active" : ""}`}
            onClick={() => setActiveView("trash")}
            aria-label="打开回收站"
          >
            <Trash2 size={16} />
            <span className="nav-item-label">回收站</span>
            <span className="nav-count">{trashedTasks.length}</span>
          </button>
        </div>

        {showPlaneSource && (
          <div className="nav-section integrations-nav">
            <span className="nav-label">任务来源</span>
            <button
              className={`nav-item ${activeView === "plane" ? "active" : ""}`}
              onClick={() => setActiveView("plane")}
            >
              <CloudDownload size={16} />
              <span className="nav-item-label">Plane 收集箱</span>
              <span className="nav-count">
                {
                  collectionCandidates.filter(
                    (candidate) => candidate.decision === "pending",
                  ).length
                }
              </span>
            </button>
          </div>
        )}

        <div className="sidebar-footer">
          <button
            type="button"
            className={`nav-item ${activeView === "settings" ? "active" : ""}`}
            onClick={openSettings}
            aria-label="打开设置"
          >
            <Settings size={16} />
            <span className="nav-item-label">设置</span>
          </button>
        </div>

        <div
          className={`panel-resizer sidebar-resizer ${
            activeResize?.panel === "sidebar" ? "active" : ""
          }`}
          role="separator"
          aria-label="调整导航栏宽度"
          aria-orientation="vertical"
          aria-valuemin={PANEL_WIDTHS.sidebar.min}
          aria-valuemax={maxPanelWidth("sidebar")}
          aria-valuenow={sidebarWidth}
          tabIndex={0}
          onPointerDown={(event) => startPanelResize("sidebar", event)}
          onKeyDown={(event) => resizePanelWithKeyboard("sidebar", event)}
        />
      </nav>

      <main className="main-shell">
        {activeView === "settings" ? (
          <SettingsPage
            engine={engines.find((engine) => engine.id === "pi")}
            planeConnected={planeAvailable}
            onBack={closeSettings}
            onPlaneConnectionChange={() =>
              setPlaneConnectionRevision((revision) => revision + 1)
            }
            onSuccess={reportSuccess}
            onError={reportError}
          />
        ) : activeView === "plane" ? (
          <PlaneCollector
            connected={planeAvailable}
            onError={reportError}
            onSuccess={reportSuccess}
            onOpenTask={(taskID) => {
              if (tasks.some((task) => task.id === taskID)) {
                selectTask(taskID);
                setStatusFilter("all");
                setActiveView("tasks");
              } else if (
                trashedTasks.some((task) => task.id === taskID)
              ) {
                setActiveView("trash");
                reportSuccess("该任务当前位于回收站");
              } else {
                reportError(new Error("没有找到这个任务"));
              }
            }}
          />
        ) : activeView === "trash" ? (
          <TrashView
            tasks={trashedTasks}
            onRestore={handleRestoreTask}
            onDeletePermanently={handleDeletePermanently}
          />
        ) : tasks.length === 0 ? (
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
              手动添加任务，或把已有聊天记录完整导入。所有内容都由你填写，
              系统只负责本地保存和分类。
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
          <div
            className={`workspace-grid ${
              taskListCollapsed ? "task-list-collapsed" : ""
            }`}
          >
            <TaskList
              tasks={filteredTasks}
              selectedTaskID={selectedTaskID}
              collapsed={taskListCollapsed}
              searchQuery={query}
              onSelect={selectTask}
              onOpenContextMenu={(taskID, x, y) =>
                setTaskMenu({ taskID, x, y })
              }
              onSearchQueryChange={setQuery}
              onToggleCollapsed={() =>
                setTaskListCollapsed((collapsed) => !collapsed)
              }
            />
            {!taskListCollapsed && (
              <div
                className={`panel-resizer task-list-resizer ${
                  activeResize?.panel === "task-list" ? "active" : ""
                }`}
                role="separator"
                aria-label="调整任务列表宽度"
                aria-orientation="vertical"
                aria-valuemin={PANEL_WIDTHS["task-list"].min}
                aria-valuemax={maxPanelWidth("task-list")}
                aria-valuenow={taskListWidth}
                tabIndex={0}
                onPointerDown={(event) =>
                  startPanelResize("task-list", event)
                }
                onKeyDown={(event) =>
                  resizePanelWithKeyboard("task-list", event)
                }
              />
            )}
            {selectedTask ? (
              <ManualTaskDetail
                task={selectedTask}
                onEdit={() => setEditingTaskID(selectedTask.id)}
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

      {menuTask && taskMenu && (
        <TaskContextMenu
          x={taskMenu.x}
          y={taskMenu.y}
          taskTitle={menuTask.title}
          onEdit={() => {
            setTaskMenu(undefined);
            setEditingTaskID(menuTask.id);
          }}
          onMoveToTrash={() => handleMoveToTrash(menuTask.id)}
          onClose={() => setTaskMenu(undefined)}
        />
      )}

      {editingTask && (
        <TaskBasicInfoDialog
          task={editingTask}
          onClose={() => setEditingTaskID(undefined)}
          onSuccess={reportSuccess}
          onError={reportError}
        />
      )}

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
