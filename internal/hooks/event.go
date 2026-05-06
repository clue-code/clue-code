package hooks

// Event names a lifecycle hook point.
type Event string

const (
	EventSessionStart     Event = "SessionStart"
	EventPreToolUse       Event = "PreToolUse"
	EventPostToolUse      Event = "PostToolUse"
	EventUserPromptSubmit Event = "UserPromptSubmit"
	EventStop             Event = "Stop"
)

var knownEvents = map[Event]struct{}{
	EventSessionStart:     {},
	EventPreToolUse:       {},
	EventPostToolUse:      {},
	EventUserPromptSubmit: {},
	EventStop:             {},
}

// Valid reports whether e is one of the five known lifecycle events.
func (e Event) Valid() bool {
	_, ok := knownEvents[e]
	return ok
}
