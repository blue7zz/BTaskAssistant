package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	permissionpolicy "github.com/blue7zz/BTaskAssistant/internal/permissions"
	"github.com/blue7zz/BTaskAssistant/internal/storage"
)

func TestServicePersistsMultiTurnMessagesAndStableEvents(t *testing.T) {
	store := newServiceTestStore(t, "task_chat", "聊天任务")
	factory := &fakeRuntimeFactory{}
	var emittedMutex sync.Mutex
	emitted := []Event{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory,
		RequestTimeout: time.Second,
		ShutdownGrace:  100 * time.Millisecond,
		Emit: func(event Event) {
			emittedMutex.Lock()
			emitted = append(emitted, event)
			emittedMutex.Unlock()
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})

	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_chat", Title: "最小会话", Mode: "ask",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for _, prompt := range []string{"第一轮", "第二轮"} {
		run, err := service.SendPrompt(context.Background(), PromptRequest{
			TaskID: "task_chat", SessionID: session.ID, Message: prompt,
		})
		if err != nil {
			t.Fatalf("send %s: %v", prompt, err)
		}
		waitForRunState(t, store, "task_chat", run.ID, "succeeded")
	}

	if factory.count() != 1 {
		t.Fatalf("multi-turn chat started %d processes", factory.count())
	}
	messages, err := service.Messages("task_chat", session.ID)
	if err != nil {
		t.Fatalf("load messages: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected two user/assistant pairs, got %#v", messages)
	}
	if messages[1].Content == nil || !strings.Contains(*messages[1].Content, "第一轮") ||
		messages[3].Content == nil || !strings.Contains(*messages[3].Content, "第二轮") {
		t.Fatalf("assistant messages were not reconciled: %#v", messages)
	}
	var events []storage.AgentEventRecord
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events, err = store.AgentEvents("task_chat", session.ID)
		if err != nil {
			t.Fatal(err)
		}
		emittedMutex.Lock()
		count := len(emitted)
		emittedMutex.Unlock()
		if count > 0 && count == len(events) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(events) == 0 {
		t.Fatal("no durable agent events")
	}
	for index := 1; index < len(events); index++ {
		if events[index].Sequence <= events[index-1].Sequence {
			t.Fatalf("event sequence is not monotonic: %#v", events)
		}
	}
	emittedMutex.Lock()
	defer emittedMutex.Unlock()
	if len(emitted) != len(events) {
		t.Fatalf("Wails events %d do not match durable events %d", len(emitted), len(events))
	}
	for _, event := range emitted {
		if event.TaskID != "task_chat" || event.SessionID != session.ID || event.Version != 1 {
			t.Fatalf("event envelope leaked scope: %#v", event)
		}
	}
}

