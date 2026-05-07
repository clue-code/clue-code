package main

import (
	"os"
	"testing"
)

// TestCmd_TUI_NotTTY verifies C3: when stdout is not a TTY, runTUI returns
// exit code 2.
//
// We use os.Pipe() to create a non-TTY file descriptor and redirect stdout.
func TestCmd_TUI_NotTTY(t *testing.T) {
	t.Parallel()

	// Create a pipe — neither end is a TTY.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = r.Close()
		_ = w.Close()
	}()

	// Swap os.Stdout so that runTUI sees a non-TTY fd.
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	code := runTUI(nil)
	if code != 2 {
		t.Errorf("runTUI with non-TTY stdout: got exit code %d, want 2", code)
	}
}
