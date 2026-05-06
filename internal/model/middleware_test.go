package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
