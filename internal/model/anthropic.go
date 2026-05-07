package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/clue-code/clue-code/internal/tokens"
)

const anthropicVersion = "2023-06-01"

// cacheControlSystemThreshold is the token count above which cache_control:ephemeral
// is injected into the Anthropic system prompt block.
const cacheControlSystemThreshold = 1024

func init() {
	RegisterProvider("anthropic", func(mc ModelConfig, apiKey string) (Client, error) {
		endpoint := mc.Endpoint
		if endpoint == "" {
			endpoint = "https://api.anthropic.com/v1/messages"
		} else {
			// Ensure we hit the /messages endpoint when a base URL is provided.
			endpoint = strings.TrimRight(endpoint, "/") + "/messages"
		}
		return &anthropicClient{
			hc:       &http.Client{Timeout: defaultTimeout},
			endpoint: endpoint,
			apiKey:   apiKey,
			modelID:  mc.ID,
		}, nil
	})
}

type anthropicClient struct {
	hc         *http.Client
	endpoint   string
	apiKey     string
	modelID    string
	middleware *Middleware
}

func (c *anthropicClient) Provider() string { return "anthropic" }

// anthropicSystemBlock is a system prompt block, optionally with cache_control.
type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicCacheControl is the cache_control annotation for Anthropic prompt caching.
type anthropicCacheControl struct {
	Type string `json:"type"`
}

// anthropicRequest is the Anthropic Messages API request body.
type anthropicRequest struct {
	Model     string                 `json:"model"`
	System    []anthropicSystemBlock `json:"system,omitempty"`
	Messages  []Message              `json:"messages"`
	MaxTokens int                    `json:"max_tokens"`
	Stream    bool                   `json:"stream,omitempty"`
}

// anthropicResponse is the non-streaming Anthropic Messages API response.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *anthropicClient) buildRequest(ctx context.Context, req ChatRequest, stream bool) (*http.Request, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 8192
	}

	// Separate system messages from conversation messages.
	// Anthropic requires system to be a top-level field (not in messages[]).
	var systemBlocks []anthropicSystemBlock
	var convMsgs []Message
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			block := anthropicSystemBlock{Type: "text", Text: m.Content}
			// Inject cache_control:ephemeral if system prompt exceeds threshold
			// and the token counter middleware is available.
			if c.middleware != nil && c.middleware.Counter != nil {
				n, err := c.middleware.Counter.Count(m.Content, tokens.TokenizerAnthropic)
				if err == nil && n > cacheControlSystemThreshold {
					block.CacheControl = &anthropicCacheControl{Type: "ephemeral"}
				}
			}
			systemBlocks = append(systemBlocks, block)
		} else {
			convMsgs = append(convMsgs, m)
		}
	}

	// Strip provider prefix (e.g. "anthropic/claude-sonnet-4-5" → "claude-sonnet-4-5")
	// so the Anthropic API receives only the bare model name.
	apiModel := req.Model
	if after, ok := strings.CutPrefix(apiModel, "anthropic/"); ok {
		apiModel = after
	}

	body := anthropicRequest{
		Model:     apiModel,
		System:    systemBlocks,
		Messages:  convMsgs,
		MaxTokens: maxTok,
		Stream:    stream,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("model: marshal anthropic request: %w", err)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("model: build anthropic request: %w", err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-api-key", c.apiKey)
	r.Header.Set("anthropic-version", anthropicVersion)
	if stream {
		r.Header.Set("Accept", "text/event-stream")
	}
	return r, nil
}

func (c *anthropicClient) Chat(ctx context.Context, req ChatRequest) (Response, error) {
	httpReq, err := c.buildRequest(ctx, req, false)
	if err != nil {
		return Response{}, err
	}

	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return Response{}, fmt.Errorf("%w: status 429", ErrRateLimit)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	var ar anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return Response{}, fmt.Errorf("model: decode anthropic response: %w", err)
	}
	if ar.Error != nil {
		return Response{}, fmt.Errorf("%w: %s", ErrUpstream, ar.Error.Message)
	}
	var text string
	for _, block := range ar.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	total := ar.Usage.InputTokens + ar.Usage.OutputTokens
	return Response{
		Content: text,
		Usage: Usage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      total,
		},
	}, nil
}

// anthropicSSEEvent covers the subset of Anthropic streaming events we need.
type anthropicSSEEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (c *anthropicClient) ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error) {
	httpReq, err := c.buildRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}

	// Use a no-timeout client for streaming.
	streamHC := &http.Client{}
	resp, err := streamHC.Do(httpReq)
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

		var inputTokens, outputTokens int
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				break
			}

			var ev anthropicSSEEvent
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}

			switch ev.Type {
			case "message_start":
				if ev.Usage != nil {
					inputTokens = ev.Usage.InputTokens
				}
			case "content_block_delta":
				if ev.Delta != nil && ev.Delta.Type == "text_delta" {
					ch <- Chunk{Delta: ev.Delta.Text}
				}
			case "message_delta":
				if ev.Usage != nil {
					outputTokens = ev.Usage.OutputTokens
				}
			case "message_stop":
				total := inputTokens + outputTokens
				ch <- Chunk{
					Done: true,
					Usage: &Usage{
						PromptTokens:     inputTokens,
						CompletionTokens: outputTokens,
						TotalTokens:      total,
					},
				}
				return
			}
		}
		// Fallback terminal chunk if message_stop was not received.
		ch <- Chunk{Done: true}
	}()

	return ch, nil
}
