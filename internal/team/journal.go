package team

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// jsonUnmarshal is an alias so journal.go doesn't need to import encoding/json
// at the call sites — keeps the call sites readable.
var jsonUnmarshal = json.Unmarshal

// Journal is an append-only, crash-safe log of Envelopes stored as NDJSON.
// It is safe for concurrent use within a single process (sync.Mutex) and
// protected against concurrent processes via an exclusive flock.
type Journal struct {
	path    string
	f       *os.File
	mu      sync.Mutex
	release func() error // flock release function
}

// journalDir returns the directory for a team journal given a project root
// and team ID.
func journalDir(projectRoot, teamID string) string {
	return filepath.Join(projectRoot, ".clue-code", "teams", teamID)
}

// journalPath returns the path to the journal file.
func journalPath(projectRoot, teamID string) string {
	return filepath.Join(journalDir(projectRoot, teamID), "journal.ndjson")
}

// OpenJournal opens (or creates) the append-only journal for teamID under
// projectRoot. It acquires an exclusive flock on a sidecar lock file and
// performs torn-tail recovery before returning.
func OpenJournal(teamID, projectRoot string) (*Journal, error) {
	dir := journalDir(projectRoot, teamID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("team: journal mkdir %q: %w", dir, err)
	}

	lockPath := filepath.Join(dir, "journal.lock")
	release, err := acquireJournalFlock(lockPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("team: journal flock %q: %w", lockPath, err)
	}

	jPath := journalPath(projectRoot, teamID)
	f, err := os.OpenFile(jPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		_ = release()
		return nil, fmt.Errorf("team: journal open %q: %w", jPath, err)
	}

	j := &Journal{
		path:    jPath,
		f:       f,
		release: release,
	}

	if err := j.RecoverTornTail(); err != nil {
		_ = f.Close()
		_ = release()
		return nil, fmt.Errorf("team: journal torn-tail recovery: %w", err)
	}

	return j, nil
}

// Append serialises env as a single NDJSON line and fsync-writes it to the
// journal. The mutex ensures sequential writes from multiple goroutines.
func (j *Journal) Append(env Envelope) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := EncodeEnvelope(j.f, env); err != nil {
		return fmt.Errorf("team: journal append: %w", err)
	}
	if err := j.f.Sync(); err != nil {
		return fmt.Errorf("team: journal sync: %w", err)
	}
	return nil
}

// Read reads all envelopes from the journal file from the beginning.
// Empty lines are ignored. Lines with an unsupported envelope version cause
// ErrUnsupportedEnvelopeVersion to be returned immediately.
func (j *Journal) Read() ([]Envelope, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Seek to start so we always read the full journal.
	if _, err := j.f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("team: journal seek: %w", err)
	}

	var envs []Envelope
	scanner := newScanner(j.f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Peek at version without full unmarshal first — just unmarshal all.
		env, err := decodeLine(line)
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("team: journal scan: %w", err)
	}
	return envs, nil
}

// decodeLine unmarshals a single NDJSON line and validates the version.
func decodeLine(line []byte) (Envelope, error) {
	// We need to peek version before full validation — unmarshal into a
	// version-only struct first to give a cleaner error.
	var ver struct {
		V uint8 `json:"v"`
	}
	if err := jsonUnmarshal(line, &ver); err != nil {
		return Envelope{}, fmt.Errorf("team: journal decode version: %w", err)
	}
	if ver.V != EnvelopeVersion {
		return Envelope{}, fmt.Errorf("%w: got %d", ErrUnsupportedEnvelopeVersion, ver.V)
	}
	var env Envelope
	if err := jsonUnmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("team: journal decode envelope: %w", err)
	}
	return env, nil
}

// RecoverTornTail scans the file for the last complete line (terminated by
// '\n') and truncates anything after it. This discards partially-written
// records that result from process crashes mid-write.
func (j *Journal) RecoverTornTail() error {
	info, err := j.f.Stat()
	if err != nil {
		return fmt.Errorf("team: journal stat: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}

	// Seek to beginning and scan for the position of the last '\n'.
	if _, err := j.f.Seek(0, 0); err != nil {
		return fmt.Errorf("team: journal seek for recovery: %w", err)
	}

	reader := bufio.NewReader(j.f)
	var lastNewline int64
	var pos int64
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break // io.EOF or other error — stop scanning
		}
		pos++
		if b == '\n' {
			lastNewline = pos
		}
	}

	if lastNewline < info.Size() {
		// There is a partial line at the end — truncate it.
		if err := j.f.Truncate(lastNewline); err != nil {
			return fmt.Errorf("team: journal truncate torn tail: %w", err)
		}
		// Seek to the new end for subsequent appends.
		if _, err := j.f.Seek(lastNewline, 0); err != nil {
			return fmt.Errorf("team: journal seek after truncate: %w", err)
		}
	}
	return nil
}

// Close releases the flock and closes the underlying file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var errs []error
	if err := j.release(); err != nil {
		errs = append(errs, err)
	}
	if err := j.f.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("team: journal close: %v", errs)
	}
	return nil
}

// acquireJournalFlock opens (or creates) the lock file at path and acquires
// an exclusive POSIX flock on it, retrying every 50 ms until timeout.
func acquireJournalFlock(path string, timeout time.Duration) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("team: open lock file %q: %w", path, err)
	}

	deadline := time.Now().Add(timeout)
	const retryInterval = 50 * time.Millisecond
	for {
		flockErr := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if flockErr == nil {
			release := func() error {
				if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
					_ = f.Close()
					return fmt.Errorf("team: unlock %q: %w", path, err)
				}
				return f.Close()
			}
			return release, nil
		}
		if flockErr != unix.EWOULDBLOCK {
			_ = f.Close()
			return nil, fmt.Errorf("team: flock %q: %w", path, flockErr)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("team: flock timeout %q", path)
		}
		time.Sleep(retryInterval)
	}
}
