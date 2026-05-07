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
	skillHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#efefef"}).
				Background(lipgloss.AdaptiveColor{Light: "#d0ffd8", Dark: "#1a3c20"}).
				Padding(0, 1)

	skillSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#b8f0c0", Dark: "#2a5030"}).
				Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})

	skillNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#222222", Dark: "#dddddd"})

	skillHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

	skillDetailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#444444", Dark: "#aaaaaa"}).
				Border(lipgloss.RoundedBorder()).
				Padding(0, 1)
)

type skillEntry struct {
	name    string
	excerpt string
	body    string
}

type skillsLoadedMsg struct {
	skills []skillEntry
	err    error
}

// SkillsModel lists available skills and shows their details on selection.
type SkillsModel struct {
	skills     []skillEntry
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

// NewSkillsModel returns an initialized SkillsModel.
func NewSkillsModel() SkillsModel {
	return SkillsModel{
		keyQ:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		keyJ:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		keyK:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		keyEnter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		keyEsc:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func (m SkillsModel) Init() tea.Cmd {
	return loadSkillsCmd()
}

func loadSkillsCmd() tea.Cmd {
	return func() tea.Msg {
		dir := skillsBaseDir()
		entries, err := os.ReadDir(dir)
		if err != nil {
			return skillsLoadedMsg{err: fmt.Errorf("skills dir: %w", err)}
		}
		var skills []skillEntry
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillMD := filepath.Join(dir, e.Name(), "SKILL.md")
			data, rerr := os.ReadFile(skillMD)
			if rerr != nil {
				continue
			}
			body := string(data)
			excerpt := extractExcerpt(body, 80)
			skills = append(skills, skillEntry{name: e.Name(), excerpt: excerpt, body: body})
		}
		return skillsLoadedMsg{skills: skills}
	}
}

func skillsBaseDir() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		dir := filepath.Dir(file)
		for i := 0; i < 4; i++ {
			dir = filepath.Dir(dir)
		}
		candidate := filepath.Join(dir, "skills")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "skills"
}

func (m SkillsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case skillsLoadedMsg:
		m.skills = msg.skills
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
			if m.selected < len(m.skills)-1 {
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
			if len(m.skills) > 0 {
				m.showDetail = true
			}
		}
	}
	return m, nil
}

func (m SkillsModel) visibleRows() int {
	rows := m.height - 3
	if rows < 1 {
		return 1
	}
	return rows
}

func (m SkillsModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(skillHeaderStyle.Width(m.width).Render("clue-code — skills"))
	b.WriteByte('\n')

	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n", m.err))
		return b.String()
	}

	if len(m.skills) == 0 {
		b.WriteString("no skills found\n")
		b.WriteString(skillHelpStyle.Render("q quit"))
		return b.String()
	}

	if m.showDetail {
		s := m.skills[m.selected]
		detail := truncateLines(s.body, m.width-4, m.height-3)
		b.WriteString(skillDetailStyle.Width(m.width - 2).Render(detail))
		b.WriteByte('\n')
		b.WriteString(skillHelpStyle.Render("esc back"))
		return b.String()
	}

	visible := m.visibleRows()
	end := m.offset + visible
	if end > len(m.skills) {
		end = len(m.skills)
	}

	for i, s := range m.skills[m.offset:end] {
		idx := m.offset + i
		row := fmt.Sprintf("%-24s  %s", s.name, s.excerpt)
		if lipgloss.Width(row) > m.width-2 {
			runes := []rune(row)
			if len(runes) > m.width-2 {
				row = string(runes[:m.width-2])
			}
		}
		if idx == m.selected {
			b.WriteString(skillSelectedStyle.Width(m.width).Render(row))
		} else {
			b.WriteString(skillNormalStyle.Render(row))
		}
		b.WriteByte('\n')
	}

	b.WriteString(skillHelpStyle.Render("j/k navigate  enter detail  q quit"))
	return b.String()
}
