//go:build tui

// Package tui provides the Bubble Tea terminal UI for clue-code.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clue-code/clue-code/internal/tui/views"
	"golang.org/x/term"
)

// viewIndex identifies which tab is active.
type viewIndex int

const (
	viewSessions viewIndex = iota
	viewAgents
	viewSkills
	viewTokens
	viewState
	viewHooks
	numViews
)

var tabLabels = []string{
	"sessions", "agents", "skills", "tokens", "state", "hooks",
}

var (
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#ffffff"}).
			Background(lipgloss.AdaptiveColor{Light: "#0055cc", Dark: "#2266ee"}).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#888888"}).
				Padding(0, 1)
)

// rootModel is the top-level Bubble Tea model. It hosts a tab bar and
// delegates to one of six child view models.
type rootModel struct {
	active   viewIndex
	sessions tea.Model
	agents   tea.Model
	skills   tea.Model
	tokens   tea.Model
	state    tea.Model
	hooks    tea.Model
	width    int
	height   int

	keyTab      key.Binding
	keyShiftTab key.Binding
	keyQ        key.Binding
}

func newRootModel() rootModel {
	return rootModel{
		active:   viewSessions,
		sessions: views.NewSessionsModel(),
		agents:   views.NewAgentsModel(),
		skills:   views.NewSkillsModel(),
		tokens:   views.NewTokensModel(),
		state:    views.NewStateModel(),
		hooks:    views.NewHooksModel(""),

		keyTab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		keyShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev view")),
		keyQ:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (m rootModel) Init() tea.Cmd {
	return tea.Batch(
		m.sessions.(interface{ Init() tea.Cmd }).Init(),
		m.agents.(interface{ Init() tea.Cmd }).Init(),
		m.skills.(interface{ Init() tea.Cmd }).Init(),
		m.tokens.(interface{ Init() tea.Cmd }).Init(),
		m.state.(interface{ Init() tea.Cmd }).Init(),
		m.hooks.(interface{ Init() tea.Cmd }).Init(),
	)
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate a reduced size (tab bar takes 1 line) to children.
		child := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 1}
		var cmds []tea.Cmd
		m.sessions, _ = m.sessions.Update(child)
		m.agents, _ = m.agents.Update(child)
		m.skills, _ = m.skills.Update(child)
		m.tokens, _ = m.tokens.Update(child)
		m.state, _ = m.state.Update(child)
		m.hooks, _ = m.hooks.Update(child)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keyQ):
			return m, tea.Quit
		case key.Matches(msg, m.keyTab):
			m.active = (m.active + 1) % numViews
			return m, nil
		case key.Matches(msg, m.keyShiftTab):
			m.active = (m.active + numViews - 1) % numViews
			return m, nil
		}
	}

	// Delegate non-navigation messages to all children so background cmds
	// (fsnotify, reload) continue to fire regardless of active tab.
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.sessions, cmd = m.sessions.Update(msg)
	cmds = append(cmds, cmd)
	m.agents, cmd = m.agents.Update(msg)
	cmds = append(cmds, cmd)
	m.skills, cmd = m.skills.Update(msg)
	cmds = append(cmds, cmd)
	m.tokens, cmd = m.tokens.Update(msg)
	cmds = append(cmds, cmd)
	m.state, cmd = m.state.Update(msg)
	cmds = append(cmds, cmd)
	m.hooks, cmd = m.hooks.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m rootModel) View() string {
	if m.width == 0 {
		return ""
	}

	// Tab bar.
	var tabs []string
	for i, label := range tabLabels {
		if viewIndex(i) == m.active {
			tabs = append(tabs, tabActiveStyle.Render(label))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(label))
		}
	}
	tabBar := strings.Join(tabs, "")

	// Active view content.
	var content string
	switch m.active {
	case viewSessions:
		content = m.sessions.View()
	case viewAgents:
		content = m.agents.View()
	case viewSkills:
		content = m.skills.View()
	case viewTokens:
		content = m.tokens.View()
	case viewState:
		content = m.state.View()
	case viewHooks:
		content = m.hooks.View()
	}

	return tabBar + "\n" + content
}

// RunTUI starts the full six-view TUI.
// Returns exit code: 0 on clean quit, 2 on non-TTY or startup error.
func RunTUI(args []string) int {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, "clue-code tui requires a TTY (try running this in a terminal, not a pipe)")
		return 2
	}

	m := newRootModel()
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui: error:", err)
		return 1
	}
	return 0
}

// RunReadOnlyState starts the read-only state inspector TUI (legacy single-view).
// Kept for backward compatibility; RunTUI is preferred.
func RunReadOnlyState(args []string) int {
	return RunTUI(args)
}
