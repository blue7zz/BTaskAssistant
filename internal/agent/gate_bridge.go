package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blue7zz/BTaskAssistant/internal/execution"
	permissionpolicy "github.com/blue7zz/BTaskAssistant/internal/permissions"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

type gateBridgeRequest struct {
	Version    string          `json:"version"`
	Nonce      string          `json:"nonce"`
	TaskID     string          `json:"taskId"`
	SessionID  string          `json:"sessionId"`
	Mode       string          `json:"mode"`
	RunID      string          `json:"runId"`
	ToolCallID string          `json:"toolCallId"`
	Operation  string          `json:"operation"`
	Args       json.RawMessage `json:"args"`
}

type gateRunRequest struct {
	Version   string `json:"version"`
	Nonce     string `json:"nonce"`
	TaskID    string `json:"taskId"`
	SessionID string `json:"sessionId"`
	Mode      string `json:"mode"`
}

type permissionEnvelope struct {
	Protocol         string `json:"protocol"`
	Version          string `json:"version"`
	Nonce            string `json:"nonce"`
	TaskID           string `json:"taskId"`
	SessionID        string `json:"sessionId"`
	RunID            string `json:"runId"`
	Mode             string `json:"mode"`
	ToolCallID       string `json:"toolCallId"`
	ToolName         string `json:"toolName"`
	Capability       string `json:"capability"`
	Subject          string `json:"subject"`
	Target           string `json:"target"`
	NormalizedTarget string `json:"normalizedTarget"`
	RiskLevel        string `json:"riskLevel"`
	ArgsDigest       string `json:"argsDigest"`
}

