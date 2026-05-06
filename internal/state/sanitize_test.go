package state

import (
	"errors"
	"testing"
)

func TestSanitizeKey_State(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantOut string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "simple", input: "notepad", wantOut: "notepad"},
		{name: "namespace", input: "team/abc/worker/1", wantOut: "team/abc/worker/1"},
		{name: "trailing slash cleaned", input: "team/abc/", wantOut: "team/abc"},
		{name: "double slash cleaned", input: "team//abc", wantOut: "team/abc"},
		{name: "current dir cleaned", input: "./notepad", wantOut: "notepad"},
		{name: "with extension", input: "config.json", wantOut: "config.json"},
		{name: "parent traversal", input: "../escape", wantErr: true},
		{name: "deep parent traversal", input: "../../etc/passwd", wantErr: true},
		{name: "embedded parent traversal", input: "team/../etc/passwd", wantErr: true},
		{name: "absolute unix", input: "/etc/passwd", wantErr: true},
		{name: "absolute leading slash", input: "/abs/key", wantErr: true},
		{name: "nul byte", input: "key\x00injection", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := SanitizeKey(c.input)
			if c.wantErr {
				if err == nil {
					t.Errorf("SanitizeKey(%q): want error, got %q", c.input, out)
				}
				if err != nil && !errors.Is(err, ErrInvalidKey) {
					t.Errorf("SanitizeKey(%q): want ErrInvalidKey, got %v", c.input, err)
				}
				return
			}
			if err != nil {
				t.Errorf("SanitizeKey(%q): unexpected error %v", c.input, err)
			}
			if out != c.wantOut {
				t.Errorf("SanitizeKey(%q): got %q, want %q", c.input, out, c.wantOut)
			}
		})
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "sess-abc123", wantErr: false},
		{name: "valid uuid-like", input: "01HQX5KZJ4M2N3P5R7T8V9W1Y2", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "with slash", input: "sess/abc", wantErr: true},
		{name: "with backslash", input: "sess\\abc", wantErr: true},
		{name: "with traversal", input: "..abc", wantErr: true},
		{name: "with nul", input: "sess\x00abc", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := SanitizeIdentifier(c.input)
			if c.wantErr && err == nil {
				t.Errorf("SanitizeIdentifier(%q): want error, got nil", c.input)
			}
			if !c.wantErr && err != nil {
				t.Errorf("SanitizeIdentifier(%q): unexpected error %v", c.input, err)
			}
		})
	}
}
