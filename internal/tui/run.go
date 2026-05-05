//go:build tui

// Package tui provides the Bubble Tea terminal UI for clue-code.
package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/clue-code/clue-code/internal/tui/views"
	"golang.org/x/term"
)

// RunReadOnlyState starts the read-only state inspector TUI.
// Returns exit code: 0 on clean quit, 2 on non-TTY or startup error.
func RunReadOnlyState(args []string) int {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, "clue-code tui requires a TTY (try running this in a terminal, not a pipe)")
		return 2
	}

	m := views.NewStateModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui: error:", err)
		return 1
	}
	return 0
}
