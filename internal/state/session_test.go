package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
)

// TestB2_StaleThreshold verifies that a session is reported as stale when its
// heartbeat mtime is older than 30 s (B2).
func TestB2_StaleThreshold(t *testing.T) {
	clk := clock.Fake(time.Now())

	// Heartbeat written at t=0; advance clock 29s → should be "active".
	hbTime := clk.Now()
	clk.Advance(29 * time.Second)
	got := sessionState(hbTime, clk)
	if got != "active" {
		t.Errorf("at 29s want active, got %q", got)
	}

	// Advance 1 more ms beyond 30s → threshold is exclusive (>) so 30s+1ms is stale.
	clk.Advance(time.Second + time.Millisecond)
	got = sessionState(hbTime, clk)
	if got != "stale" {
		t.Errorf("at 30s+1ms want stale, got %q", got)
	}

	// Advance further → still stale.
	clk.Advance(time.Minute)
	got = sessionState(hbTime, clk)
	if got != "stale" {
		t.Errorf("at 90s want stale, got %q", got)
	}
}

// TestB2_ZeroHeartbeat verifies that a missing heartbeat file yields "ended".
func TestB2_ZeroHeartbeat(t *testing.T) {
	clk := clock.Fake(time.Now())
	got := sessionState(time.Time{}, clk)
	if got != "ended" {
		t.Errorf("zero heartbeat want ended, got %q", got)
	}
}

// TestB3_ListActive verifies that three sessions across three project paths
// are all returned by ListActive (B3).
//
// Fixture is constructed inline rather than checked-in because the canonical
// fixture path uses a `.clue-code/` directory which the project-root
// `.gitignore` excludes (runtime state). Inline construction is also
// hermetic across CI runners.
func TestB3_ListActive(t *testing.T) {
	tmp := t.TempDir()
	sessDir := filepath.Join(tmp, ".clue-code", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	descs := []SessionDescriptor{
		{ID: "sess-abc123", ProjectPath: filepath.Join(tmp, "proj-a"), StartedAt: now, PID: 1001, Skill: "autopilot"},
		{ID: "sess-def456", ProjectPath: filepath.Join(tmp, "proj-b"), StartedAt: now, PID: 1002, Skill: "ralph"},
		{ID: "sess-ghi789", ProjectPath: filepath.Join(tmp, "proj-c"), StartedAt: now, PID: 1003, Skill: "team"},
	}
	data, err := json.Marshal(descs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmp)

	sessions, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("want 3 sessions, got %d: %+v", len(sessions), sessions)
	}
	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	for _, want := range []string{"sess-abc123", "sess-def456", "sess-ghi789"} {
		if !ids[want] {
			t.Errorf("missing session %q in ListActive", want)
		}
	}
}

// TestB5_PendingTasks verifies that GetStatus returns PendingTasks from the
// TeamTaskCounter hook variable (B5).
func TestB5_PendingTasks(t *testing.T) {
	// Set up a fake home with one session in index.
	tmp := t.TempDir()
	sessDir := filepath.Join(tmp, ".clue-code", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}

	desc := SessionDescriptor{
		ID:          "test-sess-001",
		ProjectPath: tmp,
		StartedAt:   time.Now(),
		PID:         os.Getpid(),
	}
	data, _ := json.Marshal([]SessionDescriptor{desc})
	if err := os.WriteFile(filepath.Join(sessDir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Write a heartbeat file so the session appears "active".
	hbDir := filepath.Join(tmp, ".clue-code", "state", "sessions", desc.ID)
	if err := os.MkdirAll(hbDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hbDir, heartbeatFile), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	// Override HOME so ListActive reads our fake index.
	t.Setenv("HOME", tmp)

	// Install a fake TeamTaskCounter returning 3.
	orig := TeamTaskCounter
	TeamTaskCounter = func(_ string) int { return 3 }
	defer func() { TeamTaskCounter = orig }()

	status, err := GetStatus(desc.ID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.PendingTasks != 3 {
		t.Errorf("PendingTasks: want 3, got %d", status.PendingTasks)
	}
}

// TestUpsertIndex_AddAndUpdate checks that upsertIndex adds a new session and
// updates an existing one without duplicating.
func TestUpsertIndex_AddAndUpdate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	d1 := SessionDescriptor{ID: "s1", ProjectPath: "/p1", PID: 1}
	d2 := SessionDescriptor{ID: "s2", ProjectPath: "/p2", PID: 2}

	if err := upsertIndex(d1); err != nil {
		t.Fatal(err)
	}
	if err := upsertIndex(d2); err != nil {
		t.Fatal(err)
	}

	sessions, err := ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2, got %d", len(sessions))
	}

	// Update d1.
	d1.Skill = "ralph"
	if err := upsertIndex(d1); err != nil {
		t.Fatal(err)
	}
	sessions, _ = ListActive()
	if len(sessions) != 2 {
		t.Fatalf("after update: want 2, got %d", len(sessions))
	}
	for _, s := range sessions {
		if s.ID == "s1" && s.Skill != "ralph" {
			t.Errorf("update not persisted: %+v", s)
		}
	}
}
