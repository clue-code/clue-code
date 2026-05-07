package aider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClient_Available_NoBinary verifies that NewClient returns !Available()
// when the aider binary cannot be found on PATH.
func TestClient_Available_NoBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // directory with no binaries
	c := NewClient()
	if c.Available() {
		t.Fatal("expected Available() == false when binary not on PATH")
	}
	if c.Version() != "" {
		t.Fatalf("expected empty Version(), got %q", c.Version())
	}
}

// TestClient_Apply_NotAvailable verifies that Apply returns ErrAiderNotAvailable
// immediately when the client has no binary (no subprocess is spawned).
func TestClient_Apply_NotAvailable(t *testing.T) {
	c := &Client{available: false}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := c.Apply(ctx, "add a comment", t.TempDir())
	if err != ErrAiderNotAvailable {
		t.Fatalf("expected ErrAiderNotAvailable, got %v", err)
	}
}

// TestClient_Apply_Crash exercises the crash-recovery path: when the subprocess
// exits non-zero, Apply must return an error without panicking.
func TestClient_Apply_Crash(t *testing.T) {
	dir := t.TempDir()
	fakeBin := writeFakeAider(t, dir, 1 /*exit code*/, "fatal: something went wrong\n")

	c := &Client{
		available: true,
		binPath:   fakeBin,
		version:   "fake 0.0.0",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := c.Apply(ctx, "do something", dir)
	if err == nil {
		t.Fatal("expected non-nil error from crashing aider subprocess")
	}
}

// writeFakeAider writes a shell script to dir that exits with exitCode and
// prints output to stderr, then returns its path.
func writeFakeAider(t *testing.T, dir string, exitCode int, output string) string {
	t.Helper()
	script := "#!/bin/sh\n"
	if output != "" {
		script += "echo '" + output + "' >&2\n"
	}
	script += "exit " + itoa(exitCode) + "\n"

	binPath := filepath.Join(dir, "aider")
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("writeFakeAider: %v", err)
	}
	return binPath
}

// itoa converts an int to its decimal string representation without importing
// strconv (keeps the helper self-contained).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
