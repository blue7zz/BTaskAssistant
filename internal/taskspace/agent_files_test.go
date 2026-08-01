package taskspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestImportAttachmentsIsTaskScopedAndSurvivesEnsure(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	for _, taskID := range []string{"task_attachment_a", "task_attachment_b"} {
		if _, err := service.Ensure(root, sampleSnapshot(taskID)); err != nil {
			t.Fatal(err)
		}
	}
	contentA := []byte("task A document")
	contentB := []byte("task B document")
	first, err := service.ImportAttachments(root, "task_attachment_a", []AttachmentInput{{
		Name: "same.md", MIMEType: "text/markdown", Content: contentA,
	}})
	if err != nil {
		t.Fatalf("import task A attachment: %v", err)
	}
	second, err := service.ImportAttachments(root, "task_attachment_b", []AttachmentInput{{
		Name: "same.md", MIMEType: "text/markdown", Content: contentB,
	}})
	if err != nil {
		t.Fatalf("import task B attachment: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID == second[0].ID || first[0].LogicalPath == second[0].LogicalPath {
		t.Fatalf("task-scoped attachment identities were not isolated: %#v %#v", first, second)
	}
	readA, _, err := service.ReadBytes(root, "task_attachment_a", first[0].LogicalPath, MaxAttachmentBytes)
	if err != nil || string(readA) != string(contentA) {
		t.Fatalf("unexpected task A content %q, error %v", readA, err)
	}
	if _, err := service.Read(root, "task_attachment_b", first[0].LogicalPath); err == nil {
		t.Fatal("task B could read task A logical attachment path")
	}

	restarted, err := service.Ensure(root, sampleSnapshot("task_attachment_a"))
	if err != nil {
		t.Fatalf("ensure after attachment import: %v", err)
	}
	found := false
	for _, resource := range restarted.Resources {
		if resource.ID == first[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatal("message attachment disappeared after workspace reconciliation")
	}
}

func TestImportAttachmentsValidatesMIMECountAndSize(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	taskID := "task_attachment_validation"
	if _, err := service.Ensure(root, sampleSnapshot(taskID)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ImportAttachments(root, taskID, []AttachmentInput{{
		Name: "fake.png", MIMEType: "image/png", Content: []byte("not an image"),
	}}); err == nil || !strings.Contains(err.Error(), "supported text extension") {
		t.Fatalf("expected fake image rejection, got %v", err)
	}
	tooMany := make([]AttachmentInput, MaxAttachmentCount+1)
	if _, err := service.ImportAttachments(root, taskID, tooMany); err == nil {
		t.Fatal("attachment count limit was not enforced")
	}
	if _, err := service.ImportAttachments(root, taskID, []AttachmentInput{{
		Name: "large.md", MIMEType: "text/markdown", Content: make([]byte, MaxAttachmentBytes+1),
	}}); err == nil || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("expected attachment size rejection, got %v", err)
	}
	if _, err := service.ImportAttachments(root, taskID, []AttachmentInput{
		{Name: "first.md", Content: make([]byte, MaxAttachmentBytes)},
		{Name: "second.md", Content: make([]byte, MaxAttachmentBytes)},
		{Name: "overflow.md", Content: []byte("x")},
	}); err == nil || !strings.Contains(err.Error(), "batch") {
		t.Fatalf("expected attachment batch size rejection, got %v", err)
	}
	resources, err := service.ImportAttachments(root, taskID, []AttachmentInput{{
		Name: "screen.jpg", MIMEType: "image/png", Content: tinyPNG,
	}})
	if err != nil || len(resources) != 1 || resources[0].MIMEType != "image/png" || !strings.HasSuffix(resources[0].LogicalPath, ".png") {
		t.Fatalf("image signature was not authoritative: %#v, error %v", resources, err)
	}
}

func TestWriteArtifactAllowsOnlyControlledArtifactDirectories(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	taskID := "task_artifact"
	if _, err := service.Ensure(root, sampleSnapshot(taskID)); err != nil {
		t.Fatal(err)
	}
	artifact, err := service.WriteArtifact(root, taskID, "proposal", "scope-change.md", []byte("# 修改建议\n"))
	if err != nil {
		t.Fatalf("write proposal: %v", err)
	}
	if artifact.LogicalPath != "artifacts/proposals/scope-change.md" || artifact.Kind != "proposal" {
		t.Fatalf("unexpected proposal %#v", artifact)
	}
	info, err := os.Stat(filepath.Join(root, taskID, filepath.FromSlash(artifact.LogicalPath)))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact mode %#v, error %v", info, err)
	}
	for _, attempt := range []struct {
		kind string
		name string
	}{
		{"proposal", "../context/requirements/current.md"},
		{"proposal", `C:\\approved.md`},
		{"source", "overwrite.md"},
		{"report", "binary.exe"},
	} {
		if _, err := service.WriteArtifact(root, taskID, attempt.kind, attempt.name, []byte("blocked")); err == nil {
			t.Fatalf("unsafe artifact target was accepted: %#v", attempt)
		}
	}
	if _, err := service.WriteArtifact(
		root, taskID, "report", "large.md", make([]byte, MaxArtifactBytes+1),
	); err == nil {
		t.Fatal("oversized artifact was accepted")
	}
	if _, err := service.WriteArtifact(root, taskID, "report", "binary.md", []byte{0xff}); err == nil {
		t.Fatal("non-UTF-8 artifact was accepted")
	}
	approved := filepath.Join(root, taskID, "context", "requirements", "approved-v1.md")
	before, err := os.ReadFile(approved)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(approved)
	if err != nil || string(after) != string(before) {
		t.Fatal("approved requirement changed during artifact writes")
	}
}

func TestWriteArtifactRejectsSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	service := fixedService()
	taskID := "task_artifact_symlink"
	if _, err := service.Ensure(root, sampleSnapshot(taskID)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, taskID, "artifacts", "reports", "linked.md")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := service.WriteArtifact(root, taskID, "report", "linked.md", []byte("overwrite")); err == nil {
		t.Fatal("artifact symlink target was accepted")
	}
	content, err := os.ReadFile(outside)
	if err != nil || string(content) != "outside" {
		t.Fatal("artifact write followed a symlink")
	}
}

func TestAtomicArtifactWriteCleansTemporaryFileAfterDiskFailure(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootPath, "artifacts", "reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	_, err = writeAtomicFileWithRename(
		root,
		"artifacts/reports/full.md",
		[]byte("content that must not become visible"),
		func(string, string) error { return syscall.ENOSPC },
	)
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("disk-full rename failure was not returned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootPath, "artifacts", "reports", "full.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed artifact became visible: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(rootPath, "artifacts", "reports"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed artifact left temporary files: %#v, %v", entries, err)
	}
}
