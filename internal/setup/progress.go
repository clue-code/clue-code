package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Progress holds the state of a partially-completed setup run.
// It is persisted to ~/.clue-code/setup-progress.json so the wizard can
// resume after a crash or Ctrl+C.
type Progress struct {
	// Stage is the last completed wizard stage (e.g. "questions", "install").
	Stage string `json:"stage"`
	// Provider is the chosen provider identifier.
	Provider string `json:"provider,omitempty"`
	// PartialAnswers holds the answers gathered so far.
	PartialAnswers Answers `json:"partial_answers,omitempty"`
	// StartedAt records when the wizard first launched.
	StartedAt time.Time `json:"started_at"`
}

// progressPath returns the path to the setup-progress.json file.
func progressPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("setup: home dir: %w", err)
	}
	return filepath.Join(home, ".clue-code", "setup-progress.json"), nil
}

// SaveProgress persists p to ~/.clue-code/setup-progress.json.
// The directory is created if it does not exist.
func SaveProgress(p Progress) error {
	path, err := progressPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("setup: create dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("setup: marshal progress: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("setup: write progress %s: %w", path, err)
	}
	return nil
}

// LoadProgress reads ~/.clue-code/setup-progress.json.
// Returns an error if the file does not exist or is malformed.
func LoadProgress() (Progress, error) {
	path, err := progressPath()
	if err != nil {
		return Progress{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Progress{}, fmt.Errorf("setup: read progress: %w", err)
	}
	var p Progress
	if err := json.Unmarshal(data, &p); err != nil {
		return Progress{}, fmt.Errorf("setup: parse progress: %w", err)
	}
	return p, nil
}

// ClearProgress deletes the setup-progress.json file.
// Returns nil if the file does not exist.
func ClearProgress() error {
	path, err := progressPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("setup: remove progress: %w", err)
	}
	return nil
}

// HasProgress reports whether a setup-progress.json file exists.
func HasProgress() bool {
	path, err := progressPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
