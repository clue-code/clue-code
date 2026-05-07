//go:build tui

package views

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clue-code/clue-code/internal/state"
	"github.com/fsnotify/fsnotify"
)

var (
	sessionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#efefef"}).
				Background(lipgloss.AdaptiveColor{Light: "#d0d0d0", Dark: "#333333"}).
				Padding(0, 1)

	sessionActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#007700", Dark: "#00cc44"})

	sessionStaleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#bb5500", Dark: "#ffaa33"})

	sessionEndedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

	sessionHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})
)

type sessionsReloadMsg struct {
	sessions []state.SessionDescriptor
	err      error
}

type sessionsFsnotifyMsg struct{}

// SessionsModel displays all active sessions with fsnotify live-reload.
type SessionsModel struct {
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

// NewSessionsModel returns an initialized SessionsModel.
func NewSessionsModel() SessionsModel {
	return SessionsModel{
		keyQ: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		keyR: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		keyJ: key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "scroll down")),
		keyK: key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "scroll up")),
	}
}

func (m SessionsModel) Init() tea.Cmd {
	return tea.Batch(loadSessionsCmd(), watchSessionsCmd())
}

func loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := state.ListActive()
		return sessionsReloadMsg{sessions: sessions, err: err}
	}
}

func watchSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			// No home dir — fall back to polling interval.
			time.Sleep(2 * time.Second)
			return sessionsFsnotifyMsg{}
		}
		sessDir := filepath.Join(home, ".clue-code", "sessions")
		w, err := fsnotify.NewWatcher()
		if err != nil {
			time.Sleep(2 * time.Second)
			return sessionsFsnotifyMsg{}
		}
		defer func() { _ = w.Close() }()
		// Best-effort: if dir doesn't exist yet, just wait.
		_ = w.Add(sessDir)
		select {
		case <-w.Events:
		case <-w.Errors:
		case <-time.After(5 * time.Second):
		}
		return sessionsFsnotifyMsg{}
	}
}

func (m SessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case sessionsReloadMsg:
		m.sessions = msg.sessions
		m.err = msg.err
		m.offset = 0
		return m, watchSessionsCmd()

	case sessionsFsnotifyMsg:
		return m, tea.Batch(loadSessionsCmd(), watchSessionsCmd())

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keyQ):
			return m, tea.Quit
		case key.Matches(msg, m.keyR):
			return m, loadSessionsCmd()
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

func (m SessionsModel) visibleRows() int {
	rows := m.height - 3
	if rows < 0 {
		return 0
	}
	return rows
}

func (m SessionsModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	title := sessionHeaderStyle.Width(m.width).Render("clue-code — sessions")
	b.WriteString(title)
	b.WriteByte('\n')

	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n", m.err))
		b.WriteString(sessionHelpStyle.Render("r reload  q quit"))
		return b.String()
	}

	if len(m.sessions) == 0 {
		b.WriteString("no active sessions\n")
		b.WriteString(sessionHelpStyle.Render("r reload  q quit"))
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

	b.WriteString(sessionHelpStyle.Render("r reload  j/k scroll  q quit"))
	return b.String()
}

func (m SessionsModel) renderSession(s state.SessionDescriptor) string {
	status, err := state.GetStatus(s.ID)
	stateStr := "unknown"
	if err == nil {
		stateStr = status.State
	}

	var stateRendered string
	switch stateStr {
	case "active":
		stateRendered = sessionActiveStyle.Render("active")
	case "stale":
		stateRendered = sessionStaleStyle.Render("stale")
	default:
		stateRendered = sessionEndedStyle.Render(stateStr)
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

	line := fmt.Sprintf("%-36s  %-8s  %-20s  %s",
		s.ID, stateRendered, skill, project)
	// Clamp to 79 visible columns (ANSI-stripped width via lipgloss.Width).
	for lipgloss.Width(line) > 79 {
		runes := []rune(line)
		line = string(runes[:len(runes)-1])
	}
	return line
}
