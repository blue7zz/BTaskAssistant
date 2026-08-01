package gitrepo

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

var branchComponentPattern = regexp.MustCompile(`[^a-z0-9._-]+`)

type Store interface {
	EnsureTaskWorkspace(string) (storage.TaskWorkspaceRecord, error)
	GitBinding(string) (storage.GitBindingRecord, error)
	UpsertGitBinding(storage.GitBindingRecord) error
	TaskStatus(string) (string, error)
}

type Service struct {
	store Store
	now   func() time.Time

	mutex     sync.Mutex
	taskLocks map[string]*sync.Mutex
}

type repositoryInspection struct {
	selectedPath string
	repository   string
	commonGitDir string
	head         string
	branch       string
	status       []byte
}

func NewService(store Store) *Service {
	return &Service{
		store:     store,
		now:       time.Now,
		taskLocks: make(map[string]*sync.Mutex),
	}
}

func (service *Service) Bind(ctx context.Context, request BindRequest) (storage.GitBindingRecord, error) {
	unlock := service.lockTask(request.TaskID)
	defer unlock()
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	inspection, err := inspectRepository(ctx, request.SourcePath)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	if existing, existingErr := service.store.GitBinding(request.TaskID); existingErr == nil {
		if existing.SourceRealPath != inspection.repository || existing.CommonGitDir != inspection.commonGitDir {
			return existing, errors.New("当前任务已绑定另一个 Git 仓库；请先安全清理原 worktree")
		}
		return service.reconcileBindingLocked(ctx, existing)
	} else if !errors.Is(existingErr, storage.ErrAgentDataNotFound) {
		return storage.GitBindingRecord{}, existingErr
	}

	worktreePath, err := managedWorktreePath(workspace.RootPath, inspection.repository, inspection.commonGitDir)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	if pathWithin(worktreePath, inspection.repository) {
		return storage.GitBindingRecord{}, errors.New("任务 worktree 不能位于原始 Git 工作区内")
	}
	if info, statErr := os.Lstat(worktreePath); statErr == nil {
		return storage.GitBindingRecord{}, fmt.Errorf("任务 worktree 目标已存在且不会被覆盖: %s", info.Name())
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return storage.GitBindingRecord{}, fmt.Errorf("检查任务 worktree 目标失败: %w", statErr)
	}

	bindingID, branch, err := allocateBindingIdentity(ctx, inspection.repository, request.TaskID)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	now := service.timestamp()
	worktreePathCopy := worktreePath
	branchCopy := branch
	record := storage.GitBindingRecord{
		ID: bindingID, TaskID: request.TaskID,
		SourcePath: inspection.selectedPath, SourceRealPath: inspection.repository,
		CommonGitDir: inspection.commonGitDir, WorktreePath: &worktreePathCopy,
		Branch: &branchCopy, BaselineCommit: inspection.head,
		SourceDirtyAtBind: len(bytes.TrimSpace(inspection.status)) > 0,
		State:             "creating", CreatedAt: now, UpdatedAt: now,
	}
	if inspection.branch != "" {
		sourceBranch := inspection.branch
		record.SourceBranch = &sourceBranch
	}
	if err := service.store.UpsertGitBinding(record); err != nil {
		return storage.GitBindingRecord{}, err
	}

	_, addErr := runGit(
		ctx, inspection.repository, gitMutationTimeout,
		"worktree", "add", "-b", branch, worktreePath, inspection.head,
	)
	if addErr != nil {
		return service.failBinding(record, fmt.Errorf("创建任务 Git worktree 失败: %w", addErr))
	}
	if err := verifyWorktree(ctx, record, true); err != nil {
		return service.failBinding(record, err)
	}
	afterStatus, err := sourceStatus(ctx, inspection.repository)
	if err != nil {
		return service.failBinding(record, fmt.Errorf("复核原始工作区失败: %w", err))
	}
	if !bytes.Equal(inspection.status, afterStatus) {
		return service.failBinding(record, errors.New("创建 worktree 后原始工作区状态发生变化，已停止且不会自动清理"))
	}
	record.State = "ready"
	record.UpdatedAt = service.timestamp()
	record.ErrorMessage = nil
	if err := service.store.UpsertGitBinding(record); err != nil {
		return record, err
	}
	return record, nil
}

