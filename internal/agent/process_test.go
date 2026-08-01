package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartProcessUsesNativeRPCProbeAndStreamsEvents(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	process, state, err := StartProcess(context.Background(), helperProcessOptions(root))
	if err != nil {
		t.Fatalf("start helper pi: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = process.Close(ctx)
	})
	if state.SessionID != "pi-helper-session" {
		t.Fatalf("unexpected probe state %#v", state)
	}

	if err := process.Call(
		context.Background(),
		"prompt",
		map[string]any{"message": "保留\u2028字符"},
		nil,
	); err != nil {
		t.Fatalf("prompt helper pi: %v", err)
	}
	want := []string{"agent_start", "message_update", "message_end", "agent_settled"}
	for _, kind := range want {
		select {
		case event := <-process.Events():
			if event.Type != kind {
				t.Fatalf("expected %s, got %#v", kind, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", kind)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := process.Close(ctx); err != nil {
		t.Fatalf("close helper pi: %v", err)
	}
	if exit := process.Exit(); exit.Code != 0 || exit.Err != nil {
		t.Fatalf("unexpected clean exit %#v", exit)
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("PI process changed work directory permissions: %v, %v", info, err)
	}
}

func TestStartProcessLoadsOnlyTheScopedGateExtension(t *testing.T) {
	root := t.TempDir()
	extensionPath := filepath.Join(root, "gate.ts")
	if err := os.WriteFile(extensionPath, []byte("export default function () {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := helperProcessOptions(root)
	options.AdditionalEnv = append(options.AdditionalEnv,
		"BTASK_PI_GATE=1",
		"BTASK_PI_GATE_VERSION=btask-gate/v1",
		"BTASK_PI_GATE_NONCE=nonce-1",
	)
	options.Gate = &GateProcessOptions{
		ExtensionPath: extensionPath,
		Version:       "btask-gate/v1",
		Nonce:         "nonce-1",
		SHA256:        fileSHA256(t, extensionPath),
	}
	process, _, err := StartProcess(context.Background(), options)
	if err != nil {
		t.Fatalf("start helper PI with gate: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := process.Close(ctx); err != nil {
		t.Fatal(err)
	}
	tampered := *options.Gate
	tampered.SHA256 = strings.Repeat("0", 64)
	if err := validateGateProcessOptions(tampered); err == nil {
		t.Fatal("tampered gate extension hash was accepted")
	}
}

func TestStartProcessRejectsUnsupportedVersion(t *testing.T) {
	root := t.TempDir()
	options := helperProcessOptions(root)
	options.AdditionalEnv = append(options.AdditionalEnv, "BTASK_PI_VERSION=0.83.0")
	_, _, err := StartProcess(context.Background(), options)
	if !errors.Is(err, ErrUnsupportedPIVersion) {
		t.Fatalf("expected unsupported version, got %v", err)
	}
}

func TestStartProcessReportsMissingExecutableAndProbeTimeout(t *testing.T) {
	root := t.TempDir()
	missing := helperProcessOptions(root)
	missing.Executable = filepath.Join(root, "missing-pi")
	if _, _, err := StartProcess(context.Background(), missing); !errors.Is(err, ErrPINotInstalled) {
		t.Fatalf("expected missing PI error, got %v", err)
	}

	timedOut := helperProcessOptions(t.TempDir())
	timedOut.StartupTimeout = 2 * time.Second
	timedOut.AdditionalEnv = append(timedOut.AdditionalEnv, "BTASK_PI_NO_PROBE=1")
	_, _, err := StartProcess(context.Background(), timedOut)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected startup probe timeout, got %v", err)
	}
}

func TestProcessTerminatesOnInvalidPIFrame(t *testing.T) {
	root := t.TempDir()
	options := helperProcessOptions(root)
	options.AdditionalEnv = append(options.AdditionalEnv, "BTASK_PI_INVALID=1")
	process, _, err := StartProcess(context.Background(), options)
	if err != nil {
		t.Fatalf("start helper pi: %v", err)
	}
	select {
	case <-process.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("invalid frame did not terminate the process")
	}
	if !errors.Is(process.Exit().Err, ErrInvalidFrame) {
		t.Fatalf("expected protocol error exit, got %#v", process.Exit())
	}
}

func TestProcessReportsTruncatedEOFAndCapsStderr(t *testing.T) {
	t.Run("truncated EOF", func(t *testing.T) {
		root := t.TempDir()
		options := helperProcessOptions(root)
		options.AdditionalEnv = append(options.AdditionalEnv, "BTASK_PI_TRUNCATED=1")
		process, _, err := StartProcess(context.Background(), options)
		if err != nil {
			t.Fatalf("start helper pi: %v", err)
		}
		select {
		case <-process.Done():
		case <-time.After(2 * time.Second):
			t.Fatal("truncated EOF did not end the process")
		}
		if !errors.Is(process.Exit().Err, ErrTruncatedFrame) {
			t.Fatalf("expected truncated frame, got %#v", process.Exit())
		}
	})

	t.Run("stderr tail", func(t *testing.T) {
		root := t.TempDir()
		options := helperProcessOptions(root)
		options.MaxStderrBytes = 32
		options.AdditionalEnv = append(options.AdditionalEnv, "BTASK_PI_STDERR=1")
		process, _, err := StartProcess(context.Background(), options)
		if err != nil {
			t.Fatalf("start helper pi: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for !strings.HasSuffix(process.StderrTail(), "TAIL") && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := process.Close(ctx); err != nil {
			t.Fatal(err)
		}
		tail := process.StderrTail()
		if len(tail) > 32 || !strings.HasSuffix(tail, "TAIL") {
			t.Fatalf("stderr tail was not bounded correctly: %q", tail)
		}
	})
}

func TestValidatePIVersion(t *testing.T) {
	for _, value := range []string{"0.82.1", "0.82.9"} {
		if err := validatePIVersion(value); err != nil {
			t.Fatalf("expected %s to be supported: %v", value, err)
		}
	}
	for _, value := range []string{"", "0.81.9", "0.83.0", "1.0.0"} {
		if !errors.Is(validatePIVersion(value), ErrUnsupportedPIVersion) {
			t.Fatalf("expected %s to be rejected", value)
		}
	}
}

func helperProcessOptions(root string) ProcessOptions {
	return ProcessOptions{
		Executable:     os.Args[0],
		PrefixArgs:     []string{"-test.run=TestPIHelperProcess", "--"},
		AdditionalEnv:  []string{"BTASK_PI_HELPER=1", "BTASK_PI_VERSION=0.82.1"},
		WorkDir:        root,
		ConfigDir:      filepath.Join(root, "config"),
		SessionDir:     filepath.Join(root, "sessions"),
		StartupTimeout: 2 * time.Second,
		RequestTimeout: 2 * time.Second,
		ShutdownGrace:  2 * time.Second,
	}
}

func TestPIHelperProcess(t *testing.T) {
	if os.Getenv("BTASK_PI_HELPER") != "1" {
		return
	}
	for _, argument := range os.Args {
		if argument == "--version" {
			fmt.Println(os.Getenv("BTASK_PI_VERSION"))
			os.Exit(0)
		}
	}

	sessionDir := helperArgument("--session-dir")
	if sessionDir == "" {
		fmt.Fprintln(os.Stderr, "missing session dir")
		os.Exit(4)
	}
	if helperArgument("--mode") != "rpc" {
		fmt.Fprintln(os.Stderr, "missing rpc mode")
		os.Exit(4)
	}
	for _, required := range []string{
		"--no-extensions",
		"--no-skills",
		"--no-prompt-templates",
		"--no-themes",
		"--no-context-files",
		"--no-approve",
		"--offline",
	} {
		if !helperHasArgument(required) {
			fmt.Fprintf(os.Stderr, "missing required flag %s\n", required)
			os.Exit(4)
		}
	}
	if os.Getenv("BTASK_PI_GATE") == "1" {
		if helperArgument("-e") == "" || !helperHasArgument("--no-builtin-tools") || helperHasArgument("--no-tools") {
			fmt.Fprintln(os.Stderr, "gate flags are invalid")
			os.Exit(4)
		}
	} else if !helperHasArgument("--no-tools") || helperHasArgument("--no-builtin-tools") {
		fmt.Fprintln(os.Stderr, "utility tool flags are invalid")
		os.Exit(4)
	}
	if os.Getenv("BTASK_PI_HANG_AFTER_EOF") == "1" {
		configureHelperHang()
	}
	sessionFile := filepath.Join(sessionDir, "pi-helper-session.jsonl")
	reader := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	write := func(value any) {
		encoded, _ := json.Marshal(value)
		_, _ = writer.Write(encoded)
		_ = writer.WriteByte('\n')
		_ = writer.Flush()
	}
	if os.Getenv("BTASK_PI_GATE") == "1" {
		write(map[string]any{
			"type":       "extension_ui_request",
			"id":         "gate-heartbeat",
			"method":     "setStatus",
			"statusKey":  "btask-gate",
			"statusText": os.Getenv("BTASK_PI_GATE_VERSION") + ":" + os.Getenv("BTASK_PI_GATE_NONCE"),
		})
	}
	for reader.Scan() {
		var request map[string]any
		if err := json.Unmarshal(reader.Bytes(), &request); err != nil {
			os.Exit(5)
		}
		id, _ := request["id"].(string)
		command, _ := request["type"].(string)
		switch command {
		case "get_state":
			if os.Getenv("BTASK_PI_NO_PROBE") == "1" {
				continue
			}
			write(map[string]any{
				"id": id, "type": "response", "command": command, "success": true,
				"data": map[string]any{
					"sessionId": "pi-helper-session", "sessionFile": sessionFile,
					"thinkingLevel": "high", "isStreaming": false,
				},
			})
			if os.Getenv("BTASK_PI_INVALID") == "1" {
				_, _ = writer.WriteString("terminal noise\n")
				_ = writer.Flush()
				time.Sleep(time.Second)
			}
			if os.Getenv("BTASK_PI_STDERR") == "1" {
				fmt.Fprint(os.Stderr, strings.Repeat("x", 128)+"TAIL")
			}
			if os.Getenv("BTASK_PI_TRUNCATED") == "1" {
				_, _ = writer.WriteString(`{"type":"agent_start"`)
				_ = writer.Flush()
				os.Exit(0)
			}
		case "prompt":
			write(map[string]any{
				"id": id, "type": "response", "command": command, "success": true,
			})
			write(map[string]any{"type": "agent_start"})
			write(map[string]any{
				"type": "message_update",
				"assistantMessageEvent": map[string]any{
					"type": "text_delta", "delta": "你好\u2028PI",
				},
			})
			write(map[string]any{
				"type": "message_end",
				"message": map[string]any{
					"role":    "assistant",
					"content": []map[string]any{{"type": "text", "text": "你好\u2028PI"}},
				},
			})
			write(map[string]any{"type": "agent_settled"})
		case "abort", "set_thinking_level", "set_model", "switch_session":
			write(map[string]any{
				"id": id, "type": "response", "command": command, "success": true,
			})
		case "get_entries":
			write(map[string]any{
				"id": id, "type": "response", "command": command, "success": true,
				"data": map[string]any{"entries": []any{}, "leafId": nil},
			})
		default:
			write(map[string]any{
				"id": id, "type": "response", "command": command,
				"success": false, "error": "unsupported helper command",
			})
		}
	}
	if os.Getenv("BTASK_PI_HANG_AFTER_EOF") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(0)
}

func helperArgument(name string) string {
	for index, argument := range os.Args {
		if argument == name && index+1 < len(os.Args) {
			return strings.TrimSpace(os.Args[index+1])
		}
	}
	return ""
}

func helperHasArgument(name string) bool {
	for _, argument := range os.Args {
		if argument == name {
			return true
		}
	}
	return false
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash[:])
}
