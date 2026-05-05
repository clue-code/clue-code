//go:build tui

// Package views contains Bubble Tea model implementations for clue-code TUI views.
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clue-code/clue-code/internal/state"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#efefef"}).
			Background(lipgloss.AdaptiveColor{Light: "#d0d0d0", Dark: "#333333"}).
			Padding(0, 1)

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#007700", Dark: "#00cc44"})

	staleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#bb5500", Dark: "#ffaa33"})

	endedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})
)

type reloadMsg struct{}

// StateModel is the read-only state inspector model.
type StateModel struct {
	sessions []state.SessionDescriptor
	err      error
	width    int
	height   int
	offset   int

	keyQ key.Binding
	keyR key.Binding
	keyJ key.Binding
	keyK key.Binding
}

// NewStateModel returns an initialized StateModel.
func NewStateModel() StateModel {
	return StateModel{
		keyQ: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		keyR: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		keyJ: key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "scroll down")),
		keyK: key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "scroll up")),
	}
}

func (m StateModel) Init() tea.Cmd {
	return func() tea.Msg { return reloadMsg{} }
}

func (m StateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case reloadMsg:
		sessions, err := state.ListActive()
		m.sessions = sessions
		m.err = err
		m.offset = 0

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keyQ):
			return m, tea.Quit
		case key.Matches(msg, m.keyR):
			return m, func() tea.Msg { return reloadMsg{} }
		case key.Matches(msg, m.keyJ):
			maxOffset := len(m.sessions) - m.visibleRows()
			if m.offset < maxOffset {
				m.offset++
			}
		case key.Matches(msg, m.keyK):
			if m.offset > 0 {
				m.offset--
			}
		}
	}
	return m, nil
}

func (m StateModel) visibleRows() int {
	// header (1) + blank (1) + help (1) = 3 reserved lines
	rows := m.height - 3
	if rows < 0 {
		return 0
	}
	return rows
}

func (m StateModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	title := headerStyle.Width(m.width).Render("clue-code — state inspector")
	b.WriteString(title)
	b.WriteByte('\n')

	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n", m.err))
		b.WriteString(helpStyle.Render("r reload  q quit"))
		return b.String()
	}

	if len(m.sessions) == 0 {
		b.WriteString("no active sessions\n")
		b.WriteString(helpStyle.Render("r reload  q quit"))
		return b.String()
	}

	visible := m.visibleRows()
	end := m.offset + visible
	if end > len(m.sessions) {
		end = len(m.sessions)
	}

	for _, s := range m.sessions[m.offset:end] {
		b.WriteString(m.renderSession(s))
		b.WriteByte('\n')
	}

	b.WriteString(helpStyle.Render("r reload  j/k scroll  q quit"))
	return b.String()
}

func (m StateModel) renderSession(s state.SessionDescriptor) string {
	status, err := state.GetStatus(s.ID)
	stateStr := "unknown"
	if err == nil {
		stateStr = status.State
	}

	var stateRendered string
	switch stateStr {
	case "active":
		stateRendered = activeStyle.Render("active")
	case "stale":
		stateRendered = staleStyle.Render("stale")
	default:
		stateRendered = endedStyle.Render(stateStr)
	}

	skill := s.Skill
	if skill == "" {
		skill = "-"
	}
	project := s.ProjectPath
	maxProjLen := m.width - 60
	if maxProjLen < 8 {
		maxProjLen = 8
	}
	if len(project) > maxProjLen {
		project = "..." + project[len(project)-maxProjLen+3:]
	}

	return fmt.Sprintf("%-36s  %-8s  %-20s  %s",
		s.ID, stateRendered, skill, project)
}
