package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	permissionpolicy "github.com/blue7zz/BTaskAssistant/internal/permissions"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

const maxToolOutputFileBytes = 4 * 1024 * 1024

var secretTextPattern = regexp.MustCompile(`(?i)(token|pat|password|secret|api[_-]?key|authorization|cookie|credential|private[_-]?key)(\s*[:=]\s*)([^\s,;]+)`)
var secretHeaderPattern = regexp.MustCompile(`(?im)(authorization|cookie)(\s*:\s*)[^\r\n]+`)

func (service *Service) handleRawEventLocked(
	managed *managedSession,
	raw rawEvent,
	workspaceRoot string,
) {
	if managed.run != nil {
		auditJSON := redactAuditJSON(raw.JSON)
		_ = appendRunFile(workspaceRoot, managed.run.record.StdoutPath, append(auditJSON, '\n'))
	}
	switch raw.Type {
	case "agent_start":
		if managed.run == nil {
			return
		}
		managed.run.record.State = "running"
		_ = service.store.UpsertExecutionRun(managed.run.record)
		_ = service.emitLocked(managed, "run.state", managed.run.record.ID, "", map[string]any{
			"state": "running",
		})
	case "message_start":
		role, _ := messageFromRaw(raw.JSON)
		if role == "assistant" {
			service.ensureAssistantMessageLocked(managed)
		}
	case "message_update":
		service.handleMessageUpdateLocked(managed, raw.JSON, workspaceRoot)
	case "message_end":
		service.handleMessageEndLocked(managed, raw.JSON)
	case "agent_end":
		var event struct {
			WillRetry bool `json:"willRetry"`
		}
		_ = json.Unmarshal(raw.JSON, &event)
		if event.WillRetry && managed.run != nil {
			_ = service.emitLocked(managed, "retry.state", managed.run.record.ID, "", map[string]any{
				"state": "scheduled",
			})
		}
	case "agent_settled":
		if managed.run == nil {
			return
		}
		runID := managed.run.record.ID
		_ = service.emitLocked(managed, "agent.settled", runID, "", map[string]any{})
		state := "succeeded"
		reason := managed.run.errorMessage
		if managed.run.stopRequested {
			state = "cancelled"
			reason = "用户已停止本次回答"
		} else if reason != "" {
			state = "failed"
		}
		service.finishRunLocked(managed, state, reason)
		if managed.runtime != nil {
			ctx, cancel := context.WithTimeout(context.Background(), service.requestTimeout)
			_ = service.syncEntriesLocked(ctx, managed)
			cancel()
		}
	case "queue_update":
		if managed.run == nil {
			return
		}
		steering, followUp := queueCounts(raw.JSON)
		_ = service.emitLocked(managed, "queue.updated", managed.run.record.ID, "", map[string]any{
			"steeringCount": steering, "followUpCount": followUp,
		})
	case "compaction_start":
		service.emitRunEventLocked(managed, "compaction.state", compactionPayload("running", raw.JSON))
	case "compaction_end":
		service.emitRunEventLocked(managed, "compaction.state", compactionPayload("succeeded", raw.JSON))
	case "auto_retry_start", "summarization_retry_scheduled", "summarization_retry_attempt_start":
		service.emitRunEventLocked(managed, "retry.state", retryPayload("running", raw.JSON))
	case "auto_retry_end", "summarization_retry_finished":
		service.emitRunEventLocked(managed, "retry.state", retryPayload("finished", raw.JSON))
	case "tool_execution_start":
		service.handleToolStartLocked(managed, raw.JSON)
	case "tool_execution_update":
		service.handleToolUpdateLocked(managed, raw.JSON)
	case "tool_execution_end":
		service.handleToolEndLocked(managed, raw.JSON)
	case "extension_ui_request":
		service.handleGateEventLocked(managed, raw.JSON)
	case "turn_start", "turn_end":
		// Turn lifecycle is retained in raw stdout; stable message/run events
		// already carry the UI state needed for Phase 2.
	case "bash_execution_update":
		service.rejectUnexpectedNoToolsEventLocked(managed, raw.Type)
	case "extension_error":
		message := "PI Extension 发生错误"
		var event struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw.JSON, &event) == nil && strings.TrimSpace(event.Error) != "" {
			message = sanitizeError(event.Error, workspaceRoot)
		}
		service.failGateProtocolLocked(managed, message)
	}
}

