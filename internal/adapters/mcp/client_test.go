package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// writeFakeServer writes a minimal Go program that acts as a fake MCP server
// and compiles it to a temporary binary. Returns the binary path and a cleanup
// function.
func compileFakeServer(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0600); err != nil {
		t.Fatalf("write fake server src: %v", err)
	}
	bin := filepath.Join(dir, "fakeserver")
	cmd := exec.Command("go", "build", "-o", bin, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile fake server: %v\n%s", err, out)
	}
	return bin
}

const fakeServerListTools = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		var id int64
		_ = json.Unmarshal(req["id"], &id)
		method := ""
		_ = json.Unmarshal(req["method"], &method)

		switch method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]string{"name": "fake", "version": "0.1"},
				},
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "%s\n", data)
		case "tools/list":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "read_file",
							"description": "Read a file from disk",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"path": map[string]string{"type": "string"},
								},
							},
						},
					},
				},
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "%s\n", data)
		}
	}
}
`

const fakeServerCallTool = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		var id int64
		_ = json.Unmarshal(req["id"], &id)
		method := ""
		_ = json.Unmarshal(req["method"], &method)

		switch method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
				},
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "%s\n", data)
		case "tools/call":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "file contents here"},
					},
				},
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "%s\n", data)
		}
	}
}
`

const fakeServerCrash = `package main

import "os"

func main() {
	os.Exit(1)
}
`

func TestClient_ListTools(t *testing.T) {
	bin := compileFakeServer(t, fakeServerListTools)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient(ctx, bin)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "read_file" {
		t.Errorf("expected tool name read_file, got %q", tools[0].Name)
	}
	if tools[0].Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestClient_CallTool(t *testing.T) {
	bin := compileFakeServer(t, fakeServerCallTool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient(ctx, bin)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	args, _ := json.Marshal(map[string]string{"path": "/tmp/test.txt"})
	result, err := client.CallTool(ctx, "read_file", args)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}

	// Verify the result contains expected content.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, ok := parsed["content"]; !ok {
		t.Error("expected content field in result")
	}
}

func TestClient_ServerCrash(t *testing.T) {
	bin := compileFakeServer(t, fakeServerCrash)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// NewClient may succeed (process starts) or fail (exits before initialize).
	// Either way, subsequent calls must return an error — not panic.
	client, newErr := NewClient(ctx, bin)

	if newErr != nil {
		// Server exited during initialization — acceptable outcome.
		t.Logf("NewClient returned error (expected on crash): %v", newErr)
		return
	}
	defer func() { _ = client.Close() }()

	// Give the server time to crash.
	time.Sleep(50 * time.Millisecond)

	_, err := client.ListTools(ctx)
	if err == nil {
		t.Fatal("expected error after server crash, got nil")
	}
	if !errors.Is(err, ErrServerCrashed) {
		t.Logf("got error (not ErrServerCrashed, acceptable): %v", err)
	}
	// Parent process must not have panicked — reaching here proves K3.
}
