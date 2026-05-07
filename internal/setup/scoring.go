// Package setup implements the interactive setup wizard for CLUE CODE.
package setup

import (
	"runtime"
	"sort"
)

// Dimension names the four scoring axes.
type Dimension string

const (
	DimPrivacy Dimension = "privacy"
	DimCost    Dimension = "cost"
	DimQuality Dimension = "quality"
	DimOffline Dimension = "offline"
)

// ProviderScore holds static scoring data for a provider/model pair.
// All dimension scores are in the range 0–10.
type ProviderScore struct {
	Provider    string  // "ollama", "deepseek", "anthropic", "groq", "openrouter", "mlx"
	Model       string  // specific model name
	Privacy     int     // 0-10: 10 = fully local, 4 = cloud with privacy policy
	Cost        int     // 0-10: 10 = free, lower = expensive
	Quality     int     // 0-10: 10 = best-in-class reasoning
	Offline     int     // 0-10: 10 = fully offline capable, 0 = cloud-only
	CostUSD1M   float64 // USD per 1M input tokens (0 for local)
	SizeGB      float64 // model size on disk in GB (0 for cloud)
	Description string  // short human-readable description
}

// ProviderTable is the compile-time source of truth for all supported providers.
// Adding a new provider requires an explicit entry here.
var ProviderTable = []ProviderScore{
	{
		Provider: "ollama", Model: "llama3.2",
		Privacy: 10, Cost: 10, Quality: 5, Offline: 10,
		CostUSD1M: 0, SizeGB: 2.0,
		Description: "Modele leger 2GB, qualite moyenne",
	},
	{
		Provider: "ollama", Model: "qwen2.5-coder:32b",
		Privacy: 10, Cost: 10, Quality: 8, Offline: 10,
		CostUSD1M: 0, SizeGB: 19,
		Description: "Modele code 19GB, qualite excellente",
	},
	{
		Provider: "ollama", Model: "deepseek-r1:7b",
		Privacy: 10, Cost: 10, Quality: 7, Offline: 10,
		CostUSD1M: 0, SizeGB: 4.7,
		Description: "Modele raisonnement 4.7GB",
	},
	{
		Provider: "mlx", Model: "Llama-3.2-3B",
		Privacy: 10, Cost: 10, Quality: 6, Offline: 10,
		CostUSD1M: 0, SizeGB: 2.5,
		Description: "Optimise Apple Silicon",
	},
	{
		Provider: "deepseek", Model: "deepseek-chat",
		Privacy: 4, Cost: 9, Quality: 8, Offline: 0,
		CostUSD1M:   0.28,
		Description: "Cloud, 53x moins cher que Claude",
	},
	{
		Provider: "anthropic", Model: "claude-sonnet-4-6",
		Privacy: 4, Cost: 2, Quality: 10, Offline: 0,
		CostUSD1M:   15,
		Description: "Top qualite, cher",
	},
	{
		Provider: "groq", Model: "llama-3.3-70b",
		Privacy: 4, Cost: 7, Quality: 8, Offline: 0,
		CostUSD1M:   0.59,
		Description: "Ultra-rapide cloud",
	},
	{
		Provider: "openrouter", Model: "various",
		Privacy: 4, Cost: 6, Quality: 9, Offline: 0,
		CostUSD1M:   1.5,
		Description: "Acces 100+ modeles",
	},
}

// Weights holds the per-dimension multipliers derived from user answers.
type Weights struct {
	Privacy float64
	Cost    float64
	Quality float64
	Offline float64
}

// WeightsFromAnswers builds dimension weights from the user's wizard answers.
// A dimension gets weight 3 if it is a user priority, 1 otherwise.
func WeightsFromAnswers(a Answers) Weights {
	w := Weights{Privacy: 1, Cost: 1, Quality: 1, Offline: 1}
	if a.Sensitive {
		w.Privacy = 3
	}
	if a.PriorityCost {
		w.Cost = 3
	} else {
		// not cost-first means quality-first
		w.Quality = 3
	}
	if a.Offline {
		w.Offline = 3
	}
	return w
}

// ScoreProvider computes the weighted total score for a single provider.
func ScoreProvider(p ProviderScore, w Weights) float64 {
	return float64(p.Privacy)*w.Privacy +
		float64(p.Cost)*w.Cost +
		float64(p.Quality)*w.Quality +
		float64(p.Offline)*w.Offline
}

// filterOut returns a copy of ps with all entries matching provider removed.
func filterOut(ps []ProviderScore, provider string) []ProviderScore {
	out := ps[:0:0]
	for _, p := range ps {
		if p.Provider != provider {
			out = append(out, p)
		}
	}
	return out
}

// RankProviders returns ProviderTable sorted descending by weighted score.
// MLX entries are removed when not running on darwin/arm64.
// The sort is stable so ties preserve declaration order (deterministic for tests).
func RankProviders(a Answers) []ProviderScore {
	w := WeightsFromAnswers(a)
	ranked := make([]ProviderScore, len(ProviderTable))
	copy(ranked, ProviderTable)

	// Remove MLX when not on Apple Silicon.
	if !(runtime.GOARCH == "arm64" && runtime.GOOS == "darwin") {
		ranked = filterOut(ranked, "mlx")
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ScoreProvider(ranked[i], w) > ScoreProvider(ranked[j], w)
	})
	return ranked
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
