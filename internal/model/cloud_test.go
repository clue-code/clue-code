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
)

// openAIStubHandler returns a handler that serves a canned OpenAI-compatible
// non-streaming response.
func openAIStubHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: content}},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		})
	}
}

// openAISSEHandler returns a handler that serves a canned OpenAI SSE stream.
func openAISSEHandler(tokens []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range tokens {
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]any{"content": tok}},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// --- DeepSeek ---

func TestDeepSeek_Chat(t *testing.T) {
	srv := httptest.NewServer(openAIStubHandler("hello from deepseek"))
	defer srv.Close()

	mc := ModelConfig{ID: "deepseek-chat", Provider: "deepseek", Endpoint: srv.URL, APIKeyEnv: "DS_KEY"}
	t.Setenv("DS_KEY", "test-key")

	ctor := providers["deepseek"]
	if ctor == nil {
		t.Fatal("deepseek provider not registered")
	}
	client, err := ctor(mc, "test-key")
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	if client.Provider() != "deepseek" {
		t.Errorf("Provider: got %q, want deepseek", client.Provider())
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from deepseek" {
		t.Errorf("Content: got %q, want %q", resp.Content, "hello from deepseek")
	}
	if resp.Usage.TotalTokens != 8 {
		t.Errorf("TotalTokens: got %d, want 8", resp.Usage.TotalTokens)
	}
}

func TestDeepSeek_ChatStream(t *testing.T) {
	srv := httptest.NewServer(openAISSEHandler([]string{"foo", " bar"}))
	defer srv.Close()

	mc := ModelConfig{ID: "deepseek-chat", Provider: "deepseek", Endpoint: srv.URL}
	ctor := providers["deepseek"]
	client, err := ctor(mc, "key")
	if err != nil {
		t.Fatal(err)
	}

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Delta)
	}
	if got := sb.String(); got != "foo bar" {
		t.Errorf("stream content: got %q, want %q", got, "foo bar")
	}
}

func TestDeepSeek_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	mc := ModelConfig{ID: "deepseek-chat", Provider: "deepseek", Endpoint: srv.URL}
	ctor := providers["deepseek"]
	client, _ := ctor(mc, "key")

	_, err := client.Chat(context.Background(), ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, ErrRateLimit) {
		t.Errorf("expected ErrRateLimit, got %v", err)
	}
}

// --- Groq ---

func TestGroq_Chat(t *testing.T) {
	srv := httptest.NewServer(openAIStubHandler("hello from groq"))
	defer srv.Close()

	mc := ModelConfig{ID: "llama3-8b-8192", Provider: "groq", Endpoint: srv.URL}
	ctor := providers["groq"]
	if ctor == nil {
		t.Fatal("groq provider not registered")
	}
	client, err := ctor(mc, "key")
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	if client.Provider() != "groq" {
		t.Errorf("Provider: got %q, want groq", client.Provider())
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "llama3-8b-8192",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from groq" {
		t.Errorf("Content: got %q, want %q", resp.Content, "hello from groq")
	}
}

func TestGroq_ChatStream(t *testing.T) {
	srv := httptest.NewServer(openAISSEHandler([]string{"groq", " stream"}))
	defer srv.Close()

	mc := ModelConfig{ID: "llama3-8b-8192", Provider: "groq", Endpoint: srv.URL}
	ctor := providers["groq"]
	client, _ := ctor(mc, "key")

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "llama3-8b-8192",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Delta)
	}
	if got := sb.String(); got != "groq stream" {
		t.Errorf("stream content: got %q, want %q", got, "groq stream")
	}
}

// --- OpenRouter ---

func TestOpenRouter_Chat(t *testing.T) {
	srv := httptest.NewServer(openAIStubHandler("hello from openrouter"))
	defer srv.Close()

	mc := ModelConfig{ID: "openai/gpt-4o", Provider: "openrouter", Endpoint: srv.URL}
	ctor := providers["openrouter"]
	if ctor == nil {
		t.Fatal("openrouter provider not registered")
	}
	client, err := ctor(mc, "key")
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	if client.Provider() != "openrouter" {
		t.Errorf("Provider: got %q, want openrouter", client.Provider())
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from openrouter" {
		t.Errorf("Content: got %q, want %q", resp.Content, "hello from openrouter")
	}
}

func TestOpenRouter_ChatStream(t *testing.T) {
	srv := httptest.NewServer(openAISSEHandler([]string{"or", " chunk"}))
	defer srv.Close()

	mc := ModelConfig{ID: "openai/gpt-4o", Provider: "openrouter", Endpoint: srv.URL}
	ctor := providers["openrouter"]
	client, _ := ctor(mc, "key")

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Delta)
	}
	if got := sb.String(); got != "or chunk" {
		t.Errorf("stream content: got %q, want %q", got, "or chunk")
	}
}

