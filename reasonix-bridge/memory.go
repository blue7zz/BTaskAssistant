package bridge

import (
	rxmemory "reasonix/internal/memory"
)

// MemoryView 返回任务控制器的记忆视图（docs + facts + storeDir），
// 字段与前端 MemoryView 契约一致。
func (m *Manager) MemoryView(taskID string) (map[string]any, error) {
	tab := m.Tab(taskID)
	if tab == nil {
		return emptyMemoryView(), nil
	}
	set := tab.Ctrl.Memory()
	if set == nil {
		return emptyMemoryView(), nil
	}
	docs := make([]map[string]any, 0, len(set.Docs))
	for _, doc := range set.Docs {
		docs = append(docs, map[string]any{
			"path":      doc.Path,
			"scope":     string(doc.Scope),
			"directory": doc.Directory,
			"body":      doc.Body,
			"depth":     doc.Depth,
			"order":     doc.Order,
		})
	}
	facts := make([]map[string]any, 0, 16)
	for _, fact := range set.Store.ListAll() {
		facts = append(facts, map[string]any{
			"id":          fact.ID,
			"name":        fact.Name,
			"title":       fact.Title,
			"description": fact.Description,
			"type":        string(fact.Type),
			"scope":       string(fact.Scope),
			"createdAt":   fact.CreatedAt.UnixMilli(),
			"updatedAt":   fact.UpdatedAt.UnixMilli(),
		})
	}
	return map[string]any{
		"docs":                  docs,
		"facts":                 facts,
		"archives":              []any{},
		"scopes":                []any{},
		"instructionDiagnostics": []any{},
		"conflicts":             []any{},
		"lastRecall":            nil,
		"storeDir":              set.Store.Dir,
		"storeGlobalDir":        set.Store.GlobalDir,
		"available":             true,
	}, nil
}

func emptyMemoryView() map[string]any {
	return map[string]any{
		"docs": []any{}, "facts": []any{}, "archives": []any{}, "scopes": []any{},
		"instructionDiagnostics": []any{}, "conflicts": []any{}, "lastRecall": nil,
		"storeDir": "", "available": false,
	}
}

var _ = rxmemory.Scope("")
