package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
	"github.com/clue-code/clue-code/internal/tokens"
)

// fakeAnthropicBody returns a minimal valid Anthropic-style response.
func fakeAnthropicBody(text string) []byte {
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type usageBlock struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	}
	type resp struct {
		Content []contentBlock `json:"content"`
		Usage   usageBlock     `json:"usage"`
	}
	b, _ := json.Marshal(resp{
		Content: []contentBlock{{Type: "text", Text: text}},
		Usage:   usageBlock{InputTokens: 10, OutputTokens: 5},
	})
	return b
}

// TestModelProxy_Middleware_BudgetBlock verifies that a $0 budget blocks every
// request before the upstream is contacted.
func TestModelProxy_Middleware_BudgetBlock(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fakeAnthropicBody("hello"))
	}))
	defer srv.Close()

	clk := clock.Fake(time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	budgetPath := t.TempDir() + "/budget.jsonl"
	bud, err := tokens.NewBudget(0.0, budgetPath, clk)
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}

	hc := newHTTPClient(srv.URL, "")
	hc.middleware = &Middleware{Budget: bud}

	body := map[string]string{"test": "payload"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = hc.postJSONWithMiddleware(ctx, body, "anthropic", "claude-sonnet-4", "key1", 100)
	if err == nil {
		t.Fatal("expected budget error, got nil")
	}
	if hits.Load() != 0 {
		t.Errorf("upstream was hit %d times, want 0 (budget should block before HTTP)", hits.Load())
	}
}

// TestModelProxy_Middleware_CacheHit verifies that a second identical request
// (same cacheKey) returns the cached payload with no second HTTP roundtrip.
func TestModelProxy_Middleware_CacheHit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fakeAnthropicBody("cached"))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	cache, err := tokens.NewCache(100, cacheDir)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	clk := clock.Fake(time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC))
	budgetPath := t.TempDir() + "/budget.jsonl"
	bud, err := tokens.NewBudget(100.0, budgetPath, clk)
	if err != nil {
		t.Fatalf("NewBudget: %v", err)
	}

	hc := newHTTPClient(srv.URL, "")
	hc.middleware = &Middleware{Cache: cache, Budget: bud}

	body := map[string]string{"test": "hit"}
	cacheKey := "deterministic-test-cache-key"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First call — hits upstream.
	data1, err := hc.postJSONWithMiddleware(ctx, body, "anthropic", "claude-sonnet-4", cacheKey, 10)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("after first call: hits = %d, want 1", hits.Load())
	}

	// Second call with same key — must return from cache.
	data2, err := hc.postJSONWithMiddleware(ctx, body, "anthropic", "claude-sonnet-4", cacheKey, 10)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("after second call: hits = %d, want 1 (expected cache hit)", hits.Load())
	}
	if string(data1) != string(data2) {
		t.Errorf("cache returned different payload: first=%q second=%q", data1, data2)
	}
}

// analyticsSpy is a test double for tokens.Analytics that records all calls to Record.
type analyticsSpy struct {
	mu      sync.Mutex
	records []analyticsCall
}

type analyticsCall struct {
	provider string
	model    string
	usage    tokens.Usage
	costUSD  float64
}

func (s *analyticsSpy) Record(provider, model string, u tokens.Usage, costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, analyticsCall{provider: provider, model: model, usage: u, costUSD: costUSD})
}

func (s *analyticsSpy) Summary(_ time.Duration) tokens.Report { return tokens.Report{} }
func (s *analyticsSpy) Top(_ int) []tokens.TopEntry           { return nil }

// TestMiddleware_AnalyticsRecord verifies that postJSONWithMiddleware calls
// Analytics.Record exactly once after a successful POST, with matching provider,
// model, usage, and non-negative cost.
func TestMiddleware_AnalyticsRecord(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fakeAnthropicBody("analytics-test"))
	}))
	defer srv.Close()

	spy := &analyticsSpy{}
	hc := newHTTPClient(srv.URL, "")
	hc.middleware = &Middleware{Analytics: spy}

	const (
		provider        = "anthropic"
		modelID         = "claude-sonnet-4"
		estimatedTokens = 100
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := hc.postJSONWithMiddleware(ctx, map[string]string{"test": "analytics"}, provider, modelID, "", estimatedTokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("upstream hit %d times, want 1", hits.Load())
	}

	spy.mu.Lock()
	defer spy.mu.Unlock()

	if len(spy.records) != 1 {
		t.Fatalf("Analytics.Record called %d times, want 1", len(spy.records))
	}
	rec := spy.records[0]
	if rec.provider != provider {
		t.Errorf("Record.provider = %q, want %q", rec.provider, provider)
	}
	if rec.model != modelID {
		t.Errorf("Record.model = %q, want %q", rec.model, modelID)
	}
	if rec.usage.InputTokens != estimatedTokens {
		t.Errorf("Record.usage.InputTokens = %d, want %d", rec.usage.InputTokens, estimatedTokens)
	}
	if rec.costUSD < 0 {
		t.Errorf("Record.costUSD = %f, want >= 0", rec.costUSD)
	}
}

// TestModelProxy_Middleware_Nil verifies nil middleware is a no-op (postJSON called directly).
func TestModelProxy_Middleware_Nil(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fakeAnthropicBody("nil-mw"))
	}))
	defer srv.Close()

	hc := newHTTPClient(srv.URL, "")
	// middleware is nil by default

	body := map[string]string{"test": "nil-middleware"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := hc.postJSONWithMiddleware(ctx, body, "anthropic", "claude-sonnet-4", "any-key", 0)
	if err != nil {
		t.Fatalf("nil middleware: unexpected error: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("nil middleware: hits = %d, want 1", hits.Load())
	}
}
