package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/clue-code/clue-code/internal/team"
)

// runTeam implements the `clue-code team` subcommand.
func runTeam(ctx context.Context, args []string) int {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: clue-code team <list|inspect|tail|demo> [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return teamList(ctx, rest)
	case "inspect":
		return teamInspect(ctx, rest)
	case "tail":
		return teamTail(ctx, rest)
	case "demo":
		return teamDemo(ctx, rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown team subcommand: %q\n", sub)
		fmt.Fprintln(os.Stderr, "usage: clue-code team <list|inspect|tail|demo>")
		return 2
	}
}

// defaultProjectRoot returns the value of CLUE_CODE_PROJECT_ROOT or the
// current working directory if the environment variable is not set.
func defaultProjectRoot() string {
	if v := os.Getenv("CLUE_CODE_PROJECT_ROOT"); v != "" {
		return v
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// teamList prints a table of all teams found under projectRoot.
func teamList(ctx context.Context, args []string) int {
	projectRoot := defaultProjectRoot()
	if len(args) > 0 {
		projectRoot = args[0]
	}

	teamsDir := filepath.Join(projectRoot, ".clue-code", "teams")
	entries, err := os.ReadDir(teamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no teams found")
			return 0
		}
		fmt.Fprintf(os.Stderr, "team list: %v\n", err)
		return 1
	}

	w := csv.NewWriter(os.Stdout)
	w.Comma = '\t'
	_ = w.Write([]string{"ID", "WORKERS", "CREATED_AT"})

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		teamJSON := filepath.Join(teamsDir, e.Name(), "team.json")
		data, err := os.ReadFile(teamJSON)
		if err != nil {
			continue
		}
		var snap struct {
			ID        string    `json:"id"`
			Workers   int       `json:"workers"`
			CreatedAt time.Time `json:"created_at"`
		}
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		_ = w.Write([]string{snap.ID, fmt.Sprintf("%d", snap.Workers), snap.CreatedAt.Format(time.RFC3339)})
	}
	w.Flush()
	return 0
}

// teamInspect prints the state of a team.
func teamInspect(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: clue-code team inspect <team-id> [project-root]")
		return 2
	}
	teamID := args[0]
	projectRoot := defaultProjectRoot()
	if len(args) > 1 {
		projectRoot = args[1]
	}

	t, err := team.Open(teamID, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "team inspect: open: %v\n", err)
		return 1
	}
	defer t.Close()

	tasks := t.TaskList()

	// Count tasks by status.
	counts := map[string]int{}
	for _, tk := range tasks {
		counts[string(tk.Status)]++
	}

	fmt.Printf("Team:    %s\n", t.ID)
	fmt.Printf("Workers: %d\n", t.Workers)
	fmt.Printf("Tasks:   %d total\n", len(tasks))
	for _, s := range []string{"pending", "running", "blocked", "completed", "failed"} {
		if n := counts[s]; n > 0 {
			fmt.Printf("  %-10s %d\n", s, n)
		}
	}
	return 0
}

// teamTail follows a team journal and prints new lines as they arrive.
func teamTail(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: clue-code team tail <team-id> [project-root]")
		return 2
	}
	teamID := args[0]
	projectRoot := defaultProjectRoot()
	if len(args) > 1 {
		projectRoot = args[1]
	}

	journalPath := filepath.Join(projectRoot, ".clue-code", "teams", teamID, "journal.ndjson")
	f, err := os.Open(journalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "team tail: open journal: %v\n", err)
		return 1
	}
	defer f.Close()

	// Seek to end so we only tail new entries.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		fmt.Fprintf(os.Stderr, "team tail: seek: %v\n", err)
		return 1
	}

	scanner := bufio.NewScanner(f)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "team tail: scan: %v\n", err)
			return 1
		}
		select {
		case <-ctx.Done():
			return 0
		case <-ticker.C:
		}
	}
}

