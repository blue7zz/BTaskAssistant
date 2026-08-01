package agent

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	permissionpolicy "github.com/blue7zz/BTaskAssistant/internal/permissions"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
	"github.com/blue7zz/BTaskAssistant/internal/taskspace"
)

const maxLazyToolOutputBytes = 2 * 1024 * 1024

func (service *Service) ToolCalls(taskID string, sessionID string) ([]storage.ToolCallRecord, error) {
	if _, err := service.store.AgentSession(taskID, sessionID); err != nil {
		return nil, err
	}
	return service.store.ToolCalls(taskID, sessionID)
}

func (service *Service) PermissionRequests(taskID string, sessionID string) ([]PermissionRequest, error) {
	if strings.TrimSpace(sessionID) != "" {
		if _, err := service.store.AgentSession(taskID, sessionID); err != nil {
			return nil, err
		}
	} else if _, err := service.store.EnsureTaskWorkspace(taskID); err != nil {
		return nil, err
	}
	records, err := service.store.PermissionRequests(taskID, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]PermissionRequest, 0, len(records))
	for _, record := range records {
		tool, toolErr := service.store.ToolCall(taskID, record.ToolCallID)
		if toolErr != nil {
			return nil, toolErr
		}
		result = append(result, service.permissionView(record, tool.ToolName))
	}
	return result, nil
}

func (service *Service) PermissionGrants(
	taskID string,
	sessionID string,
) ([]storage.PermissionGrantRecord, error) {
	if _, err := service.store.AgentSession(taskID, sessionID); err != nil {
		return nil, err
	}
	return service.store.ActivePermissionGrants(taskID, sessionID, service.timestamp())
}

func (service *Service) ResolvePermission(
	_ context.Context,
	request ResolvePermissionRequest,
) (PermissionRequest, error) {
	request.Decision = strings.ToLower(strings.TrimSpace(request.Decision))
	request.Scope = strings.ToLower(strings.TrimSpace(request.Scope))
	if request.Decision != "allow" && request.Decision != "deny" {
		return PermissionRequest{}, errors.New("权限决策必须是 allow 或 deny")
	}
	managed, err := service.loadManagedSession(request.TaskID, request.SessionID)
	if err != nil {
		return PermissionRequest{}, err
	}
	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	if managed.run == nil {
		return PermissionRequest{}, errors.New("权限请求已不属于活动运行")
	}
	pending := managed.run.permissions[request.RequestID]
	if pending == nil || pending.request.TaskID != request.TaskID || pending.request.SessionID != request.SessionID {
		return PermissionRequest{}, errors.New("权限请求不存在、已处理或不属于当前会话")
	}
	if !service.now().Before(pending.expiresAt) {
		service.resolvePendingPermissionLocked(managed, pending, "expired", "", "权限请求已超时")
		return PermissionRequest{}, storage.ErrPermissionRequestResolved
	}

	var grant *storage.PermissionGrantRecord
	state := "denied"
	reason := "用户拒绝了工具调用"
	var decisionScope *string
	if request.Decision == "allow" {
		allowed := allowedScopeStrings(permissionpolicy.RiskLevel(pending.request.RiskLevel))
		if !containsString(allowed, request.Scope) {
			return PermissionRequest{}, errors.New("所选授权作用域不适用于该风险等级")
		}
		state = "allowed"
		reason = "用户允许了工具调用"
		decisionScope = &request.Scope
		grantRecord := service.permissionGrantFor(pending.request, request.Scope)
		grant = &grantRecord
	}
	now := service.timestamp()
	resolved, err := service.store.ResolvePermissionRequest(storage.PermissionResolution{
		TaskID: request.TaskID, RequestID: request.RequestID, State: state,
		ResolvedAt: now, ResolvedBy: "user", DecisionScope: decisionScope, Reason: reason,
	}, grant)
	if err != nil {
		return PermissionRequest{}, err
	}
	service.completePendingPermissionLocked(managed, pending, resolved, request.Decision, request.Scope)
	tool, _ := service.store.ToolCall(request.TaskID, resolved.ToolCallID)
	return service.permissionView(resolved, tool.ToolName), nil
}

func (service *Service) RevokePermissionGrant(
	request RevokePermissionGrantRequest,
) (storage.PermissionGrantRecord, error) {
	record, err := service.store.RevokePermissionGrant(
		request.TaskID,
		request.GrantID,
		service.timestamp(),
	)
	if err != nil {
		return storage.PermissionGrantRecord{}, err
	}
	if record.RequestID != nil {
		if permissionRequest, requestErr := service.store.PermissionRequest(request.TaskID, *record.RequestID); requestErr == nil {
			if managed, managedErr := service.loadManagedSession(request.TaskID, permissionRequest.SessionID); managedErr == nil {
				managed.mutex.Lock()
				_ = service.emitLocked(managed, "permission.revoked", permissionRequest.RunID, permissionRequest.ToolCallID, map[string]any{
					"grantId": record.ID, "scope": record.Scope,
				})
				managed.mutex.Unlock()
			}
		}
	}
	return record, nil
}

