package setup

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// TestRecommend_All8Combos exercises all 8 three-boolean combinations and
// asserts the correct provider category is chosen for each.
// With multi-criteria scoring, specific provider expectations depend on
// platform (MLX filtered on non-darwin/arm64). The test asserts the
// correct category (local vs cloud) and that required fields are populated.
func TestRecommend_All8Combos(t *testing.T) {
	t.Parallel()

	localProviders := map[string]bool{"ollama": true, "mlx": true}
	cloudProviders := map[string]bool{"deepseek": true, "anthropic": true, "groq": true, "openrouter": true}

	cases := []struct {
		name      string
		a         Answers
		wantLocal bool // true = expect local provider, false = expect cloud
	}{
		// Any local constraint → local provider
		{"sensitive_only", Answers{Sensitive: true, HasMacM: false}, true},
		{"sensitive_and_cost", Answers{Sensitive: true, PriorityCost: true, HasMacM: false}, true},
		{"offline_only", Answers{Offline: true, HasMacM: false}, true},
		{"sensitive_and_offline", Answers{Sensitive: true, Offline: true, HasMacM: false}, true},
		{"cost_and_offline", Answers{PriorityCost: true, Offline: true, HasMacM: false}, true},
		{"all_true", Answers{Sensitive: true, PriorityCost: true, Offline: true, HasMacM: false}, true},
		// No-constraint scenarios: with weight=0 for non-prioritized dimensions,
		// only the prioritized dimension matters.
		// cost_first: Cost weight=5, Ollama Cost=10 → score 50 (wins over DeepSeek 45).
		// quality_default: Quality weight=5, Anthropic Quality=10 → score 50 (wins over Ollama 8×5=40).
		{"cost_first_no_constraints", Answers{PriorityCost: true, HasMacM: false}, true},
		{"quality_default", Answers{HasMacM: false}, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Recommend(tc.a)
			if tc.wantLocal && !localProviders[got.Provider] {
				t.Errorf("Recommend(%+v).Provider = %q, want local provider (ollama/mlx)", tc.a, got.Provider)
			}
			if !tc.wantLocal && cloudProviders[got.Provider] == false {
				t.Errorf("Recommend(%+v).Provider = %q, want cloud provider", tc.a, got.Provider)
			}
			if got.Justification == "" {
				t.Errorf("Recommend(%+v).Justification must not be empty", tc.a)
			}
			if len(got.Steps) == 0 {
				t.Errorf("Recommend(%+v).Steps must not be empty", tc.a)
			}
		})
	}
}

// TestRecommend_MLX_AppleSilicon checks that MLX is surfaced (in top results)
// for a privacy-conscious user on Apple Silicon who does not prioritise cost.
// With multi-criteria scoring, MLX competes with ollama; both are local/private.
// The test asserts a local provider is chosen (mlx or ollama).
func TestRecommend_MLX_AppleSilicon(t *testing.T) {
	t.Parallel()
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("MLX only available on darwin/arm64")
	}
	a := Answers{
		Sensitive:    true,
		PriorityCost: false,
		Offline:      false,
		HasMacM:      true,
	}
	got := Recommend(a)
	localProviders := map[string]bool{"mlx": true, "ollama": true}
	if !localProviders[got.Provider] {
		t.Errorf("expected local provider (mlx/ollama) for Apple Silicon privacy user, got %q", got.Provider)
	}
	// Verify MLX appears in ranked results.
	ranked := RankProviders(a)
	foundMLX := false
	for _, p := range ranked {
		if p.Provider == "mlx" {
			foundMLX = true
			break
		}
	}
	if !foundMLX {
		t.Error("MLX should be present in ranking on Apple Silicon")
	}
}

// TestRecommend_Sensitive verifies that Sensitive=true results in a local provider.
func TestRecommend_Sensitive(t *testing.T) {
	t.Parallel()
	got := Recommend(Answers{Sensitive: true})
	localProviders := map[string]bool{"ollama": true, "mlx": true}
	if !localProviders[got.Provider] {
		t.Errorf("Sensitive=true → want local provider (ollama/mlx), got %q", got.Provider)
	}
}

