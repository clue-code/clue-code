package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/hooks"
	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/skillrunner"
	"github.com/clue-code/clue-code/internal/state"
)

// --- helpers ---

// stubStreamHandler returns an HTTP handler that streams fixed tokens as OpenAI SSE.
func stubStreamHandler(tokens []string) http.HandlerFunc {
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

// logEntriesFromDir reads all NDJSON hook log lines written to projectDir.
func logEntriesFromDir(t *testing.T, projectDir string) []hooks.LogEntry {
	t.Helper()
	path := filepath.Join(projectDir, ".clue-code", "state", "hooks.log")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var entries []hooks.LogEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e hooks.LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	return entries
}

// buildBinary compiles the clue-code binary into a temp dir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "clue-code")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/clue-code")
	cmd.Dir = findRepoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// --- G1: Autopilot real API (integration-gated) ---

// TestG1_AutopilotRealAPI runs the autopilot skill against the real DeepSeek API.
// Skipped unless DEEPSEEK_API_KEY is set.
func TestG1_AutopilotRealAPI(t *testing.T) {
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set; skipping G1 live integration test")
	}

	cfg, err := model.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	client, err := model.NewClient(cfg, cfg.DefaultModel)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	store, err := state.Open("g1-autopilot-test")
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pr, pw, _ := os.Pipe()
	runner := skillrunner.NewRealRunner(client, store, nil, pw)
	eng := skillrunner.NewEngine(nil)
	root := findRepoRoot(t)
	if loadErr := eng.Load(filepath.Join(root, "skills")); loadErr != nil {
		t.Logf("load warnings: %v", loadErr)
	}
	eng.WithRunner(runner)

	runErr := eng.Run(ctx, "autopilot", []string{"build a hello world program in Go that prints Hello World"})
	_ = pw.Close()

	var outBuf strings.Builder
	_, _ = io.Copy(&outBuf, pr)
	output := outBuf.String()

	if runErr != nil {
		t.Fatalf("G1 autopilot run error: %v (output: %s)", runErr, output)
	}
	// Signal-of-life: response should contain Go-shaped content.
	goMarkers := []string{"package", "main", "func", "fmt"}
	found := 0
	for _, marker := range goMarkers {
		if strings.Contains(output, marker) {
			found++
		}
	}
	if found < 2 {
		t.Errorf("G1: expected Go-shaped output (at least 2 of %v), got:\n%s", goMarkers, output)
	}
}

// --- G2: Ralph real API (integration-gated) ---

// TestG2_RalphRealAPI runs the ralph skill against the real DeepSeek API.
// Skipped unless DEEPSEEK_API_KEY is set.
func TestG2_RalphRealAPI(t *testing.T) {
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set; skipping G2 live integration test")
	}

	cfg, err := model.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	client, err := model.NewClient(cfg, cfg.DefaultModel)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	store, err := state.Open("g2-ralph-test")
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}

	pr, pw, _ := os.Pipe()
	runner := skillrunner.NewRealRunner(client, store, nil, pw)
	eng := skillrunner.NewEngine(nil)
	root := findRepoRoot(t)
	if loadErr := eng.Load(filepath.Join(root, "skills")); loadErr != nil {
		t.Logf("load warnings: %v", loadErr)
	}
	eng.WithRunner(runner)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runErr := eng.Run(ctx, "ralph", []string{"produce a numbered 3-step plan to fix lint errors"})
	_ = pw.Close()

	var outBuf strings.Builder
	_, _ = io.Copy(&outBuf, pr)
	output := outBuf.String()

	if runErr != nil {
		t.Fatalf("G2 ralph run error: %v (output: %s)", runErr, output)
	}
	// Assert response contains numbered steps.
	for _, step := range []string{"1.", "2.", "3."} {
		if !strings.Contains(output, step) {
			t.Errorf("G2: expected %q in ralph output, got:\n%s", step, output)
		}
	}
}

// --- G3: Lifecycle hooks fire 5 events with stub model ---

