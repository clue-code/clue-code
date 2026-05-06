// Command clue-code is the CLUE CODE multi-agent orchestrator CLI.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `clue-code — multi-agent AI orchestration OS

Usage:
  clue-code <command> [arguments]

Commands:
  version    Print build information
  doctor     Diagnose the local environment (OS, arch, RAM, deps)
  state      Read, write, and inspect persistent agent state
  hooks      Manage and inspect lifecycle hooks
  skill      List and run skills
  chat       Send a prompt to the configured model
  tui        Launch the terminal UI (requires -tags=tui build)
  help       Show this message

Run "clue-code <command> -h" for command-specific flags.
`

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "version", "-v", "--version":
		runVersion(args)
	case "doctor":
		runDoctor(args)
	case "state":
		runState(args)
	case "hooks":
		runHooks(args)
	case "skill":
		runSkill(args)
	case "chat":
		runChat(args)
	case "tui":
		os.Exit(runTUI(args))
	case "help", "-h", "--help":
		flag.Usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", cmd)
		flag.Usage()
		os.Exit(2)
	}
}
