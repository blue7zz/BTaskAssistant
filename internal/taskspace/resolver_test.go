package taskspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLogicalPathRejectsTraversalAbsoluteAndWindowsPaths(t *testing.T) {
	for _, value := range []string{
		"../outside",
		"context/../outside",
		"/absolute",
		`C:\\Windows\\system32`,
		`\\\\server\\share`,
		"context//task.md",
		"context/./task.md",
		"CON/file.txt",
		"context/trailing./file.txt",
		" context/task.md",
		"context/ leading.md",
		"context/name:stream",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := ValidateLogicalPath(value); err == nil {
				t.Fatalf("expected unsafe path %q to be rejected", value)
			}
		})
	}
	for _, value := range []string{"", ".", "context", "context/task.md"} {
		if _, err := ValidateLogicalPath(value); err != nil {
			t.Fatalf("expected safe path %q: %v", value, err)
		}
	}
}

func TestListAndReadStayInsideTaskWorkspace(t *testing.T) {
	root := t.TempDir()
	taskID := "task_preview"
	service := fixedService()
	if _, err := service.Ensure(root, sampleSnapshot(taskID)); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	entries, err := service.List(root, taskID, "")
	if err != nil {
		t.Fatalf("list workspace root: %v", err)
	}
	if len(entries) == 0 || entries[0].Type != "directory" {
		t.Fatalf("unexpected workspace entries %#v", entries)
	}
	preview, err := service.Read(root, taskID, "context/task.md")
	if err != nil {
		t.Fatalf("read task context: %v", err)
	}
	if preview.Kind != "text" || !strings.Contains(preview.Content, "实现任务空间") || len(preview.SHA256) != 64 {
		t.Fatalf("unexpected task context preview %#v", preview)
	}
	imageEntries, err := service.List(root, taskID, "attachments/images")
	if err != nil || len(imageEntries) != 1 {
		t.Fatalf("list images: entries %#v, error %v", imageEntries, err)
	}
	image, err := service.Read(root, taskID, imageEntries[0].Path)
	if err != nil || image.Kind != "image" || !strings.HasPrefix(image.Content, "data:image/png;base64,") {
		t.Fatalf("unexpected image preview %#v, error %v", image, err)
	}
	if _, err := service.Read(root, taskID, ".btask/pi-agent/credentials.json"); err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Fatalf("expected private PI directory rejection, got %v", err)
	}
}

func TestReadRejectsSymlinksAndCaseCollisions(t *testing.T) {
	root := t.TempDir()
	taskID := "task_paths"
	service := fixedService()
	if _, err := service.Ensure(root, sampleSnapshot(taskID)); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, taskID, "context", "outside.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := service.Read(root, taskID, "context/outside.md"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if _, err := service.Read(root, taskID, "Context/task.md"); err == nil || !strings.Contains(err.Error(), "case collision") {
		t.Fatalf("expected case collision rejection, got %v", err)
	}
}
