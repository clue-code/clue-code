package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTP_Retry5xx(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable) // 503
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := newHTTPClient(srv.URL, "test-key")
	// Use very short backoff for tests by temporarily overriding retryDelays.
	orig := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	defer func() { retryDelays = orig }()

	body, err := c.postJSON(context.Background(), map[string]string{"test": "val"})
	if err != nil {
		t.Fatalf("postJSON: unexpected error after retries: %v", err)
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
	if len(body) == 0 {
		t.Error("postJSON: empty response body")
	}
}

func TestHTTP_4xxNoRetry(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		status := status
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			attempt := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt++
				w.WriteHeader(status)
			}))
			defer srv.Close()

			c := newHTTPClient(srv.URL, "key")
			_, err := c.postJSON(context.Background(), map[string]string{})
			if err == nil {
				t.Fatal("expected error for 4xx, got nil")
			}
			if !errors.Is(err, ErrUpstream) {
				t.Errorf("expected ErrUpstream, got %v", err)
			}
			if attempt != 1 {
				t.Errorf("4xx should not be retried: got %d attempts", attempt)
			}
		})
	}
}

func TestHTTP_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newHTTPClient(srv.URL, "key")
	_, err := c.postJSON(context.Background(), map[string]string{})
	if !errors.Is(err, ErrRateLimit) {
		t.Errorf("expected ErrRateLimit, got %v", err)
	}
}

func TestSSE_Parse(t *testing.T) {
	// Golden SSE payload: two data chunks then [DONE].
	sseBody := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
		"data: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := newHTTPClient(srv.URL, "key")
	ch, err := c.postSSE(context.Background(), map[string]bool{"stream": true})
	if err != nil {
		t.Fatalf("postSSE: %v", err)
	}

	var chunks []Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Fatal("postSSE: no chunks received")
	}

	// Last chunk must be Done.
	last := chunks[len(chunks)-1]
	if !last.Done {
		t.Errorf("last chunk: Done=false, want true")
	}

	// Collect all deltas.
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Delta)
	}
	got := sb.String()
	if got != "Hello world" {
		t.Errorf("assembled content: got %q, want %q", got, "Hello world")
	}
}