type gateBridgeResponse struct {
	Version string `json:"version"`
	Nonce   string `json:"nonce"`
	OK      bool   `json:"ok"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (service *Service) handleGateEventLocked(
	managed *managedSession,
	raw json.RawMessage,
) {
	var event struct {
		ID          string `json:"id"`
		Method      string `json:"method"`
		Title       string `json:"title"`
		Placeholder string `json:"placeholder"`
		Message     string `json:"message"`
		StatusKey   string `json:"statusKey"`
		StatusText  string `json:"statusText"`
	}
	if json.Unmarshal(raw, &event) != nil || event.ID == "" {
		service.failGateProtocolLocked(managed, "PI gate emitted an invalid extension UI request")
		return
	}
	if event.Method == "setStatus" {
		if event.StatusKey == "btask-gate" && event.StatusText == managed.gate.Version+":"+managed.gate.Nonce {
			return
		}
		service.failGateProtocolLocked(managed, "PI gate heartbeat identity mismatch")
		return
	}
	if event.Method == "input" && event.Title == "btask-run" {
		service.handleRunIdentityRequestLocked(managed, event.ID, event.Placeholder)
		return
	}
	if event.Method == "confirm" && event.Title == "btask-permission" {
		service.handlePermissionRequestLocked(managed, event.ID, event.Message)
		return
	}
	if event.Method != "input" || event.Title != "btask-gate" || len(event.Placeholder) > MaxRPCFrameBytes {
		service.cancelGateRequest(managed, event.ID)
		service.failGateProtocolLocked(managed, "PI extension requested an unsupported UI operation")
		return
	}
	var request gateBridgeRequest
	if json.Unmarshal([]byte(event.Placeholder), &request) != nil ||
		request.Version != managed.gate.Version ||
		request.Nonce != managed.gate.Nonce ||
		request.TaskID != managed.record.TaskID ||
		request.SessionID != managed.record.ID ||
		request.Mode != managed.record.Mode ||
		managed.run == nil || request.RunID != managed.run.record.ID {
		service.cancelGateRequest(managed, event.ID)
		service.failGateProtocolLocked(managed, "PI gate request identity mismatch")
		return
	}
	if request.ToolCallID == "" {
		service.respondGate(managed, event.ID, nil, errors.New("PI gate request has no active run"))
		return
	}
	tool, exists := managed.run.tools[request.ToolCallID]
	wantTool := map[string]string{
		"list_resources":       "btask_list_resources",
		"read_resource":        "btask_read_resource",
		"write_artifact":       "btask_write_artifact",
		"permission_probe":     "btask_permission_probe",
		"list_worktree_files":  "btask_list_worktree_files",
		"read_worktree_file":   "btask_read_worktree_file",
		"write_worktree_file":  "btask_write_worktree_file",
		"edit_worktree_file":   "btask_edit_worktree_file",
		"delete_worktree_file": "btask_delete_worktree_file",
		"shell":                "btask_shell",
	}[request.Operation]
	if !exists || wantTool == "" || tool.ToolName != wantTool {
		service.respondGate(managed, event.ID, nil, errors.New("PI gate request does not match its active tool call"))
		service.failGateProtocolLocked(managed, "PI gate tool-call identity mismatch")
		return
	}
	classification := permissionpolicy.ClassifyTool(permissionpolicy.ToolInput{
		TaskID: managed.record.TaskID, ToolName: tool.ToolName, Args: request.Args,
	})
	receipt, receiptExists := managed.run.receipts[request.ToolCallID]
	if !receiptExists || receipt.Decision != "allow" || receipt.OperationStarted ||
		receipt.Capability != classification.Capability ||
		receipt.NormalizedTarget != classification.NormalizedTarget ||
		receipt.ArgsDigest != classification.ArgsDigest || classification.HardDenyReason != "" {
		service.respondGate(managed, event.ID, nil, errors.New("PI gate operation has no matching permission receipt"))
		service.failGateProtocolLocked(managed, "PI gate permission receipt mismatch")
		return
	}
	receipt.OperationStarted = true
	managed.run.receipts[request.ToolCallID] = receipt
	_ = service.emitLocked(managed, "permission.receipt", managed.run.record.ID, tool.ID, map[string]any{
		"requestId": receipt.RequestID, "capability": receipt.Capability,
		"scope": receipt.Scope, "state": "consumed",
	})

	var data any
	var err error
	switch request.Operation {
	case "list_resources":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("resource list arguments are invalid")
		} else {
			data, err = service.gateListResources(managed.record.TaskID, args.Query, args.Limit)
		}
	case "read_resource":
		var args struct {
			ResourceID string `json:"resourceId"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("resource read arguments are invalid")
		} else {
			data, err = service.gateResourceContent(managed.record.TaskID, args.ResourceID)
		}
	case "write_artifact":
		var args struct {
			Kind    string `json:"kind"`
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("artifact write arguments are invalid")
		} else {
			data, err = service.writeArtifactLocked(managed, args.Kind, args.Name, args.Content)
		}
	case "permission_probe":
		var args struct {
			Target string `json:"target"`
		}
		if json.Unmarshal(request.Args, &args) != nil || strings.TrimSpace(args.Target) == "" {
			err = errors.New("permission probe arguments are invalid")
		} else {
			data = map[string]any{"approved": true, "target": args.Target}
		}
	case "list_worktree_files":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("worktree file list arguments are invalid")
		} else {
			data, err = service.git.ListFiles(context.Background(), managed.record.TaskID, args.Query, args.Limit)
		}
	case "read_worktree_file":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("worktree file read arguments are invalid")
		} else {
			data, err = service.git.ReadFile(context.Background(), managed.record.TaskID, args.Path)
		}
	case "write_worktree_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("worktree file write arguments are invalid")
		} else {
			data, err = service.git.WriteFile(context.Background(), managed.record.TaskID, managed.record.Mode, args.Path, args.Content)
			if err == nil {
				service.emitGitChangedLocked(managed, tool.ID, "file_write")
			}
		}
	case "edit_worktree_file":
		var args struct {
			Path       string `json:"path"`
			OldText    string `json:"oldText"`
			NewText    string `json:"newText"`
			ReplaceAll bool   `json:"replaceAll"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("worktree file edit arguments are invalid")
		} else {
			data, err = service.git.EditFile(context.Background(), managed.record.TaskID, managed.record.Mode, args.Path, args.OldText, args.NewText, args.ReplaceAll)
			if err == nil {
				service.emitGitChangedLocked(managed, tool.ID, "file_edit")
			}
		}
	case "delete_worktree_file":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(request.Args, &args) != nil {
			err = errors.New("worktree file delete arguments are invalid")
		} else {
			data, err = service.git.DeleteFile(context.Background(), managed.record.TaskID, managed.record.Mode, args.Path)
			if err == nil {
				service.emitGitChangedLocked(managed, tool.ID, "file_delete")
			}
		}
	case "shell":
		service.startShellBridgeLocked(managed, event.ID, request, tool)
		return
	}
	service.respondGate(managed, event.ID, data, err)
}

func (service *Service) startShellBridgeLocked(
	managed *managedSession,
	extensionRequestID string,
	request gateBridgeRequest,
	tool storage.ToolCallRecord,
) {
	var args struct {
		Command        string `json:"command"`
		CWD            string `json:"cwd"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	}
	if json.Unmarshal(request.Args, &args) != nil {
		service.respondGate(managed, extensionRequestID, nil, errors.New("Shell arguments are invalid"))
		return
	}
	_, cwd, cwdDisplay, err := service.git.ResolveCommandDirectory(
		context.Background(), managed.record.TaskID, managed.record.Mode, args.CWD,
	)
	if err != nil {
		service.respondGate(managed, extensionRequestID, nil, err)
		return
	}
	timeout := time.Duration(args.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	runID := managed.run.record.ID
	taskID := managed.record.TaskID
	sessionID := managed.record.ID
	workspaceRoot := managed.workspaceRoot
	_ = service.emitLocked(managed, "tool.update", runID, tool.ID, map[string]any{
		"outputPreview": "Shell 命令已启动", "accumulated": true,
		"command": args.Command, "cwd": cwdDisplay,
	})
	go func() {
		result, runErr := service.execution.Run(context.Background(), execution.RunRequest{
			TaskID: taskID, SessionID: sessionID, RunID: runID, ToolCallID: tool.ID,
			Command: args.Command, CWD: cwd, CWDDisplay: cwdDisplay,
			WorkspaceRoot: workspaceRoot, Timeout: timeout,
		})
		managed.mutex.Lock()
		defer managed.mutex.Unlock()
		if managed.run == nil || managed.run.record.ID != runID || managed.runtime == nil {
			service.cancelGateRequest(managed, extensionRequestID)
			return
		}
		if runErr == nil {
			service.emitGitChangedLocked(managed, tool.ID, "shell")
		}
		service.respondGate(managed, extensionRequestID, result, runErr)
	}()
}

