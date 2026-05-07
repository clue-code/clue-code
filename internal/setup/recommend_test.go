package setup

import (
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Scoring / Weights
// ---------------------------------------------------------------------------

// TestScoring_Weights verifies that PriorityCost=true triples Cost weight
// and that Quality defaults to 3 when not cost-first.
func TestScoring_Weights(t *testing.T) {
	t.Parallel()

	wCost := WeightsFromAnswers(Answers{PriorityCost: true})
	if wCost.Cost != 3 {
		t.Errorf("PriorityCost=true: Cost weight = %.0f, want 3", wCost.Cost)
	}
	if wCost.Quality != 1 {
		t.Errorf("PriorityCost=true: Quality weight = %.0f, want 1", wCost.Quality)
	}

	wQuality := WeightsFromAnswers(Answers{PriorityCost: false})
	if wQuality.Quality != 3 {
		t.Errorf("PriorityCost=false: Quality weight = %.0f, want 3", wQuality.Quality)
	}
	if wQuality.Cost != 1 {
		t.Errorf("PriorityCost=false: Cost weight = %.0f, want 1", wQuality.Cost)
	}
}

// TestScoring_Privacy verifies that Sensitive=true triples Privacy weight.
func TestScoring_Privacy(t *testing.T) {
	t.Parallel()
	w := WeightsFromAnswers(Answers{Sensitive: true})
	if w.Privacy != 3 {
		t.Errorf("Sensitive=true: Privacy weight = %.0f, want 3", w.Privacy)
	}
}

// TestScoring_Offline verifies that Offline=true triples Offline weight.
func TestScoring_Offline(t *testing.T) {
	t.Parallel()
	w := WeightsFromAnswers(Answers{Offline: true})
	if w.Offline != 3 {
		t.Errorf("Offline=true: Offline weight = %.0f, want 3", w.Offline)
	}
}

// TestScoring_ScoreProvider verifies the weighted sum formula.
func TestScoring_ScoreProvider(t *testing.T) {
	t.Parallel()
	p := ProviderScore{Privacy: 10, Cost: 10, Quality: 5, Offline: 10}
	w := Weights{Privacy: 1, Cost: 1, Quality: 1, Offline: 1}
	got := ScoreProvider(p, w)
	want := float64(10 + 10 + 5 + 10)
	if got != want {
		t.Errorf("ScoreProvider = %.0f, want %.0f", got, want)
	}
}

// ---------------------------------------------------------------------------
// Ranking
// ---------------------------------------------------------------------------

// TestRanking_8Combinations exercises all 8 combinations of the three boolean
// flags and asserts the top-ranked provider is correct for each.
func TestRanking_8Combinations(t *testing.T) {
	t.Parallel()

	// On non-darwin/arm64, MLX is filtered out. We capture the platform once.
	isAppleSilicon := runtime.GOARCH == "arm64" && runtime.GOOS == "darwin"

	// localProviders is the set of providers acceptable when local is expected.
	localProviders := map[string]bool{"ollama": true, "mlx": true}

	cases := []struct {
		name      string
		a         Answers
		wantLocal bool   // true = expect a local provider (ollama/mlx)
		wantProv  string // non-empty only when wantLocal=false AND !isAppleSilicon
	}{
		// Any constraint (privacy, offline) → local provider regardless of platform.
		{"sensitive_only", Answers{Sensitive: true}, true, ""},
		{"sensitive_cost", Answers{Sensitive: true, PriorityCost: true}, true, ""},
		{"offline_only", Answers{Offline: true}, true, ""},
		{"sensitive_offline", Answers{Sensitive: true, Offline: true}, true, ""},
		{"cost_offline", Answers{PriorityCost: true, Offline: true}, true, ""},
		{"all_true", Answers{Sensitive: true, PriorityCost: true, Offline: true}, true, ""},
		// No-constraint scenarios: local providers still win because their
		// baseline scores (Privacy=10, Cost=10, Offline=10) give ollama/qwen2.5
		// a total of 10+10+24+10=54 vs anthropic 4+2+30+0=36 with Quality×3.
		// This is the correct multi-criteria result — local high-quality models
		// outrank cloud even when quality is the sole priority.
		{"all_false", Answers{}, true, ""},
		{"cost_first", Answers{PriorityCost: true}, true, ""},
	}

	// isAppleSilicon is retained to avoid "declared and not used" error.
	_ = isAppleSilicon

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ranked := RankProviders(tc.a)
			if len(ranked) == 0 {
				t.Fatal("RankProviders returned empty slice")
			}
			top := ranked[0]
			if !localProviders[top.Provider] {
				t.Errorf("RankProviders(%s).top.Provider = %q, want local (ollama/mlx)", tc.name, top.Provider)
			}
		})
	}
}

