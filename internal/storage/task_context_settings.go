package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type TaskContextRootInfo struct {
	Path        string `json:"path"`
	DefaultPath string `json:"defaultPath"`
	Custom      bool   `json:"custom"`
	Available   bool   `json:"available"`
}

func (s *SQLiteStore) defaultTaskContextRoot() (string, error) {
	dataDirectory, err := s.dataDirectory()
	if err != nil || dataDirectory == "" {
		return "", err
	}
	return filepath.Join(dataDirectory, "tasks"), nil
}

func (s *SQLiteStore) taskContextRootWithConn(
	ctx context.Context,
	connection *sql.Conn,
) (string, error) {
	defaultRoot, err := s.defaultTaskContextRoot()
	if err != nil || defaultRoot == "" {
		return "", err
	}
	var customRoot sql.NullString
	err = connection.QueryRowContext(
		ctx,
		`SELECT custom_root FROM task_context_settings WHERE id = 1`,
	).Scan(&customRoot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if errors.Is(err, sql.ErrNoRows) || !customRoot.Valid || strings.TrimSpace(customRoot.String) == "" {
		if err := os.MkdirAll(defaultRoot, 0o700); err != nil {
			return "", err
		}
		return validateExistingTaskContextRoot(defaultRoot)
	}
	return validateExistingTaskContextRoot(customRoot.String)
}

func (s *SQLiteStore) TaskContextRootInfo() (TaskContextRootInfo, error) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	database, err := s.readyDatabase()
	if err != nil {
		return TaskContextRootInfo{}, err
	}
	defaultRoot, err := s.defaultTaskContextRoot()
	if err != nil {
		return TaskContextRootInfo{}, err
	}
	var customRoot sql.NullString
	err = database.QueryRow(
		`SELECT custom_root FROM task_context_settings WHERE id = 1`,
	).Scan(&customRoot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TaskContextRootInfo{}, err
	}
	path := defaultRoot
	custom := err == nil && customRoot.Valid && strings.TrimSpace(customRoot.String) != ""
	if custom {
		path = filepath.Clean(customRoot.String)
	} else if err := os.MkdirAll(path, 0o700); err != nil {
		return TaskContextRootInfo{}, fmt.Errorf("创建默认任务资料目录失败: %w", err)
	}
	info, statErr := os.Stat(path)
	return TaskContextRootInfo{
		Path:        path,
		DefaultPath: defaultRoot,
		Custom:      custom,
		Available:   statErr == nil && info.IsDir(),
	}, nil
}

func (s *SQLiteStore) SetTaskContextRoot(path string) (TaskContextRootInfo, error) {
	target, err := validateExistingTaskContextRoot(path)
	if err != nil {
		return TaskContextRootInfo{}, err
	}

	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return TaskContextRootInfo{}, err
	}

	err = withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		current, err := s.taskContextRootWithConn(ctx, connection)
		if err != nil {
			return err
		}
		same, err := sameDirectory(current, target)
		if err != nil {
			return err
		}
		if !same {
			if err := copyTaskContextRoot(current, target); err != nil {
				return err
			}
		}

		var payload string
		err = connection.QueryRowContext(
			ctx,
			`SELECT payload FROM workspace_state WHERE id = 1`,
		).Scan(&payload)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		contexts := map[string][]byte{}
		if err == nil {
			contexts, err = decodeTaskContexts(payload)
			if err != nil {
				return fmt.Errorf("decode task contexts: %w", err)
			}
		}
		if err := syncTaskContexts(target, contexts); err != nil {
			return fmt.Errorf("sync target task contexts: %w", err)
		}

		defaultRoot, err := s.defaultTaskContextRoot()
		if err != nil {
			return err
		}
		isDefault, err := sameDirectory(defaultRoot, target)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		var customRoot any = target
		if isDefault {
			customRoot = nil
		}
		_, err = connection.ExecContext(
			ctx,
			`INSERT INTO task_context_settings(id, custom_root, updated_at)
			 VALUES (1, ?, CURRENT_TIMESTAMP)
			 ON CONFLICT(id) DO UPDATE SET
			   custom_root = excluded.custom_root,
			   updated_at = CURRENT_TIMESTAMP`,
			customRoot,
		)
		return err
	})
	if err != nil {
		return TaskContextRootInfo{}, err
	}

	defaultRoot, err := s.defaultTaskContextRoot()
	if err != nil {
		return TaskContextRootInfo{}, err
	}
	isDefault, _ := sameDirectory(defaultRoot, target)
	return TaskContextRootInfo{
		Path:        target,
		DefaultPath: defaultRoot,
		Custom:      !isDefault,
		Available:   true,
	}, nil
}

func (s *SQLiteStore) ReconcileTaskContexts() error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	database, err := s.readyDatabase()
	if err != nil {
		return err
	}
	return withImmediateWrite(database, func(ctx context.Context, connection *sql.Conn) error {
		root, err := s.taskContextRootWithConn(ctx, connection)
		if err != nil {
			return err
		}
		var payload string
		err = connection.QueryRowContext(
			ctx,
			`SELECT payload FROM workspace_state WHERE id = 1`,
		).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return syncTaskContexts(root, map[string][]byte{})
		}
		if err != nil {
			return err
		}
		contexts, err := decodeTaskContexts(payload)
		if err != nil {
			return fmt.Errorf("decode task contexts: %w", err)
		}
		return syncTaskContexts(root, contexts)
	})
}

func validateExistingTaskContextRoot(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("任务资料目录不能为空")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("解析任务资料目录失败: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", fmt.Errorf("任务资料目录不可用: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("读取任务资料目录失败: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("任务资料路径不是目录")
	}
	return resolved, nil
}

func sameDirectory(left string, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, err
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func copyTaskContextRoot(source string, target string) error {
	if nestedDirectories(source, target) {
		return errors.New("新旧任务资料目录不能互相包含")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("请选择一个空目录作为新的任务资料目录")
	}

	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("任务资料目录包含不支持的符号链接: %s", relative)
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.Mkdir(destination, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("任务资料目录包含不支持的特殊文件: %s", relative)
		}
		return copyRegularFile(path, destination)
	})
}

func copyRegularFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	removeTarget := true
	defer func() {
		if removeTarget {
			_ = os.Remove(target)
		}
	}()
	sourceHash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, sourceHash), input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}

	copied, err := os.Open(target)
	if err != nil {
		return err
	}
	targetHash := sha256.New()
	_, copyErr := io.Copy(targetHash, copied)
	closeErr := copied.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !bytes.Equal(sourceHash.Sum(nil), targetHash.Sum(nil)) {
		return fmt.Errorf("验证迁移文件失败: %s", filepath.Base(source))
	}
	removeTarget = false
	return nil
}

func nestedDirectories(left string, right string) bool {
	leftRelative, leftErr := filepath.Rel(left, right)
	rightRelative, rightErr := filepath.Rel(right, left)
	return leftErr == nil && isNestedRelativePath(leftRelative) ||
		rightErr == nil && isNestedRelativePath(rightRelative)
}

func isNestedRelativePath(relative string) bool {
	return relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
