package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("AI 引擎尚未配置")

type Request struct {
	TaskID      string            `json:"taskId"`
	Instruction string            `json:"instruction"`
	Evidence    map[string]string `json:"evidence"`
}

type Result struct {
	Content string `json:"content"`
	TraceID string `json:"traceId"`
}

// Adapter is the only contract the workflow layer may use for PI or Codex.
// The first release intentionally leaves command details unconfigured instead
// of guessing a CLI protocol.
type Adapter interface {
	Name() string
	Run(context.Context, Request) (Result, error)
}

type Status struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	Configured          bool   `json:"configured"`
	RequirementAnalysis bool   `json:"requirementAnalysis"`
	Development         bool   `json:"development"`
	Description         string `json:"description"`
	CommandPath         string `json:"commandPath"`
	Version             string `json:"version"`
}

func Statuses() []Status {
	piPath, piVersion, piErr := nativePICommandDetails()
	codexPath, codexVersion, codexErr := commandDetails("codex")
	return []Status{
		{
			ID:                  "pi",
			Label:               "PI",
			Configured:          piErr == nil,
			RequirementAnalysis: false,
			Development:         false,
			Description:         nativePIDescription(piErr),
			CommandPath:         piPath,
			Version:             piVersion,
		},
		{
			ID:                  "codex",
			Label:               "Codex",
			Configured:          codexErr == nil,
			RequirementAnalysis: codexErr == nil,
			Development:         false,
			Description:         requirementEngineDescription("Codex", codexErr),
			CommandPath:         codexPath,
			Version:             codexVersion,
		},
	}
}

func commandDetails(name string) (string, string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", "", err
	}
	output, err := exec.Command(path, "--version").Output()
	if err != nil {
		return path, "", nil
	}
	return path, strings.TrimSpace(string(output)), nil
}

func nativePICommandDetails() (string, string, error) {
	path, err := exec.LookPath("pi")
	if err != nil {
		return "", "", err
	}
	directory, err := os.MkdirTemp("", "btask-pi-probe-*")
	if err != nil {
		return path, "", err
	}
	defer os.RemoveAll(directory)
	if err := os.Chmod(directory, 0o700); err != nil {
		return path, "", err
	}
	sessionDirectory := directory + string(os.PathSeparator) + "sessions"
	if err := os.Mkdir(sessionDirectory, 0o700); err != nil {
		return path, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, "--version")
	command.Dir = directory
	isolatedEnvironment := environmentWithOverride(
		os.Environ(),
		"PI_CODING_AGENT_DIR",
		directory,
	)
	command.Env = environmentWithOverride(
		isolatedEnvironment,
		"PI_CODING_AGENT_SESSION_DIR",
		sessionDirectory,
	)
	output, err := command.Output()
	if ctx.Err() != nil {
		return path, "", ctx.Err()
	}
	if err != nil {
		return path, "", err
	}
	return path, strings.TrimSpace(string(output)), nil
}

func environmentWithOverride(environment []string, key string, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

func nativePIDescription(err error) string {
	if err != nil {
		return "原生 PI CLI 未安装；仍可人工整理需求或使用 Codex。"
	}
	return "已检测到原生 PI；RPC 分析与任务会话将在阶段 2 接入。"
}

func requirementEngineDescription(label string, err error) string {
	if err != nil {
		return label + " CLI 未安装；仍可人工整理需求。"
	}
	return label + " 可用于只读需求分析；开发执行仍采用外部委托。"
}
