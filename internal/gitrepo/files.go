package gitrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blue7zz/BTaskAssistant/internal/permissions"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

const maxWorktreeFileBytes = 2 * 1024 * 1024

func (service *Service) ListFiles(
	ctx context.Context,
	taskID string,
	query string,
	limit int,
) ([]FileEntry, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.readyBindingLocked(ctx, taskID)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if len(query) > 200 || strings.ContainsRune(query, '\x00') {
		return nil, errors.New("仓库文件查询不能超过 200 字符或包含 NUL")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	root := *record.WorktreePath
	trackedResult, err := runGit(ctx, root, gitProbeTimeout, "ls-files", "-c", "-z")
	if err != nil {
		return nil, err
	}
	untrackedResult, err := runGit(ctx, root, gitProbeTimeout, "ls-files", "-o", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool)
	paths := make([]string, 0)
	appendPaths := func(raw []byte, isTracked bool) error {
		for _, value := range strings.Split(string(raw), "\x00") {
			if value == "" {
				continue
			}
			normalized, normalizeErr := permissions.NormalizeRelativePath(value)
			if normalizeErr != nil || normalized != value || strings.EqualFold(normalized, ".git") ||
				strings.HasPrefix(strings.ToLower(normalized), ".git/") {
				continue
			}
			if _, exists := tracked[normalized]; !exists {
				paths = append(paths, normalized)
			}
			tracked[normalized] = tracked[normalized] || isTracked
		}
		return nil
	}
	if err := appendPaths(trackedResult.stdout, true); err != nil {
		return nil, err
	}
	if err := appendPaths(untrackedResult.stdout, false); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootHandle.Close()
	entries := make([]FileEntry, 0, min(limit, len(paths)))
	for _, pathValue := range paths {
		if query != "" && !strings.Contains(strings.ToLower(pathValue), query) {
			continue
		}
		info, statErr := rootHandle.Lstat(filepath.FromSlash(pathValue))
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		entries = append(entries, FileEntry{
			Path: pathValue, Tracked: tracked[pathValue], ByteSize: info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(timeFormat),
		})
		if len(entries) == limit {
			break
		}
	}
	return entries, nil
}

func (service *Service) ReadFile(ctx context.Context, taskID string, pathValue string) (FileContent, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.readyBindingLocked(ctx, taskID)
	if err != nil {
		return FileContent{}, err
	}
	return readWorktreeFile(*record.WorktreePath, pathValue)
}

func (service *Service) WriteFile(
	ctx context.Context,
	taskID string,
	mode string,
	pathValue string,
	content string,
) (FileWriteResult, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.writableBindingLocked(ctx, taskID, mode)
	if err != nil {
		return FileWriteResult{}, err
	}
	return writeWorktreeFile(*record.WorktreePath, pathValue, []byte(content), true)
}

func (service *Service) EditFile(
	ctx context.Context,
	taskID string,
	mode string,
	pathValue string,
	oldText string,
	newText string,
	replaceAll bool,
) (FileWriteResult, error) {
	if oldText == "" {
		return FileWriteResult{}, errors.New("精确编辑的 oldText 不能为空")
	}
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.writableBindingLocked(ctx, taskID, mode)
	if err != nil {
		return FileWriteResult{}, err
	}
	current, err := readWorktreeFile(*record.WorktreePath, pathValue)
	if err != nil {
		return FileWriteResult{}, err
	}
	count := strings.Count(current.Content, oldText)
	if count == 0 {
		return FileWriteResult{}, errors.New("oldText 在当前文件中不存在")
	}
	if count > 1 && !replaceAll {
		return FileWriteResult{}, errors.New("oldText 在当前文件中不唯一；请提供更多上下文或明确 replaceAll")
	}
	updated := strings.Replace(current.Content, oldText, newText, 1)
	if replaceAll {
		updated = strings.ReplaceAll(current.Content, oldText, newText)
	}
	return writeWorktreeFile(*record.WorktreePath, pathValue, []byte(updated), false)
}

func (service *Service) DeleteFile(
	ctx context.Context,
	taskID string,
	mode string,
	pathValue string,
) (FileWriteResult, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.writableBindingLocked(ctx, taskID, mode)
	if err != nil {
		return FileWriteResult{}, err
	}
	normalized, err := normalizeWorktreePath(pathValue)
	if err != nil {
		return FileWriteResult{}, err
	}
	root, err := os.OpenRoot(*record.WorktreePath)
	if err != nil {
		return FileWriteResult{}, err
	}
	defer root.Close()
	nativePath := filepath.FromSlash(normalized)
	if err := validateExistingRegularFile(root, nativePath); err != nil {
		return FileWriteResult{}, err
	}
	content, err := root.ReadFile(nativePath)
	if err != nil {
		return FileWriteResult{}, err
	}
	previous := sha256.Sum256(content)
	if err := root.Remove(nativePath); err != nil {
		return FileWriteResult{}, fmt.Errorf("删除 worktree 文件失败: %w", err)
	}
	return FileWriteResult{
		Path: normalized, Operation: "delete",
		PreviousSHA: hex.EncodeToString(previous[:]),
	}, nil
}

func (service *Service) ResolveCommandDirectory(
	ctx context.Context,
	taskID string,
	mode string,
	relativeDirectory string,
) (storage.GitBindingRecord, string, string, error) {
	unlock := service.lockTask(taskID)
	defer unlock()
	record, err := service.writableBindingLocked(ctx, taskID, mode)
	if err != nil {
		return storage.GitBindingRecord{}, "", "", err
	}
	root := *record.WorktreePath
	relativeDirectory = strings.TrimSpace(relativeDirectory)
	if relativeDirectory == "" || relativeDirectory == "." {
		return record, root, ".", nil
	}
	normalized, err := normalizeWorktreePath(relativeDirectory)
	if err != nil {
		return storage.GitBindingRecord{}, "", "", err
	}
	resolved, err := permissions.ResolveWithinRoot(root, normalized, false)
	if err != nil {
		return storage.GitBindingRecord{}, "", "", fmt.Errorf("Shell cwd 不安全: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return storage.GitBindingRecord{}, "", "", errors.New("Shell cwd 不存在、不是目录或是符号链接")
	}
	return record, resolved, normalized, nil
}

func (service *Service) readyBindingLocked(ctx context.Context, taskID string) (storage.GitBindingRecord, error) {
	record, err := service.store.GitBinding(taskID)
	if err != nil {
		if errors.Is(err, storage.ErrAgentDataNotFound) {
			return storage.GitBindingRecord{}, errors.New("当前任务尚未绑定 Git 仓库")
		}
		return storage.GitBindingRecord{}, err
	}
	return service.reconcileBindingLocked(ctx, record)
}

func (service *Service) writableBindingLocked(ctx context.Context, taskID string, mode string) (storage.GitBindingRecord, error) {
	if strings.ToLower(strings.TrimSpace(mode)) != "agent" {
		return storage.GitBindingRecord{}, errors.New("仅 Agent 模式允许修改任务 worktree")
	}
	status, err := service.store.TaskStatus(taskID)
	if err != nil {
		return storage.GitBindingRecord{}, err
	}
	if status != "development" {
		return storage.GitBindingRecord{}, errors.New("仅 development 状态允许修改任务 worktree")
	}
	return service.readyBindingLocked(ctx, taskID)
}

func readWorktreeFile(rootPath string, pathValue string) (FileContent, error) {
	normalized, err := normalizeWorktreePath(pathValue)
	if err != nil {
		return FileContent{}, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return FileContent{}, err
	}
	defer root.Close()
	nativePath := filepath.FromSlash(normalized)
	if err := validateExistingRegularFile(root, nativePath); err != nil {
		return FileContent{}, err
	}
	file, err := root.Open(nativePath)
	if err != nil {
		return FileContent{}, err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxWorktreeFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return FileContent{}, readErr
	}
	if closeErr != nil {
		return FileContent{}, closeErr
	}
	if len(content) > maxWorktreeFileBytes {
		return FileContent{}, errors.New("worktree 文件超过 2 MiB 读取上限")
	}
	if !utf8.Valid(content) || strings.IndexByte(string(content), 0) >= 0 {
		return FileContent{}, errors.New("worktree 文件不是受支持的 UTF-8 文本")
	}
	hash := sha256.Sum256(content)
	return FileContent{
		Path: normalized, Content: string(content), ByteSize: int64(len(content)),
		SHA256: hex.EncodeToString(hash[:]),
	}, nil
}

func writeWorktreeFile(rootPath string, pathValue string, content []byte, createParents bool) (FileWriteResult, error) {
	if len(content) > maxWorktreeFileBytes || !utf8.Valid(content) || bytesContainNUL(content) {
		return FileWriteResult{}, errors.New("worktree 写入必须是无 NUL 的 UTF-8 文本且不超过 2 MiB")
	}
	normalized, err := normalizeWorktreePath(pathValue)
	if err != nil {
		return FileWriteResult{}, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return FileWriteResult{}, err
	}
	defer root.Close()
	nativePath := filepath.FromSlash(normalized)
	if err := ensureSafeParents(root, filepath.Dir(nativePath), createParents); err != nil {
		return FileWriteResult{}, err
	}
	if err := rejectCaseAliasInRoot(root, filepath.Dir(nativePath), filepath.Base(nativePath)); err != nil {
		return FileWriteResult{}, err
	}
	mode := os.FileMode(0o644)
	operation := "create"
	previousSHA := ""
	if info, statErr := root.Lstat(nativePath); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return FileWriteResult{}, errors.New("worktree 写入目标不是普通文件或是符号链接")
		}
		operation = "modify"
		mode = info.Mode().Perm()
		previous, readErr := root.ReadFile(nativePath)
		if readErr != nil {
			return FileWriteResult{}, readErr
		}
		hash := sha256.Sum256(previous)
		previousSHA = hex.EncodeToString(hash[:])
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return FileWriteResult{}, statErr
	}
	temporary := filepath.Join(filepath.Dir(nativePath), "."+filepath.Base(nativePath)+"-btask-"+mustTemporaryID())
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return FileWriteResult{}, err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = root.Remove(temporary)
		}
	}()
	if _, err := file.Write(content); err != nil {
		file.Close()
		return FileWriteResult{}, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return FileWriteResult{}, err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return FileWriteResult{}, err
	}
	if err := file.Close(); err != nil {
		return FileWriteResult{}, err
	}
	if err := root.Rename(temporary, nativePath); err != nil {
		return FileWriteResult{}, err
	}
	removeTemporary = false
	hash := sha256.Sum256(content)
	return FileWriteResult{
		Path: normalized, Operation: operation, PreviousSHA: previousSHA,
		SHA256: hex.EncodeToString(hash[:]), ByteSize: int64(len(content)),
	}, nil
}

