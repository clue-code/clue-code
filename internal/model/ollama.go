package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() {
	RegisterProvider("ollama", func(mc ModelConfig, _ string) (Client, error) {
		endpoint := mc.Endpoint
		if endpoint == "" {
			endpoint = "http://localhost:11434/v1/chat/completions"
		}
		return &ollamaClient{
			mc:   mc,
			base: newHTTPClient(endpoint, ""),
		}, nil
	})
}

type ollamaClient struct {
	mc   ModelConfig
	base *httpClient
}

func (c *ollamaClient) Provider() string { return "ollama" }

// checkHealth verifies Ollama is reachable via GET /api/tags.
func (c *ollamaClient) checkHealth(ctx context.Context) error {
	// Derive the base URL from the endpoint (strip path after /v1).
	base := c.base.endpoint
	if idx := strings.Index(base, "/v1"); idx != -1 {
		base = base[:idx]
	}
	tagURL := strings.TrimRight(base, "/") + "/api/tags"

	hc := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagURL, nil)
	if err != nil {
		return fmt.Errorf("model: ollama health check: %w", err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("model: ollama not running at %s (start with: ollama serve): %w", base, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model: ollama health check returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ollamaClient) Chat(ctx context.Context, req ChatRequest) (Response, error) {
	if err := c.checkHealth(ctx); err != nil {
		return Response{}, err
	}

	req.Model = c.mc.ID
	req.Stream = false
	if c.mc.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = c.mc.MaxTokens
	}

	data, err := c.base.postJSON(ctx, req)
	if err != nil {
		return Response{}, err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return Response{}, fmt.Errorf("model: ollama decode response: %w", err)
	}

	var content string
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	return Response{
		Content: content,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

func (c *ollamaClient) ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error) {
	if err := c.checkHealth(ctx); err != nil {
		return nil, err
	}

	req.Model = c.mc.ID
	req.Stream = true
	if c.mc.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = c.mc.MaxTokens
	}

	return c.base.postSSE(ctx, req)
}
