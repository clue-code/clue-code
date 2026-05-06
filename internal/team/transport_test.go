package team

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestEnvelope_RoundTrip(t *testing.T) {
	t.Parallel()
	orig := Envelope{
		V:       EnvelopeVersion,
		Seq:     42,
		From:    "worker-1",
		To:      "coordinator",
		Kind:    "result",
		Payload: json.RawMessage(`{"answer":42}`),
		Ts:      time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	if err := EncodeEnvelope(&buf, orig); err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	scanner := newScanner(&buf)
	got, err := DecodeNext(scanner)
	if err != nil {
		t.Fatalf("DecodeNext: %v", err)
	}

	if got.V != orig.V || got.Seq != orig.Seq || got.From != orig.From ||
		got.To != orig.To || got.Kind != orig.Kind {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
	if string(got.Payload) != string(orig.Payload) {
		t.Errorf("payload mismatch: got %s, want %s", got.Payload, orig.Payload)
	}
}

func TestEnvelope_UnsupportedVersion(t *testing.T) {
	t.Parallel()
	// Write a raw NDJSON line with v=99.
	line := `{"v":99,"seq":1,"from":"x","to":"y","kind":"test","ts":"2026-05-06T00:00:00Z"}` + "\n"
	scanner := newScanner(strings.NewReader(line))
	_, err := DecodeNext(scanner)
	if err == nil {
		t.Fatal("expected error for v=99, got nil")
	}
	if !isUnsupportedVersionErr(err) {
		t.Errorf("expected ErrUnsupportedEnvelopeVersion, got: %v", err)
	}
}

func TestEnvelope_LargePayload(t *testing.T) {
	t.Parallel()
	// 5 MiB payload
	large := make([]byte, 5*1024*1024)
	for i := range large {
		large[i] = 'a'
	}
	payload := json.RawMessage(`"` + string(large) + `"`)

	env := Envelope{
		V:       EnvelopeVersion,
		Seq:     1,
		From:    "sender",
		To:      "recv",
		Kind:    "data",
		Payload: payload,
		Ts:      time.Now().UTC(),
	}

	var buf bytes.Buffer
	if err := EncodeEnvelope(&buf, env); err != nil {
		t.Fatalf("EncodeEnvelope large: %v", err)
	}

	scanner := newScanner(&buf)
	got, err := DecodeNext(scanner)
	if err != nil {
		t.Fatalf("DecodeNext large: %v", err)
	}
	if len(got.Payload) != len(payload) {
		t.Errorf("payload length mismatch: got %d, want %d", len(got.Payload), len(payload))
	}
}

func TestEnvelope_EmptyLines(t *testing.T) {
	t.Parallel()
	// Two blank lines, then one valid envelope.
	env := Envelope{
		V:    EnvelopeVersion,
		Seq:  7,
		From: "a",
		To:   "b",
		Kind: "ping",
		Ts:   time.Now().UTC(),
	}
	var buf bytes.Buffer
	buf.WriteString("\n\n") // two empty lines
	if err := EncodeEnvelope(&buf, env); err != nil {
		t.Fatalf("encode: %v", err)
	}

	scanner := newScanner(&buf)
	got, err := DecodeNext(scanner)
	if err != nil {
		t.Fatalf("DecodeNext after empty lines: %v", err)
	}
	if got.Seq != 7 {
		t.Errorf("seq: got %d, want 7", got.Seq)
	}

	// No more envelopes.
	_, err = DecodeNext(scanner)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

// isUnsupportedVersionErr checks whether err wraps ErrUnsupportedEnvelopeVersion.
func isUnsupportedVersionErr(err error) bool {
	return strings.Contains(err.Error(), ErrUnsupportedEnvelopeVersion.Error())
}
