package mcp

import (
	"errors"
	"strings"
	"testing"
)

func TestTranslateError_Standard(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		message    string
		wantSentinel error
		wantSubstr string
	}{
		{
			name:         "method not found maps to ErrToolNotFound",
			code:         -32601,
			message:      "method not found",
			wantSentinel: ErrToolNotFound,
		},
		{
			name:       "parse error",
			code:       -32700,
			message:    "parse error",
			wantSubstr: "parse error",
		},
		{
			name:       "invalid request",
			code:       -32600,
			message:    "invalid request",
			wantSubstr: "invalid request",
		},
		{
			name:       "invalid params",
			code:       -32602,
			message:    "invalid params",
			wantSubstr: "invalid params",
		},
		{
			name:       "internal error",
			code:       -32603,
			message:    "internal server error",
			wantSubstr: "internal error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := TranslateError(Error{Code: tc.code, Message: tc.message})
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			if tc.wantSentinel != nil {
				if !errors.Is(err, tc.wantSentinel) {
					t.Errorf("expected errors.Is(%v), got: %v", tc.wantSentinel, err)
				}
			}
			if tc.wantSubstr != "" {
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("expected error to contain %q, got: %v", tc.wantSubstr, err)
				}
			}
		})
	}
}

func TestTranslateError_Generic(t *testing.T) {
	// Non-standard codes should be returned as-is (wrapped Error type).
	err := TranslateError(Error{Code: -32000, Message: "server error: disk full"})
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "-32000") {
		t.Errorf("expected error message to contain code -32000, got: %v", msg)
	}
	if !strings.Contains(msg, "disk full") {
		t.Errorf("expected error message to contain original message, got: %v", msg)
	}

	// Verify it does NOT map to a sentinel.
	if errors.Is(err, ErrToolNotFound) {
		t.Error("generic error should not match ErrToolNotFound")
	}
	if errors.Is(err, ErrServerCrashed) {
		t.Error("generic error should not match ErrServerCrashed")
	}
}
