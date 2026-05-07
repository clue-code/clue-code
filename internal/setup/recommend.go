// Package setup implements the interactive setup wizard for CLUE CODE.
package setup

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
type Recommendation struct {
	// Provider is a short lowercase identifier: "ollama", "deepseek", "anthropic", "mlx".
	Provider string
	// Model is the specific model name to use.
	Model string
	// Justification is a human-readable explanation shown to the user.
	Justification string
	// Cost is a short cost description (e.g. "free", "$0.14/M tokens").
	Cost string
	// Steps lists the high-level actions needed to complete setup.
	Steps []string
}

// Recommend maps a set of Answers to the best provider recommendation.
//
// Decision logic:
//   - Sensitive=true OR Offline=true → Ollama (always local, no data leaves device)
//   - !Sensitive AND !Offline AND PriorityCost=true → DeepSeek ($0.14/M, cloud)
//   - !Sensitive AND !Offline AND !PriorityCost → Anthropic Claude (best quality, cloud)
//   - HasMacM=true AND Sensitive → MLX is offered as alternative to Ollama
//
// MLX is only surfaced when HasMacM is true AND Sensitive is true AND
// PriorityCost is false (best local quality on Apple Silicon).
func Recommend(a Answers) Recommendation {
	switch {
	case a.HasMacM && a.Sensitive && !a.PriorityCost && !a.Offline:
		// Apple Silicon + privacy conscious + quality preferred → MLX
		return Recommendation{
			Provider:      "mlx",
			Model:         "mlx-community/Mistral-7B-Instruct-v0.3-4bit",
			Justification: "Vous avez un Mac Apple Silicon et souhaitez garder vos données privées. MLX offre des inférences GPU natives ultra-rapides sans aucune donnée quittant votre machine.",
			Cost:          "gratuit (GPU local)",
			Steps: []string{
				"Installer Python 3.11+  : brew install python@3.11",
				"Installer MLX-LM        : pip install mlx-lm",
				"Démarrer le serveur     : mlx_lm.server --model mlx-community/Mistral-7B-Instruct-v0.3-4bit",
				"Configurer clue-code    : clue-code mode local",
			},
		}

	case a.Sensitive || a.Offline:
		// Privacy / offline requirement → Ollama
		return Recommendation{
			Provider:      "ollama",
			Model:         "llama3.2",
			Justification: "Vos données restent entièrement sur votre machine. Ollama fait tourner les modèles en local, sans connexion internet requise après le téléchargement initial.",
			Cost:          "gratuit (CPU/GPU local)",
			Steps: []string{
				"Installer Ollama  : curl -fsSL https://ollama.com/install.sh | sh",
				"Télécharger llama3.2 : ollama pull llama3.2",
				"Démarrer Ollama   : ollama serve  (si pas déjà actif)",
				"Tester            : clue-code chat \"hello\"",
			},
		}

	case !a.Sensitive && !a.Offline && a.PriorityCost:
		// Cost-first, cloud OK → DeepSeek
		return Recommendation{
			Provider:      "deepseek",
			Model:         "deepseek-chat",
			Justification: "DeepSeek offre d'excellentes performances à un coût très faible ($0.14/M tokens en entrée). Idéal pour un usage intensif sans se ruiner.",
			Cost:          "$0.14/M tokens",
			Steps: []string{
				"Créer un compte DeepSeek : https://platform.deepseek.com",
				"Générer une clé API      : Settings → API Keys → Create",
				"Coller la clé ici        : (le wizard vous demandera)",
				"Tester                   : clue-code chat \"hello\"",
			},
		}

	default:
		// Quality-first, cloud OK → Anthropic
		return Recommendation{
			Provider:      "anthropic",
			Model:         "claude-sonnet-4-6",
			Justification: "Claude d'Anthropic offre la meilleure qualité de raisonnement, d'analyse de code et de génération de texte. Recommandé pour un usage professionnel.",
			Cost:          "~$3/M tokens",
			Steps: []string{
				"Créer un compte Anthropic : https://console.anthropic.com",
				"Générer une clé API       : Settings → API Keys → Create key",
				"Coller la clé ici         : (le wizard vous demandera)",
				"Tester                    : clue-code chat \"hello\"",
			},
		}
	}
}
