package engine

import (
	"context"
	"errors"
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
	ID          string `json:"id"`
	Label       string `json:"label"`
	Configured  bool   `json:"configured"`
	Description string `json:"description"`
}

func Statuses() []Status {
	return []Status{
		{
			ID:          "pi",
			Label:       "PI / oh-my-pi",
			Configured:  false,
			Description: "适配器边界已预留；需确认本机 CLI 调用协议后启用。",
		},
		{
			ID:          "codex",
			Label:       "Codex",
			Configured:  false,
			Description: "当前可复制已确认提示词，自动执行接口待配置。",
		},
	}
}