func (service *Service) emitGitChangedLocked(managed *managedSession, toolID string, reason string) {
	if managed.run == nil {
		return
	}
	_ = service.emitLocked(managed, "git.changed", managed.run.record.ID, toolID, map[string]any{
		"reason": reason,
	})
}

func (service *Service) handleRunIdentityRequestLocked(
	managed *managedSession,
	requestID string,
	payload string,
) {
	if len(payload) > MaxRPCFrameBytes {
		service.cancelGateRequest(managed, requestID)
		service.failGateProtocolLocked(managed, "PI run identity request exceeds the RPC limit")
		return
	}
	var request gateRunRequest
	if json.Unmarshal([]byte(payload), &request) != nil ||
		request.Version != managed.gate.Version || request.Nonce != managed.gate.Nonce ||
		request.TaskID != managed.record.TaskID || request.SessionID != managed.record.ID ||
		request.Mode != managed.record.Mode || managed.run == nil {
		service.cancelGateRequest(managed, requestID)
		service.failGateProtocolLocked(managed, "PI run identity request did not match the active gate")
		return
	}
	response, _ := json.Marshal(map[string]string{
		"version": managed.gate.Version, "nonce": managed.gate.Nonce,
		"runId": managed.run.record.ID,
	})
	service.respondExtensionInput(managed, requestID, string(response))
}

