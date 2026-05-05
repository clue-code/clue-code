package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/clue-code/clue-code/internal/state"
)

func runState(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: clue-code state <read|write|clear|list-active|status|metrics> [flags]")
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "read":
		stateRead(rest)
	case "write":
		stateWrite(rest)
	case "clear":
		stateClear(rest)
	case "list-active":
		stateListActive(rest)
	case "status":
		stateStatus(rest)
	case "metrics":
		stateMetrics(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown state subcommand: %q\n", sub)
		fmt.Fprintln(os.Stderr, "usage: clue-code state <read|write|clear|list-active|status|metrics>")
		os.Exit(2)
	}
}

func stateRead(args []string) {
	fs := flag.NewFlagSet("state read", flag.ExitOnError)
	scopeFlag := fs.String("scope", "project", "scope: global|project|session")
	sessionID := fs.String("session", "", "session ID (required for scope=session)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: clue-code state read [--scope=...] <key>")
		os.Exit(2)
	}
	key := fs.Arg(0)
	sc, err := parseScope(*scopeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	store, err := state.Open(*sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: open:", err)
		os.Exit(1)
	}
	val, ver, exists, err := store.Read(context.Background(), key, sc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: read:", err)
		os.Exit(1)
	}
	if !exists {
		fmt.Fprintf(os.Stderr, "key %q not found\n", key)
		os.Exit(1)
	}
	fmt.Printf("version: %d\n", ver)
	fmt.Printf("value:   %s\n", val)
}

func stateWrite(args []string) {
	fs := flag.NewFlagSet("state write", flag.ExitOnError)
	scopeFlag := fs.String("scope", "project", "scope: global|project|session")
	sessionID := fs.String("session", "", "session ID (required for scope=session)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: clue-code state write [--scope=...] <key> <value>")
		os.Exit(2)
	}
	key, value := fs.Arg(0), []byte(fs.Arg(1))
	sc, err := parseScope(*scopeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	store, err := state.Open(*sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: open:", err)
		os.Exit(1)
	}
	ver, err := store.Write(context.Background(), key, value, sc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: write:", err)
		os.Exit(1)
	}
	fmt.Printf("written version: %d\n", ver)
}

func stateClear(args []string) {
	fs := flag.NewFlagSet("state clear", flag.ExitOnError)
	scopeFlag := fs.String("scope", "project", "scope: global|project|session")
	sessionID := fs.String("session", "", "session ID (required for scope=session)")
	prefix := fs.String("prefix", "", "key prefix to clear (empty clears all)")
	yesIKnow := fs.Bool("yes-i-know", false, "required when --scope=global")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	sc, err := parseScope(*scopeFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if sc == state.ScopeGlobal && !*yesIKnow {
		fmt.Fprintln(os.Stderr, "clue-code state clear --scope=global requires --yes-i-know flag")
		fmt.Fprintln(os.Stderr, "This will remove all global state. Pass --yes-i-know to confirm.")
		os.Exit(2)
	}
	store, err := state.Open(*sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: open:", err)
		os.Exit(1)
	}
	removed, err := store.Clear(context.Background(), sc, *prefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: clear:", err)
		os.Exit(1)
	}
	fmt.Printf("removed: %d key(s)\n", removed)
}

func stateListActive(args []string) {
	fs := flag.NewFlagSet("state list-active", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	sessions, err := state.ListActive()
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: list-active:", err)
		os.Exit(1)
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sessions); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(sessions) == 0 {
		fmt.Println("no active sessions")
		return
	}
	for _, s := range sessions {
		fmt.Printf("%-36s  project=%-40s  pid=%-6d  skill=%s\n",
			s.ID, s.ProjectPath, s.PID, s.Skill)
	}
}

func stateStatus(args []string) {
	fs := flag.NewFlagSet("state status", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: clue-code state status [--json] <session-id>")
		os.Exit(2)
	}
	sid := fs.Arg(0)
	status, err := state.GetStatus(sid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "state: status:", err)
		os.Exit(1)
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(status); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Printf("id:            %s\n", status.Descriptor.ID)
	fmt.Printf("state:         %s\n", status.State)
	fmt.Printf("project:       %s\n", status.Descriptor.ProjectPath)
	fmt.Printf("pid:           %d\n", status.Descriptor.PID)
	fmt.Printf("skill:         %s\n", status.Descriptor.Skill)
	fmt.Printf("started:       %s\n", status.Descriptor.StartedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Printf("last_activity: %s\n", status.Descriptor.LastActivity.Format("2006-01-02T15:04:05Z"))
	fmt.Printf("pending_tasks: %d\n", status.PendingTasks)
}

func stateMetrics(args []string) {
	fs := flag.NewFlagSet("state metrics", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	m := state.SnapshotMetrics()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseScope(s string) (state.Scope, error) {
	switch s {
	case "global":
		return state.ScopeGlobal, nil
	case "project":
		return state.ScopeProject, nil
	case "session":
		return state.ScopeSession, nil
	default:
		return 0, fmt.Errorf("unknown scope %q: must be global|project|session", s)
	}
}
