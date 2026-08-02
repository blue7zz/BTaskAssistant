package bridge

import (
	"sort"

	rxconfig "reasonix/internal/config"
)

// ModelInfoView 对应前端 ModelInfo。
type ModelInfoView struct {
	Name     string `json:"name"`
	Ref      string `json:"ref"`
	Provider string `json:"provider"`
	Kind     string `json:"kind,omitempty"`
	Current  bool   `json:"current,omitempty"`
}

// Models 从工作区配置解析可用模型列表（含用户全局配置，与桌面端
// ModelsForTab 同源：provider 的 ChatModelList，ref 为 "provider/model"）。
// currentRef 非空时标记当前模型。
func Models(workspaceRoot string, currentRef string) []ModelInfoView {
	cfg, err := rxconfig.LoadForRoot(workspaceRoot)
	if err != nil {
		return []ModelInfoView{}
	}
	out := make([]ModelInfoView, 0, 8)
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !p.Configured() {
			continue
		}
		for _, m := range p.ChatModelList() {
			ref := p.Name + "/" + m
			out = append(out, ModelInfoView{
				Ref:      ref,
				Provider: p.Name,
				Name:     m,
				Current:  ref == currentRef,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

// DefaultModel 返回工作区配置的默认模型 ref（空表示未配置）。
func DefaultModel(workspaceRoot string) string {
	cfg, err := rxconfig.LoadForRoot(workspaceRoot)
	if err != nil {
		return ""
	}
	if resolved, _, ok := cfg.ResolveNewSessionChatModel(); ok {
		return resolved
	}
	return ""
}
