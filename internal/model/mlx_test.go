package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mlxStubServer(t *testing.T, streamBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

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
				{"message": map[string]any{"content": "hello from mlx"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     3,
				"completion_tokens": 4,
				"total_tokens":      7,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}

// TestMLXClient_RejectsRelativeBin verifies that a non-absolute Bin path is
// rejected at construction time.
func TestMLXClient_RejectsRelativeBin(t *testing.T) {
	mc := ModelConfig{
		ID:       "mlx/qwen3-7b",
		Provider: "mlx",
		Bin:      "mlx_lm.server", // relative — must be rejected
	}

	ctor := providers["mlx"]
	if ctor == nil {
		t.Fatal("mlx provider not registered; check init()")
	}

	_, err := ctor(mc, "")
	if err == nil {
		t.Fatal("expected error for relative bin path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error should mention absolute path, got: %v", err)
	}
}

// TestMLXClient_RejectsMissingBin verifies that a non-existent absolute Bin is rejected.
func TestMLXClient_RejectsMissingBin(t *testing.T) {
	mc := ModelConfig{
		ID:       "mlx/qwen3-7b",
		Provider: "mlx",
		Bin:      "/nonexistent/path/mlx_lm.server",
	}

	ctor := providers["mlx"]
	if ctor == nil {
		t.Fatal("mlx provider not registered")
	}

	_, err := ctor(mc, "")
	if err == nil {
		t.Fatal("expected error for missing bin, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// TestMLXClient_Chat tests the Chat method against an httptest stub.
// We bypass the constructor validation by constructing mlxClient directly.
func TestMLXClient_Chat(t *testing.T) {
	srv := mlxStubServer(t, "")
	defer srv.Close()

	mc := ModelConfig{
		ID:       "mlx/qwen3-7b",
		Provider: "mlx",
		Bin:      "/opt/homebrew/bin/mlx_lm.server", // not validated in direct construction
	}
	c := &mlxClient{
		mc:   mc,
		base: newHTTPClient(srv.URL+"/v1/chat/completions", ""),
	}

	ctx := context.Background()
	resp, err := c.Chat(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from mlx" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Errorf("unexpected total_tokens: %d", resp.Usage.TotalTokens)
	}
}

// TestMLXClient_ChatStream tests streaming via an httptest stub.
func TestMLXClient_ChatStream(t *testing.T) {
	streamBody := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n" +
		"data: [DONE]\n"

	srv := mlxStubServer(t, streamBody)
	defer srv.Close()

	mc := ModelConfig{
		ID:       "mlx/qwen3-7b",
		Provider: "mlx",
		Bin:      "/opt/homebrew/bin/mlx_lm.server",
	}
	c := &mlxClient{
		mc:   mc,
		base: newHTTPClient(srv.URL+"/v1/chat/completions", ""),
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
	if got := collected.String(); got != "ab" {
		t.Errorf("unexpected stream content: %q", got)
	}
}

// TestMLX_Integration is gated for live mlx_lm.server runs.
func TestMLX_Integration(t *testing.T) {
	t.Skip("integration test: run with -tags=integration and mlx_lm.server running")
}
