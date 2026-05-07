package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/setup"
)

// TestSetup_Recommend_Sensitive verifies that Sensitive=true maps to a local provider.
func TestSetup_Recommend_Sensitive(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{Sensitive: true})
	localProviders := map[string]bool{"ollama": true, "mlx": true}
	if !localProviders[got.Provider] {
		t.Errorf("Sensitive=true → want local provider (ollama/mlx), got %q", got.Provider)
	}
}

// TestSetup_Recommend_CostFirst verifies that PriorityCost=true maps to a
// high-cost-score provider (ollama/mlx are free so they score highest).
func TestSetup_Recommend_CostFirst(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{PriorityCost: true})
	ranked := setup.RankProviders(setup.Answers{PriorityCost: true})
	if len(ranked) == 0 {
		t.Fatal("RankProviders returned empty")
	}
	if got.Provider != ranked[0].Provider {
		t.Errorf("Recommend primary=%q but top ranked=%q", got.Provider, ranked[0].Provider)
	}
	if ranked[0].Cost < 7 {
		t.Errorf("top provider %q Cost=%d, want >=7", ranked[0].Provider, ranked[0].Cost)
	}
}

// TestSetup_Recommend_Quality verifies the default (all false) returns the
// top-ranked provider by quality weighting.
func TestSetup_Recommend_Quality(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{})
	ranked := setup.RankProviders(setup.Answers{})
	if len(ranked) == 0 {
		t.Fatal("RankProviders returned empty")
	}
	if got.Provider != ranked[0].Provider {
		t.Errorf("Recommend primary=%q but top ranked=%q", got.Provider, ranked[0].Provider)
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

// TestSetup_Recommend_Offline verifies that Offline=true results in a local provider.
func TestSetup_Recommend_Offline(t *testing.T) {
	t.Parallel()
	got := setup.Recommend(setup.Answers{Offline: true})
	localProviders := map[string]bool{"ollama": true, "mlx": true}
	if !localProviders[got.Provider] {
		t.Errorf("Offline=true → want local provider (ollama/mlx), got %q", got.Provider)
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

// ---------------------------------------------------------------------------
// confirmYN tests (O7 — strict Y/N validation)
// ---------------------------------------------------------------------------

// confirmYNFromReader is a test-only helper that exercises confirmYN by
// wiring a bytes.Reader as stdin substitute via the prompt function.
// Because prompt reads from os.Stdin directly, we test confirmYN logic
// by invoking it with a context + capturing its return value only — the
// actual stdin substitution is tested via integration paths.
// Here we verify the pure-logic layer via a table-driven approach that
// exercises isYes and the valid/invalid classification.

// TestIsYes_Extended extends the existing isYes tests with additional inputs.
func TestIsYes_Extended(t *testing.T) {
	t.Parallel()
	valid := []string{"O", "o", "y", "Y", "yes", "oui", ""}
	invalid := []string{"n", "N", "no", "non", "0", "x", "abc", "1", "oO"}

	for _, s := range valid {
		if !isYes(s) {
			t.Errorf("isYes(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if isYes(s) {
			t.Errorf("isYes(%q) = true, want false", s)
		}
	}
}

// pipeStdin replaces os.Stdin with a pipe pre-filled with data, resets the
// package-level stdinScanner to read from it, and returns a cleanup func.
// Must NOT be called in parallel tests because it mutates global state.
func pipeStdin(t *testing.T, data string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(data); err != nil {
		r.Close()
		w.Close()
		t.Fatalf("write to pipe: %v", err)
	}
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	// Reinitialize the package-level scanner to read from the new os.Stdin.
	initStdinScanner()
	t.Cleanup(func() {
		r.Close()
		os.Stdin = origStdin
		initStdinScanner()
	})
}

// TestConfirmYN_DefaultTrue verifies that empty input returns defaultVal=true.
func TestConfirmYN_DefaultTrue(t *testing.T) {
	pipeStdin(t, "\n")
	ctx := context.Background()
	got, err := confirmYN(ctx, "test? ", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true for empty input with defaultVal=true")
	}
}

// TestConfirmYN_DefaultFalse verifies that empty input returns defaultVal=false.
func TestConfirmYN_DefaultFalse(t *testing.T) {
	pipeStdin(t, "\n")
	ctx := context.Background()
	got, err := confirmYN(ctx, "test? ", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for empty input with defaultVal=false")
	}
}

// TestConfirmYN_AcceptYes verifies "O" returns true.
func TestConfirmYN_AcceptYes(t *testing.T) {
	pipeStdin(t, "O\n")
	ctx := context.Background()
	got, err := confirmYN(ctx, "test? ", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true for 'O'")
	}
}

// TestConfirmYN_AcceptNo verifies "N" returns false.
func TestConfirmYN_AcceptNo(t *testing.T) {
	pipeStdin(t, "N\n")
	ctx := context.Background()
	got, err := confirmYN(ctx, "test? ", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for 'N'")
	}
}

// TestConfirmYN_RejectThenAccept verifies that invalid input is re-prompted
// and a subsequent valid answer is accepted.
func TestConfirmYN_RejectThenAccept(t *testing.T) {
	pipeStdin(t, "0\ny\n")
	ctx := context.Background()
	got, err := confirmYN(ctx, "test? ", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true after invalid then 'y'")
	}
}

// TestConfirmYN_MaxAttempts verifies that 3 consecutive invalid responses
// return an error.
func TestConfirmYN_MaxAttempts(t *testing.T) {
	pipeStdin(t, "0\nx\nabc\n")
	ctx := context.Background()
	_, err := confirmYN(ctx, "test? ", false)
	if err == nil {
		t.Error("expected error after 3 invalid attempts")
	}
}

// ---------------------------------------------------------------------------
// Recommendation display / multi-criteria (via setup package)
// ---------------------------------------------------------------------------

// TestSetup_ConflictDetected verifies that quality+offline answers produce
// conflicts in the recommendation (not a single imposed choice).
func TestSetup_ConflictDetected(t *testing.T) {
	t.Parallel()
	// This is the user's original problem: quality + offline should conflict.
	a := setup.Answers{
		Sensitive:    false,
		PriorityCost: false, // quality priority
		Offline:      true,  // needs offline
	}
	rec := setup.Recommend(a)
	if len(rec.Conflicts) == 0 {
		t.Error("quality+offline should produce at least one conflict, got 0")
	}
	// Verify the top recommended provider is NOT the lightweight llama3.2.
	if rec.Primary.Provider == "ollama" && rec.Primary.Model == "llama3.2" {
		t.Error("quality+offline conflict: primary should not be llama3.2 (lightweight model)")
	}
}

// TestSetup_AlternativesDisplay verifies that a conflict-free scenario
// produces alternatives for the top-3 display.
func TestSetup_AlternativesDisplay(t *testing.T) {
	t.Parallel()
	// Cost-first, cloud OK, non-sensitive → no conflicts, alternatives expected.
	a := setup.Answers{PriorityCost: true, Offline: false, Sensitive: false}
	rec := setup.Recommend(a)
	if len(rec.Conflicts) != 0 {
		t.Skipf("unexpected conflicts for %+v", a)
	}
	if len(rec.Alternatives) == 0 {
		t.Error("expected at least 1 alternative for conflict-free recommendation")
	}
	if len(rec.Alternatives) > 2 {
		t.Errorf("expected at most 2 alternatives, got %d", len(rec.Alternatives))
	}
}

// TestSetup_ScoringTable_NoError verifies printScoringTable produces output
// without panicking. Not parallel because it redirects os.Stdout.
func TestSetup_ScoringTable_NoError(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w

	printScoringTable(setup.Answers{PriorityCost: false, Sensitive: false, Offline: false})

	// Restore stdout before reading (avoids race with concurrent writes).
	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	if _, readErr := buf.ReadFrom(r); readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	r.Close()

	output := buf.String()
	if !strings.Contains(output, "Provider") {
		t.Errorf("scoring table output missing 'Provider' header; got: %s", output)
	}
}

// TestSetup_Recommend_QualityOffline_NotLlama32 is the canonical regression
// test for the user's original bug report: quality+offline must NOT resolve
// to Ollama llama3.2 (lightweight).
func TestSetup_Recommend_QualityOffline_NotLlama32(t *testing.T) {
	t.Parallel()
	a := setup.Answers{
		Sensitive:    false,
		PriorityCost: false,
		Offline:      true,
	}
	rec := setup.Recommend(a)

	// Either conflicts are detected (correct behavior), OR if primary is ollama
	// it must be a high-quality model, not llama3.2.
	if len(rec.Conflicts) > 0 {
		return // conflict arbitration is the correct path — test passes
	}
	if rec.Primary.Provider == "ollama" && rec.Primary.Model == "llama3.2" {
		t.Errorf("quality+offline: primary must not be llama3.2 (low quality); got %s/%s",
			rec.Primary.Provider, rec.Primary.Model)
	}
}
