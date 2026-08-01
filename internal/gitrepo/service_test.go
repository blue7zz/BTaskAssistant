package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

type gitTestStore struct {
	mutex      sync.Mutex
	root       string
	bindings   map[string]storage.GitBindingRecord
	workspaces map[string]storage.TaskWorkspaceRecord
	statuses   map[string]string
}

func newGitTestStore(t *testing.T) *gitTestStore {
	t.Helper()
	return &gitTestStore{
		root:       t.TempDir(),
		bindings:   make(map[string]storage.GitBindingRecord),
		workspaces: make(map[string]storage.TaskWorkspaceRecord),
		statuses:   make(map[string]string),
	}
}

func (store *gitTestStore) EnsureTaskWorkspace(taskID string) (storage.TaskWorkspaceRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if workspace, exists := store.workspaces[taskID]; exists {
		return workspace, nil
	}
	root := filepath.Join(store.root, taskID)
	for _, directory := range []string{"repos", "runs", ".btask"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			return storage.TaskWorkspaceRecord{}, err
		}
	}
	workspace := storage.TaskWorkspaceRecord{TaskID: taskID, RootPath: root}
	store.workspaces[taskID] = workspace
	if store.statuses[taskID] == "" {
		store.statuses[taskID] = "development"
	}
	return workspace, nil
}

func (store *gitTestStore) GitBinding(taskID string) (storage.GitBindingRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, exists := store.bindings[taskID]
	if !exists {
		return storage.GitBindingRecord{}, storage.ErrAgentDataNotFound
	}
	return cloneGitBinding(record), nil
}

func (store *gitTestStore) UpsertGitBinding(record storage.GitBindingRecord) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if existing, exists := store.bindings[record.TaskID]; exists && existing.ID != record.ID {
		return errors.New("binding identity changed")
	}
	store.bindings[record.TaskID] = cloneGitBinding(record)
	return nil
}

func (store *gitTestStore) TaskStatus(taskID string) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	status := store.statuses[taskID]
	if status == "" {
		status = "development"
	}
	return status, nil
}

func (store *gitTestStore) setStatus(taskID string, status string) {
	store.mutex.Lock()
	store.statuses[taskID] = status
	store.mutex.Unlock()
}

func cloneGitBinding(record storage.GitBindingRecord) storage.GitBindingRecord {
	clone := record
	if record.WorktreePath != nil {
		value := *record.WorktreePath
		clone.WorktreePath = &value
	}
	if record.Branch != nil {
		value := *record.Branch
		clone.Branch = &value
	}
	if record.SourceBranch != nil {
		value := *record.SourceBranch
		clone.SourceBranch = &value
	}
	if record.ErrorMessage != nil {
		value := *record.ErrorMessage
		clone.ErrorMessage = &value
	}
	return clone
}

func createGitRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repository, "init")
	gitTest(t, repository, "config", "user.name", "BTask Test")
	gitTest(t, repository, "config", "user.email", "btask@example.invalid")
	writeTestFile(t, filepath.Join(repository, "README.md"), []byte("base\n"))
	writeTestFile(t, filepath.Join(repository, "old-name.txt"), []byte(strings.Repeat("rename source line\n", 20)))
	writeTestFile(t, filepath.Join(repository, "binary.bin"), []byte{0, 1, 2, 3, 4})
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-m", "initial")
	gitTest(t, repository, "remote", "add", "origin", "https://token:secret@example.invalid/team/repo.git?access_token=hidden")
	return repository
}

