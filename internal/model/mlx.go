package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func init() {
	RegisterProvider("mlx", func(mc ModelConfig, _ string) (Client, error) {
		if mc.Bin == "" {
			mc.Bin = "/opt/homebrew/bin/mlx_lm.server"
		}
		if !filepath.IsAbs(mc.Bin) {
			return nil, fmt.Errorf("model: mlx bin must be an absolute path, got %q", mc.Bin)
		}
		if _, err := os.Stat(mc.Bin); err != nil {
			return nil, fmt.Errorf("model: mlx bin not found at %q: %w", mc.Bin, err)
		}
		endpoint := mc.Endpoint
		if endpoint == "" {
			endpoint = "http://127.0.0.1:8080/v1/chat/completions"
		}
		return &mlxClient{
			mc:   mc,
			base: newHTTPClient(endpoint, ""),
		}, nil
	})
}

type mlxClient struct {
	mc   ModelConfig
	base *httpClient
}

func (c *mlxClient) Provider() string { return "mlx" }

// startServer launches mlx_lm.server as a subprocess and waits up to 10s for
// it to become ready (health-checked via GET /v1/models). Callers own the
// returned cmd's lifecycle.
//
// Currently unused — wired by the lazy-start hookpoint planned for Phase
// 4.5.x dogfood once we have a real MLX-equipped Mac in the loop. Kept
// here so the contract + Setpgid/WaitDelay/health-check pattern is locked
// in tests today (mlx_test.go) rather than re-litigated later.
//
//nolint:unused // forward-compat: lazy-start hookpoint for Phase 4.5.x
func (c *mlxClient) startServer(ctx context.Context) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, c.mc.Bin, "--model", c.mc.ID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("model: start mlx_lm.server: %w", err)
	}

	base := c.base.endpoint
	if idx := strings.Index(base, "/v1"); idx != -1 {
		base = base[:idx]
	}
	healthURL := strings.TrimRight(base, "/") + "/v1/models"

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return nil, ctx.Err()
		default:
		}
		hc := &http.Client{Timeout: 500 * time.Millisecond}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if resp, err := hc.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return cmd, nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}

	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return nil, fmt.Errorf("model: mlx_lm.server did not become ready within 10s")
}

func (c *mlxClient) Chat(ctx context.Context, req ChatRequest) (Response, error) {
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
		return Response{}, fmt.Errorf("model: mlx decode response: %w", err)
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

func (c *mlxClient) ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error) {
	req.Model = c.mc.ID
	req.Stream = true
	if c.mc.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = c.mc.MaxTokens
	}

	return c.base.postSSE(ctx, req)
}
