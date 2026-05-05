package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrAgentNotFound is returned when an agent lookup fails.
var ErrAgentNotFound = errors.New("agent not found")

// Registry holds the set of agents available at runtime.
//
// The registry is built once at startup by scanning the agents directory
// for *.md files. Lookups are O(1) by name.
type Registry struct {
	agents map[string]*Agent
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]*Agent)}
}

// LoadFromDir scans dir for *.md files and loads each as an Agent.
// Files that fail to parse are skipped with the error returned in errs;
// successfully loaded agents are still registered.
func (r *Registry) LoadFromDir(dir string) (errs []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []error{fmt.Errorf("read agents dir %q: %w", dir, err)}
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", path, err))
			continue
		}
		agent, err := ParseAgentFile(string(raw))
		if err != nil {
			errs = append(errs, fmt.Errorf("parse %s: %w", path, err))
			continue
		}
		r.agents[agent.Name] = agent
	}
	return errs
}

// Get returns the agent with the given name, or ErrAgentNotFound.
func (r *Registry) Get(name string) (*Agent, error) {
	if a, ok := r.agents[name]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrAgentNotFound, name)
}

// Names returns the sorted list of registered agent names.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.agents))
	for name := range r.agents {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Count returns the number of registered agents.
func (r *Registry) Count() int {
	return len(r.agents)
}
