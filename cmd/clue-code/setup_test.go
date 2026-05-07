package main

import (
	"os"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/setup"
)

// TestSetup_Recommend_Sensitive verifies that Sensitive=true maps to Ollama.
func TestSetup_Recommend_Sensitive(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{Sensitive: true})
	if got.Provider != "ollama" {
		t.Errorf("Sensitive=true → want ollama, got %q", got.Provider)
	}
}

// TestSetup_Recommend_CostFirst verifies that PriorityCost=true maps to DeepSeek.
func TestSetup_Recommend_CostFirst(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{PriorityCost: true})
	if got.Provider != "deepseek" {
		t.Errorf("PriorityCost=true → want deepseek, got %q", got.Provider)
	}
}

// TestSetup_Recommend_Quality verifies the default (all false) maps to Anthropic.
func TestSetup_Recommend_Quality(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{})
	if got.Provider != "anthropic" {
		t.Errorf("default → want anthropic, got %q", got.Provider)
	}
}

// TestSetup_DetectOllama_NotInstalled verifies that an empty PATH means
// installed=false.
func TestSetup_DetectOllama_NotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	installed, _, _ := setup.DetectOllama()
	if installed {
		t.Error("expected installed=false with empty PATH")
	}
}

// TestSetup_Progress_RoundTrip verifies that SaveProgress + LoadProgress are
// inverses.
func TestSetup_Progress_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := setup.Progress{
		Stage:    "questions",
		Provider: "deepseek",
		PartialAnswers: setup.Answers{
			PriorityCost: true,
		},
		StartedAt: time.Now().Truncate(time.Second),
	}

	if err := setup.SaveProgress(want); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	got, err := setup.LoadProgress()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if got.Stage != want.Stage {
		t.Errorf("Stage: got %q, want %q", got.Stage, want.Stage)
	}
	if got.Provider != want.Provider {
		t.Errorf("Provider: got %q, want %q", got.Provider, want.Provider)
	}
	if got.PartialAnswers != want.PartialAnswers {
		t.Errorf("PartialAnswers mismatch: got %+v, want %+v",
			got.PartialAnswers, want.PartialAnswers)
	}
}

// TestSetup_Progress_Resume simulates a pre-existing setup-progress.json file
// and verifies that HasProgress correctly detects it.
func TestSetup_Progress_Resume(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if setup.HasProgress() {
		t.Fatal("expected HasProgress=false in empty temp dir")
	}

	p := setup.Progress{
		Stage:    "q2",
		Provider: "",
		PartialAnswers: setup.Answers{
			Sensitive: true,
		},
		StartedAt: time.Now(),
	}
	if err := setup.SaveProgress(p); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	if !setup.HasProgress() {
		t.Error("expected HasProgress=true after writing progress file")
	}

	loaded, err := setup.LoadProgress()
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if loaded.Stage != "q2" {
		t.Errorf("expected stage=q2, got %q", loaded.Stage)
	}
	if !loaded.PartialAnswers.Sensitive {
		t.Error("expected PartialAnswers.Sensitive=true")
	}
}

// TestIsYes covers the isYes helper for various user inputs.
func TestIsYes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"O", true},
		{"o", true},
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"oui", true},
		{"n", false},
		{"N", false},
		{"no", false},
		{"non", false},
		{"nope", false},
	}
	for _, tc := range cases {
		if got := isYes(tc.input); got != tc.want {
			t.Errorf("isYes(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestColorHelpers verifies that color functions produce non-empty output.
// Cannot use t.Parallel() because t.Setenv is used.
func TestColorHelpers(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if bold("x") == "" {
		t.Error("bold returned empty string")
	}
	if green("x") == "" {
		t.Error("green returned empty string")
	}
	if yellow("x") == "" {
		t.Error("yellow returned empty string")
	}
	if cyan("x") == "" {
		t.Error("cyan returned empty string")
	}
	if red("x") == "" {
		t.Error("red returned empty string")
	}
}

// TestColorHelpers_NoColor verifies that NO_COLOR disables ANSI sequences.
// Cannot use t.Parallel() because t.Setenv is used.
func TestColorHelpers_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if bold("hello") != "hello" {
		t.Error("expected plain text when NO_COLOR=1")
	}
	if green("hello") != "hello" {
		t.Error("expected plain text when NO_COLOR=1")
	}
}

// TestSetup_Progress_Clear verifies ClearProgress works.
func TestSetup_Progress_Clear(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	p := setup.Progress{Stage: "q1", StartedAt: time.Now()}
	if err := setup.SaveProgress(p); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if err := setup.ClearProgress(); err != nil {
		t.Fatalf("ClearProgress: %v", err)
	}
	if setup.HasProgress() {
		t.Error("expected HasProgress=false after ClearProgress")
	}
}

// TestSetup_Recommend_Offline verifies that Offline=true forces Ollama.
func TestSetup_Recommend_Offline(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{Offline: true})
	if got.Provider != "ollama" {
		t.Errorf("Offline=true → want ollama, got %q", got.Provider)
	}
}

// TestDetectAPIKeys_Env verifies environment variable detection.
func TestDetectAPIKeys_Env(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "gsk-x")
	t.Setenv("OPENROUTER_API_KEY", "")

	keys := setup.DetectAPIKeys()
	if !keys["DEEPSEEK_API_KEY"] {
		t.Error("DEEPSEEK_API_KEY should be detected")
	}
	if keys["ANTHROPIC_API_KEY"] {
		t.Error("ANTHROPIC_API_KEY should not be detected (empty)")
	}
	if !keys["GROQ_API_KEY"] {
		t.Error("GROQ_API_KEY should be detected")
	}
	if keys["OPENROUTER_API_KEY"] {
		t.Error("OPENROUTER_API_KEY should not be detected (empty)")
	}
}

// TestDetectModelsConfigured_NoFile verifies nil return with no config.
func TestDetectModelsConfigured_NoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	providers := setup.DetectModelsConfigured()
	if len(providers) != 0 {
		t.Errorf("expected no providers, got %v", providers)
	}
}

// TestDetectModelsConfigured_WithYAML verifies parsing of a minimal config.
func TestDetectModelsConfigured_WithYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := tmp + "/.config/clue-code"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := `default_model: deepseek-chat
models:
  - id: deepseek-chat
    provider: deepseek
`
	if err := os.WriteFile(dir+"/config.yaml", []byte(yaml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	providers := setup.DetectModelsConfigured()
	if len(providers) == 0 {
		t.Fatal("expected providers from config")
	}
	found := false
	for _, p := range providers {
		if p == "deepseek" {
			found = true
		}
	}
	if !found {
		t.Errorf("deepseek not found in %v", providers)
	}
}
