package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestTaskWorkspaceAndResourceRepositoriesEnforceTaskScope(t *testing.T) {
	store := NewSQLiteStoreAt(filepath.Join(t.TempDir(), "repository.db"))
	t.Cleanup(func() { _ = store.Close() })
	createdAt := "2026-08-01T08:00:00Z"
	for _, record := range []TaskWorkspaceRecord{
		{TaskID: "task_first", WorkspaceID: "workspace-first", RootPath: "/tasks/task_first", SchemaVersion: 1, ManifestRevision: 1, State: "ready", CreatedAt: createdAt, UpdatedAt: createdAt},
		{TaskID: "task_second", WorkspaceID: "workspace-second", RootPath: "/tasks/task_second", SchemaVersion: 1, ManifestRevision: 1, State: "ready", CreatedAt: createdAt, UpdatedAt: createdAt},
	} {
		if err := store.UpsertTaskWorkspace(record); err != nil {
			t.Fatalf("upsert workspace: %v", err)
		}
	}
	path := "context/task.md"
	mimeType := "text/markdown"
	byteSize := int64(10)
	hash := repositoryTestSHA256
	resource := TaskResourceRecord{
		ID:          "resource-shared-id",
		TaskID:      "task_first",
		Kind:        "context",
		SourceType:  "task",
		LogicalPath: path,
		StoragePath: &path,
		MIMEType:    &mimeType,
		ByteSize:    &byteSize,
		SHA256:      &hash,
		Readable:    true,
		CreatedAt:   createdAt,
	}
	if err := store.ReplaceTaskResources("task_first", []TaskResourceRecord{resource}); err != nil {
		t.Fatalf("replace first resources: %v", err)
	}
	if _, err := store.TaskResource("task_second", resource.ID); !errors.Is(err, ErrTaskWorkspaceNotFound) {
		t.Fatalf("cross-task resource read was not rejected: %v", err)
	}
	resource.TaskID = "task_second"
	if err := store.ReplaceTaskResources("task_second", []TaskResourceRecord{resource}); err == nil {
		t.Fatal("cross-task resource id reuse was accepted")
	}
	firstResources, err := store.TaskResources("task_first")
	if err != nil || len(firstResources) != 1 || firstResources[0].TaskID != "task_first" {
		t.Fatalf("unexpected first task resources %#v, error %v", firstResources, err)
	}
	if err := store.DeleteTaskWorkspace("task_first"); err == nil {
		t.Fatal("workspace with retained resources was deleted")
	}
	if err := store.DeleteTaskWorkspace("task_second"); err != nil {
		t.Fatalf("delete empty workspace: %v", err)
	}
	if _, err := store.TaskWorkspace("task_second"); !errors.Is(err, ErrTaskWorkspaceNotFound) {
		t.Fatalf("deleted workspace still exists: %v", err)
	}
}
