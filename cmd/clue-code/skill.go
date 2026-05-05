package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/clue-code/clue-code/internal/hooks"
	"github.com/clue-code/clue-code/internal/skillrunner"
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
	fs := flag.NewFlagSet("skill run", flag.ExitOnError)
	skillsDir := fs.String("skills-dir", "skills", "directory containing skill subdirectories")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code skill run [flags] <name> [args...]\n\nRun a named skill.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "skill run: skill name required")
		fs.Usage()
		os.Exit(2)
	}
	name, skillArgs := rest[0], rest[1:]

	cfg, err := hooks.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill run: load hooks config: %v\n", err)
		os.Exit(1)
	}

	mgr, err := hooks.NewManager(cfg, ".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill run: new hooks manager: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = mgr.Close() }()

	eng := skillrunner.NewEngine(mgr)
	if loadErr := eng.Load(*skillsDir); loadErr != nil {
		fmt.Fprintf(os.Stderr, "skill run: load warnings: %v\n", loadErr)
	}

	if err := eng.Run(context.Background(), name, skillArgs); err != nil {
		fmt.Fprintf(os.Stderr, "skill run: %v\n", err)
		os.Exit(1)
	}
}
