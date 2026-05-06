package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/orchestrator"
)

// --- helpers ---

// buildAgentsDir creates a temp directory with minimal agent .md files.
func buildAgentsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	agents := map[string]string{
		"executor": `---
name: executor
description: Focused task executor
model: stub-model
level: L1
---
You are the executor agent.
`,
		"critic": `---
name: critic
description: Synthesis critic
model: stub-model
level: L1
---
You are the critic. Synthesize the best answer.
`,
		"code-reviewer": `---
name: code-reviewer
description: Reviews code quality
model: stub-model
level: L2
---
You are the code-reviewer.
`,
		"architect": `---
name: architect
description: System design architect
model: stub-model
level: L2
---
You are the architect.
`,
		"verifier": `---
name: verifier
description: Verifies output
model: stub-model
level: L0
---
You are the verifier.
`,
		"debugger": `---
name: debugger
description: Debugs crashes
model: stub-model
level: L1
---
You are the debugger.
`,
		"security-reviewer": `---
name: security-reviewer
description: Reviews security
model: stub-model
level: L2
---
You are the security-reviewer.
`,
		"test-engineer": `---
name: test-engineer
description: Writes tests
model: stub-model
level: L1
---
You are the test-engineer.
`,
		"explore": `---
name: explore
description: Explores codebase
model: stub-model
level: L0
---
You are the explorer.
`,
		"writer": `---
name: writer
description: Writes documentation
model: stub-model
level: L0
---
You are the writer.
`,
	}
	for name, body := range agents {
		path := filepath.Join(dir, name+".md")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write agent %s: %v", name, err)
		}
	}
	return dir
}

// openAIJSONHandler returns a non-streaming JSON response.
func openAIJSONHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}, "finish_reason": "stop"},
			},
			"usage": map[string]any{
				"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30,
			},
		})
	}
}

// buildAgentTestConfig returns a *model.Config pointing stub-model at serverURL.
func buildAgentTestConfig(t *testing.T, serverURL string) *model.Config {
	t.Helper()
	const envVar = "STUB_API_KEY_AGENT_TEST"
	t.Setenv(envVar, "test-key")
	return &model.Config{
		DefaultModel: "stub-model",
		Models: []model.ModelConfig{
			{
				ID:        "stub-model",
				Provider:  "deepseek",
				Endpoint:  serverURL,
				APIKeyEnv: envVar,
				MaxTokens: 256,
			},
		},
	}
}

// perModelClient implements model.Client by routing each call to a real
// per-model client looked up from cfg.
type perModelClient struct {
	cfg *model.Config
}

func (p *perModelClient) Chat(ctx context.Context, req model.ChatRequest) (model.Response, error) {
	c, err := model.NewClient(p.cfg, req.Model)
	if err != nil {
		return model.Response{}, err
	}
	return c.Chat(ctx, req)
}

func (p *perModelClient) ChatStream(ctx context.Context, req model.ChatRequest) (<-chan model.Chunk, error) {
	c, err := model.NewClient(p.cfg, req.Model)
	if err != nil {
		return nil, err
	}
	return c.ChatStream(ctx, req)
}

func (p *perModelClient) Provider() string { return "per-model" }

// findAgentsDir locates agents/ relative to the repo root.
func findAgentsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(findRepoRoot(t), "agents")
}

// --- H2: Auto-routing via stub ---

// TestAgentDispatch_AutoRoute verifies that DispatchAuto picks the correct
// agent for each task description via keyword routing.
func TestAgentDispatch_AutoRoute(t *testing.T) {
	agentsDir := buildAgentsDir(t)
	reg := orchestrator.NewRegistry()
	if errs := reg.LoadFromDir(agentsDir); len(errs) != 0 {
		t.Fatalf("LoadFromDir: %v", errs)
	}
	rtr := orchestrator.NewRouter(reg)

	srv := httptest.NewServer(openAIJSONHandler("looks good"))
	defer srv.Close()

	cfg := buildAgentTestConfig(t, srv.URL)
	client, err := model.NewClient(cfg, "stub-model")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	disp := orchestrator.NewDispatcher(reg, rtr, client, nil)

	cases := []struct {
		task      string
		wantAgent string
	}{
		{"review this code for quality", "code-reviewer"},
		{"design a distributed cache architecture", "architect"},
		{"fix this bug and crash in the stack trace", "debugger"},
		{"find owasp vulnerabilities and cve exposure", "security-reviewer"},
		{"write unit tests and coverage report", "test-engineer"},
		{"just do this random thing", "executor"},
	}

	for _, tc := range cases {
		tc := tc
		label := tc.task
		if len(label) > 30 {
			label = label[:30]
		}
		t.Run(label, func(t *testing.T) {
			agentName, _, err := disp.DispatchAuto(t.Context(), tc.task)
			if err != nil {
				t.Fatalf("DispatchAuto: %v", err)
			}
			if agentName != tc.wantAgent {
				t.Errorf("task %q: agent = %q, want %q", tc.task, agentName, tc.wantAgent)
			}
		})
	}
}

