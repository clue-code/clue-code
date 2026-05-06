package orchestrator

import (
	"context"
	"fmt"
	"io"

	"github.com/clue-code/clue-code/internal/model"
)

// Dispatcher wires a Registry and Router to a model.Client for live agent invocations.
type Dispatcher struct {
	registry *Registry
	router   *Router
	client   model.Client
	out      io.Writer
}

// NewDispatcher returns a Dispatcher. out receives streamed chunks as they arrive;
// pass io.Discard to suppress live output.
func NewDispatcher(reg *Registry, rtr *Router, c model.Client, out io.Writer) *Dispatcher {
	return &Dispatcher{registry: reg, router: rtr, client: c, out: out}
}

// Dispatch invokes the named agent with task as the user message.
// It streams chunks to d.out and returns the accumulated full output.
func (d *Dispatcher) Dispatch(ctx context.Context, agentName, task string) (string, error) {
	agent, err := d.registry.Get(agentName)
	if err != nil {
		return "", err
	}

	req := model.ChatRequest{
		Model: agent.Model,
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: agent.Prompt},
			{Role: model.RoleUser, Content: task},
		},
		Stream: true,
	}

	ch, err := d.client.ChatStream(ctx, req)
	if err != nil {
		return "", fmt.Errorf("dispatch %q: stream: %w", agentName, err)
	}

	var buf []byte
	for chunk := range ch {
		if chunk.Delta != "" {
			buf = append(buf, chunk.Delta...)
			if d.out != nil {
				_, _ = io.WriteString(d.out, chunk.Delta)
			}
		}
		if chunk.Done {
			break
		}
		// Check cancellation between chunks.
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}

	// Final cancellation check after draining.
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	return string(buf), nil
}

// DispatchAuto routes task to the best agent via the Router, then dispatches.
// Returns the chosen agent name alongside the output.
func (d *Dispatcher) DispatchAuto(ctx context.Context, task string) (agentName, output string, err error) {
	agent, err := d.router.Route(task)
	if err != nil {
		return "", "", fmt.Errorf("dispatch auto: route: %w", err)
	}
	out, err := d.Dispatch(ctx, agent.Name, task)
	if err != nil {
		return agent.Name, "", err
	}
	return agent.Name, out, nil
}
