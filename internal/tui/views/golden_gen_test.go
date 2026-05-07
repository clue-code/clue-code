//go:build tui

package views

import (
	"flag"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var updateGolden = flag.Bool("update", false, "regenerate golden snapshot files")

// TestGenerateGolden generates the sessions-24x80.golden.ans snapshot when
// run with -update. It is a no-op otherwise.
func TestGenerateGolden(t *testing.T) {
	if !*updateGolden {
		t.Skip("pass -update to regenerate golden snapshots")
	}

	m := NewSessionsModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := model.View()

	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/sessions-24x80.golden.ans", []byte(view), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("written testdata/sessions-24x80.golden.ans (%d bytes)", len(view))
}
