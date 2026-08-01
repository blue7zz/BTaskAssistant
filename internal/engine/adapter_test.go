package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativePIProbeUsesIsolatedAgentDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake PI probe uses a POSIX shell")
	}
	binDirectory := t.TempDir()
	capturePath := filepath.Join(t.TempDir(), "pi-agent-dir.txt")
	sessionCapturePath := filepath.Join(t.TempDir(), "pi-session-dir.txt")
	piPath := filepath.Join(binDirectory, "pi")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
printf '%%s' "$PI_CODING_AGENT_DIR" > %q
printf '%%s' "$PI_CODING_AGENT_SESSION_DIR" > %q
printf '%%s\n' '0.82.1'
`, capturePath, sessionCapturePath)
	if err := os.WriteFile(piPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory)
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(t.TempDir(), "global-pi-agent"))

	path, version, err := nativePICommandDetails()
	if err != nil {
		t.Fatalf("probe native PI: %v", err)
	}
	if path != piPath || version != "0.82.1" {
		t.Fatalf("unexpected PI probe path %q version %q", path, version)
	}
	isolated := strings.TrimSpace(string(mustReadFile(t, capturePath)))
	if isolated == "" || isolated == os.Getenv("PI_CODING_AGENT_DIR") {
		t.Fatalf("PI probe was not isolated: %q", isolated)
	}
	sessionDirectory := strings.TrimSpace(string(mustReadFile(t, sessionCapturePath)))
	if sessionDirectory != filepath.Join(isolated, "sessions") {
		t.Fatalf("PI session probe was not isolated: %q", sessionDirectory)
	}
	if _, err := os.Stat(isolated); !os.IsNotExist(err) {
		t.Fatalf("PI probe directory was not cleaned up: %v", err)
	}
}

func TestNativePIProbeRejectsBrokenExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake PI probe uses a POSIX shell")
	}
	binDirectory := t.TempDir()
	piPath := filepath.Join(binDirectory, "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory)
	if _, _, err := nativePICommandDetails(); err == nil {
		t.Fatal("broken native PI executable was reported as configured")
	}
}

func TestStatusesExposeNativePIRPCAnalysis(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake PI probe uses a POSIX shell")
	}
	binDirectory := t.TempDir()
	piPath := filepath.Join(binDirectory, "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\nprintf '0.82.1\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory)
	statuses := Statuses()
	if len(statuses) != 2 {
		t.Fatalf("unexpected statuses %#v", statuses)
	}
	pi := statuses[0]
	if pi.ID != "pi" || pi.Label != "PI" || !pi.Configured || !pi.RequirementAnalysis || pi.CommandPath != piPath {
		t.Fatalf("unexpected native PI status %#v", pi)
	}
	if strings.Contains(strings.ToLower(pi.Description), "oh-my-pi") || strings.Contains(strings.ToLower(pi.Description), "omp") {
		t.Fatalf("PI status retained legacy identity: %#v", pi)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