// --- Anthropic ---

func anthropicNonStreamHandler(text string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify Anthropic-specific headers.
		if r.Header.Get("x-api-key") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(anthropicResponse{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: text},
			},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{InputTokens: 4, OutputTokens: 6},
		})
	}
}

func anthropicSSEHandler(tokens []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writeEvent := func(eventType string, data any) {
			raw, _ := json.Marshal(data)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, raw)
		}

		writeEvent("message_start", map[string]any{
			"type":  "message_start",
			"usage": map[string]any{"input_tokens": 10},
		})
		writeEvent("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": 0,
		})
		for _, tok := range tokens {
			writeEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{"type": "text_delta", "text": tok},
			})
		}
		writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		writeEvent("message_delta", map[string]any{
			"type":  "message_delta",
			"usage": map[string]any{"output_tokens": 7},
		})
		writeEvent("message_stop", map[string]any{"type": "message_stop"})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func TestAnthropic_Chat(t *testing.T) {
	srv := httptest.NewServer(anthropicNonStreamHandler("hello from anthropic"))
	defer srv.Close()

	mc := ModelConfig{
		ID:        "claude-sonnet-4-5",
		Provider:  "anthropic",
		Endpoint:  srv.URL + "/v1",
		APIKeyEnv: "ANTHROPIC_KEY",
	}
	t.Setenv("ANTHROPIC_KEY", "test-key")

	ctor := providers["anthropic"]
	if ctor == nil {
		t.Fatal("anthropic provider not registered")
	}
	client, err := ctor(mc, "test-key")
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	if client.Provider() != "anthropic" {
		t.Errorf("Provider: got %q, want anthropic", client.Provider())
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello from anthropic" {
		t.Errorf("Content: got %q, want %q", resp.Content, "hello from anthropic")
	}
	if resp.Usage.PromptTokens != 4 {
		t.Errorf("PromptTokens: got %d, want 4", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 6 {
		t.Errorf("CompletionTokens: got %d, want 6", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 10 {
		t.Errorf("TotalTokens: got %d, want 10", resp.Usage.TotalTokens)
	}
}

func TestAnthropic_ChatStream(t *testing.T) {
	srv := httptest.NewServer(anthropicSSEHandler([]string{"ant", "hropic"}))
	defer srv.Close()

	mc := ModelConfig{
		ID:       "claude-sonnet-4-5",
		Provider: "anthropic",
		Endpoint: srv.URL + "/v1",
	}
	ctor := providers["anthropic"]
	client, err := ctor(mc, "key")
	if err != nil {
		t.Fatal(err)
	}

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var sb strings.Builder
	var lastChunk Chunk
	for chunk := range ch {
		if chunk.Done {
			lastChunk = chunk
		} else {
			sb.WriteString(chunk.Delta)
		}
	}
	if got := sb.String(); got != "anthropic" {
		t.Errorf("stream content: got %q, want %q", got, "anthropic")
	}
	if !lastChunk.Done {
		t.Error("expected final Done chunk")
	}
	if lastChunk.Usage == nil {
		t.Fatal("expected usage on final chunk")
	}
	if lastChunk.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens: got %d, want 10", lastChunk.Usage.PromptTokens)
	}
	if lastChunk.Usage.CompletionTokens != 7 {
		t.Errorf("CompletionTokens: got %d, want 7", lastChunk.Usage.CompletionTokens)
	}
}

func TestAnthropic_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	mc := ModelConfig{ID: "claude-sonnet-4-5", Provider: "anthropic", Endpoint: srv.URL + "/v1"}
	ctor := providers["anthropic"]
	client, _ := ctor(mc, "key")

	_, err := client.Chat(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, ErrRateLimit) {
		t.Errorf("expected ErrRateLimit, got %v", err)
	}
}

func TestAnthropic_DefaultEndpoint(t *testing.T) {
	ctor := providers["anthropic"]
	mc := ModelConfig{ID: "claude-sonnet-4-5", Provider: "anthropic"}
	client, err := ctor(mc, "key")
	if err != nil {
		t.Fatal(err)
	}
	ac, ok := client.(*anthropicClient)
	if !ok {
		t.Fatal("expected *anthropicClient")
	}
	if !strings.HasSuffix(ac.endpoint, "/messages") {
		t.Errorf("default endpoint should end with /messages, got %q", ac.endpoint)
	}
}
