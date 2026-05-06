package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/clue-code/clue-code/internal/clock"
	"github.com/clue-code/clue-code/internal/tokens"
)

const tokensUsage = `Usage: clue-code tokens <action> [flags]

Actions:
  summary       Show token usage and cost for the last 24 hours
  top           Show top 10 provider/model pairs by cost (last 7 days)
  clear-cache   Remove all cached token-count entries from disk

Flags:
  -h, --help    Show this message
`

func runTokens(args []string) {
	fs := flag.NewFlagSet("tokens", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, tokensUsage) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if fs.NArg() == 0 {
		fmt.Fprint(os.Stderr, tokensUsage)
		os.Exit(2)
	}

	action := fs.Arg(0)
	switch action {
	case "summary":
		runTokensSummary()
	case "top":
		runTokensTop()
	case "clear-cache":
		runTokensClearCache()
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, tokensUsage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "clue-code tokens: unknown action %q\n\n", action)
		fmt.Fprint(os.Stderr, tokensUsage)
		os.Exit(2)
	}
}

func runTokensSummary() {
	ledger := defaultLedgerPath()
	a, err := tokens.NewAnalytics(ledger, clock.Real())
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code tokens summary: %v\n", err)
		os.Exit(1)
	}

	report := a.Summary(24 * time.Hour)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Provider\tModel\tTokens\tUSD")

	// Collect per-model rows. Analytics only exposes ByModel (keyed by model
	// name) and ByProvider, so we emit one row per model with its USD cost.
	// Token counts are not broken down by model in the Report, so we show
	// the total token count only on the TOTAL line.
	for model, usd := range report.ByModel {
		provider := providerForModel(report, model)
		fmt.Fprintf(w, "%s\t%s\t—\t$%.2f\n", provider, model, usd)
	}

	fmt.Fprintln(w, "────────────────────────────────────────")
	fmt.Fprintf(w, "TOTAL\t\t%s\t$%.2f\n", formatTokens(report.TotalTokens), report.TotalUSD)
	_ = w.Flush()
}

func runTokensTop() {
	ledger := defaultLedgerPath()
	a, err := tokens.NewAnalytics(ledger, clock.Real())
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code tokens top: %v\n", err)
		os.Exit(1)
	}

	entries := a.Top(10)
	if len(entries) == 0 {
		fmt.Println("No usage records found in the last 7 days.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Provider\tModel\tUSD")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t$%.4f\n", e.Provider, e.Model, e.CostUSD)
	}
	_ = w.Flush()
}

func runTokensClearCache() {
	cacheDir := defaultCacheDir()
	c, err := tokens.NewCache(100, cacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clue-code tokens clear-cache: %v\n", err)
		os.Exit(1)
	}
	c.Clear()
	fmt.Println("cache cleared")
}

// defaultCacheDir returns the standard token cache directory.
func defaultCacheDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "clue-code", "tokens-cache")
	}
	return filepath.Join(base, "clue-code", "tokens-cache")
}

// defaultLedgerPath returns the standard analytics ledger path.
func defaultLedgerPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "clue-code", "analytics-ledger.jsonl")
	}
	return filepath.Join(base, "clue-code", "analytics-ledger.jsonl")
}

// formatTokens formats a token count as a human-readable string (e.g. "12.3K").
func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// providerForModel returns the provider name for a given model from the report's
// ByProvider map. Since Report.ByProvider is keyed by provider (not model), we
// use it only for the total. For individual rows we emit the provider from
// TopEntry when available; here we fall back to "—".
func providerForModel(_ tokens.Report, _ string) string {
	// The Summary Report does not carry per-model provider attribution.
	// Use "—" and let users run `top` for full provider+model breakdown.
	return "—"
}
