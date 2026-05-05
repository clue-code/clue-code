package state

// Scope identifies the lifetime and visibility of stored state.
type Scope int

const (
	// ScopeGlobal persists state under ~/.clue-code/ shared across all projects.
	ScopeGlobal Scope = iota
	// ScopeProject persists state under <project-root>/.clue-code/.
	ScopeProject
	// ScopeSession persists state under <project-root>/.clue-code/sessions/<id>/.
	ScopeSession
)

// String returns a human-readable scope name.
func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "global"
	case ScopeProject:
		return "project"
	case ScopeSession:
		return "session"
	default:
		return "unknown"
	}
}
