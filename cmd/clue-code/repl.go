package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/clue-code/clue-code/internal/model"
	"github.com/clue-code/clue-code/internal/version"
	"golang.org/x/term"
)

const replBanner = `CLUE CODE %s — chat interactif
Modele : %s
Tape /help pour les commandes, /exit pour quitter
`

// runREPL starts an interactive REPL session.
// Returns 0 on clean exit, 1 on fatal error.
func runREPL(ctx context.Context) int {
	cfg, err := model.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code: load config: %v\n", err)
		return 1
	}

	modelID := resolveModelID(cfg)
	client, err := model.NewClient(cfg, modelID)
	if err != nil {
		if errors.Is(err, model.ErrNoAPIKey) {
			mc, _ := cfg.FindModel(modelID)
			if mc != nil && mc.APIKeyEnv != "" {
				fmt.Fprintf(os.Stderr, "model: no API key configured (%s): set it in your environment\n", mc.APIKeyEnv)
			} else {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			fmt.Fprintln(os.Stderr, "Lancez 'clue-code setup' pour configurer un modele.")
			return 2
		}
		fmt.Fprintf(os.Stderr, "clue-code: %v\n", err)
		return 1
	}

	// Non-TTY stdin → batch mode: read all stdin, single request, exit.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return runBatchStdin(ctx, client, modelID)
	}

	sess := newReplSession(modelID)
	printBanner(modelID)

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(replPrompt())

		// Gather input, supporting '\' line-continuation.
		line, ok := readLine(scanner)
		if !ok {
			// EOF (Ctrl+D) or scanner error.
			fmt.Println()
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Meta-command dispatch.
		if strings.HasPrefix(line, "/") {
			if shouldExit := handleMetaCommand(sess, line); shouldExit {
				break
			}
			// After /model the client must be refreshed.
			newClient, cerr := model.NewClient(cfg, sess.modelID)
			if cerr == nil {
				client = newClient
			}
			continue
		}

		// Regular user message — append to history and send.
		sess.AppendUser(line)
		assistant, usage, err := sendStreaming(ctx, client, sess.modelID, sess.history)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// Ctrl+C mid-response: return to prompt without exiting.
				fmt.Println("\n[interrupted]")
				// Remove the user turn we just added since the exchange didn't complete.
				sess.history = sess.history[:len(sess.history)-1]
				// Re-arm signal context for the next iteration.
				ctx = rearmContext(ctx)
				continue
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			sess.history = sess.history[:len(sess.history)-1]
			continue
		}

		sess.AppendAssistant(assistant)
		sess.AddUsage(usage)
	}

	fmt.Println("Au revoir!")
	return 0
}

// runBatchStdin processes stdin line-by-line in non-TTY mode.
// Meta-commands (/help, /exit, /clear, /save, /model, /tokens) are handled
// immediately. Non-meta lines are accumulated and sent to the AI as one prompt.
func runBatchStdin(ctx context.Context, client model.Client, modelID string) int {
	sess := newReplSession(modelID)
	scanner := bufio.NewScanner(os.Stdin)
	var promptLines []string

	flushPrompt := func() int {
		if len(promptLines) == 0 {
			return 0
		}
		prompt := strings.TrimSpace(strings.Join(promptLines, "\n"))
		promptLines = promptLines[:0]
		if prompt == "" {
			return 0
		}
		msgs := []model.Message{{Role: model.RoleUser, Content: prompt}}
		assistant, _, err := sendStreaming(ctx, client, modelID, msgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clue-code: %v\n", err)
			return 1
		}
		fmt.Println(assistant)
		return 0
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			// Flush any accumulated prompt first.
			if code := flushPrompt(); code != 0 {
				return code
			}
			if shouldExit := handleMetaCommand(sess, line); shouldExit {
				return 0
			}
			continue
		}
		promptLines = append(promptLines, line)
	}
	return flushPrompt()
}

// handleMetaCommand executes a /command and returns true if the REPL should exit.
func handleMetaCommand(sess *replSession, line string) (exit bool) {
	parts := strings.Fields(line)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.Join(parts[1:], " ")
	}

	switch cmd {
	case "/exit", "/quit":
		return true
	case "/help", "/?":
		sess.Help()
	case "/clear":
		sess.Clear()
	case "/save":
		if err := sess.Save(arg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	case "/model":
		if arg == "" {
			fmt.Fprintf(os.Stderr, "usage: /model <id>\n")
		} else {
			sess.SetModel(arg)
		}
	case "/tokens":
		sess.TokensSummary()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s  (type /help for list)\n", cmd)
	}
	return false
}

// sendStreaming calls ChatStream and prints each token as it arrives.
// Returns the full assistant response and usage.
func sendStreaming(ctx context.Context, client model.Client, _ string, msgs []model.Message) (string, model.Usage, error) {
	req := model.ChatRequest{
		Model:    "",
		Messages: msgs,
	}

	ch, err := client.ChatStream(ctx, req)
	if err != nil {
		return "", model.Usage{}, err
	}

	fmt.Print(assistantPrefix())

	var sb strings.Builder
	var finalUsage model.Usage

	for chunk := range ch {
		if chunk.Done {
			if chunk.Usage != nil {
				finalUsage = *chunk.Usage
			}
			break
		}
		fmt.Print(chunk.Delta)
		sb.WriteString(chunk.Delta)
	}
	fmt.Println()

	if ctx.Err() != nil {
		return sb.String(), finalUsage, ctx.Err()
	}

	return sb.String(), finalUsage, nil
}

// readLine reads a (possibly multi-line via '\' continuation) input from scanner.
// Returns the assembled line and false on EOF.
func readLine(scanner *bufio.Scanner) (string, bool) {
	var sb strings.Builder
	for {
		if !scanner.Scan() {
			if sb.Len() > 0 {
				return sb.String(), true
			}
			return "", false
		}
		text := scanner.Text()
		if strings.HasSuffix(text, `\`) {
			sb.WriteString(text[:len(text)-1])
			sb.WriteByte(' ')
			fmt.Print("... ")
			continue
		}
		sb.WriteString(text)
		return sb.String(), true
	}
}

// printBanner prints the welcome banner.
func printBanner(modelID string) {
	fmt.Printf(replBanner, version.Version, modelID)
}

// replPrompt returns the user prompt string, respecting NO_COLOR.
func replPrompt() string {
	if os.Getenv("NO_COLOR") != "" {
		return "> "
	}
	return "\033[1;32m>\033[0m "
}

// assistantPrefix returns the prefix printed before the assistant response.
func assistantPrefix() string {
	if os.Getenv("NO_COLOR") != "" {
		return "Claude: "
	}
	return "\033[1;34mClaude:\033[0m "
}

// rearmContext returns a fresh signal-notify context derived from Background.
// After a Ctrl+C cancels the previous context, we need a new one for the next
// request without exiting the REPL entirely.
func rearmContext(_ context.Context) context.Context {
	newCtx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	return newCtx
}
