package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// ErrVersionMismatch is returned by WriteIfVersion when the stored version
// does not match the expected value.
var ErrVersionMismatch = errors.New("state: version mismatch")

// Store is the interface for reading and writing persistent agent state.
type Store interface {
	// Read returns the current value and version for key. exists is false
	// when the key has never been written.
	Read(ctx context.Context, key string, scope Scope) (value []byte, version uint64, exists bool, err error)

	// Write stores value under key and returns the new version.
	Write(ctx context.Context, key string, value []byte, scope Scope) (version uint64, err error)

	// WriteIfVersion stores value only when the current version equals expected.
	// Returns ErrVersionMismatch if versions do not match.
	WriteIfVersion(ctx context.Context, key string, value []byte, expected uint64, scope Scope) (version uint64, err error)

	// WriteWithRetry retries Write on ErrStateBusy with exponential backoff.
	// Backoff schedule: 10ms, 30ms, 90ms, 270ms, 500ms (capped), then 500ms
	// repeating until ctx is done. On each retry IncWriteContention is called.
	WriteWithRetry(ctx context.Context, key string, value []byte, scope Scope) (version uint64, err error)

	// Append opens key with O_APPEND|O_CREATE|O_WRONLY under LOCK_EX and
	// writes value. Used for notepad and event journals.
	Append(ctx context.Context, key string, value []byte, scope Scope) error

	// Clear removes all keys with the given prefix within scope.
	// Returns the number of keys removed.
	Clear(ctx context.Context, scope Scope, prefix string) (removed int, err error)
}

// Open returns a Store bound to the given session ID. For scopes other than
// ScopeSession the sessionID is ignored.
func Open(sessionID string) (Store, error) {
	return &jsonStore{sessionID: sessionID}, nil
}

// jsonStore is the JSON-on-disk implementation of Store.
type jsonStore struct {
	sessionID string
}

// --- JSON file format ---

type storeFile struct {
	Version int                    `json:"version"`
	Entries map[string]storeEntry  `json:"entries"`
}

type storeEntry struct {
	V     uint64 `json:"v"`
	Value string `json:"value"`
}

// --- internal helpers ---

const lockTimeout = 2 * time.Second

// jsonFilePath returns the path to the single JSON file for the given scope.
func (s *jsonStore) jsonFilePath(scope Scope) (string, error) {
	switch scope {
	case ScopeGlobal:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("state: home dir: %w", err)
		}
		return filepath.Join(home, ".clue-code", "global.json"), nil
	case ScopeProject:
		root, err := findProjectRoot()
		if err != nil {
			return "", fmt.Errorf("state: project root: %w", err)
		}
		return filepath.Join(root, ".clue-code", "state", "project.json"), nil
	case ScopeSession:
		if s.sessionID == "" {
			return "", fmt.Errorf("state: session scope requires non-empty sessionID")
		}
		root, err := findProjectRoot()
		if err != nil {
			return "", fmt.Errorf("state: project root: %w", err)
		}
		return filepath.Join(root, ".clue-code", "state", "sessions", s.sessionID, "kv.json"), nil
	default:
		return "", fmt.Errorf("state: unknown scope %d", scope)
	}
}

// lockFilePath returns the companion .lock file path for the given JSON file.
func lockFilePath(jsonPath string) string {
	return jsonPath + ".lock"
}

// readFile reads and parses the JSON file. Returns an empty storeFile if the
// file does not exist.
func readFile(path string) (storeFile, error) {
	sf := storeFile{Version: 1, Entries: make(map[string]storeEntry)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sf, nil
		}
		return sf, fmt.Errorf("state: read %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return sf, fmt.Errorf("state: parse %q: %w", path, err)
	}
	if sf.Entries == nil {
		sf.Entries = make(map[string]storeEntry)
	}
	return sf, nil
}

// writeFile atomically writes sf to path using a tmp+rename pattern.
func writeFile(path string, sf storeFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("state: mkdir %q: %w", filepath.Dir(path), err)
	}
	data, err := json.Marshal(sf)
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("state: write tmp %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state: rename %q→%q: %w", tmp, path, err)
	}
	return nil
}

// --- Store methods ---

