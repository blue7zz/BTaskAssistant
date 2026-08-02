package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UtilityImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type UtilityRequest struct {
	Prompt         string
	WorkDir        string
	Model          string
	ThinkingLevel  string
	ResourcePolicy string
	Images         []UtilityImage
	MaxOutputBytes int
}

type UtilityRunner struct {
	Executable     string
	RuntimeFactory RuntimeFactory
	StartupTimeout time.Duration
	RequestTimeout time.Duration
	ShutdownGrace  time.Duration
}

func RunUtility(ctx context.Context, request UtilityRequest) (string, error) {
	return (UtilityRunner{}).Run(ctx, request)
}

func (runner UtilityRunner) Run(
	ctx context.Context,
	request UtilityRequest,
) (string, error) {
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.Prompt == "" {
		return "", errors.New("PI utility prompt is required")
	}
	if len(request.Prompt) > 2*1024*1024 {
		return "", errors.New("PI utility prompt exceeds 2 MiB")
	}
	if err := validateThinkingLevel(request.ThinkingLevel); err != nil {
		return "", err
	}
	if request.MaxOutputBytes <= 0 {
		request.MaxOutputBytes = 2 * 1024 * 1024
	}
	images, err := validateUtilityImages(request.Images)
	if err != nil {
		return "", err
	}

	temporaryRoot, err := os.MkdirTemp("", "btask-pi-utility-*")
	if err != nil {
		return "", fmt.Errorf("创建 PI utility 临时目录失败: %w", err)
	}
	defer os.RemoveAll(temporaryRoot)
	if err := os.Chmod(temporaryRoot, 0o700); err != nil {
		return "", err
	}
	workDir := strings.TrimSpace(request.WorkDir)
	if workDir == "" {
		workDir = filepath.Join(temporaryRoot, "workspace")
		if err := os.Mkdir(workDir, 0o700); err != nil {
			return "", err
		}
	} else {
		workDir, err = filepath.Abs(workDir)
		if err != nil {
			return "", err
		}
		info, statErr := os.Stat(workDir)
		if statErr != nil || !info.IsDir() {
			return "", errors.New("PI utility 工作目录不可用")
		}
	}
	factory := runner.RuntimeFactory
	if factory == nil {
		factory = nativeRuntimeFactory{}
	}
	configDir, err := resolvePIConfigDirectory(
		request.ResourcePolicy,
		filepath.Join(temporaryRoot, "pi-agent"),
	)
	if err != nil {
		return "", err
	}
	options := ProcessOptions{
		Executable:     runner.Executable,
		WorkDir:        workDir,
		ConfigDir:      configDir,
		SessionDir:     filepath.Join(temporaryRoot, "pi-sessions"),
		StartupTimeout: runner.StartupTimeout,
		RequestTimeout: runner.RequestTimeout,
		ShutdownGrace:  runner.ShutdownGrace,
	}
	runtime, _, err := factory.Start(ctx, options)
	if err != nil {
		return "", err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownGrace)
		defer cancel()
		_ = runtime.Close(closeCtx)
	}()
	timeout := runner.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	if err := applyUtilitySettings(ctx, runtime, request.Model, request.ThinkingLevel, timeout); err != nil {
		return "", contextualizePICredentialError(err, request.ResourcePolicy)
	}
	fields := map[string]any{"message": request.Prompt}
	if len(images) > 0 {
		fields["images"] = images
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	err = runtime.Call(requestCtx, "prompt", fields, nil)
	cancel()
	if err != nil {
		return "", contextualizePICredentialError(err, request.ResourcePolicy)
	}

	var output strings.Builder
	var finalText string
	var runError string
	for {
		select {
		case raw, open := <-runtime.Events():
			if !open {
				<-runtime.Done()
				exit := runtime.Exit()
				return "", errors.New(processExitMessage(exit, ""))
			}
			switch raw.Type {
			case "message_update":
				delta := parseAssistantDelta(raw.JSON)
				if delta.Type == "text_delta" {
					if output.Len()+len(delta.Delta) > request.MaxOutputBytes {
						abortRuntime(runtime, timeout)
						return "", errors.New("PI utility output exceeds its limit")
					}
					output.WriteString(delta.Delta)
				} else if delta.Type == "error" {
					runError = strings.TrimSpace(delta.Reason)
				}
			case "message_end":
				role, content := messageFromRaw(raw.JSON)
				if role == "assistant" && content != "" {
					if len(content) > request.MaxOutputBytes {
						return "", errors.New("PI utility output exceeds its limit")
					}
					finalText = content
				}
			case "extension_error":
				runError = "PI utility extension error"
			case "tool_execution_start":
				abortRuntime(runtime, timeout)
				return "", errors.New("PI utility no-tools session attempted a tool call")
			case "agent_settled":
				if runError != "" {
					return "", errors.New(runError)
				}
				if finalText != "" {
					return finalText, nil
				}
				if output.Len() == 0 {
					return "", errors.New("PI utility returned no assistant text")
				}
				return output.String(), nil
			}
		case <-runtime.Done():
			return "", errors.New(processExitMessage(runtime.Exit(), ""))
		case <-ctx.Done():
			abortRuntime(runtime, timeout)
			return "", ctx.Err()
		}
	}
}

func applyUtilitySettings(
	ctx context.Context,
	runtime Runtime,
	model string,
	thinking string,
	timeout time.Duration,
) error {
	model = strings.TrimSpace(model)
	if model != "" {
		provider, modelID, found := strings.Cut(model, "/")
		if !found || strings.TrimSpace(provider) == "" || strings.TrimSpace(modelID) == "" {
			return errors.New("PI 模型需使用 provider/model 格式")
		}
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		err := runtime.Call(requestCtx, "set_model", map[string]any{
			"provider": provider, "modelId": modelID,
		}, nil)
		cancel()
		if err != nil {
			return err
		}
	}
	thinking = strings.ToLower(strings.TrimSpace(thinking))
	if thinking != "" {
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		err := runtime.Call(requestCtx, "set_thinking_level", map[string]any{"level": thinking}, nil)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func validateUtilityImages(images []UtilityImage) ([]map[string]string, error) {
	if len(images) > 5 {
		return nil, errors.New("PI utility accepts at most 5 images")
	}
	result := make([]map[string]string, 0, len(images))
	for _, image := range images {
		mimeType := strings.ToLower(strings.TrimSpace(image.MIMEType))
		switch mimeType {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return nil, errors.New("PI utility image MIME type is not supported")
		}
		decoded, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil || len(decoded) == 0 || len(decoded) > 4*1024*1024 {
			return nil, errors.New("PI utility image data is invalid or exceeds 4 MiB")
		}
		result = append(result, map[string]string{
			"type": "image", "data": image.Data, "mimeType": mimeType,
		})
	}
	return result, nil
}
