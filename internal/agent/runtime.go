package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrPINotInstalled       = errors.New("原生 PI CLI 未安装")
	ErrUnsupportedPIVersion = errors.New("当前原生 PI 版本尚未通过兼容性验证")
)

type SessionState struct {
	Model               json.RawMessage `json:"model,omitempty"`
	ThinkingLevel       string          `json:"thinkingLevel"`
	IsStreaming         bool            `json:"isStreaming"`
	IsCompacting        bool            `json:"isCompacting"`
	SteeringMode        string          `json:"steeringMode"`
	FollowUpMode        string          `json:"followUpMode"`
	SessionFile         string          `json:"sessionFile,omitempty"`
	SessionID           string          `json:"sessionId"`
	SessionName         string          `json:"sessionName,omitempty"`
	AutoCompaction      bool            `json:"autoCompactionEnabled"`
	MessageCount        int             `json:"messageCount"`
	PendingMessageCount int             `json:"pendingMessageCount"`
}

type ProcessExit struct {
	Code       int
	Signal     string
	Err        error
	StderrTail string
}

type Runtime interface {
	Call(context.Context, string, map[string]any, any) error
	Events() <-chan rawEvent
	Done() <-chan struct{}
	Exit() ProcessExit
	StderrTail() string
	Close(context.Context) error
}

type RuntimeFactory interface {
	Start(context.Context, ProcessOptions) (Runtime, SessionState, error)
}

type RuntimeFactoryFunc func(
	context.Context,
	ProcessOptions,
) (Runtime, SessionState, error)

func (factory RuntimeFactoryFunc) Start(
	ctx context.Context,
	options ProcessOptions,
) (Runtime, SessionState, error) {
	return factory(ctx, options)
}

type nativeRuntimeFactory struct{}

func (nativeRuntimeFactory) Start(
	ctx context.Context,
	options ProcessOptions,
) (Runtime, SessionState, error) {
	return StartProcess(ctx, options)
}

type ProcessOptions struct {
	Executable     string
	PrefixArgs     []string
	AdditionalEnv  []string
	WorkDir        string
	ConfigDir      string
	SessionDir     string
	StartupTimeout time.Duration
	RequestTimeout time.Duration
	ShutdownGrace  time.Duration
	MaxFrameBytes  int
	MaxStderrBytes int
}
