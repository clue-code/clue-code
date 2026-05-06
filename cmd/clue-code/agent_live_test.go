//go:build integration

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestAgentRun_Live_Executor tests a real agent run against the DeepSeek API.
//
// Gated by BOTH the `integration` build tag AND the DEEPSEEK_API_KEY env var.
// Run via: go test -tags=integration -run TestAgentRun_Live_Executor ./cmd/clue-code/
//
// CI does not set the integration tag, so live API calls never run on PR
// checks. Local dogfood with `DEEPSEEK_API_KEY=sk-... go test -tags=integration ./...`
// exercises the full path against api.deepseek.com.
func TestAgentRun_Live_Executor(t *testing.T) {
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set; skipping live integration test")
	}
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	bin := buildBinary(t)
	agentsDir := findAgentsDir(t)

	cmd := exec.Command(bin, "agent", "run",
		"--agents-dir", agentsDir,
		"--no-stream",
		"executor",
		"Return a one-line Go function that adds two integers.",
	)
	cmd.Env = os.Environ()

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("agent run (live): exit error after %v: %v\noutput:\n%s", elapsed, err, out)
	}
	if elapsed > 30*time.Second {
		t.Errorf("live call took %v, want < 30s", elapsed)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected non-empty output from executor agent")
	}
}
