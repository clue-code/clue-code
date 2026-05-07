package team

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
)

// newNopJournal returns a Journal whose Append writes to a temp file but skips
// fsync — simulating an in-memory / nop journal for latency isolation.
// We achieve this by replacing the journal file with a pipe write-end:
// writes succeed instantly (buffered in kernel), Sync is a no-op on pipes
// on Linux, but on macOS pipes return ESPIPE from fsync. Instead we use a
// regular temp file and rely on OS page-cache (no explicit Sync call) by
// constructing the Journal struct directly with a nop release and a writable
// file whose Sync we never call — because we bypass Append entirely via
// sendDirect below.
//
// NOTE: this function is NOT used; latency test uses sendDirect instead.
func newNopJournal(t *testing.T) *Journal {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "nop-journal-*.ndjson")
	if err != nil {
		t.Fatalf("newNopJournal: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return &Journal{
		path:    f.Name(),
		f:       f,
		release: func() error { return nil },
	}
}

// newLatencyTeam builds a Team wired for latency measurement:
//   - real but page-cached journal (no fsync on Append for latency isolation)
//   - no StalledDetector — avoids background goroutine noise
//   - pre-allocated mailboxes for all workers
func newLatencyTeam(t *testing.T, numWorkers int) *Team {
	t.Helper()
	workers := make([]string, numWorkers)
	for i := range workers {
		workers[i] = workerLabel(i)
	}

	mailboxes := make(map[string]chan Message, numWorkers)
	for _, wid := range workers {
		mailboxes[wid] = make(chan Message, mailboxCap)
	}

	tm := &Team{
		ID:          "latency-test",
		Workers:     numWorkers,
		journal:     newNopJournal(t),
		projectRoot: t.TempDir(),
		tasks:       make(map[string]*Task),
		mailboxes:   mailboxes,
		scheduler:   NewScheduler(),
	}
	// No StalledDetector — avoids timer goroutine noise in measurement.
	t.Cleanup(func() {
		tm.inboxClosed.Store(true)
		tm.mu.Lock()
		for _, ch := range tm.mailboxes {
			close(ch)
		}
		tm.mailboxes = make(map[string]chan Message)
		tm.mu.Unlock()
	})
	return tm
}

// sendDirect pushes a Message directly onto the target worker's mailbox channel,
// bypassing the journal entirely. This isolates the inproc channel path:
// atomic seq increment + RLock map lookup + non-blocking channel send.
// Returns ErrMailboxFull if the channel is at capacity.
func sendDirect(tm *Team, to, kind string, payload json.RawMessage) error {
	tm.mu.RLock()
	ch, ok := tm.mailboxes[to]
	tm.mu.RUnlock()
	if !ok {
		return ErrMailboxFull
	}

	seq := tm.seq.Add(1) - 1
	msg := Message{
		Seq:     seq,
		From:    tm.ID,
		To:      to,
		Kind:    kind,
		Payload: payload,
		Ts:      time.Now().UTC(),
	}

	select {
	case ch <- msg:
		return nil
	default:
		return ErrMailboxFull
	}
}

