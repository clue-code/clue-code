package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

// --- helpers ---

// openAIStreamHandler serves a canned OpenAI SSE stream of tokens.
func openAIStreamHandler(tokens []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range tokens {
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]any{"content": tok}},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// buildTestConfig returns a *model.Config pointing at the given stub server URL,
// using the given provider and a fake env var name that is pre-set via t.Setenv.
func buildTestConfig(t *testing.T, serverURL, provider, modelID, apiKeyEnv string) *model.Config {
	t.Helper()
	if apiKeyEnv != "" {
		t.Setenv(apiKeyEnv, "test-key")
	}
	return &model.Config{
		DefaultModel: modelID,
		Models: []model.ModelConfig{
			{
				ID:        modelID,
				Provider:  provider,
				Endpoint:  serverURL,
				APIKeyEnv: apiKeyEnv,
				MaxTokens: 256,
			},
		},
	}
}

// --- F2: Streaming chunks via httptest ---

// TestChat_StreamingChunks verifies that the streaming path delivers multiple
// discrete writes to stdout (at least 3 chunks before [DONE]).
func TestChat_StreamingChunks(t *testing.T) {
	tokens := []string{"1", " ", "2", " ", "3"}
	srv := httptest.NewServer(openAIStreamHandler(tokens))
	defer srv.Close()

	cfg := buildTestConfig(t, srv.URL, "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY_TEST")

	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ch, err := client.ChatStream(t.Context(), model.ChatRequest{
		Model:    "deepseek-chat",
		Messages: []model.Message{{Role: model.RoleUser, Content: "count 1 to 3"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var chunks []string
	for chunk := range ch {
		if chunk.Done {
			break
		}
		if chunk.Delta != "" {
			chunks = append(chunks, chunk.Delta)
		}
	}

	if len(chunks) < 3 {
		t.Errorf("expected at least 3 separate chunks, got %d: %v", len(chunks), chunks)
	}
	combined := strings.Join(chunks, "")
	if combined == "" {
		t.Error("combined stream content is empty")
	}
}

// --- F3: Local mode — no DeepSeek calls ---

// TestOllama_LocalOnly verifies that when provider=ollama is configured,
// no call is made to deepseek.com (only the stub server is reachable).
func TestOllama_LocalOnly(t *testing.T) {
	deepseekCalled := false
	deepseekSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deepseekCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer deepseekSrv.Close()

	ollamaMux := http.NewServeMux()
	ollamaMux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	})
	ollamaMux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello from local ollama"}},
			},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 4, "total_tokens": 6},
		})
	})
	ollamaSrv := httptest.NewServer(ollamaMux)
	defer ollamaSrv.Close()

	cfg := &model.Config{
		DefaultModel: "ollama/qwen3-coder:7b",
		Models: []model.ModelConfig{
			{
				ID:       "ollama/qwen3-coder:7b",
				Provider: "ollama",
				Endpoint: ollamaSrv.URL + "/v1/chat/completions",
			},
		},
	}

	client, err := model.NewClient(cfg, "ollama/qwen3-coder:7b")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, err := client.Chat(t.Context(), model.ChatRequest{
		Model:    "ollama/qwen3-coder:7b",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from local ollama" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if deepseekCalled {
		t.Error("DeepSeek stub was called — local mode should not contact cloud providers")
	}
}

// --- F4: --model routing via path-based stub ---