func (service *Service) handleMessageUpdateLocked(
	managed *managedSession,
	raw json.RawMessage,
	workspaceRoot string,
) {
	if managed.run == nil {
		return
	}
	delta := parseAssistantDelta(raw)
	switch delta.Type {
	case "text_delta":
		service.ensureAssistantMessageLocked(managed)
		if len(managed.run.assistantContent)+len(delta.Delta) > maxMessageBytes {
			message := "PI 回答超过 4 MiB，已停止本次运行"
			managed.run.errorMessage = message
			go abortRuntime(managed.runtime, service.requestTimeout)
			_ = service.emitLocked(managed, "error", managed.run.record.ID, "", errorPayload(
				"frame_too_large", message, false,
			))
			return
		}
		startChars := utf8.RuneCountInString(managed.run.assistantContent)
		managed.run.assistantContent += delta.Delta
		content := managed.run.assistantContent
		managed.run.assistant.Content = &content
		if err := service.store.UpsertAgentMessage(managed.run.assistant); err != nil {
			managed.run.errorMessage = sanitizeError(err.Error(), workspaceRoot)
			return
		}
		accumulated := startChars
		for _, part := range splitUTF8(delta.Delta, maxEventTextBytes) {
			accumulated += utf8.RuneCountInString(part)
			_ = service.emitLocked(managed, "message.delta", managed.run.record.ID, "", map[string]any{
				"messageId":        managed.run.assistant.ID,
				"delta":            part,
				"accumulatedChars": accumulated,
			})
		}
	case "thinking_delta":
		service.ensureAssistantMessageLocked(managed)
		for _, part := range splitUTF8(delta.Delta, maxEventTextBytes) {
			_ = service.emitLocked(managed, "reasoning.delta", managed.run.record.ID, "", map[string]any{
				"messageId":        managed.run.assistant.ID,
				"delta":            part,
				"accumulatedChars": utf8.RuneCountInString(part),
			})
		}
	case "error":
		message := strings.TrimSpace(delta.Reason)
		if message == "" {
			message = "PI 模型返回错误"
		}
		managed.run.errorMessage = sanitizeError(message, workspaceRoot)
		_ = service.emitLocked(managed, "error", managed.run.record.ID, "", errorPayload(
			"unknown", managed.run.errorMessage, true,
		))
	default:
		// Tool-call deltas are followed by a typed tool_execution_start event,
		// which is the authoritative allowlist decision point.
	}
}

func (service *Service) rejectUnexpectedNoToolsEventLocked(
	managed *managedSession,
	kind string,
) {
	if managed.run == nil {
		return
	}
	message := unexpectedToolMessage(kind)
	managed.run.errorMessage = message
	_ = service.emitLocked(managed, "error", managed.run.record.ID, "", errorPayload(
		"protocol_error", message, false,
	))
	go abortRuntime(managed.runtime, service.requestTimeout)
}

func (service *Service) handleMessageEndLocked(
	managed *managedSession,
	raw json.RawMessage,
) {
	if managed.run == nil {
		return
	}
	role, content := messageFromRaw(raw)
	if role != "assistant" {
		return
	}
	service.ensureAssistantMessageLocked(managed)
	if content != "" && len(content) <= maxMessageBytes {
		managed.run.assistantContent = content
	}
	status := "complete"
	if managed.run.stopRequested {
		status = "cancelled"
	} else if managed.run.errorMessage != "" {
		status = "error"
	}
	service.completeAssistantLocked(managed, status)
}

func (service *Service) ensureAssistantMessageLocked(managed *managedSession) {
	if managed.run == nil {
		return
	}
	if managed.run.assistantStarted && managed.run.assistant.Status == "streaming" {
		return
	}
	now := service.timestamp()
	content := ""
	message := storage.AgentMessageRecord{
		ID: newID("message"), TaskID: managed.record.TaskID, SessionID: managed.record.ID,
		RunID: &managed.run.record.ID, Role: "assistant", Kind: "text", Status: "streaming",
		Content: &content, Sequence: service.nextSequenceLocked(managed), CreatedAt: now,
	}
	service.touchSessionLocked(managed)
	if err := service.store.UpsertAgentMessageAndUpdateSession(message, managed.record); err != nil {
		managed.run.errorMessage = err.Error()
		return
	}
	managed.run.assistant = message
	managed.run.assistantContent = ""
	managed.run.assistantStarted = true
	_ = service.emitLocked(managed, "message.start", managed.run.record.ID, "", map[string]any{
		"messageId": message.ID, "role": "assistant", "kind": "text",
	})
}