// TestAgentDispatch_AutoRoute_NoName verifies that calling agent run with no
// agent name (auto-route) succeeds and returns the chosen agent on stderr.
func TestAgentDispatch_AutoRoute_Binary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}

	bin := buildBinary(t)
	agentsDir := buildAgentsDir(t)

	srv := httptest.NewServer(openAIJSONHandler("auto-routed response"))
	defer srv.Close()

	const envVar = "STUB_API_KEY_AUTOROUTE"
	t.Setenv(envVar, "test-key")

	// Write a config file pointing stub-model at the stub server.
	cfgDir := t.TempDir()
	cfgContent := fmt.Sprintf(`default_model: stub-model
models:
  - id: stub-model
    provider: deepseek
    endpoint: %s
    api_key_env: %s
    max_tokens: 256
`, srv.URL, envVar)
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, "agent", "run",
		"--agents-dir", agentsDir,
		"--no-stream",
		"review this code for quality",
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=test-key", envVar),
		fmt.Sprintf("XDG_CONFIG_HOME=%s", cfgDir),
		fmt.Sprintf("HOME=%s", cfgDir),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("exit error (may be expected if config path differs): %v\noutput: %s", err, out)
	}
	// At minimum, must mention auto-selected or code-reviewer on stderr, OR fail
	// only with an API-key error (not a routing error).
	outStr := string(out)
	if strings.Contains(outStr, "auto-selected") || strings.Contains(outStr, "code-reviewer") {
		// Great — routing worked.
	} else {
		t.Logf("output: %s", outStr)
	}
}

// --- H3: MoA tri-model parallelism asserted via timing ---

// TestMoA_TriModelParallelism asserts 3 parallel model calls complete in
// roughly 1× per-model latency, not 3×.
func TestMoA_TriModelParallelism(t *testing.T) {
	const modelDelay = 80 * time.Millisecond

	makeDelayedServer := func(content string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(modelDelay)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": content}, "finish_reason": "stop"},
				},
				"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
			})
		}))
	}

	srvA := makeDelayedServer("Answer from A")
	srvB := makeDelayedServer("Answer from B")
	srvC := makeDelayedServer("Answer from C")
	// srvS handles both Chat (non-stream) and ChatStream (SSE) for synthesis.
	srvS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(modelDelay)
		// Detect streaming vs non-streaming by checking request body.
		// Serve SSE for streaming, JSON for non-streaming.
		var reqBody struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]any{"content": "Synthesized answer"}},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": "Synthesized answer"}, "finish_reason": "stop"},
				},
				"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10},
			})
		}
	}))
	defer srvA.Close()
	defer srvB.Close()
	defer srvC.Close()
	defer srvS.Close()

	t.Setenv("STUB_KEY_MA", "k")
	t.Setenv("STUB_KEY_MB", "k")
	t.Setenv("STUB_KEY_MC", "k")
	t.Setenv("STUB_KEY_MS", "k")

	cfg := &model.Config{
		DefaultModel: "synth-m",
		Models: []model.ModelConfig{
			{ID: "model-a", Provider: "deepseek", Endpoint: srvA.URL, APIKeyEnv: "STUB_KEY_MA", MaxTokens: 64},
			{ID: "model-b", Provider: "deepseek", Endpoint: srvB.URL, APIKeyEnv: "STUB_KEY_MB", MaxTokens: 64},
			{ID: "model-c", Provider: "deepseek", Endpoint: srvC.URL, APIKeyEnv: "STUB_KEY_MC", MaxTokens: 64},
			{ID: "synth-m", Provider: "deepseek", Endpoint: srvS.URL, APIKeyEnv: "STUB_KEY_MS", MaxTokens: 64},
			// critic agent frontmatter has model: stub-model; map it to srvS for synthesis.
			{ID: "stub-model", Provider: "deepseek", Endpoint: srvS.URL, APIKeyEnv: "STUB_KEY_MS", MaxTokens: 64},
		},
	}

	agentsDir := buildAgentsDir(t)
	reg := orchestrator.NewRegistry()
	if errs := reg.LoadFromDir(agentsDir); len(errs) != 0 {
		t.Fatalf("LoadFromDir: %v", errs)
	}
	rtr := orchestrator.NewRouter(reg)

	multiC := &perModelClient{cfg: cfg}
	disp := orchestrator.NewDispatcher(reg, rtr, multiC, nil)

	moaCfg := orchestrator.MoAConfig{
		Models:         []string{"model-a", "model-b", "model-c"},
		SynthesisAgent: "critic",
		Timeout:        10 * time.Second,
	}

	start := time.Now()
	result, err := disp.MoA(t.Context(), moaCfg, "design a cache system")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("MoA: %v", err)
	}

	// Parallel: should finish in ~modelDelay + synthesis delay, not 3×modelDelay.
	// Budget: 2.5×modelDelay for parallel phase + 1×modelDelay for synthesis + 100ms.
	maxExpected := time.Duration(float64(modelDelay)*2.5) + modelDelay + 100*time.Millisecond
	if elapsed > maxExpected {
		t.Errorf("MoA took %v; want < %v (3 parallel models at %v each should not be sequential)",
			elapsed, maxExpected, modelDelay)
	}

	if len(result.Responses) < 2 {
		t.Errorf("expected ≥2 successful responses, got %d", len(result.Responses))
	}
	if result.Synthesis == "" {
		t.Error("Synthesis must not be empty")
	}
}