// TestRecommend_CostFirst verifies that PriorityCost=true results in a
// high-cost-score provider. On non-Apple-Silicon the top scorer is deepseek;
// on Apple Silicon local free providers dominate due to Cost=10.
func TestRecommend_CostFirst(t *testing.T) {
	t.Parallel()
	got := Recommend(Answers{PriorityCost: true})
	// Cost=10 local providers outscore cloud providers; any provider with
	// high cost score (Cost >= 7) is acceptable.
	ranked := RankProviders(Answers{PriorityCost: true})
	if len(ranked) == 0 {
		t.Fatal("RankProviders returned empty")
	}
	top := ranked[0]
	if top.Cost < 7 {
		t.Errorf("PriorityCost=true: top provider %q has Cost=%d, want >=7", top.Provider, top.Cost)
	}
	if got.Provider != top.Provider {
		t.Errorf("Recommend primary=%q but RankProviders top=%q", got.Provider, top.Provider)
	}
}

// TestRecommend_Quality verifies that the default (all false) returns a
// provider with high quality score.
func TestRecommend_Quality(t *testing.T) {
	t.Parallel()
	got := Recommend(Answers{})
	ranked := RankProviders(Answers{})
	if len(ranked) == 0 {
		t.Fatal("RankProviders returned empty")
	}
	top := ranked[0]
	if top.Quality < 5 {
		t.Errorf("default answers: top provider %q has Quality=%d, want >=5", top.Provider, top.Quality)
	}
	if got.Provider != top.Provider {
		t.Errorf("Recommend primary=%q but RankProviders top=%q", got.Provider, top.Provider)
	}
}

// TestDetectAPIKeys_FromEnv verifies that DetectAPIKeys reads from environment.
func TestDetectAPIKeys_FromEnv(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-test-123")
	t.Setenv("ANTHROPIC_API_KEY", "")

	keys := DetectAPIKeys()
	if !keys["DEEPSEEK_API_KEY"] {
		t.Error("expected DEEPSEEK_API_KEY detected=true after t.Setenv")
	}
	if keys["ANTHROPIC_API_KEY"] {
		t.Error("expected ANTHROPIC_API_KEY detected=false when empty")
	}
}

// TestDetectAPIKeys_Empty verifies that unset keys are false.
func TestDetectAPIKeys_Empty(t *testing.T) {
	// Clear all known keys.
	for _, k := range []string{
		"DEEPSEEK_API_KEY", "ANTHROPIC_API_KEY",
		"GROQ_API_KEY", "OPENROUTER_API_KEY",
	} {
		t.Setenv(k, "")
	}
	keys := DetectAPIKeys()
	for k, v := range keys {
		if v {
			t.Errorf("expected %s=false after clearing env, got true", k)
		}
	}
}

// TestDetectOllama_NotInstalled verifies that DetectOllama returns
// installed=false when the PATH is cleared (no ollama binary available).
func TestDetectOllama_NotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	installed, _, _ := DetectOllama()
	if installed {
		t.Error("expected installed=false with empty PATH")
	}
}

// TestDetectMLX_AppleOnly skips on non-arm64/darwin platforms to satisfy
// the requirement that MLX detection be Apple Silicon only.
func TestDetectMLX_AppleOnly(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("MLX detection only supported on arm64 darwin")
	}
	// On Apple Silicon, the function must return without panicking.
	installed, version := DetectMLX()
	t.Logf("DetectMLX: installed=%v version=%q", installed, version)
}

// TestDetectMLX_NonApple ensures DetectMLX returns false on non-Apple platforms.
func TestDetectMLX_NonApple(t *testing.T) {
	if runtime.GOARCH == "arm64" && runtime.GOOS == "darwin" {
		t.Skip("running on Apple Silicon, skipping non-Apple test")
	}
	installed, version := DetectMLX()
	if installed {
		t.Errorf("expected installed=false on %s/%s, got true", runtime.GOOS, runtime.GOARCH)
	}
	if version != "" {
		t.Errorf("expected empty version on non-Apple, got %q", version)
	}
}