func (service *Service) completeAssistantLocked(managed *managedSession, status string) {
	if managed.run == nil || !managed.run.assistantStarted {
		return
	}
	message := &managed.run.assistant
	if message.Status != "streaming" && message.Status != "pending" {
		return
	}
	now := service.timestamp()
	content := managed.run.assistantContent
	message.Content = &content
	message.Status = status
	message.CompletedAt = &now
	_ = service.store.UpsertAgentMessage(*message)
	payload := map[string]any{"messageId": message.ID, "status": status}
	if len(content) <= maxEventTextBytes {
		payload["content"] = content
	}
	if status == "error" {
		payload["error"] = errorPayload("unknown", managed.run.errorMessage, true)
	}
	_ = service.emitLocked(managed, "message.end", managed.run.record.ID, "", payload)
}

func (service *Service) handleToolStartLocked(managed *managedSession, raw json.RawMessage) {
	if managed.run == nil {
		return
	}
	externalID, name, args, _, _ := toolEvent(raw)
	if externalID == "" {
		externalID = newID("pi-tool")
	}
	now := service.timestamp()
	policy, allowed := gateToolPolicies[name]
	classification := permissionpolicy.ClassifyTool(permissionpolicy.ToolInput{
		TaskID: managed.record.TaskID, ToolName: name, Args: args,
	})
	record := storage.ToolCallRecord{
		ID: newID("tool"), TaskID: managed.record.TaskID, SessionID: managed.record.ID,
		RunID: managed.run.record.ID, ExternalToolCallID: externalID, ToolName: name,
		Capability: classification.Capability, RiskLevel: string(classification.RiskLevel), State: "running",
		ArgsJSON: stableArgsPreview(redactToolArgs(args)), StartedAt: &now,
	}
	if classification.Target != "" {
		record.Target = &classification.Target
	}
	if len(args) > 32*1024 {
		if ref, err := writeToolAuxFile(
			managed.workspaceRoot, managed.run.record.ID, record.ID, "args.json",
			redactToolArgs(args), maxToolOutputFileBytes,
		); err == nil {
			record.ArgsRef = &ref
		}
	}
	if !allowed || (name == "btask_write_artifact" && managed.record.Mode == "ask") {
		record.Capability = "unexpected.tool"
		record.RiskLevel = "critical"
	}
	if err := service.store.UpsertToolCall(record); err != nil {
		managed.run.errorMessage = err.Error()
		return
	}
	managed.run.tools[externalID] = record
	managed.run.toolArgs[externalID] = append(json.RawMessage(nil), args...)
	_ = service.emitLocked(managed, "tool.start", managed.run.record.ID, record.ID, map[string]any{
		"toolName": name, "capability": record.Capability, "subject": policy.subject,
		"readOnly": policy.readOnly, "riskLevel": record.RiskLevel,
		"argsPreview": stringValue(record.ArgsJSON), "argsRef": stringValue(record.ArgsRef),
	})
	if allowed && !(name == "btask_write_artifact" && managed.record.Mode == "ask") {
		return
	}
	message := "PI 尝试调用当前任务策略未允许的工具：" + name
	managed.run.errorMessage = message
	_ = service.emitLocked(managed, "error", managed.run.record.ID, "", errorPayload(
		"protocol_error", message, false,
	))
	go abortRuntime(managed.runtime, service.requestTimeout)
}

type gateToolPolicy struct {
	capability string
	riskLevel  string
	subject    string
	readOnly   bool
}

var gateToolPolicies = map[string]gateToolPolicy{
	"btask_list_resources": {
		capability: "task.resource.list", riskLevel: "low",
		subject: "列出当前任务资源", readOnly: true,
	},
	"btask_read_resource": {
		capability: "task.resource.read", riskLevel: "low",
		subject: "读取当前任务资源", readOnly: true,
	},
	"btask_write_artifact": {
		capability: "task.artifact.write", riskLevel: "medium",
		subject: "写入当前任务 artifacts", readOnly: false,
	},
	"btask_permission_probe": {
		capability: "diagnostic.permission.probe", riskLevel: "high",
		subject: "验证 BTask 权限审批链路", readOnly: true,
	},
}

