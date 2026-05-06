package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultConfig(), nil
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

	return &cfg, nil
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
