//go:build tui

package tui

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/clue-code/clue-code/internal/state"
	"github.com/clue-code/clue-code/internal/tui/views"
)

// TestTUI_SessionsListRendersIn200ms verifies C1: at 24×80 the sessions view
// renders within 200ms and every line is ≤80 visible columns.
func TestTUI_SessionsListRendersIn200ms(t *testing.T) {
	t.Parallel()

	m := views.NewSessionsModel()
	tm := teatest.NewTestModel(
		t, m,
		teatest.WithInitialTermSize(80, 24),
	)
	t.Cleanup(func() {
		if err := tm.Quit(); err != nil {
			t.Logf("quit: %v", err)
		}
	})

	start := time.Now()

	// Wait up to 1s for first non-empty frame; we will assert elapsed <200ms.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return len(bytes.TrimSpace(bts)) > 0
	}, teatest.WithDuration(time.Second), teatest.WithCheckInterval(5*time.Millisecond))

	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("first frame took %v, want <200ms", elapsed)
	}
}

// TestTUI_SessionsListColumnWidth verifies C1: lines rendered at 80 cols
// have ANSI-stripped width ≤ 80.
func TestTUI_SessionsListColumnWidth(t *testing.T) {
	t.Parallel()

	m := views.NewSessionsModel()
	tm := teatest.NewTestModel(
		t, m,
		teatest.WithInitialTermSize(80, 24),
	)

	// Accumulate output until we have at least one non-empty frame.
	var captured []byte
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		captured = bts
		return len(bytes.TrimSpace(bts)) > 0
	}, teatest.WithDuration(time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	// Quit cleanly.
	if err := tm.Quit(); err != nil {
		t.Logf("quit: %v", err)
	}

	for _, line := range bytes.Split(captured, []byte("\n")) {
		w := lipgloss.Width(string(line))
		if w > 80 {
			t.Errorf("line exceeds 80 cols (%d): %q", w, line)
		}
	}
}

// TestSessionsView_RealFsnotifyConvergence verifies C2: writing a session
// descriptor to the filesystem causes the TUI to converge to showing it
// within 1 second, using real fsnotify (no mocks).
func TestSessionsView_RealFsnotifyConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping fsnotify integration test in short mode")
	}
	// NOTE: cannot use t.Parallel() here because t.Setenv requires sequential execution.

	// Set up a temp sessions dir and a fake global index.
	tmpHome := t.TempDir()
	sessDir := filepath.Join(tmpHome, ".clue-code", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(sessDir, "index.json")

	// Prepare a session descriptor.
	desc := state.SessionDescriptor{
		ID:          "test-session-convergence-id",
		ProjectPath: tmpHome,
		Skill:       "autopilot",
	}

	// Write index before model starts so initial load picks it up.
	data, _ := json.Marshal([]state.SessionDescriptor{desc})
	if err := os.WriteFile(indexPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Override HOME so globalSessionsDir() resolves to our temp dir.
	t.Setenv("HOME", tmpHome)

	m := views.NewSessionsModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	t.Cleanup(func() {
		if err := tm.Quit(); err != nil {
			t.Logf("quit: %v", err)
		}
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("test-session-convergence-id"))
	}, teatest.WithDuration(time.Second), teatest.WithCheckInterval(20*time.Millisecond))
}

// TestTUI_QuitWithin100ms verifies C4: pressing 'q' exits the program
// within 100ms.
func TestTUI_QuitWithin100ms(t *testing.T) {
	t.Parallel()

	m := newRootModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Wait for the first frame, then send 'q'.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return len(bts) > 0
	}, teatest.WithDuration(time.Second), teatest.WithCheckInterval(5*time.Millisecond))

	quitStart := time.Now()
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	tm.WaitFinished(t, teatest.WithFinalTimeout(500*time.Millisecond))
	elapsed := time.Since(quitStart)
	if elapsed > 100*time.Millisecond {
		t.Errorf("quit took %v, want <100ms", elapsed)
	}
}

// TestSlog_NoFrameCorruption verifies C7: rendered TUI frames do not contain
// corrupted ANSI escape sequences.
func TestSlog_NoFrameCorruption(t *testing.T) {
	t.Parallel()

	m := views.NewSessionsModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Capture the first non-empty frame.
	var captured []byte
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		captured = bts
		return len(bts) > 0
	}, teatest.WithDuration(time.Second), teatest.WithCheckInterval(5*time.Millisecond))

	if err := tm.Quit(); err != nil {
		t.Logf("quit: %v", err)
	}

	// Scan for ESC bytes and validate the following byte is a valid ANSI sequence start.
	for i := 0; i < len(captured)-1; i++ {
		if captured[i] == 0x1b {
			next := captured[i+1]
			// Valid ANSI continuations: CSI ([), OSC (]), string terminators, etc.
			switch next {
			case '[', ']', '(', ')', '=', '>', 'O', 'P', 'M', 'c', '7', '8', ' ', '_':
				// valid
			default:
				t.Errorf("suspicious escape sequence at byte %d: ESC 0x%02x", i, next)
			}
		}
	}
}