func (service *Service) handleToolUpdateLocked(managed *managedSession, raw json.RawMessage) {
	if managed.run == nil {
		return
	}
	externalID, _, _, output, _ := toolEvent(raw)
	record, exists := managed.run.tools[externalID]
	if !exists {
		return
	}
	output = redactSecrets(output)
	preview := boundedText(output, 32*1024)
	payload := map[string]any{
		"outputPreview": preview, "accumulated": true, "truncated": len(output) > len(preview),
	}
	if len(output) > 32*1024 {
		if ref, err := writeToolAuxFile(
			managed.workspaceRoot, managed.run.record.ID, record.ID, "output.txt",
			[]byte(output), maxToolOutputFileBytes,
		); err == nil {
			record.OutputRef = &ref
			payload["outputRef"] = ref
		}
	}
	record.OutputSummary = &preview
	_ = service.store.UpsertToolCall(record)
	managed.run.tools[externalID] = record
	_ = service.emitLocked(managed, "tool.update", managed.run.record.ID, record.ID, payload)
}

func (service *Service) handleToolEndLocked(managed *managedSession, raw json.RawMessage) {
	if managed.run == nil {
		return
	}
	externalID, _, _, output, isError := toolEvent(raw)
	record, exists := managed.run.tools[externalID]
	if !exists {
		return
	}
	now := service.timestamp()
	receipt, hasReceipt := managed.run.receipts[externalID]
	record.State = "succeeded"
	if !hasReceipt {
		isError = true
		record.State = "failed"
		message := "工具结束时没有可对账的 BTask 权限回执"
		managed.run.errorMessage = message
		_ = service.emitLocked(managed, "error", managed.run.record.ID, record.ID, errorPayload(
			"protocol_error", message, false,
		))
	} else if receipt.Decision != "allow" {
		isError = true
		if receipt.Decision == "cancelled" || receipt.Decision == "expired" {
			record.State = "cancelled"
		} else {
			record.State = "denied"
		}
	} else if !receipt.OperationStarted {
		isError = true
		record.State = "failed"
		message := "工具未通过 BTask 执行桥却返回了结果"
		managed.run.errorMessage = message
		_ = service.emitLocked(managed, "error", managed.run.record.ID, record.ID, errorPayload(
			"protocol_error", message, false,
		))
	} else if isError {
		record.State = "failed"
	}
	record.IsError = isError
	output = redactSecrets(output)
	preview := boundedText(output, 32*1024)
	record.OutputSummary = &preview
	if len(output) > 32*1024 {
		if ref, err := writeToolAuxFile(
			managed.workspaceRoot, managed.run.record.ID, record.ID, "output.txt",
			[]byte(output), maxToolOutputFileBytes,
		); err == nil {
			record.OutputRef = &ref
		}
	}
	record.FinishedAt = &now
	_ = service.store.UpsertToolCall(record)
	managed.run.tools[externalID] = record
	payload := map[string]any{
		"state": record.State, "outputSummary": preview,
		"outputRef": stringValue(record.OutputRef),
	}
	if isError {
		payload["error"] = errorPayload("tool_error", preview, false)
	}
	_ = service.emitLocked(managed, "tool.end", managed.run.record.ID, record.ID, payload)
	if hasReceipt {
		_ = service.emitLocked(managed, "permission.reconciled", managed.run.record.ID, record.ID, map[string]any{
			"requestId": receipt.RequestID, "state": record.State,
			"operationStarted": receipt.OperationStarted,
		})
	}
}

func (service *Service) emitRunEventLocked(
	managed *managedSession,
	kind string,
	payload any,
) {
	if managed.run != nil {
		_ = service.emitLocked(managed, kind, managed.run.record.ID, "", payload)
	}
}

