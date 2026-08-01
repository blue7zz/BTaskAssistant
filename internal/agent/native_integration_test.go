package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativePIProbeIntegration(t *testing.T) {
	if os.Getenv("BTASK_TEST_NATIVE_PI") != "1" {
		t.Skip("set BTASK_TEST_NATIVE_PI=1 to probe the installed PI CLI")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	process, state, err := StartProcess(ctx, ProcessOptions{
		WorkDir:        root,
		ConfigDir:      filepath.Join(root, "pi-agent"),
		SessionDir:     filepath.Join(root, "pi-sessions"),
		StartupTimeout: 10 * time.Second,
		ShutdownGrace:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("probe installed PI RPC: %v", err)
	}
	if state.SessionID == "" {
		t.Fatal("installed PI RPC returned an empty session id")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := process.Close(closeCtx); err != nil {
		t.Fatalf("close installed PI RPC: %v", err)
	}
}
