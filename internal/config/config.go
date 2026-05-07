package config

import (
	"encoding/json"
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

// persistConfig is the JSON structure used for config.json persistence.
// It is separate from Config to allow partial saves without overwriting
// fields managed by other subsystems (e.g. ModelByTier, BudgetUSDPerDay).
type persistConfig struct {
	Mode string `json:"mode"`
}

// ProviderKeys holds API keys read from config.json.
// Keys are set by ConfigureAnthropic / ConfigureDeepSeek in the setup wizard.
// DefaultProvider is "anthropic" or "deepseek" depending on what was last configured.
type ProviderKeys struct {
	AnthropicAPIKey string
	DeepSeekAPIKey  string
	DefaultProvider string
	Mode            Mode
}

// LoadJSONConfig reads config.json at path and returns the persisted provider
// keys and mode. Returns a zero-value ProviderKeys (no error) if the file does
// not exist, so callers degrade gracefully when no wizard has been run.
func LoadJSONConfig(path string) (ProviderKeys, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProviderKeys{}, nil
		}
		return ProviderKeys{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ProviderKeys{}, fmt.Errorf("config: parse %s: %w", path, err)
	}

	pk := ProviderKeys{}
	if v, ok := raw["anthropic_api_key"].(string); ok {
		pk.AnthropicAPIKey = v
	}
	if v, ok := raw["deepseek_api_key"].(string); ok {
		pk.DeepSeekAPIKey = v
	}
	if v, ok := raw["default_provider"].(string); ok {
		pk.DefaultProvider = v
	}
	if v, ok := raw["mode"].(string); ok {
		switch Mode(v) {
		case ModeLocal, ModeCloud, ModeHybrid:
			pk.Mode = Mode(v)
		}
	}
	return pk, nil
}

// SaveMode persists mode to the JSON config file at path.
// The directory is created if it does not exist.
func SaveMode(path string, mode Mode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: create dir %s: %w", dir, err)
	}

	// Read existing JSON to preserve other fields.
	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	existing["mode"] = string(mode)

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}

// LoadMode reads only the mode field from the JSON config file at path.
// Returns ModeHybrid if the file does not exist or has no mode set.
func LoadMode(path string) (Mode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ModeHybrid, nil
		}
		return "", fmt.Errorf("config: read %s: %w", path, err)
	}
	var pc persistConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return ModeHybrid, nil
	}
	if pc.Mode == "" {
		return ModeHybrid, nil
	}
	switch Mode(pc.Mode) {
	case ModeLocal, ModeCloud, ModeHybrid:
		return Mode(pc.Mode), nil
	default:
		return "", fmt.Errorf("config: invalid mode %q in %s", pc.Mode, path)
	}
}

// JSONConfigPath returns the path to the JSON config file used for persistent
// mode storage. It uses os.UserConfigDir() so it respects XDG on Linux and
// ~/Library/Application Support on macOS.
func JSONConfigPath() (string, error) {
	if v := os.Getenv("CLUE_CODE_CONFIG"); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: user config dir: %w", err)
	}
	return filepath.Join(base, "clue-code", "config.json"), nil
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
