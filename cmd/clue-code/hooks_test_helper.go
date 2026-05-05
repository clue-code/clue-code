//go:build test

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/clue-code/clue-code/internal/hooks"
)

// init enables the "fire-test" subcommand in the hooks dispatcher.
// Only compiled with -tags=test; absent in the production binary.
func init() {
	hooksFireTestEnabled = true
}

// runHooksFireTest fires the named event via a real Manager and exits with a
// code reflecting success (0) or ErrHookDepthExceeded (3).
// Used by TestDepthGuard_Hermetic (A3) to exercise the depth guard end-to-end
// without a subprocess calling back into the test binary.
func runHooksFireTest(args []string) {
	fs := flag.NewFlagSet("hooks fire-test", flag.ExitOnError)
	eventFlag := fs.String("event", "SessionStart", "lifecycle event to fire")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code hooks fire-test -event=<Event>\n\n[test-only] Fire a hook event and exit 0 on success, 3 on ErrHookDepthExceeded.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	ev := hooks.Event(*eventFlag)
	if !ev.Valid() {
		fmt.Fprintf(os.Stderr, "hooks fire-test: unknown event %q\n", ev)
		os.Exit(2)
	}

	cfg, err := hooks.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hooks fire-test: load config: %v\n", err)
		os.Exit(1)
	}

	mgr, err := hooks.NewManager(cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "hooks fire-test: new manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	payload := map[string]any{
		"session_id": "fire-test",
	}

	_, fireErr := mgr.Fire(context.Background(), ev, payload)
	if fireErr != nil {
		if isDepthExceeded(fireErr) {
			os.Exit(3)
		}
		fmt.Fprintf(os.Stderr, "hooks fire-test: %v\n", fireErr)
		os.Exit(1)
	}
	os.Exit(0)
}

func isDepthExceeded(err error) bool {
	return errors.Is(err, hooks.ErrHookDepthExceeded)
}
