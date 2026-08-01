package agent

import (
	"context"
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