func (service *Service) ReadToolOutput(request ToolOutputRequest) (ToolOutput, error) {
	tool, err := service.store.ToolCall(request.TaskID, request.ToolCallID)
	if err != nil {
		return ToolOutput{}, err
	}
	if tool.OutputRef == nil || strings.TrimSpace(*tool.OutputRef) == "" {
		return ToolOutput{}, errors.New("该工具调用没有可懒加载的输出")
	}
	logicalPath, err := taskspace.ValidateLogicalPath(*tool.OutputRef)
	if err != nil || logicalPath != *tool.OutputRef {
		return ToolOutput{}, errors.New("工具输出引用不安全")
	}
	wantPrefix := "runs/" + tool.RunID + "/"
	if strings.ContainsAny(tool.RunID, "/\\\x00") || !strings.HasPrefix(logicalPath, wantPrefix) {
		return ToolOutput{}, errors.New("工具输出引用不属于该运行")
	}
	filename := strings.TrimPrefix(logicalPath, wantPrefix)
	if filename == "" || strings.ContainsAny(filename, "/\\\x00") {
		return ToolOutput{}, errors.New("工具输出引用不是该运行的直接文件")
	}
	workspace, err := service.store.EnsureTaskWorkspace(request.TaskID)
	if err != nil {
		return ToolOutput{}, err
	}
	directoryRoot, err := os.OpenRoot(filepath.Join(workspace.RootPath, "runs", tool.RunID))
	if err != nil {
		return ToolOutput{}, errors.New("工具输出目录不存在或不安全")
	}
	defer directoryRoot.Close()
	info, err := directoryRoot.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ToolOutput{}, errors.New("工具输出文件不存在或不安全")
	}
	file, err := directoryRoot.Open(filename)
	if err != nil {
		return ToolOutput{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxLazyToolOutputBytes+1))
	if err != nil {
		return ToolOutput{}, err
	}
	read := len(content)
	truncated := read > maxLazyToolOutputBytes
	if truncated {
		read = maxLazyToolOutputBytes
	}
	for read > 0 && read < len(content) && !utf8.RuneStart(content[read]) {
		read--
	}
	content = content[:read]
	if !utf8.Valid(content) {
		return ToolOutput{}, errors.New("工具输出不是有效 UTF-8 文本")
	}
	return ToolOutput{
		Reference: logicalPath, Content: string(content), ByteSize: info.Size(), Truncated: truncated,
	}, nil
}

func (service *Service) permissionView(
	record storage.PermissionRequestRecord,
	toolName string,
) PermissionRequest {
	view := PermissionRequest{
		ID: record.ID, TaskID: record.TaskID, SessionID: record.SessionID,
		RunID: record.RunID, ToolCallID: record.ToolCallID, ToolName: toolName,
		Capability: record.Capability, Target: record.Target, Subject: record.Subject,
		RiskLevel: record.RiskLevel, State: record.State, RequestedAt: record.RequestedAt,
		AllowedScopes: []string{},
	}
	if record.NormalizedTarget != nil {
		view.NormalizedTarget = *record.NormalizedTarget
	}
	if record.ResolvedAt != nil {
		view.ResolvedAt = *record.ResolvedAt
	}
	if record.ResolvedBy != nil {
		view.ResolvedBy = *record.ResolvedBy
	}
	if record.DecisionScope != nil {
		view.DecisionScope = *record.DecisionScope
	}
	if record.Reason != nil {
		view.Reason = *record.Reason
	}
	if record.State == "pending" {
		view.AllowedScopes = allowedScopeStrings(permissionpolicy.RiskLevel(record.RiskLevel))
		if requestedAt, err := time.Parse(time.RFC3339Nano, record.RequestedAt); err == nil {
			view.ExpiresAt = requestedAt.Add(service.permissionTimeout).UTC().Format(time.RFC3339Nano)
		}
	}
	return view
}

func (service *Service) permissionGrantFor(
	request storage.PermissionRequestRecord,
	scope string,
) storage.PermissionGrantRecord {
	now := service.timestamp()
	taskID := request.TaskID
	sessionID := request.SessionID
	requestID := request.ID
	grant := storage.PermissionGrantRecord{
		ID: newID("grant"), RequestID: &requestID, Capability: request.Capability,
		TargetPattern: stringValue(request.NormalizedTarget), Scope: scope,
		Decision: "allow", RiskCeiling: request.RiskLevel, CreatedAt: now, CreatedBy: "user",
	}
	switch scope {
	case "once":
		grant.TaskID = &taskID
		grant.SessionID = &sessionID
		grant.ConsumedAt = &now
	case "session":
		grant.TaskID = &taskID
		grant.SessionID = &sessionID
	case "task":
		grant.TaskID = &taskID
	case "permanent":
		// The source request id is retained for audit; permanent scope remains app-wide.
	}
	return grant
}

