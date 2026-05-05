// Package orchestrator implements the CLUE CODE multi-agent core:
// the agent registry, router, and (in later phases) MoA aggregator.
package orchestrator

import (
	"bufio"
	"fmt"
	"strings"
)

// Agent represents a typed CLUE CODE agent loaded from a markdown file
// with YAML frontmatter.
type Agent struct {
	// Name is the unique identifier of the agent (matches the filename stem).
	Name string
	// Description is a one-line summary used for routing and discovery.
	Description string
	// Model is the model id this agent prefers (e.g. "qwen3-coder:30b").
	Model string
	// Level is the routing tier (L0/L1/L2/L3).
	Level string
	// Prompt is the agent's system prompt body (Markdown, post-frontmatter).
	Prompt string
}

// ParseAgentFile parses an agent definition from raw markdown content.
//
// Expected format:
//
//	---
//	name: executor
//	description: Focused task executor
//	model: qwen3-coder:30b
//	level: L1
//	---
//
//	<prompt body in Markdown>
//
// Frontmatter keys are case-insensitive, values are trimmed. Unknown keys
// are ignored (forward-compatible).
func ParseAgentFile(content string) (*Agent, error) {
	a := &Agent{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	state := stateStart
	var bodyLines []string

	for scanner.Scan() {
		line := scanner.Text()
		switch state {
		case stateStart:
			if strings.TrimSpace(line) == "---" {
				state = stateFrontmatter
				continue
			}
			// No frontmatter — treat full file as prompt.
			bodyLines = append(bodyLines, line)
			state = stateBody
		case stateFrontmatter:
			if strings.TrimSpace(line) == "---" {
				state = stateBody
				continue
			}
			parseFrontmatterLine(a, line)
		case stateBody:
			bodyLines = append(bodyLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan agent: %w", err)
	}
	if a.Name == "" {
		return nil, fmt.Errorf("agent file is missing required 'name' frontmatter key")
	}
	a.Prompt = strings.TrimLeft(strings.Join(bodyLines, "\n"), "\n")
	return a, nil
}

const (
	stateStart = iota
	stateFrontmatter
	stateBody
)

func parseFrontmatterLine(a *Agent, line string) {
	idx := strings.IndexByte(line, ':')
	if idx <= 0 {
		return
	}
	key := strings.ToLower(strings.TrimSpace(line[:idx]))
	value := strings.TrimSpace(line[idx+1:])
	// Strip optional surrounding quotes.
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	switch key {
	case "name":
		a.Name = value
	case "description":
		a.Description = value
	case "model":
		a.Model = value
	case "level":
		a.Level = value
	}
}