func TestServicePagesHistoryAndUsesDistinctSteerAndFollowUpCommands(t *testing.T) {
	store := newServiceTestStore(t, "task_queue", "队列任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_queue", Title: "队列会话", Mode: "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: session.TaskID, SessionID: session.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	steering, err := service.Steer(context.Background(), PromptRequest{
		TaskID: session.TaskID, SessionID: session.ID, Message: "先处理错误路径",
	})
	if err != nil {
		t.Fatalf("steer: %v", err)
	}
	followUp, err := service.FollowUp(context.Background(), PromptRequest{
		TaskID: session.TaskID, SessionID: session.ID, Message: "完成后总结测试",
	})
	if err != nil {
		t.Fatalf("follow-up: %v", err)
	}
	if steering.Status != "pending" || followUp.Status != "pending" {
		t.Fatalf("queued messages were not visibly pending: %#v %#v", steering, followUp)
	}
	runtime := factory.runtime(0)
	if !runtime.called("steer") || !runtime.called("follow_up") {
		t.Fatal("PI queue commands were conflated")
	}
	page, err := service.HistoryPage(HistoryPageRequest{
		TaskID: session.TaskID, SessionID: session.ID, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || page.NextCursor == "" || len(page.Messages) != 2 ||
		page.Messages[0].ID != steering.ID || page.Messages[1].ID != followUp.ID {
		t.Fatalf("unexpected latest history page: %#v", page)
	}
	older, err := service.HistoryPage(HistoryPageRequest{
		TaskID: session.TaskID, SessionID: session.ID, Cursor: page.NextCursor, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Messages) != 2 || older.Messages[0].Role != "user" || older.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected older history page: %#v", older)
	}
	if _, err := service.HistoryPage(HistoryPageRequest{
		TaskID: session.TaskID, SessionID: session.ID, Cursor: "not-a-cursor", Limit: 2,
	}); err == nil {
		t.Fatal("invalid history cursor was accepted")
	}
	if err := service.Abort(context.Background(), AbortRequest{
		TaskID: session.TaskID, SessionID: session.ID, RunID: run.ID,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestServiceExplicitlyResumesAFailedSession(t *testing.T) {
	store := newServiceTestStore(t, "task_resume_action", "显式恢复任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_resume_action", Title: "恢复会话", Mode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	factory.runtime(0).crash(errors.New("fake PI crash"), "provider exited")
	waitForSessionState(t, store, session.TaskID, session.ID, "failed")

	resumed, err := service.ResumeSession(context.Background(), SessionRequest{
		TaskID: session.TaskID, SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("resume session: %v", err)
	}
	if resumed.State != "idle" || resumed.ErrorMessage != nil || factory.count() != 2 {
		t.Fatalf("unexpected resumed session %#v; runtimes=%d", resumed, factory.count())
	}
}

func TestServiceAbortCancelsOnlyMatchingRun(t *testing.T) {
	store := newServiceTestStore(t, "task_abort", "停止任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_abort", Title: "停止会话", Mode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_abort", SessionID: session.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !service.ActiveTask("task_abort") || service.ActiveTask("task_other") {
		t.Fatal("active task registry did not match the running PI request")
	}
	if err := service.Abort(context.Background(), AbortRequest{
		TaskID: "task_abort", SessionID: session.ID, RunID: "wrong-run",
	}); err == nil {
		t.Fatal("mismatched abort was accepted")
	}
	if err := service.Abort(context.Background(), AbortRequest{
		TaskID: "task_abort", SessionID: session.ID, RunID: run.ID,
	}); err != nil {
		t.Fatalf("abort run: %v", err)
	}
	waitForRunState(t, store, "task_abort", run.ID, "cancelled")
	if service.ActiveTask("task_abort") {
		t.Fatal("cancelled PI run remained active")
	}
}

func TestServiceAllowsOnlyOneActiveRunPerTaskAcrossSessions(t *testing.T) {
	store := newServiceTestStore(t, "task_single", "单运行任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	first, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_single", Title: "会话一",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_single", Title: "会话二",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_single", SessionID: first.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_single", SessionID: second.ID, Message: "并发回答",
	}); err == nil {
		t.Fatal("second active run for the same task was accepted")
	}
	workspace, err := store.TaskWorkspace("task_single")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(workspace.RootPath, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != active.ID {
		t.Fatalf("rejected run left orphan files: %#v", entries)
	}
	if err := service.Abort(context.Background(), AbortRequest{
		TaskID: "task_single", SessionID: first.ID, RunID: active.ID,
	}); err != nil {
		t.Fatal(err)
	}
	waitForRunState(t, store, "task_single", active.ID, "cancelled")
}

func TestServiceStopsToolsOutsideTheTaskGateAllowlist(t *testing.T) {
	store := newServiceTestStore(t, "task_no_tools", "无工具任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_no_tools",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_no_tools", SessionID: session.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	factory.runtime(0).emit(map[string]any{
		"type":       "tool_execution_start",
		"toolCallId": "external-shell",
		"toolName":   "bash",
		"args":       map[string]any{"command": "pwd"},
	})
	waitForRunState(t, store, "task_no_tools", run.ID, "failed")
	stored, err := store.ExecutionRun("task_no_tools", run.ID)
	if err != nil || stored.ErrorMessage == nil ||
		!strings.Contains(*stored.ErrorMessage, "未允许") {
		t.Fatalf("unexpected task-gate failure %#v, %v", stored, err)
	}
}

func TestServiceRejectsWorktreeMutationAndShellOutsideAgentMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		tool string
		args map[string]any
	}{
		{name: "ask shell", mode: "ask", tool: "btask_shell", args: map[string]any{"command": "go test ./...", "cwd": ".", "timeoutSeconds": 30}},
		{name: "plan worktree write", mode: "plan", tool: "btask_write_worktree_file", args: map[string]any{"path": "new.go", "content": "package main\n"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskID := "task_mode_" + strings.ReplaceAll(test.mode, "-", "_")
			store := newServiceTestStore(t, taskID, test.name)
			factory := &fakeRuntimeFactory{}
			service := NewService(store, ServiceOptions{RuntimeFactory: factory, RequestTimeout: time.Second})
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = service.Close(ctx)
			})
			session, err := service.CreateSession(context.Background(), CreateSessionRequest{
				TaskID: taskID, Mode: test.mode,
			})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.SendPrompt(context.Background(), PromptRequest{
				TaskID: taskID, SessionID: session.ID, Message: "hold",
			})
			if err != nil {
				t.Fatal(err)
			}
			factory.runtime(0).emit(map[string]any{
				"type": "tool_execution_start", "toolCallId": "mode-tool",
				"toolName": test.tool, "args": test.args,
			})
			waitForRunState(t, store, taskID, run.ID, "failed")
			tools, err := service.ToolCalls(taskID, session.ID)
			if err != nil || len(tools) != 1 || tools[0].Capability != "unexpected.tool" || tools[0].RiskLevel != "critical" {
				t.Fatalf("mode-rejected tool was not audited fail-closed: %#v, %v", tools, err)
			}
		})
	}
}

func TestServiceTaskIsolationAndAbnormalExit(t *testing.T) {
	store := newServiceTestStore(t, "task_alpha", "Alpha")
	seedServiceTask(t, store, "task_beta", "Beta")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	alpha, err := service.CreateSession(context.Background(), CreateSessionRequest{TaskID: "task_alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := service.CreateSession(context.Background(), CreateSessionRequest{TaskID: "task_beta"})
	if err != nil {
		t.Fatal(err)
	}
	alphaTaskID := "task_alpha"
	alphaSessionID := alpha.ID
	if err := store.UpsertPermissionGrant(storage.PermissionGrantRecord{
		ID: "grant-crash-session", TaskID: &alphaTaskID, SessionID: &alphaSessionID,
		Capability: "task.resource.read", TargetPattern: "*", Scope: "session",
		Decision: "allow", RiskCeiling: "medium",
		CreatedAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), CreatedBy: "user",
	}); err != nil {
		t.Fatal(err)
	}
	alphaRun, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_alpha", SessionID: alpha.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Messages("task_beta", alpha.ID); err == nil {
		t.Fatal("cross-task messages were readable")
	}
	factory.runtime(0).crash(errors.New("fake provider crash"), "secret/path")
	waitForRunState(t, store, "task_alpha", alphaRun.ID, "interrupted")
	storedAlpha := waitForSessionState(t, store, "task_alpha", alpha.ID, "failed")
	_, err = store.AgentSession("task_alpha", alpha.ID)
	if err != nil || storedAlpha.State != "failed" {
		t.Fatalf("unexpected crashed session %#v, %v", storedAlpha, err)
	}
	grants, err := store.ActivePermissionGrants(
		"task_alpha", alpha.ID, time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano),
	)
	if err != nil || len(grants) != 0 {
		t.Fatalf("session grant survived PI crash: %#v, %v", grants, err)
	}
	storedBeta, err := store.AgentSession("task_beta", beta.ID)
	if err != nil || storedBeta.State != "idle" {
		t.Fatalf("other task changed after crash %#v, %v", storedBeta, err)
	}
}

