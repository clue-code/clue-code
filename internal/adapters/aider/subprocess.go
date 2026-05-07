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

	// processDone is closed by the single cmd.Wait() call on the main path
	// below. The escalation goroutine selects on this channel instead of
	// calling Process.Wait() itself — on POSIX only one waiter per process is
	// allowed, so a second concurrent Wait produces undefined behaviour.
	processDone := make(chan struct{})

	// Escalation goroutine: SIGTERM → (3 s) → SIGKILL.
	// It never calls cmd.Wait() or cmd.Process.Wait(); it relies solely on
	// processDone which is closed by the single authoritative Wait below.
	escalationDone := make(chan struct{})
	go func() {
		defer close(escalationDone)
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
			case <-processDone:
				// Process already exited — nothing to kill.
			}
		case <-processDone:
			// Normal exit before context cancellation.
		}
	}()

	// Single authoritative Wait. Closing processDone unblocks the escalation
	// goroutine so it can exit cleanly.
	waitErr := cmd.Wait()
	close(processDone)
	<-escalationDone // Ensure the goroutine has exited before we return.

	if waitErr != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("aider: exited with error: %w; stderr: %s", waitErr, stderrStr)
		}
		return nil, fmt.Errorf("aider: exited with error: %w", waitErr)
	}

	return stdout.Bytes(), nil
}
