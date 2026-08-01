package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateSessionPathRejectsEscapeNestedAndSymlinkPaths(t *testing.T) {
	base := t.TempDir()
	valid := filepath.Join(base, "session.jsonl")
	if err := os.WriteFile(valid, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := validateSessionPath(base, valid, true); err != nil || got != valid {
		t.Fatalf("valid direct session path was rejected: %q, %v", got, err)
	}
	if _, err := validateSessionPath(base, filepath.Join(t.TempDir(), "outside.jsonl"), false); err == nil {
		t.Fatal("outside session path was accepted")
	}
	nested := filepath.Join(base, "nested", "session.jsonl")
	if _, err := validateSessionPath(base, nested, false); err == nil {
		t.Fatal("nested session path was accepted")
	}
	missing := filepath.Join(base, "missing.jsonl")
	if _, err := validateSessionPath(base, missing, false); err != nil {
		t.Fatalf("future direct session path was rejected: %v", err)
	}
	if _, err := validateSessionPath(base, missing, true); err == nil {
		t.Fatal("missing registered session path was accepted")
	}

	if runtime.GOOS != "windows" {
		link := filepath.Join(base, "linked.jsonl")
		if err := os.Symlink(valid, link); err != nil {
			t.Fatal(err)
		}
		if _, err := validateSessionPath(base, link, true); err == nil {
			t.Fatal("symlink session path was accepted")
		}
	}
}
