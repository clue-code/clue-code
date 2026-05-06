package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/clue-code/clue-code/internal/config"
	"github.com/clue-code/clue-code/internal/orchestrator"
	"github.com/clue-code/clue-code/internal/version"
)

// runDoctor inspects the local environment and prints a health report.
func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose output")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	fmt.Println("CLUE CODE — doctor")
	fmt.Println("====================")
	fmt.Printf("Build:       %s\n", version.String())
	fmt.Printf("OS / arch:   %s / %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Go runtime:  %s\n", runtime.Version())
	fmt.Printf("Logical CPU: %d\n", runtime.NumCPU())

	fmt.Println()
	fmt.Println("Configuration:")
	cfg := config.Load()
	fmt.Printf("  Mode:        %s\n", cfg.Mode)
	if *verbose {
		for _, t := range []config.Tier{config.TierL0, config.TierL1, config.TierL2, config.TierL3} {
			fmt.Printf("  Model %s:    %s\n", t, cfg.ModelByTier[t])
		}
	}
	if path, err := config.ConfigPath(); err == nil {
		fmt.Printf("  Config path: %s\n", path)
	}

	fmt.Println()
	fmt.Println("External dependencies:")
	checkBinary("aider", "edit engine (Phase 2+)")
	checkBinary("ollama", "local model runtime (cross-platform fallback)")
	checkBinary("mlx_lm.generate", "Apple Silicon native inference (preferred on macOS)")
	checkBinary("python3", "required for LoRA pipeline (Phase 5+)")
	checkBinary("git", "required for repo-aware operations")

	fmt.Println()
	fmt.Println("Agents:")
	dir := agentsDir()
	reg := orchestrator.NewRegistry()
	if errs := reg.LoadFromDir(dir); len(errs) > 0 {
		fmt.Printf("  Could not fully load agents from %s:\n", dir)
		for _, e := range errs {
			fmt.Printf("    - %v\n", e)
		}
	}
	if reg.Count() == 0 {
		fmt.Printf("  No agents found in %s\n", dir)
	} else {
		fmt.Printf("  %d agent(s) loaded from %s\n", reg.Count(), dir)
		for _, name := range reg.Names() {
			fmt.Printf("    - %s\n", name)
		}
	}

	fmt.Println()
	fmt.Println("Status: ready (Phase 1 MVP scaffold).")
}

func checkBinary(bin, purpose string) {
	if path, err := exec.LookPath(bin); err == nil {
		fmt.Printf("  ✓ %-22s found at %s\n", bin, path)
	} else {
		fmt.Printf("  ✗ %-22s NOT found  — %s\n", bin, purpose)
	}
}

// agentsDir resolves the directory holding agent definitions.
// Resolution order:
//  1. CLUE_CODE_AGENTS_DIR environment variable (absolute path).
//  2. ./agents relative to the current working directory.
//  3. ./agents relative to the binary location (fallback).
func agentsDir() string {
	if v := os.Getenv("CLUE_CODE_AGENTS_DIR"); v != "" {
		return v
	}
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "agents")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "agents"
	}
	return filepath.Join(filepath.Dir(exe), "agents")
}