func gitTest(t *testing.T, directory string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBindCreatesTaskIsolatedWorktreesAndPreservesDirtySource(t *testing.T) {
	repository := createGitRepository(t)
	baseline := strings.TrimSpace(string(gitTest(t, repository, "rev-parse", "HEAD")))
	writeTestFile(t, filepath.Join(repository, "README.md"), []byte("dirty source\n"))
	writeTestFile(t, filepath.Join(repository, "source-only.txt"), []byte("untracked\n"))
	sourceStatusBefore := gitTest(t, repository, "status", "--porcelain=v1", "-z", "--untracked-files=all")

	store := newGitTestStore(t)
	service := NewService(store)
	first, err := service.Bind(context.Background(), BindRequest{TaskID: "task_a", SourcePath: repository})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Bind(context.Background(), BindRequest{TaskID: "task_b", SourcePath: filepath.Join(repository, ".git", "..")})
	if err != nil {
		t.Fatal(err)
	}
	if first.WorktreePath == nil || second.WorktreePath == nil || *first.WorktreePath == *second.WorktreePath {
		t.Fatalf("worktrees are not isolated: %#v %#v", first, second)
	}
	if first.Branch == nil || second.Branch == nil || *first.Branch == *second.Branch {
		t.Fatalf("task branches are not unique: %#v %#v", first.Branch, second.Branch)
	}
	if first.BaselineCommit != baseline || second.BaselineCommit != baseline || !first.SourceDirtyAtBind || !second.SourceDirtyAtBind {
		t.Fatalf("binding baseline/source status mismatch: %#v %#v", first, second)
	}
	if got := gitTest(t, repository, "status", "--porcelain=v1", "-z", "--untracked-files=all"); !bytes.Equal(got, sourceStatusBefore) {
		t.Fatalf("source status changed during bind: %q != %q", got, sourceStatusBefore)
	}
	if content, err := os.ReadFile(filepath.Join(*first.WorktreePath, "README.md")); err != nil || string(content) != "base\n" {
		t.Fatalf("task worktree did not use committed baseline: %q, %v", content, err)
	}

	if _, err := service.WriteFile(context.Background(), "task_a", "agent", "feature/new.txt", "task a\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, "feature", "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Agent write escaped into source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(*second.WorktreePath, "feature", "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Agent write crossed task worktrees: %v", err)
	}
	status, err := service.Status(context.Background(), "task_a")
	if err != nil || len(status.Files) != 1 || status.Files[0].Path != "feature/new.txt" {
		t.Fatalf("unexpected task status %#v, %v", status, err)
	}
	if status.RemoteURL != "https://example.invalid/team/repo.git" || strings.Contains(status.RemoteURL, "token") || strings.Contains(status.RemoteURL, "secret") {
		t.Fatalf("remote URL was not safely exposed: %q", status.RemoteURL)
	}

	restarted := NewService(store)
	restartedStatus, err := restarted.Status(context.Background(), "task_a")
	if err != nil || restartedStatus.Binding.ID != first.ID || restartedStatus.Binding.State != "ready" {
		t.Fatalf("binding did not survive service restart: %#v, %v", restartedStatus, err)
	}
	if got := gitTest(t, repository, "status", "--porcelain=v1", "-z", "--untracked-files=all"); !bytes.Equal(got, sourceStatusBefore) {
		t.Fatalf("source status changed after task write: %q != %q", got, sourceStatusBefore)
	}
}

func TestStatusAndDiffCoverStagedUnstagedUntrackedRenameDeleteAndLargeText(t *testing.T) {
	repository := createGitRepository(t)
	store := newGitTestStore(t)
	service := NewService(store)
	binding, err := service.Bind(context.Background(), BindRequest{TaskID: "task_diff", SourcePath: repository})
	if err != nil {
		t.Fatal(err)
	}
	root := *binding.WorktreePath
	writeTestFile(t, filepath.Join(root, "README.md"), []byte("staged line\n"))
	gitTest(t, root, "add", "README.md")
	file, err := os.OpenFile(filepath.Join(root, "README.md"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("unstaged line\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "untracked.txt"), []byte("new file\n"))
	if err := os.Rename(filepath.Join(root, "old-name.txt"), filepath.Join(root, "new-name.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "binary.bin")); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "add", "-A", "old-name.txt", "new-name.txt", "binary.bin")
	large := bytes.Repeat([]byte("large diff line\n"), 150000)
	writeTestFile(t, filepath.Join(root, "large.txt"), large)

	status, err := service.Status(context.Background(), "task_diff")
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]ChangedFile)
	for _, changed := range status.Files {
		byPath[changed.Path] = changed
	}
	if readme := byPath["README.md"]; !readme.Staged || !readme.Unstaged {
		t.Fatalf("README did not expose staged and unstaged changes: %#v", readme)
	}
	if byPath["untracked.txt"].Status != "untracked" || byPath["large.txt"].Status != "untracked" {
		t.Fatalf("untracked files were not reported: %#v", status.Files)
	}
	if byPath["new-name.txt"].Status != "renamed" || byPath["new-name.txt"].OriginalPath != "old-name.txt" {
		t.Fatalf("rename was not reported: %#v", byPath["new-name.txt"])
	}
	if byPath["binary.bin"].Status != "deleted" {
		t.Fatalf("delete was not reported: %#v", byPath["binary.bin"])
	}

	readmeDiff, err := service.Diff(context.Background(), "task_diff", "README.md")
	if err != nil || !strings.Contains(readmeDiff.Staged, "staged line") || !strings.Contains(readmeDiff.Unstaged, "unstaged line") {
		t.Fatalf("staged/unstaged diff mismatch: %#v, %v", readmeDiff, err)
	}
	untrackedDiff, err := service.Diff(context.Background(), "task_diff", "untracked.txt")
	if err != nil || !strings.Contains(untrackedDiff.Unstaged, "+new file") {
		t.Fatalf("untracked diff mismatch: %#v, %v", untrackedDiff, err)
	}
	binaryDiff, err := service.Diff(context.Background(), "task_diff", "binary.bin")
	if err != nil || !binaryDiff.Binary {
		t.Fatalf("binary diff mismatch: %#v, %v", binaryDiff, err)
	}
	largeDiff, err := service.Diff(context.Background(), "task_diff", "large.txt")
	if err != nil || !largeDiff.Truncated || largeDiff.ByteSize <= maxDiffBytes {
		t.Fatalf("large diff was not safely truncated: %#v, %v", largeDiff, err)
	}
}

func TestExplicitCommitCleanupRecoveryAndMissingSourceHandling(t *testing.T) {
	repository := createGitRepository(t)
	store := newGitTestStore(t)
	service := NewService(store)
	binding, err := service.Bind(context.Background(), BindRequest{TaskID: "task_lifecycle", SourcePath: repository})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.WriteFile(context.Background(), "task_lifecycle", "agent", "feature.txt", "feature\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Cleanup(context.Background(), CleanupRequest{TaskID: "task_lifecycle", Confirmed: true}); err == nil {
		t.Fatal("dirty worktree cleanup was accepted")
	}
	status, err := service.Status(context.Background(), "task_lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Commit(context.Background(), CommitRequest{
		TaskID: "task_lifecycle", Message: "feat(test): local", ExpectedSnapshot: status.Snapshot,
	}); err == nil {
		t.Fatal("unconfirmed local commit was accepted")
	}
	store.setStatus("task_lifecycle", "requirements")
	if _, err := service.Commit(context.Background(), CommitRequest{
		TaskID: "task_lifecycle", Message: "feat(test): local", ExpectedSnapshot: status.Snapshot, Confirmed: true,
	}); err == nil {
		t.Fatal("local commit outside development was accepted")
	}
	store.setStatus("task_lifecycle", "development")
	if _, err := service.Commit(context.Background(), CommitRequest{
		TaskID: "task_lifecycle", Message: "feat(test): local", ExpectedSnapshot: "stale", Confirmed: true,
	}); err == nil {
		t.Fatal("stale commit preview was accepted")
	}
	committed, err := service.Commit(context.Background(), CommitRequest{
		TaskID: "task_lifecycle", Message: "feat(test): local", ExpectedSnapshot: status.Snapshot, Confirmed: true,
	})
	if err != nil || committed.Commit == binding.BaselineCommit || len(committed.Status.Files) != 0 || committed.Status.AheadOfBaseline != 1 {
		t.Fatalf("unexpected commit result %#v, %v", committed, err)
	}
	if _, err := service.Cleanup(context.Background(), CleanupRequest{TaskID: "task_lifecycle"}); err == nil {
		t.Fatal("unconfirmed cleanup was accepted")
	}
	archived, err := service.Cleanup(context.Background(), CleanupRequest{TaskID: "task_lifecycle", Confirmed: true})
	if err != nil || archived.State != "archived" {
		t.Fatalf("clean worktree was not archived: %#v, %v", archived, err)
	}
	if _, err := os.Stat(*binding.WorktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree remained after cleanup: %v", err)
	}
	archivedStatus, err := service.Status(context.Background(), "task_lifecycle")
	if err != nil || archivedStatus.Binding.State != "archived" || archivedStatus.ErrorMessage == "" {
		t.Fatalf("archived status was not recoverable: %#v, %v", archivedStatus, err)
	}
	if _, err := service.Recover(context.Background(), RecoverRequest{TaskID: "task_lifecycle"}); err == nil {
		t.Fatal("unconfirmed recovery was accepted")
	}
	recovered, err := service.Recover(context.Background(), RecoverRequest{TaskID: "task_lifecycle", Confirmed: true})
	if err != nil || recovered.State != "ready" {
		t.Fatalf("worktree recovery failed: %#v, %v", recovered, err)
	}
	if head := strings.TrimSpace(string(gitTest(t, *recovered.WorktreePath, "rev-parse", "HEAD"))); head != committed.Commit {
		t.Fatalf("recovery lost the task commit: %s != %s", head, committed.Commit)
	}

	gitTest(t, repository, "worktree", "remove", "--force", *recovered.WorktreePath)
	missing, err := service.Status(context.Background(), "task_lifecycle")
	if err != nil || missing.Binding.State != "missing" || missing.ErrorMessage == "" {
		t.Fatalf("external deletion was not detected: %#v, %v", missing, err)
	}
	recovered, err = service.Recover(context.Background(), RecoverRequest{TaskID: "task_lifecycle", Confirmed: true})
	if err != nil || recovered.State != "ready" {
		t.Fatalf("externally deleted worktree was not recovered: %#v, %v", recovered, err)
	}

	moved := repository + "-moved"
	if err := os.Rename(repository, moved); err != nil {
		t.Fatal(err)
	}
	unavailable, err := service.Status(context.Background(), "task_lifecycle")
	if err != nil || unavailable.Binding.State != "missing" || !strings.Contains(unavailable.ErrorMessage, "移动") {
		t.Fatalf("moved source repository was not detected: %#v, %v", unavailable, err)
	}
}

func TestBindRejectsInvalidRepositoriesAndAcceptsSubdirectory(t *testing.T) {
	store := newGitTestStore(t)
	service := NewService(store)
	if _, err := service.Bind(context.Background(), BindRequest{TaskID: "task_plain", SourcePath: t.TempDir()}); err == nil {
		t.Fatal("non-Git directory was accepted")
	}
	bare := filepath.Join(t.TempDir(), "bare.git")
	if err := os.Mkdir(bare, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, bare, "init", "--bare")
	if _, err := service.Bind(context.Background(), BindRequest{TaskID: "task_bare", SourcePath: bare}); err == nil {
		t.Fatal("bare repository was accepted")
	}
	unborn := filepath.Join(t.TempDir(), "unborn")
	if err := os.Mkdir(unborn, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, unborn, "init")
	if _, err := service.Bind(context.Background(), BindRequest{TaskID: "task_unborn", SourcePath: unborn}); err == nil {
		t.Fatal("repository without HEAD was accepted")
	}
	repository := createGitRepository(t)
	subdirectory := filepath.Join(repository, "nested", "project")
	if err := os.MkdirAll(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	binding, err := service.Bind(context.Background(), BindRequest{TaskID: "task_subdir", SourcePath: subdirectory})
	realRepository, realRepositoryErr := filepath.EvalSymlinks(repository)
	realSubdirectory, realSubdirectoryErr := filepath.EvalSymlinks(subdirectory)
	if err != nil || realRepositoryErr != nil || realSubdirectoryErr != nil ||
		binding.SourceRealPath != realRepository || binding.SourcePath != realSubdirectory {
		t.Fatalf("repository subdirectory did not resolve safely: %#v, %v", binding, err)
	}
}
