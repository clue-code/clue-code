package state

import (
	"strings"
	"testing"
)

func TestScopeString(t *testing.T) {
	tests := []struct {
		scope Scope
		want  string
	}{
		{ScopeGlobal, "global"},
		{ScopeProject, "project"},
		{ScopeSession, "session"},
		{Scope(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.scope.String(); got != tt.want {
			t.Errorf("Scope(%d).String() = %q, want %q", tt.scope, got, tt.want)
		}
	}
}

func TestResolvePath_Global(t *testing.T) {
	path, err := ResolvePath(ScopeGlobal, "foo/bar.json", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(path, ".clue-code/state/foo/bar.json") {
		t.Errorf("global path %q missing expected suffix", path)
	}
}

func TestResolvePath_Project(t *testing.T) {
	path, err := ResolvePath(ScopeProject, "agent.json", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(path, ".clue-code/state/agent.json") {
		t.Errorf("project path %q missing expected suffix", path)
	}
}

func TestResolvePath_Session(t *testing.T) {
	path, err := ResolvePath(ScopeSession, "run.json", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(path, ".clue-code/sessions/abc123/run.json") {
		t.Errorf("session path %q missing expected suffix", path)
	}
}

func TestResolvePath_SessionMissingID(t *testing.T) {
	_, err := ResolvePath(ScopeSession, "run.json", "")
	if err == nil {
		t.Fatal("expected error for empty sessionID, got nil")
	}
}

func TestResolvePath_UnknownScope(t *testing.T) {
	_, err := ResolvePath(Scope(99), "k", "")
	if err == nil {
		t.Fatal("expected error for unknown scope, got nil")
	}
}
