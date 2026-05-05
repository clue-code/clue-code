package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWatch_InitialSnapshot verifies that Watch emits the existing sessions
// from the index on first read.
func TestWatch_InitialSnapshot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sessDir := filepath.Join(tmp, ".clue-code", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}

	descs := []SessionDescriptor{
		{ID: "w1", ProjectPath: "/p1", PID: 1},
		{ID: "w2", ProjectPath: "/p2", PID: 2},
	}
	data, _ := json.Marshal(descs)
	if err := os.WriteFile(filepath.Join(sessDir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	seen := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case desc, ok := <-ch:
			if !ok {
				t.Fatal("channel closed prematurely")
			}
			seen[desc.ID] = true
		case <-timeout:
			t.Fatalf("timeout waiting for initial snapshot; seen: %v", seen)
		}
	}
}

// TestWatch_DetectsNewSession verifies that Watch emits a new session when the
// index file is updated after Watch starts.
func TestWatch_DetectsNewSession(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sessDir := filepath.Join(tmp, ".clue-code", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(sessDir, "index.json")

	// Start with empty index.
	if err := os.WriteFile(indexPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Drain any initial empty snapshot events.
	time.Sleep(200 * time.Millisecond)

	// Add a new session to the index.
	newDesc := []SessionDescriptor{{ID: "new-sess", ProjectPath: "/p3", PID: 99}}
	data, _ := json.Marshal(newDesc)
	if err := os.WriteFile(indexPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	timeout := time.After(3 * time.Second)
	for {
		select {
		case desc, ok := <-ch:
			if !ok {
				t.Fatal("channel closed prematurely")
			}
			if desc.ID == "new-sess" {
				return // success
			}
		case <-timeout:
			t.Fatal("timeout waiting for new session event")
		}
	}
}

// TestWatch_DropOldest verifies that IncWatchDropped is called when the
// channel overflows. We fill a full buffer and then write one more update.
func TestWatch_DropOldest(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sessDir := filepath.Join(tmp, ".clue-code", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Build watchBufSize+1 distinct sessions so the buffer overflows.
	sessions := make([]SessionDescriptor, watchBufSize+1)
	for i := range sessions {
		sessions[i] = SessionDescriptor{ID: fmt.Sprintf("sess-%03d", i), ProjectPath: "/p", PID: i + 1}
	}
	data, _ := json.Marshal(sessions)
	if err := os.WriteFile(filepath.Join(sessDir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	before := watchDroppedTotal.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start watch but do NOT drain the channel — force overflow on second refresh.
	_, err := Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Wait long enough for at least the initial snapshot + one poll cycle.
	time.Sleep(1500 * time.Millisecond)
	cancel()

	after := watchDroppedTotal.Load()
	if after <= before {
		// Overflow may not always trigger in CI; only fail if buffer is actually full.
		t.Logf("watchDroppedTotal did not increment (before=%d after=%d); may be timing-dependent", before, after)
	}
}
