package gitrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/blue7zz/BTaskAssistant/internal/permissions"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

func (service *Service) Status(ctx context.Context, taskID string) (StatusView, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.store.GitBinding(taskID)
	if errors.Is(err, storage.ErrAgentDataNotFound) {
		return StatusView{Files: []ChangedFile{}}, nil
	}
	if err != nil {
		return StatusView{}, err
	}
	reconciled, reconcileErr := service.reconcileBindingLocked(ctx, record)
	if reconcileErr != nil {
		return StatusView{
			Bound: true, Binding: reconciled, Files: []ChangedFile{},
			ErrorMessage: reconcileErr.Error(),
		}, nil
	}
	return service.statusLocked(ctx, reconciled)
}

func (service *Service) statusLocked(ctx context.Context, record storage.GitBindingRecord) (StatusView, error) {
	if record.WorktreePath == nil || record.State != "ready" {
		return StatusView{}, errors.New("任务 Git worktree 尚未就绪")
	}
	if err := verifyWorktree(ctx, record, false); err != nil {
		return StatusView{}, err
	}
	headResult, err := runGit(ctx, *record.WorktreePath, gitProbeTimeout, "rev-parse", "HEAD")
	if err != nil {
		return StatusView{}, err
	}
	head := strings.TrimSpace(string(headResult.stdout))
	porcelain, err := runGit(
		ctx, *record.WorktreePath, gitProbeTimeout,
		"status", "--porcelain=v1", "-z", "--untracked-files=all",
	)
	if err != nil {
		return StatusView{}, err
	}
	files, err := parsePorcelainStatus(porcelain.stdout)
	if err != nil {
		return StatusView{}, err
	}
	snapshotHash := sha256.New()
	_, _ = snapshotHash.Write([]byte(head))
	_, _ = snapshotHash.Write([]byte{0})
	_, _ = snapshotHash.Write(porcelain.stdout)
	ahead, behind := 0, 0
	divergence, divergenceErr := runGit(
		ctx, *record.WorktreePath, gitProbeTimeout,
		"rev-list", "--left-right", "--count", record.BaselineCommit+"...HEAD",
	)
	if divergenceErr == nil {
		parts := strings.Fields(string(divergence.stdout))
		if len(parts) == 2 {
			behind, _ = strconv.Atoi(parts[0])
			ahead, _ = strconv.Atoi(parts[1])
		}
	}
	return StatusView{
		Bound: true, Binding: record, RemoteURL: safeRemoteURL(ctx, record.SourceRealPath), Head: head,
		Snapshot: hex.EncodeToString(snapshotHash.Sum(nil)), Files: files,
		AheadOfBaseline: ahead, BehindBaseline: behind,
	}, nil
}

func safeRemoteURL(ctx context.Context, repository string) string {
	result, err := runGitAllowExitOne(ctx, repository, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(result.stdout))
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	parsed, parseErr := url.Parse(value)
	if parseErr == nil && parsed.Scheme != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	return value
}

func parsePorcelainStatus(raw []byte) ([]ChangedFile, error) {
	tokens := bytes.Split(raw, []byte{0})
	files := make([]ChangedFile, 0, len(tokens))
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if len(token) == 0 {
			continue
		}
		if len(token) < 4 || token[2] != ' ' || !utf8.Valid(token[3:]) {
			return nil, errors.New("Git status 包含无法安全显示的文件名")
		}
		indexStatus := token[0]
		worktreeStatus := token[1]
		pathValue := string(token[3:])
		originalPath := ""
		if indexStatus == 'R' || indexStatus == 'C' || worktreeStatus == 'R' || worktreeStatus == 'C' {
			index++
			if index >= len(tokens) || !utf8.Valid(tokens[index]) {
				return nil, errors.New("Git status rename 记录不完整")
			}
			originalPath = string(tokens[index])
		}
		files = append(files, ChangedFile{
			Path: pathValue, OriginalPath: originalPath,
			Status:      statusLabel(indexStatus, worktreeStatus),
			IndexStatus: string(indexStatus), WorktreeStatus: string(worktreeStatus),
			Staged:   indexStatus != ' ' && indexStatus != '?',
			Unstaged: worktreeStatus != ' ' || indexStatus == '?',
		})
	}
	sort.Slice(files, func(left int, right int) bool {
		return files[left].Path < files[right].Path
	})
	return files, nil
}

func statusLabel(indexStatus byte, worktreeStatus byte) string {
	if indexStatus == '?' {
		return "untracked"
	}
	if indexStatus == 'U' || worktreeStatus == 'U' ||
		(indexStatus == 'A' && worktreeStatus == 'A') ||
		(indexStatus == 'D' && worktreeStatus == 'D') {
		return "conflicted"
	}
	for _, value := range []byte{indexStatus, worktreeStatus} {
		switch value {
		case 'R':
			return "renamed"
		case 'C':
			return "copied"
		case 'D':
			return "deleted"
		case 'A':
			return "added"
		case 'M', 'T':
			return "modified"
		}
	}
	return "modified"
}

