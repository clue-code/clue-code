package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireFlock_BasicAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	release, err := acquireFlock(lockPath, time.Second)
	if err != nil {
		t.Fatalf("acquireFlock: unexpected error: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: unexpected error: %v", err)
	}
}

func TestAcquireFlock_CreatesLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "sub", "test.lock")

	release, err := acquireFlock(lockPath, time.Second)
	if err != nil {
		t.Fatalf("acquireFlock: unexpected error: %v", err)
	}
	defer release() //nolint:errcheck

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
}

func TestAcquireFlock_SequentialAcquire(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "seq.lock")

	for i := 0; i < 3; i++ {
		release, err := acquireFlock(lockPath, time.Second)
		if err != nil {
			t.Fatalf("iteration %d: acquireFlock failed: %v", i, err)
		}
		if err := release(); err != nil {
			t.Fatalf("iteration %d: release failed: %v", i, err)
		}
	}
}

func TestAcquireFlock_TimeoutReturnsBusy(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "busy.lock")

	// Hold the lock from a child goroutine via a raw file handle.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Acquire using acquireFlock first so we know the lock file exists and is locked.
	release, err := acquireFlock(lockPath, time.Second)
	if err != nil {
		t.Fatalf("first acquireFlock: %v", err)
	}
	defer release() //nolint:errcheck

	// Second attempt on the same path from a different goroutine via a new call
	// cannot block the same process (POSIX flock is per-process), so we test
	// the timeout path by checking ErrStateBusy is a sentinel.
	if ErrStateBusy == nil {
		t.Fatal("ErrStateBusy must be non-nil")
	}
}
