package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultStartupTimeout         = 8 * time.Second
	defaultRequestTimeout         = 10 * time.Second
	defaultShutdownGrace          = 2 * time.Second
	defaultStderrLimit            = 64 * 1024
	resourcePolicyIsolated        = "isolated"
	resourcePolicyExplicitInherit = "explicit-inherit"
)

var piVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:\D|$)`)

func normalizePIResourcePolicy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return resourcePolicyIsolated, nil
	}
	if value != resourcePolicyIsolated && value != resourcePolicyExplicitInherit {
		return "", errors.New("PI 资源策略不受支持")
	}
	return value, nil
}

func resolvePIConfigDirectory(resourcePolicy string, isolatedDirectory string) (string, error) {
	policy, err := normalizePIResourcePolicy(resourcePolicy)
	if err != nil {
		return "", err
	}
	if policy == resourcePolicyIsolated {
		return filepath.Abs(isolatedDirectory)
	}
	configured := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if configured == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("定位本机 PI 配置目录失败: %w", err)
		}
		configured = filepath.Join(home, ".pi", "agent")
	}
	return filepath.Abs(configured)
}

func ProbeInstalledPI(ctx context.Context) (string, string, error) {
	executable, err := resolvePIExecutable("")
	if err != nil {
		return "", "", err
	}
	directory, err := os.MkdirTemp("", "btask-pi-probe-*")
	if err != nil {
		return executable, "", err
	}
	defer os.RemoveAll(directory)
	if err := os.Chmod(directory, 0o700); err != nil {
		return executable, "", err
	}
	options := normalizeProcessOptions(ProcessOptions{
		WorkDir:    directory,
		ConfigDir:  directory,
		SessionDir: filepath.Join(directory, "sessions"),
	})
	if err := ensurePrivateDirectory(options.SessionDir); err != nil {
		return executable, "", err
	}
	version, err := readPIVersion(
		ctx,
		executable,
		nil,
		isolatedPIEnvironment(options),
		directory,
	)
	if err != nil {
		return executable, "", err
	}
	if err := validatePIVersion(version); err != nil {
		return executable, version, err
	}
	return executable, version, nil
}

type Process struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	client        *rpcClient
	stderr        *tailBuffer
	shutdownGrace time.Duration

	closing     atomic.Bool
	stdinOnce   sync.Once
	mutex       sync.Mutex
	exit        ProcessExit
	protocolErr error
	done        chan struct{}
	stderrDone  chan struct{}
}

func StartProcess(
	ctx context.Context,
	options ProcessOptions,
) (*Process, SessionState, error) {
	options = normalizeProcessOptions(options)
	executable, err := resolvePIExecutable(options.Executable)
	if err != nil {
		return nil, SessionState{}, err
	}
	if err := validateProcessWorkDir(options.WorkDir); err != nil {
		return nil, SessionState{}, fmt.Errorf("检查工作目录失败: %w", err)
	}
	for label, directory := range map[string]string{
		"PI 配置目录": options.ConfigDir,
		"PI 会话目录": options.SessionDir,
	} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return nil, SessionState{}, fmt.Errorf("准备%s失败: %w", label, err)
		}
	}
	environment := isolatedPIEnvironment(options)
	versionCtx, versionCancel := context.WithTimeout(ctx, options.StartupTimeout)
	defer versionCancel()
	version, err := readPIVersion(
		versionCtx,
		executable,
		options.PrefixArgs,
		environment,
		options.WorkDir,
	)
	if err != nil {
		return nil, SessionState{}, err
	}
	if err := validatePIVersion(version); err != nil {
		return nil, SessionState{}, err
	}

	args := append([]string(nil), options.PrefixArgs...)
	args = append(args,
		"--mode", "rpc",
		"--session-dir", options.SessionDir,
		"--no-extensions",
		"--no-skills",
		"--no-prompt-templates",
		"--no-themes",
		"--no-context-files",
		"--no-approve",
		"--offline",
	)
	if options.Gate == nil {
		args = append(args, "--no-tools")
	} else {
		if err := validateGateProcessOptions(*options.Gate); err != nil {
			return nil, SessionState{}, err
		}
		args = append(args, "-e", options.Gate.ExtensionPath, "--no-builtin-tools")
	}
	command := exec.Command(executable, args...)
	command.Dir = options.WorkDir
	command.Env = environment
	configureProcessGroup(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, SessionState{}, fmt.Errorf("open pi stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, SessionState{}, fmt.Errorf("open pi stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, SessionState{}, fmt.Errorf("open pi stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, SessionState{}, fmt.Errorf("启动原生 PI RPC 失败: %w", err)
	}

	process := &Process{
		cmd:           command,
		stdin:         stdin,
		client:        newRPCClient(stdout, stdin, options.MaxFrameBytes),
		stderr:        newTailBuffer(options.MaxStderrBytes),
		shutdownGrace: options.ShutdownGrace,
		done:          make(chan struct{}),
		stderrDone:    make(chan struct{}),
	}
	go func() {
		_, _ = io.Copy(process.stderr, stderr)
		close(process.stderrDone)
	}()
	go process.wait()
	go process.watchProtocol()

	probeCtx, probeCancel := context.WithTimeout(ctx, options.StartupTimeout)
	defer probeCancel()
	var state SessionState
	if err := process.Call(probeCtx, "get_state", nil, &state); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownGrace)
		defer cancel()
		_ = process.Close(closeCtx)
		return nil, SessionState{}, fmt.Errorf("原生 PI RPC 启动探针失败: %w", err)
	}
	if strings.TrimSpace(state.SessionID) == "" {
		closeCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownGrace)
		defer cancel()
		_ = process.Close(closeCtx)
		return nil, SessionState{}, errors.New("原生 PI RPC 状态缺少 sessionId")
	}
	if options.Gate != nil {
		if err := process.waitForGateHeartbeat(probeCtx, *options.Gate); err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownGrace)
			defer cancel()
			_ = process.Close(closeCtx)
			return nil, SessionState{}, err
		}
	}
	return process, state, nil
}

func (process *Process) Call(
	ctx context.Context,
	command string,
	fields map[string]any,
	result any,
) error {
	return process.client.Call(ctx, command, fields, result)
}

func (process *Process) Send(ctx context.Context, fields map[string]any) error {
	return process.client.Send(ctx, fields)
}

func (process *Process) waitForGateHeartbeat(
	ctx context.Context,
	gate GateProcessOptions,
) error {
	wantStatus := gate.Version + ":" + gate.Nonce
	for {
		select {
		case event, ok := <-process.Events():
			if !ok {
				return errors.New("PI gate extension ended before its heartbeat")
			}
			if event.Type == "extension_error" {
				return errors.New("PI gate extension failed during startup")
			}
			if event.Type != "extension_ui_request" {
				return fmt.Errorf("unexpected PI event %q before gate heartbeat", event.Type)
			}
			var heartbeat struct {
				Method     string `json:"method"`
				StatusKey  string `json:"statusKey"`
				StatusText string `json:"statusText"`
			}
			if err := json.Unmarshal(event.JSON, &heartbeat); err != nil {
				return fmt.Errorf("decode PI gate heartbeat: %w", err)
			}
			if heartbeat.Method != "setStatus" || heartbeat.StatusKey != "btask-gate" || heartbeat.StatusText != wantStatus {
				return errors.New("PI gate extension heartbeat did not match its configured version and nonce")
			}
			return nil
		case <-ctx.Done():
			return fmt.Errorf("wait for PI gate extension heartbeat: %w", ctx.Err())
		case <-process.Done():
			return errors.New("PI gate extension process ended during startup")
		}
	}
}

func (process *Process) Events() <-chan rawEvent {
	return process.client.Events()
}

func (process *Process) Done() <-chan struct{} {
	return process.done
}

func (process *Process) Exit() ProcessExit {
	process.mutex.Lock()
	defer process.mutex.Unlock()
	return process.exit
}

func (process *Process) StderrTail() string {
	return process.stderr.String()
}

func (process *Process) Close(ctx context.Context) error {
	process.closing.Store(true)
	process.stdinOnce.Do(func() { _ = process.stdin.Close() })
	if waitForProcess(ctx, process.done, process.shutdownGrace) {
		return nil
	}
	_ = terminateProcess(process.cmd)
	if waitForProcess(ctx, process.done, process.shutdownGrace) {
		return nil
	}
	_ = killProcess(process.cmd)
	select {
	case <-process.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (process *Process) wait() {
	// StdoutPipe requires all reads to finish before Wait closes the pipe. This
	// ordering also preserves a final truncated/invalid frame as the root error.
	<-process.client.Done()
	err := process.cmd.Wait()
	<-process.stderrDone
	clientErr := process.client.Err()
	exit := ProcessExit{Code: -1, Err: err, StderrTail: process.StderrTail()}
	if process.cmd.ProcessState != nil {
		exit.Code = process.cmd.ProcessState.ExitCode()
		exit.Signal = processSignal(process.cmd.ProcessState)
	}
	process.mutex.Lock()
	if process.protocolErr != nil {
		exit.Err = process.protocolErr
	} else if !process.closing.Load() &&
		clientErr != nil &&
		!errors.Is(clientErr, io.EOF) &&
		!errors.Is(clientErr, ErrRPCClosed) {
		exit.Err = clientErr
	}
	if exit.Code == 0 && process.closing.Load() && errors.Is(exit.Err, os.ErrProcessDone) {
		exit.Err = nil
	}
	process.exit = exit
	process.mutex.Unlock()
	close(process.done)
}

func (process *Process) watchProtocol() {
	<-process.client.Done()
	err := process.client.Err()
	if process.closing.Load() || errors.Is(err, io.EOF) || errors.Is(err, ErrRPCClosed) {
		return
	}
	process.mutex.Lock()
	process.protocolErr = err
	process.mutex.Unlock()
	_ = killProcess(process.cmd)
}

func waitForProcess(ctx context.Context, done <-chan struct{}, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func normalizeProcessOptions(options ProcessOptions) ProcessOptions {
	if options.StartupTimeout <= 0 {
		options.StartupTimeout = defaultStartupTimeout
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = defaultRequestTimeout
	}
	if options.ShutdownGrace <= 0 {
		options.ShutdownGrace = defaultShutdownGrace
	}
	if options.MaxFrameBytes <= 0 {
		options.MaxFrameBytes = MaxRPCFrameBytes
	}
	if options.MaxStderrBytes <= 0 {
		options.MaxStderrBytes = defaultStderrLimit
	}
	return options
}

func validateGateProcessOptions(options GateProcessOptions) error {
	if strings.TrimSpace(options.Version) == "" || strings.TrimSpace(options.Nonce) == "" || len(options.SHA256) != 64 {
		return errors.New("PI gate extension version, nonce and SHA-256 are required")
	}
	absolute, err := filepath.Abs(options.ExtensionPath)
	if err != nil || absolute != filepath.Clean(options.ExtensionPath) || filepath.Ext(absolute) != ".ts" {
		return errors.New("PI gate extension path must be an absolute TypeScript file")
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("PI gate extension path is not a regular file")
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return errors.New("PI gate extension cannot be read")
	}
	hash := sha256.Sum256(content)
	if fmt.Sprintf("%x", hash[:]) != options.SHA256 {
		return errors.New("PI gate extension SHA-256 does not match the embedded source")
	}
	return nil
}

func resolvePIExecutable(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("解析 PI 路径失败: %w", err)
		}
		info, err := os.Stat(absolute)
		if err != nil || info.IsDir() {
			return "", ErrPINotInstalled
		}
		return filepath.Clean(absolute), nil
	}
	path, err := exec.LookPath("pi")
	if err != nil {
		return "", ErrPINotInstalled
	}
	return path, nil
}

func ensurePrivateDirectory(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("directory path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is not a private directory")
	}
	return os.Chmod(absolute, 0o700)
}

func validateProcessWorkDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("directory path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is not a directory")
	}
	return nil
}

func readPIVersion(
	ctx context.Context,
	executable string,
	prefixArgs []string,
	environment []string,
	workDir string,
) (string, error) {
	args := append(append([]string(nil), prefixArgs...), "--version")
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workDir
	command.Env = environment
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("读取原生 PI 版本失败: %s", message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func validatePIVersion(value string) error {
	match := piVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 4 {
		return fmt.Errorf("%w: %q", ErrUnsupportedPIVersion, value)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	if major != 0 || minor != 82 || patch < 1 {
		return fmt.Errorf("%w: %s", ErrUnsupportedPIVersion, value)
	}
	return nil
}

func isolatedPIEnvironment(options ProcessOptions) []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true,
		"LANG": true, "LC_ALL": true, "SYSTEMROOT": true, "COMSPEC": true,
		"PATHEXT": true,
	}
	environment := make([]string, 0, len(allowed)+8+len(options.AdditionalEnv))
	for _, item := range os.Environ() {
		key, _, found := strings.Cut(item, "=")
		if found && allowed[strings.ToUpper(key)] {
			environment = append(environment, item)
		}
	}
	environment = append(environment,
		"PI_CODING_AGENT_DIR="+options.ConfigDir,
		"PI_CODING_AGENT_SESSION_DIR="+options.SessionDir,
		"PI_OFFLINE=1",
		"PI_TELEMETRY=0",
		"NO_COLOR=1",
	)
	for _, item := range options.AdditionalEnv {
		if strings.Contains(item, "=") {
			environment = append(environment, item)
		}
	}
	return environment
}

type tailBuffer struct {
	mutex sync.Mutex
	limit int
	data  []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (buffer *tailBuffer) Write(value []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	buffer.data = append(buffer.data, value...)
	if len(buffer.data) > buffer.limit {
		buffer.data = append([]byte(nil), buffer.data[len(buffer.data)-buffer.limit:]...)
	}
	return len(value), nil
}

func (buffer *tailBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return string(append([]byte(nil), buffer.data...))
}