func (s *jsonStore) Read(_ context.Context, key string, scope Scope) ([]byte, uint64, bool, error) {
	jsonPath, err := s.jsonFilePath(scope)
	if err != nil {
		return nil, 0, false, err
	}
	sf, err := readFile(jsonPath)
	if err != nil {
		return nil, 0, false, err
	}
	entry, ok := sf.Entries[key]
	if !ok {
		return nil, 0, false, nil
	}
	return []byte(entry.Value), entry.V, true, nil
}

func (s *jsonStore) Write(_ context.Context, key string, value []byte, scope Scope) (uint64, error) {
	jsonPath, err := s.jsonFilePath(scope)
	if err != nil {
		return 0, err
	}
	release, err := acquireFlock(lockFilePath(jsonPath), lockTimeout)
	if err != nil {
		return 0, err
	}
	defer release() //nolint:errcheck

	sf, err := readFile(jsonPath)
	if err != nil {
		return 0, err
	}
	newV := sf.Entries[key].V + 1
	sf.Entries[key] = storeEntry{V: newV, Value: string(value)}
	if err := writeFile(jsonPath, sf); err != nil {
		return 0, err
	}
	return newV, nil
}

func (s *jsonStore) WriteIfVersion(_ context.Context, key string, value []byte, expected uint64, scope Scope) (uint64, error) {
	jsonPath, err := s.jsonFilePath(scope)
	if err != nil {
		return 0, err
	}
	release, err := acquireFlock(lockFilePath(jsonPath), lockTimeout)
	if err != nil {
		return 0, err
	}
	defer release() //nolint:errcheck

	sf, err := readFile(jsonPath)
	if err != nil {
		return 0, err
	}
	current := sf.Entries[key].V
	if current != expected {
		return current, ErrVersionMismatch
	}
	newV := current + 1
	sf.Entries[key] = storeEntry{V: newV, Value: string(value)}
	if err := writeFile(jsonPath, sf); err != nil {
		return 0, err
	}
	return newV, nil
}

// backoffSchedule is the WriteWithRetry delay sequence (ms). After the last
// entry the final value repeats until ctx is done.
var backoffSchedule = []time.Duration{
	10 * time.Millisecond,
	30 * time.Millisecond,
	90 * time.Millisecond,
	270 * time.Millisecond,
	500 * time.Millisecond,
}

func (s *jsonStore) WriteWithRetry(ctx context.Context, key string, value []byte, scope Scope) (uint64, error) {
	scopeStr := scope.String()
	for i := 0; ; i++ {
		v, err := s.Write(ctx, key, value, scope)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, ErrStateBusy) {
			return 0, err
		}
		// Contention — pick next backoff delay.
		delay := backoffSchedule[len(backoffSchedule)-1]
		if i < len(backoffSchedule) {
			delay = backoffSchedule[i]
		}
		IncWriteContention(key, scopeStr)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (s *jsonStore) Append(_ context.Context, key string, value []byte, scope Scope) error {
	jsonPath, err := s.jsonFilePath(scope)
	if err != nil {
		return err
	}
	// Append uses the raw file path derived from key, not the JSON wrapper.
	appendPath := filepath.Join(filepath.Dir(jsonPath), key)
	if err := os.MkdirAll(filepath.Dir(appendPath), 0o700); err != nil {
		return fmt.Errorf("state: mkdir for append %q: %w", appendPath, err)
	}
	f, err := os.OpenFile(appendPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("state: open append file %q: %w", appendPath, err)
	}
	defer f.Close() //nolint:errcheck

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("state: flock append %q: %w", appendPath, err)
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:errcheck

	if _, err := f.Write(value); err != nil {
		return fmt.Errorf("state: write append %q: %w", appendPath, err)
	}
	return nil
}

func (s *jsonStore) Clear(_ context.Context, scope Scope, prefix string) (int, error) {
	jsonPath, err := s.jsonFilePath(scope)
	if err != nil {
		return 0, err
	}
	release, err := acquireFlock(lockFilePath(jsonPath), lockTimeout)
	if err != nil {
		return 0, err
	}
	defer release() //nolint:errcheck

	sf, err := readFile(jsonPath)
	if err != nil {
		return 0, err
	}
	removed := 0
	for k := range sf.Entries {
		if strings.HasPrefix(k, prefix) {
			delete(sf.Entries, k)
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	if err := writeFile(jsonPath, sf); err != nil {
		return 0, err
	}
	return removed, nil
}
