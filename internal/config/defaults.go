// Package config holds the runtime configuration for CLUE CODE.
package config

// Mode represents how CLUE CODE dispatches model calls.
type Mode string

const (
	// ModeLocal: 100% on-device inference, no network calls.
	ModeLocal Mode = "local"
	// ModeCloud: 100% cloud APIs (DeepSeek, Groq, OpenRouter, ...).
	ModeCloud Mode = "cloud"
	// ModeHybrid: smart routing, local + cloud (default).
	ModeHybrid Mode = "hybrid"
)

// Tier represents the routing tier for a task.
type Tier string

const (
	// TierL0: fast lookup tasks (Read/Glob/completion).
	TierL0 Tier = "L0"
	// TierL1: standard edits and refactors.
	TierL1 Tier = "L1"
	// TierL2: cloud-grade architecture and complex multi-file changes.
	TierL2 Tier = "L2"
	// TierL3: critical/security decisions, MoA aggregation.
	TierL3 Tier = "L3"
)

// ModelDefaults maps each tier to a recommended model id.
// These are sensible defaults for an Apple Silicon machine with 32GB+ RAM.
// Users can override per-agent in agents/*.md frontmatter or in clue-code.yaml.
var ModelDefaults = map[Tier]string{
	TierL0: "qwen3-coder:7b",
	TierL1: "qwen3-coder:30b",
	TierL2: "deepseek-v3.2",
	TierL3: "moa:r1+v3.2+qwen3",
}

// Defaults returns a Config populated with safe defaults.
func Defaults() *Config {
	return &Config{
		Mode:                ModeLocal,
		ModelByTier:         copyModelDefaults(),
		Telemetry:           false,
		BudgetUSDPerDay:     5.0,
		TokensEngineEnabled: true,
	}
}

func copyModelDefaults() map[Tier]string {
	out := make(map[Tier]string, len(ModelDefaults))
	for k, v := range ModelDefaults {
		out[k] = v
	}
	return out
}
