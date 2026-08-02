// settings.go — Reasonix 设置的内核读写（阶段 4）。
// 配置走内核 rxconfig（Load/Set*/UpsertProvider/SaveTo）；凭据只经内核
// 凭据机制（.env + CredentialResolver），返回 "已配置/未配置" 状态，
// 绝不返回明文。

package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	rxconfig "reasonix/internal/config"
)

// SettingsView 是宿主设置页的配置视图（凭据只含已配置状态）。
type SettingsView struct {
	ConfigPath          string                `json:"configPath"`
	HomePath            string                `json:"homePath"`
	Isolated            bool                  `json:"isolated"`
	DefaultModel        string                `json:"defaultModel"`
	PlannerModel        string                `json:"plannerModel"`
	DefaultToolApproval string                `json:"defaultToolApproval"`
	Effort              string                `json:"effort"`
	Models              []string              `json:"models"`
	Providers           []ProviderSettingView `json:"providers"`
	ReasoningLanguage   string                `json:"reasoningLanguage"`
	MaxSessionsPerTask  int                   `json:"maxSessionsPerTask"`
	MaxIdleRuntimes     int                   `json:"maxIdleRuntimes"`
}

// ProviderSettingView 是 Provider 的展示视图（无明文凭据）。
type ProviderSettingView struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"baseUrl"`
	KeySet  bool   `json:"keySet"`
}

// rxProviderKeySet 判断 provider 是否已配置凭据（不返回明文）。
func rxProviderKeySet(entry rxconfig.ProviderEntry, home string) bool {
	envName := strings.TrimSpace(entry.APIKeyEnv)
	if envName == "" {
		return false
	}
	return rxconfig.NewCredentialResolverForRoot(home).ResolveGlobalFirst(envName).Set
}

// Settings 返回内核真实配置（平铺视图）。
func Settings() (SettingsView, error) {
	cfg, err := rxconfig.Load()
	if err != nil {
		return SettingsView{}, fmt.Errorf("读取 Reasonix 配置失败: %w", err)
	}
	home := rxconfig.ReasonixHomeDir()
	providers := make([]ProviderSettingView, 0, len(cfg.Providers))
	for _, entry := range cfg.Providers {
		providers = append(providers, ProviderSettingView{
			Name:    entry.Name,
			Kind:    entry.Kind,
			BaseURL: entry.BaseURL,
			KeySet:  rxProviderKeySet(entry, home),
		})
	}
	approvalMode := cfg.DesktopDefaultToolApprovalMode()
	models := make([]string, 0)
	for _, model := range Models("", cfg.DefaultModel) {
		models = append(models, model.Ref)
	}
	return SettingsView{
		ConfigPath:          rxconfig.UserConfigPath(),
		HomePath:            home,
		Isolated:            rxconfig.IsolatedHomeDir() != "",
		DefaultModel:        cfg.DefaultModel,
		PlannerModel:        cfg.Agent.PlannerModel,
		DefaultToolApproval: approvalMode,
		Effort:              cfg.Agent.SubagentEffort,
		Models:              models,
		Providers:           providers,
		ReasoningLanguage:   cfg.Language,
		MaxSessionsPerTask:  MaxSessionsPerTask,
		MaxIdleRuntimes:     MaxIdleRuntimes,
	}, nil
}

// SetDefaultModel 设置默认模型（写入内核 config.toml）。
func SetDefaultModel(model string) error {
	cfg, err := rxconfig.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetDefaultModel(model); err != nil {
		return err
	}
	return cfg.SaveTo(rxconfig.UserConfigPath())
}

// SetPlannerModel 设置 Planner 模型（写入内核 config.toml）。
func SetPlannerModel(model string) error {
	cfg, err := rxconfig.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetPlannerModel(model); err != nil {
		return err
	}
	return cfg.SaveTo(rxconfig.UserConfigPath())
}

// SetApprovalMode 设置默认工具审批模式（写入内核 config.toml）。
func SetApprovalMode(mode string) error {
	cfg, err := rxconfig.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetDesktopDefaultToolApprovalMode(mode); err != nil {
		return err
	}
	return cfg.SaveTo(rxconfig.UserConfigPath())
}

// SaveProvider 新增/更新 Provider；key 写入内核 .env（0600，原子替换）。
func SaveProvider(name string, kind string, baseURL string, apiKeyEnv string, key string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("Provider 名称不能为空")
	}
	if strings.TrimSpace(apiKeyEnv) == "" && strings.TrimSpace(key) != "" {
		return errors.New("请先填写凭据环境变量名（APIKeyEnv）")
	}
	cfg, err := rxconfig.Load()
	if err != nil {
		return err
	}
	entry := rxconfig.ProviderEntry{
		Name:      strings.TrimSpace(name),
		Kind:      strings.TrimSpace(kind),
		BaseURL:   strings.TrimSpace(baseURL),
		APIKeyEnv: strings.TrimSpace(apiKeyEnv),
		Models:    []string{"default"},
	}
	if err := cfg.UpsertProvider(entry); err != nil {
		return err
	}
	if err := cfg.SaveTo(rxconfig.UserConfigPath()); err != nil {
		return err
	}
	if key != "" {
		if err := writeCredential(rxconfig.ReasonixHomeDir(), entry.APIKeyEnv, key); err != nil {
			return fmt.Errorf("保存凭据失败: %w", err)
		}
	}
	return nil
}

// RemoveProvider 删除 Provider。
func RemoveProvider(name string) error {
	cfg, err := rxconfig.Load()
	if err != nil {
		return err
	}
	next := make([]rxconfig.ProviderEntry, 0, len(cfg.Providers))
	for _, entry := range cfg.Providers {
		if entry.Name != name {
			next = append(next, entry)
		}
	}
	cfg.Providers = next
	return cfg.SaveTo(rxconfig.UserConfigPath())
}

// TestProvider 校验 Provider 配置状态（不发模型请求）。
func TestProvider(name string) (map[string]any, error) {
	cfg, err := rxconfig.Load()
	if err != nil {
		return map[string]any{}, err
	}
	home := rxconfig.ReasonixHomeDir()
	for _, entry := range cfg.Providers {
		if entry.Name == name {
			keySet := rxProviderKeySet(entry, home)
			return map[string]any{
				"ok":     keySet,
				"keySet": keySet,
				"reason": map[bool]string{true: "", false: "未配置 API Key"}[keySet],
			}, nil
		}
	}
	return map[string]any{"ok": false, "keySet": false, "reason": "Provider 不存在"}, nil
}

// writeCredential 写入 <home>/.env 中的 KEY=value（0600 权限，原子替换）。
func writeCredential(home string, envName string, value string) error {
	if home == "" || envName == "" {
		return errors.New("凭据目标无效")
	}
	path := filepath.Join(home, ".env")
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key := trimmed
		if idx := strings.IndexAny(trimmed, "=:"); idx >= 0 {
			key = strings.TrimSpace(trimmed[:idx])
		}
		if key == envName {
			lines[i] = envName + "=" + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, envName+"="+value)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteCredential 写入 <home>/.env 中的 KEY=value（供宿主凭据绑定）。
func WriteCredential(envName string, value string) error {
	return writeCredential(rxconfig.ReasonixHomeDir(), envName, value)
}

// ClearCredential 从 <home>/.env 移除 KEY。
func ClearCredential(envName string) error {
	home := rxconfig.ReasonixHomeDir()
	if home == "" || envName == "" {
		return errors.New("凭据目标无效")
	}
	path := filepath.Join(home, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // 无 .env 即视为已清除
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		key := trimmed
		if idx := strings.IndexAny(trimmed, "=:"); idx >= 0 {
			key = strings.TrimSpace(trimmed[:idx])
		}
		if key != envName {
			kept = append(kept, line)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
