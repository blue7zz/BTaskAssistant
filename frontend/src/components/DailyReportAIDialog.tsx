import {
  AlertTriangle,
  ArrowLeft,
  Check,
  FolderOpen,
  FolderGit2,
  ListChecks,
  LoaderCircle,
  Pencil,
  Plus,
  Sparkles,
  X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  buildDailyReportMarkdown,
  dailyReportGenerationToDraft,
  isDailyReportDraftEmpty,
  localDateString,
  type DailyReportGenerationResult,
  type DailyReportProjectHistoryItem,
  type DailyReportWorkflowTask,
} from "../domain/report";
import { STATUS_META } from "../domain/task";
import {
  dailyReportAIAvailable,
  dailyReportDirectoryPickerAvailable,
  generateDailyReport,
  selectDailyReportProjectDirectory,
} from "../lib/bridge";
import { useWorkspaceStore } from "../store/workspace";

interface DailyReportAIDialogProps {
  open: boolean;
  onClose(): void;
  onSuccess(message: string): void;
  onError(error: unknown): void;
}

const MAX_SELECTED_PROJECTS = 8;
const MAX_SELECTED_TASKS = 30;

function normalizedPath(value: string): string {
  const trimmed = value.trim();
  if (trimmed === "/" || /^[A-Za-z]:[\\/]?$/.test(trimmed)) return trimmed;
  return trimmed.replace(/[\\/]+$/, "");
}

function isAbsolutePath(path: string): boolean {
  return (
    path.startsWith("/") ||
    /^[A-Za-z]:[\\/]/.test(path) ||
    path.startsWith("\\\\")
  );
}

function pathName(path: string): string {
  const parts = normalizedPath(path).split(/[\\/]/).filter(Boolean);
  return parts[parts.length - 1] ?? "本地项目";
}

function mergeProjectCandidates(
  history: DailyReportProjectHistoryItem[],
  tasks: ReturnType<typeof useWorkspaceStore.getState>["tasks"],
  local: DailyReportProjectHistoryItem[],
): DailyReportProjectHistoryItem[] {
  const byPath = new Map<string, DailyReportProjectHistoryItem>();
  for (const item of [...history, ...local]) {
    const path = normalizedPath(item.path);
    if (!path) continue;
    byPath.set(path, { ...item, path });
  }
  for (const task of tasks) {
    const path = normalizedPath(task.projectPath ?? "");
    if (!path || byPath.has(path)) continue;
    byPath.set(path, {
      id: `task-project-${task.id}`,
      projectNo: "",
      projectName: task.projectName.trim() || pathName(path),
      path,
      lastUsedAt: "",
    });
  }
  return [...byPath.values()];
}

function taskMatchesReportDate(updatedAt: string, reportDate: string): boolean {
  const date = new Date(updatedAt);
  return !Number.isNaN(date.getTime()) && localDateString(date) === reportDate;
}

