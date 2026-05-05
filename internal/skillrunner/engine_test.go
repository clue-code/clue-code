package skillrunner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/hooks"
)

// makeManager builds a Manager backed by a temp project dir.
func makeManager(t *testing.T, cfg *hooks.Config) (*hooks.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	m, err := hooks.NewManager(cfg, dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, dir
}

// logEntries reads all NDJSON lines from hooks.log in projectDir.
func logEntries(t *testing.T, projectDir string) []hooks.LogEntry {
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

// E1: malformed-yaml-skill is skipped; synthetic-skill IS loaded.
func TestLoad_SkipMalformedSkills(t *testing.T) {
	e := NewEngine(nil)
	err := e.Load("testdata")
	if err == nil {
		t.Fatal("Load: want LoadErrors for malformed skill, got nil")
	}
	var le LoadErrors
	if !errors.As(err, &le) {
		t.Fatalf("Load: want LoadErrors, got %T: %v", err, err)
	}
	if len(le) == 0 {
		t.Fatal("LoadErrors: want at least 1 error")
	}
	// malformed-yaml-skill must appear in the error message.
	found := false
	for _, e := range le {
		if strings.Contains(e.Error(), "malformed-yaml-skill") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("LoadErrors: want reference to malformed-yaml-skill, got: %v", le)
	}
	// synthetic-skill must have been loaded despite the error.
	if e.skills["synthetic-skill"] == nil {
		t.Error("synthetic-skill: want loaded, got nil")
	}
}

// E2: recursion guard — at MaxSkillDepth the 5th call returns ErrSkillDepthExceeded.
func TestRun_RecursionGuard(t *testing.T) {
	e := NewEngine(nil)
	e.skills["synthetic-skill"] = &Skill{Name: "synthetic-skill"}

	// Simulate being at max depth via env var.
	t.Setenv(EnvSkillDepth, fmt.Sprintf("%d", MaxSkillDepth))

	err := e.Run(context.Background(), "synthetic-skill", nil)
	if !errors.Is(err, ErrSkillDepthExceeded) {
		t.Errorf("want ErrSkillDepthExceeded, got %v", err)
	}
}

// E3: panic in skill body → Run recovers, returns wrapped error, SessionStart fired BEFORE panic, Stop fired AFTER.
func TestRun_LifecycleHooksAroundPanic(t *testing.T) {
	cfg := &hooks.Config{
		Events: map[hooks.Event][]hooks.Spec{
			hooks.EventSessionStart: {{Command: "echo start", Timeout: 5 * time.Second}},
			hooks.EventStop:         {{Command: "echo stop", Timeout: 5 * time.Second}},
		},
	}
	hm, projectDir := makeManager(t, cfg)
	e := NewEngine(hm)
	e.skills["synthetic-skill"] = &Skill{Name: "synthetic-skill"}
	e.WithRunFunc(func(ctx context.Context, skill *Skill, args []string) error {
		panic("intentional test panic")
	})

	err := e.Run(context.Background(), "synthetic-skill", nil)
	if err == nil {
		t.Fatal("Run: want error from panic recovery, got nil")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("Run error: want 'panic' in message, got %q", err.Error())
	}

	entries := logEntries(t, projectDir)
	if len(entries) < 2 {
		t.Fatalf("hooks.log: want >=2 entries (SessionStart + Stop), got %d", len(entries))
	}
	events := make([]string, len(entries))
	for i, en := range entries {
		events[i] = string(en.Event)
	}
	if events[0] != string(hooks.EventSessionStart) {
		t.Errorf("entry[0]: want SessionStart, got %s", events[0])
	}
	if events[len(events)-1] != string(hooks.EventStop) {
		t.Errorf("entry[last]: want Stop, got %s", events[len(events)-1])
	}
}

// E4: ctx cancellation → Run returns within 200ms with ctx.Err(), Stop fires exactly once.
func TestRun_GracefulCancel(t *testing.T) {
	cfg := &hooks.Config{
		Events: map[hooks.Event][]hooks.Spec{
			hooks.EventSessionStart: {{Command: "echo start", Timeout: 5 * time.Second}},
			hooks.EventStop:         {{Command: "echo stop", Timeout: 5 * time.Second}},
		},
	}
	hm, projectDir := makeManager(t, cfg)
	e := NewEngine(hm)
	e.skills["synthetic-skill"] = &Skill{Name: "synthetic-skill"}
	e.WithRunFunc(func(ctx context.Context, skill *Skill, args []string) error {
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := e.Run(ctx, "synthetic-skill", nil)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("Run took %v, want ≤200ms", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run: want context.Canceled, got %v", err)
	}

	// Stop must have fired exactly once.
	entries := logEntries(t, projectDir)
	stopCount := 0
	for _, en := range entries {
		if en.Event == hooks.EventStop {
			stopCount++
		}
	}
	if stopCount != 1 {
		t.Errorf("Stop hook: want fired 1 time, got %d (entries: %v)", stopCount, entries)
	}
}

// E5: all 5 lifecycle events fire in order: SessionStart, PreToolUse, UserPromptSubmit, PostToolUse, Stop.
func TestEngine_AllLifecycleHooksFire(t *testing.T) {
	cfg := &hooks.Config{
		Events: map[hooks.Event][]hooks.Spec{
			hooks.EventSessionStart:     {{Command: "echo start", Timeout: 5 * time.Second}},
			hooks.EventPreToolUse:       {{Command: "echo pre", Timeout: 5 * time.Second}},
			hooks.EventUserPromptSubmit: {{Command: "echo prompt", Timeout: 5 * time.Second}},
			hooks.EventPostToolUse:      {{Command: "echo post", Timeout: 5 * time.Second}},
			hooks.EventStop:             {{Command: "echo stop", Timeout: 5 * time.Second}},
		},
	}
	hm, projectDir := makeManager(t, cfg)
	e := NewEngine(hm)
	e.skills["synthetic-skill"] = &Skill{Name: "synthetic-skill"}

	// RunFunc triggers mid-run events (PreToolUse, UserPromptSubmit, PostToolUse).
	e.WithRunFunc(func(ctx context.Context, skill *Skill, args []string) error {
		sessionID := fmt.Sprintf("skill-%s-0", skill.Name)
		payload := map[string]any{"session_id": sessionID, "skill": skill.Name, "args": args}
		if _, err := hm.Fire(ctx, hooks.EventPreToolUse, payload); err != nil {
			return err
		}
		if _, err := hm.Fire(ctx, hooks.EventUserPromptSubmit, payload); err != nil {
			return err
		}
		if _, err := hm.Fire(ctx, hooks.EventPostToolUse, payload); err != nil {
			return err
		}
		return nil
	})

	if err := e.Run(context.Background(), "synthetic-skill", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries := logEntries(t, projectDir)
	if len(entries) != 5 {
		t.Fatalf("want 5 log entries, got %d: %v", len(entries), entries)
	}
	want := []hooks.Event{
		hooks.EventSessionStart,
		hooks.EventPreToolUse,
		hooks.EventUserPromptSubmit,
		hooks.EventPostToolUse,
		hooks.EventStop,
	}
	for i, en := range entries {
		if en.Event != want[i] {
			t.Errorf("entry[%d]: want %s, got %s", i, want[i], en.Event)
		}
	}
}
