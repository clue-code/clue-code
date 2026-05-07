package mcp

import (
	"encoding/json"
	"testing"
)

func TestToolsToOpenAI(t *testing.T) {
	inputSchema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]string{"type": "string"},
		},
		"required": []string{"path"},
	})

	tools := []Tool{
		{
			Name:        "read_file",
			Description: "Read a file from disk",
			InputSchema: inputSchema,
		},
		{
			Name:        "write_file",
			Description: "Write content to a file",
			InputSchema: nil, // should default to empty object schema
		},
	}

	result := ToolsToOpenAI(tools)

	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}

	// First tool: type, name, description, parameters preserved.
	if result[0].Type != "function" {
		t.Errorf("expected type=function, got %q", result[0].Type)
	}
	if result[0].Function.Name != "read_file" {
		t.Errorf("expected name=read_file, got %q", result[0].Function.Name)
	}
	if result[0].Function.Description != "Read a file from disk" {
		t.Errorf("unexpected description: %q", result[0].Function.Description)
	}

	// Verify parameters are valid JSON.
	var params map[string]any
	if err := json.Unmarshal(result[0].Function.Parameters, &params); err != nil {
		t.Errorf("parameters is not valid JSON: %v", err)
	}
	if params["type"] != "object" {
		t.Errorf("expected type=object in parameters, got %v", params["type"])
	}

	// Second tool: nil inputSchema should become default empty object schema.
	if result[1].Type != "function" {
		t.Errorf("expected type=function for second tool, got %q", result[1].Type)
	}
	var defaultParams map[string]any
	if err := json.Unmarshal(result[1].Function.Parameters, &defaultParams); err != nil {
		t.Errorf("default parameters is not valid JSON: %v", err)
	}
	if defaultParams["type"] != "object" {
		t.Errorf("expected type=object in default parameters, got %v", defaultParams["type"])
	}
}

func TestResponseToOpenAI(t *testing.T) {
	rawResult, _ := json.Marshal(map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": "hello world"},
		},
	})

	result := ResponseToOpenAI(rawResult, "read_file", "call_abc123")

	if result.Role != "tool" {
		t.Errorf("expected role=tool, got %q", result.Role)
	}
	if result.ToolCallID != "call_abc123" {
		t.Errorf("expected tool_call_id=call_abc123, got %q", result.ToolCallID)
	}
	if result.Name != "read_file" {
		t.Errorf("expected name=read_file, got %q", result.Name)
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}

	// Test with empty result.
	emptyResult := ResponseToOpenAI(nil, "tool_name", "call_xyz")
	if emptyResult.Content != "" {
		t.Errorf("expected empty content for nil result, got %q", emptyResult.Content)
	}
}
