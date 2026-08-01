package storage

import "testing"

func TestResourceReferencesAreScopedRemovableAndKeepImmutableResources(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_reference_store_a")
	seedNormalizedTask(t, store, "task_reference_store_b")
	for _, taskID := range []string{"task_reference_store_a", "task_reference_store_b"} {
		byteSize := int64(4)
		sha := repositoryTestSHA256
		path := "attachments/documents/same-" + taskID + ".md"
		if err := store.ReplaceTaskResources(taskID, []TaskResourceRecord{{
			ID: "resource-" + taskID, TaskID: taskID, Kind: "attachment",
			SourceType: "message_attachment", LogicalPath: path,
			StoragePath: stringPointer(path), MIMEType: stringPointer("text/markdown"),
			ByteSize: &byteSize, SHA256: &sha, Immutable: true, Readable: true,
			CreatedAt: repositoryTestTime,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	session := repositorySession("task_reference_store_a", "reference-session")
	session.LastSequence = 1
	if err := store.UpsertAgentSession(session); err != nil {
		t.Fatal(err)
	}
	content := "inspect"
	message := AgentMessageRecord{
		ID: "reference-message", TaskID: session.TaskID, SessionID: session.ID,
		Role: "user", Kind: "text", Status: "complete", Content: &content,
		Sequence: 1, CreatedAt: repositoryTestTime, CompletedAt: stringPointer(repositoryTestTime),
	}
	if err := store.UpsertAgentMessageWithReferences(message, session, []AgentReferenceRecord{{
		ResourceID: "resource-task_reference_store_a", TargetType: "resource",
		Method: "attachment", CreatedAt: repositoryTestTime,
	}}); err != nil {
		t.Fatal(err)
	}
	messages, err := store.AgentMessages(session.TaskID, session.ID)
	if err != nil || len(messages) != 1 || len(messages[0].References) != 1 {
		t.Fatalf("unexpected stored references %#v, error %v", messages, err)
	}
	if err := store.AddAgentMessageReference(AgentReferenceRecord{
		TaskID: session.TaskID, SessionID: session.ID, MessageID: message.ID,
		ResourceID: "resource-task_reference_store_b", TargetType: "resource",
		Method: "mention", Position: 1, CreatedAt: repositoryTestTime,
	}); err == nil {
		t.Fatal("cross-task resource reference was accepted")
	}
	if err := store.RemoveAgentMessageReference(
		session.TaskID, session.ID, message.ID, "resource-missing",
	); err == nil {
		t.Fatal("missing message reference removal was accepted")
	}
	if err := store.RemoveAgentMessageReference(
		session.TaskID, session.ID, message.ID, "resource-task_reference_store_a",
	); err != nil {
		t.Fatal(err)
	}
	messages, err = store.AgentMessages(session.TaskID, session.ID)
	if err != nil || len(messages[0].References) != 0 {
		t.Fatalf("reference was not removed: %#v, error %v", messages, err)
	}
	if _, err := store.TaskResource(session.TaskID, "resource-task_reference_store_a"); err != nil {
		t.Fatalf("removing a reference removed its immutable resource: %v", err)
	}
}

func TestRequirementProposalReferencesAnExistingProposalArtifact(t *testing.T) {
	store := newNormalizedRepositoryStore(t)
	seedNormalizedTask(t, store, "task_proposal_store")
	artifact := WorkspaceArtifactRecord{
		ID: "artifact-proposal", TaskID: "task_proposal_store",
		LogicalPath: "artifacts/proposals/change.md", Kind: "proposal",
		MIMEType: stringPointer("text/markdown"), ByteSize: 8,
		SHA256: repositoryTestSHA256, CreatedAt: repositoryTestTime,
		UpdatedAt: repositoryTestTime,
	}
	if err := store.UpsertWorkspaceArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	proposal := RequirementProposalRecord{
		TaskID: artifact.TaskID, ArtifactID: artifact.ID, BaseRevision: 7,
		State: "pending", CreatedAt: repositoryTestTime,
	}
	if err := store.UpsertRequirementProposal(proposal); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRequirementProposal(proposal); err != nil {
		t.Fatalf("pending proposal update was not idempotent: %v", err)
	}
	proposals, err := store.RequirementProposals(artifact.TaskID)
	if err != nil || len(proposals) != 1 || proposals[0].BaseRevision != 7 {
		t.Fatalf("unexpected proposals %#v, error %v", proposals, err)
	}
	missing := proposal
	missing.ArtifactID = "artifact-missing"
	if err := store.UpsertRequirementProposal(missing); err == nil {
		t.Fatal("proposal without an artifact was accepted")
	}
}