// TestG3_LifecycleHooksFire verifies all 5 lifecycle events fire in order
// when a skill runs. Uses a stub HTTP model — no API key needed.
func TestG3_LifecycleHooksFire(t *testing.T) {
	srv := httptest.NewServer(stubStreamHandler([]string{"hello", " ", "world"}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY_G3", "stub-key")
	cfg := &model.Config{
		DefaultModel: "deepseek-chat",
		Models: []model.ModelConfig{
			{
				ID:        "deepseek-chat",
				Provider:  "deepseek",
				Endpoint:  srv.URL,
				APIKeyEnv: "DEEPSEEK_API_KEY_G3",
				MaxTokens: 256,
			},
		},
	}
	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	projectDir := t.TempDir()
	hooksCfg := &hooks.Config{
		Events: map[hooks.Event][]hooks.Spec{
			hooks.EventSessionStart:     {{Command: "echo start", Timeout: 5 * time.Second}},
			hooks.EventPreToolUse:       {{Command: "echo pre", Timeout: 5 * time.Second}},
			hooks.EventUserPromptSubmit: {{Command: "echo prompt", Timeout: 5 * time.Second}},
			hooks.EventPostToolUse:      {{Command: "echo post", Timeout: 5 * time.Second}},
			hooks.EventStop:             {{Command: "echo stop", Timeout: 5 * time.Second}},
		},
	}
	hm, err := hooks.NewManager(hooksCfg, projectDir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = hm.Close() })

	store, err := state.Open("g3-hooks-test")
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}

	eng := skillrunner.NewEngine(hm)
	// RealRunner fires mid-run hooks (UserPromptSubmit, PreToolUse, PostToolUse)
	// natively. Engine wraps with SessionStart/Stop. No shim needed.
	runner := skillrunner.NewRealRunner(client, store, hm, nil)
	eng.WithRunner(runner)
	eng.SetSkill("synthetic-skill", &skillrunner.Skill{
		Name: "synthetic-skill",
		Body: "You are a helpful assistant.",
	})

	if err := eng.Run(context.Background(), "synthetic-skill", []string{"test"}); err != nil {
		t.Fatalf("G3 run error: %v", err)
	}

	entries := logEntriesFromDir(t, projectDir)
	if len(entries) < 5 {
		t.Fatalf("G3: want >=5 hook log entries, got %d", len(entries))
	}
	// Natural order: SessionStart (session begin) → UserPromptSubmit (user typed) →
	// PreToolUse (about to call model) → PostToolUse (model returned) → Stop.
	want := []hooks.Event{
		hooks.EventSessionStart,
		hooks.EventUserPromptSubmit,
		hooks.EventPreToolUse,
		hooks.EventPostToolUse,
		hooks.EventStop,
	}
	for i, ev := range want {
		if i >= len(entries) {
			t.Errorf("G3: missing entry[%d] want %s", i, ev)
			continue
		}
		if entries[i].Event != ev {
			t.Errorf("G3: entry[%d] want %s, got %s", i, ev, entries[i].Event)
		}
	}
}

// --- G4: Empty args → exit 2 + usage stderr ---

// TestG4_EmptyArgs verifies that `clue-code skill run` with no skill name
// exits 2 and prints usage to stderr.
func TestG4_EmptyArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke test in short mode")
	}

	bin := buildBinary(t)

	cmd := exec.Command(bin, "skill", "run")
	out, err := cmd.CombinedOutput()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	if exitCode != 2 {
		t.Errorf("G4: want exit code 2, got %d (output: %s)", exitCode, out)
	}
	if !strings.Contains(string(out), "usage") && !strings.Contains(string(out), "Usage") {
		t.Errorf("G4: want 'usage' in stderr output, got: %s", out)
	}
}

// --- G5: ctx.Cancel → graceful return + Stop hook fires ---