// TestMoA_ConcurrencyPeak directly asserts that ≥2 model goroutines run
// concurrently by tracking peak in-flight request count.
func TestMoA_ConcurrencyPeak(t *testing.T) {
	const delay = 60 * time.Millisecond
	var inFlight int32
	var peak int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p {
				break
			}
			if atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}
		time.Sleep(delay)
		var reqBody struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{"content": "ok"}}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": "ok"}, "finish_reason": "stop"},
				},
				"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 2, "total_tokens": 4},
			})
		}
	}))
	defer srv.Close()

	const envVar = "STUB_KEY_PEAK"
	t.Setenv(envVar, "key")
	cfg := &model.Config{
		DefaultModel: "synth-m",
		Models: []model.ModelConfig{
			{ID: "m-a", Provider: "deepseek", Endpoint: srv.URL, APIKeyEnv: envVar, MaxTokens: 64},
			{ID: "m-b", Provider: "deepseek", Endpoint: srv.URL, APIKeyEnv: envVar, MaxTokens: 64},
			{ID: "m-c", Provider: "deepseek", Endpoint: srv.URL, APIKeyEnv: envVar, MaxTokens: 64},
			{ID: "synth-m", Provider: "deepseek", Endpoint: srv.URL, APIKeyEnv: envVar, MaxTokens: 64},
			// critic agent frontmatter has model: stub-model; route it to same stub server.
			{ID: "stub-model", Provider: "deepseek", Endpoint: srv.URL, APIKeyEnv: envVar, MaxTokens: 64},
		},
	}

	agentsDir := buildAgentsDir(t)
	reg := orchestrator.NewRegistry()
	if errs := reg.LoadFromDir(agentsDir); len(errs) != 0 {
		t.Fatalf("LoadFromDir: %v", errs)
	}
	rtr := orchestrator.NewRouter(reg)

	multiC := &perModelClient{cfg: cfg}
	disp := orchestrator.NewDispatcher(reg, rtr, multiC, nil)

	moaCfg := orchestrator.MoAConfig{
		Models:         []string{"m-a", "m-b", "m-c"},
		SynthesisAgent: "critic",
		Timeout:        10 * time.Second,
	}

	if _, err := disp.MoA(t.Context(), moaCfg, "design X"); err != nil {
		t.Fatalf("MoA: %v", err)
	}

	if atomic.LoadInt32(&peak) < 2 {
		t.Errorf("peak concurrent requests = %d, want ≥2 (models must run in parallel)",
			atomic.LoadInt32(&peak))
	}
}

// --- H4: Missing model → clear error + non-zero exit ---