func TestServiceImportsAndPromptsWithTaskScopedReferences(t *testing.T) {
	store := newServiceTestStore(t, "task_reference_a", "引用任务 A")
	seedServiceTask(t, store, "task_reference_b", "引用任务 B")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{RuntimeFactory: factory, RequestTimeout: time.Second})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})

	imageBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	for _, taskID := range []string{"task_reference_a", "task_reference_b"} {
		if _, err := service.ImportAttachments(ImportAttachmentsRequest{
			TaskID: taskID,
			Files: []AttachmentUpload{
				{Name: "same.md", MIMEType: "text/markdown", DataBase64: base64.StdEncoding.EncodeToString([]byte("document for " + taskID))},
				{Name: "same.png", MIMEType: "image/png", DataBase64: imageBase64},
			},
		}); err != nil {
			t.Fatalf("import %s attachments: %v", taskID, err)
		}
	}
	resourcesA, err := service.Resources(ResourceSearchRequest{TaskID: "task_reference_a", Query: "same", Limit: 10})
	if err != nil || len(resourcesA) != 2 {
		t.Fatalf("unexpected task A resources %#v, error %v", resourcesA, err)
	}
	resourcesB, err := service.Resources(ResourceSearchRequest{TaskID: "task_reference_b", Query: "same", Limit: 10})
	if err != nil || len(resourcesB) != 2 {
		t.Fatalf("unexpected task B resources %#v, error %v", resourcesB, err)
	}
	for _, left := range resourcesA {
		for _, right := range resourcesB {
			if left.ID == right.ID {
				t.Fatalf("resource id crossed task scope: %s", left.ID)
			}
		}
	}
	if _, err := service.PreviewResource(ResourcePreviewRequest{
		TaskID: "task_reference_b", ResourceID: resourcesA[0].ID,
	}); err == nil {
		t.Fatal("task B previewed a task A resource id")
	}

	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_reference_a", Mode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	resourceIDs := []string{resourcesA[0].ID, resourcesA[1].ID}
	run, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_reference_a", SessionID: session.ID,
		Message: "检查引用", ResourceIDs: resourceIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunState(t, store, "task_reference_a", run.ID, "succeeded")
	promptFields := factory.runtime(0).lastCallFields("prompt")
	prompt, _ := promptFields["message"].(string)
	if !strings.Contains(prompt, resourcesA[0].ID) || !strings.Contains(prompt, resourcesA[1].ID) ||
		strings.Contains(prompt, "document for task_reference_a") {
		t.Fatalf("prompt reference manifest is incorrect: %q", prompt)
	}
	images, ok := promptFields["images"].([]map[string]any)
	if !ok || len(images) != 1 || images[0]["mimeType"] != "image/png" {
		t.Fatalf("prompt image payload is incorrect: %#v", promptFields["images"])
	}
	messages, err := service.Messages("task_reference_a", session.ID)
	if err != nil || len(messages) < 2 || len(messages[0].References) != 2 {
		t.Fatalf("message references were not persisted: %#v, error %v", messages, err)
	}
	removed := messages[0].References[0]
	if err := service.RemoveReference(RemoveReferenceRequest{
		TaskID: "task_reference_a", SessionID: session.ID,
		MessageID: messages[0].ID, ResourceID: removed.ResourceID,
	}); err != nil {
		t.Fatal(err)
	}
	messages, err = service.Messages("task_reference_a", session.ID)
	if err != nil || len(messages[0].References) != 1 {
		t.Fatalf("reference removal was not durable: %#v, error %v", messages, err)
	}
	if _, err := service.PreviewResource(ResourcePreviewRequest{
		TaskID: "task_reference_a", ResourceID: removed.ResourceID,
	}); err != nil {
		t.Fatalf("removing a reference deleted immutable attachment bytes: %v", err)
	}
	if _, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_reference_a", SessionID: session.ID,
	}); err == nil {
		t.Fatal("empty prompt without references was accepted")
	}
	attachmentOnlyRun, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_reference_a", SessionID: session.ID,
		ResourceIDs: []string{resourcesA[0].ID},
	})
	if err != nil {
		t.Fatalf("send attachment-only prompt: %v", err)
	}
	waitForRunState(t, store, "task_reference_a", attachmentOnlyRun.ID, "succeeded")
	attachmentOnlyPrompt, _ := factory.runtime(0).lastCallFields("prompt")["message"].(string)
	if !strings.Contains(attachmentOnlyPrompt, "请查看所附的当前任务资源") ||
		!strings.Contains(attachmentOnlyPrompt, resourcesA[0].ID) {
		t.Fatalf("attachment-only prompt did not receive the safe fallback and manifest: %q", attachmentOnlyPrompt)
	}

	restarted := NewService(store, ServiceOptions{RuntimeFactory: &fakeRuntimeFactory{}})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = restarted.Close(ctx)
	}()
	restartedMessages, err := restarted.Messages("task_reference_a", session.ID)
	if err != nil || len(restartedMessages[0].References) != 1 {
		t.Fatalf("references did not survive service restart: %#v, error %v", restartedMessages, err)
	}
}

