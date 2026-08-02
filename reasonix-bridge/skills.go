package bridge

import (
	"fmt"

	rxagent "reasonix/internal/agent"
	rxmemory "reasonix/internal/memory"
)

// QuickAdd 手动添加记忆（scope: "project"/"global"/"user"）。
func (m *Manager) QuickAdd(taskID string, scope string, note string) (string, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return "", fmt.Errorf("任务 %s 的 Reasonix 会话尚未初始化", taskID)
	}
	// 与桌面端 parseScope 一致：user→ScopeUser，其余（含 global/project）→ScopeProject
	var rxScope rxmemory.Scope
	if scope == "user" {
		rxScope = rxmemory.ScopeUser
	} else {
		rxScope = rxmemory.ScopeProject
	}
	return tab.Ctrl.QuickAdd(rxScope, note)
}

// SkillsView 返回任务控制器的技能视图（字段与前端 SkillView 契约一致）。
func (m *Manager) SkillsView(taskID string) []map[string]any {
	tab := m.Tab(taskID)
	if tab == nil {
		return []map[string]any{}
	}
	disabled := make(map[string]bool)
	for _, disabledSkill := range tab.Ctrl.DisabledSkills() {
		disabled[disabledSkill.Name] = true
	}
	skills := tab.Ctrl.Skills()
	out := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		out = append(out, map[string]any{
			"name":        skill.Name,
			"description": skill.Description,
			"scope":       string(skill.Scope),
			"enabled":     !disabled[skill.Name],
			"plugin":      skill.Plugin,
			"readOnly":    false,
		})
	}
	return out
}

// CapabilitiesView 返回能力视图（servers/skills/skillRoots/plugins）。
func (m *Manager) CapabilitiesView(taskID string) map[string]any {
	return map[string]any{
		"servers":    []any{},
		"skills":     m.SkillsView(taskID),
		"skillRoots": []any{},
		"plugins":    []any{},
	}
}

// RecoveryAction 是内核 agent.RecoveryAction 的公开别名（字符串常量），
// 供 rx_bindings 中转 Auto Guard 恢复卡 action 值。
type RecoveryAction = rxagent.RecoveryAction
