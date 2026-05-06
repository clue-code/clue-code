package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

// stubClient is a controllable model.Client for dispatcher tests.
type stubClient struct {
	chunks []model.Chunk
	// blockUntil causes ChatStream to block until this channel is closed.
	blockUntil <-chan struct{}
}

func (s *stubClient) Chat(_ context.Context, _ model.ChatRequest) (model.Response, error) {
	return model.Response{}, nil
}

func (s *stubClient) ChatStream(ctx context.Context, _ model.ChatRequest) (<-chan model.Chunk, error) {
	ch := make(chan model.Chunk, len(s.chunks)+1)
	chunks := s.chunks
	blockUntil := s.blockUntil
	go func() {
		defer close(ch)
		if blockUntil != nil {
			select {
			case <-blockUntil:
			case <-ctx.Done():
				return
			}
		}
		for _, c := range chunks {
			select {
			case ch <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (s *stubClient) Provider() string { return "stub" }

// makeTestRegistry creates a Registry with the given agents written to a temp dir.
func makeTestRegistry(t *testing.T, agents map[string]string) *Registry {
	t.Helper()
	dir := t.TempDir()
	for name, body := range agents {
		mustWrite(t, filepath.Join(dir, name+".md"), body)
	}
	reg := NewRegistry()
	if errs := reg.LoadFromDir(dir); len(errs) != 0 {
		t.Fatalf("LoadFromDir: %v", errs)
	}
	return reg
}

const executorAgentMD = `---
name: executor
description: Focused task executor
model: qwen3-coder:30b
level: L1
---

You are the executor agent.
`

const codeReviewerAgentMD = `---
name: code-reviewer
description: Reviews code
model: qwen3-coder:30b
level: L2
---

You are the code-reviewer agent.
`

func TestDispatcher_Run(t *testing.T) {
	t.Parallel()

	reg := makeTestRegistry(t, map[string]string{"executor": executorAgentMD})
	rtr := NewRouter(reg)
	client := &stubClient{
		chunks: []model.Chunk{
			{Delta: "fix1 "},
			{Delta: "fix2 "},
			{Delta: "fix3", Done: true},
		},
	}

	d := NewDispatcher(reg, rtr, client, io.Discard)
	got, err := d.Dispatch(context.Background(), "executor", "fix this")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	const want = "fix1 fix2 fix3"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDispatcher_AgentNotFound(t *testing.T) {
	t.Parallel()

	reg := makeTestRegistry(t, map[string]string{"executor": executorAgentMD})
	rtr := NewRouter(reg)
	client := &stubClient{}

	d := NewDispatcher(reg, rtr, client, io.Discard)
	_, err := d.Dispatch(context.Background(), "nonexistent", "do something")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("error = %v, want ErrAgentNotFound", err)
	}
}

func TestDispatcher_StreamCancellation(t *testing.T) {
	t.Parallel()

	reg := makeTestRegistry(t, map[string]string{"executor": executorAgentMD})
	rtr := NewRouter(reg)

	// blockUntil is never closed — the stub will block until ctx is cancelled.
	blockCh := make(chan struct{})
	client := &stubClient{blockUntil: blockCh}

	d := NewDispatcher(reg, rtr, client, io.Discard)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := d.Dispatch(ctx, "executor", "long task")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context error", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("cancellation took %v, want < 200ms", elapsed)
	}
}

func TestDispatcher_Auto(t *testing.T) {
	t.Parallel()

	reg := makeTestRegistry(t, map[string]string{
		"executor":      executorAgentMD,
		"code-reviewer": codeReviewerAgentMD,
	})
	rtr := NewRouter(reg)
	client := &stubClient{
		chunks: []model.Chunk{{Delta: "looks good", Done: true}},
	}

	d := NewDispatcher(reg, rtr, client, io.Discard)
	agentName, _, err := d.DispatchAuto(context.Background(), "review this code")
	if err != nil {
		t.Fatalf("DispatchAuto: %v", err)
	}
	if agentName != "code-reviewer" {
		t.Errorf("agentName = %q, want %q", agentName, "code-reviewer")
	}
}

func TestDispatcher_StreamsToOut(t *testing.T) {
	t.Parallel()

	reg := makeTestRegistry(t, map[string]string{"executor": executorAgentMD})
	rtr := NewRouter(reg)
	client := &stubClient{
		chunks: []model.Chunk{
			{Delta: "chunk1"},
			{Delta: "chunk2"},
			{Delta: "chunk3", Done: true},
		},
	}

	var buf bytes.Buffer
	d := NewDispatcher(reg, rtr, client, &buf)
	_, err := d.Dispatch(context.Background(), "executor", "stream test")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"chunk1", "chunk2", "chunk3"} {
		if !containsStr(got, want) {
			t.Errorf("out = %q, missing %q", got, want)
		}
	}

	// Verify order: chunk1 before chunk2 before chunk3.
	i1 := indexOf(got, "chunk1")
	i2 := indexOf(got, "chunk2")
	i3 := indexOf(got, "chunk3")
	if !(i1 < i2 && i2 < i3) {
		t.Errorf("chunks out of order in %q", got)
	}
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOfStr(s, sub) >= 0)
}

func indexOfStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func indexOf(s, sub string) int { return indexOfStr(s, sub) }