func TestPlanGateWritesArtifactsAndRequirementProposals(t *testing.T) {
	store := newServiceTestStore(t, "task_gate_artifact", "Artifact 任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{RuntimeFactory: factory, RequestTimeout: time.Second})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_gate_artifact", Mode: "plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_gate_artifact", SessionID: session.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := factory.runtime(0)
	toolArgs := json.RawMessage(`{"kind":"proposal","name":"scope.md","content":"# 范围修改建议\n"}`)
	var emittedArgs map[string]any
	if err := json.Unmarshal(toolArgs, &emittedArgs); err != nil {
		t.Fatal(err)
	}
	runtime.emit(map[string]any{
		"type": "tool_execution_start", "toolCallId": "artifact-call",
		"toolName": "btask_write_artifact",
		"args":     emittedArgs,
	})
	options := factory.processOptions(0)
	classification := permissionpolicy.ClassifyTool(permissionpolicy.ToolInput{
		TaskID: "task_gate_artifact", ToolName: "btask_write_artifact", Args: toolArgs,
	})
	envelope, _ := json.Marshal(permissionEnvelope{
		Protocol: permissionProtocolVersion, Version: options.Gate.Version, Nonce: options.Gate.Nonce,
		TaskID: "task_gate_artifact", SessionID: session.ID, RunID: run.ID, Mode: "plan",
		ToolCallID: "artifact-call", ToolName: "btask_write_artifact",
		Capability: classification.Capability, Subject: classification.Subject,
		Target: classification.Target, NormalizedTarget: classification.NormalizedTarget,
		RiskLevel: string(classification.RiskLevel), ArgsDigest: classification.ArgsDigest,
	})
	runtime.emit(map[string]any{
		"type": "extension_ui_request", "id": "permission-request-1",
		"method": "confirm", "title": "btask-permission", "message": string(envelope),
	})
	permissionResponse := runtime.waitNotification(t)
	if confirmed, _ := permissionResponse["confirmed"].(bool); !confirmed {
		t.Fatalf("artifact permission was not allowed: %#v", permissionResponse)
	}
	request := gateBridgeRequest{
		Version: options.Gate.Version, Nonce: options.Gate.Nonce,
		TaskID: "task_gate_artifact", SessionID: session.ID, RunID: run.ID, Mode: "plan",
		ToolCallID: "artifact-call", Operation: "write_artifact",
		Args: toolArgs,
	}
	placeholder, _ := json.Marshal(request)
	runtime.emit(map[string]any{
		"type": "extension_ui_request", "id": "gate-request-1",
		"method": "input", "title": "btask-gate", "placeholder": string(placeholder),
	})
	response := runtime.waitNotificationID(t, "gate-request-1")
	value, _ := response["value"].(string)
	var bridgeResponse gateBridgeResponse
	if err := json.Unmarshal([]byte(value), &bridgeResponse); err != nil || !bridgeResponse.OK {
		t.Fatalf("artifact gate response failed: %#v, decode error %v", response, err)
	}
	artifacts, err := service.Artifacts("task_gate_artifact")
	if err != nil || len(artifacts) != 1 || artifacts[0].LogicalPath != "artifacts/proposals/scope.md" ||
		artifacts[0].ProposalState == nil || *artifacts[0].ProposalState != "pending" {
		t.Fatalf("proposal artifact was not indexed: %#v, error %v", artifacts, err)
	}
	preview, err := service.PreviewResource(ResourcePreviewRequest{
		TaskID: "task_gate_artifact", ResourceID: artifacts[0].ID,
	})
	if err != nil || !strings.Contains(preview.Content, "范围修改建议") {
		t.Fatalf("proposal preview is unavailable: %#v, error %v", preview, err)
	}
	proposals, err := store.RequirementProposals("task_gate_artifact")
	if err != nil || len(proposals) != 1 || proposals[0].BaseRevision != 1 || proposals[0].State != "pending" {
		t.Fatalf("requirement proposal row is incorrect: %#v, error %v", proposals, err)
	}
	messages, err := service.Messages("task_gate_artifact", session.ID)
	if err != nil || len(messages) < 2 || len(messages[1].References) != 1 ||
		messages[1].References[0].Method != "generated" {
		t.Fatalf("generated artifact was not linked to the assistant message: %#v, error %v", messages, err)
	}
	runtime.emit(map[string]any{
		"type": "tool_execution_end", "toolCallId": "artifact-call",
		"toolName": "btask_write_artifact", "result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": "created"}},
		},
	})
	runtime.emit(map[string]any{"type": "agent_settled"})
	waitForRunState(t, store, "task_gate_artifact", run.ID, "succeeded")
}

