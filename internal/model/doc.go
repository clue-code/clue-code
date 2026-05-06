// Package model implements the L2 model proxy layer for clue-code.
//
// All providers expose a common Client interface that speaks the OpenAI-compatible
// LCD (lowest-common-denominator) wire format: a ChatRequest containing a model
// ID, a slice of Messages, and optional streaming/temperature/token parameters.
//
// Cloud providers (DeepSeek, Anthropic, Groq, OpenRouter) are thin wrappers
// around the shared httpClient which handles retries, backoff, and SSE parsing.
// Local providers (Ollama, MLX-LM) are launched as subprocesses and speak the
// same OpenAI-compatible JSON protocol over HTTP or stdin/stdout.
//
// The Client interface contract:
//
//	Chat        — blocking, returns Response with full content and token usage.
//	ChatStream  — returns a channel of Chunk values; caller must drain the channel.
//	             The last Chunk has Done=true. The channel is closed after Done.
//	Provider    — returns the provider name string (e.g. "deepseek", "ollama").
//
// Config is loaded from ~/.config/clue-code/config.yaml. If absent, a minimal
// default config is returned with a single DeepSeek model entry.
package model