func (service *Service) handlePermissionRequestLocked(
	managed *managedSession,
	extensionUIRequestID string,
	payload string,
) {
	if len(payload) == 0 || len(payload) > MaxRPCFrameBytes || managed.run == nil {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, "PI permission envelope is missing, oversized, or has no active run")
		return
	}
	var envelope permissionEnvelope
	if json.Unmarshal([]byte(payload), &envelope) != nil ||
		envelope.Protocol != permissionProtocolVersion ||
		envelope.Version != managed.gate.Version || envelope.Nonce != managed.gate.Nonce ||
		envelope.TaskID != managed.record.TaskID || envelope.SessionID != managed.record.ID ||
		envelope.RunID != managed.run.record.ID || envelope.Mode != managed.record.Mode {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, "PI permission envelope identity mismatch")
		return
	}
	tool, exists := managed.run.tools[envelope.ToolCallID]
	policy, knownTool := gateToolPolicies[envelope.ToolName]
	if !exists || !knownTool || tool.ToolName != envelope.ToolName {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, "PI permission envelope tool-call mismatch")
		return
	}
	classification := permissionpolicy.ClassifyTool(permissionpolicy.ToolInput{
		TaskID:   managed.record.TaskID,
		ToolName: envelope.ToolName,
		Args:     managed.run.toolArgs[envelope.ToolCallID],
	})
	if classification.Capability != envelope.Capability ||
		classification.Subject != envelope.Subject || classification.Target != envelope.Target ||
		classification.NormalizedTarget != envelope.NormalizedTarget ||
		string(classification.RiskLevel) != envelope.RiskLevel ||
		classification.ArgsDigest != envelope.ArgsDigest ||
		classification.Capability != policy.capability {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, "PI permission envelope did not match authoritative classification")
		return
	}

	now := service.now().UTC()
	requestID := newID("permission")
	grants, err := service.store.ActivePermissionGrants(
		managed.record.TaskID, managed.record.ID, now.Format(time.RFC3339Nano),
	)
	if err != nil {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, fmt.Sprintf("load permission grants: %v", err))
		return
	}
	taskStatus, err := service.store.TaskStatus(managed.record.TaskID)
	if err != nil {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, fmt.Sprintf("load task status: %v", err))
		return
	}
	policyRequest := permissionpolicy.Request{
		RequestID: requestID, TaskID: managed.record.TaskID, SessionID: managed.record.ID,
		RunID: managed.run.record.ID, ToolCallID: tool.ID, ToolName: tool.ToolName,
		Mode: managed.record.Mode, TaskStatus: taskStatus, GateValid: true,
		Classification: classification,
	}
	decision := permissionpolicy.Evaluate(policyRequest, permissionGrants(grants), now)
	requestedAt := now.Format(time.RFC3339Nano)
	normalizedTarget := classification.NormalizedTarget
	reason := decision.Reason
	record := storage.PermissionRequestRecord{
		ID: requestID, TaskID: managed.record.TaskID, SessionID: managed.record.ID,
		RunID: managed.run.record.ID, ToolCallID: tool.ID,
		Capability: classification.Capability, Target: classification.Target,
		NormalizedTarget: &normalizedTarget, Subject: classification.Subject,
		RiskLevel: string(classification.RiskLevel), State: "pending",
		RequestedAt: requestedAt, Reason: &reason,
	}
	if decision.Outcome != permissionpolicy.OutcomeAsk {
		resolvedAt := requestedAt
		resolvedBy := "policy"
		record.ResolvedAt = &resolvedAt
		record.ResolvedBy = &resolvedBy
		if decision.Outcome == permissionpolicy.OutcomeAllow {
			record.State = "allowed"
		} else {
			record.State = "denied"
		}
		if decision.MatchedGrantID != "" {
			for _, grant := range grants {
				if grant.ID == decision.MatchedGrantID {
					scope := grant.Scope
					record.DecisionScope = &scope
					break
				}
			}
		}
	}
	if err := service.store.UpsertPermissionRequest(record); err != nil {
		service.respondPermission(managed, extensionUIRequestID, false, true)
		service.failGateProtocolLocked(managed, fmt.Sprintf("persist permission request: %v", err))
		return
	}

	if decision.Outcome == permissionpolicy.OutcomeAsk {
		expiresAt := now.Add(service.permissionTimeout)
		managed.run.permissions[record.ID] = &pendingPermission{
			request: record, extensionUIRequestID: extensionUIRequestID, expiresAt: expiresAt,
		}
		tool.State = "waiting_permission"
		managed.run.tools[envelope.ToolCallID] = tool
		managed.run.record.State = "waiting_permission"
		_ = service.store.UpsertToolCall(tool)
		_ = service.store.UpsertExecutionRun(managed.run.record)
		_ = service.emitLocked(managed, "permission.requested", record.RunID, tool.ID, map[string]any{
			"requestId": record.ID, "capability": record.Capability,
			"subject": record.Subject, "target": record.Target,
			"riskLevel":            record.RiskLevel,
			"allowedScopes":        scopeStrings(decision.AllowedScopes),
			"expiresAt":            expiresAt.Format(time.RFC3339Nano),
			"extensionUIRequestId": extensionUIRequestID,
		})
		time.AfterFunc(service.permissionTimeout, func() {
			service.expirePermission(record.ID, record.TaskID, record.SessionID)
		})
		return
	}

	if decision.Outcome == permissionpolicy.OutcomeAllow && decision.ConsumeGrantID != "" {
		if err := service.store.ConsumePermissionGrant(
			record.TaskID, decision.ConsumeGrantID, record.ID, requestedAt,
		); err != nil {
			service.respondPermission(managed, extensionUIRequestID, false, true)
			service.failGateProtocolLocked(managed, "once permission grant could not be consumed")
			return
		}
	}
	scope := stringValue(record.DecisionScope)
	decisionValue := "deny"
	if decision.Outcome == permissionpolicy.OutcomeAllow {
		decisionValue = "allow"
		managed.run.receipts[envelope.ToolCallID] = permissionReceipt{
			RequestID: record.ID, Capability: record.Capability,
			NormalizedTarget: normalizedTarget, ArgsDigest: classification.ArgsDigest,
			Scope: scope, Decision: "allow", Mutating: classification.Mutating,
		}
	} else {
		tool.State = "denied"
		_ = service.store.UpsertToolCall(tool)
		managed.run.tools[envelope.ToolCallID] = tool
		managed.run.receipts[envelope.ToolCallID] = permissionReceipt{
			RequestID: record.ID, Capability: record.Capability,
			NormalizedTarget: normalizedTarget, ArgsDigest: classification.ArgsDigest,
			Scope: scope, Decision: record.State, Mutating: classification.Mutating,
		}
	}
	_ = service.emitLocked(managed, "permission.resolved", record.RunID, tool.ID, map[string]any{
		"requestId": record.ID, "decision": record.State, "scope": scope,
		"matchedGrantId":       decision.MatchedGrantID,
		"extensionUIRequestId": extensionUIRequestID,
	})
	service.respondPermission(managed, extensionUIRequestID, decisionValue == "allow", false)
}