func (service *Service) Diff(ctx context.Context, taskID string, pathValue string) (FileDiffView, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.store.GitBinding(taskID)
	if err != nil {
		return FileDiffView{}, err
	}
	record, err = service.reconcileBindingLocked(ctx, record)
	if err != nil {
		return FileDiffView{}, err
	}
	status, err := service.statusLocked(ctx, record)
	if err != nil {
		return FileDiffView{}, err
	}
	normalized, err := permissions.NormalizeRelativePath(pathValue)
	if err != nil {
		return FileDiffView{}, fmt.Errorf("Diff 路径不安全: %w", err)
	}
	var changed *ChangedFile
	for index := range status.Files {
		if status.Files[index].Path == normalized {
			changed = &status.Files[index]
			break
		}
	}
	if changed == nil {
		return FileDiffView{}, errors.New("文件不在当前任务的未提交变更中")
	}
	root := *record.WorktreePath
	stagedResult, err := runGitWithLimit(
		ctx, root, gitProbeTimeout, maxDiffBytes, true,
		"diff", "--cached", "--no-ext-diff", "--no-color", "--binary", "--full-index", "--", normalized,
	)
	if err != nil {
		return FileDiffView{}, err
	}
	unstagedResult := commandOutput{}
	if changed.Status == "untracked" {
		nullDevice := "/dev/null"
		if runtime.GOOS == "windows" {
			nullDevice = "NUL"
		}
		unstagedResult, err = runGitWithLimit(
			ctx, root, gitProbeTimeout, maxDiffBytes, true,
			"diff", "--no-index", "--no-ext-diff", "--no-color", "--binary", "--", nullDevice, normalized,
		)
		if err != nil && unstagedResult.exitCode != 1 {
			return FileDiffView{}, err
		}
	} else {
		unstagedResult, err = runGitWithLimit(
			ctx, root, gitProbeTimeout, maxDiffBytes, true,
			"diff", "--no-ext-diff", "--no-color", "--binary", "--full-index", "--", normalized,
		)
		if err != nil {
			return FileDiffView{}, err
		}
	}
	staged, unstaged, trimmed := fitDiffOutput(stagedResult.stdout, unstagedResult.stdout, maxDiffBytes)
	combined := staged + unstaged
	added, removed := countDiffLines(combined)
	binary := strings.Contains(combined, "GIT binary patch") || strings.Contains(combined, "Binary files ")
	return FileDiffView{
		TaskID: taskID, Path: normalized, Status: changed.Status,
		Staged: staged, Unstaged: unstaged,
		Added: added, Removed: removed, Binary: binary,
		Truncated: stagedResult.truncated || unstagedResult.truncated || trimmed,
		ByteSize:  stagedResult.total + unstagedResult.total,
	}, nil
}

func fitDiffOutput(staged []byte, unstaged []byte, limit int) (string, string, bool) {
	trimmed := false
	if len(staged) > limit {
		staged = staged[:limit]
		unstaged = nil
		trimmed = true
	} else if len(staged)+len(unstaged) > limit {
		unstaged = unstaged[:limit-len(staged)]
		trimmed = true
	}
	return safeUTF8(staged), safeUTF8(unstaged), trimmed
}

func safeUTF8(value []byte) string {
	for len(value) > 0 && !utf8.Valid(value) {
		value = value[:len(value)-1]
	}
	return string(value)
}

func countDiffLines(value string) (added int, removed int) {
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return added, removed
}

func (service *Service) Commit(ctx context.Context, request CommitRequest) (CommitResult, error) {
	if !request.Confirmed {
		return CommitResult{}, errors.New("本地 commit 需要在变更预览后明确确认")
	}
	message := strings.TrimSpace(request.Message)
	if message == "" || len([]byte(message)) > 4096 || strings.ContainsRune(message, '\x00') {
		return CommitResult{}, errors.New("commit message 不能为空、不得包含 NUL 且不能超过 4 KiB")
	}
	unlock := service.lockTask(request.TaskID)
	defer unlock()
	if status, err := service.store.TaskStatus(request.TaskID); err != nil {
		return CommitResult{}, err
	} else if status != "development" {
		return CommitResult{}, errors.New("仅 development 状态允许创建本地 commit")
	}
	record, err := service.store.GitBinding(request.TaskID)
	if err != nil {
		return CommitResult{}, err
	}
	record, err = service.reconcileBindingLocked(ctx, record)
	if err != nil {
		return CommitResult{}, err
	}
	status, err := service.statusLocked(ctx, record)
	if err != nil {
		return CommitResult{}, err
	}
	if request.ExpectedSnapshot == "" || request.ExpectedSnapshot != status.Snapshot {
		return CommitResult{}, errors.New("Git 变更在确认后发生变化，请刷新并重新预览")
	}
	if len(status.Files) == 0 {
		return CommitResult{}, errors.New("当前任务 worktree 没有可提交的修改")
	}
	root := *record.WorktreePath
	if _, err := runGit(ctx, root, gitMutationTimeout, "add", "-A", "--", "."); err != nil {
		return CommitResult{}, fmt.Errorf("暂存任务变更失败；现有文件不会被清理: %w", err)
	}
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return CommitResult{}, err
	}
	disabledHooks := filepath.Join(workspace.RootPath, ".btask", "disabled-git-hooks")
	if err := os.MkdirAll(disabledHooks, 0o700); err != nil {
		return CommitResult{}, fmt.Errorf("创建禁用 hooks 的安全目录失败: %w", err)
	}
	if _, err := runGit(
		ctx, root, gitMutationTimeout,
		"-c", "core.hooksPath="+disabledHooks,
		"-c", "commit.gpgsign=false",
		"commit", "--no-verify", "-m", message,
	); err != nil {
		return CommitResult{}, fmt.Errorf("创建本地 commit 失败；已暂存修改会保留供用户检查: %w", err)
	}
	headResult, err := runGit(ctx, root, gitProbeTimeout, "rev-parse", "HEAD")
	if err != nil {
		return CommitResult{}, err
	}
	record.UpdatedAt = service.timestamp()
	if err := service.store.UpsertGitBinding(record); err != nil {
		return CommitResult{}, err
	}
	updated, err := service.statusLocked(ctx, record)
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Commit: strings.TrimSpace(string(headResult.stdout)), Status: updated}, nil
}