func TestPermissionProtocolAllowDenyTimeoutCancelAndLargeOutput(t *testing.T) {
	t.Run("allow once and reconcile large output", func(t *testing.T) {
		service, store, factory := newPermissionTestService(t, "task_permission_allow", time.Second)
		session, run, runtime, request := startPermissionProbe(t, service, store, factory, "task_permission_allow")
		resolved, err := service.ResolvePermission(context.Background(), ResolvePermissionRequest{
			TaskID: "task_permission_allow", SessionID: session.ID,
			RequestID: request.ID, Decision: "allow", Scope: "once",
		})
		if err != nil || resolved.State != "allowed" || resolved.DecisionScope != "once" {
			t.Fatalf("allow once failed: %#v, %v", resolved, err)
		}
		if _, err := service.ResolvePermission(context.Background(), ResolvePermissionRequest{
			TaskID: "task_permission_allow", SessionID: session.ID,
			RequestID: request.ID, Decision: "allow", Scope: "once",
		}); err == nil {
			t.Fatal("repeated permission submission was accepted")
		}
		response := runtime.waitNotificationID(t, "probe-confirm")
		if confirmed, _ := response["confirmed"].(bool); !confirmed {
			t.Fatalf("allow did not resume PI confirm: %#v", response)
		}
		emitProbeBridge(t, runtime, factory.processOptions(0), session.ID, run.ID)
		bridgeResponse := runtime.waitNotificationID(t, "probe-bridge")
		if value, _ := bridgeResponse["value"].(string); !strings.Contains(value, `"approved":true`) {
			t.Fatalf("approved probe did not execute: %#v", bridgeResponse)
		}
		large := strings.Repeat("output-", 12000)
		runtime.emit(map[string]any{
			"type": "tool_execution_update", "toolCallId": "probe-call",
			"toolName": "btask_permission_probe", "partialResult": map[string]any{
				"content": []map[string]any{{"type": "text", "text": large}},
			},
		})
		runtime.emit(map[string]any{
			"type": "tool_execution_end", "toolCallId": "probe-call",
			"toolName": "btask_permission_probe", "result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": large}},
			},
		})
		runtime.emit(map[string]any{"type": "agent_settled"})
		waitForRunState(t, store, "task_permission_allow", run.ID, "succeeded")
		tools, err := service.ToolCalls("task_permission_allow", session.ID)
		if err != nil || len(tools) != 1 || tools[0].State != "succeeded" || tools[0].OutputRef == nil {
			t.Fatalf("tool receipt/output was not reconciled: %#v, %v", tools, err)
		}
		output, err := service.ReadToolOutput(ToolOutputRequest{
			TaskID: "task_permission_allow", ToolCallID: tools[0].ID,
		})
		if err != nil || !strings.HasPrefix(output.Content, "output-output-") || output.ByteSize <= 32*1024 {
			t.Fatalf("large output was not loaded lazily: %#v, %v", output, err)
		}
	})

	t.Run("deny prevents bridge execution", func(t *testing.T) {
		service, store, factory := newPermissionTestService(t, "task_permission_deny", time.Second)
		session, run, runtime, request := startPermissionProbe(t, service, store, factory, "task_permission_deny")
		resolved, err := service.ResolvePermission(context.Background(), ResolvePermissionRequest{
			TaskID: "task_permission_deny", SessionID: session.ID,
			RequestID: request.ID, Decision: "deny",
		})
		if err != nil || resolved.State != "denied" {
			t.Fatalf("deny failed: %#v, %v", resolved, err)
		}
		response := runtime.waitNotificationID(t, "probe-confirm")
		if confirmed, _ := response["confirmed"].(bool); confirmed || response["cancelled"] == true {
			t.Fatalf("deny returned the wrong PI response: %#v", response)
		}
		runtime.emit(map[string]any{
			"type": "tool_execution_end", "toolCallId": "probe-call",
			"toolName": "btask_permission_probe", "isError": true,
			"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "blocked"}}},
		})
		runtime.emit(map[string]any{"type": "agent_settled"})
		waitForRunState(t, store, "task_permission_deny", run.ID, "succeeded")
		tools, _ := service.ToolCalls("task_permission_deny", session.ID)
		if len(tools) != 1 || tools[0].State != "denied" {
			t.Fatalf("denied tool was treated as executed: %#v", tools)
		}
	})

	t.Run("timeout expires without grant", func(t *testing.T) {
		service, store, factory := newPermissionTestService(t, "task_permission_timeout", 30*time.Millisecond)
		session, _, runtime, request := startPermissionProbe(t, service, store, factory, "task_permission_timeout")
		response := runtime.waitNotificationID(t, "probe-confirm")
		if response["cancelled"] != true {
			t.Fatalf("timeout did not cancel PI confirm: %#v", response)
		}
		stored, err := store.PermissionRequest("task_permission_timeout", request.ID)
		if err != nil || stored.State != "expired" || stored.DecisionScope != nil {
			t.Fatalf("timeout permission state is wrong: %#v, %v", stored, err)
		}
		grants, err := service.PermissionGrants("task_permission_timeout", session.ID)
		if err != nil || len(grants) != 0 {
			t.Fatalf("timeout created a grant: %#v, %v", grants, err)
		}
	})

	t.Run("abort cancels pending permission", func(t *testing.T) {
		service, store, factory := newPermissionTestService(t, "task_permission_cancel", time.Second)
		session, run, runtime, request := startPermissionProbe(t, service, store, factory, "task_permission_cancel")
		if err := service.Abort(context.Background(), AbortRequest{
			TaskID: "task_permission_cancel", SessionID: session.ID, RunID: run.ID,
		}); err != nil {
			t.Fatal(err)
		}
		response := runtime.waitNotificationID(t, "probe-confirm")
		if response["cancelled"] != true {
			t.Fatalf("abort did not cancel PI confirm: %#v", response)
		}
		stored, err := store.PermissionRequest("task_permission_cancel", request.ID)
		if err != nil || stored.State != "cancelled" {
			t.Fatalf("abort permission state is wrong: %#v, %v", stored, err)
		}
	})
}

func TestPermissionProtocolBadNonceExtensionErrorAndMissingReceiptFailClosed(t *testing.T) {
	service, store, factory := newPermissionTestService(t, "task_permission_fail_closed", time.Second)
	session, run, runtime, _ := startPermissionProbe(t, service, store, factory, "task_permission_fail_closed")
	badEnvelope := permissionEnvelope{
		Protocol: permissionProtocolVersion, Version: gateExtensionVersion, Nonce: "bad-nonce",
		TaskID: "task_permission_fail_closed", SessionID: session.ID, RunID: run.ID,
		Mode: "agent", ToolCallID: "probe-call", ToolName: "btask_permission_probe",
	}
	encoded, _ := json.Marshal(badEnvelope)
	runtime.emit(map[string]any{
		"type": "extension_ui_request", "id": "bad-confirm", "method": "confirm",
		"title": "btask-permission", "message": string(encoded),
	})
	response := runtime.waitNotificationID(t, "bad-confirm")
	if response["cancelled"] != true {
		t.Fatalf("bad nonce was not rejected: %#v", response)
	}
	waitForRunState(t, store, "task_permission_fail_closed", run.ID, "failed")

	service2, store2, factory2 := newPermissionTestService(t, "task_permission_extension_error", time.Second)
	session2, run2, runtime2, _ := startPermissionProbe(t, service2, store2, factory2, "task_permission_extension_error")
	runtime2.emit(map[string]any{"type": "extension_error", "error": "gate crashed"})
	waitForRunState(t, store2, "task_permission_extension_error", run2.ID, "failed")
	if session2.ID == "" {
		t.Fatal("unreachable")
	}

	service3, store3, factory3 := newPermissionTestService(t, "task_permission_receipt", time.Second)
	session3, run3, runtime3, _ := startPermissionProbe(t, service3, store3, factory3, "task_permission_receipt")
	runtime3.emit(map[string]any{
		"type": "tool_execution_end", "toolCallId": "probe-call",
		"toolName": "btask_permission_probe", "result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": "bypassed"}},
		},
	})
	runtime3.emit(map[string]any{"type": "agent_settled"})
	waitForRunState(t, store3, "task_permission_receipt", run3.ID, "failed")
	tools, _ := service3.ToolCalls("task_permission_receipt", session3.ID)
	if len(tools) != 1 || tools[0].State != "failed" {
		t.Fatalf("missing receipt was not treated as security failure: %#v", tools)
	}
}

