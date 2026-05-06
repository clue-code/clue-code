package skillrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/state"
)

// TranscriptEntry is one line in the session transcript NDJSON file.
type TranscriptEntry struct {
	Timestamp time.Time    `json:"ts"`
	Role      string       `json:"role"` // system, user, assistant, tool
	Content   string       `json:"content,omitempty"`
	Delta     string       `json:"delta,omitempty"`
	Usage     *model.Usage `json:"usage,omitempty"`
	Done      bool         `json:"done,omitempty"`
}

// PersistEntry serialises entry as a JSON line and appends it to the
// session-scoped "transcript" key in store.
// sessionID is validated with state.SanitizeIdentifier before use.
func PersistEntry(ctx context.Context, store state.Store, sessionID string, entry TranscriptEntry) error {
	if _, err := state.SanitizeIdentifier(sessionID); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("transcript: marshal: %w", err)
	}
	line := append(data, '\n')
	return store.Append(ctx, "transcript", line, state.ScopeSession)
}
