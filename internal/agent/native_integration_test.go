package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	permissionpolicy "github.com/blue7zz/BTaskAssistant/internal/permissions"
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
		Version:            gateExtensionVersion,
		PermissionProtocol: permissionProtocolVersion,
		Nonce:              "native-integration-nonce",
		TaskID:             "task_native_gate",
		SessionID:          "session_native_gate",
		Mode:               "ask",
		SelfTest:           true,
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
	callDone := make(chan error, 1)
	go func() {
		callCtx, callCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer callCancel()
		callDone <- process.Call(callCtx, "prompt", map[string]any{
			"message": "/btask-gate-self-test",
		}, nil)
	}()
	sawConfirm := false
	sawSuccessNotice := false
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for !sawConfirm || !sawSuccessNotice {
		select {
		case event, open := <-process.Events():
			if !open {
				t.Fatal("installed PI exited during permission self-test")
			}
			if event.Type != "extension_ui_request" {
				continue
			}
			var request struct {
				ID         string `json:"id"`
				Method     string `json:"method"`
				Title      string `json:"title"`
				Message    string `json:"message"`
				NotifyType string `json:"notifyType"`
			}
			if json.Unmarshal(event.JSON, &request) != nil {
				t.Fatalf("decode native PI extension UI request: %s", event.JSON)
			}
			if request.Method == "confirm" && request.Title == "btask-permission-self-test" {
				var envelope map[string]string
				wantDigest, digestErr := permissionpolicy.CanonicalArgsDigest(json.RawMessage(
					`{"text":"line\u2028next\u2029end","nested":{"b":2,"a":1}}`,
				))
				if json.Unmarshal([]byte(request.Message), &envelope) != nil ||
					digestErr != nil || envelope["argsDigest"] != wantDigest ||
					envelope["protocol"] != permissionProtocolVersion ||
					envelope["version"] != gateExtensionVersion ||
					envelope["nonce"] != "native-integration-nonce" {
					t.Fatalf("native PI permission envelope mismatch: %s", request.Message)
				}
				responseCtx, responseCancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := process.Send(responseCtx, map[string]any{
					"type": "extension_ui_response", "id": request.ID, "confirmed": true,
				})
				responseCancel()
				if err != nil {
					t.Fatalf("respond to native PI confirm: %v", err)
				}
				sawConfirm = true
			}
			if request.Method == "notify" && strings.Contains(request.Message, "self-test passed") {
				sawSuccessNotice = true
			}
		case err := <-callDone:
			if err != nil {
				t.Fatalf("invoke native PI gate self-test: %v", err)
			}
			callDone = nil
		case <-deadline.C:
			t.Fatalf("native PI confirm self-test timed out; confirm=%v notice=%v", sawConfirm, sawSuccessNotice)
		}
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := process.Close(closeCtx); err != nil {
		t.Fatalf("close installed PI gate session: %v", err)
	}
}
