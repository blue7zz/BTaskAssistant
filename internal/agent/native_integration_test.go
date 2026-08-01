package agent

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestNativePIGateExtensionIntegration(t *testing.T) {
	if os.Getenv("BTASK_TEST_NATIVE_PI") != "1" {
		t.Skip("set BTASK_TEST_NATIVE_PI=1 to probe the installed PI CLI")
	}
	root := t.TempDir()
	config, err := json.Marshal(gateExtensionConfig{
		Version:   gateExtensionVersion,
		Nonce:     "native-integration-nonce",
		TaskID:    "task_native_gate",
		SessionID: "session_native_gate",
		Mode:      "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	extensionPath := filepath.Join(root, "btask-gate.ts")
	content := bytes.Replace(gateExtensionTemplate, []byte("__BTASK_GATE_CONFIG__"), config, 1)
	if err := os.WriteFile(extensionPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	process, state, err := StartProcess(ctx, ProcessOptions{
		WorkDir: root, ConfigDir: filepath.Join(root, "pi-agent"),
		SessionDir:     filepath.Join(root, "pi-sessions"),
		StartupTimeout: 20 * time.Second, ShutdownGrace: 2 * time.Second,
		Gate: &GateProcessOptions{
			ExtensionPath: extensionPath, Version: gateExtensionVersion,
			Nonce: "native-integration-nonce", SHA256: fileSHA256(t, extensionPath),
		},
	})
	if err != nil {
		t.Fatalf("load installed PI with BTask gate: %v", err)
	}
	if state.SessionID == "" {
		t.Fatal("installed PI gate session returned an empty session id")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := process.Close(closeCtx); err != nil {
		t.Fatalf("close installed PI gate session: %v", err)
	}
}
