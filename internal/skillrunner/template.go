package skillrunner

import (
	"bytes"
	"strings"
	"text/template"
	"time"
)

// SkillContext holds the values available to a SKILL.md body template.
type SkillContext struct {
	SkillName   string
	SkillArgs   []string
	ProjectRoot string
	SessionID   string
	Now         time.Time
	UserShell   string
}

// RenderSkillPrompt renders the SKILL.md body with ctx substitution.
// If the body contains no "{{" markers it is returned as-is to avoid
// parse overhead and to keep static bodies working unchanged.
func RenderSkillPrompt(body string, ctx SkillContext) (string, error) {
	if !strings.Contains(body, "{{") {
		return body, nil
	}
	tmpl, err := template.New("skill").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
