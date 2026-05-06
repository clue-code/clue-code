package tokens

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
)

// ---- Budget tests ----

func TestBudget_BlockOnExceed(t *testing.T) {
	clk := clock.Fake(time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	b, err := NewBudget(5.0, filepath.Join(t.TempDir(), "ledger.jsonl"), clk)
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}

	// Reserve exactly at the limit should succeed.
	if err := b.CheckAndReserve(5.0); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	// Release via commit so we can test the block next.
	b.Commit(5.0)

	// Now spent=5.0; any additional reservation must be blocked.
	if err := b.CheckAndReserve(0.01); err != ErrBudgetExceeded {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestBudget_DailyReset(t *testing.T) {
	start := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	clk := clock.Fake(start)
	b, err := NewBudget(5.0, filepath.Join(t.TempDir(), "ledger.jsonl"), clk)
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}

	// Spend 3.0 USD today.
	if err := b.CheckAndReserve(3.0); err != nil {
		t.Fatalf("CheckAndReserve: %v", err)
	}
	b.Commit(3.0)

	if got := b.SpentToday(); got != 3.0 {
		t.Fatalf("SpentToday before reset: want 3.0, got %v", got)
	}

	// Advance clock by 24 hours (new calendar day).
	clk.Advance(24 * time.Hour)

	// SpentToday should now be 0 after the day rolls over.
	if got := b.SpentToday(); got != 0 {
		t.Fatalf("SpentToday after 24h advance: want 0.0, got %v", got)
	}
}

func TestBudget_ConcurrentReserve(t *testing.T) {
	clk := clock.Fake(time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	b, err := NewBudget(5.0, filepath.Join(t.TempDir(), "ledger.jsonl"), clk)
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}

	const (
		goroutines    = 100
		reservePerReq = 0.06
		// floor(5.0 / 0.06) = 83
		expectedSuccesses = 83
	)

	var successes atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := b.CheckAndReserve(reservePerReq); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	got := int(successes.Load())
	if got != expectedSuccesses {
		t.Fatalf("concurrent reserve: want %d successes, got %d", expectedSuccesses, got)
	}
}

func TestBudget_CommitReconciliation(t *testing.T) {
	clk := clock.Fake(time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	b, err := NewBudget(5.0, filepath.Join(t.TempDir(), "ledger.jsonl"), clk)
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}

	// Reserve 0.10, commit 0.08 (actual < estimated).
	if err := b.CheckAndReserve(0.10); err != nil {
		t.Fatalf("CheckAndReserve: %v", err)
	}
	b.Commit(0.08)

	if got := b.SpentToday(); got != 0.08 {
		t.Fatalf("SpentToday after commit reconciliation: want 0.08, got %v", got)
	}
}

// ---- Analytics tests ----

func TestAnalytics_Summary7Days(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	clk := clock.Fake(now)

	a, err := NewAnalytics(filepath.Join(t.TempDir(), "analytics.jsonl"), clk)
	if err != nil {
		t.Fatalf("NewAnalytics: %v", err)
	}

	// Record 50 entries: 25 anthropic/claude-sonnet-4, 25 deepseek/deepseek-chat.
	u := Usage{InputTokens: 1000, OutputTokens: 500}
	for i := 0; i < 25; i++ {
		a.Record("anthropic", "claude-sonnet-4", u, 0.012)
	}
	for i := 0; i < 25; i++ {
		a.Record("deepseek", "deepseek-chat", u, 0.001)
	}

	report := a.Summary(7 * 24 * time.Hour)

	if report.TotalTokens != 50*(1000+500) {
		t.Fatalf("TotalTokens: want %d, got %d", 50*1500, report.TotalTokens)
	}

	wantAnthropic := 25 * 0.012
	if diff := report.ByProvider["anthropic"] - wantAnthropic; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("ByProvider[anthropic]: want %.6f, got %.6f", wantAnthropic, report.ByProvider["anthropic"])
	}

	wantDeepSeek := 25 * 0.001
	if diff := report.ByProvider["deepseek"] - wantDeepSeek; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("ByProvider[deepseek]: want %.6f, got %.6f", wantDeepSeek, report.ByProvider["deepseek"])
	}

	wantTotal := wantAnthropic + wantDeepSeek
	if diff := report.TotalUSD - wantTotal; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("TotalUSD: want %.6f, got %.6f", wantTotal, report.TotalUSD)
	}
}

func TestAnalytics_Top(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	clk := clock.Fake(now)

	a, err := NewAnalytics(filepath.Join(t.TempDir(), "analytics.jsonl"), clk)
	if err != nil {
		t.Fatalf("NewAnalytics: %v", err)
	}

	u := Usage{InputTokens: 100, OutputTokens: 50}

	// 10 entries with distinct costs.
	entries := []struct {
		provider string
		model    string
		cost     float64
	}{
		{"anthropic", "claude-opus-4", 1.50},
		{"openai", "gpt-4o", 0.80},
		{"anthropic", "claude-sonnet-4", 0.60},
		{"deepseek", "deepseek-chat", 0.05},
		{"openai", "gpt-4o-mini", 0.03},
		{"anthropic", "claude-haiku-4.5", 0.02},
		{"deepseek", "deepseek-r1", 0.10},
		{"openai", "o3", 2.00},
		{"ollama", "llama3", 0.00},
		{"mlx", "phi4", 0.00},
	}

	for _, e := range entries {
		a.Record(e.provider, e.model, u, e.cost)
	}

	top := a.Top(3)
	if len(top) != 3 {
		t.Fatalf("Top(3): want 3 entries, got %d", len(top))
	}

	// Top 3 by cost should be: openai/o3 (2.00), anthropic/claude-opus-4 (1.50), openai/gpt-4o (0.80).
	expectedTop := []struct{ provider, model string }{
		{"openai", "o3"},
		{"anthropic", "claude-opus-4"},
		{"openai", "gpt-4o"},
	}
	for i, want := range expectedTop {
		if top[i].Provider != want.provider || top[i].Model != want.model {
			t.Fatalf("Top[%d]: want %s/%s, got %s/%s",
				i, want.provider, want.model, top[i].Provider, top[i].Model)
		}
	}
}
