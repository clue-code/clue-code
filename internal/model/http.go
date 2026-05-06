package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/clue-code/clue-code/internal/tokens"
)

const (
	defaultTimeout = 60 * time.Second
	maxRetries     = 3
)

// retryDelays are the backoff durations between retry attempts (3 attempts = 2 gaps).
var retryDelays = []time.Duration{200 * time.Millisecond, 600 * time.Millisecond, 1800 * time.Millisecond}

// httpClient is the shared base for HTTP-based providers.
type httpClient struct {
	endpoint   string
	apiKey     string
	hc         *http.Client
	middleware *Middleware
}

// newHTTPClient constructs an httpClient with a 60s default timeout.
func newHTTPClient(endpoint, apiKey string) *httpClient {
	return &httpClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		hc:       &http.Client{Timeout: defaultTimeout},
	}
}

// withMiddleware attaches optional token-engine middleware to the client.
func (c *httpClient) withMiddleware(mw *Middleware) {
	c.middleware = mw
}

// postJSON sends body as JSON POST to c.endpoint, retrying on 5xx up to maxRetries times.
// Maps 429 to ErrRateLimit and other non-2xx to ErrUpstream. 4xx (except 429) are not retried.
func (c *httpClient) postJSON(ctx context.Context, body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("model: marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryDelays[attempt-1]
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("model: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		resp, err := c.hc.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %w", ErrUpstream, err)
			continue
		}
		// NOTE: body must be closed on every path inside this loop. Using
		// `defer` here would accumulate one open body per retry attempt
		// because defers fire at function return, not loop iteration end.
		// Explicit close on each path prevents fd leaks under retry storms.

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("%w: status 429", ErrRateLimit)
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			// 4xx (except 429) — not retried
			_ = resp.Body.Close()
			return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
		}
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("model: read response: %w", err)
		}
		return data, nil
	}

	return nil, lastErr
}

// postJSONWithMiddleware wraps postJSON with optional token-engine hooks:
//   - Cache: return cached payload on hit (no HTTP roundtrip)
//   - Budget: reserve estimated cost before POST; commit actual cost after
//   - Analytics: record actual usage after POST
//
// provider/model/cacheKey are used only when middleware is non-nil.
// If middleware is nil this is identical to postJSON.
func (c *httpClient) postJSONWithMiddleware(
	ctx context.Context,
	body any,
	provider, model, cacheKey string,
	estimatedTokens int,
) ([]byte, error) {
	mw := c.middleware
	if mw == nil {
		return c.postJSON(ctx, body)
	}

	// --- Cache check ---
	if mw.Cache != nil && cacheKey != "" {
		if entry, ok := mw.Cache.Get(cacheKey); ok {
			return entry.Payload, nil
		}
	}

	// --- Budget reservation ---
	var estCostUSD float64
	if mw.Budget != nil && estimatedTokens > 0 {
		estCostUSD = tokens.CostUSD(provider, model, tokens.Usage{InputTokens: estimatedTokens})
		if err := mw.Budget.CheckAndReserve(estCostUSD); err != nil {
			return nil, fmt.Errorf("model: %w", err)
		}
	}

	// --- Actual HTTP call ---
	data, err := c.postJSON(ctx, body)
	if err != nil {
		// Release budget reservation on failure (commit 0).
		if mw.Budget != nil && estCostUSD > 0 {
			mw.Budget.Commit(0)
		}
		return nil, err
	}

	// --- Post-call: commit budget + record analytics ---
	// We rely on the caller to parse actual usage; here we commit estCost as
	// an approximation. Callers that have actual usage should call Commit themselves.
	if mw.Budget != nil && estCostUSD > 0 {
		mw.Budget.Commit(estCostUSD)
	}

	// Store in cache.
	if mw.Cache != nil && cacheKey != "" {
		mw.Cache.Put(cacheKey, tokens.Entry{
			Key:     cacheKey,
			Payload: data,
		})
	}

	return data, nil
}

// postSSE sends body as JSON POST and returns a channel of Chunks parsed from
// the SSE stream. The channel is closed after the Done chunk is sent.
// Lines are expected in `data: {...}` or `data: [DONE]` format.
func (c *httpClient) postSSE(ctx context.Context, body any) (<-chan Chunk, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("model: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("model: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Use a client without the global timeout for streaming.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status 429", ErrRateLimit)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	ch := make(chan Chunk, 64)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				ch <- Chunk{Done: true}
				return
			}

			var event sseEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}

			var delta string
			if len(event.Choices) > 0 {
				delta = event.Choices[0].Delta.Content
			}

			var usage *Usage
			if event.Usage != nil {
				usage = &Usage{
					PromptTokens:     event.Usage.PromptTokens,
					CompletionTokens: event.Usage.CompletionTokens,
					TotalTokens:      event.Usage.TotalTokens,
				}
			}

			ch <- Chunk{Delta: delta, Usage: usage}
		}
		// Stream ended without [DONE]; send terminal chunk.
		ch <- Chunk{Done: true}
	}()

	return ch, nil
}

// sseEvent is the OpenAI-compatible SSE payload shape.
type sseEvent struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}