// teamDemo runs a demonstration: spawns 2 workers, exchanges 100 messages
// each, and verifies all acks are received.
func teamDemo(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("team demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	transportFlag := fs.String("transport", "inproc", "transport type: inproc or subprocess")
	projectRootFlag := fs.String("project-root", defaultProjectRoot(), "project root directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	transport := *transportFlag
	projectRoot := *projectRootFlag

	const numWorkers = 2
	const msgsPerWorker = 100

	switch transport {
	case "inproc":
		return demoInproc(ctx, numWorkers, msgsPerWorker)
	case "subprocess":
		return demoSubprocess(ctx, projectRoot, numWorkers, msgsPerWorker)
	default:
		fmt.Fprintf(os.Stderr, "team demo: unknown transport %q (use inproc or subprocess)\n", transport)
		return 2
	}
}

// demoInproc runs the demo using in-process pipe transports.
func demoInproc(ctx context.Context, numWorkers, msgsPerWorker int) int {
	type result struct {
		workerID int
		sent     int
		recvd    int
		err      error
	}
	results := make(chan result, numWorkers)

	for i := 0; i < numWorkers; i++ {
		t1, t2 := team.NewInprocPair()
		wID := i
		go func(parentSide, workerSide team.Transport) {
			// Worker goroutine: receive messages, send acks.
			go func() {
				for {
					env, err := workerSide.Recv()
					if err != nil {
						return
					}
					ack := team.Envelope{
						V:    team.EnvelopeVersion,
						Seq:  env.Seq,
						From: fmt.Sprintf("worker-%d", wID),
						To:   "parent",
						Kind: "ack",
						Ts:   time.Now().UTC(),
					}
					if err := workerSide.Send(ack); err != nil {
						return
					}
				}
			}()

			// Parent side: send messages and count acks.
			var sent, recvd int
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for recvd < msgsPerWorker {
					_, err := parentSide.Recv()
					if err != nil {
						return
					}
					recvd++
				}
			}()

			for j := 0; j < msgsPerWorker; j++ {
				env := team.Envelope{
					V:    team.EnvelopeVersion,
					Seq:  uint64(j),
					From: "parent",
					To:   fmt.Sprintf("worker-%d", wID),
					Kind: "ping",
					Ts:   time.Now().UTC(),
				}
				if err := parentSide.Send(env); err != nil {
					results <- result{wID, sent, recvd, err}
					return
				}
				sent++
			}
			wg.Wait()
			_ = parentSide.Close()
			_ = workerSide.Close()
			results <- result{wID, sent, recvd, nil}
		}(t1, t2)
	}

	var totalSent, totalRecvd int
	for i := 0; i < numWorkers; i++ {
		r := <-results
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "demo: worker %d error: %v\n", r.workerID, r.err)
			return 1
		}
		totalSent += r.sent
		totalRecvd += r.recvd
	}

	fmt.Printf("demo (inproc): %d workers, %d sent, %d acks received\n",
		numWorkers, totalSent, totalRecvd)
	if totalSent != numWorkers*msgsPerWorker || totalRecvd != numWorkers*msgsPerWorker {
		fmt.Fprintf(os.Stderr, "demo: message count mismatch\n")
		return 1
	}
	return 0
}

// demoSubprocess runs the demo using subprocess transports.
func demoSubprocess(ctx context.Context, projectRoot string, numWorkers, msgsPerWorker int) int {
	// Create a team so the journal dir exists.
	t, err := team.TeamCreate(team.Spec{
		Workers:     numWorkers,
		ProjectRoot: projectRoot,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo: team create: %v\n", err)
		return 1
	}
	teamID := t.ID

	type result struct {
		workerID string
		sent     int
		recvd    int
		err      error
	}
	results := make(chan result, numWorkers)

	var totalMsgsSent atomic.Int64

	for i := 0; i < numWorkers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)

		// Create a task for this worker in the journal.
		_, _ = t.TaskCreate(team.TaskSpec{Owner: workerID})

		tr, err := team.NewSubprocessTransport(teamID, workerID, projectRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "demo: spawn %s: %v\n", workerID, err)
			_ = t.Close()
			return 1
		}

		go func(wID string, tr team.Transport) {
			defer tr.Close()
			var sent, recvd int

			// Receive acks concurrently.
			ackDone := make(chan error, 1)
			go func() {
				for recvd < msgsPerWorker {
					_, err := tr.Recv()
					if err != nil {
						ackDone <- err
						return
					}
					recvd++
				}
				ackDone <- nil
			}()

			// Send messages.
			for j := 0; j < msgsPerWorker; j++ {
				payload, _ := json.Marshal(map[string]int{"n": j})
				env := team.Envelope{
					V:    team.EnvelopeVersion,
					Seq:  uint64(j),
					From: "parent",
					To:   wID,
					Kind: "ping",
					Payload: json.RawMessage(payload),
					Ts:   time.Now().UTC(),
				}
				if err := tr.Send(env); err != nil {
					results <- result{wID, sent, recvd, err}
					return
				}
				sent++
				totalMsgsSent.Add(1)
			}

			if err := <-ackDone; err != nil {
				results <- result{wID, sent, recvd, err}
				return
			}
			results <- result{wID, sent, recvd, nil}
		}(workerID, tr)
	}

	var totalSent, totalRecvd int
	ok := true
	for i := 0; i < numWorkers; i++ {
		r := <-results
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "demo: %s error: %v\n", r.workerID, r.err)
			ok = false
		}
		totalSent += r.sent
		totalRecvd += r.recvd
	}

	_ = t.Close()

	fmt.Printf("demo (subprocess): %d workers, %d sent, %d acks received\n",
		numWorkers, totalSent, totalRecvd)

	if !ok || totalSent != numWorkers*msgsPerWorker || totalRecvd != numWorkers*msgsPerWorker {
		fmt.Fprintf(os.Stderr, "demo: message count mismatch (sent=%d want=%d, recvd=%d want=%d)\n",
			totalSent, numWorkers*msgsPerWorker, totalRecvd, numWorkers*msgsPerWorker)
		return 1
	}
	return 0
}
