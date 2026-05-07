//go:build tui

package views

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	agentHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#efefef"}).
				Background(lipgloss.AdaptiveColor{Light: "#d0e8ff", Dark: "#1a3a5c"}).
				Padding(0, 1)

	agentSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#c8e0ff", Dark: "#2a4a6c"}).
				Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})

	agentNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#222222", Dark: "#dddddd"})

	agentHelpStyle2 = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

	agentDetailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#444444", Dark: "#aaaaaa"}).
				Border(lipgloss.RoundedBorder()).
				Padding(0, 1)
)

type agentEntry struct {
	name    string
	excerpt string
	body    string
}

type agentsLoadedMsg struct {
	agents []agentEntry
	err    error
}

// AgentsModel lists available agents and shows their details on selection.
type AgentsModel struct {
	agents     []agentEntry
	err        error
	selected   int
	showDetail bool
	width      int
	height     int
	offset     int

	keyQ     key.Binding
	keyJ     key.Binding
	keyK     key.Binding
	keyEnter key.Binding
	keyEsc   key.Binding
}

// NewAgentsModel returns an initialized AgentsModel.
func NewAgentsModel() AgentsModel {
	return AgentsModel{
		keyQ:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		keyJ:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		keyK:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		keyEnter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		keyEsc:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func (m AgentsModel) Init() tea.Cmd {
	return loadAgentsCmd()
}

func loadAgentsCmd() tea.Cmd {
	return func() tea.Msg {
		dir := agentsDir()
		entries, err := os.ReadDir(dir)
		if err != nil {
			return agentsLoadedMsg{err: fmt.Errorf("agents dir: %w", err)}
		}
		var agents []agentEntry
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
			if rerr != nil {
				continue
			}
			body := string(data)
			excerpt := extractExcerpt(body, 80)
			agents = append(agents, agentEntry{name: name, excerpt: excerpt, body: body})
		}
		return agentsLoadedMsg{agents: agents}
	}
}

func agentsDir() string {
	// Resolve relative to the binary's module root: walk up from binary location.
	// Fallback: look for agents/ relative to cwd.
	if _, file, _, ok := runtime.Caller(0); ok {
		// file is something like .../internal/tui/views/agents.go
		// Walk up 4 levels to repo root.
		dir := filepath.Dir(file)
		for i := 0; i < 4; i++ {
			dir = filepath.Dir(dir)
		}
		candidate := filepath.Join(dir, "agents")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "agents"
}

func extractExcerpt(body string, maxLen int) string {
	// Skip YAML front matter (--- ... ---).
	lines := strings.Split(body, "\n")
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				start = i + 1
				break
			}
		}
	}
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasPrefix(line, "#") {
			if len(line) > maxLen {
				line = line[:maxLen-3] + "..."
			}
			return line
		}
	}
	return ""
}

func (m AgentsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case agentsLoadedMsg:
		m.agents = msg.agents
		m.err = msg.err
		m.selected = 0
		m.offset = 0

	case tea.KeyMsg:
		if m.showDetail {
			switch {
			case key.Matches(msg, m.keyEsc), key.Matches(msg, m.keyQ):
				m.showDetail = false
			}
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keyQ):
			return m, tea.Quit
		case key.Matches(msg, m.keyJ):
			if m.selected < len(m.agents)-1 {
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
		case key.Matches(msg, m.keyEnter):
			if len(m.agents) > 0 {
				m.showDetail = true
			}
		}
	}
	return m, nil
}

func (m AgentsModel) visibleRows() int {
	rows := m.height - 3
	if rows < 1 {
		return 1
	}
	return rows
}

func (m AgentsModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(agentHeaderStyle.Width(m.width).Render("clue-code — agents"))
	b.WriteByte('\n')

	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n", m.err))
		return b.String()
	}

	if m.showDetail && len(m.agents) > 0 {
		a := m.agents[m.selected]
		detail := truncateLines(a.body, m.width-4, m.height-3)
		b.WriteString(agentDetailStyle.Width(m.width - 2).Render(detail))
		b.WriteByte('\n')
		b.WriteString(agentHelpStyle2.Render("esc back"))
		return b.String()
	}

	visible := m.visibleRows()
	end := m.offset + visible
	if end > len(m.agents) {
		end = len(m.agents)
	}

	for i, a := range m.agents[m.offset:end] {
		idx := m.offset + i
		row := fmt.Sprintf("%-20s  %s", a.name, a.excerpt)
		if lipgloss.Width(row) > m.width-2 {
			row = row[:m.width-2]
		}
		if idx == m.selected {
			b.WriteString(agentSelectedStyle.Width(m.width).Render(row))
		} else {
			b.WriteString(agentNormalStyle.Render(row))
		}
		b.WriteByte('\n')
	}

	b.WriteString(agentHelpStyle2.Render("j/k navigate  enter detail  q quit"))
	return b.String()
}

func truncateLines(s string, maxWidth, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	var out []string
	for _, l := range lines {
		if lipgloss.Width(l) > maxWidth {
			runes := []rune(l)
			if len(runes) > maxWidth {
				l = string(runes[:maxWidth])
			}
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
