// Package mcp provides a JSON-RPC 2.0 client for Model Context Protocol servers
// spawned as subprocesses communicating over stdin/stdout.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// Tool represents an MCP tool definition returned by tools/list.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// rpcRequest is a JSON-RPC 2.0 request.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is a JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is the JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Client manages a subprocess MCP server over stdin/stdout JSON-RPC 2.0.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	mu      sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan rpcResponse

	closed atomic.Bool
}

// NewClient spawns the MCP server at command with optional args and performs
// the JSON-RPC 2.0 initialization handshake. The subprocess communicates via
// stdin/stdout; stderr is inherited for diagnostics.
func NewClient(ctx context.Context, command string, args ...string) (*Client, error) {
	binPath, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("mcp: binary %q not found: %w", command, err)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("mcp: start server: %w", err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[int64]chan rpcResponse),
	}

	go c.readLoop()

	// Send JSON-RPC initialize request.
	initParams, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "clue-code",
			"version": "1.0",
		},
	})

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Server may not implement initialize — treat as non-fatal.
	// Some minimal MCP servers skip the handshake.
	_, _ = c.call(initCtx, "initialize", initParams)

	return c, nil
}

// ListTools calls the MCP tools/list method and returns the available tools.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if c.closed.Load() {
		return nil, ErrServerCrashed
	}

	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list response: %w", err)
	}
	return resp.Tools, nil
}

// CallTool calls the named MCP tool with the given JSON arguments.
// Returns the raw JSON result content.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, ErrServerCrashed
	}

	params, err := json.Marshal(map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal call params: %w", err)
	}

	result, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Close terminates the MCP server process: sends SIGTERM and waits up to 3
// seconds, then escalates to SIGKILL.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	_ = c.stdin.Close()

	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		_ = c.cmd.Process.Kill()
		return <-done
	}
}

// call sends a JSON-RPC 2.0 request and waits for the matching response.
func (c *Client) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}
	data = append(data, '\n')

	c.mu.Lock()
	_, writeErr := c.stdin.Write(data)
	c.mu.Unlock()

	if writeErr != nil {
		return nil, fmt.Errorf("mcp: write request: %w", writeErr)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			return nil, ErrServerCrashed
		}
		if resp.Error != nil {
			return nil, TranslateError(Error{
				Code:    resp.Error.Code,
				Message: resp.Error.Message,
				Data:    resp.Error.Data,
			})
		}
		return resp.Result, nil
	}
}

// readLoop reads newline-delimited JSON responses from stdout and dispatches
// them to waiting callers via the pending map. It recovers from panics to
// ensure the parent process survives server crashes.
func (c *Client) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			// Panic inside readLoop — mark closed and drain pending channels.
			_ = r
		}
		c.closed.Store(true)
		c.drainPending()
	}()

	scanner := bufio.NewScanner(c.stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Malformed line — skip.
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()

		if ok {
			ch <- resp
		}
	}
	// scanner.Scan() returned false: EOF or error (server exited).
}

// drainPending closes all pending response channels so blocked callers
// receive ErrServerCrashed via the closed channel signal.
func (c *Client) drainPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// errServerCrashedCheck is a sentinel so callers can detect closed channels.
var _ = errors.New // keep errors import used via translator.go
