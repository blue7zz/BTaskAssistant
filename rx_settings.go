// rx_settings.go — Reasonix 设置绑定（阶段 4）。
// 配置读写实现在 reasonix-bridge/settings.go（内核 rxconfig）；本文件只做
// 绑定薄封装。凭据只经内核机制，前端只接收"已配置/未配置"。

package main

import (
	"os"
	"path/filepath"
	"strings"

	"reasonix/bridge"
)

// rxReasonixHomeMode 记录 Home 使用策略（BTask 数据目录内，与 SQLite 同级）。
const rxHomeModeFile = "reasonix-home-mode"

// rxIsolatedHomeDir 返回 BTask 隔离的 Reasonix Home 目录。
func rxIsolatedHomeDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "BTaskAssistant", "reasonix-home")
}

// ReasonixHomeInfo 返回当前 Reasonix Home 路径与使用策略。
func (a *App) ReasonixHomeInfo() (map[string]any, error) {
	mode := "shared"
	if data, err := os.ReadFile(filepath.Join(rxDataDirectory(), rxHomeModeFile)); err == nil {
		mode = strings.TrimSpace(string(data))
	}
	return map[string]any{
		"mode":      mode, // "isolated" | "shared"
		"isolated":  mode == "isolated",
		"available": rxIsolatedHomeDir() != "",
	}, nil
}

// ReasonixSetHomeIsolated 切换 Reasonix Home 使用策略（isolated/shared）。
// 持久化模式并设置进程内 REASONIX_HOME；已构建的会话控制器不受影响
// （下次激活/重建生效）。
func (a *App) ReasonixSetHomeIsolated(isolated bool) error {
	mode := "shared"
	if isolated {
		dir := rxIsolatedHomeDir()
		if dir == "" {
			return errNoUserConfigDir
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.Setenv("REASONIX_HOME", dir); err != nil {
			return err
		}
		mode = "isolated"
	} else {
		_ = os.Unsetenv("REASONIX_HOME")
	}
	return os.WriteFile(filepath.Join(rxDataDirectory(), rxHomeModeFile), []byte(mode), 0o644)
}

var errNoUserConfigDir = &reasonixError{msg: "无法解析 BTask 数据目录"}

type reasonixError struct{ msg string }

func (e *reasonixError) Error() string { return e.msg }

// ReasonixSettings 返回内核真实配置（平铺视图；凭据只含已配置状态）。
func (a *App) ReasonixSettings() (bridge.SettingsView, error) {
	return bridge.Settings()
}

// ReasonixProviders 返回 Provider 列表（不含明文凭据）。
func (a *App) ReasonixProviders() ([]bridge.ProviderSettingView, error) {
	view, err := bridge.Settings()
	if err != nil {
		return []bridge.ProviderSettingView{}, err
	}
	return view.Providers, nil
}

// ReasonixSetDefaultModel 设置默认模型（写入内核 config.toml）。
func (a *App) ReasonixSetDefaultModel(model string) error {
	return bridge.SetDefaultModel(model)
}

// ReasonixSetPlannerModel 设置 Planner 模型（写入内核 config.toml）。
func (a *App) ReasonixSetPlannerModel(model string) error {
	return bridge.SetPlannerModel(model)
}

// ReasonixSetApprovalMode 设置默认工具审批模式（写入内核 config.toml）。
func (a *App) ReasonixSetApprovalMode(mode string) error {
	return bridge.SetApprovalMode(mode)
}

// ReasonixSaveProvider 新增/更新 Provider。key 仅在本调用中出现，
// 不落 SQLite、不进入日志、不进入前端状态。
func (a *App) ReasonixSaveProvider(name string, kind string, baseURL string, apiKeyEnv string, key string) error {
	return bridge.SaveProvider(name, kind, baseURL, apiKeyEnv, key)
}

// ReasonixRemoveProvider 删除 Provider。
func (a *App) ReasonixRemoveProvider(name string) error {
	return bridge.RemoveProvider(name)
}

// ReasonixTestProvider 校验 Provider 配置状态（不发模型请求）。
func (a *App) ReasonixTestProvider(name string) (map[string]any, error) {
	return bridge.TestProvider(name)
}
