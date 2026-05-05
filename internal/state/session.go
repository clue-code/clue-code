package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
)

// ErrSessionNotFound is returned by GetStatus when no session descriptor exists.
var ErrSessionNotFound = errors.New("state: session not found")

// SessionDescriptor describes a session visible across all projects.
type SessionDescriptor struct {
	ID           string    `json:"id"`
	ProjectPath  string    `json:"project_path"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
	PID          int       `json:"pid"`
	Skill        string    `json:"current_skill,omitempty"`
}

// SessionStatus is the live view of one session.
type SessionStatus struct {
	Descriptor   SessionDescriptor `json:"descriptor"`
	State        string            `json:"state"`         // "active" | "stale" | "ended"
	PendingTasks int               `json:"pending_tasks"` // from teams journal
}

// TeamTaskCounter is a function variable that returns the number of pending
// tasks for a given session. Wired by the team package via init().
var TeamTaskCounter func(sessionID string) int = func(_ string) int { return 0 }

const (
	staleThreshold    = 30 * time.Second
	heartbeatInterval = 5 * time.Second
	heartbeatFile     = "heartbeat"
	indexFile         = "index.json"
)

// globalSessionsDir returns ~/.clue-code/sessions/.
func globalSessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("state: home dir: %w", err)
	}
	return filepath.Join(home, ".clue-code", "sessions"), nil
}

// heartbeatPath returns the path to the heartbeat file for a session.
// Sessions live at <project>/.clue-code/state/sessions/<sid>/heartbeat.
func heartbeatPath(projectPath, sessionID string) string {
	return filepath.Join(projectPath, ".clue-code", "state", "sessions", sessionID, heartbeatFile)
}

// writeHeartbeat touches the heartbeat file, updating its mtime.
func writeHeartbeat(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("state: mkdir heartbeat dir: %w", err)
	}
	now := time.Now()
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		return fmt.Errorf("state: write heartbeat %q: %w", path, err)
	}
	return os.Chtimes(path, now, now)
}

// readHeartbeatTime returns the mtime of the heartbeat file, or zero if absent.
func readHeartbeatTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// sessionState computes "active", "stale", or "ended" from heartbeat mtime.
func sessionState(hbTime time.Time, clk clock.Clock) string {
	if hbTime.IsZero() {
		return "ended"
	}
	if clk.Since(hbTime) > staleThreshold {
		return "stale"
	}
	return "active"
}

// StartSession registers the session in the global index and begins writing
// heartbeats every 5 s. The goroutine exits when done is closed.
// clk is injectable for tests.
func StartSession(desc SessionDescriptor, clk clock.Clock, done <-chan struct{}) error {
	if err := upsertIndex(desc); err != nil {
		return err
	}
	hbPath := heartbeatPath(desc.ProjectPath, desc.ID)
	if err := writeHeartbeat(hbPath); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_ = writeHeartbeat(hbPath)
			}
		}
	}()
	return nil
}

// upsertIndex adds or updates desc in the global sessions index.
func upsertIndex(desc SessionDescriptor) error {
	dir, err := globalSessionsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("state: mkdir sessions dir: %w", err)
	}
	indexPath := filepath.Join(dir, indexFile)
	lockPath := indexPath + ".lock"

	release, err := acquireFlock(lockPath, lockTimeout)
	if err != nil {
		return fmt.Errorf("state: upsert index lock: %w", err)
	}
	defer release() //nolint:errcheck

	sessions, err := readIndex(indexPath)
	if err != nil {
		return err
	}
	updated := false
	for i, s := range sessions {
		if s.ID == desc.ID {
			sessions[i] = desc
			updated = true
			break
		}
	}
	if !updated {
		sessions = append(sessions, desc)
	}
	return writeIndex(indexPath, sessions)
}

// removeFromIndex removes a session from the global index.
func removeFromIndex(sessionID string) error {
	dir, err := globalSessionsDir()
	if err != nil {
		return err
	}
	indexPath := filepath.Join(dir, indexFile)
	lockPath := indexPath + ".lock"

	release, err := acquireFlock(lockPath, lockTimeout)
	if err != nil {
		return fmt.Errorf("state: remove index lock: %w", err)
	}
	defer release() //nolint:errcheck

	sessions, err := readIndex(indexPath)
	if err != nil {
		return err
	}
	filtered := sessions[:0]
	for _, s := range sessions {
		if s.ID != sessionID {
			filtered = append(filtered, s)
		}
	}
	return writeIndex(indexPath, filtered)
}

func readIndex(path string) ([]SessionDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("state: read index %q: %w", path, err)
	}
	var sessions []SessionDescriptor
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("state: parse index %q: %w", path, err)
	}
	return sessions, nil
}

func writeIndex(path string, sessions []SessionDescriptor) error {
	data, err := json.Marshal(sessions)
	if err != nil {
		return fmt.Errorf("state: marshal index: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("state: write index tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state: rename index: %w", err)
	}
	return nil
}

// ListActive returns all sessions across all projects on this host.
// Sessions with heartbeat older than 30s are reported as state=stale.
func ListActive() ([]SessionDescriptor, error) {
	dir, err := globalSessionsDir()
	if err != nil {
		return nil, err
	}
	indexPath := filepath.Join(dir, indexFile)
	return readIndex(indexPath)
}

// GetStatus returns the live status for one session.
// Returns ErrSessionNotFound if no descriptor exists.
func GetStatus(sessionID string) (SessionStatus, error) {
	sessions, err := ListActive()
	if err != nil {
		return SessionStatus{}, err
	}
	clk := clock.Real()
	for _, desc := range sessions {
		if desc.ID == sessionID {
			hbPath := heartbeatPath(desc.ProjectPath, desc.ID)
			hbTime := readHeartbeatTime(hbPath)
			state := sessionState(hbTime, clk)
			lag := clk.Since(hbTime)
			if lag > staleThreshold {
				SetSessionStaleLag(lag.Seconds())
			}
			return SessionStatus{
				Descriptor:   desc,
				State:        state,
				PendingTasks: TeamTaskCounter(sessionID),
			}, nil
		}
	}
	return SessionStatus{}, ErrSessionNotFound
}