// TestG5_CtrlCGraceful verifies that cancelling the context during a running
// skill (the in-process equivalent of Ctrl-C) causes Engine.Run to return
// within 500ms AND fires the Stop hook before returning.
//
// Tests in-process via direct skillrunner.Engine usage rather than spawning
// the binary + sending SIGINT, because OS-level signal delivery to a child
// `go test`-spawned binary on darwin under `-race` is unreliable for sub-200ms
// timing. The production code path (`signal.NotifyContext` in skill.go) also
// turns SIGINT into ctx.Cancel, so this in-process test exercises the same
// post-signal path that the real binary would.
func TestG5_CtrlCGraceful(t *testing.T) {
	// Stub server that blocks the model call indefinitely until the client
	// (RealRunner) disconnects via ctx.Cancel.
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-blocked:
		}
	}))
	defer srv.Close()
	defer close(blocked)

	t.Setenv("DEEPSEEK_API_KEY_G5", "stub-key")
	cfg := &model.Config{
		DefaultModel: "deepseek-chat",
		Models: []model.ModelConfig{
			{
				ID:        "deepseek-chat",
				Provider:  "deepseek",
				Endpoint:  srv.URL,
				APIKeyEnv: "DEEPSEEK_API_KEY_G5",
				MaxTokens: 256,
			},
		},
	}
	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Stop hook touches a marker file so we can assert it fired even after cancel.
	projectDir := t.TempDir()
	stopMarker := filepath.Join(projectDir, "stop-fired")
	hooksCfg := &hooks.Config{
		Events: map[hooks.Event][]hooks.Spec{
			hooks.EventStop: {
				{Command: fmt.Sprintf("touch %s", stopMarker), Timeout: 5 * time.Second},
			},
		},
	}
	hm, err := hooks.NewManager(hooksCfg, projectDir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = hm.Close() })

	store, err := state.Open("g5-cancel-test")
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}

	eng := skillrunner.NewEngine(hm)
	runner := skillrunner.NewRealRunner(client, store, hm, io.Discard)
	eng.WithRunner(runner)
	eng.SetSkill("blocking-skill", &skillrunner.Skill{
		Name: "blocking-skill",
		Body: "You are a slow assistant.",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run in a goroutine and cancel after 50ms; assert return within 500ms.
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- eng.Run(ctx, "blocking-skill", []string{"hang please"})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case runErr := <-done:
		elapsed := time.Since(start)
		if elapsed > 500*time.Millisecond {
			t.Errorf("G5: Engine.Run took %v, want ≤500ms after cancel", elapsed)
		}
		if runErr == nil {
			t.Error("G5: Engine.Run returned nil, expected ctx.Canceled")
		} else if !errors.Is(runErr, context.Canceled) {
			t.Logf("G5: Engine.Run returned %v (acceptable; non-nil is the contract)", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("G5: Engine.Run did not return within 2s after cancel — production code is stuck")
	}

	// Assert Stop hook fired (engine.go fires Stop via deferred call with fresh
	// context, so cancellation does not suppress it). Brief grace period for
	// the touch subprocess to complete.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(stopMarker); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("G5: Stop hook marker %q not found — Stop hook did not fire after cancel", stopMarker)
}

// --- G6: CLUE_CODE_SKILL_DEPTH=4 → ErrSkillDepthExceeded ---

// TestG6_SkillDepthExceeded verifies that setting CLUE_CODE_SKILL_DEPTH=4 causes
// the engine to return ErrSkillDepthExceeded before calling the model.
func TestG6_SkillDepthExceeded(t *testing.T) {
	t.Setenv(skillrunner.EnvSkillDepth, fmt.Sprintf("%d", skillrunner.MaxSkillDepth))

	// Stub server that should never be reached.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("G6: model server was called — depth guard should have blocked execution")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY_G6", "stub-key")
	cfg := &model.Config{
		DefaultModel: "deepseek-chat",
		Models: []model.ModelConfig{
			{
				ID:        "deepseek-chat",
				Provider:  "deepseek",
				Endpoint:  srv.URL,
				APIKeyEnv: "DEEPSEEK_API_KEY_G6",
				MaxTokens: 256,
			},
		},
	}
	client, err := model.NewClient(cfg, "deepseek-chat")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	store, err := state.Open("g6-depth-test")
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}

	eng := skillrunner.NewEngine(nil)
	runner := skillrunner.NewRealRunner(client, store, nil, nil)
	eng.WithRunner(runner)
	eng.SetSkill("cancel", &skillrunner.Skill{
		Name: "cancel",
		Body: "You are a cancellation handler.",
	})

	runErr := eng.Run(context.Background(), "cancel", []string{"stop"})
	if !errors.Is(runErr, skillrunner.ErrSkillDepthExceeded) {
		t.Errorf("G6: want ErrSkillDepthExceeded, got %v", runErr)
	}
}
