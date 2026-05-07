package team

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
)

const defaultStalledThreshold = 60 * time.Second

// StalledDetector monitors a Team for progress stalls. If no progress is
// recorded for longer than threshold, it broadcasts a "team-event:stalled"
// envelope to every known mailbox and records the event in the journal.
//
// Clock injection is mandatory — callers must supply a clock.Clock so that
// tests can fast-forward time without real sleeps (D8).
type StalledDetector struct {
	team      *Team
	clk       clock.Clock
	threshold time.Duration

	// lastProgress holds Unix nanoseconds of the last RecordProgress call.
	// Stored as int64 for atomic access.
	lastProgress atomic.Int64

	done    chan struct{}
	doneAck chan struct{}

	mu    sync.RWMutex
	state string // "active" | "stalled"
}

// NewStalledDetector creates a StalledDetector for team using clk and the
// given threshold. Call Start to begin monitoring.
func NewStalledDetector(team *Team, clk clock.Clock, threshold time.Duration) *StalledDetector {
	if threshold <= 0 {
		threshold = defaultStalledThreshold
	}
	sd := &StalledDetector{
		team:      team,
		clk:       clk,
		threshold: threshold,
		done:      make(chan struct{}),
		doneAck:   make(chan struct{}),
		state:     "active",
	}
	// Initialise lastProgress to now so fresh teams don't immediately stall.
	sd.lastProgress.Store(clk.Now().UnixNano())
	return sd
}

// ArmFromJournal sets lastProgress to the maximum Ts seen in envs. This
// prevents a false stall event immediately after a crash-restart (D8 re-arm
// requirement). Call this after replaying the journal in Open().
func (sd *StalledDetector) ArmFromJournal(envs []Envelope) {
	var maxNs int64
	for _, env := range envs {
		ns := env.Ts.UnixNano()
		if ns > maxNs {
			maxNs = ns
		}
	}
	if maxNs > 0 {
		sd.lastProgress.Store(maxNs)
	}
}

// Start spawns the background monitoring goroutine. The goroutine exits when
// Stop is called or ctx is cancelled.
func (sd *StalledDetector) Start(ctx context.Context) {
	tickInterval := sd.threshold / 4
	if tickInterval < 5*time.Second {
		tickInterval = 5 * time.Second
	}
	// For very small thresholds (tests), allow sub-5s intervals.
	if sd.threshold < 5*time.Second {
		tickInterval = sd.threshold / 4
		if tickInterval < time.Millisecond {
			tickInterval = time.Millisecond
		}
	}

	ticker := sd.clk.NewTicker(tickInterval)

	go func() {
		defer close(sd.doneAck)
		defer ticker.Stop()
		defer func() {
			if r := recover(); r != nil {
				// Swallow panics in the detector goroutine — log nothing to
				// avoid introducing a logger dependency here.
				_ = r
			}
		}()

		for {
			select {
			case <-sd.done:
				return
			case <-ctx.Done():
				return
			case _, ok := <-ticker.C():
				if !ok {
					return
				}
				sd.check(sd.clk.Now())
			}
		}
	}()
}

// check evaluates whether the team has stalled at wall-clock time now.
func (sd *StalledDetector) check(now time.Time) {
	lastNs := sd.lastProgress.Load()
	lastTime := time.Unix(0, lastNs)
	age := now.Sub(lastTime)
	if age <= sd.threshold {
		return
	}

	sd.mu.Lock()
	already := sd.state == "stalled"
	if !already {
		sd.state = "stalled"
	}
	sd.mu.Unlock()

	if already {
		return // already broadcast; don't spam
	}

	// Build payload.
	depths := sd.MailboxDepths()
	type stalledPayload struct {
		LastProgressAgeNs int64          `json:"last_progress_age_ns"`
		MailboxDepths     map[string]int `json:"mailbox_depths"`
	}
	p := stalledPayload{
		LastProgressAgeNs: age.Nanoseconds(),
		MailboxDepths:     depths,
	}
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		return
	}

	// Append to journal directly (bypass SendMessage to avoid RecordProgress loop).
	env := Envelope{
		V:       EnvelopeVersion,
		Seq:     sd.team.seq.Add(1) - 1,
		From:    sd.team.ID,
		To:      sd.team.ID,
		Kind:    "team-event:stalled",
		Payload: json.RawMessage(payloadBytes),
		Ts:      now,
	}
	_ = sd.team.journal.Append(env)

	// Broadcast to all mailboxes (best-effort, non-blocking).
	sd.team.mu.RLock()
	mailboxes := make(map[string]chan Message, len(sd.team.mailboxes))
	for k, v := range sd.team.mailboxes {
		mailboxes[k] = v
	}
	sd.team.mu.RUnlock()

	msg := Message{
		Seq:     env.Seq,
		From:    env.From,
		Kind:    env.Kind,
		Payload: env.Payload,
		Ts:      env.Ts,
	}
	for _, ch := range mailboxes {
		select {
		case ch <- msg:
		default:
			// mailbox full — drop
		}
	}
}

// RecordProgress records that meaningful work has occurred. Call this from
// Team.SendMessage and Team.TaskUpdate.
func (sd *StalledDetector) RecordProgress() {
	sd.lastProgress.Store(sd.clk.Now().UnixNano())
	sd.mu.Lock()
	sd.state = "active"
	sd.mu.Unlock()
}

// Stop signals the background goroutine to exit and waits up to 100ms for it
// to acknowledge. Safe to call multiple times.
func (sd *StalledDetector) Stop() {
	// Close done channel idempotently.
	select {
	case <-sd.done:
		// already closed
	default:
		close(sd.done)
	}

	// Wait up to 100ms for the goroutine to exit.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	select {
	case <-sd.doneAck:
	case <-ctx.Done():
	}
}

// State returns the current stall state ("active" or "stalled") and the time
// elapsed since the last recorded progress.
func (sd *StalledDetector) State() (string, time.Duration) {
	lastNs := sd.lastProgress.Load()
	lastTime := time.Unix(0, lastNs)
	age := sd.clk.Now().Sub(lastTime)

	sd.mu.RLock()
	state := sd.state
	sd.mu.RUnlock()

	return state, age
}

// MailboxDepths returns a snapshot of the current message count in each
// worker's mailbox.
func (sd *StalledDetector) MailboxDepths() map[string]int {
	sd.team.mu.RLock()
	defer sd.team.mu.RUnlock()

	out := make(map[string]int, len(sd.team.mailboxes))
	for id, ch := range sd.team.mailboxes {
		out[id] = len(ch)
	}
	return out
}
