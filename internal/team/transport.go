package team

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// EnvelopeVersion is the current wire version.
const EnvelopeVersion uint8 = 1

// ErrUnsupportedEnvelopeVersion is returned when an envelope carries an
// unrecognised version number.
var ErrUnsupportedEnvelopeVersion = errors.New("team: unsupported envelope version")

// maxScanTokenSize is the maximum size of a single NDJSON line (10 MiB).
const maxScanTokenSize = 10 * 1024 * 1024

// Envelope is the wire-level message exchanged between team workers.
type Envelope struct {
	V       uint8           `json:"v"`
	Seq     uint64          `json:"seq"`
	From    string          `json:"from"`
	To      string          `json:"to"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Ts      time.Time       `json:"ts"`
}

// Transport is the low-level message-passing contract between team components.
type Transport interface {
	Send(env Envelope) error
	Recv() (Envelope, error)
	Close() error
}

// EncodeEnvelope serialises env as a single NDJSON line (JSON + "\n") into w.
func EncodeEnvelope(w io.Writer, e Envelope) error {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("team: marshal envelope: %w", err)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// newScanner returns a bufio.Scanner configured for NDJSON with a 10 MiB
// token buffer.
func newScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), maxScanTokenSize)
	return s
}

// NewScanner returns a bufio.Scanner configured for NDJSON with a 10 MiB
// token buffer. Use this in external packages (e.g. cmd/clue-code) that need
// to call DecodeNext on an arbitrary reader.
func NewScanner(r io.Reader) *bufio.Scanner {
	return newScanner(r)
}

// DecodeNext reads the next non-empty line from s, unmarshals it as an
// Envelope, and validates the version field.
// It returns (Envelope{}, io.EOF) when there are no more lines.
func DecodeNext(s *bufio.Scanner) (Envelope, error) {
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue // skip blank lines
		}
		var env Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return Envelope{}, fmt.Errorf("team: unmarshal envelope: %w", err)
		}
		if env.V != EnvelopeVersion {
			return Envelope{}, fmt.Errorf("%w: got %d", ErrUnsupportedEnvelopeVersion, env.V)
		}
		return env, nil
	}
	if err := s.Err(); err != nil {
		return Envelope{}, fmt.Errorf("team: scanner: %w", err)
	}
	return Envelope{}, io.EOF
}
