package skillrunner

import (
	"context"
	"fmt"

	"github.com/clue-code/clue-code/internal/hooks"
)

// fireLifecycle fires a lifecycle hook event with a standard payload.
// Errors are returned but callers may choose to log-and-continue for non-critical events.
func fireLifecycle(ctx context.Context, hm *hooks.Manager, ev hooks.Event, sessionID, skillName string, args []string) error {
	if hm == nil {
		return nil
	}
	payload := map[string]any{
		"session_id": sessionID,
		"skill":      skillName,
		"args":       args,
	}
	_, err := hm.Fire(ctx, ev, payload)
	if err != nil {
		return fmt.Errorf("skillrunner: hook %s: %w", ev, err)
	}
	return nil
}
