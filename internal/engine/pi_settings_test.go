package engine

import (
	"slices"
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

	settings, err = NormalizePISettings(PISettings{ThinkingEffort: "max"})
	if err != nil {
		t.Fatalf("normalize legacy max setting: %v", err)
	}
	if settings.ThinkingEffort != "xhigh" {
		t.Fatalf("legacy max = %q, want xhigh", settings.ThinkingEffort)
	}
}

func TestPICommandArgumentsOverrideGlobalDefaults(t *testing.T) {
	args, err := piCommandArguments(PISettings{
		Model:          "openai-codex/gpt-5.5",
		ThinkingEffort: "high",
		TimeoutMinutes: 5,
	})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	for _, expected := range []string{
		"--model=openai-codex/gpt-5.5",
		"--thinking=high",
		"--max-time=5m",
	} {
		if !slices.Contains(args, expected) {
			t.Fatalf("args %v missing %q", args, expected)
		}
	}
}