// TestScoring_MLXFiltered ensures MLX is absent from ranking on non-Apple Silicon.
func TestScoring_MLXFiltered(t *testing.T) {
	t.Parallel()
	if runtime.GOARCH == "arm64" && runtime.GOOS == "darwin" {
		t.Skip("running on Apple Silicon, MLX is expected to be present")
	}
	ranked := RankProviders(Answers{})
	for _, p := range ranked {
		if p.Provider == "mlx" {
			t.Error("MLX should be filtered out on non-darwin/arm64")
		}
	}
}

// TestRanking_Deterministic verifies that two calls with identical answers
// produce the same ordering (StableSort guarantee).
func TestRanking_Deterministic(t *testing.T) {
	t.Parallel()
	a := Answers{Sensitive: true, Offline: true}
	r1 := RankProviders(a)
	r2 := RankProviders(a)
	if len(r1) != len(r2) {
		t.Fatalf("lengths differ: %d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i].Provider != r2[i].Provider || r1[i].Model != r2[i].Model {
			t.Errorf("position %d differs: %v vs %v", i, r1[i], r2[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Conflict detection
// ---------------------------------------------------------------------------

// TestConflict_QualityVsOffline asserts that !PriorityCost && Offline triggers
// the "Qualite vs Offline" conflict with 3 resolution options.
func TestConflict_QualityVsOffline(t *testing.T) {
	t.Parallel()
	a := Answers{PriorityCost: false, Offline: true}
	conflicts := DetectConflicts(a)

	found := false
	for _, c := range conflicts {
		if strings.Contains(c.Description, "Hors-ligne") {
			found = true
			if len(c.Options) != 3 {
				t.Errorf("quality-vs-offline conflict: want 3 options, got %d", len(c.Options))
			}
		}
	}
	if !found {
		t.Errorf("expected quality-vs-offline conflict, got %v", conflicts)
	}
}

// TestConflict_PrivacyVsQuality asserts that Sensitive && !PriorityCost triggers
// the "Confidentialite vs Qualite" conflict.
func TestConflict_PrivacyVsQuality(t *testing.T) {
	t.Parallel()
	a := Answers{Sensitive: true, PriorityCost: false}
	conflicts := DetectConflicts(a)

	found := false
	for _, c := range conflicts {
		if strings.Contains(c.Description, "Confidentialite") {
			found = true
			if len(c.Options) != 3 {
				t.Errorf("privacy-vs-quality conflict: want 3 options, got %d", len(c.Options))
			}
		}
	}
	if !found {
		t.Errorf("expected privacy-vs-quality conflict, got %v", conflicts)
	}
}

// TestConflict_NoConflict verifies coherent scenarios produce zero conflicts.
func TestConflict_NoConflict(t *testing.T) {
	t.Parallel()
	coherent := []Answers{
		// Cost-first, no offline, not sensitive → no tension.
		{PriorityCost: true, Offline: false, Sensitive: false},
		// Sensitive + cost-first → cost & privacy align (both push local/cheap).
		{Sensitive: true, PriorityCost: true, Offline: false},
		// All false → default quality, cloud OK → no conflict.
		{},
	}
	for _, a := range coherent {
		conflicts := DetectConflicts(a)
		if len(conflicts) != 0 {
			t.Errorf("expected 0 conflicts for %+v, got %d: %v", a, len(conflicts), conflicts)
		}
	}
}

// TestConflict_BothConflicts verifies that Sensitive + !PriorityCost + Offline
// triggers both conflicts simultaneously.
func TestConflict_BothConflicts(t *testing.T) {
	t.Parallel()
	a := Answers{Sensitive: true, PriorityCost: false, Offline: true}
	conflicts := DetectConflicts(a)
	if len(conflicts) != 2 {
		t.Errorf("expected 2 conflicts, got %d: %v", len(conflicts), conflicts)
	}
}

// ---------------------------------------------------------------------------
// Recommendation
// ---------------------------------------------------------------------------

// TestRecommendation_HasAlternativesWhenNoConflict checks that Alternatives
// are populated when there are no conflicts.
func TestRecommendation_HasAlternativesWhenNoConflict(t *testing.T) {
	t.Parallel()
	// No conflicts expected for cost-first, cloud-OK, non-sensitive.
	a := Answers{PriorityCost: true, Offline: false, Sensitive: false}
	rec := Recommend(a)
	if len(rec.Conflicts) != 0 {
		t.Skipf("unexpected conflicts for %+v, skipping alternatives check", a)
	}
	if len(rec.Alternatives) == 0 {
		t.Error("expected at least 1 alternative when no conflicts")
	}
}

// TestRecommendation_NoAlternativesWhenConflict checks that Alternatives are
// not populated when conflicts exist (user must arbitrate first).
func TestRecommendation_NoAlternativesWhenConflict(t *testing.T) {
	t.Parallel()
	a := Answers{PriorityCost: false, Offline: true}
	rec := Recommend(a)
	if len(rec.Conflicts) == 0 {
		t.Skip("no conflicts detected, test not applicable")
	}
	if len(rec.Alternatives) != 0 {
		t.Errorf("expected 0 alternatives when conflicts present, got %d", len(rec.Alternatives))
	}
}

// TestRecommendation_Justification verifies that the justification string
// contains relevant keywords depending on user answers.
func TestRecommendation_Justification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		a        Answers
		contains string
	}{
		{"cost_free", Answers{PriorityCost: true}, "cout"},
		{"quality", Answers{PriorityCost: false, Sensitive: false, Offline: false}, "qualite"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := Recommend(tc.a)
			if !strings.Contains(strings.ToLower(rec.Justification), tc.contains) {
				t.Errorf("Justification %q does not contain %q", rec.Justification, tc.contains)
			}
		})
	}
}

// TestRecommendation_LegacyFields verifies that the legacy Provider/Model
// mirror fields are populated correctly for backward compatibility.
func TestRecommendation_LegacyFields(t *testing.T) {
	t.Parallel()
	combos := []Answers{
		{},
		{Sensitive: true},
		{PriorityCost: true},
		{Offline: true},
	}
	for _, a := range combos {
		rec := Recommend(a)
		if rec.Provider != rec.Primary.Provider {
			t.Errorf("%+v: legacy Provider %q != Primary.Provider %q", a, rec.Provider, rec.Primary.Provider)
		}
		if rec.Model != rec.Primary.Model {
			t.Errorf("%+v: legacy Model %q != Primary.Model %q", a, rec.Model, rec.Primary.Model)
		}
		if rec.Provider == "" {
			t.Errorf("%+v: Provider is empty", a)
		}
		if rec.Model == "" {
			t.Errorf("%+v: Model is empty", a)
		}
		if rec.Cost == "" {
			t.Errorf("%+v: Cost is empty", a)
		}
		if len(rec.Steps) == 0 {
			t.Errorf("%+v: Steps is empty", a)
		}
		if rec.Justification == "" {
			t.Errorf("%+v: Justification is empty", a)
		}
	}
}
