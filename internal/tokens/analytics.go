// Package tokens — analytics.go aggregates per-provider/model usage and cost
// records into a JSONL ledger for reporting and top-N analysis.
package tokens

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
)

// Record captures one completed inference call.
type Record struct {
	Timestamp time.Time `json:"timestamp"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Usage     Usage     `json:"usage"`
	CostUSD   float64   `json:"cost_usd"`
}

// Report summarises usage over a time window.
type Report struct {
	TotalUSD    float64            `json:"total_usd"`
	TotalTokens int                `json:"total_tokens"`
	ByProvider  map[string]float64 `json:"by_provider"`
	ByModel     map[string]float64 `json:"by_model"`
	WindowStart time.Time          `json:"window_start"`
	WindowEnd   time.Time          `json:"window_end"`
}

// TopEntry is one entry in the Top-N ranking.
type TopEntry struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	CostUSD  float64 `json:"cost_usd"`
}

// Analytics records inference calls and produces cost/usage reports.
type Analytics interface {
	// Record appends one call record to the ledger.
	Record(provider, model string, u Usage, costUSD float64)

	// Summary returns an aggregated Report for calls within the last window
	// duration from clk.Now().
	Summary(window time.Duration) Report

	// Top returns the top n provider+model pairs ranked by USD cost within
	// the last 7 days.
	Top(n int) []TopEntry
}

// analytics is the concrete implementation of Analytics.
type analytics struct {
	mu         sync.Mutex
	ledgerPath string
	clk        clock.Clock
}

// NewAnalytics returns an Analytics backed by a JSONL ledger at ledgerPath.
// If ledgerPath is empty, os.UserConfigDir()/clue-code/analytics-ledger.jsonl
// is used.
func NewAnalytics(ledgerPath string, clk clock.Clock) (Analytics, error) {
	if ledgerPath == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("tokens/analytics: os.UserConfigDir(): %w", err)
		}
		dir := filepath.Join(base, "clue-code")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("tokens/analytics: mkdir %q: %w", dir, err)
		}
		ledgerPath = filepath.Join(dir, "analytics-ledger.jsonl")
	} else {
		if err := os.MkdirAll(filepath.Dir(ledgerPath), 0700); err != nil {
			return nil, fmt.Errorf("tokens/analytics: mkdir for ledger %q: %w", ledgerPath, err)
		}
	}

	return &analytics{
		ledgerPath: ledgerPath,
		clk:        clk,
	}, nil
}

// Record appends one call record to the JSONL ledger.
func (a *analytics) Record(provider, model string, u Usage, costUSD float64) {
	r := Record{
		Timestamp: a.clk.Now(),
		Provider:  provider,
		Model:     model,
		Usage:     u,
		CostUSD:   costUSD,
	}

	data, err := json.Marshal(r)
	if err != nil {
		slog.Warn("tokens/analytics: marshal record", "err", err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	f, err := os.OpenFile(a.ledgerPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		slog.Warn("tokens/analytics: open ledger", "path", a.ledgerPath, "err", err)
		return
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		slog.Warn("tokens/analytics: write ledger", "path", a.ledgerPath, "err", err)
	}
}

// Summary reads the ledger and returns aggregated stats for records within
// the last window duration from clk.Now().
func (a *analytics) Summary(window time.Duration) Report {
	now := a.clk.Now()
	windowStart := now.Add(-window)

	records := a.readLedgerSince(windowStart)

	report := Report{
		ByProvider:  make(map[string]float64),
		ByModel:     make(map[string]float64),
		WindowStart: windowStart,
		WindowEnd:   now,
	}

	for _, r := range records {
		report.TotalUSD += r.CostUSD
		report.TotalTokens += r.Usage.InputTokens + r.Usage.OutputTokens +
			r.Usage.CacheReadTokens + r.Usage.CacheWriteTokens
		report.ByProvider[r.Provider] += r.CostUSD
		report.ByModel[r.Model] += r.CostUSD
	}

	return report
}

// Top returns the top n provider+model pairs by USD cost over the last 7 days.
func (a *analytics) Top(n int) []TopEntry {
	if n <= 0 {
		return nil
	}

	now := a.clk.Now()
	since := now.Add(-7 * 24 * time.Hour)
	records := a.readLedgerSince(since)

	// Aggregate by provider+model key.
	type key struct{ provider, model string }
	agg := make(map[key]float64)
	for _, r := range records {
		agg[key{r.Provider, r.Model}] += r.CostUSD
	}

	entries := make([]TopEntry, 0, len(agg))
	for k, cost := range agg {
		entries = append(entries, TopEntry{Provider: k.provider, Model: k.model, CostUSD: cost})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CostUSD > entries[j].CostUSD
	})

	if n > len(entries) {
		n = len(entries)
	}
	return entries[:n]
}

// readLedgerSince reads the JSONL ledger and returns all records at or after
// the given cutoff time. Malformed lines are skipped with a warning.
func (a *analytics) readLedgerSince(since time.Time) []Record {
	a.mu.Lock()
	defer a.mu.Unlock()

	f, err := os.Open(a.ledgerPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("tokens/analytics: open ledger for read", "path", a.ledgerPath, "err", err)
		}
		return nil
	}
	defer f.Close()

	var out []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			slog.Warn("tokens/analytics: unmarshal record", "err", err)
			continue
		}
		if !r.Timestamp.Before(since) {
			out = append(out, r)
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("tokens/analytics: scan ledger", "path", a.ledgerPath, "err", err)
	}
	return out
}
