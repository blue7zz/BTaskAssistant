package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type gateBridgeRequest struct {
	Version    string          `json:"version"`
	Nonce      string          `json:"nonce"`
	TaskID     string          `json:"taskId"`
	SessionID  string          `json:"sessionId"`
	Mode       string          `json:"mode"`
	ToolCallID string          `json:"toolCallId"`
	Operation  string          `json:"operation"`
	Args       json.RawMessage `json:"args"`
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
		request.Mode != managed.record.Mode {
		service.cancelGateRequest(managed, event.ID)
		service.failGateProtocolLocked(managed, "PI gate request identity mismatch")
		return
	}
	if managed.run == nil || request.ToolCallID == "" {
		service.respondGate(managed, event.ID, nil, errors.New("PI gate request has no active run"))
		return
	}
	tool, exists := managed.run.tools[request.ToolCallID]
	wantTool := map[string]string{
		"list_resources": "btask_list_resources",
		"read_resource":  "btask_read_resource",
		"write_artifact": "btask_write_artifact",
	}[request.Operation]
	if !exists || wantTool == "" || tool.ToolName != wantTool {
		service.respondGate(managed, event.ID, nil, errors.New("PI gate request does not match its active tool call"))
		service.failGateProtocolLocked(managed, "PI gate tool-call identity mismatch")
		return
	}

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
	}
	service.respondGate(managed, event.ID, data, err)
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
