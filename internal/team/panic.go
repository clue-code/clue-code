package team

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrWorkerPanic is returned by RunWorker when the worker function panicked.
var ErrWorkerPanic = errors.New("team: worker panicked")

// RunWorker executes fn in a panic-safe wrapper. If fn panics:
//   - The owning task (taskID) is marked TaskFailed in the journal.
//   - An "agent-down" envelope is broadcast to all other workers' mailboxes.
//   - ErrWorkerPanic (wrapped with the panic value) is returned.
//
// Panics in fn never cascade to sibling workers or their tasks (D4).
// If fn returns a non-nil error, that error is returned directly.
func RunWorker(team *Team, workerID string, taskID string, fn func() error) (retErr error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		// Mark the owning task as failed.
		_ = team.TaskUpdate(taskID, TaskStatusFailed)

		// Build "agent-down" payload.
		type agentDownPayload struct {
			WorkerID   string `json:"worker_id"`
			TaskID     string `json:"task_id"`
			PanicValue string `json:"panic_value"`
		}
		p := agentDownPayload{
			WorkerID:   workerID,
			TaskID:     taskID,
			PanicValue: fmt.Sprintf("%v", r),
		}
		payloadBytes, err := json.Marshal(p)
		if err != nil {
			retErr = fmt.Errorf("%w: %v (marshal error: %v)", ErrWorkerPanic, r, err)
			return
		}

		// Append agent-down to journal directly (avoid SendMessage calling
		// RecordProgress which could mask the stall signal).
		env := Envelope{
			V:       EnvelopeVersion,
			Seq:     team.seq.Add(1) - 1,
			From:    workerID,
			To:      team.ID,
			Kind:    "agent-down",
			Payload: json.RawMessage(payloadBytes),
			Ts:      nowFunc(),
		}
		_ = team.journal.Append(env)

		// Broadcast to all mailboxes except the panicking worker's own.
		team.mu.RLock()
		mailboxes := make(map[string]chan Message, len(team.mailboxes))
		for k, v := range team.mailboxes {
			mailboxes[k] = v
		}
		team.mu.RUnlock()

		msg := Message{
			Seq:     env.Seq,
			From:    env.From,
			Kind:    env.Kind,
			Payload: env.Payload,
			Ts:      env.Ts,
		}
		for id, ch := range mailboxes {
			if id == workerID {
				continue // don't send to own mailbox
			}
			select {
			case ch <- msg:
			default:
				// mailbox full — drop
			}
		}

		retErr = fmt.Errorf("%w: %v", ErrWorkerPanic, r)
	}()

	return fn()
}