function taskUpdatedTime(updatedAt: string): string {
  return new Date(updatedAt).toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function DailyReportAIDialog({
  open,
  onClose,
  onSuccess,
  onError,
}: DailyReportAIDialogProps) {
  const tasks = useWorkspaceStore((state) => state.tasks);
  const settings = useWorkspaceStore((state) => state.dailyReportSettings);
  const aiSettings = useWorkspaceStore(
    (state) => state.dailyReportAISettings,
  );
  const piSettings = useWorkspaceStore((state) => state.piSettings);
  const reportDate = useWorkspaceStore((state) => state.dailyReportDate);
  const draft = useWorkspaceStore(
    (state) => state.dailyReportDrafts[state.dailyReportDate],
  );
  const history = useWorkspaceStore(
    (state) => state.dailyReportProjectHistory,
  );
  const rememberProjects = useWorkspaceStore(
    (state) => state.rememberDailyReportProjects,
  );
  const updateProject = useWorkspaceStore(
    (state) => state.updateDailyReportProject,
  );
  const updateDraft = useWorkspaceStore(
    (state) => state.updateDailyReportDraft,
  );
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
  const [selectedTaskIDs, setSelectedTaskIDs] = useState<string[]>([]);
  const [localProjects, setLocalProjects] = useState<
    DailyReportProjectHistoryItem[]
  >([]);
  const [newProject, setNewProject] = useState({
    projectNo: "",
    projectName: "",
    path: "",
  });
  const [editingProjectID, setEditingProjectID] = useState<string>();
  const [selectingDirectory, setSelectingDirectory] = useState(false);
  const [manualDescription, setManualDescription] = useState("");
  const [generated, setGenerated] = useState<DailyReportGenerationResult>();
  const [generating, setGenerating] = useState(false);
  const requestSequence = useRef(0);
  const candidates = useMemo(
    () => mergeProjectCandidates(history, tasks, localProjects),
    [history, localProjects, tasks],
  );
  const selectedProjects = candidates.filter((project) =>
    selectedPaths.includes(project.path),
  );
  const editableProjectIDs = useMemo(
    () => new Set([...history, ...localProjects].map((project) => project.id)),
    [history, localProjects],
  );
  const directoryPickerAvailable = dailyReportDirectoryPickerAvailable();
  const reportTasks = useMemo(
    () =>
      tasks
        .filter((task) => taskMatchesReportDate(task.updatedAt, reportDate))
        .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt)),
    [reportDate, tasks],
  );
  const selectedWorkflowTasks = reportTasks.filter((task) =>
    selectedTaskIDs.includes(task.id),
  );
  const canGenerate =
    dailyReportAIAvailable() &&
    !generating &&
    (selectedProjects.length > 0 ||
      selectedWorkflowTasks.length > 0 ||
      Boolean(manualDescription.trim()));
  const generatedDraft = generated
    ? dailyReportGenerationToDraft(generated.reportDate, generated)
    : undefined;
  const preview = generatedDraft
    ? buildDailyReportMarkdown(settings, generatedDraft)
    : "";
  const willOverwriteDraft = Boolean(
    draft && !isDailyReportDraftEmpty(draft),
  );

  useEffect(() => {
    if (!open) return;
    requestSequence.current += 1;
    setSelectedPaths([]);
    setSelectedTaskIDs([]);
    setLocalProjects([]);
    setNewProject({ projectNo: "", projectName: "", path: "" });
    setEditingProjectID(undefined);
    setSelectingDirectory(false);
    setManualDescription("");
    setGenerated(undefined);
    setGenerating(false);
  }, [open]);

  if (!open) return null;

  const close = () => {
    requestSequence.current += 1;
    setGenerating(false);
    onClose();
  };

  const cancelProjectEdit = () => {
    setEditingProjectID(undefined);
    setNewProject({ projectNo: "", projectName: "", path: "" });
  };

  const toggleProject = (path: string) => {
    if (
      !selectedPaths.includes(path) &&
      selectedPaths.length >= MAX_SELECTED_PROJECTS
    ) {
      onError(new Error(`每次最多选择 ${MAX_SELECTED_PROJECTS} 个项目`));
      return;
    }
    if (selectedPaths.includes(path)) {
      const project = candidates.find((candidate) => candidate.path === path);
      if (project?.id === editingProjectID) cancelProjectEdit();
    }
    setSelectedPaths((current) =>
      current.includes(path)
        ? current.filter((value) => value !== path)
        : [...current, path],
    );
  };

  const toggleWorkflowTask = (taskID: string) => {
    if (
      !selectedTaskIDs.includes(taskID) &&
      selectedTaskIDs.length >= MAX_SELECTED_TASKS
    ) {
      onError(new Error(`每次最多选择 ${MAX_SELECTED_TASKS} 个工作流任务`));
      return;
    }
    setSelectedTaskIDs((current) =>
      current.includes(taskID)
        ? current.filter((value) => value !== taskID)
        : [...current, taskID],
    );
  };

  const startProjectEdit = (project: DailyReportProjectHistoryItem) => {
    setEditingProjectID(project.id);
    setNewProject({
      projectNo: project.projectNo,
      projectName: project.projectName,
      path: project.path,
    });
  };

  const chooseProjectDirectory = async () => {
    if (!directoryPickerAvailable || selectingDirectory) return;
    setSelectingDirectory(true);
    try {
      const path = await selectDailyReportProjectDirectory();
      if (path.trim()) {
        setNewProject((current) => ({ ...current, path }));
      }
    } catch (error) {
      onError(error);
    } finally {
      setSelectingDirectory(false);
    }
  };

  const projectFromForm = () => {
    const path = normalizedPath(newProject.path);
    if (!path || !isAbsolutePath(path)) {
      throw new Error("请输入本地 Git 仓库的绝对路径");
    }
    return {
      projectNo: newProject.projectNo.trim(),
      projectName: newProject.projectName.trim() || pathName(path),
      path,
    };
  };

  const addProject = () => {
    try {
      const project = projectFromForm();
      const { path } = project;
      if (
        !selectedPaths.includes(path) &&
        selectedPaths.length >= MAX_SELECTED_PROJECTS
      ) {
        throw new Error(`每次最多选择 ${MAX_SELECTED_PROJECTS} 个项目`);
      }
      const item: DailyReportProjectHistoryItem = {
        id: `local-report-project-${Date.now()}`,
        ...project,
        lastUsedAt: "",
      };
      setLocalProjects((current) => [
        ...current.filter((project) => project.path !== path),
        item,
      ]);
      setSelectedPaths((current) =>
        current.includes(path) ? current : [...current, path],
      );
      setNewProject({ projectNo: "", projectName: "", path: "" });
    } catch (error) {
      onError(error);
    }
  };

  const saveProjectEdit = () => {
    const editingProject = candidates.find(
      (project) => project.id === editingProjectID,
    );
    if (!editingProject) return;
    try {
      const project = projectFromForm();
      if (
        candidates.some(
          (candidate) =>
            candidate.id !== editingProject.id &&
            normalizedPath(candidate.path) === project.path,
        )
      ) {
        throw new Error("该仓库路径已存在，请选择其他文件夹");
      }
      const localProject = localProjects.some(
        (candidate) => candidate.id === editingProject.id,
      );
      if (localProject) {
        setLocalProjects((current) =>
          current.map((candidate) =>
            candidate.id === editingProject.id
              ? { ...candidate, ...project }
              : candidate,
          ),
        );
      } else {
        updateProject(editingProject.id, project);
      }
      setSelectedPaths((current) =>
        Array.from(
          new Set(
            current.map((path) =>
              path === editingProject.path ? project.path : path,
            ),
          ),
        ),
      );
      cancelProjectEdit();
    } catch (error) {
      onError(error);
    }
  };

  const runGeneration = async () => {
    if (!settings.organization.trim()) {
      onError(new Error("请先在日报设置中填写组织"));
      return;
    }
    if (
      selectedProjects.length === 0 &&
      selectedWorkflowTasks.length === 0 &&
      !manualDescription.trim()
    ) {
      onError(
        new Error("请至少选择一个项目、工作流任务，或填写人工补充"),
      );
      return;
    }
    const sequence = requestSequence.current + 1;
    requestSequence.current = sequence;
    const requestedDate = reportDate;
    setGenerating(true);
    setGenerated(undefined);
    try {
      const result = await generateDailyReport(
        {
          reportDate: requestedDate,
          organization: settings.organization,
          level: settings.level,
          role: settings.role,
          engine: aiSettings.engine,
          customInstructions: aiSettings.customInstructions,
          gitAuthor: aiSettings.gitAuthor,
          includeUncommitted: aiSettings.includeUncommitted,
          projects: selectedProjects.map((project) => ({
            projectNo: project.projectNo,
            projectName: project.projectName.trim() || pathName(project.path),
            path: project.path,
          })),
          workflowTasks: selectedWorkflowTasks.map(
            (task): DailyReportWorkflowTask => ({
              taskId: task.id,
              title: task.title,
              projectName: task.projectName,
              status: task.status,
              summary: task.summary,
              developmentState: task.development.state,
              developmentResult: task.development.resultNote,
              reviewNote: task.review.note,
              updatedAt: task.updatedAt,
            }),
          ),
          manualDescription: manualDescription.trim(),
        },
        piSettings,
      );
      if (requestSequence.current !== sequence) return;
      rememberProjects(selectedProjects);
      setGenerated(result);
    } catch (error) {
      if (requestSequence.current === sequence) onError(error);
    } finally {
      if (requestSequence.current === sequence) setGenerating(false);
    }
  };

  const confirmGenerated = () => {
    if (!generated || !generatedDraft) return;
    if (useWorkspaceStore.getState().dailyReportDate !== generated.reportDate) {
      onError(new Error("报告日期已变化，请按当前日期重新生成"));
      return;
    }
    updateDraft({
      results: generatedDraft.results,
      blockers: generatedDraft.blockers,
      reviews: generatedDraft.reviews,
      nextActions: generatedDraft.nextActions,
    });
    onSuccess("AI 日报已填入表单，请核对后再上传或复制");
    close();
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={close}>
      <section
        className="dialog daily-report-ai-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="daily-report-ai-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="dialog-header">
          <div>
            <span className="eyebrow">AI 辅助填报</span>
            <h2 id="daily-report-ai-title">
              {generated ? "确认生成结果" : "AI 生成日报"}
            </h2>
          </div>
          <button
            type="button"
            className="icon-button"
            onClick={close}
            aria-label={
              generating ? "关闭并忽略本次 AI 生成" : "关闭 AI 生成日报"
            }
          >
            <X size={18} />
          </button>
        </header>

        {generated ? (
          <div className="daily-report-ai-body">
            <div className="daily-report-ai-note success">
              <Check size={16} />
              <span>
                AI 已填写结构化事实字段，预览由代码按日报模板生成；确认前不会修改当前表单。
              </span>
            </div>
            {willOverwriteDraft && (
              <div className="daily-report-ai-note warning">
                <AlertTriangle size={16} />
                <span>
                  当前日期已有日报正文，确认后将替换四个正文区块；身份与日报设置不会改变。
                </span>
              </div>
            )}
            <section className="daily-report-ai-preview">
              <header>
                <strong>生成预览</strong>
                <span>{generated.reportDate}</span>
              </header>
              <pre>{preview}</pre>
            </section>
            <div className="dialog-actions">
              <button
                type="button"
                className="button secondary"
                onClick={() => setGenerated(undefined)}
              >
                <ArrowLeft size={15} />
                返回修改
              </button>
              <button
                type="button"
                className="button primary"
                onClick={confirmGenerated}
              >
                <Check size={15} />
                确认填入表单
              </button>
            </div>
          </div>
        ) : (
          <div className="daily-report-ai-body">
            {!dailyReportAIAvailable() && (
              <div className="daily-report-ai-note">
                浏览器预览模式不能执行本机 AI；请在 Wails 桌面客户端中生成。
              </div>
            )}

            {!settings.organization.trim() && (
              <div className="daily-report-ai-note warning">
                请先在“日报设置”中填写组织；生成规则需要该字段确定固定锚点。
              </div>
            )}

            <section className="daily-report-ai-section">
              <header>
                <div>
                  <FolderGit2 size={17} />
                  <strong>选择项目</strong>
                </div>
                <span>{`已选择 ${selectedProjects.length}/${MAX_SELECTED_PROJECTS} 个，成功后会记入历史`}</span>
              </header>

              {candidates.length > 0 ? (
                <div className="daily-report-project-options">
                  {candidates.map((project) => {
                    const selected = selectedPaths.includes(project.path);
                    const editable =
                      selected && editableProjectIDs.has(project.id);
                    return (
                      <div
                        className="daily-report-project-option-shell"
                        key={project.path}
                      >
                        <label
                          className={`daily-report-project-option ${selected ? "selected" : ""} ${editable ? "has-edit-action" : ""}`}
                        >
                          <input
                            type="checkbox"
                            checked={selected}
                            aria-label={`选择项目 ${project.projectName || pathName(project.path)}`}
                            onChange={() => toggleProject(project.path)}
                          />
                          <span>
                            <strong>
                              {project.projectNo
                                ? `${project.projectNo} · `
                                : ""}
                              {project.projectName || pathName(project.path)}
                            </strong>
                            <small>{project.path}</small>
                          </span>
                        </label>
                        {editable && (
                          <button
                            type="button"
                            className="daily-report-project-edit-button"
                            aria-label={`编辑项目 ${project.projectName || pathName(project.path)}`}
                            onClick={() => startProjectEdit(project)}
                          >
                            <Pencil size={12} />
                            编辑
                          </button>
                        )}
                      </div>
                    );
                  })}
                </div>
              ) : (
                <p className="daily-report-project-empty">
                  暂无历史项目，可在下方添加本地 Git 仓库。
                </p>
              )}

              <div className="daily-report-project-add">
                <input
                  aria-label="AI 日报项目编号"
                  value={newProject.projectNo}
                  onChange={(event) =>
                    setNewProject((current) => ({
                      ...current,
                      projectNo: event.target.value,
                    }))
                  }
                  placeholder="项目编号，可选"
                />
                <input
                  aria-label="AI 日报项目名称"
                  value={newProject.projectName}
                  onChange={(event) =>
                    setNewProject((current) => ({
                      ...current,
                      projectName: event.target.value,
                    }))
                  }
                  placeholder="项目名称，可选"
                />
                <div className="daily-report-project-path-field">
                  <input
                    aria-label="AI 日报仓库路径"
                    value={newProject.path}
                    onChange={(event) =>
                      setNewProject((current) => ({
                        ...current,
                        path: event.target.value,
                      }))
                    }
                    placeholder="/绝对路径/to/repository"
                  />
                  <button
                    type="button"
                    className="button secondary compact"
                    aria-label="选择 AI 日报项目文件夹"
                    title={
                      directoryPickerAvailable
                        ? "选择本地 Git 仓库文件夹"
                        : "浏览器预览模式不支持选择文件夹，请手动输入绝对路径"
                    }
                    disabled={!directoryPickerAvailable || selectingDirectory}
                    onClick={chooseProjectDirectory}
                  >
                    {selectingDirectory ? (
                      <LoaderCircle className="spin" size={14} />
                    ) : (
                      <FolderOpen size={14} />
                    )}
                    选择文件夹
                  </button>
                </div>
                <div className="daily-report-project-form-actions">
                  {editingProjectID && (
                    <button
                      type="button"
                      className="button ghost compact"
                      aria-label="取消编辑 AI 日报项目"
                      onClick={cancelProjectEdit}
                    >
                      取消
                    </button>
                  )}
                  <button
                    type="button"
                    className="button secondary compact"
                    onClick={editingProjectID ? saveProjectEdit : addProject}
                  >
                    {editingProjectID ? <Check size={14} /> : <Plus size={14} />}
                    {editingProjectID ? "保存修改" : "添加并选择"}
                  </button>
                </div>
              </div>
            </section>

            <section className="daily-report-ai-section">
              <header>
                <div>
                  <ListChecks size={17} />
                  <strong>选择工作流任务</strong>
                </div>
                <span>{`报告日有更新的任务 · 已选择 ${selectedWorkflowTasks.length}/${MAX_SELECTED_TASKS} 个`}</span>
              </header>

              {reportTasks.length > 0 ? (
                <div className="daily-report-project-options">
                  {reportTasks.map((task) => {
                    const selected = selectedTaskIDs.includes(task.id);
                    return (
                      <label
                        className={`daily-report-project-option ${selected ? "selected" : ""}`}
                        key={task.id}
                      >
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={() => toggleWorkflowTask(task.id)}
                        />
                        <span>
                          <strong>{task.title}</strong>
                          <small>
                            {task.projectName || "未分类项目"} · {STATUS_META[task.status].label} · {taskUpdatedTime(task.updatedAt)} 更新
                          </small>
                        </span>
                      </label>
                    );
                  })}
                </div>
              ) : (
                <p className="daily-report-project-empty">
                  当前报告日期没有更新过的工作流任务，可仅选择项目或填写人工补充。
                </p>
              )}
            </section>

            <section className="daily-report-ai-section">
              <header>
                <div>
                  <Sparkles size={17} />
                  <strong>人工补充</strong>
                </div>
                <span>自由描述即可，由 AI 归纳到日报四个区块</span>
              </header>
              <label className="daily-report-manual-input">
                <span>补充未出现在项目代码或工作流任务中的事实</span>
                <textarea
                  aria-label="日报人工补充"
                  value={manualDescription}
                  onChange={(event) =>
                    setManualDescription(event.target.value)
                  }
                  placeholder="例如：今天完成任务：Y06 项目完成登录页联调；参加需求评审并确认验收范围。明天计划：完成 Y06 三个回归问题。"
                />
              </label>
            </section>

            <div className="daily-report-ai-note">
              AI 只接收所选仓库的 Git 摘要、勾选的工作流任务摘要和人工补充；云端 Token、API 地址、工号不会发送给 AI。
            </div>

            <div className="dialog-actions">
              <button
                type="button"
                className="button ghost"
                onClick={close}
              >
                {generating ? "关闭并忽略结果" : "取消"}
              </button>
              <button
                type="button"
                className="button primary"
                disabled={!canGenerate}
                onClick={runGeneration}
              >
                {generating ? (
                  <LoaderCircle className="spin" size={15} />
                ) : (
                  <Sparkles size={15} />
                )}
                {generating ? "正在生成" : "生成预览"}
              </button>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