// TestProgress_RoundTrip verifies that SaveProgress and LoadProgress are
// inverses of each other.
func TestProgress_RoundTrip(t *testing.T) {
	// Use a temporary home directory to avoid polluting the real one.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := Progress{
		Stage:    "questions",
		Provider: "deepseek",
		PartialAnswers: Answers{
			Sensitive:    false,
			PriorityCost: true,
			Offline:      false,
		},
		StartedAt: time.Now().Truncate(time.Second),
	}

	if err := SaveProgress(want); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	got, err := LoadProgress()
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
		t.Errorf("PartialAnswers: got %+v, want %+v", got.PartialAnswers, want.PartialAnswers)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, want.StartedAt)
	}
}

// TestProgress_Clear verifies that ClearProgress removes the file and that
// subsequent LoadProgress returns an error.
func TestProgress_Clear(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	p := Progress{Stage: "test", StartedAt: time.Now()}
	if err := SaveProgress(p); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if !HasProgress() {
		t.Fatal("expected HasProgress=true after save")
	}
	if err := ClearProgress(); err != nil {
		t.Fatalf("ClearProgress: %v", err)
	}
	if HasProgress() {
		t.Error("expected HasProgress=false after clear")
	}
	if _, err := LoadProgress(); err == nil {
		t.Error("expected LoadProgress to fail after clear")
	}
}

// TestProgress_HasProgress verifies the HasProgress helper.
func TestProgress_HasProgress(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if HasProgress() {
		t.Error("expected HasProgress=false in fresh temp dir")
	}

	p := Progress{Stage: "fresh", StartedAt: time.Now()}
	if err := SaveProgress(p); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if !HasProgress() {
		t.Error("expected HasProgress=true after SaveProgress")
	}
}

// TestClearProgress_NoFile verifies that ClearProgress is idempotent when
// no file exists.
func TestClearProgress_NoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := ClearProgress(); err != nil {
		t.Errorf("ClearProgress on missing file: %v", err)
	}
}

// TestRecommend_NoEmptyFields ensures every path through Recommend fills all
// required fields.
func TestRecommend_NoEmptyFields(t *testing.T) {
	t.Parallel()
	combos := []Answers{
		{},
		{Sensitive: true},
		{PriorityCost: true},
		{Offline: true},
		{HasMacM: true, Sensitive: true},
	}
	for _, a := range combos {
		r := Recommend(a)
		if r.Provider == "" {
			t.Errorf("Recommend(%+v).Provider is empty", a)
		}
		if r.Model == "" {
			t.Errorf("Recommend(%+v).Model is empty", a)
		}
		if r.Justification == "" {
			t.Errorf("Recommend(%+v).Justification is empty", a)
		}
		if r.Cost == "" {
			t.Errorf("Recommend(%+v).Cost is empty", a)
		}
		if len(r.Steps) == 0 {
			t.Errorf("Recommend(%+v).Steps is empty", a)
		}
	}
}

// TestDetectModelsConfigured_Empty returns nil for a non-existent config.
func TestDetectModelsConfigured_Empty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	providers := DetectModelsConfigured()
	if len(providers) != 0 {
		t.Errorf("expected empty providers, got %v", providers)
	}
}

// TestDetectModelsConfigured_WithFile parses a minimal YAML-like config.
func TestDetectModelsConfigured_WithFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := tmp + "/.config/clue-code"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `default_model: deepseek-chat
models:
  - id: deepseek-chat
    provider: deepseek
    api_key_env: DEEPSEEK_API_KEY
`
	if err := os.WriteFile(dir+"/config.yaml", []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	providers := DetectModelsConfigured()
	if len(providers) == 0 {
		t.Error("expected at least one provider from config file")
	}
	found := false
	for _, p := range providers {
		if p == "deepseek" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'deepseek' in providers, got %v", providers)
	}
}
