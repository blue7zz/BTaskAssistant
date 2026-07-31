package report

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectGitContextCollectsDailySummaryWithoutSensitiveMetadata(t *testing.T) {
	repoPath := createGitContextTestRepo(t)
	trackedPath := filepath.Join(repoPath, "tracked.txt")
	if err := os.WriteFile(
		trackedPath,
		[]byte("committed\nVERY_PRIVATE_DIFF_CONTENT\n"),
		0o600,
	); err != nil {
		t.Fatalf("write working change: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "untracked.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	contextText, err := CollectGitContext(
		context.Background(),
		"2026-07-30",
		"Daily Tester",
		true,
		[]GitProject{{
			ProjectNo:   "BTA-1",
			ProjectName: "日报助手",
			Path:        repoPath,
		}},
	)
	if err != nil {
		t.Fatalf("collect Git context: %v", err)
	}

	for _, expected := range []string{
		"project_no: BTA-1",
		"project_name: 日报助手",
		"branch:",
		"subject: implement daily context",
		"stat: 1 file changed, 1 insertion(+)",
		" M tracked.txt",
		"?? untracked.txt",
		"working_tree_stat:",
	} {
		if !strings.Contains(contextText, expected) {
			t.Fatalf("expected context to contain %q, got:\n%s", expected, contextText)
		}
	}
	for _, forbidden := range []string{
		repoPath,
		"daily.tester@example.com",
		"https://user:remote-secret@example.com/private/repo.git",
		"remote-secret",
		"VERY_PRIVATE_DIFF_CONTENT",
	} {
		if strings.Contains(contextText, forbidden) {
			t.Fatalf("context leaked %q:\n%s", forbidden, contextText)
		}
	}
}

func TestCollectGitContextCanExcludeUncommittedState(t *testing.T) {
	repoPath := createGitContextTestRepo(t)
	if err := os.WriteFile(filepath.Join(repoPath, "private-uncommitted.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write uncommitted file: %v", err)
	}

	contextText, err := CollectGitContext(
		context.Background(),
		"2026-07-30",
		"",
		false,
		[]GitProject{{ProjectNo: "BTA-2", Path: repoPath}},
	)
	if err != nil {
		t.Fatalf("collect Git context: %v", err)
	}
	if strings.Contains(contextText, "uncommitted:") ||
		strings.Contains(contextText, "private-uncommitted.txt") {
		t.Fatalf("uncommitted state was included:\n%s", contextText)
	}
}

func TestCollectGitContextAllowsSupplementOnlyGeneration(t *testing.T) {
	contextText, err := CollectGitContext(
		context.Background(),
		"2026-07-30",
		"",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("collect empty Git context: %v", err)
	}
	if contextText != "No Git projects selected." {
		t.Fatalf("unexpected empty Git context %q", contextText)
	}
}

func TestCollectGitContextUsesRepositoryUserWhenAuthorIsEmpty(t *testing.T) {
	repoPath := createGitContextTestRepo(t)
	if err := os.WriteFile(filepath.Join(repoPath, "other.txt"), []byte("other\n"), 0o600); err != nil {
		t.Fatalf("write other author file: %v", err)
	}
	runGitContextTestCommand(t, repoPath, nil, "add", "other.txt")
	runGitContextTestCommand(
		t,
		repoPath,
		[]string{
			"GIT_AUTHOR_NAME=Other Author",
			"GIT_AUTHOR_EMAIL=other@example.com",
			"GIT_AUTHOR_DATE=2026-07-30T11:00:00+08:00",
			"GIT_COMMITTER_DATE=2026-07-30T11:00:00+08:00",
		},
		"commit",
		"-m",
		"other author change",
	)

	contextText, err := CollectGitContext(
		context.Background(),
		"2026-07-30",
		"",
		false,
		[]GitProject{{ProjectName: "日报助手", Path: repoPath}},
	)
	if err != nil {
		t.Fatalf("collect Git context: %v", err)
	}
	if !strings.Contains(contextText, "implement daily context") ||
		strings.Contains(contextText, "other author change") {
		t.Fatalf("repository user filter was not applied:\n%s", contextText)
	}
}

func TestCollectGitContextRejectsNonGitDirectoryWithoutLeakingPath(t *testing.T) {
	directory := t.TempDir()
	_, err := CollectGitContext(
		context.Background(),
		"2026-07-30",
		"",
		false,
		[]GitProject{{ProjectNo: "BTA-3", Path: directory}},
	)
	if err == nil || !strings.Contains(err.Error(), "不是 Git 仓库") {
		t.Fatalf("expected non-Git directory error, got %v", err)
	}
	if strings.Contains(err.Error(), directory) {
		t.Fatalf("error leaked repository path: %v", err)
	}
}

func createGitContextTestRepo(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	runGitContextTestCommand(t, repoPath, nil, "init", "-b", "main")
	runGitContextTestCommand(t, repoPath, nil, "config", "user.name", "Daily Tester")
	runGitContextTestCommand(t, repoPath, nil, "config", "user.email", "daily.tester@example.com")
	runGitContextTestCommand(
		t,
		repoPath,
		nil,
		"remote",
		"add",
		"origin",
		"https://user:remote-secret@example.com/private/repo.git",
	)
	if err := os.WriteFile(filepath.Join(repoPath, "tracked.txt"), []byte("committed\n"), 0o600); err != nil {
		t.Fatalf("write committed file: %v", err)
	}
	runGitContextTestCommand(t, repoPath, nil, "add", "tracked.txt")
	runGitContextTestCommand(
		t,
		repoPath,
		[]string{
			"GIT_AUTHOR_DATE=2026-07-30T10:00:00+08:00",
			"GIT_COMMITTER_DATE=2026-07-30T10:00:00+08:00",
		},
		"commit",
		"-m",
		"implement daily context for daily.tester@example.com in "+repoPath+
			" from https://user:remote-secret@example.com/private/repo.git",
	)
	return repoPath
}

func runGitContextTestCommand(
	t *testing.T,
	repoPath string,
	environment []string,
	args ...string,
) {
	t.Helper()
	commandArgs := append([]string{"-C", repoPath}, args...)
	command := exec.Command("git", commandArgs...)
	command.Env = append(os.Environ(), environment...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run git %v: %v\n%s", args, err, output)
	}
}
