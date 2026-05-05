package state_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/state"
)

// openTempStore opens a Store rooted in a fresh temp directory.
// It redirects both $HOME and cwd so all scopes (global, project, session)
// write into the temp dir rather than the real home or project root.
func openTempStore(t *testing.T, sessionID string) state.Store {
	t.Helper()
	dir := t.TempDir()
	// Make the temp dir look like a project root so findProjectRoot picks it up.
	if err := os.MkdirAll(filepath.Join(dir, ".clue-code"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Redirect HOME so ScopeGlobal writes into the temp dir.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Change working directory so findProjectRoot finds dir.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	st, err := state.Open(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestReadWrite verifies basic round-trip for all scopes.
func TestReadWrite(t *testing.T) {
	ctx := context.Background()
	scopes := []state.Scope{state.ScopeGlobal, state.ScopeProject, state.ScopeSession}
	for _, sc := range scopes {
		sc := sc
		t.Run(sc.String(), func(t *testing.T) {
			st := openTempStore(t, "sess-rw")
			val := []byte("hello-" + sc.String())
			v, err := st.Write(ctx, "mykey", val, sc)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if v != 1 {
				t.Fatalf("expected version 1, got %d", v)
			}
			got, ver, exists, err := st.Read(ctx, "mykey", sc)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if !exists {
				t.Fatal("expected key to exist")
			}
			if ver != 1 {
				t.Fatalf("expected version 1, got %d", ver)
			}
			if string(got) != string(val) {
				t.Fatalf("expected %q, got %q", val, got)
			}
		})
	}
}

// TestReadMissing verifies Read returns exists=false for unknown keys.
func TestReadMissing(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-miss")
	_, _, exists, err := st.Read(ctx, "no-such-key", state.ScopeProject)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false for missing key")
	}
}

// TestVersionIncrement verifies each Write increments the version.
func TestVersionIncrement(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-ver")
	for i := uint64(1); i <= 5; i++ {
		v, err := st.Write(ctx, "k", []byte("val"), state.ScopeProject)
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
		if v != i {
			t.Fatalf("expected version %d, got %d", i, v)
		}
	}
}

// TestWriteIfVersion_Match verifies successful CAS write.
func TestWriteIfVersion_Match(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-cas")
	v, err := st.Write(ctx, "k", []byte("initial"), state.ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := st.WriteIfVersion(ctx, "k", []byte("updated"), v, state.ScopeProject)
	if err != nil {
		t.Fatalf("WriteIfVersion: %v", err)
	}
	if v2 != v+1 {
		t.Fatalf("expected version %d, got %d", v+1, v2)
	}
}

// B4: WriteIfVersion with wrong expected version returns ErrVersionMismatch.
func TestWriteIfVersion_Mismatch(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-b4")

	// Write 7 times so version=7.
	for i := 0; i < 7; i++ {
		if _, err := st.Write(ctx, "k", []byte("v"), state.ScopeProject); err != nil {
			t.Fatal(err)
		}
	}
	_, err := st.WriteIfVersion(ctx, "k", []byte("new"), 5, state.ScopeProject)
	if err == nil {
		t.Fatal("expected ErrVersionMismatch, got nil")
	}
	if err != state.ErrVersionMismatch {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}
}

// B1: Two concurrent Write calls succeed; final version equals 2.
func TestConcurrentWrite(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-b1")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = st.Write(ctx, "k", []byte(fmt.Sprintf("val%d", idx)), state.ScopeProject)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	_, ver, exists, err := st.Read(ctx, "k", state.ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("key not found after concurrent writes")
	}
	if ver != 2 {
		t.Fatalf("expected version 2, got %d", ver)
	}
}

// B6: Clear removes only keys with matching prefix.
func TestClear_Prefix(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-b6")

	teamKeys := []string{"team/a", "team/b", "team/c", "team/d", "team/e"}
	otherKeys := []string{"other/x", "other/y"}

	for _, k := range teamKeys {
		if _, err := st.Write(ctx, k, []byte("v"), state.ScopeSession); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range otherKeys {
		if _, err := st.Write(ctx, k, []byte("v"), state.ScopeSession); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := st.Clear(ctx, state.ScopeSession, "team/")
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if removed != 5 {
		t.Fatalf("expected 5 removed, got %d", removed)
	}

	// Verify team keys are gone, other keys intact.
	for _, k := range teamKeys {
		_, _, exists, err := st.Read(ctx, k, state.ScopeSession)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("key %q should have been cleared", k)
		}
	}
	for _, k := range otherKeys {
		_, _, exists, err := st.Read(ctx, k, state.ScopeSession)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("key %q should still exist", k)
		}
	}
}

// B7: 8 concurrent WriteWithRetry all succeed within 5s, zero ErrStateBusy.
func TestWriteContention_NoBusyErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	st := openTempStore(t, "sess-b7")

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = st.WriteWithRetry(ctx, "shared", []byte(fmt.Sprintf("writer-%d", idx)), state.ScopeProject)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, err)
		}
	}
	_, ver, _, err := st.Read(ctx, "shared", state.ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if ver != n {
		t.Fatalf("expected version %d (one per writer), got %d", n, ver)
	}
}

// B8: Two concurrent Append calls both visible in file order with intact headers.
func TestAppend_TwoWritersIntact(t *testing.T) {
	ctx := context.Background()
	st := openTempStore(t, "sess-b8")

	var wg sync.WaitGroup
	writeAppend := func(content string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := st.Append(ctx, "notepad", []byte(content), state.ScopeSession); err != nil {
				t.Errorf("Append: %v", err)
			}
		}()
	}

	writeAppend("## skill-X @ 1000\ncontent X\n")
	writeAppend("## skill-Y @ 1001\ncontent Y\n")
	wg.Wait()

	// Read the resulting file.
	dir := t.TempDir() // only to derive appendPath — we need the actual path
	// The file lives at <sessionDir>/notepad relative to the session kv.json dir.
	// Re-derive: Read an existing key to get the path indirectly via the store.
	// Instead: use the session scope path directly.
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	appendPath := filepath.Join(root, ".clue-code", "state", "sessions", "sess-b8", "notepad")
	_ = dir // unused

	data, err := os.ReadFile(appendPath)
	if err != nil {
		t.Fatalf("read notepad: %v", err)
	}
	content := string(data)

	rX := regexp.MustCompile(`## skill-X @ \d+`)
	rY := regexp.MustCompile(`## skill-Y @ \d+`)
	if !rX.MatchString(content) {
		t.Errorf("skill-X header not found in notepad:\n%s", content)
	}
	if !rY.MatchString(content) {
		t.Errorf("skill-Y header not found in notepad:\n%s", content)
	}
}
