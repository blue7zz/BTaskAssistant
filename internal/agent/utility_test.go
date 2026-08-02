package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUtilityRunnerUsesOneNoToolsRPCSession(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	runner := UtilityRunner{RuntimeFactory: factory, RequestTimeout: time.Second}
	output, err := runner.Run(context.Background(), UtilityRequest{
		Prompt: "结构化输入", Model: "openai/test-model", ThinkingLevel: "high",
	})
	if err != nil {
		t.Fatalf("run utility: %v", err)
	}
	if output != "回答：结构化输入" {
		t.Fatalf("unexpected utility output %q", output)
	}
	if factory.count() != 1 || !factory.runtime(0).called("set_model") ||
		!factory.runtime(0).called("set_thinking_level") || !factory.runtime(0).called("prompt") {
		t.Fatalf("unexpected utility calls %#v", factory.runtime(0).calls)
	}
}

func TestUtilityRunnerExplicitlyUsesLocalPILoginConfig(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "pi-login")
	t.Setenv("PI_CODING_AGENT_DIR", configured)
	factory := &fakeRuntimeFactory{}
	runner := UtilityRunner{RuntimeFactory: factory, RequestTimeout: time.Second}
	if _, err := runner.Run(context.Background(), UtilityRequest{
		Prompt: "结构化输入", ResourcePolicy: resourcePolicyExplicitInherit,
	}); err != nil {
		t.Fatalf("run utility: %v", err)
	}
	options := factory.processOptions(0)
	if options.ConfigDir != configured {
		t.Fatalf("config dir = %q, want %q", options.ConfigDir, configured)
	}
	if options.SessionDir == configured || !strings.Contains(options.SessionDir, "btask-pi-utility-") {
		t.Fatalf("utility session dir was not isolated: %q", options.SessionDir)
	}
}

func TestUtilityRunnerRejectsBadModelAndOversizeOutput(t *testing.T) {
	factory := &fakeRuntimeFactory{}
	runner := UtilityRunner{RuntimeFactory: factory, RequestTimeout: time.Second}
	if _, err := runner.Run(context.Background(), UtilityRequest{
		Prompt: "input", Model: "missing-provider",
	}); err == nil {
		t.Fatal("invalid model was accepted")
	}
	if _, err := runner.Run(context.Background(), UtilityRequest{
		Prompt: "123456789", MaxOutputBytes: 4,
	}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected output limit error, got %v", err)
	}
}

func TestContextualizePICredentialErrorExplainsSelectedPolicy(t *testing.T) {
	missing := errors.New("No API key found for the selected model")
	isolated := contextualizePICredentialError(missing, resourcePolicyIsolated)
	if !strings.Contains(isolated.Error(), "设置 > PI") {
		t.Fatalf("isolated error did not point to PI settings: %v", isolated)
	}
	inherited := contextualizePICredentialError(missing, resourcePolicyExplicitInherit)
	if !strings.Contains(inherited.Error(), "/login") {
		t.Fatalf("inherited error did not point to PI login: %v", inherited)
	}
}
