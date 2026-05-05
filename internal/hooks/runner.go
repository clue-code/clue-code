package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	EnvHookDepth   = "CLUE_CODE_HOOK_DEPTH"
	MaxHookDepth   = 3
	MaxOutputBytes = 64 * 1024 // 64 KB
)

// ErrHookDepthExceeded is returned when the hook recursion depth would exceed
// MaxHookDepth, preventing infinite hook→hook→hook loops.
var ErrHookDepthExceeded = errors.New("hooks: depth exceeded")

// Result captures the outcome of a single hook subprocess execution.
type Result struct {
	ExitCode   int
	Stdout     []byte
	Stderr     []byte
	DurationMS int64
	TimedOut   bool
	Truncated  bool
}

// runSpec executes spec as a subprocess and returns the result.
// It honours the hook depth guard, the configured timeout, and the 64 KB
// stdout cap.
func runSpec(ctx context.Context, spec Spec) (Result, error) {
	// --- recursion guard ---
	parentDepth, err := hookDepth()
	if err != nil {
		return Result{}, fmt.Errorf("hooks: read depth env: %w", err)
	}
	if parentDepth >= MaxHookDepth {
		return Result{}, ErrHookDepthExceeded
	}

	// --- timeout ---
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// --- command ---
	cmd := exec.CommandContext(tctx, "sh", "-c", spec.Command)

	// Propagate env and increment depth for child.
	cmd.Env = childEnv(parentDepth + 1)

	// Cap the post-cancel I/O drain so a forked grandchild (e.g. `sh -c
	// 'sleep 60'` on Linux/dash where the child is forked rather than
	// exec-replaced) cannot keep stdout/stderr pipes open after the shell
	// is SIGKILLed. Without WaitDelay, Wait() blocks until the orphan
	// finishes naturally — observed as 60s "TimedOut" tests on ubuntu CI.
	cmd.WaitDelay = 2 * time.Second

	// Capture stdout with a bounded reader; capture stderr unbounded (it is
	// internal diagnostic output and is not forwarded to the user).
	var stdoutBuf boundedBuffer
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	result := Result{
		Stdout:     stdoutBuf.Bytes(),
		Stderr:     []byte(stderrBuf.String()),
		DurationMS: elapsed.Milliseconds(),
		Truncated:  stdoutBuf.truncated,
	}

	// Detect timeout vs other errors.
	if errors.Is(tctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.ExitCode = -1
		return result, nil
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("hooks: exec: %w", runErr)
	}
	result.ExitCode = 0
	return result, nil
}

// hookDepth parses CLUE_CODE_HOOK_DEPTH from the current process environment.
// Returns 0 if the variable is absent or unparseable.
func hookDepth() (int, error) {
	val := os.Getenv(EnvHookDepth)
	if val == "" {
		return 0, nil
	}
	d, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", EnvHookDepth, val, err)
	}
	return d, nil
}

// childEnv builds the environment for a child hook process: inherits the
// current process env, then overrides CLUE_CODE_HOOK_DEPTH with childDepth.
func childEnv(childDepth int) []string {
	parent := os.Environ()
	depthKV := EnvHookDepth + "=" + strconv.Itoa(childDepth)
	out := make([]string, 0, len(parent)+1)
	replaced := false
	for _, kv := range parent {
		if strings.HasPrefix(kv, EnvHookDepth+"=") {
			out = append(out, depthKV)
			replaced = true
		} else {
			out = append(out, kv)
		}
	}
	if !replaced {
		out = append(out, depthKV)
	}
	return out
}

// boundedBuffer is an io.Writer that captures at most MaxOutputBytes bytes and
// sets truncated=true when the limit is reached.
type boundedBuffer struct {
	buf       []byte
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := MaxOutputBytes - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil // consume without storing
	}
	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	return b.buf
}

// Ensure boundedBuffer satisfies io.Writer at compile time.
var _ io.Writer = (*boundedBuffer)(nil)