func TestPermissionAuditRedactsSecrets(t *testing.T) {
	for limit := 0; limit < 8; limit++ {
		bounded := boundedText("界界界", limit)
		if len(bounded) > limit || !utf8.ValidString(bounded) {
			t.Fatalf("bounded UTF-8 text exceeded %d bytes: %q", limit, bounded)
		}
	}
	redacted := string(redactToolArgs(json.RawMessage(`{"token":"top-secret","api-key":"key-value","nested":{"password":"hunter2"},"safe":"ok"}`)))
	if strings.Contains(redacted, "top-secret") || strings.Contains(redacted, "hunter2") ||
		strings.Contains(redacted, "key-value") || !strings.Contains(redacted, "[REDACTED]") ||
		!strings.Contains(redacted, `"safe":"ok"`) {
		t.Fatalf("structured secrets were not redacted: %s", redacted)
	}
	text := redactSecrets("Authorization: Bearer live-token\npassword=hunter2\nsafe=ok")
	if strings.Contains(text, "live-token") || strings.Contains(text, "hunter2") || !strings.Contains(text, "safe=ok") {
		t.Fatalf("text secrets were not redacted: %s", text)
	}
	audit := string(redactAuditJSON(json.RawMessage(`{"type":"tool_execution_start","args":{"pat":"github-pat","safe":"ok"},"result":"Authorization: Bearer event-token"}`)))
	if strings.Contains(audit, "github-pat") || strings.Contains(audit, "event-token") ||
		!strings.Contains(audit, `"safe":"ok"`) {
		t.Fatalf("raw audit event was not redacted: %s", audit)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runs", "run-utf8"), 0o700); err != nil {
		t.Fatal(err)
	}
	ref, err := writeToolAuxFile(
		root, "run-utf8", "tool-utf8", "output.txt", []byte("1234界"), 5,
	)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
	if err != nil || string(written) != "1234" || !utf8.Valid(written) {
		t.Fatalf("UTF-8 output truncation is invalid: %q, %v", written, err)
	}
}

func newPermissionTestService(
	t *testing.T,
	taskID string,
	permissionTimeout time.Duration,
) (*Service, *storage.SQLiteStore, *fakeRuntimeFactory) {
	t.Helper()
	store := newServiceTestStore(t, taskID, "权限任务")
	factory := &fakeRuntimeFactory{}
	service := NewService(store, ServiceOptions{
		RuntimeFactory: factory, RequestTimeout: time.Second,
		PermissionTimeout: permissionTimeout,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})
	return service, store, factory
}

func startPermissionProbe(
	t *testing.T,
	service *Service,
	store *storage.SQLiteStore,
	factory *fakeRuntimeFactory,
	taskID string,
) (storage.AgentSessionRecord, storage.ExecutionRunRecord, *fakeRuntime, PermissionRequest) {
	t.Helper()
	session, err := service.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: taskID, Mode: "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.SendPrompt(context.Background(), PromptRequest{
		TaskID: taskID, SessionID: session.ID, Message: "hold",
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := factory.runtime(0)
	args := json.RawMessage(`{"target":"rpc-confirm"}`)
	runtime.emit(map[string]any{
		"type": "tool_execution_start", "toolCallId": "probe-call",
		"toolName": "btask_permission_probe", "args": map[string]any{"target": "rpc-confirm"},
	})
	classification := permissionpolicy.ClassifyTool(permissionpolicy.ToolInput{
		TaskID: taskID, ToolName: "btask_permission_probe", Args: args,
	})
	options := factory.processOptions(0)
	envelope, _ := json.Marshal(permissionEnvelope{
		Protocol: permissionProtocolVersion, Version: options.Gate.Version, Nonce: options.Gate.Nonce,
		TaskID: taskID, SessionID: session.ID, RunID: run.ID, Mode: "agent",
		ToolCallID: "probe-call", ToolName: "btask_permission_probe",
		Capability: classification.Capability, Subject: classification.Subject,
		Target: classification.Target, NormalizedTarget: classification.NormalizedTarget,
		RiskLevel: string(classification.RiskLevel), ArgsDigest: classification.ArgsDigest,
	})
	runtime.emit(map[string]any{
		"type": "extension_ui_request", "id": "probe-confirm", "method": "confirm",
		"title": "btask-permission", "message": string(envelope),
	})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		requests, requestErr := service.PermissionRequests(taskID, session.ID)
		if requestErr == nil && len(requests) == 1 && requests[0].State == "pending" {
			if len(requests[0].AllowedScopes) != 1 || requests[0].AllowedScopes[0] != "once" {
				t.Fatalf("high-risk probe exposed broad scopes: %#v", requests[0])
			}
			return session, run, runtime, requests[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	storedRun, _ := store.ExecutionRun(taskID, run.ID)
	t.Fatalf("permission request did not become pending; run %#v", storedRun)
	return storage.AgentSessionRecord{}, storage.ExecutionRunRecord{}, nil, PermissionRequest{}
}

func emitProbeBridge(
	t *testing.T,
	runtime *fakeRuntime,
	options ProcessOptions,
	sessionID string,
	runID string,
) {
	t.Helper()
	request, _ := json.Marshal(gateBridgeRequest{
		Version: options.Gate.Version, Nonce: options.Gate.Nonce,
		TaskID: "task_permission_allow", SessionID: sessionID, RunID: runID, Mode: "agent",
		ToolCallID: "probe-call", Operation: "permission_probe",
		Args: json.RawMessage(`{"target":"rpc-confirm"}`),
	})
	runtime.emit(map[string]any{
		"type": "extension_ui_request", "id": "probe-bridge", "method": "input",
		"title": "btask-gate", "placeholder": string(request),
	})
}

func waitForSessionState(
	t *testing.T,
	store *storage.SQLiteStore,
	taskID string,
	sessionID string,
	want string,
) storage.AgentSessionRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session, err := store.AgentSession(taskID, sessionID)
		if err == nil && session.State == want {
			return session
		}
		time.Sleep(5 * time.Millisecond)
	}
	session, err := store.AgentSession(taskID, sessionID)
	t.Fatalf("session did not reach %s: %#v, %v", want, session, err)
	return storage.AgentSessionRecord{}
}

