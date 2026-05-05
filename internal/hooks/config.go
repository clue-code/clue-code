package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultTimeout = 5 * time.Second
	MaxTimeout     = 30 * time.Second
	minTimeout     = 1 * time.Second
)

// Spec describes a single hook command bound to a lifecycle event.
type Spec struct {
	Command  string        `yaml:"command"`
	Matcher  string        `yaml:"matcher,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
	Blocking bool          `yaml:"blocking,omitempty"`
	Inject   bool          `yaml:"inject,omitempty"`
}

// Config is the parsed contents of ~/.config/clue-code/hooks.yaml.
type Config struct {
	Events       map[Event][]Spec `yaml:"events"`
	Allowlist    []string         `yaml:"allowlist,omitempty"`
	AllowSelfInv bool             `yaml:"allow_self_invoke,omitempty"`
}

// LoadConfig reads ~/.config/clue-code/hooks.yaml and returns the parsed
// Config.  If the file does not exist a zero Config (no hooks) is returned
// without an error.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("hooks: read config %q: %w", path, err)
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("hooks: config %q: %w", path, err)
	}
	return cfg, nil
}

// parseConfig unmarshals raw YAML bytes into a validated Config.
func parseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("hooks: parse config: %w", err)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("hooks: invalid config: %w", err)
	}
	return &cfg, nil
}

// configPath returns the absolute path to the hooks config file.
func configPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("hooks: user config dir: %w", err)
	}
	return filepath.Join(cfgDir, "clue-code", "hooks.yaml"), nil
}

// validateConfig normalises per-spec fields and validates matchers.
func validateConfig(cfg *Config) error {
	for event, specs := range cfg.Events {
		if !event.Valid() {
			return fmt.Errorf("unknown event %q", event)
		}
		for i := range specs {
			s := &specs[i]
			if s.Command == "" {
				return fmt.Errorf("event %q spec %d: command must not be empty", event, i)
			}
			if s.Matcher != "" {
				if _, err := regexp.Compile(s.Matcher); err != nil {
					return fmt.Errorf("event %q spec %d: invalid matcher regexp: %w", event, i, err)
				}
			}
			// Clamp timeout to [1s, 30s]; default 5s when unset.
			switch {
			case s.Timeout == 0:
				s.Timeout = DefaultTimeout
			case s.Timeout < minTimeout:
				s.Timeout = minTimeout
			case s.Timeout > MaxTimeout:
				s.Timeout = MaxTimeout
			}
		}
		cfg.Events[event] = specs
	}
	return nil
}
