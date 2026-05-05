package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// ErrStateBusy is returned when acquireFlock cannot obtain the lock within
// the requested timeout.
var ErrStateBusy = errors.New("state: lock busy")

const retryInterval = 50 * time.Millisecond

// acquireFlock opens (or creates) the lock file at path and acquires an
// exclusive POSIX flock on it. It retries every 50 ms until timeout elapses.
// On success it returns a release function that unlocks and closes the file.
// On timeout it returns ErrStateBusy.
func acquireFlock(path string, timeout time.Duration) (release func() error, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("state: mkdir for lock %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("state: open lock file %q: %w", path, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		flockErr := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if flockErr == nil {
			release := func() error {
				if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
					_ = f.Close()
					return fmt.Errorf("state: unlock %q: %w", path, err)
				}
				return f.Close()
			}
			return release, nil
		}
		if !errors.Is(flockErr, unix.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("state: flock %q: %w", path, flockErr)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, ErrStateBusy
		}
		time.Sleep(retryInterval)
	}
}
