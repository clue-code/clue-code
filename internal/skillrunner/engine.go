package skillrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/clue-code/clue-code/internal/hooks"
	"github.com/clue-code/clue-code/internal/state"
)

const (
	// EnvSkillDepth tracks recursion depth across skill invocations.
	// Justified by the autopilot→ralph→executor→verifier chain (4 levels).
	EnvSkillDepth = "CLUE_CODE_SKILL_DEPTH"
	MaxSkillDepth = 4
)

// ErrSkillDepthExceeded is returned when a skill invocation would exceed MaxSkillDepth.
var ErrSkillDepthExceeded = errors.New("skillrunner: depth exceeded")

// RunFunc is the function the Engine uses to execute a skill's body.
// It is a field so tests can substitute a seam without subprocess overhead.
type RunFunc func(ctx context.Context, skill *Skill, args []string) error

// Engine loads and executes skills with hook lifecycle integration.
type Engine struct {
	hm     *hooks.Manager
	skills map[string]*Skill
	runFn  RunFunc
	runner Runner
}

// NewEngine constructs an Engine. hm may be nil for hook-less operation.
func NewEngine(hm *hooks.Manager) *Engine {
	return &Engine{
		hm:     hm,
		skills: make(map[string]*Skill),
		runFn:  defaultRunFn,
	}
}

// Load reads all skills from skillsDir. Non-fatal parse errors are returned
// as LoadErrors but successfully parsed skills are still registered.
func (e *Engine) Load(skillsDir string) error {
	loaded, errs := loadSkillsDir(skillsDir)
	for name, s := range loaded {
		e.skills[name] = s
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// Run executes the named skill with args.
// It enforces the skill depth guard, fires SessionStart/Stop lifecycle hooks,
// and wraps the execution in recover() so panics don't skip Stop.
func (e *Engine) Run(ctx context.Context, name string, args []string) (retErr error) {
	// Sanitize name to prevent path traversal.
	if err := validateSkillName(name); err != nil {
		return err
	}

	// Depth guard: read current depth from env.
	depth, err := skillDepth()
	if err != nil {
		return fmt.Errorf("skillrunner: read depth env: %w", err)
	}
	if depth >= MaxSkillDepth {
		return ErrSkillDepthExceeded
	}

	skill, ok := e.skills[name]
	if !ok {
		return fmt.Errorf("skillrunner: skill %q not found", name)
	}

	sessionID := fmt.Sprintf("skill-%s-%d", name, depth)

	// Fire SessionStart before any execution.
	if err := fireLifecycle(ctx, e.hm, hooks.EventSessionStart, sessionID, name, args); err != nil {
		return err
	}

	// Stop fires after execution regardless of panic or cancellation.
	defer func() {
		// Use a fresh background context for Stop in case ctx was cancelled.
		stopCtx := context.Background()
		stopErr := fireLifecycle(stopCtx, e.hm, hooks.EventStop, sessionID, name, args)
		if retErr == nil {
			retErr = stopErr
		}
	}()

	// Propagate incremented depth to child processes.
	childDepthEnv := EnvSkillDepth + "=" + strconv.Itoa(depth+1)
	_ = childDepthEnv // used by subprocess env when skill spawns child processes

	// Wrap in recover() per E3: panics must not skip Stop hook.
	var runErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("skillrunner: skill %q panicked: %v", name, r)
			}
		}()
		if e.runner != nil {
			runErr = e.runner.Run(ctx, skill, args)
		} else {
			runErr = e.runFn(ctx, skill, args)
		}
	}()

	return runErr
}

// WithRunFunc replaces the skill execution function (test seam).
func (e *Engine) WithRunFunc(fn RunFunc) *Engine {
	e.runFn = fn
	return e
}

// WithRunner sets a Runner that takes priority over RunFunc.
// Use this to wire RealRunner in production.
func (e *Engine) WithRunner(r Runner) *Engine {
	e.runner = r
	return e
}

// SetSkill registers a skill directly without loading from disk.
// Useful in tests to inject synthetic skills.
func (e *Engine) SetSkill(name string, s *Skill) {
	e.skills[name] = s
}

// defaultRunFn is a no-op executor. Real skill execution would interpret
// the skill body as a prompt or script; for now the body is informational.
func defaultRunFn(ctx context.Context, skill *Skill, args []string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// skillDepth reads CLUE_CODE_SKILL_DEPTH from the environment.
func skillDepth() (int, error) {
	val := os.Getenv(EnvSkillDepth)
	if val == "" {
		return 0, nil
	}
	d, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", EnvSkillDepth, val, err)
	}
	return d, nil
}

// validateSkillName delegates to state.SanitizeIdentifier — the canonical
// flat-identifier validator across internal/* packages. A skill name becomes
// a directory component (skills/<name>/SKILL.md) so it must not contain path
// separators, traversal segments, NUL bytes, or empty input.
func validateSkillName(name string) error {
	if _, err := state.SanitizeIdentifier(name); err != nil {
		return fmt.Errorf("skillrunner: %w", err)
	}
	return nil
}