func (service *Service) Binding(ctx context.Context, taskID string) (storage.GitBindingRecord, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.store.GitBinding(taskID)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	return service.reconcileBindingLocked(ctx, record)
}

func (service *Service) Cleanup(ctx context.Context, request CleanupRequest) (storage.GitBindingRecord, error) {
	if !request.Confirmed {
		return storage.GitBindingRecord{}, errors.New("清理任务 worktree 需要明确确认")
	}
	unlock := service.lockTask(request.TaskID)
	defer unlock()
	record, err := service.store.GitBinding(request.TaskID)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	status, err := service.statusLocked(ctx, record)
	if err != nil {
		return record, err
	}
	if len(status.Files) > 0 {
		return record, errors.New("任务 worktree 仍有未提交修改，拒绝清理")
	}
	if record.WorktreePath == nil || *record.WorktreePath == "" {
		return record, errors.New("任务 worktree 路径未登记")
	}
	if _, err := runGit(
		ctx, record.SourceRealPath, gitMutationTimeout,
		"worktree", "remove", *record.WorktreePath,
	); err != nil {
		return service.failBinding(record, fmt.Errorf("安全清理任务 worktree 失败: %w", err))
	}
	if _, statErr := os.Lstat(*record.WorktreePath); !errors.Is(statErr, os.ErrNotExist) {
		if statErr == nil {
			return service.failBinding(record, errors.New("Git 报告清理成功，但 worktree 目录仍存在"))
		}
		return service.failBinding(record, fmt.Errorf("复核 worktree 清理结果失败: %w", statErr))
	}
	record.State = "archived"
	record.UpdatedAt = service.timestamp()
	record.ErrorMessage = nil
	if err := service.store.UpsertGitBinding(record); err != nil {
		return record, err
	}
	return record, nil
}

func (service *Service) Recover(ctx context.Context, request RecoverRequest) (storage.GitBindingRecord, error) {
	if !request.Confirmed {
		return storage.GitBindingRecord{}, errors.New("恢复任务 worktree 需要明确确认")
	}
	unlock := service.lockTask(request.TaskID)
	defer unlock()
	record, err := service.store.GitBinding(request.TaskID)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	if record.WorktreePath == nil || record.Branch == nil || *record.WorktreePath == "" || *record.Branch == "" {
		return record, errors.New("任务 worktree 缺少路径或分支记录，无法恢复")
	}
	if _, statErr := os.Lstat(*record.WorktreePath); statErr == nil {
		return service.reconcileBindingLocked(ctx, record)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return record, fmt.Errorf("检查待恢复 worktree 失败: %w", statErr)
	}
	inspection, err := inspectRepository(ctx, record.SourcePath)
	if err != nil {
		return service.failBinding(record, fmt.Errorf("绑定的原始仓库不可用: %w", err))
	}
	if inspection.repository != record.SourceRealPath || inspection.commonGitDir != record.CommonGitDir {
		return service.failBinding(record, errors.New("绑定仓库已移动或被替换，拒绝恢复到不一致来源"))
	}
	branchResult, err := runGitAllowExitOne(
		ctx, inspection.repository, "show-ref", "--verify", "--quiet", "refs/heads/"+*record.Branch,
	)
	if err != nil || branchResult.exitCode != 0 {
		return service.failBinding(record, errors.New("任务分支不存在，拒绝猜测恢复基线"))
	}
	if err := ensureRecoverableWorktreeIdentity(ctx, record); err != nil {
		return service.failBinding(record, err)
	}
	if _, err := runGit(
		ctx, inspection.repository, gitMutationTimeout,
		"worktree", "add", "--force", *record.WorktreePath, *record.Branch,
	); err != nil {
		return service.failBinding(record, fmt.Errorf("恢复任务 worktree 失败: %w", err))
	}
	if err := verifyWorktree(ctx, record, false); err != nil {
		return service.failBinding(record, err)
	}
	afterStatus, err := sourceStatus(ctx, inspection.repository)
	if err != nil || !bytes.Equal(inspection.status, afterStatus) {
		if err == nil {
			err = errors.New("恢复 worktree 后原始工作区状态发生变化")
		}
		return service.failBinding(record, err)
	}
	record.State = "ready"
	record.UpdatedAt = service.timestamp()
	record.ErrorMessage = nil
	if err := service.store.UpsertGitBinding(record); err != nil {
		return record, err
	}
	return record, nil
}

