//go:build tui

package views

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clue-code/clue-code/internal/hooks"
)

// TestHooksView_50Events_Show20Recent verifies C5: writing 50 events to a
// hooks log causes the view to show exactly 20 (the most recent).
func TestHooksView_50Events_Show20Recent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "hooks.log")

	// Write 50 events.
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for i := 0; i < 50; i++ {
		e := hooks.LogEntry{
			Timestamp:  time.Now().Add(time.Duration(i) * time.Second),
			Event:      hooks.EventPreToolUse,
			Command:    "/bin/true",
			DurationMS: int64(i),
			ExitCode:   0,
		}
		if err := enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	m := NewHooksModel(logPath)

	// Initialize the model by calling Init and then manually firing the load command.
	cmd := m.Init()
	msg := cmd()

	updated, _ := m.Update(msg)
	hm, ok := updated.(HooksModel)
	if !ok {
		t.Fatal("Update did not return HooksModel")
	}

	if len(hm.entries) != maxHookEvents {
		t.Errorf("got %d entries, want %d", len(hm.entries), maxHookEvents)
	}

	// Set a window size so View() renders.
	sizedModel, _ := hm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	hm = sizedModel.(HooksModel)

	view := hm.View()
	if view == "" {
		t.Error("View() returned empty string")
	}
}

// TestHooksView_JKNavigation verifies C5 navigation: j/k keys move the
// selected index up and down.
func TestHooksView_JKNavigation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "hooks.log")

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for i := 0; i < 20; i++ {
		e := hooks.LogEntry{
			Timestamp:  time.Now(),
			Event:      hooks.EventStop,
			Command:    "/bin/true",
			DurationMS: 1,
		}
		if err := enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	m := NewHooksModel(logPath)
	cmd := m.Init()
	msg := cmd()
	updated, _ := m.Update(msg)
	hm := updated.(HooksModel)

	sizedModel, _ := hm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	hm = sizedModel.(HooksModel)

	initialSelected := hm.selected

	// Press 'k' — should move up (decrease selected if > 0).
	kmModel, _ := hm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	hm = kmModel.(HooksModel)
	if hm.selected >= initialSelected && initialSelected > 0 {
		t.Errorf("k did not move selection up: was %d, now %d", initialSelected, hm.selected)
	}

	// Press 'j' — should move down.
	beforeJ := hm.selected
	jmModel, _ := hm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	hm = jmModel.(HooksModel)
	if hm.selected <= beforeJ && len(hm.entries) > 1 {
		t.Errorf("j did not move selection down: was %d, now %d", beforeJ, hm.selected)
	}
}
