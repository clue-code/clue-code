package aider

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const (
	// sigKillDelay is how long after SIGTERM we wait before escalating to SIGKILL.
	sigKillDelay = 3 * time.Second
)

// runAider executes the aider binary with --no-auto-commit --yes and the given
// instruction. It captures stdout and stderr separately.
//
// binPath must be the absolute path to the aider binary (as resolved by
// exec.LookPath at client init time).
//
// Context cancellation is respected: the function sends SIGTERM first, then
// escalates to SIGKILL after sigKillDelay to avoid zombie processes.
//
// On a non-zero exit the error wraps the captured stderr for diagnosis.
func runAider(ctx context.Context, binPath, repoRoot, instruction string) (output []byte, err error) {
	cmd := exec.CommandContext(ctx, binPath,
		"--no-auto-commit",
		"--yes",
		"--message", instruction,
	)
	cmd.Dir = repoRoot

	// WaitDelay gives the process a grace period after the context is done
	// before exec.CommandContext sends SIGKILL. This mirrors the linux-dash
	// workaround established in Phase 4.6.
	cmd.WaitDelay = 5 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the process so we can handle escalation ourselves.
	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("aider: start failed: %w", startErr)
	}

	// Escalation goroutine: SIGTERM → (3 s) → SIGKILL.
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			// Send SIGTERM first for a graceful shutdown.
			_ = sendSignal(cmd.Process, sigTermSignal)
			// After the grace period, escalate to SIGKILL.
			timer := time.NewTimer(sigKillDelay)
			defer timer.Stop()
			select {
			case <-timer.C:
				_ = sendSignal(cmd.Process, sigKillSignal)
			case <-waitDone(cmd):
				// Process already exited — nothing to kill.
			}
		case <-waitDone(cmd):
			// Normal exit before context cancellation.
		}
	}()

	waitErr := cmd.Wait()
	<-done // Ensure the goroutine has exited before we return.

	if waitErr != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("aider: exited with error: %w; stderr: %s", waitErr, stderrStr)
		}
		return nil, fmt.Errorf("aider: exited with error: %w", waitErr)
	}

	return stdout.Bytes(), nil
}

// waitDone returns a channel that is closed when cmd's process exits.
// This is a lightweight helper to avoid blocking the escalation goroutine.
func waitDone(cmd *exec.Cmd) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		// ProcessState is set by cmd.Wait(); we poll Process.Wait here to get
		// an independent signal. Errors are intentionally ignored.
		if cmd.Process != nil {
			_, _ = cmd.Process.Wait()
		}
		close(ch)
	}()
	return ch
}
