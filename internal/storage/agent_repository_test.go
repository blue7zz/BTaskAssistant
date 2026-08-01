package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAgentMessagePageReadsOnlyTheRequestedHistoryWindow(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_history")
	session := repositorySession("task_history", "session-history")
	if err := store.UpsertAgentSession(session); err != nil {
		t.Fatal(err)
	}
	for sequence := int64(1); sequence <= 205; sequence++ {
		content := fmt.Sprintf("message-%03d", sequence)
		if err := store.UpsertAgentMessage(AgentMessageRecord{
			ID: fmt.Sprintf("message-%03d", sequence), TaskID: session.TaskID,
			SessionID: session.ID, Role: "assistant", Kind: "text",
			Status: "complete", Content: &content, Sequence: sequence,
			CreatedAt: repositoryTestTime,
		}); err != nil {
			t.Fatalf("insert message %d: %v", sequence, err)
		}
	}

	latest, hasMore, err := store.AgentMessagePage(session.TaskID, session.ID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMore || len(latest) != 50 || latest[0].Sequence != 156 || latest[49].Sequence != 205 {
		t.Fatalf("unexpected latest page: hasMore=%v first=%d last=%d count=%d", hasMore, latest[0].Sequence, latest[len(latest)-1].Sequence, len(latest))
	}
	older, olderHasMore, err := store.AgentMessagePage(session.TaskID, session.ID, latest[0].Sequence, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !olderHasMore || len(older) != 50 || older[0].Sequence != 106 || older[49].Sequence != 155 {
		t.Fatalf("unexpected older page: hasMore=%v first=%d last=%d count=%d", olderHasMore, older[0].Sequence, older[len(older)-1].Sequence, len(older))
	}
	if _, _, err := store.AgentMessagePage(session.TaskID, session.ID, -1, 50); err == nil {
		t.Fatal("negative history cursor was accepted")
	}
	if _, _, err := store.AgentMessagePage(session.TaskID, session.ID, 0, 201); err == nil {
		t.Fatal("oversized history page was accepted")
	}
}

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

func TestInterruptActiveAgentActivityPreservesHistoryAndMarksActiveRows(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_interrupted")
	session := repositorySession("task_interrupted", "session-interrupted")
	session.State = "running"
	if err := store.UpsertAgentSession(session); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	run := repositoryRun("task_interrupted", session.ID, "run-interrupted")
	run.State = "running"
	if err := store.UpsertExecutionRun(run); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	content := "partial"
	message := AgentMessageRecord{
		ID: "message-interrupted", TaskID: "task_interrupted", SessionID: session.ID,
		RunID: &run.ID, Role: "assistant", Kind: "text", Status: "streaming",
		Content: &content, Sequence: 1, CreatedAt: repositoryTestTime,
	}
	if err := store.UpsertAgentMessage(message); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	interruptedAt := "2026-08-01T08:05:00Z"
	if err := store.InterruptActiveAgentActivity(interruptedAt, "应用已重启"); err != nil {
		t.Fatalf("interrupt activity: %v", err)
	}
	storedSession, err := store.AgentSession("task_interrupted", session.ID)
	if err != nil || storedSession.State != "interrupted" || storedSession.ErrorMessage == nil {
		t.Fatalf("unexpected interrupted session %#v, %v", storedSession, err)
	}
	storedRun, err := store.ExecutionRun("task_interrupted", run.ID)
	if err != nil || storedRun.State != "interrupted" || storedRun.FinishedAt == nil {
		t.Fatalf("unexpected interrupted run %#v, %v", storedRun, err)
	}
	messages, err := store.AgentMessages("task_interrupted", session.ID)
	if err != nil || len(messages) != 1 || messages[0].Status != "error" || messages[0].Content == nil {
		t.Fatalf("unexpected interrupted messages %#v, %v", messages, err)
	}
	if err := store.InterruptActiveAgentActivity(interruptedAt, "应用已重启"); err != nil {
		t.Fatalf("repeat interruption must be idempotent: %v", err)
	}
}

func TestPermissionResolutionOnceConsumptionRevokeAndIsolation(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	for _, taskID := range []string{"task_permission_a", "task_permission_b"} {
		seedNormalizedTask(t, store, taskID)
		sessionID := "session-" + taskID
		if err := store.UpsertAgentSession(repositorySession(taskID, sessionID)); err != nil {
			t.Fatal(err)
		}
		run := repositoryRun(taskID, sessionID, "run-"+taskID)
		run.State = "succeeded"
		if err := store.UpsertExecutionRun(run); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertToolCall(repositoryToolCall(taskID, sessionID, run.ID, "tool-"+taskID)); err != nil {
			t.Fatal(err)
		}
	}

	taskID := "task_permission_a"
	sessionID := "session-task_permission_a"
	request := PermissionRequestRecord{
		ID: "request-atomic", TaskID: taskID, SessionID: sessionID,
		RunID: "run-task_permission_a", ToolCallID: "tool-task_permission_a",
		Capability: "task.artifact.write", Target: "artifacts/plans/a.md",
		NormalizedTarget: stringPointer(`{"rootKind":"task-artifacts","rootId":"task_permission_a","relativePath":"plans/a.md","operation":"modify"}`),
		Subject:          "write artifact", RiskLevel: "medium", State: "pending",
		RequestedAt: repositoryTestTime,
	}
	if err := store.UpsertPermissionRequest(request); err != nil {
		t.Fatal(err)
	}
	grantID := "grant-atomic"
	grant := PermissionGrantRecord{
		ID: grantID, TaskID: &taskID, SessionID: &sessionID, RequestID: &request.ID,
		Capability: request.Capability, TargetPattern: *request.NormalizedTarget,
		Scope: "once", Decision: "allow", RiskCeiling: "medium",
		CreatedAt: repositoryTestTime, CreatedBy: "user",
	}
	scope := "once"
	resolved, err := store.ResolvePermissionRequest(PermissionResolution{
		TaskID: taskID, RequestID: request.ID, State: "allowed",
		ResolvedAt: "2026-08-01T08:01:00Z", ResolvedBy: "user", DecisionScope: &scope,
	}, &grant)
	if err != nil || resolved.State != "allowed" {
		t.Fatalf("atomic resolution failed: %#v, %v", resolved, err)
	}
	if _, err := store.ResolvePermissionRequest(PermissionResolution{
		TaskID: taskID, RequestID: request.ID, State: "allowed",
		ResolvedAt: "2026-08-01T08:01:01Z", ResolvedBy: "user", DecisionScope: &scope,
	}, nil); !errors.Is(err, ErrPermissionRequestResolved) {
		t.Fatalf("repeated resolution returned %v", err)
	}

	var successes atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := store.ConsumePermissionGrant(
				taskID, grantID, request.ID, "2026-08-01T08:02:00Z",
			); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrPermissionGrantUnavailable) {
				t.Errorf("unexpected consume error: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("once grant was consumed %d times", successes.Load())
	}
	storedGrant, err := store.PermissionGrant(&taskID, grantID)
	if err != nil || storedGrant.ConsumedAt == nil {
		t.Fatalf("once consumption was not persisted: %#v, %v", storedGrant, err)
	}

	taskGrant := PermissionGrantRecord{
		ID: "grant-task-a", TaskID: &taskID, RequestID: &request.ID,
		Capability: request.Capability, TargetPattern: *request.NormalizedTarget,
		Scope: "task", Decision: "allow", RiskCeiling: "medium",
		CreatedAt: repositoryTestTime, CreatedBy: "user",
	}
	if err := store.UpsertPermissionGrant(taskGrant); err != nil {
		t.Fatal(err)
	}
	permanent := PermissionGrantRecord{
		ID: "grant-permanent-audit", RequestID: &request.ID,
		Capability: "task.resource.read", TargetPattern: "*", Scope: "permanent",
		Decision: "allow", RiskCeiling: "medium", CreatedAt: repositoryTestTime, CreatedBy: "user",
	}
	if err := store.UpsertPermissionGrant(permanent); err != nil {
		t.Fatal(err)
	}
	grantsA, err := store.ActivePermissionGrants(taskID, sessionID, "2026-08-01T08:03:00Z")
	if err != nil || !grantIDs(grantsA)[taskGrant.ID] || !grantIDs(grantsA)[permanent.ID] || grantIDs(grantsA)[grantID] {
		t.Fatalf("unexpected task A grants: %#v, %v", grantsA, err)
	}
	grantsB, err := store.ActivePermissionGrants(
		"task_permission_b", "session-task_permission_b", "2026-08-01T08:03:00Z",
	)
	if err != nil || grantIDs(grantsB)[taskGrant.ID] || !grantIDs(grantsB)[permanent.ID] {
		t.Fatalf("task grant leaked or permanent grant disappeared: %#v, %v", grantsB, err)
	}
	revoked, err := store.RevokePermissionGrant(taskID, taskGrant.ID, "2026-08-01T08:04:00Z")
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("revoke failed: %#v, %v", revoked, err)
	}
	grantsA, _ = store.ActivePermissionGrants(taskID, sessionID, "2026-08-01T08:05:00Z")
	if grantIDs(grantsA)[taskGrant.ID] {
		t.Fatal("revoked grant remained active")
	}
}

func TestPermissionRestartExpiresPendingAndSessionGrant(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_permission_restart")
	taskID := "task_permission_restart"
	sessionID := "session-restart"
	if err := store.UpsertAgentSession(repositorySession(taskID, sessionID)); err != nil {
		t.Fatal(err)
	}
	run := repositoryRun(taskID, sessionID, "run-restart")
	run.State = "succeeded"
	if err := store.UpsertExecutionRun(run); err != nil {
		t.Fatal(err)
	}
	tool := repositoryToolCall(taskID, sessionID, run.ID, "tool-restart")
	if err := store.UpsertToolCall(tool); err != nil {
		t.Fatal(err)
	}
	request := PermissionRequestRecord{
		ID: "request-restart", TaskID: taskID, SessionID: sessionID,
		RunID: run.ID, ToolCallID: tool.ID, Capability: "shell.execute",
		Target: "go test ./...", Subject: "run tests", RiskLevel: "high",
		State: "pending", RequestedAt: repositoryTestTime,
	}
	if err := store.UpsertPermissionRequest(request); err != nil {
		t.Fatal(err)
	}
	grant := PermissionGrantRecord{
		ID: "grant-session-restart", TaskID: &taskID, SessionID: &sessionID,
		RequestID: &request.ID, Capability: "task.resource.read", TargetPattern: "*",
		Scope: "session", Decision: "allow", RiskCeiling: "medium",
		CreatedAt: repositoryTestTime, CreatedBy: "user",
	}
	if err := store.UpsertPermissionGrant(grant); err != nil {
		t.Fatal(err)
	}
	count, err := store.ExpirePendingPermissionRequests(
		"2026-08-01T08:10:00Z", "application restarted",
	)
	if err != nil || count != 1 {
		t.Fatalf("restart expiry failed: %d, %v", count, err)
	}
	stored, err := store.PermissionRequest(taskID, request.ID)
	if err != nil || stored.State != "expired" || stored.ResolvedBy == nil || *stored.ResolvedBy != "system" {
		t.Fatalf("pending request did not expire: %#v, %v", stored, err)
	}
	active, err := store.ActivePermissionGrants(taskID, sessionID, "2026-08-01T08:10:01Z")
	if err != nil || len(active) != 0 {
		t.Fatalf("session grant survived restart: %#v, %v", active, err)
	}
}

func grantIDs(records []PermissionGrantRecord) map[string]bool {
	result := make(map[string]bool, len(records))
	for _, record := range records {
		result[record.ID] = true
	}
	return result
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
