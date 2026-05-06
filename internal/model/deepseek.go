package model

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() {
	RegisterProvider("deepseek", func(mc ModelConfig, apiKey string) (Client, error) {
		endpoint := mc.Endpoint
		if endpoint == "" {
			endpoint = "https://api.deepseek.com/v1/chat/completions"
		}
		return &genericOpenAIClient{
			httpClient: newHTTPClient(endpoint, apiKey),
			modelID:    mc.ID,
			providerID: "deepseek",
		}, nil
	})
}

// genericOpenAIClient handles any OpenAI-compatible provider (DeepSeek, Groq, OpenRouter).
type genericOpenAIClient struct {
	*httpClient
	modelID    string
	providerID string
}

func (c *genericOpenAIClient) Provider() string { return c.providerID }

func (c *genericOpenAIClient) Chat(ctx context.Context, req ChatRequest) (Response, error) {
	req.Stream = false
	raw, err := c.postJSON(ctx, req)
	if err != nil {
		return Response{}, err
	}

	var resp openAIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Response{}, fmt.Errorf("model: decode response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Response{}, fmt.Errorf("%w: empty choices in response", ErrUpstream)
	}
	return Response{
		Content: resp.Choices[0].Message.Content,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

func (c *genericOpenAIClient) ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error) {
	req.Stream = true
	return c.postSSE(ctx, req)
}

// openAIResponse is the non-streaming OpenAI-compatible response shape.
type openAIResponse struct {
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
