package hooks

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunSpec_Success(t *testing.T) {
	t.Setenv(EnvHookDepth, "")
	spec := Spec{Command: "echo hello", Timeout: 5 * time.Second}
	res, err := runSpec(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code: got %d, want 0", res.ExitCode)
	}
	got := strings.TrimSpace(string(res.Stdout))
	if got != "hello" {
		t.Errorf("stdout: got %q, want %q", got, "hello")
	}
	if res.TimedOut {
		t.Error("TimedOut should be false")
	}
	if res.Truncated {
		t.Error("Truncated should be false")
	}
}

func TestRunSpec_Timeout(t *testing.T) {
	t.Setenv(EnvHookDepth, "")
	spec := Spec{Command: "sleep 10", Timeout: 200 * time.Millisecond}
	res, err := runSpec(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.TimedOut {
		t.Error("expected TimedOut=true")
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code: got %d, want -1", res.ExitCode)
	}
}

func TestRunSpec_DepthGuard(t *testing.T) {
	// Simulate being inside a hook at max depth.
	t.Setenv(EnvHookDepth, "3")
	spec := Spec{Command: "echo should-not-run", Timeout: 5 * time.Second}
	_, err := runSpec(context.Background(), spec)
	if !errors.Is(err, ErrHookDepthExceeded) {
		t.Errorf("expected ErrHookDepthExceeded, got %v", err)
	}
}

func TestRunSpec_DepthIncrement(t *testing.T) {
	// Parent depth is unset (0); child should see depth=1.
	t.Setenv(EnvHookDepth, "")
	spec := Spec{Command: `printf "%s" "$CLUE_CODE_HOOK_DEPTH"`, Timeout: 5 * time.Second}
	res, err := runSpec(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code: got %d, want 0", res.ExitCode)
	}
	got := strings.TrimSpace(string(res.Stdout))
	if got != "1" {
		t.Errorf("CLUE_CODE_HOOK_DEPTH in child: got %q, want %q", got, "1")
	}
}

func TestRunSpec_OutputTruncation(t *testing.T) {
	t.Setenv(EnvHookDepth, "")
	// Generate ~200KB of output via dd or a shell loop.
	spec := Spec{
		// Write 200*1024 'x' bytes in 1KB chunks.
		Command: `dd if=/dev/zero bs=1024 count=200 2>/dev/null | tr '\0' 'x'`,
		Timeout: 10 * time.Second,
	}
	res, err := runSpec(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Truncated {
		t.Error("expected Truncated=true")
	}
	if len(res.Stdout) != MaxOutputBytes {
		t.Errorf("stdout len: got %d, want %d", len(res.Stdout), MaxOutputBytes)
	}
	if !bytes.Equal(res.Stdout, bytes.Repeat([]byte("x"), MaxOutputBytes)) {
		t.Error("stdout content does not match expected truncated output")
	}
}
