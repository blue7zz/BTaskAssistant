package engine

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/agent"
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

// Adapter keeps fixed workflow analysis behind a small engine boundary. Native
// PI interactive sessions use internal/agent's explicit RPC protocol instead.
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
			RequirementAnalysis: piErr == nil,
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return agent.ProbeInstalledPI(ctx)
}

func nativePIDescription(err error) string {
	if err != nil {
		return "原生 PI CLI 未安装；仍可人工整理需求或使用 Codex。"
	}
	return "已检测到原生 PI RPC；任务会话和无工具分析可用，模型调用需要显式凭据。"
}

func requirementEngineDescription(label string, err error) string {
	if err != nil {
		return label + " CLI 未安装；仍可人工整理需求。"
	}
	return label + " 可用于只读需求分析；开发执行仍采用外部委托。"
}
