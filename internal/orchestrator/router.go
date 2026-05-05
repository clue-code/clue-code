package orchestrator

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Router classifies an incoming task and returns the agent that should
// handle it. This is a Phase 1 stub: classification is keyword-based.
// Phase 3 will introduce token-count, criticality, and cost-aware routing.
type Router struct {
	registry *Registry
	// warnOut receives fallback warnings; defaults to os.Stderr but can be
	// overridden in tests.
	warnOut io.Writer
}

// NewRouter returns a Router backed by the given Registry.
func NewRouter(r *Registry) *Router {
	return &Router{registry: r, warnOut: os.Stderr}
}

// Route picks an agent for the given task description.
// If the classified agent is missing, falls back to the first registered
// agent (deterministic via sorted Names) and emits a warning to warnOut.
func (r *Router) Route(task string) (*Agent, error) {
	if r.registry == nil || r.registry.Count() == 0 {
		return nil, fmt.Errorf("router: registry is empty")
	}
	name := classify(task)
	if a, err := r.registry.Get(name); err == nil {
		return a, nil
	}
	names := r.registry.Names()
	if len(names) == 0 {
		return nil, ErrAgentNotFound
	}
	fallback := names[0]
	if r.warnOut != nil {
		fmt.Fprintf(r.warnOut, "warning: agent %q not registered, falling back to %q\n", name, fallback)
	}
	return r.registry.Get(fallback)
}

// classify is the Phase 1 keyword router.
func classify(task string) string {
	t := strings.ToLower(task)
	switch {
	case containsAny(t, "review", "audit", "smell", "code review"):
		return "code-reviewer"
	case containsAny(t, "design", "architecture", "plan a", "system design"):
		return "architect"
	case containsAny(t, "verify", "check", "validate"):
		return "verifier"
	case containsAny(t, "debug", "bug", "crash", "stack trace"):
		return "debugger"
	case containsAny(t, "security", "vulnerability", "owasp", "cve"):
		return "security-reviewer"
	case containsAny(t, "test", "coverage", "unit test", "integration"):
		return "test-engineer"
	case containsAny(t, "explore", "search codebase", "find usage"):
		return "explore"
	case containsAny(t, "document", "readme", "docs"):
		return "writer"
	default:
		return "executor"
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
