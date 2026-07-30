package engine

import (
	"context"
	"errors"
	"os/exec"
	"strings"
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
	piPath, piVersion, piErr := commandDetails("omp")
	codexPath, codexVersion, codexErr := commandDetails("codex")
	return []Status{
		{
			ID:                  "pi",
			Label:               "PI / oh-my-pi",
			Configured:          piErr == nil,
			RequirementAnalysis: piErr == nil,
			Development:         false,
			Description:         requirementEngineDescription("PI", piErr),
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

func requirementEngineDescription(label string, err error) string {
	if err != nil {
		return label + " CLI 未安装；仍可人工整理需求。"
	}
	return label + " 可用于只读需求分析；开发执行仍采用外部委托。"
}
