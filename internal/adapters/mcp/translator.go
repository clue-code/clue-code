package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSON-RPC 2.0 standard error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// Sentinel errors for well-known MCP failure modes.
var (
	// ErrServerCrashed is returned when the MCP server process exits unexpectedly.
	ErrServerCrashed = errors.New("mcp: server crashed")

	// ErrToolNotFound is returned when the requested tool does not exist on the server.
	ErrToolNotFound = errors.New("mcp: tool not found")
)

// Error carries an MCP / JSON-RPC 2.0 error payload.
type Error struct {
	Code    int
	Message string
	Data    json.RawMessage
}

func (e Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("mcp error %d: %s (data: %s)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message)
}

// TranslateError maps an MCP Error to a canonical Go error, preserving
// sentinel values for well-known codes.
func TranslateError(mcpErr Error) error {
	switch mcpErr.Code {
	case codeMethodNotFound:
		return fmt.Errorf("%w: %s", ErrToolNotFound, mcpErr.Message)
	case codeParseError:
		return fmt.Errorf("mcp: parse error: %s", mcpErr.Message)
	case codeInvalidRequest:
		return fmt.Errorf("mcp: invalid request: %s", mcpErr.Message)
	case codeInvalidParams:
		return fmt.Errorf("mcp: invalid params: %s", mcpErr.Message)
	case codeInternalError:
		return fmt.Errorf("mcp: internal error: %s", mcpErr.Message)
	default:
		return mcpErr
	}
}
