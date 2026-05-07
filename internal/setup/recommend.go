package setup

import (
	"fmt"
	"strings"
)

// Answers holds the user's responses to the three wizard questions.
type Answers struct {
	// Sensitive is true when the user wants data to stay on-device.
	Sensitive bool
	// PriorityCost is true when cost is the primary concern over quality.
	PriorityCost bool
	// Offline is true when the user needs offline / air-gapped operation.
	Offline bool
	// HasMacM is true when running on Apple Silicon (arm64 darwin).
	HasMacM bool
}

// Recommendation is the output of the recommendation engine.
//
// When Conflicts is non-empty the caller must present arbitration options to
// the user before proceeding. When Conflicts is empty, Primary is the best
// single choice and Alternatives holds the next 1–2 runners-up.
type Recommendation struct {
	// Primary is the top-ranked provider/model.
	Primary ProviderScore
	// Alternatives holds up to 2 additional ranked options shown when there
	// are no conflicts.
	Alternatives []ProviderScore
	// Conflicts lists detected tensions that require user arbitration.
	Conflicts []Conflict
	// Justification is a human-readable explanation of why Primary was chosen.
	Justification string
	// Cost is a short cost description for backward-compat display (e.g. "free").
	Cost string
	// Steps lists the high-level actions needed to complete setup.
	Steps []string

	// Legacy fields kept for backward compatibility with existing callers.
	// Provider and Model mirror Primary.Provider and Primary.Model.
	Provider string
	Model    string
}

// Recommend maps a set of Answers to the best provider recommendation.
//
// It replaces the old binary if/else logic with weighted multi-criteria
// scoring across 4 dimensions (Privacy, Cost, Quality, Offline).
// When conflicting priorities are detected the Conflicts field is populated
// so the caller can present explicit arbitration options.
func Recommend(a Answers) Recommendation {
	conflicts := DetectConflicts(a)
	ranked := RankProviders(a)

	top := ranked[0]

	rec := Recommendation{
		Primary:       top,
		Conflicts:     conflicts,
		Justification: buildJustification(a, top),
		Cost:          CostLabel(top),
		Steps:         BuildSteps(top),
		// Legacy mirror fields.
		Provider: top.Provider,
		Model:    top.Model,
	}

	// Only surface alternatives when there is no conflict to arbitrate.
	if len(conflicts) == 0 && len(ranked) > 1 {
		end := minInt(3, len(ranked))
		rec.Alternatives = ranked[1:end]
	}

	return rec
}

// buildJustification produces a concise explanation of why top was chosen.
func buildJustification(a Answers, top ProviderScore) string {
	parts := []string{}
	if (a.Sensitive || top.Privacy >= 8) && top.Privacy >= 8 {
		parts = append(parts, "vos donnees restent privees")
	}
	if a.PriorityCost && top.Cost >= 8 {
		if top.CostUSD1M == 0 {
			parts = append(parts, "cout minimal (gratuit)")
		} else {
			parts = append(parts, fmt.Sprintf("cout minimal ($%.2f/M tokens)", top.CostUSD1M))
		}
	}
	if !a.PriorityCost && top.Quality >= 8 {
		parts = append(parts, "qualite top niveau")
	}
	if a.Offline && top.Offline >= 8 {
		parts = append(parts, "fonctionne hors-ligne")
	}
	if len(parts) == 0 {
		parts = append(parts, top.Description)
	}
	return strings.Join(parts, ", ")
}

// CostLabel returns a human-readable cost string for display.
// Exported for use by the cmd layer.
func CostLabel(p ProviderScore) string {
	if p.CostUSD1M == 0 {
		return "gratuit (local)"
	}
	return fmt.Sprintf("$%.2f/M tokens", p.CostUSD1M)
}

// BuildSteps returns setup instructions for the given provider.
// It is exported so the cmd layer can rebuild steps after conflict arbitration.
func BuildSteps(p ProviderScore) []string {
	switch p.Provider {
	case "ollama":
		return []string{
			"Installer Ollama  : curl -fsSL https://ollama.com/install.sh | sh",
			fmt.Sprintf("Telecharger le modele : ollama pull %s", p.Model),
			"Demarrer Ollama   : ollama serve  (si pas deja actif)",
			"Tester            : clue-code chat \"hello\"",
		}
	case "mlx":
		return []string{
			"Installer Python 3.11+ : brew install python@3.11",
			"Installer MLX-LM       : pip install mlx-lm",
			fmt.Sprintf("Demarrer le serveur    : mlx_lm.server --model mlx-community/%s", p.Model),
			"Configurer clue-code   : clue-code mode local",
		}
	case "deepseek":
		return []string{
			"Creer un compte DeepSeek : https://platform.deepseek.com",
			"Generer une cle API      : Settings -> API Keys -> Create",
			"Coller la cle ici        : (le wizard vous demandera)",
			"Tester                   : clue-code chat \"hello\"",
		}
	case "anthropic":
		return []string{
			"Creer un compte Anthropic : https://console.anthropic.com",
			"Generer une cle API       : Settings -> API Keys -> Create key",
			"Coller la cle ici         : (le wizard vous demandera)",
			"Tester                    : clue-code chat \"hello\"",
		}
	case "groq":
		return []string{
			"Creer un compte Groq : https://console.groq.com",
			"Generer une cle API  : API Keys -> Create API Key",
			"Coller la cle ici    : (le wizard vous demandera)",
			"Tester               : clue-code chat \"hello\"",
		}
	case "openrouter":
		return []string{
			"Creer un compte OpenRouter : https://openrouter.ai",
			"Generer une cle API        : Keys -> Create Key",
			"Coller la cle ici          : (le wizard vous demandera)",
			"Tester                     : clue-code chat \"hello\"",
		}
	default:
		return []string{
			fmt.Sprintf("Configurer le provider %q manuellement", p.Provider),
			"Tester : clue-code chat \"hello\"",
		}
	}
}
