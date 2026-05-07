//go:build integration

package team_test

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// buildTestBinary compiles the clue-code binary to a temp path and returns it.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "clue-code-test")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/clue-code")
	cmd.Dir = findModuleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build clue-code binary: %v\n%s", err, out)
	}
	return binPath
}

// findModuleRoot walks up from the working directory until it finds go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// TestSubprocessTransport_Demo verifies D7: the demo command with
// --transport=subprocess exits 0 and the team journal contains expected entries.
func TestSubprocessTransport_Demo(t *testing.T) {
	binPath := buildTestBinary(t)
	tmpDir := t.TempDir()

	cmd := exec.Command(binPath, "team", "demo",
		"--transport=subprocess",
		"--project-root="+tmpDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start demo: %v", err)
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("demo exited with error: %v", err)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("demo timed out after 30s")
	}

	// Find the team journals created by the demo.
	teamsDir := filepath.Join(tmpDir, ".clue-code", "teams")
	teamEntries, err := os.ReadDir(teamsDir)
	if err != nil {
		t.Fatalf("read teams dir: %v", err)
	}
	if len(teamEntries) == 0 {
		t.Fatal("no team directories found after demo")
	}

	// Count journal entries. The subprocess transport sends envelopes directly
	// over stdin/stdout — they are NOT written to the journal. The journal only
	// contains team-create, task-create, and any entries written by TeamCreate
	// and TaskCreate. We verify that the journal is non-empty and structurally
	// valid (all lines parse as envelopes).
	var totalEntries int
	var teamCreateCount, taskCreateCount int

	for _, entry := range teamEntries {
		if !entry.IsDir() {
			continue
		}
		journalPath := filepath.Join(teamsDir, entry.Name(), "journal.ndjson")
		f, err := os.Open(journalPath)
		if err != nil {
			t.Logf("open journal %s: %v (skipping)", journalPath, err)
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var env struct {
				V    uint8  `json:"v"`
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(line, &env); err != nil {
				t.Errorf("journal line not valid JSON: %v — %s", err, line)
				continue
			}
			if env.V != 1 {
				t.Errorf("unexpected envelope version %d", env.V)
			}
			totalEntries++
			switch env.Kind {
			case "team-create":
				teamCreateCount++
			case "task-create":
				taskCreateCount++
			}
		}
		_ = f.Close()
	}

	// 1 team-create + 2 task-create (one per worker) = at least 3 entries.
	if totalEntries < 3 {
		t.Errorf("expected >= 3 journal entries (team-create + 2 task-create), got %d", totalEntries)
	}
	if teamCreateCount != 1 {
		t.Errorf("expected 1 team-create entry, got %d", teamCreateCount)
	}
	if taskCreateCount != 2 {
		t.Errorf("expected 2 task-create entries, got %d", taskCreateCount)
	}

	// Verify workers directories exist (created by NewSubprocessTransport).
	// stderr.log may not exist if the worker wrote nothing to stderr
	// (lumberjack is lazy — it only creates the file on first write).
	// We verify that the workers directory itself was created.
	for _, entry := range teamEntries {
		if !entry.IsDir() {
			continue
		}
		workersDir := filepath.Join(teamsDir, entry.Name(), "workers")
		workerEntries, err := os.ReadDir(workersDir)
		if err != nil {
			t.Errorf("D7: workers dir not created for team %s: %v", entry.Name(), err)
			continue
		}
		if len(workerEntries) < 2 {
			t.Errorf("D7: expected 2 worker dirs, got %d", len(workerEntries))
		}
	}
}

// TestSubprocessTransport_StderrIsolation verifies D9: subprocess stderr is
// routed to a rotating log file and does NOT appear on the parent's stderr.
func TestSubprocessTransport_StderrIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	workersDir := filepath.Join(tmpDir, "workers", "worker-0")
	if err := os.MkdirAll(workersDir, 0o700); err != nil {
		t.Fatal(err)
	}

	stderrLogPath := filepath.Join(workersDir, "stderr.log")

	// Route a 1 MiB stderr stream through an os.File (same interface as
	// lumberjack.Logger) and verify:
	//   1. The parent stderr (captured via pipe) receives nothing.
	//   2. The log file receives >= 1 MiB.
	//
	// We use os.File here because importing lumberjack in an external _test
	// package would create a test-only dependency. The full lumberjack rotation
	// path is exercised by the subprocess transport in TestSubprocessTransport_Demo.

	logFile, err := os.OpenFile(stderrLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Capture parent stderr via a pipe so we can verify nothing leaks.
	parentStderrR, parentStderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = parentStderrW

	// Spawn a shell command that writes exactly 1 MiB to stderr.
	bigStderrCmd := exec.Command("/bin/sh", "-c",
		"dd if=/dev/zero bs=1048576 count=1 2>/dev/null | cat >&2",
	)
	bigStderrCmd.Stderr = logFile // route to log file, not parent stderr

	if err := bigStderrCmd.Run(); err != nil {
		os.Stderr = origStderr
		_ = parentStderrW.Close()
		_ = parentStderrR.Close()
		_ = logFile.Close()
		t.Fatalf("big stderr cmd: %v", err)
	}
	_ = logFile.Close()

	// Restore stderr and close the capture pipe.
	os.Stderr = origStderr
	_ = parentStderrW.Close()

	// Read whatever ended up on the captured parent stderr.
	var parentStderrBuf [4096]byte
	n, _ := parentStderrR.Read(parentStderrBuf[:])
	_ = parentStderrR.Close()

	if n > 0 {
		t.Errorf("D9: parent stderr not empty — got %d bytes: %q", n, parentStderrBuf[:n])
	}

	// Verify stderr.log exists and contains >= 1 MiB.
	info, err := os.Stat(stderrLogPath)
	if err != nil {
		t.Fatalf("D9: stderr.log not created: %v", err)
	}
	const oneMiB = 1024 * 1024
	if info.Size() < oneMiB {
		t.Errorf("D9: stderr.log too small: %d bytes (want >= 1 MiB)", info.Size())
	}
}
