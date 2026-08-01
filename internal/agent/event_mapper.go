package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

type assistantDelta struct {
	Type   string
	Delta  string
	Reason string
}

func parseAssistantDelta(raw json.RawMessage) assistantDelta {
	var event struct {
		Assistant struct {
			Type   string `json:"type"`
			Delta  string `json:"delta"`
			Reason string `json:"reason"`
		} `json:"assistantMessageEvent"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return assistantDelta{}
	}
	return assistantDelta{
		Type: event.Assistant.Type, Delta: event.Assistant.Delta, Reason: event.Assistant.Reason,
	}
}

func messageFromRaw(raw json.RawMessage) (string, string) {
	var event struct {
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return "", ""
	}
	return decodeMessage(event.Message)
}

func decodeMessage(raw json.RawMessage) (string, string) {
	var message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &message) != nil {
		return "", ""
	}
	if len(message.Content) == 0 {
		return strings.ToLower(message.Role), ""
	}
	var text string
	if json.Unmarshal(message.Content, &text) == nil {
		return strings.ToLower(message.Role), text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(message.Content, &blocks) != nil {
		return strings.ToLower(message.Role), ""
	}
	var result strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			result.WriteString(block.Text)
		}
	}
	return strings.ToLower(message.Role), result.String()
}

func queueCounts(raw json.RawMessage) (int, int) {
	var event struct {
		Steering []json.RawMessage `json:"steering"`
		FollowUp []json.RawMessage `json:"followUp"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return 0, 0
	}
	return len(event.Steering), len(event.FollowUp)
}

func toolEvent(raw json.RawMessage) (
	toolCallID string,
	toolName string,
	args json.RawMessage,
	output string,
	isError bool,
) {
	var event struct {
		ToolCallID string          `json:"toolCallId"`
		ToolName   string          `json:"toolName"`
		Args       json.RawMessage `json:"args"`
		Partial    json.RawMessage `json:"partialResult"`
		Result     json.RawMessage `json:"result"`
		IsError    bool            `json:"isError"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return "", "", nil, "", false
	}
	result := event.Partial
	if len(result) == 0 {
		result = event.Result
	}
	return event.ToolCallID, event.ToolName, event.Args, textFromToolResult(result), event.IsError
}

func textFromToolResult(raw json.RawMessage) string {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return ""
	}
	var text strings.Builder
	for _, item := range result.Content {
		if item.Type == "text" {
			text.WriteString(item.Text)
		}
	}
	return text.String()
}

func retryPayload(kind string, raw json.RawMessage) map[string]any {
	var event map[string]any
	_ = json.Unmarshal(raw, &event)
	payload := map[string]any{"state": kind}
	for _, key := range []string{"attempt", "maxAttempts", "delayMs", "success"} {
		if value, exists := event[key]; exists {
			payload[key] = value
		}
	}
	return payload
}

func compactionPayload(kind string, raw json.RawMessage) map[string]any {
	var event struct {
		Reason       string `json:"reason"`
		Aborted      bool   `json:"aborted"`
		ErrorMessage string `json:"errorMessage"`
	}
	_ = json.Unmarshal(raw, &event)
	payload := map[string]any{"state": kind, "reason": event.Reason}
	if event.Aborted {
		payload["state"] = "aborted"
	}
	if event.ErrorMessage != "" {
		payload["state"] = "failed"
		payload["error"] = map[string]any{
			"code": "unknown", "message": boundedText(event.ErrorMessage, 600), "retryable": true,
		}
	}
	return payload
}

func boundedText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func stableArgsPreview(raw json.RawMessage) *string {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	if len(raw) > 32*1024 {
		value := `{"truncated":true}`
		return &value
	}
	value := string(raw)
	return &value
}

func unexpectedToolMessage(name string) string {
	if strings.TrimSpace(name) == "" {
		name = "unknown"
	}
	return fmt.Sprintf("无工具 PI 会话收到了工具执行事件：%s", name)
}
