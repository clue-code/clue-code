package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentFile(t *testing.T) {
	t.Parallel()

	const content = `---
name: executor
description: Focused task executor
model: qwen3-coder:30b
level: L1
---

This is the prompt body.
`
	a, err := ParseAgentFile(content)
	if err != nil {
		t.Fatalf("ParseAgentFile: unexpected error: %v", err)
	}
	if a.Name != "executor" {
		t.Errorf("Name = %q, want %q", a.Name, "executor")
	}
	if a.Description != "Focused task executor" {
		t.Errorf("Description = %q", a.Description)
	}
	if a.Model != "qwen3-coder:30b" {
		t.Errorf("Model = %q", a.Model)
	}
	if a.Level != "L1" {
		t.Errorf("Level = %q", a.Level)
	}
	if !strings.Contains(a.Prompt, "This is the prompt body.") {
		t.Errorf("Prompt missing body, got %q", a.Prompt)
	}
}

func TestParseAgentFile_MissingName(t *testing.T) {
	t.Parallel()

	const content = `---
description: agent without name
---

body
`
	if _, err := ParseAgentFile(content); err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestParseAgentFile_QuotedValues(t *testing.T) {
	t.Parallel()

	const content = `---
name: "quoted-agent"
description: 'single quoted'
---
`
	a, err := ParseAgentFile(content)
	if err != nil {
		t.Fatalf("ParseAgentFile: %v", err)
	}
	if a.Name != "quoted-agent" {
		t.Errorf("Name = %q", a.Name)
	}
	if a.Description != "single quoted" {
		t.Errorf("Description = %q", a.Description)
	}
}

func TestRegistryLoadFromDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "executor.md"), `---
name: executor
description: Focused executor
model: qwen3-coder:30b
level: L1
---

body
`)
	mustWrite(t, filepath.Join(dir, "verifier.md"), `---
name: verifier
description: Verifies output
model: qwen3-coder:7b
level: L0
---

body
`)
	// A non-markdown file should be skipped without error.
	mustWrite(t, filepath.Join(dir, "README"), "ignored")

	r := NewRegistry()
	if errs := r.LoadFromDir(dir); len(errs) != 0 {
		t.Fatalf("LoadFromDir: unexpected errors: %v", errs)
	}
	if r.Count() != 2 {
		t.Errorf("Count = %d, want 2", r.Count())
	}

	got, err := r.Get("executor")
	if err != nil {
		t.Fatalf("Get(executor): %v", err)
	}
	if got.Level != "L1" {
		t.Errorf("Level = %q", got.Level)
	}

	if _, err := r.Get("nonexistent"); err == nil {
		t.Error("Get(nonexistent) should error")
	}
}

func TestRouterFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "executor.md"), `---
name: executor
description: default
model: qwen3-coder:30b
level: L1
---
body
`)
	r := NewRegistry()
	if errs := r.LoadFromDir(dir); len(errs) != 0 {
		t.Fatalf("LoadFromDir: %v", errs)
	}
	router := NewRouter(r)

	a, err := router.Route("write a small bash script")
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if a.Name != "executor" {
		t.Errorf("Route returned %q, want fallback executor", a.Name)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
