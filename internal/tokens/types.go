package tokens

// Usage holds token consumption for a single request, including prompt-cache
// fields reported by Anthropic's API (Phase 4.8 token engine, I1).
type Usage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheWriteTokens  int
}

// TokenizerKind identifies which tokenizer strategy to apply.
type TokenizerKind int

const (
	// TokenizerAnthropic uses a calibrated heuristic (chars/3.5) because the
	// Anthropic tokenizer is not open-source. CountResult.Approximate is true.
	TokenizerAnthropic TokenizerKind = iota

	// TokenizerOpenAI uses cl100k_base BPE (exact count, Approximate=false).
	TokenizerOpenAI

	// TokenizerDeepSeek uses cl100k_base BPE, same as OpenAI (Approximate=false).
	TokenizerDeepSeek
)

// String returns the human-readable name of the tokenizer.
func (k TokenizerKind) String() string {
	switch k {
	case TokenizerAnthropic:
		return "anthropic"
	case TokenizerOpenAI:
		return "openai"
	case TokenizerDeepSeek:
		return "deepseek"
	default:
		return "unknown"
	}
}

// CountResult is the output of a single Count or CountMessages call.
type CountResult struct {
	// Tokens is the estimated or exact token count.
	Tokens int

	// Tokenizer identifies which strategy produced the count.
	Tokenizer TokenizerKind

	// Approximate is true when the count is produced by a heuristic rather
	// than a proper BPE encoder (e.g. Anthropic tokenizer).
	Approximate bool
}
