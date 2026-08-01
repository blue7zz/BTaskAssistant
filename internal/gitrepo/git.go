package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	gitProbeTimeout    = 20 * time.Second
	gitMutationTimeout = 5 * time.Minute
	maxGitOutputBytes  = 4 * 1024 * 1024
)

type commandOutput struct {
	stdout    []byte
	stderr    []byte
	truncated bool
	exitCode  int
	total     int64
}

type cappedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	total     int64
	truncated bool
}

func (writer *cappedBuffer) Write(content []byte) (int, error) {
	written := len(content)
	writer.total += int64(written)
	remaining := writer.limit - writer.buffer.Len()
	if remaining > 0 {
		if remaining > written {
			remaining = written
		}
		_, _ = writer.buffer.Write(content[:remaining])
	}
	if writer.buffer.Len() < int(writer.total) {
		writer.truncated = true
	}
	return written, nil
}

func runGit(ctx context.Context, directory string, timeout time.Duration, args ...string) (commandOutput, error) {
	return runGitWithLimit(ctx, directory, timeout, maxGitOutputBytes, false, args...)
}

func runGitWithLimit(
	ctx context.Context,
	directory string,
	timeout time.Duration,
	stdoutLimit int,
	allowTruncated bool,
	args ...string,
) (commandOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = gitProbeTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	commandArgs := []string{"-c", "core.fsmonitor=false", "-c", "maintenance.auto=false"}
	if runtime.GOOS == "windows" {
		commandArgs = append(commandArgs, "-c", "core.longpaths=true")
	}
	commandArgs = append(commandArgs, "-C", directory)
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(requestContext, "git", commandArgs...)
	stdout := &cappedBuffer{limit: stdoutLimit}
	stderr := &cappedBuffer{limit: 64 * 1024}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := commandOutput{
		stdout: stdout.buffer.Bytes(), stderr: stderr.buffer.Bytes(),
		truncated: stdout.truncated || stderr.truncated, total: stdout.total,
	}
	if command.ProcessState != nil {
		result.exitCode = command.ProcessState.ExitCode()
	}
	if requestContext.Err() != nil {
		return result, requestContext.Err()
	}
	if err != nil {
		return result, fmt.Errorf("git %s: %w%s", strings.Join(args, " "), err, stderrSuffix(result.stderr))
	}
	if result.truncated && !allowTruncated {
		return result, errors.New("Git 输出超过安全上限")
	}
	return result, nil
}

func runGitAllowExitOne(ctx context.Context, directory string, args ...string) (commandOutput, error) {
	result, err := runGit(ctx, directory, gitProbeTimeout, args...)
	if err != nil && result.exitCode == 1 {
		return result, nil
	}
	return result, err
}

func stderrSuffix(stderr []byte) string {
	value := strings.TrimSpace(string(stderr))
	if value == "" {
		return ""
	}
	if len(value) > 500 {
		value = value[:500] + "…"
	}
	return ": " + value
}