func (service *Service) emitLocked(
	managed *managedSession,
	kind string,
	runID string,
	toolCallID string,
	payload any,
) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := service.timestamp()
	sequence := service.nextSequenceLocked(managed)
	event := Event{
		Version: 1, EventID: newID("event"), Sequence: sequence, Kind: kind,
		TaskID: managed.record.TaskID, SessionID: managed.record.ID,
		RunID: runID, ToolCallID: toolCallID, OccurredAt: now, Payload: payload,
	}
	record := storage.AgentEventRecord{
		EventID: event.EventID, Version: event.Version, TaskID: event.TaskID,
		SessionID: event.SessionID, Sequence: event.Sequence, Kind: event.Kind,
		PayloadJSON: string(payloadJSON), OccurredAt: event.OccurredAt,
	}
	if runID != "" {
		record.RunID = &runID
	}
	if toolCallID != "" {
		record.ToolCallID = &toolCallID
	}
	service.touchSessionLocked(managed)
	if err := service.store.AppendAgentEventAndUpdateSession(record, managed.record); err != nil {
		return err
	}
	if runID != "" && managed.run != nil && managed.run.record.ID == runID {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := appendRunFile(
			managed.workspaceRoot,
			managed.run.record.EventsPath,
			append(encoded, '\n'),
		); err != nil {
			// The database is authoritative. A missing supplementary run log must
			// not cause an already durable event to be emitted twice.
			_ = err
		}
	}
	if err := service.delivery.enqueue(event); err != nil {
		if managed.run != nil && managed.run.errorMessage == "" {
			managed.run.errorMessage = "PI 事件投递队列已满，已停止本次运行"
			managed.run.record.State = "stopping"
			managed.run.record.ErrorMessage = &managed.run.errorMessage
			_ = service.store.UpsertExecutionRun(managed.run.record)
			go abortRuntime(managed.runtime, service.requestTimeout)
		}
		return err
	}
	return nil
}

func (service *Service) finishRunLocked(
	managed *managedSession,
	state string,
	reason string,
) {
	if managed.run == nil {
		return
	}
	service.cancelPendingPermissionsLocked(managed, "运行结束，待处理权限请求已取消")
	run := managed.run
	messageStatus := "complete"
	if state == "cancelled" {
		messageStatus = "cancelled"
	} else if state == "failed" || state == "interrupted" {
		messageStatus = "error"
	}
	service.completeAssistantLocked(managed, messageStatus)
	if run.assistantStarted && run.assistant.Status != messageStatus {
		now := service.timestamp()
		run.assistant.Status = messageStatus
		run.assistant.CompletedAt = &now
		content := run.assistantContent
		run.assistant.Content = &content
		_ = service.store.UpsertAgentMessage(run.assistant)
		payload := map[string]any{"messageId": run.assistant.ID, "status": messageStatus}
		if len(content) <= maxEventTextBytes {
			payload["content"] = content
		}
		_ = service.emitLocked(managed, "message.end", run.record.ID, "", payload)
	}
	now := service.timestamp()
	run.record.State = state
	run.record.FinishedAt = &now
	if reason != "" {
		run.record.ErrorMessage = &reason
	}
	if strings.TrimSpace(run.assistantContent) != "" {
		summary := boundedText(strings.TrimSpace(run.assistantContent), 500)
		run.record.ResultSummary = &summary
	}
	if managed.workspaceRoot != "" {
		_ = writeRunFile(managed.workspaceRoot, run.record.ResultPath, []byte(run.assistantContent))
		if managed.runtime != nil {
			_ = writeRunFile(
				managed.workspaceRoot,
				run.record.StderrPath,
				[]byte(managed.runtime.StderrTail()),
			)
		}
	}
	_ = service.store.UpsertExecutionRun(run.record)
	managed.record.State = "idle"
	if state == "interrupted" {
		managed.record.State = "interrupted"
	}
	if reason == "" {
		managed.record.ErrorMessage = nil
	} else {
		managed.record.ErrorMessage = &reason
	}
	service.touchSessionLocked(managed)
	_ = service.store.UpsertAgentSession(managed.record)
	_ = service.emitLocked(managed, "run.state", run.record.ID, "", map[string]any{
		"state": state, "reason": reason,
	})
	_ = service.emitLocked(managed, "session.state", "", "", map[string]any{
		"state": managed.record.State,
	})
	managed.run = nil
}