func (service *Service) reconcileBindingLocked(ctx context.Context, record storage.GitBindingRecord) (storage.GitBindingRecord, error) {
	if record.WorktreePath == nil || *record.WorktreePath == "" {
		return service.markBinding(record, "missing", errors.New("任务 worktree 路径未登记"))
	}
	if _, err := os.Lstat(record.SourceRealPath); err != nil {
		return service.markBinding(record, "missing", errors.New("绑定的原始 Git 仓库已移动或不可用"))
	}
	if _, err := os.Lstat(*record.WorktreePath); errors.Is(err, os.ErrNotExist) {
		if record.State == "archived" {
			return record, errors.New("任务 worktree 已安全清理；如需继续开发请显式恢复")
		}
		return service.markBinding(record, "missing", errors.New("任务 worktree 已被外部删除；请显式恢复"))
	} else if err != nil {
		return service.markBinding(record, "missing", fmt.Errorf("检查任务 worktree 失败: %w", err))
	}
	if err := verifyWorktree(ctx, record, false); err != nil {
		return service.markBinding(record, "missing", err)
	}
	if record.State != "ready" || record.ErrorMessage != nil {
		record.State = "ready"
		record.ErrorMessage = nil
		record.UpdatedAt = service.timestamp()
		if err := service.store.UpsertGitBinding(record); err != nil {
			return record, err
		}
	}
	return record, nil
}

func (service *Service) markBinding(record storage.GitBindingRecord, state string, operationErr error) (storage.GitBindingRecord, error) {
	message := operationErr.Error()
	record.State = state
	record.ErrorMessage = &message
	record.UpdatedAt = service.timestamp()
	if err := service.store.UpsertGitBinding(record); err != nil {
		return record, errors.Join(operationErr, err)
	}
	return record, operationErr
}

func (service *Service) failBinding(record storage.GitBindingRecord, operationErr error) (storage.GitBindingRecord, error) {
	return service.markBinding(record, "cleanup_failed", operationErr)
}

