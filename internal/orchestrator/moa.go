package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/clue-code/clue-code/internal/model"
)

const (
	moaDefaultTimeout   = 60 * time.Second
	moaSuccessThreshold = 2.0 / 3.0 // ≥2/3 of models must succeed
)

// ErrAllModelsFailed is returned (wrapped) when MoA cannot meet its success
// threshold — i.e. fewer than ⌈2/3⌉ of the configured models produced a
// successful response. Callers should use errors.Is to detect this case
// rather than string-matching on the wrapped error message.
var ErrAllModelsFailed = errors.New("moa: success threshold not met")

// MoAConfig configures a Mixture-of-Agents run.
type MoAConfig struct {
	// Models is the list of model IDs to query in parallel.
	// Must contain at least 1 entry.
	Models []string

	// SynthesisAgent is the name of the agent used to synthesize responses.
	// Defaults to "critic" if empty.
	SynthesisAgent string

	// Timeout caps the parallel phase. Defaults to 60s if zero.
	Timeout time.Duration
}

// MoAResult holds the aggregated output of a MoA run.
type MoAResult struct {
	// Synthesis is the final synthesized output from the SynthesisAgent.
	Synthesis string

	// Responses maps model ID → raw response for each successful model.
	Responses map[string]string

	// Errors maps model ID → error for each failed model.
	Errors map[string]error
}

type moaResponse struct {
	modelID string
	output  string
	err     error
}

// MoA runs task against cfg.Models in parallel, then synthesizes via the
// SynthesisAgent. It returns an error only when fewer than ⌈2/3⌉ models
// succeed or when the synthesis itself fails.
func (d *Dispatcher) MoA(ctx context.Context, cfg MoAConfig, task string) (MoAResult, error) {
	if len(cfg.Models) == 0 {
		return MoAResult{}, fmt.Errorf("moa: at least one model required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = moaDefaultTimeout
	}

	synthAgent := cfg.SynthesisAgent
	if synthAgent == "" {
		synthAgent = "critic"
	}

	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results := make(chan moaResponse, len(cfg.Models))
	var wg sync.WaitGroup

	for _, modelID := range cfg.Models {
		modelID := modelID
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- moaResponse{
						modelID: modelID,
						err:     fmt.Errorf("moa: panic in model %q goroutine: %v", modelID, r),
					}
				}
			}()

			req := model.ChatRequest{
				Model:    modelID,
				Messages: []model.Message{{Role: model.RoleUser, Content: task}},
			}

			resp, err := d.client.Chat(tCtx, req)
			if err != nil {
				results <- moaResponse{modelID: modelID, err: err}
				return
			}
			results <- moaResponse{modelID: modelID, output: resp.Content}
		}()
	}

	// Close results channel once all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	result := MoAResult{
		Responses: make(map[string]string),
		Errors:    make(map[string]error),
	}

	for r := range results {
		if r.err != nil {
			result.Errors[r.modelID] = r.err
		} else {
			result.Responses[r.modelID] = r.output
		}
	}

	total := len(cfg.Models)
	succeeded := len(result.Responses)
	threshold := int(float64(total)*moaSuccessThreshold + 0.999) // ceiling
	if threshold < 1 {
		threshold = 1
	}

	if succeeded < threshold {
		return result, fmt.Errorf("%w: only %d/%d models succeeded (need ≥%d): %v",
			ErrAllModelsFailed, succeeded, total, threshold, collectErrors(result.Errors))
	}

	// Synthesis pass via dedicated agent.
	synthesis, err := d.synthesize(ctx, synthAgent, task, result.Responses)
	if err != nil {
		return result, fmt.Errorf("moa: synthesis failed: %w", err)
	}
	result.Synthesis = synthesis
	return result, nil
}

// synthesize asks the synthesis agent to merge the per-model responses.
func (d *Dispatcher) synthesize(ctx context.Context, agentName, task string, responses map[string]string) (string, error) {
	var sb strings.Builder
	sb.WriteString("You are synthesizing responses from multiple models.\n\n")
	sb.WriteString("Original task:\n")
	sb.WriteString(task)
	sb.WriteString("\n\nModel responses:\n")
	for modelID, resp := range responses {
		sb.WriteString("\n--- ")
		sb.WriteString(modelID)
		sb.WriteString(" ---\n")
		sb.WriteString(resp)
		sb.WriteString("\n")
	}
	sb.WriteString("\nSynthesize the best answer from these responses.")

	return d.Dispatch(ctx, agentName, sb.String())
}

// collectErrors joins all error messages into a single wrapped error.
func collectErrors(errs map[string]error) error {
	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(errs))
	for id, err := range errs {
		msgs = append(msgs, fmt.Sprintf("%s: %v", id, err))
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}