func allowedScopeStrings(risk permissionpolicy.RiskLevel) []string {
	scopes := permissionpolicy.AllowedScopes(risk)
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, string(scope))
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (service *Service) expirePermission(requestID string, taskID string, sessionID string) {
	managed, err := service.loadManagedSession(taskID, sessionID)
	if err != nil {
		return
	}
	managed.mutex.Lock()
	defer managed.mutex.Unlock()
	if managed.run == nil {
		return
	}
	pending := managed.run.permissions[requestID]
	if pending == nil || service.now().Before(pending.expiresAt) {
		return
	}
	service.resolvePendingPermissionLocked(managed, pending, "expired", "", "权限请求等待超时")
}

func (service *Service) cancelPendingPermissionsLocked(managed *managedSession, reason string) {
	if managed.run == nil || len(managed.run.permissions) == 0 {
		return
	}
	pending := make([]*pendingPermission, 0, len(managed.run.permissions))
	for _, request := range managed.run.permissions {
		pending = append(pending, request)
	}
	for _, request := range pending {
		service.resolvePendingPermissionLocked(managed, request, "cancelled", "", reason)
	}
}

func (service *Service) resolvePendingPermissionLocked(
	managed *managedSession,
	pending *pendingPermission,
	state string,
	scope string,
	reason string,
) {
	var scopePointer *string
	if scope != "" {
		scopePointer = &scope
	}
	resolved, err := service.store.ResolvePermissionRequest(storage.PermissionResolution{
		TaskID: pending.request.TaskID, RequestID: pending.request.ID,
		State: state, ResolvedAt: service.timestamp(), ResolvedBy: "system",
		DecisionScope: scopePointer, Reason: reason,
	}, nil)
	if err != nil {
		return
	}
	decision := "deny"
	if state == "allowed" {
		decision = "allow"
	}
	service.completePendingPermissionLocked(managed, pending, resolved, decision, scope)
}

func (service *Service) completePendingPermissionLocked(
	managed *managedSession,
	pending *pendingPermission,
	resolved storage.PermissionRequestRecord,
	decision string,
	scope string,
) {
	if managed.run == nil {
		return
	}
	delete(managed.run.permissions, pending.request.ID)
	tool, exists := managed.run.toolsByRecordID(resolved.ToolCallID)
	if !exists {
		service.respondPermission(managed, pending.extensionUIRequestID, false, true)
		return
	}
	if tool.FinishedAt != nil || tool.State == "succeeded" || tool.State == "failed" {
		_ = service.emitLocked(managed, "permission.resolved", resolved.RunID, tool.ID, map[string]any{
			"requestId": resolved.ID, "decision": resolved.State, "scope": scope,
		})
		service.respondPermission(
			managed,
			pending.extensionUIRequestID,
			false,
			resolved.State == "expired" || resolved.State == "cancelled",
		)
		return
	}
	if decision == "allow" {
		managed.run.receipts[tool.ExternalToolCallID] = permissionReceipt{
			RequestID: resolved.ID, Capability: resolved.Capability,
			NormalizedTarget: stringValue(resolved.NormalizedTarget),
			ArgsDigest:       service.toolArgsDigest(managed.run, tool.ExternalToolCallID),
			Scope:            scope, Decision: "allow", Mutating: !gateToolPolicies[tool.ToolName].readOnly,
		}
		tool.State = "running"
		managed.run.record.State = "running"
	} else {
		managed.run.receipts[tool.ExternalToolCallID] = permissionReceipt{
			RequestID: resolved.ID, Capability: resolved.Capability,
			NormalizedTarget: stringValue(resolved.NormalizedTarget),
			ArgsDigest:       service.toolArgsDigest(managed.run, tool.ExternalToolCallID),
			Scope:            scope, Decision: resolved.State, Mutating: !gateToolPolicies[tool.ToolName].readOnly,
		}
		tool.State = map[string]string{"denied": "denied", "expired": "cancelled", "cancelled": "cancelled"}[resolved.State]
		if tool.State == "" {
			tool.State = "denied"
		}
		managed.run.record.State = "running"
	}
	_ = service.store.UpsertToolCall(tool)
	_ = service.store.UpsertExecutionRun(managed.run.record)
	managed.run.tools[tool.ExternalToolCallID] = tool
	_ = service.emitLocked(managed, "permission.resolved", resolved.RunID, tool.ID, map[string]any{
		"requestId": resolved.ID, "decision": resolved.State, "scope": scope,
	})
	service.respondPermission(
		managed,
		pending.extensionUIRequestID,
		decision == "allow",
		resolved.State == "expired" || resolved.State == "cancelled",
	)
}

func (run *activeRun) toolsByRecordID(recordID string) (storage.ToolCallRecord, bool) {
	for _, tool := range run.tools {
		if tool.ID == recordID {
			return tool, true
		}
	}
	return storage.ToolCallRecord{}, false
}

func (service *Service) toolArgsDigest(run *activeRun, externalToolCallID string) string {
	raw := run.toolArgs[externalToolCallID]
	digest, _ := permissionpolicy.CanonicalArgsDigest(raw)
	return digest
}
