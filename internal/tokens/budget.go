// Package tokens — budget.go enforces a daily USD spending cap with an
// atomic reserve/commit ledger persisted as append-only JSONL.
//
// Design:
//   - CheckAndReserve acquires a mutex, checks spent+reserved vs limit, and
//     appends a "reserve" record to the JSONL ledger.
//   - Commit releases the reserved amount and appends a "commit" record.
//   - Daily reset is driven by the injected clock.Clock; no background goroutine.
//   - slog.Warn is emitted on budget exceeded (not fmt.Println).
package tokens

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
)

// ErrBudgetExceeded is returned by CheckAndReserve when the daily limit would
// be exceeded by the requested reservation.
var ErrBudgetExceeded = errors.New("budget_usd_per_day exceeded")

// Budget enforces a daily USD spending cap.
type Budget interface {
	// CheckAndReserve tentatively reserves estCostUSD against today's budget.
	// Returns ErrBudgetExceeded if the reservation would breach the daily limit.
	CheckAndReserve(estCostUSD float64) error

	// Commit reconciles the actual cost after a call completes.
	// actualCostUSD replaces the previously reserved amount.
	Commit(actualCostUSD float64)

	// SpentToday returns the reconciled USD spend for the current calendar day.
	SpentToday() float64

	// Reset clears the in-memory state for today (does not truncate the ledger).
	Reset()
}

// ledgerRecord is one line in the JSONL ledger file.
type ledgerRecord struct {
	TS   time.Time `json:"ts"`
	Type string    `json:"type"` // "reserve" | "commit"
	USD  float64   `json:"usd"`
}

// budget is the concrete implementation of Budget.
type budget struct {
	mu         sync.Mutex
	dailyLimit float64
	spent      float64 // reconciled (committed) spend today
	reserved   float64 // pending reservations not yet committed
	lastReset  string  // YYYY-MM-DD of last reset
	ledgerPath string
	clk        clock.Clock
}

// NewBudget returns a Budget that enforces dailyLimitUSD per calendar day.
// ledgerPath is the path to the JSONL ledger file; if empty,
// os.UserConfigDir()/clue-code/budget-ledger.jsonl is used.
// clk is used for all time operations (injectable for tests).
func NewBudget(dailyLimitUSD float64, ledgerPath string, clk clock.Clock) (Budget, error) {
	if ledgerPath == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("tokens/budget: os.UserConfigDir(): %w", err)
		}
		dir := filepath.Join(base, "clue-code")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("tokens/budget: mkdir %q: %w", dir, err)
		}
		ledgerPath = filepath.Join(dir, "budget-ledger.jsonl")
	} else {
		if err := os.MkdirAll(filepath.Dir(ledgerPath), 0700); err != nil {
			return nil, fmt.Errorf("tokens/budget: mkdir for ledger %q: %w", ledgerPath, err)
		}
	}

	b := &budget{
		dailyLimit: dailyLimitUSD,
		ledgerPath: ledgerPath,
		clk:        clk,
	}

	// Replay today's ledger to restore in-memory state.
	b.lastReset = b.today()
	if err := b.replayLedger(); err != nil {
		// Non-fatal: start fresh if ledger is unreadable.
		slog.Warn("tokens/budget: replay ledger failed, starting fresh", "path", ledgerPath, "err", err)
	}

	return b, nil
}

// today returns the current date as YYYY-MM-DD using the injected clock.
func (b *budget) today() string {
	return b.clk.Now().Format("2006-01-02")
}

// checkDailyReset resets spent/reserved if the calendar day has changed.
// Must be called with b.mu held.
func (b *budget) checkDailyReset() {
	today := b.today()
	if today != b.lastReset {
		b.spent = 0
		b.reserved = 0
		b.lastReset = today
	}
}

// CheckAndReserve reserves estCostUSD against today's budget.
func (b *budget) CheckAndReserve(estCostUSD float64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.checkDailyReset()

	if b.spent+b.reserved+estCostUSD > b.dailyLimit {
		slog.Warn("tokens/budget: budget exceeded",
			"spent_usd", b.spent,
			"reserved_usd", b.reserved,
			"requested_usd", estCostUSD,
			"limit_usd", b.dailyLimit,
		)
		return ErrBudgetExceeded
	}

	b.reserved += estCostUSD
	b.appendLedger(ledgerRecord{TS: b.clk.Now(), Type: "reserve", USD: estCostUSD})
	return nil
}

// Commit reconciles the actual cost, replacing the most-recent reservation.
// If no prior reservation exists (e.g. direct commit without reserve), the
// amount is added directly to spent.
func (b *budget) Commit(actualCostUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.checkDailyReset()

	// Release reserved slot: we assume each Commit matches one CheckAndReserve.
	// If reserved would go negative, clamp to zero.
	if b.reserved >= actualCostUSD {
		b.reserved -= actualCostUSD
	} else {
		b.reserved = 0
	}
	b.spent += actualCostUSD
	b.appendLedger(ledgerRecord{TS: b.clk.Now(), Type: "commit", USD: actualCostUSD})
}

// SpentToday returns the reconciled USD spend for today.
func (b *budget) SpentToday() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkDailyReset()
	return b.spent
}

// Reset clears in-memory state for today without truncating the ledger.
func (b *budget) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spent = 0
	b.reserved = 0
	b.lastReset = b.today()
}

// appendLedger appends one JSON record to the ledger file.
// Errors are logged but not returned to keep the hot path non-blocking.
func (b *budget) appendLedger(r ledgerRecord) {
	data, err := json.Marshal(r)
	if err != nil {
		slog.Warn("tokens/budget: marshal ledger record", "err", err)
		return
	}
	f, err := os.OpenFile(b.ledgerPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		slog.Warn("tokens/budget: open ledger", "path", b.ledgerPath, "err", err)
		return
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		slog.Warn("tokens/budget: write ledger", "path", b.ledgerPath, "err", err)
	}
}

// replayLedger reads the JSONL ledger and rebuilds today's spent/reserved state.
func (b *budget) replayLedger() error {
	f, err := os.Open(b.ledgerPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // first run
		}
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	today := b.today()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r ledgerRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue // skip malformed lines
		}
		if r.TS.Format("2006-01-02") != today {
			continue // only today's records matter
		}
		switch r.Type {
		case "reserve":
			b.reserved += r.USD
		case "commit":
			// On replay we move from reserved to spent.
			if b.reserved >= r.USD {
				b.reserved -= r.USD
			} else {
				b.reserved = 0
			}
			b.spent += r.USD
		}
	}
	return scanner.Err()
}
