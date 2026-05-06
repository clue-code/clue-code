package skillrunner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// reloadHooks holds callbacks registered via RegisterReloadHook.
// Intentionally package-level so callers (e.g. the token cache) can register
// without creating an import cycle with internal/tokens.
var (
	reloadMu    sync.RWMutex
	reloadHooks []func(path string)
)

// RegisterReloadHook registers fn to be called whenever a SKILL.md file is
// reloaded. fn receives the absolute path of the reloaded file. Safe to call
// from multiple goroutines.
func RegisterReloadHook(fn func(path string)) {
	reloadMu.Lock()
	reloadHooks = append(reloadHooks, fn)
	reloadMu.Unlock()
}

// OnSkillReload invokes all registered reload hooks for the given path.
// Call this after successfully reloading a skill file.
func OnSkillReload(path string) {
	reloadMu.RLock()
	hooks := make([]func(string), len(reloadHooks))
	copy(hooks, reloadHooks)
	reloadMu.RUnlock()
	for _, fn := range hooks {
		fn(path)
	}
}

// Skill represents a loaded skill with parsed frontmatter and body.
type Skill struct {
	Name        string
	Description string
	Body        string
}

// LoadErrors is a collection of non-fatal load errors (malformed skills).
type LoadErrors []error

func (le LoadErrors) Error() string {
	msgs := make([]string, len(le))
	for i, e := range le {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// loadSkillsDir scans dir/<name>/SKILL.md files and returns a map of loaded
// skills. Malformed skills are skipped but their errors are collected.
func loadSkillsDir(dir string) (map[string]*Skill, LoadErrors) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, LoadErrors{fmt.Errorf("skillrunner: read dir %q: %w", dir, err)}
	}

	skills := make(map[string]*Skill)
	var errs LoadErrors

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("skillrunner: read %q: %w", skillPath, err))
			continue
		}
		skill, err := parseSkillFile(string(data))
		if err != nil {
			errs = append(errs, fmt.Errorf("skillrunner: parse %q: %w", skillPath, err))
			continue
		}
		if skill.Name == "" {
			skill.Name = entry.Name()
		}
		skills[skill.Name] = skill
	}

	return skills, errs
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillFile parses YAML frontmatter delimited by "---" and extracts body.
func parseSkillFile(content string) (*Skill, error) {
	const delim = "---"
	sc := bufio.NewScanner(strings.NewReader(content))

	// State machine: find opening "---", collect frontmatter, find closing "---", rest is body.
	type state int
	const (
		stateStart       state = iota
		stateFrontmatter state = iota
		stateBody        state = iota
	)

	cur := stateStart
	var fmLines []string
	var bodyLines []string

	for sc.Scan() {
		line := sc.Text()
		switch cur {
		case stateStart:
			if strings.TrimSpace(line) == delim {
				cur = stateFrontmatter
			}
		case stateFrontmatter:
			if strings.TrimSpace(line) == delim {
				cur = stateBody
			} else {
				fmLines = append(fmLines, line)
			}
		case stateBody:
			bodyLines = append(bodyLines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	var fm skillFrontmatter
	if len(fmLines) > 0 {
		if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
			return nil, fmt.Errorf("frontmatter yaml: %w", err)
		}
	}

	return &Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Body:        strings.Join(bodyLines, "\n"),
	}, nil
}
