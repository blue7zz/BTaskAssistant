package engine

import (
	"errors"
	"strings"
)

const (
	defaultPIThinkingEffort = "xhigh"
	defaultPITimeoutMinutes = 3
)

type PISettings struct {
	Model          string `json:"model"`
	ThinkingEffort string `json:"thinkingEffort"`
	TimeoutMinutes int    `json:"timeoutMinutes"`
}

func NormalizePISettings(settings PISettings) (PISettings, error) {
	settings.Model = strings.TrimSpace(settings.Model)
	if len(settings.Model) > 200 {
		return PISettings{}, errors.New("PI 模型标识不能超过 200 个字符")
	}

	settings.ThinkingEffort = strings.ToLower(
		strings.TrimSpace(settings.ThinkingEffort),
	)
	if settings.ThinkingEffort == "" || settings.ThinkingEffort == "max" {
		settings.ThinkingEffort = defaultPIThinkingEffort
	}
	switch settings.ThinkingEffort {
	case "low", "medium", "high", "xhigh":
	default:
		return PISettings{}, errors.New(
			"PI 思考强度仅支持 low、medium、high 或 xhigh",
		)
	}

	if settings.TimeoutMinutes == 0 {
		settings.TimeoutMinutes = defaultPITimeoutMinutes
	}
	if settings.TimeoutMinutes < 1 || settings.TimeoutMinutes > 10 {
		return PISettings{}, errors.New("PI 单次执行时限必须在 1 到 10 分钟之间")
	}
	return settings, nil
}
