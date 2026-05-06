package model

import (
	"context"
	"errors"
)

// Sentinel errors returned by all providers.
var (
	ErrNoAPIKey      = errors.New("model: no API key configured")
	ErrModelNotFound = errors.New("model: model id not found")
	ErrRateLimit     = errors.New("model: rate limit")
	ErrUpstream      = errors.New("model: upstream error")
)

// Role identifies who authored a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the provider-agnostic request envelope.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Chunk is one streaming delta from a provider.
// Done is true on the final (possibly empty) chunk.
type Chunk struct {
	Delta string
	Done  bool
	Usage *Usage
}

// Usage holds token consumption for a request.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Response is the complete (non-streaming) result from a provider.
type Response struct {
	Content string
	Usage   Usage
}

// Client is the common interface every provider must implement.
type Client interface {
	// Chat sends req and waits for the complete response.
	Chat(ctx context.Context, req ChatRequest) (Response, error)

	// ChatStream sends req and returns a channel of incremental Chunks.
	// The channel is closed after the Chunk with Done=true is sent.
	// Callers must drain the channel to avoid goroutine leaks.
	ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)

	// Provider returns a short lowercase provider name (e.g. "deepseek").
	Provider() string
}
