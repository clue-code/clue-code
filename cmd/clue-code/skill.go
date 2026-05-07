package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/clue-code/clue-code/internal/adapters/aider"
	"github.com/clue-code/clue-code/internal/hooks"
	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/skillrunner"
	"github.com/clue-code/clue-code/internal/state"
)

const skillUsage = `Usage: clue-code skill <subcommand> [flags]

Subcommands:
  list   List available skills from the skills/ directory
  run    Run a named skill

Run "clue-code skill <subcommand> -h" for subcommand flags.
`

func runSkill(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, skillUsage)
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		runSkillList(rest)
	case "run":
		runSkillRun(rest)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, skillUsage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "skill: unknown subcommand %q\n\n", sub)
		fmt.Fprint(os.Stderr, skillUsage)
		os.Exit(2)
	}
}

func runSkillList(args []string) {
	fs := flag.NewFlagSet("skill list", flag.ExitOnError)
	skillsDir := fs.String("skills-dir", "skills", "directory containing skill subdirectories")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code skill list [flags]\n\nList all available skills.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	entries, err := os.ReadDir(*skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No skills found (skills/ directory does not exist).")
			return
		}
		fmt.Fprintf(os.Stderr, "skill list: %v\n", err)
		os.Exit(1)
	}

	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(*skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			continue
		}
		fmt.Printf("  %s\n", entry.Name())
		found = true
	}
	if !found {
		fmt.Println("No skills found.")
	}
}

func runSkillRun(args []string) {
	// Install signal handler FIRST so SIGINT during init still triggers
	// graceful cancellation rather than killing the process before defers run
	// (G5 acceptance: Stop hook must fire on Ctrl-C).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet("skill run", flag.ExitOnError)
	skillsDir := fs.String("skills-dir", "skills", "directory containing skill subdirectories")
	useAider := fs.Bool("use-aider", false, "apply edits via the aider AI coding assistant when available (falls back to default edit logic when aider is not installed)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code skill run [flags] <name> [args...]\n\nRun a named skill.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "skill run: skill name required\nusage: skill run requires at least one argument")
		fs.Usage()
		os.Exit(2)
	}
	name, skillArgs := rest[0], rest[1:]

	hooksCfg, err := hooks.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill run: load hooks config: %v\n", err)
		os.Exit(1)
	}

	mgr, err := hooks.NewManager(hooksCfg, ".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill run: new hooks manager: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = mgr.Close() }()

	eng := skillrunner.NewEngine(mgr)
	if loadErr := eng.Load(*skillsDir); loadErr != nil {
		fmt.Fprintf(os.Stderr, "skill run: load warnings: %v\n", loadErr)
	}

	// When --use-aider is requested, probe for the aider binary.
	// Available()=false is non-fatal: the warning is already emitted by
	// NewClient via slog.Warn and execution continues with the default runner.
	var aiderClient *aider.Client
	if *useAider {
		aiderClient = aider.NewClient()
		if aiderClient.Available() {
			repoRoot, _ := os.Getwd()
			eng.WithAiderApply(func(instruction string) ([]string, string, error) {
				return aiderClient.Apply(ctx, instruction, repoRoot)
			})
		}
	}

	// Wire RealRunner when a model client is available.
	// If the API key is missing the engine falls back to the default no-op runner,
	// which allows skills like cancel to execute without requiring a model.
	modelCfg, modelErr := model.LoadConfig()
	if modelErr != nil {
		fmt.Fprintf(os.Stderr, "skill run: load model config: %v\n", modelErr)
		os.Exit(1)
	}
	client, clientErr := model.NewClient(modelCfg, modelCfg.DefaultModel)
	if clientErr == nil {
		store, storeErr := state.Open(fmt.Sprintf("skill-%s", name))
		if storeErr != nil {
			fmt.Fprintf(os.Stderr, "skill run: open state store: %v\n", storeErr)
			os.Exit(1)
		}
		runner := skillrunner.NewRealRunner(client, store, mgr, os.Stdout)
		eng.WithRunner(runner)
	} else {
		fmt.Fprintf(os.Stderr, "skill run: model unavailable (%v); running in offline mode\n", clientErr)
	}

	if err := eng.Run(ctx, name, skillArgs); err != nil {
		fmt.Fprintf(os.Stderr, "skill run: %v\n", err)
		os.Exit(1)
	}
}