func normalizeWorktreePath(value string) (string, error) {
	normalized, err := permissions.NormalizeRelativePath(value)
	if err != nil {
		return "", fmt.Errorf("worktree 路径不安全: %w", err)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if strings.EqualFold(segment, ".git") {
			return "", errors.New("禁止访问 worktree 的 .git 元数据")
		}
		for _, character := range segment {
			if unicode.IsControl(character) {
				return "", errors.New("worktree 路径包含控制字符")
			}
		}
	}
	return normalized, nil
}

func validateExistingRegularFile(root *os.Root, pathValue string) error {
	if err := ensureSafeParents(root, filepath.Dir(pathValue), false); err != nil {
		return err
	}
	info, err := root.Lstat(pathValue)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("worktree 文件不存在、不是普通文件或是符号链接")
	}
	return nil
}

func ensureSafeParents(root *os.Root, parent string, create bool) error {
	if parent == "." || parent == "" {
		return nil
	}
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(parent), "/") {
		if segment == "" || segment == "." {
			continue
		}
		if err := rejectCaseAliasInRoot(root, current, segment); err != nil {
			return err
		}
		current = filepath.Join(current, filepath.FromSlash(segment))
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && create {
			if err := root.Mkdir(current, 0o755); err != nil {
				return err
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("worktree 路径父级不是目录或是符号链接")
		}
	}
	return nil
}

func rejectCaseAliasInRoot(root *os.Root, parent string, requested string) error {
	if parent == "" {
		parent = "."
	}
	directory, err := root.Open(parent)
	if err != nil {
		return err
	}
	names, readErr := directory.Readdirnames(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	for _, name := range names {
		if strings.EqualFold(name, requested) && name != requested {
			return fmt.Errorf("路径大小写 %q 与磁盘上的 %q 不一致", requested, name)
		}
	}
	return nil
}

func bytesContainNUL(value []byte) bool {
	for _, character := range value {
		if character == 0 {
			return true
		}
	}
	return false
}

func mustTemporaryID() string {
	identifier, err := randomHex(6)
	if err != nil {
		return fmt.Sprintf("%d", os.Getpid())
	}
	return identifier
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"
