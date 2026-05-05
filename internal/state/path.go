package state

import (
	"fmt"
	"os"
	"path/filepath"
)

const dirName = ".clue-code"

// ResolvePath returns the filesystem path for the given scope and key.
// For ScopeSession, sessionID must be non-empty; it is ignored otherwise.
//
// Both key and sessionID are sanitized to prevent path-traversal escape
// (see sanitizeKey, sanitizeSessionID). Inputs containing "..", absolute
// paths, NUL bytes, or (for sessionID) path separators are rejected with
// ErrInvalidKey.
func ResolvePath(scope Scope, key, sessionID string) (string, error) {
	cleanKey, err := SanitizeKey(key)
	if err != nil {
		return "", err
	}
	switch scope {
	case ScopeGlobal:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("state: resolve global path: %w", err)
		}
		return filepath.Join(home, dirName, "state", cleanKey), nil

	case ScopeProject:
		root, err := findProjectRoot()
		if err != nil {
			return "", fmt.Errorf("state: resolve project path: %w", err)
		}
		return filepath.Join(root, dirName, "state", cleanKey), nil

	case ScopeSession:
		cleanSID, err := SanitizeIdentifier(sessionID)
		if err != nil {
			return "", err
		}
		root, err := findProjectRoot()
		if err != nil {
			return "", fmt.Errorf("state: resolve session path: %w", err)
		}
		return filepath.Join(root, dirName, "sessions", cleanSID, cleanKey), nil

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
