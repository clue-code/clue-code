package setup

// ConflictResolution describes one of the arbitration options presented to
// the user when conflicting priorities are detected.
type ConflictResolution struct {
	Label    string // display label, e.g. "Qualite avant tout"
	Provider string // provider key, e.g. "anthropic" or "hybrid:ollama+anthropic"
	Model    string // specific model, may be empty for hybrid
	Tradeoff string // what the user gives up
	CostNote string // short cost description
}

// Conflict bundles a detected tension between user priorities with the set of
// resolution options that should be presented for arbitration.
type Conflict struct {
	Description string               // short label, e.g. "Qualite maximale vs Hors-ligne"
	Reason      string               // human-readable explanation of the tension
	Options     []ConflictResolution // exactly 3 options (tradeoff A, tradeoff B, compromise)
}

// DetectConflicts inspects the user's answers and returns any conflicts that
// require explicit arbitration before a single recommendation can be made.
//
// Defined conflicts:
//  1. Quality + Offline: best models are cloud; local models are ~80 % quality.
//  2. Sensitive + Quality: top cloud models receive user prompts.
func DetectConflicts(a Answers) []Conflict {
	var conflicts []Conflict

	// Conflict 1: Quality priority AND Offline required.
	if !a.PriorityCost && a.Offline {
		conflicts = append(conflicts, Conflict{
			Description: "Qualite maximale vs Hors-ligne",
			Reason: "Les meilleurs modeles (Claude, GPT-4) sont cloud. " +
				"Les modeles locaux atteignent 60-80% de la qualite cloud.",
			Options: []ConflictResolution{
				{
					Label:    "Qualite avant tout",
					Provider: "anthropic",
					Model:    "claude-sonnet-4-6",
					Tradeoff: "Bloque hors-ligne",
					CostNote: "~$0.45/jour usage leger",
				},
				{
					Label:    "Hors-ligne avant tout",
					Provider: "ollama",
					Model:    "qwen2.5-coder:32b",
					Tradeoff: "80% qualite Claude (excellent pour le code)",
					CostNote: "Gratuit, 19 GB disque",
				},
				{
					Label:    "Compromis intelligent (mode hybrid)",
					Provider: "hybrid:ollama+anthropic",
					Model:    "qwen2.5-coder:32b + claude-sonnet-4-6",
					Tradeoff: "Bascule automatique selon la connexion",
					CostNote: "~$0.10/jour (cloud rare)",
				},
			},
		})
	}

	// Conflict 2: Sensitive (privacy) AND Quality priority (not cost-first).
	if a.Sensitive && !a.PriorityCost {
		conflicts = append(conflicts, Conflict{
			Description: "Confidentialite vs Qualite",
			Reason: "Les meilleurs modeles cloud (Claude, GPT-4) recoivent vos prompts. " +
				"Prive = local, moins puissant.",
			Options: []ConflictResolution{
				{
					Label:    "Confidentialite avant tout",
					Provider: "ollama",
					Model:    "qwen2.5-coder:32b",
					Tradeoff: "Donnees 100% locales",
					CostNote: "Gratuit",
				},
				{
					Label:    "Qualite avant tout",
					Provider: "anthropic",
					Model:    "claude-sonnet-4-6",
					Tradeoff: "Prompts envoyes a Anthropic (politique zero retention)",
					CostNote: "~$0.45/jour",
				},
				{
					Label:    "Prive local + cloud pour non-sensible (mode hybrid)",
					Provider: "hybrid:ollama+anthropic",
					Model:    "",
					Tradeoff: "Vous classifiez par projet",
					CostNote: "Variable",
				},
			},
		})
	}

	return conflicts
}
