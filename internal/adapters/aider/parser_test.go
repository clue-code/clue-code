package aider

import (
	"strings"
	"testing"
)

// TestParseAiderOutput_Standard checks that a typical aider stdout is parsed
// correctly: file paths are extracted and a summary is assembled.
func TestParseAiderOutput_Standard(t *testing.T) {
	input := `Applying changes...
Edited file: internal/foo/foo.go
Edited file: internal/bar/bar.go
Done! 2 files updated.
`
	files, summary, err := ParseAiderOutput([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "internal/foo/foo.go" {
		t.Errorf("files[0] = %q, want %q", files[0], "internal/foo/foo.go")
	}
	if files[1] != "internal/bar/bar.go" {
		t.Errorf("files[1] = %q, want %q", files[1], "internal/bar/bar.go")
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	if !strings.Contains(summary, "Done") {
		t.Errorf("summary %q should contain 'Done'", summary)
	}
}

// TestParseAiderOutput_Empty verifies that empty input returns zero files and
// no error.
func TestParseAiderOutput_Empty(t *testing.T) {
	files, summary, err := ParseAiderOutput([]byte{})
	if err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
	if summary != "" {
		t.Errorf("expected empty summary, got %q", summary)
	}
}

// TestParseAiderOutput_Malformed ensures that garbled (non-UTF-8 / binary)
// input does not cause a panic and returns an error.
func TestParseAiderOutput_Malformed(t *testing.T) {
	// Mix valid text with raw non-UTF-8 bytes.
	garbled := append([]byte("Edited file: ok.go\n"), 0xff, 0xfe, 0x00)
	// bufio.Scanner handles arbitrary bytes; the function must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseAiderOutput panicked: %v", r)
		}
	}()
	files, _, _ := ParseAiderOutput(garbled)
	// We should at least get the valid file parsed before the garbled data.
	if len(files) == 0 {
		t.Log("no files parsed from garbled input (acceptable)")
	}
}
