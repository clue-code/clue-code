package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/orchestrator"
)

const agentUsage = `Usage: clue-code agent <subcommand> [flags]

Subcommands:
  list   List all available agents
  run    Run a named agent (or auto-route) with a task
  moa    Run a Mixture-of-Agents query across multiple models

Run "clue-code agent <subcommand> -h" for subcommand flags.
`

func runAgent(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, agentUsage)
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		runAgentList(rest)
	case "run":
		runAgentRun(rest)
	case "moa":
		runAgentMoA(rest)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, agentUsage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "agent: unknown subcommand %q\n\n", sub)
		fmt.Fprint(os.Stderr, agentUsage)
		os.Exit(2)
	}
}

func runAgentList(args []string) {
	fs := flag.NewFlagSet("agent list", flag.ExitOnError)
	agentsDir := fs.String("agents-dir", "agents", "directory containing agent markdown files")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clue-code agent list [flags]\n\nList all available agents.")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	reg := orchestrator.NewRegistry()
	errs := reg.LoadFromDir(*agentsDir)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "agent list: warning: %v\n", e)
	}

	names := reg.Names()
	if len(names) == 0 {
		fmt.Println("No agents found.")
		return
	}
	for _, name := range names {
		fmt.Printf("  %s\n", name)
	}
}