func (service *Service) syncEntriesLocked(
	ctx context.Context,
	managed *managedSession,
) error {
	if managed.runtime == nil {
		return nil
	}
	fields := map[string]any{}
	if managed.record.LastEntryID != nil && *managed.record.LastEntryID != "" {
		fields["since"] = *managed.record.LastEntryID
	}
	var response entriesResponse
	err := managed.runtime.Call(ctx, "get_entries", fields, &response)
	if err != nil && len(fields) > 0 {
		response = entriesResponse{}
		err = managed.runtime.Call(ctx, "get_entries", nil, &response)
	}
	if err != nil {
		return err
	}
	existing, err := service.store.AgentMessages(managed.record.TaskID, managed.record.ID)
	if err != nil {
		return err
	}
	byEntry := make(map[string]bool, len(existing))
	for _, message := range existing {
		if message.PIEntryID != nil {
			byEntry[*message.PIEntryID] = true
		}
	}
	for _, entry := range response.Entries {
		if entry.Type != "message" || entry.ID == "" || byEntry[entry.ID] {
			continue
		}
		role, content := decodeMessage(entry.Message)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		matched := false
		for index := range existing {
			message := &existing[index]
			if message.PIEntryID == nil && message.Role == role && message.Content != nil && *message.Content == content {
				message.PIEntryID = &entry.ID
				if err := service.store.UpsertAgentMessage(*message); err != nil {
					return err
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		createdAt := strings.TrimSpace(entry.Timestamp)
		if _, parseErr := time.Parse(time.RFC3339, createdAt); parseErr != nil {
			createdAt = service.timestamp()
		}
		entryID := entry.ID
		contentCopy := content
		message := storage.AgentMessageRecord{
			ID: newID("message"), TaskID: managed.record.TaskID, SessionID: managed.record.ID,
			Role: role, Kind: "text", Status: "complete", Content: &contentCopy,
			Sequence: service.nextSequenceLocked(managed), PIEntryID: &entryID,
			CreatedAt: createdAt, CompletedAt: &createdAt,
		}
		service.touchSessionLocked(managed)
		if err := service.store.UpsertAgentMessageAndUpdateSession(message, managed.record); err != nil {
			return err
		}
		_ = service.emitLocked(managed, "message.start", "", "", map[string]any{
			"messageId": message.ID, "role": role, "kind": "text",
		})
		payload := map[string]any{"messageId": message.ID, "status": "complete"}
		if len(content) <= maxEventTextBytes {
			payload["content"] = content
		}
		_ = service.emitLocked(managed, "message.end", "", "", payload)
	}
	if response.LeafID != nil && *response.LeafID != "" {
		managed.record.LastEntryID = response.LeafID
	}
	service.touchSessionLocked(managed)
	return service.store.UpsertAgentSession(managed.record)
}

func prepareRunFiles(root string, run storage.ExecutionRunRecord) (err error) {
	runDirectory := filepath.Join(root, "runs", run.ID)
	if err := os.Mkdir(runDirectory, 0o700); err != nil {
		return fmt.Errorf("创建 PI 运行目录失败: %w", err)
	}
	defer func() {
		if err != nil {
			discardRunFiles(root, run.ID)
		}
	}()
	if info, err := os.Lstat(runDirectory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("PI 运行目录不安全")
	}
	for _, logicalPath := range []string{run.EventsPath, run.StdoutPath, run.StderrPath, run.ResultPath} {
		path := filepath.Join(root, filepath.FromSlash(logicalPath))
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("创建 PI 运行文件失败: %w", err)
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func discardRunFiles(root string, runID string) {
	if root == "" || runID == "" || strings.ContainsAny(runID, "/\\\x00") {
		return
	}
	directory := filepath.Join(root, "runs", runID)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	_ = os.RemoveAll(directory)
}

func appendRunFile(root string, logicalPath string, content []byte) error {
	if root == "" {
		return nil
	}
	path := filepath.Join(root, filepath.FromSlash(logicalPath))
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("PI 运行日志路径不安全")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return err
	}
	return file.Sync()
}

func writeRunFile(root string, logicalPath string, content []byte) error {
	if root == "" {
		return nil
	}
	path := filepath.Join(root, filepath.FromSlash(logicalPath))
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("PI 运行结果路径不安全")
	}
	file, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return err
	}
	return file.Sync()
}

func writeToolAuxFile(
	root string,
	runID string,
	toolID string,
	suffix string,
	content []byte,
	limit int,
) (string, error) {
	if root == "" || runID == "" || toolID == "" ||
		strings.ContainsAny(runID+toolID+suffix, "/\\\x00") {
		return "", errors.New("PI tool output identity is unsafe")
	}
	if len(content) > limit {
		end := limit
		for end > 0 && end < len(content) && !utf8.RuneStart(content[end]) {
			end--
		}
		content = content[:end]
	}
	logicalPath := "runs/" + runID + "/tool-" + toolID + "-" + suffix
	directory := filepath.Join(root, "runs", runID)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("PI tool output directory is unsafe")
	}
	directoryRoot, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer directoryRoot.Close()
	filename := "tool-" + toolID + "-" + suffix
	if info, err := directoryRoot.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("PI tool output path is unsafe")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporaryName := "." + filename + "-" + newID("tmp")
	file, err := directoryRoot.OpenFile(temporaryName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = directoryRoot.Remove(temporaryName)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return "", err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := directoryRoot.Rename(temporaryName, filename); err != nil {
		return "", err
	}
	removeTemporary = false
	if err := directoryRoot.Chmod(filename, 0o600); err != nil {
		return "", err
	}
	return logicalPath, nil
}

func redactToolArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return raw
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	value = redactJSONValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}

func redactJSONValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if credentialAuditKey(key) {
				current[key] = "[REDACTED]"
				continue
			}
			current[key] = redactJSONValue(child)
		}
	case []any:
		for index, child := range current {
			current[index] = redactJSONValue(child)
		}
	case string:
		return redactSecrets(current)
	}
	return value
}

