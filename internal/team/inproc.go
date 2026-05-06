package team

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// InprocTransport is an in-process, pipe-backed implementation of Transport.
// It is safe for concurrent use.
type InprocTransport struct {
	in      *io.PipeReader
	out     *io.PipeWriter
	mu      sync.Mutex // serialises writes to out
	scanner *bufio.Scanner
	closed  atomic.Bool
}

// NewInprocPair returns two fully connected Transport instances backed by a
// pair of io.Pipe connections. Sending on t1 delivers to t2.Recv() and vice
// versa (full-duplex).
func NewInprocPair() (Transport, Transport) {
	// pipe1: t1 writes, t2 reads
	r1, w1 := io.Pipe()
	// pipe2: t2 writes, t1 reads
	r2, w2 := io.Pipe()

	t1 := &InprocTransport{
		in:      r2,
		out:     w1,
		scanner: newScanner(r2),
	}
	t2 := &InprocTransport{
		in:      r1,
		out:     w2,
		scanner: newScanner(r1),
	}
	return t1, t2
}

// Send serialises env as a single NDJSON line and writes it to the pipe.
// Concurrent calls are serialised by an internal mutex.
func (t *InprocTransport) Send(env Envelope) error {
	if t.closed.Load() {
		return fmt.Errorf("team: inproc transport: send on closed transport")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := EncodeEnvelope(t.out, env); err != nil {
		return fmt.Errorf("team: inproc send: %w", err)
	}
	return nil
}

// Recv reads the next Envelope from the pipe. It blocks until data arrives or
// the transport is closed.
func (t *InprocTransport) Recv() (Envelope, error) {
	env, err := DecodeNext(t.scanner)
	if err != nil {
		if err == io.EOF {
			return Envelope{}, io.EOF
		}
		return Envelope{}, fmt.Errorf("team: inproc recv: %w", err)
	}
	return env, nil
}

// Close closes both ends of the pipes. Idempotent — subsequent calls are no-ops.
func (t *InprocTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil // already closed
	}
	var errs []error
	if err := t.out.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := t.in.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("team: inproc close: %v", errs)
	}
	return nil
}
