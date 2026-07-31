package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

const (
	repositoryTestTime   = "2026-08-01T08:00:00Z"
	repositoryTestSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestAgentRepositoriesPersistUpdatesAndEnforceTaskScope(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_agent_a")
	seedNormalizedTask(t, store, "task_agent_b")

	sessionA := repositorySession("task_agent_a", "session-a")
	sessionB := repositorySession("task_agent_b", "session-b")
	for _, session := range []AgentSessionRecord{sessionA, sessionB} {
		if err := store.UpsertAgentSession(session); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	sessionA.State = "running"
	sessionA.LastSequence = 4
	if err := store.UpsertAgentSession(sessionA); err != nil {
		t.Fatalf("update session: %v", err)
	}
	gotSession, err := store.AgentSession("task_agent_a", "session-a")
	if err != nil || gotSession.State != "running" || gotSession.LastSequence != 4 {
		t.Fatalf("unexpected session %#v, error %v", gotSession, err)
	}
	if _, err := store.AgentSession("task_agent_b", "session-a"); !errors.Is(err, ErrAgentDataNotFound) {
		t.Fatalf("cross-task session read was not rejected: %v", err)
	}
	if sessions, err := store.AgentSessions("task_agent_a"); err != nil || len(sessions) != 1 {
		t.Fatalf("unexpected session list %#v, error %v", sessions, err)
	}

	runA := repositoryRun("task_agent_a", "session-a", "run-a")
	if err := store.UpsertExecutionRun(runA); err != nil {
		t.Fatalf("create run: %v", err)
	}
	crossRun := repositoryRun("task_agent_b", "session-a", "run-cross")
	if err := store.UpsertExecutionRun(crossRun); err == nil {
		t.Fatal("cross-task execution run was accepted")
	}
	secondActive := repositoryRun("task_agent_a", "session-a", "run-active-2")
	if err := store.UpsertExecutionRun(secondActive); err == nil {
		t.Fatal("second active run for one task was accepted")
	}
	runA.State = "succeeded"
	runA.FinishedAt = stringPointer("2026-08-01T08:01:00Z")
	if err := store.UpsertExecutionRun(runA); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	gotRun, err := store.ExecutionRun("task_agent_a", "run-a")
	if err != nil || gotRun.State != "succeeded" || gotRun.FinishedAt == nil {
		t.Fatalf("unexpected run %#v, error %v", gotRun, err)
	}
	if runs, err := store.ExecutionRuns("task_agent_a", "session-a"); err != nil || len(runs) != 1 {
		t.Fatalf("unexpected run list %#v, error %v", runs, err)
	}

	content := "hello"
	message := AgentMessageRecord{
		ID:        "message-a",
		TaskID:    "task_agent_a",
		SessionID: "session-a",
		RunID:     stringPointer("run-a"),
		Role:      "assistant",
		Kind:      "text",
		Status:    "streaming",
		Content:   &content,
		Sequence:  1,
		CreatedAt: repositoryTestTime,
	}
	if err := store.UpsertAgentMessage(message); err != nil {
		t.Fatalf("create message: %v", err)
	}
	updatedContent := "hello world"
	message.Status = "complete"
	message.Content = &updatedContent
	message.CompletedAt = stringPointer("2026-08-01T08:01:00Z")
	if err := store.UpsertAgentMessage(message); err != nil {
		t.Fatalf("update message: %v", err)
	}
	messages, err := store.AgentMessages("task_agent_a", "session-a")
	if err != nil || len(messages) != 1 || messages[0].Content == nil || *messages[0].Content != updatedContent {
		t.Fatalf("unexpected message list %#v, error %v", messages, err)
	}
	crossMessage := message
	crossMessage.TaskID = "task_agent_b"
	crossMessage.SessionID = "session-b"
	crossMessage.RunID = nil
	if err := store.UpsertAgentMessage(crossMessage); err == nil {
		t.Fatal("cross-task message id reuse was accepted")
	}

	event := AgentEventRecord{
		EventID:     "event-a",
		Version:     1,
		TaskID:      "task_agent_a",
		SessionID:   "session-a",
		RunID:       stringPointer("run-a"),
		Sequence:    1,
		Kind:        "message.completed",
		PayloadJSON: `{}`,
		OccurredAt:  repositoryTestTime,
	}
	if err := store.AppendAgentEvent(event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := store.AppendAgentEvent(event); err == nil {
		t.Fatal("duplicate append-only event was accepted")
	}
	events, err := store.AgentEvents("task_agent_a", "session-a")
	if err != nil || len(events) != 1 || events[0].EventID != "event-a" {
		t.Fatalf("unexpected event list %#v, error %v", events, err)
	}

	toolCall := repositoryToolCall("task_agent_a", "session-a", "run-a", "tool-a")
	if err := store.UpsertToolCall(toolCall); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	toolCall.State = "succeeded"
	toolCall.OutputSummary = stringPointer("done")
	if err := store.UpsertToolCall(toolCall); err != nil {
		t.Fatalf("update tool call: %v", err)
	}
	gotToolCall, err := store.ToolCall("task_agent_a", "tool-a")
	if err != nil || gotToolCall.State != "succeeded" {
		t.Fatalf("unexpected tool call %#v, error %v", gotToolCall, err)
	}
	crossToolCall := toolCall
	crossToolCall.TaskID = "task_agent_b"
	crossToolCall.SessionID = "session-b"
	crossToolCall.RunID = "run-b"
	if err := store.UpsertToolCall(crossToolCall); err == nil {
		t.Fatal("cross-task tool-call id reuse was accepted")
	}
}

func TestGovernanceRepositoriesPersistLifecycleAndEnforceScope(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_guard_a")
	seedNormalizedTask(t, store, "task_guard_b")
	for _, session := range []AgentSessionRecord{
		repositorySession("task_guard_a", "guard-session-a"),
		repositorySession("task_guard_b", "guard-session-b"),
	} {
		if err := store.UpsertAgentSession(session); err != nil {
			t.Fatal(err)
		}
	}
	for _, run := range []ExecutionRunRecord{
		repositoryRun("task_guard_a", "guard-session-a", "guard-run-a"),
		repositoryRun("task_guard_b", "guard-session-b", "guard-run-b"),
	} {
		run.State = "succeeded"
		if err := store.UpsertExecutionRun(run); err != nil {
			t.Fatal(err)
		}
	}
	for _, toolCall := range []ToolCallRecord{
		repositoryToolCall("task_guard_a", "guard-session-a", "guard-run-a", "guard-tool-a"),
		repositoryToolCall("task_guard_b", "guard-session-b", "guard-run-b", "guard-tool-b"),
	} {
		if err := store.UpsertToolCall(toolCall); err != nil {
			t.Fatal(err)
		}
	}
	crossToolEvent := AgentEventRecord{
		EventID:     "guard-cross-tool-event",
		Version:     1,
		TaskID:      "task_guard_a",
		SessionID:   "guard-session-a",
		RunID:       stringPointer("guard-run-a"),
		ToolCallID:  stringPointer("guard-tool-b"),
		Sequence:    1,
		Kind:        "tool.started",
		PayloadJSON: `{}`,
		OccurredAt:  repositoryTestTime,
	}
	if err := store.AppendAgentEvent(crossToolEvent); err == nil {
		t.Fatal("cross-task tool event was accepted")
	}

	binding := GitBindingRecord{
		ID:                "binding-a",
		TaskID:            "task_guard_a",
		SourcePath:        "/source/a",
		SourceRealPath:    "/source/a",
		CommonGitDir:      "/source/a/.git",
		BaselineCommit:    "0123456789abcdef",
		SourceDirtyAtBind: true,
		State:             "bound",
		CreatedAt:         repositoryTestTime,
		UpdatedAt:         repositoryTestTime,
	}
	if err := store.UpsertGitBinding(binding); err != nil {
		t.Fatalf("create git binding: %v", err)
	}
	if got, err := store.GitBinding("task_guard_a"); err != nil || got.ID != binding.ID {
		t.Fatalf("unexpected git binding %#v, error %v", got, err)
	}
	if _, err := store.GitBinding("task_guard_b"); !errors.Is(err, ErrAgentDataNotFound) {
		t.Fatalf("cross-task git binding read was not rejected: %v", err)
	}
	crossBinding := binding
	crossBinding.TaskID = "task_guard_b"
	if err := store.UpsertGitBinding(crossBinding); err == nil {
		t.Fatal("cross-task git binding id reuse was accepted")
	}

	request := PermissionRequestRecord{
		ID:          "request-a",
		TaskID:      "task_guard_a",
		SessionID:   "guard-session-a",
		RunID:       "guard-run-a",
		ToolCallID:  "guard-tool-a",
		Capability:  "filesystem.write",
		Target:      "artifacts/report.md",
		Subject:     "write report",
		RiskLevel:   "medium",
		State:       "pending",
		RequestedAt: repositoryTestTime,
	}
	if err := store.UpsertPermissionRequest(request); err != nil {
		t.Fatalf("create permission request: %v", err)
	}
	if _, err := store.PermissionRequest("task_guard_b", request.ID); !errors.Is(err, ErrAgentDataNotFound) {
		t.Fatalf("cross-task permission read was not rejected: %v", err)
	}
	crossRequest := request
	crossRequest.TaskID = "task_guard_b"
	crossRequest.SessionID = "guard-session-b"
	crossRequest.RunID = "guard-run-b"
	crossRequest.ToolCallID = "guard-tool-b"
	if err := store.UpsertPermissionRequest(crossRequest); err == nil {
		t.Fatal("cross-task permission request id reuse was accepted")
	}

	taskID := "task_guard_a"
	sessionID := "guard-session-a"
	requestID := "request-a"
	grant := PermissionGrantRecord{
		ID:            "grant-once-a",
		TaskID:        &taskID,
		SessionID:     &sessionID,
		RequestID:     &requestID,
		Capability:    "filesystem.write",
		TargetPattern: "artifacts/report.md",
		Scope:         "once",
		Decision:      "allow",
		RiskCeiling:   "medium",
		CreatedAt:     repositoryTestTime,
		CreatedBy:     "user",
	}
	if err := store.UpsertPermissionGrant(grant); err != nil {
		t.Fatalf("create once grant: %v", err)
	}
	if got, err := store.PermissionGrant(&taskID, grant.ID); err != nil || got.Scope != "once" {
		t.Fatalf("unexpected grant %#v, error %v", got, err)
	}
	otherTaskID := "task_guard_b"
	if _, err := store.PermissionGrant(&otherTaskID, grant.ID); !errors.Is(err, ErrAgentDataNotFound) {
		t.Fatalf("cross-task permission grant read was not rejected: %v", err)
	}
	invalidOnce := grant
	invalidOnce.ID = "grant-invalid"
	invalidOnce.RequestID = nil
	if err := store.UpsertPermissionGrant(invalidOnce); err == nil {
		t.Fatal("once grant without request was accepted")
	}
	permanent := PermissionGrantRecord{
		ID:            "grant-permanent",
		Capability:    "filesystem.read",
		TargetPattern: "*",
		Scope:         "permanent",
		Decision:      "deny",
		RiskCeiling:   "high",
		CreatedAt:     repositoryTestTime,
		CreatedBy:     "user",
	}
	if err := store.UpsertPermissionGrant(permanent); err != nil {
		t.Fatalf("create permanent grant: %v", err)
	}
	if _, err := store.PermissionGrant(nil, permanent.ID); err != nil {
		t.Fatalf("read permanent grant: %v", err)
	}

	request.State = "expired"
	request.ResolvedAt = stringPointer("2026-08-01T08:02:00Z")
	request.ResolvedBy = stringPointer("system")
	request.Reason = stringPointer("application restarted before the request was resolved")
	if err := store.UpsertPermissionRequest(request); err != nil {
		t.Fatalf("update permission request: %v", err)
	}
	gotRequest, err := store.PermissionRequest("task_guard_a", request.ID)
	if err != nil || gotRequest.State != "expired" || gotRequest.ResolvedBy == nil || *gotRequest.ResolvedBy != "system" {
		t.Fatalf("unexpected expired request %#v, error %v", gotRequest, err)
	}

	artifact := WorkspaceArtifactRecord{
		ID:          "artifact-a",
		TaskID:      "task_guard_a",
		SessionID:   &sessionID,
		RunID:       stringPointer("guard-run-a"),
		LogicalPath: "artifacts/reports/report.md",
		Kind:        "report",
		ByteSize:    12,
		SHA256:      repositoryTestSHA256,
		CreatedAt:   repositoryTestTime,
		UpdatedAt:   repositoryTestTime,
	}
	if err := store.UpsertWorkspaceArtifact(artifact); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	artifacts, err := store.WorkspaceArtifacts("task_guard_a")
	if err != nil || len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("unexpected artifacts %#v, error %v", artifacts, err)
	}
	crossArtifact := artifact
	crossArtifact.TaskID = "task_guard_b"
	crossArtifact.SessionID = stringPointer("guard-session-b")
	crossArtifact.RunID = stringPointer("guard-run-b")
	if err := store.UpsertWorkspaceArtifact(crossArtifact); err == nil {
		t.Fatal("cross-task artifact id reuse was accepted")
	}
	artifact.DeletedAt = stringPointer("2026-08-01T08:03:00Z")
	if err := store.UpsertWorkspaceArtifact(artifact); err != nil {
		t.Fatalf("soft delete artifact: %v", err)
	}
	if artifacts, err := store.WorkspaceArtifacts("task_guard_a"); err != nil || len(artifacts) != 0 {
		t.Fatalf("soft-deleted artifact remained active %#v, error %v", artifacts, err)
	}
}

func TestNormalizedRepositoriesRejectUnsafeStoredPaths(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_paths_repo")
	if err := store.ReplaceTaskResources("task_paths_repo", []TaskResourceRecord{{
		ID:          "unsafe-resource",
		TaskID:      "task_paths_repo",
		Kind:        "context",
		SourceType:  "task",
		LogicalPath: "../outside",
		CreatedAt:   repositoryTestTime,
	}}); err == nil {
		t.Fatal("unsafe resource path was accepted")
	}

	if err := store.UpsertAgentSession(repositorySession("task_paths_repo", "paths-session")); err != nil {
		t.Fatal(err)
	}
	run := repositoryRun("task_paths_repo", "paths-session", "paths-run")
	run.EventsPath = "../events.jsonl"
	if err := store.UpsertExecutionRun(run); err == nil {
		t.Fatal("unsafe run path was accepted")
	}

	if err := store.UpsertGitBinding(GitBindingRecord{
		ID:             "paths-binding",
		TaskID:         "task_paths_repo",
		SourcePath:     "/source/../source",
		SourceRealPath: "/source",
		CommonGitDir:   "/source/.git",
		BaselineCommit: "0123456789abcdef",
		State:          "bound",
		CreatedAt:      repositoryTestTime,
		UpdatedAt:      repositoryTestTime,
	}); err == nil {
		t.Fatal("non-canonical git path was accepted")
	}

	if err := store.UpsertWorkspaceArtifact(WorkspaceArtifactRecord{
		ID:          "paths-artifact",
		TaskID:      "task_paths_repo",
		LogicalPath: "artifacts/../context/task.md",
		Kind:        "report",
		SHA256:      repositoryTestSHA256,
		CreatedAt:   repositoryTestTime,
		UpdatedAt:   repositoryTestTime,
	}); err == nil {
		t.Fatal("unsafe artifact path was accepted")
	}
}

func newNormalizedRepositoryStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store := NewSQLiteStoreAt(filepath.Join(t.TempDir(), "normalized.db"))
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedNormalizedTask(t *testing.T, store *SQLiteStore, taskID string) {
	t.Helper()
	if err := store.UpsertTaskWorkspace(TaskWorkspaceRecord{
		TaskID:           taskID,
		WorkspaceID:      "workspace-" + taskID,
		RootPath:         "/tasks/" + taskID,
		SchemaVersion:    1,
		ManifestRevision: 1,
		State:            "ready",
		CreatedAt:        repositoryTestTime,
		UpdatedAt:        repositoryTestTime,
	}); err != nil {
		t.Fatalf("seed task workspace: %v", err)
	}
}

func repositorySession(taskID string, sessionID string) AgentSessionRecord {
	return AgentSessionRecord{
		ID:             sessionID,
		TaskID:         taskID,
		Engine:         "pi",
		Title:          sessionID,
		Mode:           "ask",
		ResourcePolicy: "isolated",
		State:          "idle",
		CreatedAt:      repositoryTestTime,
		UpdatedAt:      repositoryTestTime,
		LastActiveAt:   repositoryTestTime,
	}
}

func repositoryRun(taskID string, sessionID string, runID string) ExecutionRunRecord {
	return ExecutionRunRecord{
		ID:         runID,
		TaskID:     taskID,
		SessionID:  sessionID,
		Mode:       "ask",
		State:      "queued",
		EventsPath: "runs/" + runID + "/events.jsonl",
		StdoutPath: "runs/" + runID + "/stdout.log",
		StderrPath: "runs/" + runID + "/stderr.log",
		ResultPath: "runs/" + runID + "/result.json",
		StartedAt:  repositoryTestTime,
	}
}

func repositoryToolCall(
	taskID string,
	sessionID string,
	runID string,
	toolCallID string,
) ToolCallRecord {
	return ToolCallRecord{
		ID:                 toolCallID,
		TaskID:             taskID,
		SessionID:          sessionID,
		RunID:              runID,
		ExternalToolCallID: "external-" + toolCallID,
		ToolName:           "read",
		Capability:         "filesystem.read",
		RiskLevel:          "low",
		State:              "received",
	}
}
