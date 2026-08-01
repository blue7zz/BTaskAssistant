package agent

import (
	"encoding/json"
	"testing"
)

func TestDecodeMessageKeepsAssistantReasoningOutOfVisibleHistory(t *testing.T) {
	role, content := decodeMessage(json.RawMessage(`{
		"role":"assistant",
		"content":[
			{"type":"thinking","text":"内部推理"},
			{"type":"text","text":"用户可见回答"}
		]
	}`))
	if role != "assistant" || content != "用户可见回答" {
		t.Fatalf("unexpected visible message role=%q content=%q", role, content)
	}
}
