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
	runtime.emit(map[string]any{
		"type": "tool_execution_start", "toolCallId": "artifact-call",
		"toolName": "btask_write_artifact",
		"args":     map[string]any{"kind": "proposal", "name": "scope.md"},
	})
	options := factory.processOptions(0)
	request := gateBridgeRequest{
		Version: options.Gate.Version, Nonce: options.Gate.Nonce,
		TaskID: "task_gate_artifact", SessionID: session.ID, Mode: "plan",
		ToolCallID: "artifact-call", Operation: "write_artifact",
		Args: json.RawMessage(`{"kind":"proposal","name":"scope.md","content":"# 范围修改建议\n"}`),
	}
	placeholder, _ := json.Marshal(request)
	runtime.emit(map[string]any{
		"type": "extension_ui_request", "id": "gate-request-1",
		"method": "input", "title": "btask-gate", "placeholder": string(placeholder),
	})
	response := runtime.waitNotification(t)
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
