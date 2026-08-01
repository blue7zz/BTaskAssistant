package execution

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func executionRequest(t *testing.T, runID string, toolID string, command string) RunRequest {
	t.Helper()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "runs", runID), 0o700); err != nil {
		t.Fatal(err)
	}
	return RunRequest{
		TaskID: "task_shell", SessionID: "session_shell", RunID: runID,
		ToolCallID: toolID, Command: command, CWD: workspace, CWDDisplay: ".",
		WorkspaceRoot: workspace,
	}
}

func TestRunCapturesStdoutStderrExitAndSafeEnvironmentNames(t *testing.T) {
	t.Setenv("BTASK_TEST_SECRET_TOKEN", "must-not-enter-shell")
	service := NewService()
	request := executionRequest(t, "run_output", "tool_output", `printf 'hello:%s' "${BTASK_TEST_SECRET_TOKEN-unset}"; printf 'problem' >&2; exit 7`)
	result, err := service.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.ExitCode != 7 || result.Stdout != "hello:unset" || result.Stderr != "problem" {
		t.Fatalf("unexpected command result %#v", result)
	}
	if result.StdoutRef == "" || result.StderrRef == "" || result.StdoutBytes != 11 || result.StderrBytes != 7 {
		t.Fatalf("command output metadata is incomplete: %#v", result)
	}
	stdout, err := os.ReadFile(filepath.Join(request.WorkspaceRoot, filepath.FromSlash(result.StdoutRef)))
	if err != nil || string(stdout) != "hello:unset" || strings.Contains(string(stdout), "must-not-enter-shell") {
		t.Fatalf("stdout log mismatch: %q, %v", stdout, err)
	}
	stderr, err := os.ReadFile(filepath.Join(request.WorkspaceRoot, filepath.FromSlash(result.StderrRef)))
	if err != nil || string(stderr) != "problem" {
		t.Fatalf("stderr log mismatch: %q, %v", stderr, err)
	}
	sensitive := regexp.MustCompile(`(?i)(token|secret|pass(?:word|phrase)?|credential|cookie|authorization|private|api.?key|(^|[_-])pat($|[_-])|auth[_-]?sock|askpass)`)
	for _, name := range result.Environment {
		if strings.Contains(name, "=") || sensitive.MatchString(name) {
			t.Fatalf("environment value or sensitive name leaked: %q", name)
		}
	}
}

func TestStopTerminatesBackgroundProcessGroup(t *testing.T) {
	service := NewService()
	request := executionRequest(t, "run_stop", "tool_stop", "sleep 30 & wait")
	resultChannel := make(chan Result, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, err := service.Run(context.Background(), request)
		resultChannel <- result
		errorChannel <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !service.ActiveTask(request.TaskID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !service.ActiveTask(request.TaskID) {
		t.Fatal("Shell command did not enter the running registry")
	}
	if err := service.Stop(StopRequest{
		TaskID: request.TaskID, SessionID: request.SessionID,
		RunID: request.RunID, ToolCallID: request.ToolCallID,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-resultChannel:
		if err := <-errorChannel; err != nil {
			t.Fatal(err)
		}
		if !result.Stopped || result.Success || result.DurationMS >= 10000 {
			t.Fatalf("background process group was not stopped promptly: %#v", result)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("stopped Shell command did not return")
	}
	if service.ActiveTask(request.TaskID) {
		t.Fatal("stopped Shell command remained active")
	}
}

func TestTimeoutAndOutputCapsAreRecorded(t *testing.T) {
	service := NewService()
	timeoutRequest := executionRequest(t, "run_timeout", "tool_timeout", "sleep 5")
	timeoutRequest.Timeout = 100 * time.Millisecond
	timedOut, err := service.Run(context.Background(), timeoutRequest)
	if err != nil || !timedOut.TimedOut || !timedOut.Stopped || timedOut.Success {
		t.Fatalf("timeout was not recorded: %#v, %v", timedOut, err)
	}

	largeRequest := executionRequest(t, "run_large", "tool_large", "head -c 17000000 /dev/zero")
	large, err := service.Run(context.Background(), largeRequest)
	if err != nil || !large.Success || !large.Truncated || large.StdoutBytes != 17000000 || len(large.Stdout) != maxOutputPreviewBytes {
		t.Fatalf("large output was not bounded: bytes=%d preview=%d result=%#v err=%v", large.StdoutBytes, len(large.Stdout), large, err)
	}
	info, err := os.Stat(filepath.Join(largeRequest.WorkspaceRoot, filepath.FromSlash(large.StdoutRef)))
	if err != nil || info.Size() != maxCommandOutputBytes {
		t.Fatalf("large stdout file cap mismatch: %#v, %v", info, err)
	}
}

func TestCloseStopsAllCommandsAndUnsafePathsAreRejected(t *testing.T) {
	service := NewService()
	request := executionRequest(t, "run_close", "tool_close", "sleep 30")
	done := make(chan struct{})
	go func() {
		_, _ = service.Run(context.Background(), request)
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !service.ActiveTask(request.TaskID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close returned before the Shell command stopped")
	}
	if _, err := service.Run(context.Background(), executionRequest(t, "run_closed", "tool_closed", "true")); err == nil {
		t.Fatal("closed execution service accepted a new command")
	}

	unsafe := executionRequest(t, "run_unsafe", "tool_unsafe", "true")
	symlink := filepath.Join(t.TempDir(), "cwd-link")
	if err := os.Symlink(unsafe.CWD, symlink); err != nil {
		t.Fatal(err)
	}
	unsafe.CWD = symlink
	if _, err := NewService().Run(context.Background(), unsafe); err == nil {
		t.Fatal("symlink cwd was accepted")
	}
}
