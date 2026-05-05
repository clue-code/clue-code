package state

import (
	"fmt"
	"os"
	"path/filepath"
)

const dirName = ".clue-code"

// ResolvePath returns the filesystem path for the given scope and key.
// For ScopeSession, sessionID must be non-empty; it is ignored otherwise.
func ResolvePath(scope Scope, key, sessionID string) (string, error) {
	switch scope {
	case ScopeGlobal:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("state: resolve global path: %w", err)
		}
		return filepath.Join(home, dirName, "state", key), nil

	case ScopeProject:
		root, err := findProjectRoot()
		if err != nil {
			return "", fmt.Errorf("state: resolve project path: %w", err)
		}
		return filepath.Join(root, dirName, "state", key), nil

	case ScopeSession:
		if sessionID == "" {
			return "", fmt.Errorf("state: session scope requires a non-empty sessionID")
		}
		root, err := findProjectRoot()
		if err != nil {
			return "", fmt.Errorf("state: resolve session path: %w", err)
		}
		return filepath.Join(root, dirName, "sessions", sessionID, key), nil

	default:
		return "", fmt.Errorf("state: unknown scope %d", scope)
	}
}

// findProjectRoot walks up from cwd looking for a directory that contains
// a .clue-code/ entry. Falls back to cwd if none is found.
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, dirName)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding a marker — use cwd.
			return cwd, nil
		}
		dir = parent
	}
}
