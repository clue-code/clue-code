package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// InstallOllama installs Ollama via the official install script, waits for
// the service to become ready, pulls llama3.2, and runs a smoke test.
// progressCb receives stage name and a rough 0–1 progress fraction.
//
//nolint:cyclop // wizard install flow is inherently sequential
func InstallOllama(ctx context.Context, progressCb func(stage string, pct float64)) error {
	if progressCb == nil {
		progressCb = func(_ string, _ float64) {}
	}

	// Skip if already installed and running.
	installed, _, running := DetectOllama()
	if installed && running {
		progressCb("already_installed", 1.0)
		return nil
	}

	if !installed {
		progressCb("downloading_installer", 0.05)
		// Download and run the official install script via curl | sh.
		// We use exec.CommandContext so ctx.Done() cancels the download.
		curlArgs := []string{"-fsSL", "https://ollama.com/install.sh"}
		curlCmd := exec.CommandContext(ctx, "curl", curlArgs...)
		installScript, err := curlCmd.Output()
		if err != nil {
			return fmt.Errorf("setup: download ollama install script: %w", err)
		}

		progressCb("running_installer", 0.2)
		shCmd := exec.CommandContext(ctx, "sh")
		shCmd.Stdin = strings.NewReader(string(installScript))
		shCmd.Stdout = os.Stdout
		shCmd.Stderr = os.Stderr
		if err := shCmd.Run(); err != nil {
			return fmt.Errorf("setup: run ollama install script: %w", err)
		}
	}

	// Wait for the Ollama service to become ready.
	progressCb("waiting_for_service", 0.5)
	if err := waitOllamaReady(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("setup: ollama service not ready: %w", err)
	}

	// Pull the model.
	progressCb("pulling_model", 0.6)
	pullCmd := exec.CommandContext(ctx, "ollama", "pull", "llama3.2")
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("setup: ollama pull llama3.2: %w", err)
	}

	// Smoke test.
	progressCb("smoke_test", 0.9)
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	testCmd := exec.CommandContext(testCtx, "ollama", "run", "llama3.2", "hi")
	testCmd.Stdout = os.Stdout
	testCmd.Stderr = os.Stderr
	if err := testCmd.Run(); err != nil {
		return fmt.Errorf("setup: ollama smoke test failed: %w", err)
	}

	progressCb("done", 1.0)
	return nil
}

// waitOllamaReady polls the local Ollama API until it responds or timeout
// expires.
func waitOllamaReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", timeout)
		}
		resp, err := client.Get("http://localhost:11434/api/version")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ConfigureDeepSeek validates the API key against the DeepSeek API, then
// writes it to ~/.config/clue-code/config.yaml and suggests adding it to
// the shell profile.
func ConfigureDeepSeek(ctx context.Context, apiKey string) error {
	if !strings.HasPrefix(apiKey, "sk-") {
		return fmt.Errorf("setup: DeepSeek API key must start with 'sk-'")
	}

	// Test the key against the models endpoint.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.deepseek.com/v1/models", nil)
	if err != nil {
		return fmt.Errorf("setup: build deepseek request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("setup: deepseek API test: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("setup: DeepSeek API key invalid (401 Unauthorized)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("setup: DeepSeek API returned status %d", resp.StatusCode)
	}

	// Persist the key into config.yaml.
	if err := writeDeepSeekConfig(apiKey); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  Suggestion: ajoutez cette ligne a votre ~/.zshrc ou ~/.bashrc :")
	fmt.Printf("  export DEEPSEEK_API_KEY=%s\n", apiKey)
	return nil
}

// writeDeepSeekConfig writes (or updates) the DeepSeek model entry in
// ~/.config/clue-code/config.yaml using a minimal JSON config.json approach
// compatible with the existing config package.
func writeDeepSeekConfig(apiKey string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("setup: home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "clue-code")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("setup: create config dir: %w", err)
	}

	// Write to config.json (used by the mode/config subsystem).
	jsonPath := filepath.Join(dir, "config.json")
	existing := map[string]any{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	existing["deepseek_api_key"] = apiKey
	existing["default_provider"] = "deepseek"

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("setup: marshal config: %w", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		return fmt.Errorf("setup: write config.json: %w", err)
	}

	// Also set the env var for the current process so subsequent commands work.
	if err := os.Setenv("DEEPSEEK_API_KEY", apiKey); err != nil {
		return fmt.Errorf("setup: setenv: %w", err)
	}

	return nil
}

// ConfigureAnthropic validates and persists an Anthropic API key.
func ConfigureAnthropic(ctx context.Context, apiKey string) error {
	if !strings.HasPrefix(apiKey, "sk-ant-") {
		return fmt.Errorf("setup: Anthropic API key must start with 'sk-ant-'")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("setup: home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "clue-code")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("setup: create config dir: %w", err)
	}

	jsonPath := filepath.Join(dir, "config.json")
	existing := map[string]any{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	existing["anthropic_api_key"] = apiKey
	existing["default_provider"] = "anthropic"

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("setup: marshal config: %w", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		return fmt.Errorf("setup: write config.json: %w", err)
	}

	if err := os.Setenv("ANTHROPIC_API_KEY", apiKey); err != nil {
		return fmt.Errorf("setup: setenv: %w", err)
	}

	fmt.Println()
	fmt.Println("  Suggestion: ajoutez cette ligne a votre ~/.zshrc ou ~/.bashrc :")
	fmt.Printf("  export ANTHROPIC_API_KEY=%s\n", apiKey)
	return nil
}

// OpenBrowser opens url in the system default browser.
// Supports macOS (open), Linux (xdg-open), and Windows (start).
func OpenBrowser(url string) error {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "start"
	default:
		return fmt.Errorf("setup: OpenBrowser not supported on %s", runtime.GOOS)
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("setup: browser command %q not found: %w", cmd, err)
	}
	return exec.Command(cmd, url).Start()
}