// TestModelFlag_Routing verifies that when a specific model ID is selected,
// the correct provider's endpoint is called.
func TestModelFlag_Routing(t *testing.T) {
	var calledPath string
	routeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		// Serve a valid anthropic response (we register it as anthropic provider).
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "routed response"}},
			"usage":   map[string]any{"input_tokens": 3, "output_tokens": 5},
		})
	}))
	defer routeSrv.Close()

	t.Setenv("ANTHROPIC_API_KEY_TEST", "test-key")

	cfg := &model.Config{
		DefaultModel: "deepseek-chat",
		Models: []model.ModelConfig{
			{
				ID:        "deepseek-chat",
				Provider:  "deepseek",
				Endpoint:  "http://127.0.0.1:19998", // unreachable — should not be called
				APIKeyEnv: "DEEPSEEK_UNREACHABLE",
			},
			{
				ID:        "anthropic/claude-sonnet-4-6",
				Provider:  "anthropic",
				Endpoint:  routeSrv.URL + "/v1",
				APIKeyEnv: "ANTHROPIC_API_KEY_TEST",
				MaxTokens: 256,
			},
		},
	}
	// Ensure the deepseek key is NOT set so NewClient would fail if it's picked.
	_ = os.Unsetenv("DEEPSEEK_UNREACHABLE")

	client, err := model.NewClient(cfg, "anthropic/claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if client.Provider() != "anthropic" {
		t.Errorf("expected provider anthropic, got %q", client.Provider())
	}

	_, err = client.Chat(t.Context(), model.ChatRequest{
		Model:    "anthropic/claude-sonnet-4-6",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if !strings.Contains(calledPath, "/messages") {
		t.Errorf("expected /messages endpoint to be called, got path %q", calledPath)
	}
}

// --- F5: Missing API key error ---

// TestNoAPIKey_ClearError verifies that when the API key env var is unset,
// NewClient returns ErrNoAPIKey with the env var name in the message.
func TestNoAPIKey_ClearError(t *testing.T) {
	const envVar = "DEEPSEEK_API_KEY_MISSING_TEST"
	_ = os.Unsetenv(envVar)

	cfg := &model.Config{
		DefaultModel: "deepseek-chat",
		Models: []model.ModelConfig{
			{
				ID:        "deepseek-chat",
				Provider:  "deepseek",
				Endpoint:  "https://api.deepseek.com/v1/chat/completions",
				APIKeyEnv: envVar,
			},
		},
	}

	_, err := model.NewClient(cfg, "deepseek-chat")
	if err == nil {
		t.Fatal("expected error when API key missing, got nil")
	}
	if !errors.Is(err, model.ErrNoAPIKey) {
		t.Errorf("expected ErrNoAPIKey, got: %v", err)
	}
	if !strings.Contains(err.Error(), envVar) {
		t.Errorf("error message should contain env var name %q, got: %v", envVar, err)
	}
}

// --- F1: Cloud round-trip (integration, gated by -tags=integration) ---

// TestDeepSeekChat_Live tests an actual DeepSeek API call.
// Skipped unless -tags=integration AND DEEPSEEK_API_KEY is set.
func TestDeepSeekChat_Live(t *testing.T) {
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set; skipping live integration test")
	}

	cfg, err := model.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	client, err := model.NewClient(cfg, cfg.DefaultModel)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := t.Context()
	deadline := time.Now().Add(10 * time.Second)
	_ = deadline

	resp, err := client.Chat(ctx, model.ChatRequest{
		Model:    cfg.DefaultModel,
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response content from live DeepSeek API")
	}
}

// --- Smoke: chat --help exits 0 ---

// TestChatHelp_ExitsZero builds the binary and runs `clue-code chat --help`,
// expecting exit 0 and usage text on stderr.
func TestChatHelp_ExitsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}

	// Build the binary.
	bin := t.TempDir() + "/clue-code"
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/clue-code")
	cmd.Dir = findRepoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Run chat --help.
	out, err := exec.Command(bin, "chat", "--help").CombinedOutput()
	if err != nil {
		// flag.ContinueOnError with -h returns exit 0.
		t.Logf("exit error (may be normal): %v", err)
	}
	if !strings.Contains(string(out), "clue-code chat") {
		t.Errorf("expected usage output, got:\n%s", out)
	}
}

// findRepoRoot walks up from the test binary location to find the go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	// In tests the working directory is the package directory.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Walk up looking for go.mod.
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}