func permissionGrants(records []storage.PermissionGrantRecord) []permissionpolicy.Grant {
	result := make([]permissionpolicy.Grant, 0, len(records))
	for _, record := range records {
		grant := permissionpolicy.Grant{
			ID: record.ID, TaskID: stringValue(record.TaskID), SessionID: stringValue(record.SessionID),
			RequestID: stringValue(record.RequestID), Capability: record.Capability,
			TargetPattern: record.TargetPattern, Scope: permissionpolicy.Scope(record.Scope),
			Decision:    permissionpolicy.Outcome(record.Decision),
			RiskCeiling: permissionpolicy.RiskLevel(record.RiskCeiling),
		}
		grant.ExpiresAt = parsePermissionTime(record.ExpiresAt)
		grant.ConsumedAt = parsePermissionTime(record.ConsumedAt)
		grant.RevokedAt = parsePermissionTime(record.RevokedAt)
		result = append(result, grant)
	}
	return result
}

func parsePermissionTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil
	}
	return &parsed
}

func scopeStrings(scopes []permissionpolicy.Scope) []string {
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, string(scope))
	}
	return result
}

func (service *Service) respondGate(
	managed *managedSession,
	requestID string,
	data any,
	operationErr error,
) {
	response := gateBridgeResponse{
		Version: managed.gate.Version,
		Nonce:   managed.gate.Nonce,
		OK:      operationErr == nil,
		Data:    data,
	}
	if operationErr != nil {
		response.Error = sanitizeError(operationErr.Error(), managed.workspaceRoot)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		service.failGateProtocolLocked(managed, "PI gate response could not be encoded")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), service.requestTimeout)
	defer cancel()
	if err := sendRuntimeNotification(ctx, managed.runtime, map[string]any{
		"type":  "extension_ui_response",
		"id":    requestID,
		"value": string(encoded),
	}); err != nil {
		service.failGateProtocolLocked(managed, fmt.Sprintf("PI gate response failed: %v", err))
	}
}