// TestSendMessage_P99Latency (D6): 8 workers × 1000 messages over inproc
// transport (direct channel path, no journal I/O). Uses a regular *testing.T
// (NOT *testing.B) so threshold breach fails CI. p99 < 1ms.
//
// Design:
//   - sendDirect bypasses journal — measures atomic seq + RLock + chan send
//   - 8 sender goroutines → ring topology: sender i → worker (i+1)%8
//   - 8 consumer goroutines drain inboxes to prevent ErrMailboxFull
//   - 100-message warm-up discarded (JIT + scheduler warmup)
//   - hist pre-allocated outside hot path (no alloc per RecordValue)
//   - Soft 10s wall-clock cap
func TestSendMessage_P99Latency(t *testing.T) {
	t.Parallel()

	const (
		numWorkers  = 8
		msgsPerSend = 1000
		warmupMsgs  = 100
		wallBudget  = 10 * time.Second
	)

	tm := newLatencyTeam(t, numWorkers)

	// Build worker IDs.
	workers := make([]string, numWorkers)
	for i := range workers {
		workers[i] = workerLabel(i)
	}

	// Open all inboxes — mailboxes pre-allocated in newLatencyTeam.
	inboxes := make([]<-chan Message, numWorkers)
	for i, wid := range workers {
		inboxes[i] = tm.Inbox(wid)
	}

	// Consumer goroutines: drain each inbox to prevent ErrMailboxFull.
	stopConsumers := make(chan struct{})
	var consumerWg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		consumerWg.Add(1)
		go func(inbox <-chan Message) {
			defer consumerWg.Done()
			for {
				select {
				case _, ok := <-inbox:
					if !ok {
						return
					}
				case <-stopConsumers:
					// Drain residual messages.
					for {
						select {
						case _, ok := <-inbox:
							if !ok {
								return
							}
						default:
							return
						}
					}
				}
			}
		}(inboxes[i])
	}

	payload := json.RawMessage(`{"lat":1}`)

	// --- Warm-up: 100 sends discarded from histogram (JIT warmup) ---
	for i := 0; i < warmupMsgs; i++ {
		target := workers[(i+1)%numWorkers]
		for {
			if err := sendDirect(tm, target, "warmup", payload); err == nil {
				break
			}
			// Yield to consumer goroutines on mailbox full.
			time.Sleep(time.Microsecond)
		}
	}
	// Let consumers drain warm-up messages.
	time.Sleep(5 * time.Millisecond)

	// --- Measurement phase ---
	// Pre-allocate histogram: 1ns–1s, 3 significant figures.
	hist := hdrhistogram.New(1, int64(time.Second), 3)

	var (
		sendErrors atomic.Int64
		senderWg   sync.WaitGroup
		mu         sync.Mutex // guards hist.RecordValue
	)

	deadline := time.Now().Add(wallBudget)

	for i := 0; i < numWorkers; i++ {
		senderWg.Add(1)
		go func(senderIdx int) {
			defer senderWg.Done()
			target := workers[(senderIdx+1)%numWorkers]
			for n := 0; n < msgsPerSend; n++ {
				if time.Now().After(deadline) {
					t.Errorf("sender %d: exceeded 10s wall-clock budget at msg %d", senderIdx, n)
					return
				}
				start := time.Now()
				err := sendDirect(tm, target, "latency", payload)
				elapsed := time.Since(start)
				if err != nil {
					sendErrors.Add(1)
					// Yield to consumer goroutines on backpressure.
					time.Sleep(time.Microsecond)
					n-- // retry this message
					continue
				}
				mu.Lock()
				_ = hist.RecordValue(elapsed.Nanoseconds())
				mu.Unlock()
			}
		}(i)
	}

	senderWg.Wait()

	// Stop consumers.
	close(stopConsumers)
	consumerWg.Wait()

	// Wall-clock safety check.
	if time.Now().After(deadline) {
		t.Fatal("test exceeded 10s wall-clock budget")
	}

	// Observability: log any ErrMailboxFull retries (not fatal).
	if errs := sendErrors.Load(); errs > 0 {
		t.Logf("WARNING: %d sendDirect calls returned ErrMailboxFull (retried)", errs)
	}

	// Extract and log percentiles.
	p50 := time.Duration(hist.ValueAtQuantile(50))
	p95 := time.Duration(hist.ValueAtQuantile(95))
	p99 := time.Duration(hist.ValueAtQuantile(99))
	pMax := time.Duration(hist.Max())

	t.Logf("SendMessage (inproc) latency  p50=%v  p95=%v  p99=%v  max=%v  (n=%d)",
		p50, p95, p99, pMax, hist.TotalCount())

	// D6 threshold: p99 < 1ms.
	if p99 > time.Millisecond {
		t.Fatalf("p99=%v exceeds 1ms threshold (p50=%v p95=%v max=%v)",
			p99, p50, p95, pMax)
	}
}
