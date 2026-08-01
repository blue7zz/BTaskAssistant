package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxCommandBytes       = 64 * 1024
	maxCommandOutputBytes = 16 * 1024 * 1024
	maxOutputPreviewBytes = 32 * 1024
	defaultTimeout        = 10 * time.Minute
	maximumTimeout        = time.Hour
	stopGrace             = 2 * time.Second
)

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,199}$`)
var credentialEnvironmentName = regexp.MustCompile(`(?i)(token|secret|pass(?:word|phrase)?|credential|cookie|authorization|private|api.?key|(^|[_-])pat($|[_-])|auth[_-]?sock|askpass)`)

type RunRequest struct {
	TaskID        string
	SessionID     string
	RunID         string
	ToolCallID    string
	Command       string
	CWD           string
	CWDDisplay    string
	WorkspaceRoot string
	Timeout       time.Duration
}

type StopRequest struct {
	TaskID     string `json:"taskId"`
	SessionID  string `json:"sessionId"`
	RunID      string `json:"runId"`
	ToolCallID string `json:"toolCallId"`
}

type Result struct {
	TaskID       string   `json:"taskId"`
	SessionID    string   `json:"sessionId"`
	RunID        string   `json:"runId"`
	ToolCallID   string   `json:"toolCallId"`
	Command      string   `json:"command"`
	CWD          string   `json:"cwd"`
	Environment  []string `json:"environment"`
	Success      bool     `json:"success"`
	ExitCode     int      `json:"exitCode"`
	Stopped      bool     `json:"stopped"`
	TimedOut     bool     `json:"timedOut"`
	DurationMS   int64    `json:"durationMs"`
	Stdout       string   `json:"stdout"`
	Stderr       string   `json:"stderr"`
	StdoutRef    string   `json:"stdoutRef"`
	StderrRef    string   `json:"stderrRef"`
	StdoutBytes  int64    `json:"stdoutBytes"`
	StderrBytes  int64    `json:"stderrBytes"`
	Truncated    bool     `json:"truncated"`
	ErrorMessage string   `json:"errorMessage,omitempty"`
}

type Service struct {
	mutex   sync.Mutex
	running map[string]*runningCommand
	closed  bool
}

type runningCommand struct {
	request  RunRequest
	command  *exec.Cmd
	done     chan struct{}
	stopping bool
	stopped  bool
}

type boundedOutput struct {
	file      *os.File
	preview   bytes.Buffer
	total     int64
	written   int64
	truncated bool
}

func NewService() *Service {
	return &Service{running: make(map[string]*runningCommand)}
}

func (service *Service) Run(ctx context.Context, request RunRequest) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if request.Timeout <= 0 {
		request.Timeout = defaultTimeout
	}
	if request.Timeout > maximumTimeout {
		return Result{}, errors.New("Shell 超时不能超过 1 小时")
	}
	stdout, stderr, stdoutRef, stderrRef, err := prepareOutputs(request)
	if err != nil {
		return Result{}, err
	}
	removeOutputs := true
	defer func() {
		if removeOutputs {
			_ = os.Remove(stdout.file.Name())
			_ = os.Remove(stderr.file.Name())
		}
	}()
	command := shellCommand(request.Command)
	command.Dir = request.CWD
	commandEnvironment, environmentNames := safeShellEnvironment()
	command.Env = commandEnvironment
	configureProcessGroup(command)
	command.Stdout = stdout
	command.Stderr = stderr
	startedAt := time.Now()
	if err := command.Start(); err != nil {
		stdout.close()
		stderr.close()
		return Result{}, fmt.Errorf("启动 Shell 命令失败: %w", err)
	}
	running := &runningCommand{request: request, command: command, done: make(chan struct{})}
	key := requestKey(request.TaskID, request.SessionID, request.RunID, request.ToolCallID)
	service.mutex.Lock()
	if service.closed {
		service.mutex.Unlock()
		_ = terminateProcessTree(command, true)
		_ = command.Wait()
		stdout.close()
		stderr.close()
		return Result{}, errors.New("Shell 执行服务已关闭")
	}
	if service.running[key] != nil {
		service.mutex.Unlock()
		_ = terminateProcessTree(command, true)
		_ = command.Wait()
		stdout.close()
		stderr.close()
		return Result{}, errors.New("同一工具已有正在执行的 Shell 命令")
	}
	service.running[key] = running
	service.mutex.Unlock()

	runContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-runContext.Done():
			_ = service.stopKey(key)
		case <-watchDone:
		}
	}()
	waitErr := command.Wait()
	close(watchDone)
	close(running.done)
	service.mutex.Lock()
	stopped := running.stopped
	delete(service.running, key)
	service.mutex.Unlock()
	stdoutErr := stdout.close()
	stderrErr := stderr.close()
	removeOutputs = false
	if stdoutErr != nil || stderrErr != nil {
		return Result{}, errors.Join(stdoutErr, stderrErr)
	}
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	result := Result{
		TaskID: request.TaskID, SessionID: request.SessionID,
		RunID: request.RunID, ToolCallID: request.ToolCallID,
		Command: request.Command, CWD: request.CWDDisplay,
		Environment: environmentNames, ExitCode: exitCode, Stopped: stopped,
		TimedOut:   errors.Is(runContext.Err(), context.DeadlineExceeded),
		DurationMS: time.Since(startedAt).Milliseconds(),
		Stdout:     stdout.previewText(), Stderr: stderr.previewText(),
		StdoutRef: stdoutRef, StderrRef: stderrRef,
		StdoutBytes: stdout.total, StderrBytes: stderr.total,
		Truncated: stdout.truncated || stderr.truncated,
	}
	result.Success = waitErr == nil && !result.Stopped && !result.TimedOut
	if waitErr != nil {
		result.ErrorMessage = boundedError(waitErr.Error())
	}
	if result.TimedOut {
		result.ErrorMessage = "Shell 命令执行超时并已停止"
	} else if result.Stopped && result.ErrorMessage == "" {
		result.ErrorMessage = "Shell 命令已停止"
	}
	return result, nil
}

func (service *Service) Stop(request StopRequest) error {
	for _, value := range []string{request.TaskID, request.SessionID, request.RunID, request.ToolCallID} {
		if !identityPattern.MatchString(value) {
			return errors.New("Shell 停止请求身份无效")
		}
	}
	return service.stopKey(requestKey(request.TaskID, request.SessionID, request.RunID, request.ToolCallID))
}

func (service *Service) StopRun(taskID string, sessionID string, runID string) {
	service.mutex.Lock()
	keys := make([]string, 0)
	prefix := requestKey(taskID, sessionID, runID, "")
	for key := range service.running {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	service.mutex.Unlock()
	for _, key := range keys {
		_ = service.stopKey(key)
	}
}

func (service *Service) ActiveTask(taskID string) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	prefix := taskID + "\x00"
	for key := range service.running {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (service *Service) Close(ctx context.Context) error {
	service.mutex.Lock()
	service.closed = true
	keys := make([]string, 0, len(service.running))
	commands := make([]*runningCommand, 0, len(service.running))
	for key, command := range service.running {
		keys = append(keys, key)
		commands = append(commands, command)
	}
	service.mutex.Unlock()
	for _, key := range keys {
		_ = service.stopKey(key)
	}
	for _, command := range commands {
		select {
		case <-command.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (service *Service) stopKey(key string) error {
	service.mutex.Lock()
	running := service.running[key]
	if running == nil {
		service.mutex.Unlock()
		return errors.New("没有匹配的运行中 Shell 命令")
	}
	if running.stopping {
		service.mutex.Unlock()
		return nil
	}
	running.stopping = true
	running.stopped = true
	command := running.command
	done := running.done
	service.mutex.Unlock()
	if err := terminateProcessTree(command, false); err != nil {
		_ = terminateProcessTree(command, true)
		return err
	}
	go func() {
		select {
		case <-done:
		case <-time.After(stopGrace):
			_ = terminateProcessTree(command, true)
		}
	}()
	return nil
}

func validateRequest(request RunRequest) error {
	for label, value := range map[string]string{
		"task": request.TaskID, "session": request.SessionID,
		"run": request.RunID, "tool": request.ToolCallID,
	} {
		if !identityPattern.MatchString(value) {
			return fmt.Errorf("Shell %s identity is invalid", label)
		}
	}
	if strings.TrimSpace(request.Command) == "" || len([]byte(request.Command)) > maxCommandBytes || strings.ContainsRune(request.Command, '\x00') {
		return errors.New("Shell 命令不能为空、不得包含 NUL 且不能超过 64 KiB")
	}
	for label, value := range map[string]string{"cwd": request.CWD, "workspace": request.WorkspaceRoot} {
		if !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("Shell %s path is invalid", label)
		}
		info, err := os.Lstat(value)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Shell %s path is unavailable or unsafe", label)
		}
	}
	return nil
}

func prepareOutputs(request RunRequest) (*boundedOutput, *boundedOutput, string, string, error) {
	runDirectory := filepath.Join(request.WorkspaceRoot, "runs", request.RunID)
	info, err := os.Lstat(runDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, "", "", errors.New("Shell 运行日志目录不存在或不安全")
	}
	root, err := os.OpenRoot(runDirectory)
	if err != nil {
		return nil, nil, "", "", err
	}
	defer root.Close()
	stdoutName := "shell-" + request.ToolCallID + ".stdout.log"
	stderrName := "shell-" + request.ToolCallID + ".stderr.log"
	stdoutFile, err := root.OpenFile(stdoutName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, "", "", err
	}
	stderrFile, err := root.OpenFile(stderrName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		stdoutFile.Close()
		_ = root.Remove(stdoutName)
		return nil, nil, "", "", err
	}
	stdoutRef := "runs/" + request.RunID + "/" + stdoutName
	stderrRef := "runs/" + request.RunID + "/" + stderrName
	return &boundedOutput{file: stdoutFile}, &boundedOutput{file: stderrFile}, stdoutRef, stderrRef, nil
}

func (output *boundedOutput) Write(content []byte) (int, error) {
	written := len(content)
	output.total += int64(written)
	fileRemaining := int64(maxCommandOutputBytes) - output.written
	if fileRemaining > 0 {
		chunk := content
		if int64(len(chunk)) > fileRemaining {
			chunk = chunk[:fileRemaining]
		}
		count, err := output.file.Write(chunk)
		output.written += int64(count)
		if err != nil {
			return 0, err
		}
	}
	previewRemaining := maxOutputPreviewBytes - output.preview.Len()
	if previewRemaining > 0 {
		chunk := content
		if len(chunk) > previewRemaining {
			chunk = chunk[:previewRemaining]
		}
		_, _ = output.preview.Write(chunk)
	}
	if output.total > maxCommandOutputBytes || output.total > int64(maxOutputPreviewBytes) {
		output.truncated = true
	}
	return written, nil
}

func (output *boundedOutput) close() error {
	if output == nil || output.file == nil {
		return nil
	}
	if err := output.file.Sync(); err != nil {
		_ = output.file.Close()
		return err
	}
	return output.file.Close()
}

func (output *boundedOutput) previewText() string {
	value := output.preview.Bytes()
	for len(value) > 0 && !utf8.Valid(value) {
		value = value[:len(value)-1]
	}
	return string(value)
}

func shellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/D", "/S", "/C", command)
	}
	return exec.Command("/bin/sh", "-c", command)
}

func requestKey(taskID string, sessionID string, runID string, toolCallID string) string {
	return taskID + "\x00" + sessionID + "\x00" + runID + "\x00" + toolCallID
}

func safeShellEnvironment() ([]string, []string) {
	environment := make([]string, 0)
	names := make([]string, 0)
	seen := make(map[string]bool)
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found || name == "" || credentialEnvironmentName.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		environment = append(environment, entry)
		names = append(names, name)
	}
	sort.Strings(environment)
	sort.Strings(names)
	return environment, names
}

func boundedError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		value = value[:500] + "…"
	}
	return value
}

var _ io.Writer = (*boundedOutput)(nil)
