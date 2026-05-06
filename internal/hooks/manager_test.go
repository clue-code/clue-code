package hooks

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readLogLines returns all NDJSON lines from hooks.log in projectDir.
func readLogLines(t *testing.T, projectDir string) []LogEntry {
	t.Helper()
	path := filepath.Join(projectDir, ".clue-code", "state", "hooks.log")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open hooks.log: %v", err)
	}
	defer func() { _ = f.Close() }()
	var entries []LogEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	return entries
}

// A1: SessionStart → "echo hello" logs exactly one entry with event=SessionStart, exit_code=0.
func TestSessionStart_LogsExactlyOneEntry(t *testing.T) {
	cfg := &Config{
		Events: map[Event][]Spec{
			EventSessionStart: {
				{Command: "echo hello", Timeout: 5 * time.Second},
			},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}

	dir := t.TempDir()
	m, err := NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer func() { _ = m.Close() }()

	_, err = m.Fire(context.Background(), EventSessionStart, map[string]any{"session_id": "s1"})
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}

	entries := readLogLines(t, dir)
	if len(entries) != 1 {
		t.Fatalf("want 1 log entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Event != EventSessionStart {
		t.Errorf("event: want %s, got %s", EventSessionStart, e.Event)
	}
	if e.ExitCode != 0 {
		t.Errorf("exit_code: want 0, got %d", e.ExitCode)
	}
	// stdout_len should reflect "hello\n" = 6 bytes
	if e.StdoutLen == 0 {
		t.Errorf("stdout_len: want > 0")
	}
}

// A2: sleep 60 with Timeout=200ms → timed_out=true, exit_code=-1.
func TestRunSpec_TimeoutKilled(t *testing.T) {
	cfg := &Config{
		Events: map[Event][]Spec{
			EventPreToolUse: {
				{Command: "sleep 60", Timeout: 200 * time.Millisecond},
			},
		},
	}
	if err := validateConfig(cfg); err != nil {
		// validateConfig clamps timeout to minTimeout (1s); bypass by setting directly.
		_ = err
	}
	// Set timeout directly to avoid clamp in validateConfig.
	cfg.Events[EventPreToolUse][0].Timeout = 200 * time.Millisecond

	dir := t.TempDir()
	m, err := NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer func() { _ = m.Close() }()

	start := time.Now()
	_, fireErr := m.Fire(context.Background(), EventPreToolUse, map[string]any{"session_id": "s2"})
	elapsed := time.Since(start)

	// Non-blocking spec: Fire should NOT return an error even on timeout.
	if fireErr != nil {
		t.Fatalf("Fire: unexpected error: %v", fireErr)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Fire took %v, expected ≤5s (timeout not honoured)", elapsed)
	}

	entries := readLogLines(t, dir)
	if len(entries) != 1 {
		t.Fatalf("want 1 log entry, got %d", len(entries))
	}
	e := entries[0]
	if !e.TimedOut {
		t.Errorf("timed_out: want true")
	}
	if e.ExitCode != -1 {
		t.Errorf("exit_code: want -1, got %d", e.ExitCode)
	}
}

// A3: depth guard — at depth 3 (MaxHookDepth), Fire returns ErrHookDepthExceeded.
func TestDepthGuard_Hermetic(t *testing.T) {
	cfg := &Config{
		Events: map[Event][]Spec{
			EventPreToolUse: {
				{Command: "echo ok", Timeout: 5 * time.Second},
			},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}

	// Simulate being inside MaxHookDepth nested hooks by setting the env var.
	t.Setenv(EnvHookDepth, "3")

	dir := t.TempDir()
	m, err := NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer func() { _ = m.Close() }()

	_, err = m.Fire(context.Background(), EventPreToolUse, map[string]any{"session_id": "s3"})
	if !errors.Is(err, ErrHookDepthExceeded) {
		t.Errorf("want ErrHookDepthExceeded, got %v", err)
	}
}

// A4: inject:true wraps stdout in <hook-context source="...">...</hook-context>.
func TestInject_WrapsContext(t *testing.T) {
	cmd := "echo BUDGET=10000"
	cfg := &Config{
		Events: map[Event][]Spec{
			EventUserPromptSubmit: {
				{Command: cmd, Timeout: 5 * time.Second, Inject: true},
			},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}

	dir := t.TempDir()
	m, err := NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer func() { _ = m.Close() }()

	injected, err := m.Fire(context.Background(), EventUserPromptSubmit, map[string]any{"session_id": "s4"})
	if err != nil {
		t.Fatalf("Fire: %v", err)
	}

	want := `<hook-context source="echo BUDGET=10000">BUDGET=10000` + "\n</hook-context>"
	if injected != want {
		t.Errorf("injected:\nwant: %q\ngot:  %q", want, injected)
	}
}

// A5: blocking:true with exit 1 → Fire returns error; no subsequent specs run.
func TestBlocking_AbortsOnNonZero(t *testing.T) {
	ran := false
	cfg := &Config{
		Events: map[Event][]Spec{
			EventPreToolUse: {
				{Command: "exit 1", Timeout: 5 * time.Second, Blocking: true},
				// This second spec must NOT run if the first blocking spec fails.
				{Command: "echo second", Timeout: 5 * time.Second},
			},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	_ = ran

	dir := t.TempDir()
	m, err := NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer func() { _ = m.Close() }()

	_, fireErr := m.Fire(context.Background(), EventPreToolUse, map[string]any{"session_id": "s5"})
	if fireErr == nil {
		t.Fatal("Fire: want error from blocking non-zero exit, got nil")
	}

	// Only 1 log entry (for the blocking spec); the second spec must not have run.
	entries := readLogLines(t, dir)
	if len(entries) != 1 {
		t.Errorf("want 1 log entry (second spec must not run), got %d", len(entries))
	}
}

// A6: no hooks.yaml → no-op manager; Fire returns no error; hooks.log does NOT exist.
func TestNoConfig_CleanRun(t *testing.T) {
	cfg := &Config{} // empty, no events
	dir := t.TempDir()

	m, err := NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer func() { _ = m.Close() }()

	injected, err := m.Fire(context.Background(), EventSessionStart, map[string]any{"session_id": "s6"})
	if err != nil {
		t.Errorf("Fire: unexpected error: %v", err)
	}
	if injected != "" {
		t.Errorf("injected: want empty, got %q", injected)
	}

	logPath := filepath.Join(dir, ".clue-code", "state", "hooks.log")
	if _, statErr := os.Stat(logPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("hooks.log must not exist for no-op manager, got stat err: %v", statErr)
	}
}
