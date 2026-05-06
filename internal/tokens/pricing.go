// Package tokens — pricing.go holds a hardcoded USD pricing table for known
// providers and models.
//
// Prices sourced from official provider pricing pages on 2026-05-06:
//   - Anthropic: https://www.anthropic.com/pricing
//   - DeepSeek:  https://platform.deepseek.com/api-docs/pricing
//   - OpenAI:    https://openai.com/api/pricing
//   - Ollama / MLX: local inference, no cost.
package tokens

// modelPricing holds per-million-token USD rates for one model.
type modelPricing struct {
	InputPer1M       float64
	OutputPer1M      float64
	CacheReadPer1M   float64
	CacheWritePer1M  float64
}

// pricingTable maps provider → model → rates.
// Keys use the canonical lowercase form used by each provider's API.
var pricingTable = map[string]map[string]modelPricing{
	"anthropic": {
		// claude-sonnet-4 (aka claude-sonnet-4-5 family)
		"claude-sonnet-4":   {InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75},
		"claude-sonnet-4-5": {InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75},
		// claude-haiku-4.5
		"claude-haiku-4-5": {InputPer1M: 0.25, OutputPer1M: 1.25, CacheReadPer1M: 0.03, CacheWritePer1M: 0.30},
		"claude-haiku-4.5": {InputPer1M: 0.25, OutputPer1M: 1.25, CacheReadPer1M: 0.03, CacheWritePer1M: 0.30},
		// claude-opus-4
		"claude-opus-4": {InputPer1M: 15.00, OutputPer1M: 75.00, CacheReadPer1M: 1.50, CacheWritePer1M: 18.75},
	},
	"deepseek": {
		// deepseek-chat (DeepSeek-V3): $0.14 input / $0.28 output per 1M tokens
		"deepseek-chat":    {InputPer1M: 0.14, OutputPer1M: 0.28, CacheReadPer1M: 0.014, CacheWritePer1M: 0.14},
		"deepseek-v3":      {InputPer1M: 0.14, OutputPer1M: 0.28, CacheReadPer1M: 0.014, CacheWritePer1M: 0.14},
		"deepseek-v3.2":    {InputPer1M: 0.14, OutputPer1M: 0.28, CacheReadPer1M: 0.014, CacheWritePer1M: 0.14},
		// deepseek-reasoner (R1)
		"deepseek-reasoner": {InputPer1M: 0.55, OutputPer1M: 2.19, CacheReadPer1M: 0.055, CacheWritePer1M: 0.55},
		"deepseek-r1":        {InputPer1M: 0.55, OutputPer1M: 2.19, CacheReadPer1M: 0.055, CacheWritePer1M: 0.55},
	},
	"openai": {
		// gpt-4o
		"gpt-4o":      {InputPer1M: 2.50, OutputPer1M: 10.00, CacheReadPer1M: 1.25, CacheWritePer1M: 0},
		"gpt-4o-mini": {InputPer1M: 0.15, OutputPer1M: 0.60, CacheReadPer1M: 0.075, CacheWritePer1M: 0},
		// o3 / o4-mini
		"o3":      {InputPer1M: 10.00, OutputPer1M: 40.00, CacheReadPer1M: 2.50, CacheWritePer1M: 0},
		"o4-mini": {InputPer1M: 1.10, OutputPer1M: 4.40, CacheReadPer1M: 0.275, CacheWritePer1M: 0},
	},
	// Local inference providers — no cost.
	"ollama": {},
	"mlx":    {},
}

// CostUSD computes the USD cost for a Usage record given provider and model.
// Provider and model matching is case-insensitive. Returns 0.0 for unknown
// providers/models (e.g. ollama, mlx, custom local models) to avoid false
// budget blocking on local inference.
func CostUSD(provider, model string, u Usage) float64 {
	models, ok := pricingTable[provider]
	if !ok {
		return 0
	}
	p, ok := models[model]
	if !ok {
		return 0
	}

	const perMillion = 1_000_000.0

	cost := float64(u.InputTokens)*p.InputPer1M/perMillion +
		float64(u.OutputTokens)*p.OutputPer1M/perMillion +
		float64(u.CacheReadTokens)*p.CacheReadPer1M/perMillion +
		float64(u.CacheWriteTokens)*p.CacheWritePer1M/perMillion

	return cost
}
