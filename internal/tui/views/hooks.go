//go:build tui

package views

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clue-code/clue-code/internal/hooks"
)

var (
	hooksHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#efefef"}).
				Background(lipgloss.AdaptiveColor{Light: "#ffd0d0", Dark: "#3c0000"}).
				Padding(0, 1)

	hooksBlockingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#cc0000", Dark: "#ff4444"})

	hooksInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#444444", Dark: "#aaaaaa"})

	hooksHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

	hooksSelectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#ffc0c0", Dark: "#5c1010"})
)

const maxHookEvents = 20

type hooksLoadedMsg struct {
	entries []hooks.LogEntry
	err     error
}

// HooksModel shows the 20 most recent hook log events with j/k navigation.
type HooksModel struct {
	entries  []hooks.LogEntry
	err      error
	selected int
	width    int
	height   int
	offset   int
	logPath  string

	keyQ key.Binding
	keyR key.Binding
	keyJ key.Binding
	keyK key.Binding
}

// NewHooksModel returns an initialized HooksModel using the given hooks log path.
// If logPath is empty, the default project-scoped path is used.
func NewHooksModel(logPath string) HooksModel {
	if logPath == "" {
		logPath = defaultHooksLogPath()
	}
	return HooksModel{
		logPath: logPath,
		keyQ:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		keyR:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		keyJ:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "scroll down")),
		keyK:    key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "scroll up")),
	}
}

func defaultHooksLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".clue-code/state/hooks.log"
	}
	return filepath.Join(home, ".clue-code", "state", "hooks.log")
}

func (m HooksModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m HooksModel) loadCmd() tea.Cmd {
	logPath := m.logPath
	return func() tea.Msg {
		entries, err := readHooksLog(logPath)
		return hooksLoadedMsg{entries: entries, err: err}
	}
}

// readHooksLog reads the NDJSON hooks log and returns the last maxHookEvents entries.
func readHooksLog(path string) ([]hooks.LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("hooks log: %w", err)
	}
	defer func() { _ = f.Close() }()

	var all []hooks.LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e hooks.LogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		all = append(all, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("hooks log scan: %w", err)
	}

	// Return only the most recent maxHookEvents entries (oldest first for display).
	if len(all) > maxHookEvents {
		all = all[len(all)-maxHookEvents:]
	}
	return all, nil
}

func (m HooksModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case hooksLoadedMsg:
		m.entries = msg.entries
		m.err = msg.err
		m.selected = len(m.entries) - 1
		if m.selected < 0 {
			m.selected = 0
		}
		m.offset = 0

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keyQ):
			return m, tea.Quit
		case key.Matches(msg, m.keyR):
			return m, m.loadCmd()
		case key.Matches(msg, m.keyJ):
			if m.selected < len(m.entries)-1 {
				m.selected++
				if m.selected >= m.offset+m.visibleRows() {
					m.offset++
				}
			}
		case key.Matches(msg, m.keyK):
			if m.selected > 0 {
				m.selected--
				if m.selected < m.offset {
					m.offset--
				}
			}
		}
	}
	return m, nil
}

func (m HooksModel) visibleRows() int {
	rows := m.height - 3
	if rows < 1 {
		return 1
	}
	return rows
}

func (m HooksModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(hooksHeaderStyle.Width(m.width).Render(
		fmt.Sprintf("clue-code — hooks log (last %d)", maxHookEvents)))
	b.WriteByte('\n')

	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n", m.err))
		b.WriteString(hooksHelpStyle.Render("r reload  q quit"))
		return b.String()
	}

	if len(m.entries) == 0 {
		b.WriteString("no hook events recorded\n")
		b.WriteString(hooksHelpStyle.Render("r reload  q quit"))
		return b.String()
	}

	visible := m.visibleRows()
	end := m.offset + visible
	if end > len(m.entries) {
		end = len(m.entries)
	}

	for i, e := range m.entries[m.offset:end] {
		idx := m.offset + i
		row := m.renderEntry(e)
		if idx == m.selected {
			b.WriteString(hooksSelectedRowStyle.Width(m.width).Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteByte('\n')
	}

	b.WriteString(hooksHelpStyle.Render("r reload  j/k scroll  q quit"))
	return b.String()
}

func (m HooksModel) renderEntry(e hooks.LogEntry) string {
	ts := e.Timestamp.Format(time.RFC3339)
	blocking := e.ExitCode != 0 || e.TimedOut

	status := "ok"
	if e.TimedOut {
		status = "timeout"
	} else if e.ExitCode != 0 {
		status = fmt.Sprintf("exit=%d", e.ExitCode)
	}

	line := fmt.Sprintf("%-20s  %-20s  %-8s  %dms  %s",
		ts[:min(len(ts), 20)],
		string(e.Event)[:min(len(string(e.Event)), 20)],
		status,
		e.DurationMS,
		truncateStr(e.Command, 20),
	)

	if lipgloss.Width(line) > m.width-1 {
		runes := []rune(line)
		if len(runes) > m.width-1 {
			line = string(runes[:m.width-1])
		}
	}

	if blocking {
		return hooksBlockingStyle.Render(line)
	}
	return hooksInfoStyle.Render(line)
}

func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