func TestServiceRestartReadsHistoryAndResumesRegisteredSession(t *testing.T) {
	store := newServiceTestStore(t, "task_resume", "恢复任务")
	firstFactory := &fakeRuntimeFactory{}
	first := NewService(store, ServiceOptions{RuntimeFactory: firstFactory, RequestTimeout: time.Second})
	session, err := first.CreateSession(context.Background(), CreateSessionRequest{TaskID: "task_resume"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := first.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_resume", SessionID: session.ID, Message: "恢复前",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunState(t, store, "task_resume", run.ID, "succeeded")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()

	secondFactory := &fakeRuntimeFactory{}
	second := NewService(store, ServiceOptions{RuntimeFactory: secondFactory, RequestTimeout: time.Second})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = second.Close(ctx)
	})
	history, err := second.Messages("task_resume", session.ID)
	if err != nil || len(history) != 2 {
		t.Fatalf("restart history %#v, %v", history, err)
	}
	resumedRun, err := second.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_resume", SessionID: session.ID, Message: "恢复后",
	})
	if err != nil {
		t.Fatalf("resume session: %v", err)
	}
	waitForRunState(t, store, "task_resume", resumedRun.ID, "succeeded")
	if !secondFactory.runtime(0).called("switch_session") {
		t.Fatal("registered PI session was not resumed with switch_session")
	}
}

