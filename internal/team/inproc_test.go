package team

import (
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

func TestInproc_RoundTrip(t *testing.T) {
	t.Parallel()
	t1, t2 := NewInprocPair()
	defer func() { _ = t1.Close() }()
	defer func() { _ = t2.Close() }()

	sent := Envelope{
		V:       EnvelopeVersion,
		Seq:     42,
		From:    "worker-a",
		To:      "worker-b",
		Kind:    "test-message",
		Payload: json.RawMessage(`{"x":1}`),
		Ts:      time.Now().UTC().Truncate(time.Millisecond),
	}

	done := make(chan Envelope, 1)
	go func() {
		env, err := t2.Recv()
		if err != nil {
			t.Errorf("Recv: %v", err)
			return
		}
		done <- env
	}()

	if err := t1.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-done:
		if got.Seq != sent.Seq {
			t.Errorf("Seq: want %d, got %d", sent.Seq, got.Seq)
		}
		if got.From != sent.From {
			t.Errorf("From: want %q, got %q", sent.From, got.From)
		}
		if got.To != sent.To {
			t.Errorf("To: want %q, got %q", sent.To, got.To)
		}
		if got.Kind != sent.Kind {
			t.Errorf("Kind: want %q, got %q", sent.Kind, got.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Recv")
	}
}

func TestInproc_Concurrent(t *testing.T) {
	t.Parallel()
	ta, tb := NewInprocPair()
	defer func() { _ = ta.Close() }()
	defer func() { _ = tb.Close() }()

	const count = 100
	var wg sync.WaitGroup

	// Sender goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			env := Envelope{
				V:    EnvelopeVersion,
				Seq:  uint64(i),
				From: "sender",
				To:   "receiver",
				Kind: "ping",
				Ts:   time.Now().UTC(),
			}
			if err := ta.Send(env); err != nil {
				t.Errorf("Send %d: %v", i, err)
				return
			}
		}
	}()

	// Receiver goroutine.
	received := make([]Envelope, 0, count)
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			env, err := tb.Recv()
			if err != nil {
				t.Errorf("Recv %d: %v", i, err)
				return
			}
			mu.Lock()
			received = append(received, env)
			mu.Unlock()
		}
	}()

	wg.Wait()

	mu.Lock()
	n := len(received)
	mu.Unlock()

	if n != count {
		t.Errorf("received %d envelopes, want %d", n, count)
	}
}

func TestInproc_Close(t *testing.T) {
	t.Parallel()
	ta, tb := NewInprocPair()

	// Close ta — sends on ta should fail, recv on tb should return EOF.
	if err := ta.Close(); err != nil {
		t.Fatalf("Close ta: %v", err)
	}

	// Recv on tb should return EOF (writer side closed).
	_, err := tb.Recv()
	if err == nil {
		t.Fatal("expected error from Recv after peer closed, got nil")
	}

	// Send on ta after Close should fail.
	sendErr := ta.Send(Envelope{V: EnvelopeVersion})
	if sendErr == nil {
		t.Fatal("expected error from Send after Close, got nil")
	}

	// Close tb for cleanup.
	_ = tb.Close()

	// Second Close on ta must be idempotent.
	if err := ta.Close(); err != nil {
		t.Errorf("second Close ta: %v", err)
	}
}

func TestInproc_Close_RecvEOF(t *testing.T) {
	t.Parallel()
	ta, tb := NewInprocPair()

	// Close the write side of tb so that recv on ta returns EOF/error.
	_ = tb.Close()

	_, err := ta.Recv()
	if err == nil {
		t.Fatal("expected EOF or error, got nil")
	}
	// Either io.EOF or a wrapped error is acceptable.
	_ = err

	_ = ta.Close()
}

// helper to suppress unused import warning if needed.
var _ = io.EOF
