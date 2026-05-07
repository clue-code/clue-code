package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

// replSession holds the mutable state of a single interactive REPL session.
type replSession struct {
	history    []model.Message
	modelID    string
	tokensUsed model.Usage
}

func newReplSession(modelID string) *replSession {
	return &replSession{modelID: modelID}
}

// Help prints the available meta-commands to w.
func (s *replSession) Help() {
	fmt.Print(`REPL commands:
  /help, /?          Show this message
  /exit, /quit       Exit the REPL (also Ctrl+D)
  /clear             Clear conversation history (start a new topic)
  /save <file>       Save the conversation to a Markdown file
  /model <id>        Switch the active model for subsequent requests
  /tokens            Show cumulative token usage for this session
`)
}

// Clear resets the conversation history.
func (s *replSession) Clear() {
	s.history = s.history[:0]
	fmt.Println("[history cleared]")
}

// SetModel updates the active model ID.
func (s *replSession) SetModel(id string) {
	s.modelID = id
	fmt.Printf("[model set to %s]\n", id)
}

// TokensSummary prints cumulative token usage to stdout.
func (s *replSession) TokensSummary() {
	u := s.tokensUsed
	fmt.Printf("[tokens] prompt=%d completion=%d total=%d\n",
		u.PromptTokens, u.CompletionTokens, u.TotalTokens)
}

// AddUsage accumulates token counts from one request.
func (s *replSession) AddUsage(u model.Usage) {
	s.tokensUsed.PromptTokens += u.PromptTokens
	s.tokensUsed.CompletionTokens += u.CompletionTokens
	s.tokensUsed.TotalTokens += u.TotalTokens
}

// Save writes the conversation history as a Markdown file with YAML frontmatter.
func (s *replSession) Save(path string) error {
	if path == "" {
		return fmt.Errorf("usage: /save <file>")
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "date: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&sb, "model: %s\n", s.modelID)
	fmt.Fprintf(&sb, "tokens: %d\n", s.tokensUsed.TotalTokens)
	sb.WriteString("---\n\n")

	for _, msg := range s.history {
		switch msg.Role {
		case model.RoleUser:
			sb.WriteString("## You\n")
		case model.RoleAssistant:
			sb.WriteString("## Claude\n")
		case model.RoleSystem:
			sb.WriteString("## System\n")
		default:
			sb.WriteString(fmt.Sprintf("## %s\n", msg.Role))
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	fmt.Printf("[conversation saved to %s]\n", path)
	return nil
}

// AppendUser appends a user turn to the history.
func (s *replSession) AppendUser(content string) {
	s.history = append(s.history, model.Message{Role: model.RoleUser, Content: content})
}

// AppendAssistant appends an assistant turn to the history.
func (s *replSession) AppendAssistant(content string) {
	s.history = append(s.history, model.Message{Role: model.RoleAssistant, Content: content})
}
