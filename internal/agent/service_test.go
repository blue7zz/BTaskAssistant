package agent

import (
	"context"
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

func TestServiceStopsUnexpectedToolIntentInNoToolsSession(t *testing.T) {
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
		"type": "message_update",
		"assistantMessageEvent": map[string]any{
			"type": "toolcall_start",
		},
	})
	waitForRunState(t, store, "task_no_tools", run.ID, "failed")
	stored, err := store.ExecutionRun("task_no_tools", run.ID)
	if err != nil || stored.ErrorMessage == nil ||
		!strings.Contains(*stored.ErrorMessage, "无工具") {
		t.Fatalf("unexpected no-tools failure %#v, %v", stored, err)
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

type fakeRuntime struct {
	mutex           sync.Mutex
	calls           []string
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
