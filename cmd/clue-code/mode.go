package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/clue-code/clue-code/internal/config"
	"github.com/clue-code/clue-code/internal/orchestrator"
)

const modeUsage = `Usage: clue-code mode <action> [arguments]

Actions:
  get          Show the current mode and active providers for that mode
  set <mode>   Persist a new mode (local|cloud|hybrid)

Flags:
  -h, --help   Show this message
`

// runMode is the entry point for "clue-code mode {get,set}".
// It installs a signal-aware context so that Ctrl-C during a slow config
// path write does not leave a partially-written file.
func runMode(ctx context.Context, args []string) int {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet("mode", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, modeUsage) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if fs.NArg() == 0 {
		fmt.Fprint(os.Stderr, modeUsage)
		return 2
	}

	action := fs.Arg(0)
	switch action {
	case "get":
		return runModeGet(ctx)
	case "set":
		if fs.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "clue-code mode set: missing mode argument (local|cloud|hybrid)")
			return 2
		}
		return runModeSet(ctx, fs.Arg(1))
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, modeUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "clue-code mode: unknown action %q\n\n", action)
		fmt.Fprint(os.Stderr, modeUsage)
		return 2
	}
}

// runModeGet prints the currently persisted mode and the set of providers
// that would be used in that mode.
func runModeGet(ctx context.Context) int {
	path, err := modeConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code mode get: %v\n", err)
		return 1
	}

	// Check context before I/O.
	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "clue-code mode get: cancelled")
		return 1
	default:
	}

	mode, err := config.LoadMode(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code mode get: %v\n", err)
		return 1
	}

	fmt.Printf("mode: %s\n", mode)
	fmt.Printf("config: %s\n", path)
	fmt.Printf("active providers:\n")
	for _, p := range activeProvidersForMode(mode) {
		fmt.Printf("  - %s\n", p)
	}
	return 0
}

// runModeSet validates and persists a new mode.
func runModeSet(ctx context.Context, modeStr string) int {
	mode, err := orchestrator.ParseMode(modeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code mode set: %v\n", err)
		return 2
	}

	path, err := modeConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code mode set: %v\n", err)
		return 1
	}

	// Check context before writing.
	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "clue-code mode set: cancelled")
		return 1
	default:
	}

	if err := config.SaveMode(path, mode); err != nil {
		fmt.Fprintf(os.Stderr, "clue-code mode set: %v\n", err)
		return 1
	}

	fmt.Printf("mode set to %q (saved to %s)\n", mode, path)
	return 0
}

// modeConfigPath returns the path used to persist the mode setting.
// Uses the CLUE_CODE_CONFIG env override when set (supports test isolation).
func modeConfigPath() (string, error) {
	return config.JSONConfigPath()
}

// activeProvidersForMode returns a human-readable list of providers
// that are eligible in the given mode.
func activeProvidersForMode(mode config.Mode) []string {
	switch mode {
	case config.ModeLocal:
		return []string{"ollama (local)", "mlx (local)"}
	case config.ModeCloud:
		return []string{"deepseek (cloud)", "anthropic (cloud)", "groq (cloud)", "openrouter (cloud)"}
	default: // hybrid
		return []string{
			"ollama (local, preferred for read/edit)",
			"mlx (local, preferred for read/edit)",
			"deepseek (cloud, preferred for architecture)",
			"anthropic (cloud, preferred for architecture)",
		}
	}
}
