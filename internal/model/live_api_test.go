//go:build live_api

package model_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveAnthropicAPI sends a minimal request to api.anthropic.com and asserts
// a 200 response. It requires both ANTHROPIC_API_KEY and CLUE_CODE_TEST_LIVE=1
// to be set; otherwise the test is skipped.
//
// This test exists specifically to catch model-ID hallucinations (e.g. a
// model string that doesn't exist on the Anthropic platform) before they reach
// production.
func TestLiveAnthropicAPI(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping live API test")
	}
	if os.Getenv("CLUE_CODE_TEST_LIVE") != "1" {
		t.Skip("CLUE_CODE_TEST_LIVE != 1 — skipping live API test")
	}

	// Use the model ID that the codebase injects when provider=anthropic.
	// This is the canary: if this model ID is wrong, the test fails before merge.
	modelID := "claude-sonnet-4-5"

	reqBody := map[string]any{
		"model":      modelID,
		"max_tokens": 16,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with the single word: ok"},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST api.anthropic.com/v1/messages: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Anthropic API returned status %d for model %q\nBody: %s\n"+
			"HINT: model ID may be invalid or the key may lack access.",
			resp.StatusCode, modelID, body)
	}

	// Parse and validate the response has content.
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse response: %v\nBody: %s", err, body)
	}
	if parsed.Error != nil {
		t.Fatalf("Anthropic API error for model %q: %s", modelID, parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		t.Fatalf("Anthropic API returned empty content for model %q\nBody: %s", modelID, body)
	}
	t.Logf("Anthropic live API OK: model=%s response=%q", modelID, parsed.Content[0].Text)
}

// TestLiveDeepSeekAPI sends a minimal request to api.deepseek.com and asserts
// a 200 response. Requires DEEPSEEK_API_KEY and CLUE_CODE_TEST_LIVE=1.
func TestLiveDeepSeekAPI(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set — skipping live DeepSeek API test")
	}
	if os.Getenv("CLUE_CODE_TEST_LIVE") != "1" {
		t.Skip("CLUE_CODE_TEST_LIVE != 1 — skipping live API test")
	}

	modelID := "deepseek-chat"

	reqBody := map[string]any{
		"model":      modelID,
		"max_tokens": 16,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with the single word: ok"},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.deepseek.com/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST api.deepseek.com: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DeepSeek API returned status %d for model %q\nBody: %s",
			resp.StatusCode, modelID, body)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse response: %v\nBody: %s", err, body)
	}
	if parsed.Error != nil {
		t.Fatalf("DeepSeek API error for model %q: %s", modelID, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		t.Fatalf("DeepSeek API returned no choices for model %q\nBody: %s", modelID, body)
	}
	t.Logf("DeepSeek live API OK: model=%s response=%q", modelID,
		strings.TrimSpace(parsed.Choices[0].Message.Content))
	fmt.Println() // flush for CI log alignment
}
