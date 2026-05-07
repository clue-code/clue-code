package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DetectOllama probes whether Ollama is installed and whether the service is
// currently running (by querying http://localhost:11434/api/version).
func DetectOllama() (installed bool, version string, running bool) {
	// Check binary presence.
	binPath, err := exec.LookPath("ollama")
	if err != nil {
		return false, "", false
	}
	installed = true

	// Get version from binary.
	out, err := exec.Command(binPath, "--version").Output()
	if err == nil {
		version = strings.TrimSpace(string(out))
	}

	// Probe the local API.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ver, err := ollamaAPIVersion(ctx)
	if err == nil {
		running = true
		if version == "" {
			version = ver
		}
	}

	return installed, version, running
}

// ollamaAPIVersion fetches the version string from the local Ollama API.
var ollamaAPIVersion = func(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "curl", "-s", "--max-time", "2",
		"http://localhost:11434/api/version").Output()
	if err != nil {
		return "", err
	}
	var resp struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse ollama version: %w", err)
	}
	return resp.Version, nil
}

// DetectMLX checks whether the MLX inference server is available.
// This is only meaningful on Apple Silicon (arm64 darwin); on other
// platforms it always returns false.
func DetectMLX() (installed bool, version string) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		return false, ""
	}
	// Prefer the standalone mlx_lm.server binary.
	if _, err := os.Stat("/usr/local/bin/mlx_lm.server"); err == nil {
		return true, "binary"
	}
	// Fall back to Python package check.
	py, err := exec.LookPath("python3")
	if err != nil {
		return false, ""
	}
	out, err := exec.Command(py, "-c",
		"import mlx_lm; print(mlx_lm.__version__)").Output()
	if err != nil {
		return false, ""
	}
	return true, strings.TrimSpace(string(out))
}

// DetectAPIKeys returns a map of provider → bool indicating whether the
// corresponding API key environment variable is set and non-empty.
func DetectAPIKeys() map[string]bool {
	keys := map[string]bool{
		"DEEPSEEK_API_KEY":   os.Getenv("DEEPSEEK_API_KEY") != "",
		"ANTHROPIC_API_KEY":  os.Getenv("ANTHROPIC_API_KEY") != "",
		"GROQ_API_KEY":       os.Getenv("GROQ_API_KEY") != "",
		"OPENROUTER_API_KEY": os.Getenv("OPENROUTER_API_KEY") != "",
	}
	return keys
}

// DetectModelsConfigured reads ~/.config/clue-code/config.yaml and returns
// a list of provider names that have valid (non-empty) API key configuration.
func DetectModelsConfigured() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".config", "clue-code", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// Minimal YAML parsing without importing yaml package — extract provider lines.
	var configured []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "provider:") {
			provider := strings.TrimSpace(strings.TrimPrefix(line, "provider:"))
			provider = strings.Trim(provider, `"'`)
			if provider != "" && !seen[provider] {
				seen[provider] = true
				configured = append(configured, provider)
			}
		}
	}
	return configured
}