func (service *Service) respondExtensionInput(
	managed *managedSession,
	requestID string,
	value string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), service.requestTimeout)
	defer cancel()
	if err := sendRuntimeNotification(ctx, managed.runtime, map[string]any{
		"type": "extension_ui_response", "id": requestID, "value": value,
	}); err != nil {
		service.failGateProtocolLocked(managed, fmt.Sprintf("PI extension response failed: %v", err))
	}
}

func (service *Service) respondPermission(
	managed *managedSession,
	requestID string,
	confirmed bool,
	cancelled bool,
) {
	if managed.runtime == nil || strings.TrimSpace(requestID) == "" {
		return
	}
	response := map[string]any{
		"type": "extension_ui_response", "id": requestID, "confirmed": confirmed,
	}
	if cancelled {
		response = map[string]any{
			"type": "extension_ui_response", "id": requestID, "cancelled": true,
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), service.requestTimeout)
	defer cancel()
	if err := sendRuntimeNotification(ctx, managed.runtime, response); err != nil {
		service.failGateProtocolLocked(managed, fmt.Sprintf("PI permission response failed: %v", err))
	}
}

func (service *Service) cancelGateRequest(managed *managedSession, requestID string) {
	if managed.runtime == nil || strings.TrimSpace(requestID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), service.requestTimeout)
	defer cancel()
	_ = sendRuntimeNotification(ctx, managed.runtime, map[string]any{
		"type":      "extension_ui_response",
		"id":        requestID,
		"cancelled": true,
	})
}

func (service *Service) failGateProtocolLocked(managed *managedSession, message string) {
	message = sanitizeError(message, managed.workspaceRoot)
	if managed.run != nil {
		managed.run.errorMessage = message
		_ = service.emitLocked(managed, "error", managed.run.record.ID, "", errorPayload(
			"protocol_error", message, false,
		))
		go abortRuntime(managed.runtime, service.requestTimeout)
		return
	}
	managed.record.State = "failed"
	managed.record.ErrorMessage = &message
	service.touchSessionLocked(managed)
	_ = service.store.UpsertAgentSession(managed.record)
}
