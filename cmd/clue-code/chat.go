package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/clue-code/clue-code/internal/model"
)

const chatUsage = `Usage: clue-code chat [flags] <prompt>

Send a single-turn prompt to the configured model and print the response.

Flags:
  --model <id>     Model ID from config (default: config default_model)
  --no-stream      Buffer the full response before printing (useful for piping)
  --json           Emit each chunk as NDJSON to stdout
  --system <text>  Prepend a system message
  -h, --help       Show this message

The token summary is always written to stderr on completion.

Examples:
  clue-code chat "hello"
  clue-code chat --model anthropic/claude-sonnet-4-6 "explain Go interfaces"
  clue-code chat --no-stream "summarise this" < input.txt
  clue-code chat --json "count to 5" | jq .delta
`

// chunkJSON is the NDJSON shape emitted with --json.
type chunkJSON struct {
	Delta string       `json:"delta,omitempty"`
	Done  bool         `json:"done,omitempty"`
	Usage *model.Usage `json:"usage,omitempty"`
}

func runChat(args []string) {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, chatUsage) }

	var (
		modelID  string
		noStream bool
		jsonMode bool
		system   string
	)
	fs.StringVar(&modelID, "model", "", "model ID (default: config default_model)")
	fs.BoolVar(&noStream, "no-stream", false, "buffer full response before printing")
	fs.BoolVar(&jsonMode, "json", false, "emit chunks as NDJSON")
	fs.StringVar(&system, "system", "", "system message prepended to conversation")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	prompt := fs.Arg(0)
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "clue-code chat: prompt required")
		fmt.Fprint(os.Stderr, chatUsage)
		os.Exit(2)
	}

	cfg, err := model.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code chat: load config: %v\n", err)
		os.Exit(1)
	}

	if modelID == "" {
		modelID = cfg.DefaultModel
	}

	client, err := model.NewClient(cfg, modelID)
	if err != nil {
		if errors.Is(err, model.ErrNoAPIKey) {
			// Extract the env var name for a friendly message.
			mc, _ := cfg.FindModel(modelID)
			envVar := ""
			if mc != nil {
				envVar = mc.APIKeyEnv
			}
			if envVar != "" {
				fmt.Fprintf(os.Stderr, "model: no API key configured (%s): set it in your environment\n", envVar)
			} else {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "clue-code chat: %v\n", err)
		os.Exit(1)
	}

	messages := buildMessages(system, prompt)
	req := model.ChatRequest{
		Model:    modelID,
		Messages: messages,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if noStream {
		resp, err := client.Chat(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clue-code chat: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(resp.Content)
		if len(resp.Content) > 0 && resp.Content[len(resp.Content)-1] != '\n' {
			fmt.Println()
		}
		printTokenSummary(resp.Usage)
		return
	}

	// Streaming path.
	ch, err := client.ChatStream(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code chat: %v\n", err)
		os.Exit(1)
	}

	var finalUsage model.Usage
	for chunk := range ch {
		if chunk.Done {
			if chunk.Usage != nil {
				finalUsage = *chunk.Usage
			}
			if jsonMode {
				emitJSON(chunkJSON{Done: true, Usage: chunk.Usage})
			}
			break
		}
		if jsonMode {
			emitJSON(chunkJSON{Delta: chunk.Delta})
		} else {
			fmt.Print(chunk.Delta)
		}
	}
	if !jsonMode {
		fmt.Println()
	}
	printTokenSummary(finalUsage)
}

func buildMessages(system, prompt string) []model.Message {
	var msgs []model.Message
	if system != "" {
		msgs = append(msgs, model.Message{Role: model.RoleSystem, Content: system})
	}
	msgs = append(msgs, model.Message{Role: model.RoleUser, Content: prompt})
	return msgs
}

func emitJSON(v chunkJSON) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func printTokenSummary(u model.Usage) {
	if u.TotalTokens == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "[tokens] prompt=%d completion=%d total=%d\n",
		u.PromptTokens, u.CompletionTokens, u.TotalTokens)
}
