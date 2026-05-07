//go:build tui

package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clue-code/clue-code/internal/clock"
	"github.com/clue-code/clue-code/internal/tokens"
)

var (
	tokensHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#efefef"}).
				Background(lipgloss.AdaptiveColor{Light: "#fff0d0", Dark: "#3c2800"}).
				Padding(0, 1)

	tokensCellStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#222222", Dark: "#dddddd"})

	tokensHeaderRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#aaaaaa"})

	tokensHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})
)

type tokensLoadedMsg struct {
	today Report3
	week  Report3
	month Report3
	err   error
}

// Report3 holds aggregated token/cost data for one window.
type Report3 struct {
	label       string
	totalUSD    float64
	totalTokens int
	byProvider  map[string]float64
	byModel     map[string]float64
}

// TokensModel displays token usage by provider/model for today, 7d, 30d.
type TokensModel struct {
	today  Report3
	week   Report3
	month  Report3
	err    error
	width  int
	height int

	keyQ key.Binding
	keyR key.Binding
}

// NewTokensModel returns an initialized TokensModel.
func NewTokensModel() TokensModel {
	return TokensModel{
		keyQ: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		keyR: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
	}
}

func (m TokensModel) Init() tea.Cmd {
	return loadTokensCmd()
}

func loadTokensCmd() tea.Cmd {
	return func() tea.Msg {
		a, err := tokens.NewAnalytics("", clock.Real())
		if err != nil {
			return tokensLoadedMsg{err: err}
		}

		toReport3 := func(label string, r tokens.Report) Report3 {
			return Report3{
				label:       label,
				totalUSD:    r.TotalUSD,
				totalTokens: r.TotalTokens,
				byProvider:  r.ByProvider,
				byModel:     r.ByModel,
			}
		}

		return tokensLoadedMsg{
			today: toReport3("today", a.Summary(24*time.Hour)),
			week:  toReport3("7d", a.Summary(7*24*time.Hour)),
			month: toReport3("30d", a.Summary(30*24*time.Hour)),
		}
	}
}

func (m TokensModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tokensLoadedMsg:
		m.today = msg.today
		m.week = msg.week
		m.month = msg.month
		m.err = msg.err

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keyQ):
			return m, tea.Quit
		case key.Matches(msg, m.keyR):
			return m, loadTokensCmd()
		}
	}
	return m, nil
}

func (m TokensModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(tokensHeaderStyle.Width(m.width).Render("clue-code — token usage"))
	b.WriteByte('\n')

	if m.err != nil {
		b.WriteString(fmt.Sprintf("error: %v\n", m.err))
		b.WriteString(tokensHelpStyle.Render("r reload  q quit"))
		return b.String()
	}

	// Summary table header.
	header := fmt.Sprintf("%-8s  %10s  %12s", "window", "tokens", "USD")
	b.WriteString(tokensHeaderRowStyle.Render(header))
	b.WriteByte('\n')

	for _, r := range []Report3{m.today, m.week, m.month} {
		row := fmt.Sprintf("%-8s  %10d  %12.4f", r.label, r.totalTokens, r.totalUSD)
		b.WriteString(tokensCellStyle.Render(row))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')

	// Per-model breakdown for the 7-day window.
	if len(m.week.byModel) > 0 {
		b.WriteString(tokensHeaderRowStyle.Render("7d by model:"))
		b.WriteByte('\n')
		for model, usd := range m.week.byModel {
			row := fmt.Sprintf("  %-30s  $%.4f", model, usd)
			if lipgloss.Width(row) > m.width-1 {
				runes := []rune(row)
				if len(runes) > m.width-1 {
					row = string(runes[:m.width-1])
				}
			}
			b.WriteString(tokensCellStyle.Render(row))
			b.WriteByte('\n')
		}
	}

	b.WriteString(tokensHelpStyle.Render("r reload  q quit"))
	return b.String()
}
