package hooks

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Manager orchestrates hook execution across lifecycle events.
type Manager struct {
	cfg   *Config
	log   *Log
	mu    sync.Mutex
	depth map[string]int // per-session in-process depth counter
	noLog bool           // true when no projectDir was provided (no-op mode)
}

// NewManager constructs a Manager from cfg.  If cfg has no events and projectDir
// is empty the manager operates in no-op mode (no log file opened).
// projectDir is used solely to open the hooks log; it is validated by OpenLog.
func NewManager(cfg *Config, projectDir string) (*Manager, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	m := &Manager{
		cfg:   cfg,
		depth: make(map[string]int),
	}
	if projectDir == "" || len(cfg.Events) == 0 {
		m.noLog = true
		return m, nil
	}
	l, err := OpenLog(projectDir)
	if err != nil {
		return nil, err
	}
	m.log = l
	return m, nil
}

// Fire runs all specs for ev synchronously.  Blocking specs that exit non-zero
// cause Fire to return an error after all blocking specs for that event have
// been attempted (fail-fast: first failure stops subsequent blocking specs).
// inject:true specs have their stdout wrapped and concatenated into injected.
func (m *Manager) Fire(ctx context.Context, ev Event, payload map[string]any) (injected string, err error) {
	specs, ok := m.cfg.Events[ev]
	if !ok || len(specs) == 0 {
		return "", nil
	}

	sessionID := ""
	if sid, ok := payload["session_id"]; ok {
		sessionID, _ = sid.(string)
	}

	// Env-based depth guard (catches hook→hook recursion via subprocess env).
	envDepth, envErr := hookDepth()
	if envErr != nil {
		return "", fmt.Errorf("hooks: read depth env: %w", envErr)
	}
	if envDepth >= MaxHookDepth {
		return "", ErrHookDepthExceeded
	}

	// In-process depth guard (defense-in-depth for same-process re-entry).
	if sessionID != "" {
		m.mu.Lock()
		cur := m.depth[sessionID]
		if cur >= MaxHookDepth {
			m.mu.Unlock()
			return "", ErrHookDepthExceeded
		}
		m.depth[sessionID]++
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.depth[sessionID]--
			m.mu.Unlock()
		}()
	}

	var sb strings.Builder
	for i := range specs {
		s := specs[i]

		if !m.allowed(s.Command) {
			continue
		}

		if !matchesPayload(s.Matcher, payload) {
			continue
		}

		result, runErr := runSpec(ctx, s)
		if runErr != nil {
			if s.Blocking {
				return sb.String(), fmt.Errorf("hooks: blocking spec %d for %s: %w", i, ev, runErr)
			}
			continue
		}

		m.writeLog(ev, s.Command, result)

		if s.Inject && len(result.Stdout) > 0 {
			sb.WriteString("<hook-context source=\"")
			sb.WriteString(s.Command)
			sb.WriteString("\">")
			sb.Write(result.Stdout)
			sb.WriteString("</hook-context>")
		}

		if s.Blocking && result.ExitCode != 0 {
			return sb.String(), fmt.Errorf("hooks: blocking spec %d for %s exited %d", i, ev, result.ExitCode)
		}
	}
	return sb.String(), nil
}

// FireAndForget spawns a goroutine for each spec in ev, ignoring results.
// Used for SessionStart/Stop where the caller must not block.
func (m *Manager) FireAndForget(ev Event, payload map[string]any) {
	specs, ok := m.cfg.Events[ev]
	if !ok || len(specs) == 0 {
		return
	}
	for i := range specs {
		s := specs[i]
		if !m.allowed(s.Command) {
			continue
		}
		go func(spec Spec) {
			result, err := runSpec(context.Background(), spec)
			if err != nil {
				return
			}
			m.writeLog(ev, spec.Command, result)
		}(s)
	}
}

// Close flushes and closes the underlying log.
func (m *Manager) Close() error {
	if m.log == nil {
		return nil
	}
	return m.log.Close()
}

// allowed returns true when the command is permitted under the current
// allowlist + self-invoke policy.
//
// Truth table (cfg.Allowlist nil/empty = "any"):
//
//	Allowlist | AllowSelfInv | Behavior
//	any       | true         | all commands pass; self-invoke also passes
//	any       | false        | all commands pass EXCEPT self-invoke
//	non-empty | true         | only allowlist; self-invoke also passes
//	non-empty | false        | only allowlist AND no self-invoke
func (m *Manager) allowed(command string) bool {
	self := isSelfInvoke(command)
	if self && !m.cfg.AllowSelfInv {
		return false
	}
	if len(m.cfg.Allowlist) == 0 {
		return true
	}
	for _, pat := range m.cfg.Allowlist {
		matched, err := regexp.MatchString(pat, command)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// isSelfInvoke returns true if command starts with "clue-code" or contains
// a path-prefixed "/clue-code " invocation.
func isSelfInvoke(command string) bool {
	trimmed := strings.TrimSpace(command)
	return strings.HasPrefix(trimmed, "clue-code") ||
		strings.Contains(trimmed, "/clue-code ")
}

// matchesPayload returns true if the spec's matcher is empty or if the
// payload["tool"] value matches the compiled regexp.
func matchesPayload(matcher string, payload map[string]any) bool {
	if matcher == "" {
		return true
	}
	tool, _ := payload["tool"].(string)
	matched, err := regexp.MatchString(matcher, tool)
	return err == nil && matched
}

// writeLog writes a log entry if a log is open.
func (m *Manager) writeLog(ev Event, command string, r Result) {
	if m.log == nil {
		return
	}
	_ = m.log.Write(LogEntry{
		Event:      ev,
		Command:    command,
		DurationMS: r.DurationMS,
		ExitCode:   r.ExitCode,
		TimedOut:   r.TimedOut,
		Truncated:  r.Truncated,
		StdoutLen:  len(r.Stdout),
	})
}