func inspectRepository(ctx context.Context, selectedPath string) (repositoryInspection, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return repositoryInspection{}, errors.New("未找到 Git，无法绑定本地仓库")
	}
	selectedPath = strings.TrimSpace(selectedPath)
	if selectedPath == "" || strings.ContainsRune(selectedPath, '\x00') {
		return repositoryInspection{}, errors.New("请选择本地 Git 仓库目录")
	}
	absolute, err := filepath.Abs(selectedPath)
	if err != nil {
		return repositoryInspection{}, fmt.Errorf("解析仓库目录失败: %w", err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return repositoryInspection{}, errors.New("所选仓库路径不存在、不是目录或是符号链接")
	}
	selectedReal, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return repositoryInspection{}, fmt.Errorf("解析仓库真实路径失败: %w", err)
	}
	repositoryResult, err := runGit(ctx, selectedReal, gitProbeTimeout, "rev-parse", "--show-toplevel")
	if err != nil {
		return repositoryInspection{}, errors.New("所选目录不在 Git 仓库中")
	}
	repository := filepath.Clean(strings.TrimSpace(string(repositoryResult.stdout)))
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil || !filepath.IsAbs(repository) {
		return repositoryInspection{}, errors.New("Git 未返回可验证的仓库根目录")
	}
	bare, err := runGit(ctx, repository, gitProbeTimeout, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(string(bare.stdout)) == "true" {
		return repositoryInspection{}, errors.New("裸仓库不能绑定为任务 worktree 来源")
	}
	headResult, err := runGit(ctx, repository, gitProbeTimeout, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(headResult.stdout)) == "" {
		return repositoryInspection{}, errors.New("仓库需要至少一个已提交的 HEAD")
	}
	commonResult, err := runGit(ctx, repository, gitProbeTimeout, "rev-parse", "--git-common-dir")
	if err != nil {
		return repositoryInspection{}, fmt.Errorf("读取 Git common dir 失败: %w", err)
	}
	commonDir := filepath.Clean(strings.TrimSpace(string(commonResult.stdout)))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repository, commonDir)
	}
	commonDir, err = filepath.EvalSymlinks(commonDir)
	if err != nil {
		return repositoryInspection{}, fmt.Errorf("验证 Git common dir 失败: %w", err)
	}
	branchResult, _ := runGitAllowExitOne(ctx, repository, "symbolic-ref", "--quiet", "--short", "HEAD")
	status, err := sourceStatus(ctx, repository)
	if err != nil {
		return repositoryInspection{}, err
	}
	return repositoryInspection{
		selectedPath: selectedReal,
		repository:   repository,
		commonGitDir: commonDir,
		head:         strings.TrimSpace(string(headResult.stdout)),
		branch:       strings.TrimSpace(string(branchResult.stdout)),
		status:       status,
	}, nil
}

func sourceStatus(ctx context.Context, repository string) ([]byte, error) {
	result, err := runGit(
		ctx, repository, gitProbeTimeout,
		"status", "--porcelain=v1", "-z", "--untracked-files=all",
	)
	if err != nil {
		return nil, fmt.Errorf("读取原始工作区状态失败: %w", err)
	}
	return append([]byte(nil), result.stdout...), nil
}

func managedWorktreePath(workspaceRoot string, repository string, commonDir string) (string, error) {
	reposRoot := filepath.Join(workspaceRoot, "repos")
	info, err := os.Lstat(reposRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("任务 repos 目录不存在或不安全")
	}
	reposRoot, err = filepath.EvalSymlinks(reposRoot)
	if err != nil || !filepath.IsAbs(reposRoot) {
		return "", errors.New("任务 repos 目录真实路径不可验证")
	}
	base := safePathComponent(filepath.Base(repository))
	if base == "" {
		base = "repository"
	}
	hash := sha256.Sum256([]byte(commonDir))
	path := filepath.Clean(filepath.Join(reposRoot, base+"-"+hex.EncodeToString(hash[:4])))
	relative, err := filepath.Rel(reposRoot, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("任务 worktree 目标逃逸 repos 目录")
	}
	return path, nil
}

func allocateBindingIdentity(ctx context.Context, repository string, taskID string) (string, string, error) {
	component := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(taskID)), "task_")
	component = strings.Trim(branchComponentPattern.ReplaceAllString(component, "-"), ".-")
	if component == "" {
		component = "task"
	}
	if len(component) > 40 {
		component = component[:40]
	}
	for attempt := 0; attempt < 20; attempt++ {
		identifier, err := randomHex(8)
		if err != nil {
			return "", "", err
		}
		branch := "btask/" + component + "-" + identifier[:10]
		if _, err := runGit(ctx, repository, gitProbeTimeout, "check-ref-format", "--branch", branch); err != nil {
			return "", "", errors.New("无法生成安全的任务分支名")
		}
		result, err := runGitAllowExitOne(ctx, repository, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
		if err != nil {
			return "", "", err
		}
		if result.exitCode == 1 {
			return "binding_" + identifier, branch, nil
		}
	}
	return "", "", errors.New("无法分配不冲突的任务分支")
}

