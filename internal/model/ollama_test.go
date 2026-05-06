package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ollamaStubServer(t *testing.T, streamBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Health check endpoint.
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	})

	// Chat completions endpoint.
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(streamBody))
			return
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello from ollama"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 4,
				"total_tokens":      9,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}

func TestOllamaClient_Chat(t *testing.T) {
	srv := ollamaStubServer(t, "")
	defer srv.Close()

	mc := ModelConfig{
		ID:       "ollama/qwen3-coder:7b",
		Provider: "ollama",
		Endpoint: srv.URL + "/v1/chat/completions",
	}
	c := &ollamaClient{
		mc:   mc,
		base: newHTTPClient(mc.Endpoint, ""),
	}
	// Override health check to point at stub.
	// ollamaClient derives health URL by stripping /v1 from endpoint.

	ctx := context.Background()
	resp, err := c.Chat(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from ollama" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 9 {
		t.Errorf("unexpected total_tokens: %d", resp.Usage.TotalTokens)
	}
}

func TestOllamaClient_ChatStream(t *testing.T) {
	streamBody := "data: {\"choices\":[{\"delta\":{\"content\":\"tok1\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"tok2\"}}]}\n" +
		"data: [DONE]\n"

	srv := ollamaStubServer(t, streamBody)
	defer srv.Close()

	mc := ModelConfig{
		ID:       "ollama/qwen3-coder:7b",
		Provider: "ollama",
		Endpoint: srv.URL + "/v1/chat/completions",
	}
	c := &ollamaClient{
		mc:   mc,
		base: newHTTPClient(mc.Endpoint, ""),
	}

	ctx := context.Background()
	ch, err := c.ChatStream(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var collected strings.Builder
	for chunk := range ch {
		if chunk.Done {
			break
		}
		collected.WriteString(chunk.Delta)
	}
	if got := collected.String(); got != "tok1tok2" {
		t.Errorf("unexpected stream content: %q", got)
	}
}

func TestOllamaClient_HealthCheckFailure(t *testing.T) {
	// No server running — health check should return a clear error.
	mc := ModelConfig{
		ID:       "ollama/qwen3-coder:7b",
		Provider: "ollama",
		Endpoint: "http://127.0.0.1:19999/v1/chat/completions",
	}
	c := &ollamaClient{
		mc:   mc,
		base: newHTTPClient(mc.Endpoint, ""),
	}

	ctx := context.Background()
	_, err := c.Chat(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error when Ollama not running, got nil")
	}
	if !strings.Contains(err.Error(), "ollama") {
		t.Errorf("error should mention ollama, got: %v", err)
	}
}

// TestOllama_Integration is gated by -tags=integration and requires a live Ollama instance.
func TestOllama_Integration(t *testing.T) {
	t.Skip("integration test: run with -tags=integration and ollama serving")
}
