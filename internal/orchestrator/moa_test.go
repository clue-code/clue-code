package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

// moaStub is a model.Client for MoA tests that supports per-model responses.
type moaStub struct {
	// responses maps model ID → response content.
	responses map[string]string
	// errModels maps model ID → error to return.
	errModels map[string]error
	// panicModel causes a panic inside the goroutine for this model ID.
	panicModel string
}

func (s *moaStub) Chat(_ context.Context, req model.ChatRequest) (model.Response, error) {
	if s.panicModel != "" && req.Model == s.panicModel {
		panic(fmt.Sprintf("deliberate panic for %s", req.Model))
	}
	if err, ok := s.errModels[req.Model]; ok {
		return model.Response{}, err
	}
	if content, ok := s.responses[req.Model]; ok {
		return model.Response{Content: content}, nil
	}
	return model.Response{}, fmt.Errorf("no stub for model %q", req.Model)
}

func (s *moaStub) ChatStream(_ context.Context, req model.ChatRequest) (<-chan model.Chunk, error) {
	resp, err := s.Chat(context.Background(), req)
	if err != nil {
		return nil, err
	}
	ch := make(chan model.Chunk, 1)
	ch <- model.Chunk{Delta: resp.Content, Done: true}
	close(ch)
	return ch, nil
}

func (s *moaStub) Provider() string { return "moa-stub" }

// buildMoADispatcher creates a Dispatcher with a registry containing a critic agent.
func buildMoADispatcher(t *testing.T, client model.Client) *Dispatcher {
	t.Helper()
	reg := makeTestRegistry(t, map[string]string{
		"critic": `---
name: critic
description: synthesis critic
model: stub-synth
level: L1
---
You are a synthesis critic.
`,
	})
	rtr := NewRouter(reg)
	return NewDispatcher(reg, rtr, client, io.Discard)
}

func TestMoA_AllSuccess(t *testing.T) {
	t.Parallel()

	client := &moaStub{
		responses: map[string]string{
			"model-a":    "Response from A",
			"model-b":    "Response from B",
			"model-c":    "Response from C",
			"stub-synth": "Synthesized answer",
		},
	}
	d := buildMoADispatcher(t, client)

	cfg := MoAConfig{
		Models:         []string{"model-a", "model-b", "model-c"},
		SynthesisAgent: "critic",
		Timeout:        5 * time.Second,
	}

	result, err := d.MoA(context.Background(), cfg, "design a cache")
	if err != nil {
		t.Fatalf("MoA: unexpected error: %v", err)
	}
	if result.Synthesis == "" {
		t.Error("Synthesis should not be empty")
	}
	if len(result.Responses) != 3 {
		t.Errorf("Responses = %d, want 3", len(result.Responses))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want none", result.Errors)
	}
}

func TestMoA_OneFails(t *testing.T) {
	t.Parallel()

	// model-b fails; 2/3 succeed → meets ≥2/3 threshold.
	client := &moaStub{
		responses: map[string]string{
			"model-a":    "Response from A",
			"model-c":    "Response from C",
			"stub-synth": "Synthesized answer",
		},
		errModels: map[string]error{
			"model-b": errors.New("model-b unavailable"),
		},
	}
	d := buildMoADispatcher(t, client)

	cfg := MoAConfig{
		Models:         []string{"model-a", "model-b", "model-c"},
		SynthesisAgent: "critic",
		Timeout:        5 * time.Second,
	}

	result, err := d.MoA(context.Background(), cfg, "design a cache")
	if err != nil {
		t.Fatalf("MoA: unexpected error when 2/3 succeed: %v", err)
	}
	if len(result.Responses) != 2 {
		t.Errorf("Responses = %d, want 2", len(result.Responses))
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
}

func TestMoA_AllFail(t *testing.T) {
	t.Parallel()

	client := &moaStub{
		errModels: map[string]error{
			"model-a": errors.New("down"),
			"model-b": errors.New("down"),
			"model-c": errors.New("down"),
		},
	}
	d := buildMoADispatcher(t, client)

	cfg := MoAConfig{
		Models:  []string{"model-a", "model-b", "model-c"},
		Timeout: 5 * time.Second,
	}

	_, err := d.MoA(context.Background(), cfg, "design a cache")
	if err == nil {
		t.Fatal("MoA: expected error when all models fail, got nil")
	}
	if !strings.Contains(err.Error(), "models succeeded") {
		t.Errorf("error message %q should mention models succeeded count", err.Error())
	}
}

func TestMoA_Timeout(t *testing.T) {
	t.Parallel()

	client := &moaStub{
		errModels: map[string]error{
			"model-a": context.DeadlineExceeded,
			"model-b": context.DeadlineExceeded,
			"model-c": context.DeadlineExceeded,
		},
	}
	d := buildMoADispatcher(t, client)

	cfg := MoAConfig{
		Models:  []string{"model-a", "model-b", "model-c"},
		Timeout: 1 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := d.MoA(ctx, cfg, "design a cache")
	if err == nil {
		t.Fatal("MoA: expected error on timeout, got nil")
	}
}

func TestMoA_SynthesisFromCriticAgent(t *testing.T) {
	t.Parallel()

	const synthOutput = "Final synthesis: use LRU with 2-level hierarchy"
	client := &moaStub{
		responses: map[string]string{
			"model-a":    "Use LRU",
			"model-b":    "Use LFU",
			"stub-synth": synthOutput,
		},
	}
	d := buildMoADispatcher(t, client)

	cfg := MoAConfig{
		Models:         []string{"model-a", "model-b"},
		SynthesisAgent: "critic",
		Timeout:        5 * time.Second,
	}

	result, err := d.MoA(context.Background(), cfg, "design a cache")
	if err != nil {
		t.Fatalf("MoA: unexpected error: %v", err)
	}
	if result.Synthesis != synthOutput {
		t.Errorf("Synthesis = %q, want %q", result.Synthesis, synthOutput)
	}
	if _, ok := result.Responses["model-a"]; !ok {
		t.Error("model-a response missing from result")
	}
	if _, ok := result.Responses["model-b"]; !ok {
		t.Error("model-b response missing from result")
	}
}

func TestMoA_PanicRecovery(t *testing.T) {
	t.Parallel()

	// model-b panics; model-a and model-c succeed → 2/3 threshold met.
	client := &moaStub{
		responses: map[string]string{
			"model-a":    "Response from A",
			"model-c":    "Response from C",
			"stub-synth": "Synthesized answer",
		},
		panicModel: "model-b",
	}
	d := buildMoADispatcher(t, client)

	cfg := MoAConfig{
		Models:         []string{"model-a", "model-b", "model-c"},
		SynthesisAgent: "critic",
		Timeout:        5 * time.Second,
	}

	result, err := d.MoA(context.Background(), cfg, "design a cache")
	if err != nil {
		t.Fatalf("MoA: panic goroutine should be recovered, got error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1 (for the panicking model)", len(result.Errors))
	}
}

func TestMoA_NoModels(t *testing.T) {
	t.Parallel()

	client := &moaStub{}
	d := buildMoADispatcher(t, client)

	_, err := d.MoA(context.Background(), MoAConfig{}, "task")
	if err == nil {
		t.Fatal("expected error for empty models list")
	}
}
