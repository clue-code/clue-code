package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/clue-code/clue-code/internal/config"
	"gopkg.in/yaml.v3"
)

// ModelConfig describes a single model entry in config.yaml.
type ModelConfig struct {
	ID        string `yaml:"id"`
	Provider  string `yaml:"provider"`
	Endpoint  string `yaml:"endpoint,omitempty"`
	APIKeyEnv string `yaml:"api_key_env,omitempty"`
	MaxTokens int    `yaml:"max_tokens,omitempty"`
	Bin       string `yaml:"bin,omitempty"` // path to mlx_lm binary for local subprocess
}

// Config is the top-level structure for ~/.config/clue-code/config.yaml.
type Config struct {
	DefaultModel    string        `yaml:"default_model"`
	BudgetUSDPerDay float64       `yaml:"budget_usd_per_day,omitempty"`
	Models          []ModelConfig `yaml:"models"`
}

// defaultConfig returns a minimal usable config when no file exists.
func defaultConfig() *Config {
	return &Config{
		DefaultModel: "deepseek-chat",
		Models: []ModelConfig{
			{
				ID:        "deepseek-chat",
				Provider:  "deepseek",
				Endpoint:  "https://api.deepseek.com/v1/chat/completions",
				APIKeyEnv: "DEEPSEEK_API_KEY",
				MaxTokens: 8192,
			},
		},
	}
}

// configPath returns the canonical path to the config file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("model: home dir: %w", err)
	}
	return filepath.Join(home, ".config", "clue-code", "config.yaml"), nil
}

// LoadConfig reads ~/.config/clue-code/config.yaml.
// If the file does not exist, a minimal default config is returned.
// It also merges API keys persisted in config.json by the setup wizard so that
// users who ran 'clue-code setup' do not need to export env vars manually.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := defaultConfig()
			mergeJSONConfigKeys(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("model: read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("model: parse config: %w", err)
	}

	if cfg.DefaultModel == "" && len(cfg.Models) > 0 {
		cfg.DefaultModel = cfg.Models[0].ID
	}

	mergeJSONConfigKeys(&cfg)
	return &cfg, nil
}

// mergeJSONConfigKeys reads config.json and injects persisted API keys into
// the Config so that factory.NewClient can use them without requiring env vars.
// Priority follows the 12-factor app convention: env var > config.json > error.
// An environment variable that is already set is never overwritten.
func mergeJSONConfigKeys(cfg *Config) {
	jsonPath, err := config.JSONConfigPath()
	if err != nil {
		return // non-fatal: env-only users still work
	}
	pk, err := config.LoadJSONConfig(jsonPath)
	if err != nil {
		return // non-fatal
	}

	// Map provider name → key from config.json
	jsonKeys := map[string]string{}
	if pk.AnthropicAPIKey != "" {
		jsonKeys["anthropic"] = pk.AnthropicAPIKey
	}
	if pk.DeepSeekAPIKey != "" {
		jsonKeys["deepseek"] = pk.DeepSeekAPIKey
	}

	// Inject anthropic model if wizard set default_provider=anthropic but no
	// anthropic model exists in config.yaml yet (covers fresh install scenario).
	if pk.DefaultProvider == "anthropic" && pk.AnthropicAPIKey != "" {
		hasAnthropic := false
		for i := range cfg.Models {
			if cfg.Models[i].Provider == "anthropic" {
				hasAnthropic = true
				break
			}
		}
		if !hasAnthropic {
			cfg.Models = append(cfg.Models, ModelConfig{
				ID:        "anthropic/claude-sonnet-4-5",
				Provider:  "anthropic",
				Endpoint:  "https://api.anthropic.com/v1",
				MaxTokens: 8192,
			})
			cfg.DefaultModel = "anthropic/claude-sonnet-4-5"
		}
	}

	// For each model entry: if its provider has a key in config.json,
	// store that key via a synthetic env var so NewClient can pick it up.
	// We use os.Setenv only if the var is currently empty (env wins if set).
	for i := range cfg.Models {
		mc := &cfg.Models[i]
		key, hasKey := jsonKeys[mc.Provider]
		if !hasKey || key == "" {
			continue
		}
		// If no APIKeyEnv is set on this entry, create a synthetic one.
		if mc.APIKeyEnv == "" {
			syntheticEnv := "CLUE_CODE_" + upperProvider(mc.Provider) + "_API_KEY"
			mc.APIKeyEnv = syntheticEnv
		}
		// Only inject if current env value is empty (preserves user's exported var).
		if os.Getenv(mc.APIKeyEnv) == "" {
			_ = os.Setenv(mc.APIKeyEnv, key)
		}
	}
}

// upperProvider returns a simple uppercase version of a provider name for use
// in synthetic env var names (e.g. "anthropic" → "ANTHROPIC").
func upperProvider(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		b[i] = c
	}
	return string(b)
}

// FindModel returns the ModelConfig for the given id.
// Returns ErrModelNotFound if no matching entry exists.
func (c *Config) FindModel(id string) (*ModelConfig, error) {
	for i := range c.Models {
		if c.Models[i].ID == id {
			return &c.Models[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrModelNotFound, id)
}