func verifyWorktree(ctx context.Context, record storage.GitBindingRecord, requireBaseline bool) error {
	if record.WorktreePath == nil || record.Branch == nil {
		return errors.New("任务 worktree 记录缺少路径或分支")
	}
	info, err := os.Lstat(*record.WorktreePath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("任务 worktree 不存在、不是目录或是符号链接")
	}
	rootResult, err := runGit(ctx, *record.WorktreePath, gitProbeTimeout, "rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("任务 worktree 不是有效 Git 工作区")
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(strings.TrimSpace(string(rootResult.stdout))))
	if err != nil || root != *record.WorktreePath {
		return errors.New("任务 worktree 根目录与登记路径不一致")
	}
	commonResult, err := runGit(ctx, root, gitProbeTimeout, "rev-parse", "--git-common-dir")
	if err != nil {
		return errors.New("无法验证任务 worktree 的 Git 来源")
	}
	commonDir := filepath.Clean(strings.TrimSpace(string(commonResult.stdout)))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	commonDir, err = filepath.EvalSymlinks(commonDir)
	if err != nil || commonDir != record.CommonGitDir {
		return errors.New("任务 worktree 指向了不同 Git 仓库")
	}
	branchResult, err := runGit(ctx, root, gitProbeTimeout, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(string(branchResult.stdout)) != *record.Branch {
		return errors.New("任务 worktree 分支与登记分支不一致")
	}
	if requireBaseline {
		headResult, headErr := runGit(ctx, root, gitProbeTimeout, "rev-parse", "HEAD")
		if headErr != nil || strings.TrimSpace(string(headResult.stdout)) != record.BaselineCommit {
			return errors.New("新建任务 worktree 的 HEAD 与基线 commit 不一致")
		}
	}
	return nil
}

func ensureRecoverableWorktreeIdentity(ctx context.Context, record storage.GitBindingRecord) error {
	result, err := runGit(ctx, record.SourceRealPath, gitProbeTimeout, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	blocks := strings.Split(strings.TrimSpace(string(result.stdout)), "\n\n")
	targetBranch := "refs/heads/" + *record.Branch
	for _, block := range blocks {
		fields := strings.Split(block, "\n")
		pathValue := ""
		branchValue := ""
		for _, field := range fields {
			if strings.HasPrefix(field, "worktree ") {
				pathValue = strings.TrimPrefix(field, "worktree ")
			}
			if strings.HasPrefix(field, "branch ") {
				branchValue = strings.TrimPrefix(field, "branch ")
			}
		}
		if branchValue == targetBranch && filepath.Clean(pathValue) != filepath.Clean(*record.WorktreePath) {
			return errors.New("任务分支已在另一个 worktree 中使用，拒绝强制恢复")
		}
	}
	return nil
}

func pathWithin(pathValue string, possibleParent string) bool {
	relative, err := filepath.Rel(filepath.Clean(possibleParent), filepath.Clean(pathValue))
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

func safePathComponent(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(character rune) rune {
		if character < 32 || strings.ContainsRune(`/\\:<>"|?*`, character) {
			return '-'
		}
		return character
	}, value)
	value = strings.Trim(value, ". ")
	reserved := strings.ToUpper(strings.SplitN(value, ".", 2)[0])
	reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	for _, name := range reservedNames {
		if name == reserved {
			return "_" + value
		}
	}
	return value
}

func randomHex(bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("生成任务 Git 标识失败: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func (service *Service) lockTask(taskID string) func() {
	service.mutex.Lock()
	lock := service.taskLocks[taskID]
	if lock == nil {
		lock = &sync.Mutex{}
		service.taskLocks[taskID] = lock
	}
	service.mutex.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (service *Service) timestamp() string {
	return service.now().UTC().Format(time.RFC3339Nano)
}
