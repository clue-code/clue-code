package team

import (
	"encoding/json"
	"time"
)

// Message is the application-level view of an Envelope delivered to a worker's
// inbox. It strips the wire-level version field and provides typed fields.
type Message struct {
	Seq     uint64          `json:"seq"`
	From    string          `json:"from"`
	To      string          `json:"to"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Ts      time.Time       `json:"ts"`
}

// MessageFromEnvelope converts a wire-level Envelope to an application-level
// Message. The version field is intentionally dropped.
func MessageFromEnvelope(env Envelope) Message {
	return Message{
		Seq:     env.Seq,
		From:    env.From,
		To:      env.To,
		Kind:    env.Kind,
		Payload: env.Payload,
		Ts:      env.Ts,
	}
}
