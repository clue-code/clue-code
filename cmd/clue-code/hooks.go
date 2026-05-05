package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/clue-code/clue-code/internal/hooks"
)

const hooksUsage = `Usage: clue-code hooks <subcommand> [flags]

Subcommands:
  list   List configured hooks from ~/.config/clue-code/hooks.yaml
  test   Fire a hook event with a synthetic payload and print the result
  tail   Stream the hooks log from <project>/.clue-code/state/hooks.log

Run "clue-code hooks <subcommand> -h" for subcommand flags.
`

func runHooks(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, hooksUsage)
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		runHooksList(rest)
	case "test":
		runHooksTest(rest)
	case "tail":
		runHooksTail(rest)
	case "fire-test":
		if !hooksFireTestEnabled {
			fmt.Fprintln(os.Stderr, "hooks fire-test: only available in test builds (-tags=test)")
			os.Exit(2)
		}
		runHooksFireTest(rest)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, hooksUsage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "hooks: unknown subcommand %q\n\n", sub)
		fmt.Fprint(os.Stderr, hooksUsage)
		os.Exit(2)
	}
}

// hooksFireTestEnabled is false by default; set to true by hooks_test_helper.go
// (compiled only with -tags=test).
var hooksFireTestEnabled bool

func runHooksList(args []string) {
	fs := flag.NewFlagSet("hooks list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code hooks list\n\nPrint all configured hooks from ~/.config/clue-code/hooks.yaml.")
	}
	_ = fs.Parse(args)

	cfg, err := hooks.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hooks list: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Events) == 0 {
		fmt.Println("No hooks configured.")
		return
	}

	events := []hooks.Event{
		hooks.EventSessionStart,
		hooks.EventPreToolUse,
		hooks.EventPostToolUse,
		hooks.EventUserPromptSubmit,
		hooks.EventStop,
	}
	for _, ev := range events {
		specs, ok := cfg.Events[ev]
		if !ok || len(specs) == 0 {
			continue
		}
		fmt.Printf("[%s]\n", ev)
		for i, s := range specs {
			attrs := ""
			if s.Blocking {
				attrs += " blocking"
			}
			if s.Inject {
				attrs += " inject"
			}
			if s.Matcher != "" {
				attrs += fmt.Sprintf(" matcher=%q", s.Matcher)
			}
			fmt.Printf("  %d: %s  timeout=%s%s\n", i, s.Command, s.Timeout.Round(time.Millisecond), attrs)
		}
	}
}

func runHooksTest(args []string) {
	fs := flag.NewFlagSet("hooks test", flag.ExitOnError)
	eventFlag := fs.String("event", "SessionStart", "lifecycle event to fire (SessionStart|PreToolUse|PostToolUse|UserPromptSubmit|Stop)")
	toolFlag := fs.String("tool", "", "tool name for matcher filtering (PreToolUse/PostToolUse)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code hooks test [flags]\n\nFire a hook event with a synthetic payload and print results.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	ev := hooks.Event(*eventFlag)
	if !ev.Valid() {
		fmt.Fprintf(os.Stderr, "hooks test: unknown event %q\n", ev)
		os.Exit(1)
	}

	cfg, err := hooks.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hooks test: load config: %v\n", err)
		os.Exit(1)
	}

	mgr, err := hooks.NewManager(cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "hooks test: new manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	payload := map[string]any{
		"session_id": "hooks-test",
		"tool":       *toolFlag,
	}

	fmt.Printf("Firing %s...\n", ev)
	injected, fireErr := mgr.Fire(context.Background(), ev, payload)
	if fireErr != nil {
		fmt.Fprintf(os.Stderr, "hooks test: %v\n", fireErr)
		os.Exit(1)
	}
	if injected != "" {
		fmt.Printf("Injected output:\n%s\n", injected)
	} else {
		fmt.Println("OK (no injected output)")
	}
}

func runHooksTail(args []string) {
	fs := flag.NewFlagSet("hooks tail", flag.ExitOnError)
	projectDir := fs.String("project", ".", "project directory containing .clue-code/state/hooks.log")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code hooks tail [flags]\n\nStream new lines from the hooks log.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	logPath := filepath.Join(*projectDir, ".clue-code", "state", "hooks.log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "hooks tail: log not found at %s (no hooks have fired yet)\n", logPath)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "hooks tail: open log: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Seek(0, 2); err != nil {
		fmt.Fprintf(os.Stderr, "hooks tail: seek: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Tailing %s (Ctrl-C to stop)...\n", logPath)
	scanner := bufio.NewScanner(f)
	for {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				fmt.Println(line)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "hooks tail: scan: %v\n", err)
			os.Exit(1)
		}
		time.Sleep(200 * time.Millisecond)
		scanner = bufio.NewScanner(f)
	}
}
