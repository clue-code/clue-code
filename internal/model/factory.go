package model

import (
	"fmt"
	"os"
)

// providerConstructor is the function signature for provider-specific constructors.
// Each provider sub-package registers itself via init() into the providers map.
type providerConstructor func(mc ModelConfig, apiKey string) (Client, error)

// providers maps provider names to their constructors.
// Cloud provider sub-packages register here via init().
var providers = map[string]providerConstructor{}

// RegisterProvider registers a constructor for a named provider.
// Called from provider sub-package init() functions.
func RegisterProvider(name string, fn providerConstructor) {
	providers[name] = fn
}

// NewClient constructs a Client for the given modelID using cfg.
// For cloud providers the API key is read from the environment variable
// specified in ModelConfig.APIKeyEnv; ErrNoAPIKey is returned if unset.
// Local providers (ollama, mlx) do not require an API key.
func NewClient(cfg *Config, modelID string) (Client, error) {
	mc, err := cfg.FindModel(modelID)
	if err != nil {
		return nil, err
	}

	var apiKey string
	if mc.APIKeyEnv != "" {
		apiKey = os.Getenv(mc.APIKeyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("%w: set %s", ErrNoAPIKey, mc.APIKeyEnv)
		}
	}

	ctor, ok := providers[mc.Provider]
	if !ok {
		// No constructor registered yet — return a stub that surfaces the
		// missing provider clearly instead of panicking at call time.
		return nil, fmt.Errorf("model: unknown provider %q for model %q", mc.Provider, modelID)
	}

	return ctor(*mc, apiKey)
}
