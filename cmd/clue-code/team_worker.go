package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clue-code/clue-code/internal/team"
)

// runTeamWorker is the hidden subcommand spawned by SubprocessTransport.
// It reads NDJSON envelopes from stdin, sends ack envelopes to stdout,
// and exits cleanly on EOF or context cancellation.
//
// IMPORTANT: nothing other than NDJSON envelopes must be written to stdout —
// any extraneous output would corrupt the parent's NDJSON parser.
func runTeamWorker(ctx context.Context, args []string) int {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet("team-worker", flag.ContinueOnError)
	fs.SetOutput(os.Stderr) // flag errors go to stderr, never stdout
	teamID := fs.String("team-id", "", "team ID (required)")
	workerID := fs.String("worker-id", "", "worker ID (required)")
	// project-root accepted but not required by worker logic
	_ = fs.String("project-root", "", "project root directory")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "team-worker: flag parse: %v\n", err)
		return 2
	}
	if *teamID == "" || *workerID == "" {
		fmt.Fprintln(os.Stderr, "team-worker: --team-id and --worker-id are required")
		return 2
	}

	scanner := team.NewScanner(os.Stdin)
	var seq uint64

	for {
		// Check for cancellation before each blocking read.
		select {
		case <-ctx.Done():
			return 0
		default:
		}

		env, err := team.DecodeNext(scanner)
		if err != nil {
			if err == io.EOF {
				return 0
			}
			fmt.Fprintf(os.Stderr, "team-worker: recv error: %v\n", err)
			return 1
		}

		// Build ack payload.
		ackPayload, _ := json.Marshal(map[string]interface{}{
			"ack_seq":   env.Seq,
			"worker_id": *workerID,
		})

		ack := team.Envelope{
			V:       team.EnvelopeVersion,
			Seq:     seq,
			From:    *workerID,
			To:      env.From,
			Kind:    "ack",
			Payload: json.RawMessage(ackPayload),
			Ts:      time.Now().UTC(),
		}
		seq++

		if err := team.EncodeEnvelope(os.Stdout, ack); err != nil {
			fmt.Fprintf(os.Stderr, "team-worker: send ack error: %v\n", err)
			return 1
		}
	}
}