func TestServiceRestartsUnusedLazyPISessionWithoutFalseRecoveryFailure(t *testing.T) {
	store := newServiceTestStore(t, "task_lazy", "懒写会话")
	firstFactory := &lazyFileRuntimeFactory{}
	first := NewService(store, ServiceOptions{
		RuntimeFactory: firstFactory, RequestTimeout: time.Second,
	})
	session, err := first.CreateSession(context.Background(), CreateSessionRequest{
		TaskID: "task_lazy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.ExternalSessionPath == nil {
		t.Fatal("expected PI to report its lazy session path")
	}
	if _, err := os.Stat(*session.ExternalSessionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lazy PI session unexpectedly existed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()

	secondFactory := &lazyFileRuntimeFactory{}
	second := NewService(store, ServiceOptions{
		RuntimeFactory: secondFactory, RequestTimeout: time.Second,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = second.Close(ctx)
	})
	run, err := second.SendPrompt(context.Background(), PromptRequest{
		TaskID: "task_lazy", SessionID: session.ID, Message: "首次回答",
	})
	if err != nil {
		t.Fatalf("start unused lazy session after restart: %v", err)
	}
	waitForRunState(t, store, "task_lazy", run.ID, "succeeded")
	if secondFactory.runtime(0).called("switch_session") {
		t.Fatal("unused missing session file was passed to switch_session")
	}
	updated, err := store.AgentSession("task_lazy", session.ID)
	if err != nil || updated.ExternalSessionPath == nil {
		t.Fatalf("new PI session identity was not recorded: %#v, %v", updated, err)
	}
	if _, err := os.Stat(*updated.ExternalSessionPath); err != nil {
		t.Fatalf("first assistant response did not persist PI session: %v", err)
	}
}

func newServiceTestStore(t *testing.T, taskID string, title string) *storage.SQLiteStore {
	t.Helper()
	store := storage.NewSQLiteStoreAt(filepath.Join(t.TempDir(), "service.db"))
	t.Cleanup(func() { _ = store.Close() })
	root := filepath.Join(t.TempDir(), "task-data")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if _, err := store.SetTaskContextRoot(root); err != nil {
		t.Fatalf("set task root: %v", err)
	}
	seedServiceTask(t, store, taskID, title)
	return store
}

func seedServiceTask(t *testing.T, store *storage.SQLiteStore, taskID string, title string) {
	t.Helper()
	payload, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	tasks := []map[string]any{}
	if payload != "" {
		var existing struct {
			State struct {
				Tasks []map[string]any `json:"tasks"`
			} `json:"state"`
		}
		if err := json.Unmarshal([]byte(payload), &existing); err != nil {
			t.Fatal(err)
		}
		tasks = existing.State.Tasks
	}
	tasks = append(tasks, map[string]any{
		"id": taskID, "title": title, "summary": "测试", "revision": 1,
		"createdAt": "2026-08-01T08:00:00Z", "updatedAt": "2026-08-01T08:00:00Z",
	})
	encoded, _ := json.Marshal(map[string]any{"state": map[string]any{"tasks": tasks}})
	if err := store.Save(string(encoded)); err != nil {
		t.Fatalf("save task: %v", err)
	}
}

func waitForRunState(
	t *testing.T,
	store *storage.SQLiteStore,
	taskID string,
	runID string,
	want string,
) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := store.ExecutionRun(taskID, runID)
		if err == nil && run.State == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	run, err := store.ExecutionRun(taskID, runID)
	t.Fatalf("run did not reach %s: %#v, %v", want, run, err)
}

type fakeRuntimeFactory struct {
	mutex    sync.Mutex
	runtimes []*fakeRuntime
	options  []ProcessOptions
}

type lazyFileRuntimeFactory struct {
	fakeRuntimeFactory
}

func (factory *lazyFileRuntimeFactory) Start(
	_ context.Context,
	options ProcessOptions,
) (Runtime, SessionState, error) {
	if err := os.MkdirAll(options.SessionDir, 0o700); err != nil {
		return nil, SessionState{}, err
	}
	path := filepath.Join(options.SessionDir, newID("pi-session")+".jsonl")
	runtime := newFakeRuntime(path)
	runtime.persistOnPrompt = true
	factory.mutex.Lock()
	factory.runtimes = append(factory.runtimes, runtime)
	factory.options = append(factory.options, options)
	factory.mutex.Unlock()
	return runtime, SessionState{SessionID: newID("pi"), SessionFile: path}, nil
}

func (factory *fakeRuntimeFactory) Start(
	_ context.Context,
	options ProcessOptions,
) (Runtime, SessionState, error) {
	if err := os.MkdirAll(options.SessionDir, 0o700); err != nil {
		return nil, SessionState{}, err
	}
	path := filepath.Join(options.SessionDir, newID("pi-session")+".jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		return nil, SessionState{}, err
	}
	runtime := newFakeRuntime(path)
	factory.mutex.Lock()
	factory.runtimes = append(factory.runtimes, runtime)
	factory.options = append(factory.options, options)
	factory.mutex.Unlock()
	return runtime, SessionState{SessionID: newID("pi"), SessionFile: path}, nil
}

func (factory *fakeRuntimeFactory) count() int {
	factory.mutex.Lock()
	defer factory.mutex.Unlock()
	return len(factory.runtimes)
}

func (factory *fakeRuntimeFactory) runtime(index int) *fakeRuntime {
	factory.mutex.Lock()
	defer factory.mutex.Unlock()
	return factory.runtimes[index]
}

func (factory *fakeRuntimeFactory) processOptions(index int) ProcessOptions {
	factory.mutex.Lock()
	defer factory.mutex.Unlock()
	return factory.options[index]
}

type fakeRuntime struct {
	mutex           sync.Mutex
	calls           []string
	callFields      []map[string]any
	notifications   []map[string]any
	events          chan rawEvent
	done            chan struct{}
	exit            ProcessExit
	closeOnce       sync.Once
	session         string
	persistOnPrompt bool
}

func newFakeRuntime(session string) *fakeRuntime {
	return &fakeRuntime{
		events: make(chan rawEvent, 64), done: make(chan struct{}), session: session,
	}
}

func (runtime *fakeRuntime) Call(
	_ context.Context,
	command string,
	fields map[string]any,
	result any,
) error {
	runtime.mutex.Lock()
	runtime.calls = append(runtime.calls, command)
	runtime.callFields = append(runtime.callFields, fields)
	runtime.mutex.Unlock()
	switch command {
	case "get_entries":
		return assignFakeResult(result, map[string]any{"entries": []any{}, "leafId": nil})
	case "get_state":
		return assignFakeResult(result, SessionState{SessionID: "fake", SessionFile: runtime.session})
	case "prompt":
		if runtime.persistOnPrompt {
			if err := os.WriteFile(runtime.session, []byte("{}\n"), 0o600); err != nil {
				return err
			}
		}
		message, _ := fields["message"].(string)
		runtime.emit(map[string]any{"type": "agent_start"})
		if message == "hold" {
			return nil
		}
		answer := "回答：" + message
		runtime.emit(map[string]any{
			"type":                  "message_update",
			"assistantMessageEvent": map[string]any{"type": "text_delta", "delta": answer},
		})
		runtime.emit(map[string]any{
			"type": "message_end",
			"message": map[string]any{
				"role": "assistant", "content": []map[string]any{{"type": "text", "text": answer}},
			},
		})
		runtime.emit(map[string]any{"type": "agent_settled"})
	case "abort":
		runtime.emit(map[string]any{"type": "agent_settled"})
	}
	return nil
}

func (runtime *fakeRuntime) Send(_ context.Context, fields map[string]any) error {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	copy := make(map[string]any, len(fields))
	for key, value := range fields {
		copy[key] = value
	}
	runtime.notifications = append(runtime.notifications, copy)
	return nil
}

func (runtime *fakeRuntime) Events() <-chan rawEvent { return runtime.events }
func (runtime *fakeRuntime) Done() <-chan struct{}   { return runtime.done }
func (runtime *fakeRuntime) StderrTail() string      { return runtime.Exit().StderrTail }
func (runtime *fakeRuntime) Exit() ProcessExit {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	return runtime.exit
}

func (runtime *fakeRuntime) Close(context.Context) error {
	runtime.closeOnce.Do(func() {
		close(runtime.events)
		close(runtime.done)
	})
	return nil
}

func (runtime *fakeRuntime) crash(err error, stderr string) {
	runtime.mutex.Lock()
	runtime.exit = ProcessExit{Code: 2, Err: err, StderrTail: stderr}
	runtime.mutex.Unlock()
	runtime.closeOnce.Do(func() {
		close(runtime.events)
		close(runtime.done)
	})
}

func (runtime *fakeRuntime) called(command string) bool {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	for _, call := range runtime.calls {
		if call == command {
			return true
		}
	}
	return false
}

func (runtime *fakeRuntime) lastCallFields(command string) map[string]any {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	for index := len(runtime.calls) - 1; index >= 0; index-- {
		if runtime.calls[index] == command {
			return runtime.callFields[index]
		}
	}
	return nil
}

func (runtime *fakeRuntime) waitNotification(t *testing.T) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.mutex.Lock()
		if len(runtime.notifications) > 0 {
			value := runtime.notifications[len(runtime.notifications)-1]
			runtime.mutex.Unlock()
			return value
		}
		runtime.mutex.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for PI extension bridge response")
	return nil
}

func (runtime *fakeRuntime) waitNotificationID(t *testing.T, requestID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.mutex.Lock()
		for _, value := range runtime.notifications {
			if value["id"] == requestID {
				runtime.mutex.Unlock()
				return value
			}
		}
		runtime.mutex.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PI extension response %s", requestID)
	return nil
}

func (runtime *fakeRuntime) emit(value any) {
	encoded, _ := json.Marshal(value)
	var envelope struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(encoded, &envelope)
	runtime.events <- rawEvent{Type: envelope.Type, JSON: encoded}
}

func assignFakeResult(target any, value any) error {
	if target == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return json.Unmarshal(encoded, target)
}
