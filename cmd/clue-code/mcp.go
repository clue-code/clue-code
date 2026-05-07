package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/clue-code/clue-code/internal/adapters/mcp"
)

// runMCP implements the `clue-code mcp` subcommand.
func runMCP(ctx context.Context, args []string) int {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: clue-code mcp <list|call> [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return mcpList(ctx, rest)
	case "call":
		return mcpCall(ctx, rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown mcp subcommand: %q\n", sub)
		fmt.Fprintln(os.Stderr, "usage: clue-code mcp <list|call>")
		return 2
	}
}

// mcpList spawns an MCP server and prints its available tools as a table.
//
// Usage: clue-code mcp list <command> [args...]
func mcpList(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("mcp list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: clue-code mcp list <server-command> [server-args...]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	command := fs.Arg(0)
	serverArgs := fs.Args()[1:]

	client, err := mcp.NewClient(ctx, command, serverArgs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp list: connect: %v\n", err)
		return 1
	}
	defer func() { _ = client.Close() }()

	tools, err := client.ListTools(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp list: %v\n", err)
		return 1
	}

	if len(tools) == 0 {
		fmt.Println("(no tools available)")
		return 0
	}

	fmt.Printf("%-30s  %s\n", "TOOL", "DESCRIPTION")
	fmt.Printf("%-30s  %s\n", "----", "-----------")
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Printf("%-30s  %s\n", t.Name, desc)
	}
	return 0
}

// mcpCall spawns an MCP server, calls a single tool, and prints the result.
//
// Usage: clue-code mcp call <server-command> [server-args...] -- <tool-name> <json-args>
func mcpCall(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("mcp call", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: clue-code mcp call <server-command> [server-args...] -- <tool-name> <json-args>")
		fmt.Fprintln(os.Stderr, "example: clue-code mcp call npx @modelcontextprotocol/server-filesystem -- read_file '{\"path\":\"/tmp/a.txt\"}'")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Split on "--" separator between server args and tool args.
	remaining := fs.Args()
	sepIdx := -1
	for i, a := range remaining {
		if a == "--" {
			sepIdx = i
			break
		}
	}

	if sepIdx < 0 || sepIdx+2 >= len(remaining) {
		fs.Usage()
		return 2
	}

	command := remaining[0]
	var serverArgs []string
	if sepIdx > 1 {
		serverArgs = remaining[1:sepIdx]
	}
	toolName := remaining[sepIdx+1]
	toolArgsRaw := remaining[sepIdx+2]

	// Validate JSON args.
	toolArgs := json.RawMessage(toolArgsRaw)
	if !json.Valid(toolArgs) {
		fmt.Fprintf(os.Stderr, "mcp call: tool args are not valid JSON: %s\n", toolArgsRaw)
		return 2
	}

	client, err := mcp.NewClient(ctx, command, serverArgs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp call: connect: %v\n", err)
		return 1
	}
	defer func() { _ = client.Close() }()

	result, err := client.CallTool(ctx, toolName, toolArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp call: %v\n", err)
		return 1
	}

	// Pretty-print JSON result if possible.
	var pretty json.RawMessage
	if err := json.Unmarshal(result, &pretty); err == nil {
		out, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println(string(result))
	}
	return 0
}
