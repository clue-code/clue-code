package team

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// SubprocessTransport implements Transport by spawning a clue-code team-worker
// subprocess. NDJSON envelopes are exchanged over stdin/stdout. The subprocess
// stderr is redirected to a rotating log file via lumberjack.
type SubprocessTransport struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderrLog *lumberjack.Logger
	scanner   *bufio.Scanner
	mu        sync.Mutex // serialises Send
	closed    atomic.Bool
	waitDone  chan struct{}
	waitErr   error
}

// NewSubprocessTransport spawns a new clue-code team-worker subprocess for
// the given teamID and workerID. The current executable is used (via
// os.Executable) — PATH lookup is never performed.
//
// Subprocess stderr is written to:
//
//	<projectRoot>/.clue-code/teams/<teamID>/workers/<workerID>/stderr.log
//
// with lumberjack rotation (10 MiB × 3 backups).
func NewSubprocessTransport(teamID, workerID, projectRoot string) (Transport, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("team: subprocess: resolve executable: %w", err)
	}

	workersDir := filepath.Join(projectRoot, ".clue-code", "teams", teamID, "workers", workerID)
	if err := os.MkdirAll(workersDir, 0o700); err != nil {
		return nil, fmt.Errorf("team: subprocess: mkdir workers dir: %w", err)
	}

	stderrLogPath := filepath.Join(workersDir, "stderr.log")
	stderrLog := &lumberjack.Logger{
		Filename:   stderrLogPath,
		MaxSize:    10, // 10 MiB per file
		MaxBackups: 3,
	}

	cmd := exec.Command(exe, "team-worker",
		"--team-id="+teamID,
		"--worker-id="+workerID,
		"--project-root="+projectRoot,
	)
	cmd.Env = append(os.Environ(), IncrementedDepthEnv()...)
	cmd.Stderr = stderrLog

	// WaitDelay ensures that any forked children of the subprocess (linux dash
	// workaround) are reaped within 2 seconds after the process exits.
	cmd.WaitDelay = 2 * time.Second

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stderrLog.Close()
		return nil, fmt.Errorf("team: subprocess: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stderrLog.Close()
		return nil, fmt.Errorf("team: subprocess: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderrLog.Close()
		return nil, fmt.Errorf("team: subprocess: start: %w", err)
	}

	t := &SubprocessTransport{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderrLog: stderrLog,
		scanner:   newScanner(stdout),
		waitDone:  make(chan struct{}),
	}

	go func() {
		t.waitErr = cmd.Wait()
		close(t.waitDone)
		_ = stderrLog.Close()
	}()

	return t, nil
}

// Send serialises env as a single NDJSON line and writes it to the subprocess
// stdin. Concurrent calls are serialised by an internal mutex.
func (t *SubprocessTransport) Send(env Envelope) error {
	if t.closed.Load() {
		return fmt.Errorf("team: subprocess transport: send on closed transport")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := EncodeEnvelope(t.stdin, env); err != nil {
		return fmt.Errorf("team: subprocess send: %w", err)
	}
	return nil
}

// Recv reads the next Envelope from the subprocess stdout. It blocks until
// data arrives, the transport is closed, or the subprocess exits.
func (t *SubprocessTransport) Recv() (Envelope, error) {
	env, err := DecodeNext(t.scanner)
	if err != nil {
		if err == io.EOF {
			return Envelope{}, io.EOF
		}
		return Envelope{}, fmt.Errorf("team: subprocess recv: %w", err)
	}
	return env, nil
}

// Close gracefully shuts down the subprocess. It signals SIGTERM, waits up to
// 3 seconds for a clean exit, then escalates to SIGKILL. Idempotent.
func (t *SubprocessTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil // already closed
	}

	// Close stdin so the worker sees EOF and begins shutdown.
	_ = t.stdin.Close()

	// Send SIGTERM to the process group.
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(syscall.SIGTERM)
	}

	// Wait up to 3 seconds for clean exit.
	select {
	case <-t.waitDone:
		// clean exit
	case <-time.After(3 * time.Second):
		// Escalate to SIGKILL.
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Signal(syscall.SIGKILL)
		}
		// Wait for the kill to take effect.
		<-t.waitDone
	}

	return nil
}
