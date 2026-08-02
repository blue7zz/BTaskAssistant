package engine

import (
	"testing"
)

func TestNormalizePISettingsUsesCompatibleThinkingDefault(t *testing.T) {
	settings, err := NormalizePISettings(PISettings{})
	if err != nil {
		t.Fatalf("normalize settings: %v", err)
	}
	if settings.ThinkingEffort != "xhigh" {
		t.Fatalf("thinking effort = %q, want xhigh", settings.ThinkingEffort)
	}
	if settings.TimeoutMinutes != 3 {
		t.Fatalf("timeout = %d, want 3", settings.TimeoutMinutes)
	}
	if settings.ResourcePolicy != "isolated" {
		t.Fatalf("resource policy = %q, want isolated", settings.ResourcePolicy)
	}

	settings, err = NormalizePISettings(PISettings{ThinkingEffort: "max"})
	if err != nil {
		t.Fatalf("normalize legacy max setting: %v", err)
	}
	if settings.ThinkingEffort != "xhigh" {
		t.Fatalf("legacy max = %q, want xhigh", settings.ThinkingEffort)
	}
}

func TestNormalizePISettingsPreservesNativeRPCPreferences(t *testing.T) {
	settings, err := NormalizePISettings(PISettings{
		Model:          "  openai-codex/gpt-5.5  ",
		ThinkingEffort: "high",
		TimeoutMinutes: 5,
		ResourcePolicy: "explicit-inherit",
	})
	if err != nil {
		t.Fatalf("normalize settings: %v", err)
	}
	if settings.Model != "openai-codex/gpt-5.5" ||
		settings.ThinkingEffort != "high" ||
		settings.TimeoutMinutes != 5 ||
		settings.ResourcePolicy != "explicit-inherit" {
		t.Fatalf("unexpected normalized PI settings %#v", settings)
	}
	if _, err := NormalizePISettings(PISettings{ResourcePolicy: "global"}); err == nil {
		t.Fatal("unsupported PI resource policy was accepted")
	}
}
