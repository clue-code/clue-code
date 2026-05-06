package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config is the runtime configuration loaded from clue-code.yaml + env.
type Config struct {
	// Mode controls model dispatch strategy: local, cloud, or hybrid.
	Mode Mode `yaml:"mode"`

	// ModelByTier maps each routing tier to a model id.
	ModelByTier map[Tier]string `yaml:"model_by_tier"`

	// AgentsDir is the absolute path to the directory holding agent definitions.
	// Defaults to "agents/" relative to the binary.
	AgentsDir string `yaml:"agents_dir"`

	// SkillsDir is the absolute path to the directory holding skill definitions.
	SkillsDir string `yaml:"skills_dir"`

	// HooksDir is the absolute path to the directory holding hooks.
	HooksDir string `yaml:"hooks_dir"`

	// Telemetry, when true, enables anonymous usage metrics.
	Telemetry bool `yaml:"telemetry"`

	// BudgetUSDPerDay is the maximum USD spend allowed per calendar day across
	// all model calls. 0 disables the budget check. Default: 5.0.
	BudgetUSDPerDay float64 `json:"budget_usd_per_day" yaml:"budget_usd_per_day"`

	// TokensEngineEnabled activates the token counter/cache/budget middleware in
	// the model HTTP layer. Defaults to true; set to false for rollback.
	TokensEngineEnabled bool `json:"tokens_engine_enabled" yaml:"tokens_engine_enabled"`
}

// ConfigPath returns the standard config file location.
// Order of resolution:
//  1. CLUE_CODE_CONFIG environment variable (absolute path)
//  2. $XDG_CONFIG_HOME/clue-code/clue-code.yaml
//  3. $HOME/.config/clue-code/clue-code.yaml
func ConfigPath() (string, error) {
	if v := os.Getenv("CLUE_CODE_CONFIG"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "clue-code", "clue-code.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "clue-code", "clue-code.yaml"), nil
}

// Load returns a Config initialized from defaults, then overridden by env vars.
//
// File-based loading (YAML parsing) is deliberately deferred to a later phase
// to keep the MVP free of third-party dependencies. The CLUE_CODE_MODE env var
// is honored as a minimal override mechanism.
func Load() *Config {
	c := Defaults()
	if m := os.Getenv("CLUE_CODE_MODE"); m != "" {
		switch Mode(m) {
		case ModeLocal, ModeCloud, ModeHybrid:
			c.Mode = Mode(m)
		}
	}
	return c
}

// Validate returns an error if c contains invalid values.
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeLocal, ModeCloud, ModeHybrid:
	default:
		return fmt.Errorf("invalid mode %q (expected local, cloud, or hybrid)", c.Mode)
	}
	if len(c.ModelByTier) == 0 {
		return fmt.Errorf("model_by_tier must define at least one tier")
	}
	return nil
}