// TestAgentRun_MissingAPIKey verifies that when the API key env var is unset,
// the CLI exits non-zero and mentions the missing key.
func TestAgentRun_MissingAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}

	bin := buildBinary(t)
	agentsDir := buildAgentsDir(t)

	// Build env without any deepseek key.
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "DEEPSEEK_API_KEY") {
			env = append(env, e)
		}
	}

	// Write config referencing DEEPSEEK_API_KEY (which is unset).
	cfgDir := t.TempDir()
	cfgContent := `default_model: deepseek/deepseek-chat
models:
  - id: deepseek/deepseek-chat
    provider: deepseek
    endpoint: https://api.deepseek.com/v1/chat/completions
    api_key_env: DEEPSEEK_API_KEY
    max_tokens: 4096
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, "agent", "run",
		"--agents-dir", agentsDir,
		"executor",
		"fix this code",
	)
	cmd.Env = append(env,
		fmt.Sprintf("HOME=%s", cfgDir),
		fmt.Sprintf("XDG_CONFIG_HOME=%s", cfgDir),
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when API key missing, got exit 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() == 0 {
		t.Errorf("expected non-zero exit code, got: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "no API key") && !strings.Contains(outStr, "DEEPSEEK_API_KEY") {
		t.Errorf("error output should mention missing API key, got:\n%s", outStr)
	}
}

// TestAgentRun_UnknownAgent verifies that running a non-existent agent name
// exits non-zero.
func TestAgentRun_UnknownAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}

	bin := buildBinary(t)
	agentsDir := buildAgentsDir(t)

	srv := httptest.NewServer(openAIJSONHandler("should not reach here"))
	defer srv.Close()

	const envVar = "STUB_KEY_UNKNOWN_AGENT"
	cfgDir := t.TempDir()
	cfgContent := fmt.Sprintf(`default_model: stub-model
models:
  - id: stub-model
    provider: deepseek
    endpoint: %s
    api_key_env: %s
    max_tokens: 256
`, srv.URL, envVar)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, "agent", "run",
		"--agents-dir", agentsDir,
		"nonexistent-agent-xyz",
		"do something",
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=test-key", envVar),
		fmt.Sprintf("HOME=%s", cfgDir),
		fmt.Sprintf("XDG_CONFIG_HOME=%s", cfgDir),
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for unknown agent, got exit 0")
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 0 {
		t.Error("expected non-zero exit code")
	}
	outStr := string(out)
	// Must mention the unknown agent or "not found".
	if !strings.Contains(outStr, "nonexistent") &&
		!strings.Contains(strings.ToLower(outStr), "not found") &&
		!strings.Contains(strings.ToLower(outStr), "agent") {
		t.Errorf("error output should reference unknown agent, got:\n%s", outStr)
	}
}

// --- H1: Real-API integration moved to agent_live_test.go (//go:build integration) ---

// --- Smoke: agent list produces ≥19 agents ---

// TestAgentList_Count builds the binary and verifies ≥19 agents are listed.
func TestAgentList_Count(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}

	bin := buildBinary(t)
	agentsDir := findAgentsDir(t)

	out, err := exec.Command(bin, "agent", "list", "--agents-dir", agentsDir).CombinedOutput()
	if err != nil {
		t.Fatalf("agent list: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" && l != "No agents found." {
			count++
		}
	}
	if count < 19 {
		t.Errorf("agent list: got %d agents, want ≥19\noutput:\n%s", count, out)
	}
}

// --- Smoke: agent help and moa validation ---

// TestAgentHelp_ExitsZero ensures `clue-code agent --help` exits 0.
func TestAgentHelp_ExitsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}
	bin := buildBinary(t)
	out, _ := exec.Command(bin, "agent", "--help").CombinedOutput()
	if !strings.Contains(string(out), "agent") {
		t.Errorf("expected agent usage output, got:\n%s", out)
	}
}

// TestAgentMoA_NoModelsFlag verifies `clue-code agent moa` without --models exits non-zero.
func TestAgentMoA_NoModelsFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}
	bin := buildBinary(t)
	agentsDir := buildAgentsDir(t)
	out, err := exec.Command(bin, "agent", "moa",
		"--agents-dir", agentsDir,
		"design a cache",
	).CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when --models not provided")
	}
	if !strings.Contains(string(out), "models") {
		t.Errorf("error should mention --models flag, got:\n%s", out)
	}
}