func redactSecrets(value string) string {
	value = secretHeaderPattern.ReplaceAllString(value, `$1$2[REDACTED]`)
	return secretTextPattern.ReplaceAllString(value, `$1$2[REDACTED]`)
}

func redactAuditJSON(raw json.RawMessage) []byte {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return []byte(redactSecrets(string(raw)))
	}
	encoded, err := json.Marshal(redactJSONValue(value))
	if err != nil {
		return []byte(redactSecrets(string(raw)))
	}
	return encoded
}

func credentialAuditKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return lower == "pat" || strings.Contains(lower, "token") ||
		strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api_key") || strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey") || strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "cookie") || strings.Contains(lower, "credential") ||
		strings.Contains(lower, "private_key") || strings.Contains(lower, "private-key") ||
		strings.Contains(lower, "signature")
}

func splitUTF8(value string, maxBytes int) []string {
	if value == "" {
		return nil
	}
	parts := make([]string, 0, len(value)/maxBytes+1)
	for len(value) > maxBytes {
		end := maxBytes
		for end > 0 && !utf8.RuneStart(value[end]) {
			end--
		}
		if end == 0 {
			end = maxBytes
		}
		parts = append(parts, value[:end])
		value = value[end:]
	}
	if value != "" {
		parts = append(parts, value)
	}
	return parts
}

func abortRuntime(runtime Runtime, timeout time.Duration) {
	if runtime == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = runtime.Call(ctx, "abort", nil, nil)
}

func errorPayload(code string, message string, retryable bool) map[string]any {
	return map[string]any{"code": code, "message": message, "retryable": retryable}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrPINotInstalled):
		return "pi_not_installed"
	case errors.Is(err, ErrUnsupportedPIVersion):
		return "unsupported_pi_version"
	case errors.Is(err, context.DeadlineExceeded):
		return "startup_timeout"
	case errors.Is(err, ErrFrameTooLarge):
		return "frame_too_large"
	case errors.Is(err, ErrTruncatedFrame):
		return "truncated_frame"
	case errors.Is(err, ErrInvalidFrame):
		return "protocol_error"
	default:
		return "unknown"
	}
}

func exitErrorCode(exit ProcessExit) string {
	if exit.Err != nil {
		code := errorCode(exit.Err)
		if code != "unknown" {
			return code
		}
	}
	return "process_exit"
}

func processExitMessage(exit ProcessExit, workspaceRoot string) string {
	message := "原生 PI 进程已退出"
	if exit.Code >= 0 {
		message += fmt.Sprintf("（exit %d）", exit.Code)
	}
	if exit.Signal != "" {
		message += "，signal " + exit.Signal
	}
	if exit.Err != nil && !errors.Is(exit.Err, context.Canceled) {
		message += "：" + exit.Err.Error()
	}
	if tail := strings.TrimSpace(exit.StderrTail); tail != "" {
		message += "；stderr: " + boundedText(tail, 300)
	}
	return sanitizeError(message, workspaceRoot)
}