func runAgentRun(args []string) {
	// Signal handler FIRST — Phase 4.6 lesson: must be at top of runAgent.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet("agent run", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `Usage: clue-code agent run [flags] [<agent-name>] <task>

Run a named agent with the given task. If <agent-name> is omitted, the router
auto-selects the best agent for the task.

Flags:`)
		fs.PrintDefaults()
	}

	var (
		modelID   string
		noStream  bool
		jsonMode  bool
		agentsDir string
	)
	fs.StringVar(&modelID, "model", "", "model ID override (default: config default_model)")
	fs.BoolVar(&noStream, "no-stream", false, "buffer full response before printing")
	fs.BoolVar(&jsonMode, "json", false, "emit output as JSON object")
	fs.StringVar(&agentsDir, "agents-dir", "agents", "directory containing agent markdown files")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "agent run: task required")
		fs.Usage()
		os.Exit(2)
	}

	// If two positional args: first is agent name, second is task.
	// If one positional arg: auto-route.
	var agentName, task string
	if len(rest) >= 2 {
		agentName = rest[0]
		task = strings.Join(rest[1:], " ")
	} else {
		task = rest[0]
	}

	// Load registry FIRST so unknown-agent errors surface BEFORE model
	// client construction (which can fail on missing API key). This matches
	// natural UX: tell the user their agent name is wrong before complaining
	// about API keys.
	reg := orchestrator.NewRegistry()
	if loadErrs := reg.LoadFromDir(agentsDir); len(loadErrs) > 0 {
		for _, e := range loadErrs {
			fmt.Fprintf(os.Stderr, "agent run: warning: %v\n", e)
		}
	}
	rtr := orchestrator.NewRouter(reg)

	// Validate agent name (when explicitly provided) before loading model.
	if agentName != "" {
		if _, lookupErr := reg.Get(agentName); lookupErr != nil {
			fmt.Fprintf(os.Stderr, "agent run: agent %q not found\n", agentName)
			os.Exit(2)
		}
	}

	cfg, err := model.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent run: load config: %v\n", err)
		os.Exit(1)
	}
	if modelID == "" {
		modelID = cfg.DefaultModel
	}

	client, err := model.NewClient(cfg, modelID)
	if err != nil {
		if errors.Is(err, model.ErrNoAPIKey) {
			mc, _ := cfg.FindModel(modelID)
			if mc != nil && mc.APIKeyEnv != "" {
				fmt.Fprintf(os.Stderr, "model: no API key configured (%s): set it in your environment\n", mc.APIKeyEnv)
			} else {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "agent run: %v\n", err)
		os.Exit(1)
	}

	out := os.Stdout
	if noStream {
		out = nil
	}
	disp := orchestrator.NewDispatcher(reg, rtr, client, out)

	var output, chosenAgent string
	if agentName == "" {
		// Auto-route.
		var autoErr error
		chosenAgent, output, autoErr = disp.DispatchAuto(ctx, task)
		if autoErr != nil {
			fmt.Fprintf(os.Stderr, "agent run: %v\n", autoErr)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[agent] auto-selected: %s\n", chosenAgent)
	} else {
		chosenAgent = agentName
		var dispErr error
		output, dispErr = disp.Dispatch(ctx, agentName, task)
		if dispErr != nil {
			fmt.Fprintf(os.Stderr, "agent run: %v\n", dispErr)
			os.Exit(1)
		}
	}

	if noStream {
		if jsonMode {
			b, _ := json.Marshal(map[string]string{"agent": chosenAgent, "output": output})
			fmt.Println(string(b))
		} else {
			fmt.Print(output)
			if len(output) > 0 && output[len(output)-1] != '\n' {
				fmt.Println()
			}
		}
	} else if jsonMode {
		// Streaming already printed deltas; emit final JSON marker.
		b, _ := json.Marshal(map[string]string{"agent": chosenAgent, "done": "true"})
		fmt.Println(string(b))
	}
}

func runAgentMoA(args []string) {
	// Signal handler FIRST.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet("agent moa", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `Usage: clue-code agent moa [flags] <task>

Run a Mixture-of-Agents query: query multiple models in parallel, then
synthesize the results via the synthesis agent.

Flags:`)
		fs.PrintDefaults()
	}

	var (
		modelsFlag string
		synthAgent string
		noStream   bool
		jsonMode   bool
		agentsDir  string
		modelID    string
	)
	fs.StringVar(&modelsFlag, "models", "", "comma-separated model IDs to query in parallel (required)")
	fs.StringVar(&synthAgent, "synthesizer", "critic", "agent name used to synthesize responses")
	fs.BoolVar(&noStream, "no-stream", false, "buffer full response before printing")
	fs.BoolVar(&jsonMode, "json", false, "emit result as JSON object")
	fs.StringVar(&agentsDir, "agents-dir", "agents", "directory containing agent markdown files")
	fs.StringVar(&modelID, "model", "", "model ID for the synthesis agent (default: config default_model)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "agent moa: task required")
		fs.Usage()
		os.Exit(2)
	}
	task := strings.Join(rest, " ")

	if modelsFlag == "" {
		fmt.Fprintln(os.Stderr, "agent moa: --models flag required (comma-separated model IDs)")
		fs.Usage()
		os.Exit(2)
	}
	models := strings.Split(modelsFlag, ",")
	for i, m := range models {
		models[i] = strings.TrimSpace(m)
	}

	cfg, err := model.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent moa: load config: %v\n", err)
		os.Exit(1)
	}
	if modelID == "" {
		modelID = cfg.DefaultModel
	}

	client, err := model.NewClient(cfg, modelID)
	if err != nil {
		if errors.Is(err, model.ErrNoAPIKey) {
			mc, _ := cfg.FindModel(modelID)
			if mc != nil && mc.APIKeyEnv != "" {
				fmt.Fprintf(os.Stderr, "model: no API key configured (%s): set it in your environment\n", mc.APIKeyEnv)
			} else {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "agent moa: %v\n", err)
		os.Exit(1)
	}

	reg := orchestrator.NewRegistry()
	if loadErrs := reg.LoadFromDir(agentsDir); len(loadErrs) > 0 {
		for _, e := range loadErrs {
			fmt.Fprintf(os.Stderr, "agent moa: warning: %v\n", e)
		}
	}
	rtr := orchestrator.NewRouter(reg)

	synthOut := os.Stdout
	if noStream {
		synthOut = nil
	}
	disp := orchestrator.NewDispatcher(reg, rtr, client, synthOut)

	moaCfg := orchestrator.MoAConfig{
		Models:         models,
		SynthesisAgent: synthAgent,
	}

	result, err := disp.MoA(ctx, moaCfg, task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent moa: %v\n", err)
		// Print partial results even on error.
		for mid, resp := range result.Responses {
			fmt.Fprintf(os.Stderr, "[partial] %s: %s\n", mid, resp)
		}
		for mid, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "[error]   %s: %v\n", mid, e)
		}
		os.Exit(1)
	}

	if jsonMode {
		type jsonResult struct {
			Synthesis string            `json:"synthesis"`
			Responses map[string]string `json:"responses"`
		}
		b, _ := json.Marshal(jsonResult{Synthesis: result.Synthesis, Responses: result.Responses})
		fmt.Println(string(b))
	} else if noStream {
		fmt.Print(result.Synthesis)
		if len(result.Synthesis) > 0 && result.Synthesis[len(result.Synthesis)-1] != '\n' {
			fmt.Println()
		}
	}
	// In streaming mode the synthesis chunks already printed via disp.out.
}
