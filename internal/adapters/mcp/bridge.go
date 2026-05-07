package mcp

import (
	"encoding/json"
)

// OpenAITool represents a tool in OpenAI's tool calling format.
type OpenAITool struct {
	Type     string     `json:"type"`
	Function OpenAIFunc `json:"function"`
}

// OpenAIFunc holds the function specification within an OpenAI tool.
type OpenAIFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// OpenAIToolResult represents a tool call result in OpenAI format.
type OpenAIToolResult struct {
	Role       string `json:"role"`
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Content    string `json:"content"`
}

// ToolsToOpenAI translates a slice of MCP Tool definitions to the OpenAI tool
// calling format used by non-Anthropic models (e.g. DeepSeek, GPT).
func ToolsToOpenAI(tools []Tool) []OpenAITool {
	out := make([]OpenAITool, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			// Provide an empty object schema when the MCP server omits it.
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, OpenAITool{
			Type: "function",
			Function: OpenAIFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

// ResponseToOpenAI wraps a raw MCP tool result in the OpenAI tool message
// format. toolCallID should match the id from the model's tool_calls array.
func ResponseToOpenAI(result json.RawMessage, toolName string, toolCallID string) OpenAIToolResult {
	content := string(result)
	// If the raw result is a JSON object/array, embed as-is; otherwise treat
	// as a plain string already unmarshalled by the caller.
	if len(result) == 0 {
		content = ""
	}
	return OpenAIToolResult{
		Role:       "tool",
		ToolCallID: toolCallID,
		Name:       toolName,
		Content:    content,
	}
}
