// Package model — middleware.go wires the optional token engine
// (Counter/Cache/Budget/Analytics) into the model HTTP layer.
//
// Middleware is opt-in: callers that pass nil receive no-op behaviour and
// incur zero overhead. This preserves full backward compatibility.
package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/clue-code/clue-code/internal/tokens"
)

// Middleware bundles the optional token-engine components.
// Any field may be nil; nil fields are silently skipped.
type Middleware struct {
	Counter   tokens.Counter
	Cache     tokens.Cache
	Budget    tokens.Budget
	Analytics tokens.Analytics
	// Provider and Model are injected at call time; stored here for cache-key
	// construction helpers that need them before the HTTP call.
}

// CacheKey computes a deterministic sha256-based key from provider, model,
// system prompt, and messages. Used for both cache lookup and storage.
func CacheKey(provider, model, system string, msgs []Message) string {
	type payload struct {
		Provider string    `json:"p"`
		Model    string    `json:"m"`
		System   string    `json:"s"`
		Messages []Message `json:"msgs"`
	}
	data, _ := json.Marshal(payload{Provider: provider, Model: model, System: system, Messages: msgs})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
