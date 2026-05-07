//go:build tui

package views

import "os"

// userHomeDir returns the user's home directory, delegating to os.UserHomeDir.
// Extracted here so other view files can call it without re-importing os.
func userHomeDir() (string, error) {
	return os.UserHomeDir()
}
