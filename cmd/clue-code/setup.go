package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/clue-code/clue-code/internal/setup"
)

const setupBanner = `
  _____ _    _   _ _____    ____ ___  ____  _____
 / ____| |  | | | | ____|  / ___/ _ \|  _ \| ____|
| |    | |  | | | |  _|   | |  | | | | | | |  _|
| |____| |__| |_| | |___  | |__| |_| | |_| | |___
 \_____|_____\___/|_____|  \____\___/|____/|_____|

  Setup Wizard — configuration guidee pour non-developpeurs
`

const noColor = "NO_COLOR"

// color helpers — disabled when NO_COLOR env is set.
func colorEnabled() bool { return os.Getenv(noColor) == "" }

func bold(s string) string {
	if !colorEnabled() {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

func green(s string) string {
	if !colorEnabled() {
		return s
	}
	return "\033[32m" + s + "\033[0m"
}

func yellow(s string) string {
	if !colorEnabled() {
		return s
	}
	return "\033[33m" + s + "\033[0m"
}

func cyan(s string) string {
	if !colorEnabled() {
		return s
	}
	return "\033[36m" + s + "\033[0m"
}

func red(s string) string {
	if !colorEnabled() {
		return s
	}
	return "\033[31m" + s + "\033[0m"
}

// runSetup runs the interactive setup wizard.
// Returns 0 on success, 1 on error, 2 on usage error.
func runSetup(ctx context.Context, args []string) int {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Check for --explain flag.
	explainMode := false
	for _, a := range args {
		if a == "--explain" {
			explainMode = true
		}
	}

	// Check for resume.
	if setup.HasProgress() {
		prog, err := setup.LoadProgress()
		if err == nil && prog.Stage != "" {
			fmt.Printf("\n%s\n", yellow("Une session de setup precedente a ete detectee."))
			fmt.Printf("  Demarree le : %s\n", prog.StartedAt.Format("2006-01-02 15:04"))
			fmt.Printf("  Etape atteinte : %s\n", prog.Stage)
			fmt.Println()
			ans := prompt(ctx, "Reprendre cette session ? [O/n] : ")
			if isYes(ans) {
				return resumeSetup(ctx, prog, explainMode)
			}
			// User declined — clear and start fresh.
			_ = setup.ClearProgress()
		}
	}

	return runWizard(ctx, explainMode)
}

// runWizard executes the full 3-question wizard from the beginning.
func runWizard(ctx context.Context, explainMode bool) int {
	fmt.Print(setupBanner)
	fmt.Println(cyan("Ce wizard vous guide en 3 questions vers la configuration ideale."))
	fmt.Println(cyan("Appuyez sur Ctrl+C a tout moment pour annuler."))
	fmt.Println()

	prog := setup.Progress{StartedAt: time.Now()}

	// --- Question 1: Sensitive data ---
	fmt.Println(bold("Question 1/3 — Confidentialite des donnees"))
	fmt.Println("  Vos prompts contiendront-ils des donnees sensibles ou confidentielles")
	fmt.Println("  (code proprietaire, donnees personnelles, secrets d'entreprise) ?")
	fmt.Println()
	fmt.Println("  [1] Oui — je veux que tout reste sur ma machine")
	fmt.Println("  [2] Non — les services cloud sont OK")
	fmt.Println()
	q1 := promptChoice(ctx, "Votre choix [1/2] : ", []string{"1", "2"})
	if q1 == "" {
		return handleCancel()
	}
	sensitive := q1 == "1"

	prog.Stage = "q1"
	prog.PartialAnswers.Sensitive = sensitive
	_ = setup.SaveProgress(prog)

	fmt.Println()

	// --- Question 2: Priority ---
	fmt.Println(bold("Question 2/3 — Priorite"))
	fmt.Println("  Qu'est-ce qui compte le plus pour vous ?")
	fmt.Println()
	fmt.Println("  [1] Cout minimal (gratuit ou le moins cher possible)")
	fmt.Println("  [2] Meilleure qualite (raisonnement, code, analyse)")
	fmt.Println()
	q2 := promptChoice(ctx, "Votre choix [1/2] : ")
	if q2 == "" {
		return handleCancel()
	}
	priorityCost := q2 == "1"

	prog.Stage = "q2"
	prog.PartialAnswers.PriorityCost = priorityCost
	_ = setup.SaveProgress(prog)

	fmt.Println()

	// --- Question 3: Online/Offline ---
	fmt.Println(bold("Question 3/3 — Connectivite"))
	fmt.Println("  Avez-vous besoin que CLUE CODE fonctionne sans connexion internet ?")
	fmt.Println()
	fmt.Println("  [1] Oui — je travaille parfois hors-ligne ou sur reseau restreint")
	fmt.Println("  [2] Non — j'ai toujours internet")
	fmt.Println()
	q3 := promptChoice(ctx, "Votre choix [1/2] : ")
	if q3 == "" {
		return handleCancel()
	}
	offline := q3 == "1"

	prog.Stage = "q3"
	prog.PartialAnswers.Offline = offline
	_ = setup.SaveProgress(prog)

	answers := setup.Answers{
		Sensitive:    sensitive,
		PriorityCost: priorityCost,
		Offline:      offline,
		HasMacM:      runtime.GOARCH == "arm64" && runtime.GOOS == "darwin",
	}

	return runInstallPhase(ctx, answers, &prog, explainMode)
}

// resumeSetup restores wizard state from a Progress snapshot.
func resumeSetup(ctx context.Context, prog setup.Progress, explainMode bool) int {
	fmt.Printf("\n%s\n\n", bold("Reprise du wizard depuis l'etape : "+prog.Stage))
	answers := prog.PartialAnswers
	answers.HasMacM = runtime.GOARCH == "arm64" && runtime.GOOS == "darwin"
	return runInstallPhase(ctx, answers, &prog, explainMode)
}

// runInstallPhase shows the recommendation and executes the install.
func runInstallPhase(ctx context.Context, answers setup.Answers, prog *setup.Progress, explainMode bool) int {
	rec := setup.Recommend(answers)

	fmt.Println()
	fmt.Println("─────────────────────────────────────────────────────")

	// If conflicts detected, present arbitration UI.
	if len(rec.Conflicts) > 0 {
		chosenProvider, chosenModel := resolveConflicts(ctx, rec.Conflicts)
		if chosenProvider == "" {
			return handleCancel()
		}
		// Re-derive rec with the user's explicit choice by finding the matching
		// entry in RankProviders output; if not found, build a minimal rec.
		ranked := setup.RankProviders(answers)
		for _, ps := range ranked {
			if ps.Provider == chosenProvider && (chosenModel == "" || ps.Model == chosenModel) {
				rec.Primary = ps
				rec.Provider = ps.Provider
				rec.Model = ps.Model
				rec.Cost = setup.CostLabel(ps)
				rec.Steps = setup.BuildSteps(ps)
				rec.Justification = "Choix explicite de l'utilisateur apres arbitrage des conflits"
				break
			}
		}
		// If hybrid or unknown provider, set fields directly.
		if strings.HasPrefix(chosenProvider, "hybrid:") {
			rec.Provider = chosenProvider
			rec.Model = chosenModel
			rec.Cost = "variable"
			rec.Steps = []string{
				"Mode hybrid: configurez d'abord Ollama (local), puis Anthropic (cloud).",
				"Installer Ollama  : curl -fsSL https://ollama.com/install.sh | sh",
				"Configurer Anthropic : clue-code setup (choisir Anthropic)",
				"Activer le mode hybrid : clue-code mode hybrid",
			}
			rec.Justification = "Mode hybrid: local en priorite, cloud en fallback"
		}
	}

	// Display scoring table if --explain or user asks for it.
	if explainMode {
		printScoringTable(answers)
	}

	fmt.Printf("%s  Recommandation : %s\n", bold(">>"), bold(cyan(strings.ToUpper(rec.Provider))))
	fmt.Printf("   Modele       : %s\n", rec.Model)
	fmt.Printf("   Cout         : %s\n", rec.Cost)
	fmt.Println()
	fmt.Printf("   %s\n", rec.Justification)

	// Show top 3 alternatives when no conflicts.
	if len(rec.Conflicts) == 0 && len(rec.Alternatives) > 0 {
		fmt.Println()
		fmt.Println("   Alternatives (Top 3 selon vos reponses) :")
		for i, alt := range rec.Alternatives {
			fmt.Printf("     %d. %-12s %-25s %s\n", i+2, alt.Provider, alt.Model, alt.Description)
		}
	}

	fmt.Println()
	fmt.Println("   Etapes :")
	for i, step := range rec.Steps {
		fmt.Printf("     %d. %s\n", i+1, step)
	}
	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Println()

	// When no conflicts, present interactive choice menu (P1-P7).
	// Conflict path already resolved above; this branch handles the common case.
	if len(rec.Conflicts) == 0 {
		selected, action, err := chooseProvider(ctx, rec, answers, explainMode)
		if err != nil || action == "cancel" {
			fmt.Println()
			fmt.Println(yellow("Setup annule. Relancez 'clue-code setup' quand vous etes pret."))
			_ = setup.ClearProgress()
			return 0
		}
		if action == "restart" {
			_ = setup.ClearProgress()
			return runWizard(ctx, explainMode)
		}
		// action == "install": update rec with the selected provider.
		rec.Primary = selected
		rec.Provider = selected.Provider
		rec.Model = selected.Model
		rec.Cost = setup.CostLabel(selected)
		rec.Steps = setup.BuildSteps(selected)
	} else {
		// Conflict path: ask with the old simple confirm (no alternative menu needed).
		if !explainMode {
			detailAns, err := confirmYN(ctx, "Voir le detail du scoring ? [o/N] : ", false)
			if err != nil {
				return handleCancel()
			}
			if detailAns {
				printScoringTable(answers)
			}
		}
		installAns, err := confirmYN(ctx, "Voulez-vous installer/configurer maintenant ? [O/n] : ", true)
		if err != nil {
			return handleCancel()
		}
		if !installAns {
			fmt.Println()
			fmt.Println(yellow("Setup annule. Relancez 'clue-code setup' quand vous etes pret."))
			_ = setup.ClearProgress()
			return 0
		}
	}

	prog.Stage = "install"
	prog.Provider = rec.Provider
	_ = setup.SaveProgress(*prog)

	var installErr error
	switch rec.Provider {
	case "ollama":
		fmt.Println()
		fmt.Println(cyan("Installation d'Ollama en cours..."))
		installErr = setup.InstallOllama(ctx, func(stage string, pct float64) {
			fmt.Printf("  [%.0f%%] %s\n", pct*100, stage)
		})

	case "deepseek":
		fmt.Println()
		if err := setup.OpenBrowser("https://platform.deepseek.com/api_keys"); err == nil {
			fmt.Println(cyan("Navigateur ouvert -> https://platform.deepseek.com/api_keys"))
		} else {
			fmt.Println(yellow("Ouvrez: https://platform.deepseek.com/api_keys"))
		}
		fmt.Println()
		key := prompt(ctx, "Collez votre cle API DeepSeek (sk-...) : ")
		if key == "" {
			fmt.Println(red("Aucune cle fournie. Setup annule."))
			return 1
		}
		installErr = setup.ConfigureDeepSeek(ctx, strings.TrimSpace(key))

	case "anthropic":
		fmt.Println()
		if err := setup.OpenBrowser("https://console.anthropic.com/settings/keys"); err == nil {
			fmt.Println(cyan("Navigateur ouvert -> https://console.anthropic.com/settings/keys"))
		} else {
			fmt.Println(yellow("Ouvrez: https://console.anthropic.com/settings/keys"))
		}
		fmt.Println()
		key := prompt(ctx, "Collez votre cle API Anthropic (sk-ant-...) : ")
		if key == "" {
			fmt.Println(red("Aucune cle fournie. Setup annule."))
			return 1
		}
		installErr = setup.ConfigureAnthropic(ctx, strings.TrimSpace(key))

	case "mlx":
		fmt.Println()
		fmt.Println(cyan("Instructions pour MLX (Apple Silicon) :"))
		for i, step := range rec.Steps {
			fmt.Printf("  %d. %s\n", i+1, step)
		}
		fmt.Println()
		fmt.Println(yellow("MLX requiert Python. Une fois installe, relancez 'clue-code chat \"hello\"'."))
		_ = setup.ClearProgress()
		return 0

	default:
		// hybrid or unknown providers — print steps and exit successfully.
		fmt.Println()
		fmt.Println(cyan("Instructions de configuration :"))
		for i, step := range rec.Steps {
			fmt.Printf("  %d. %s\n", i+1, step)
		}
		fmt.Println()
		_ = setup.ClearProgress()
		return 0
	}

	if installErr != nil {
		fmt.Fprintf(os.Stderr, "\n%s\n", red("Erreur lors de l'installation : "+installErr.Error()))
		fmt.Println(yellow("La progression a ete sauvegardee. Relancez 'clue-code setup' pour reprendre."))
		return 1
	}

	// Success — run a quick validation chat.
	prog.Stage = "done"
	_ = setup.SaveProgress(*prog)

	fmt.Println()
	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Printf("%s %s\n", green("✓"), bold("Configuration terminee avec succes !"))
	fmt.Println()
	fmt.Println("  Test rapide :")
	fmt.Println("    clue-code chat \"hello\"")
	fmt.Println()
	fmt.Println("  Autres commandes utiles :")
	fmt.Println("    clue-code doctor       # verifier l'environnement")
	fmt.Println("    clue-code agent list   # lister les agents disponibles")
	fmt.Println("─────────────────────────────────────────────────────")

	_ = setup.ClearProgress()
	return 0
}

// resolveConflicts presents each conflict with numbered arbitration options
// and returns the chosen (provider, model). Returns ("", "") on cancel.
func resolveConflicts(ctx context.Context, conflicts []setup.Conflict) (string, string) {
	fmt.Println()
	fmt.Printf("%s %s\n", bold(yellow("! CONFLITS DETECTES")),
		yellow("— vos priorites sont en tension. Choisissez comment les arbitrer."))
	fmt.Println()

	var chosenProvider, chosenModel string

	for ci, c := range conflicts {
		fmt.Printf("%s Conflit %d/%d : %s\n", bold(">>"), ci+1, len(conflicts), bold(c.Description))
		fmt.Printf("   %s\n", c.Reason)
		fmt.Println()
		for i, opt := range c.Options {
			fmt.Printf("   [%d] %s\n", i+1, bold(opt.Label))
			fmt.Printf("       Contre-partie : %s\n", opt.Tradeoff)
			fmt.Printf("       Cout          : %s\n", opt.CostNote)
			fmt.Println()
		}

		validChoices := make([]string, len(c.Options))
		for i := range c.Options {
			validChoices[i] = fmt.Sprintf("%d", i+1)
		}
		choice := promptChoice(ctx, fmt.Sprintf("Votre choix [1-%d] : ", len(c.Options)), validChoices)
		if choice == "" {
			return "", ""
		}

		// Parse choice index (1-based).
		idx := 0
		for i, v := range validChoices {
			if choice == v {
				idx = i
				break
			}
		}
		chosen := c.Options[idx]
		chosenProvider = chosen.Provider
		chosenModel = chosen.Model
		fmt.Println()
	}

	return chosenProvider, chosenModel
}

// printScoringTable renders the full provider scoring table with weighted totals.
func printScoringTable(a setup.Answers) {
	w := setup.WeightsFromAnswers(a)
	ranked := setup.RankProviders(a)

	fmt.Println()
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	fmt.Println(bold("  Tableau de scoring detaille"))
	fmt.Printf("  Poids : Prive=%.0f  Cout=%.0f  Qualite=%.0f  Offline=%.0f\n",
		w.Privacy, w.Cost, w.Quality, w.Offline)
	fmt.Println()
	fmt.Printf("  %-12s %-25s %6s %6s %8s %8s %8s\n",
		"Provider", "Modele", "Prive", "Cout", "Qualite", "Offline", "TOTAL")
	fmt.Println("  " + strings.Repeat("-", 73))
	for _, p := range ranked {
		total := setup.ScoreProvider(p, w)
		fmt.Printf("  %-12s %-25s %6d %6d %8d %8d %8.0f\n",
			p.Provider, p.Model, p.Privacy, p.Cost, p.Quality, p.Offline, total)
	}
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	fmt.Println()
}

// chooseProvider presents the interactive alternative-choice menu (P1-P7).
// It returns the selected ProviderScore and one of the action strings:
// "install", "restart", or "cancel".
// When explainMode is already true, [s] still works but the table was already
// printed above; the function just re-prompts.
// Returns an error after 3 consecutive invalid responses.
func chooseProvider(ctx context.Context, rec setup.Recommendation, answers setup.Answers, explainMode bool) (setup.ProviderScore, string, error) {
	const maxAttempts = 3

	// Build the ordered list: [1]=primary, [2]=alt[0], [3]=alt[1] (when present).
	providers := []setup.ProviderScore{rec.Primary}
	providers = append(providers, rec.Alternatives...)

menuLoop:
	for {
		// Print menu.
		fmt.Println()
		fmt.Println(bold("Quel provider voulez-vous installer ?"))
		fmt.Println()
		for i, p := range providers {
			label := fmt.Sprintf("[%d] %-12s %-25s %s", i+1, p.Provider, p.Model, p.Description)
			if i == 0 {
				fmt.Printf("  %s  %s\n", label, cyan("← recommande"))
			} else {
				fmt.Printf("  %s\n", label)
			}
		}
		fmt.Println()
		fmt.Println("  [s] Voir le detail du scoring")
		fmt.Println("  [r] Refaire les questions")
		fmt.Println("  [n] Annuler")
		fmt.Println()

		for attempt := 0; attempt < maxAttempts; attempt++ {
			line := prompt(ctx, "Votre choix [1] : ")
			if line == "" && ctx.Err() != nil {
				return setup.ProviderScore{}, "cancel", ctx.Err()
			}
			trimmed := strings.TrimSpace(strings.ToLower(line))

			// Default: empty input selects primary (P7).
			if trimmed == "" {
				return providers[0], "install", nil
			}

			// Named actions.
			switch trimmed {
			case "s":
				// Show scoring table then re-display the full menu.
				printScoringTable(answers)
				continue menuLoop
			case "r":
				return setup.ProviderScore{}, "restart", nil
			case "n":
				return setup.ProviderScore{}, "cancel", nil
			}

			// Numeric selection.
			for i, p := range providers {
				if trimmed == fmt.Sprintf("%d", i+1) {
					return p, "install", nil
				}
			}

			// Invalid input.
			validChoices := make([]string, len(providers))
			for i := range providers {
				validChoices[i] = fmt.Sprintf("%d", i+1)
			}
			fmt.Printf("  %s Entrez %s, s, r ou n.\n", yellow("?"), strings.Join(validChoices, "/"))
		}
		// After maxAttempts invalid inputs, abort.
		fmt.Println(red("Trop de tentatives invalides. Setup annule."))
		return setup.ProviderScore{}, "cancel", fmt.Errorf("trop de tentatives invalides")
	}
}

// handleCancel prints a cancellation message and returns exit code 1.
func handleCancel() int {
	fmt.Println()
	fmt.Println(yellow("Setup interrompu. Relancez 'clue-code setup' quand vous etes pret."))
	return 1
}

// isYes returns true for empty input, "o", "O", "y", "Y", "yes", "oui".
func isYes(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "" || s == "o" || s == "y" || s == "yes" || s == "oui"
}

// confirmYN prompts for a strict O/N answer with re-prompt on invalid input.
// defaultVal is used when the user presses Enter without typing.
// Returns an error after 3 consecutive invalid responses.
func confirmYN(ctx context.Context, msg string, defaultVal bool) (bool, error) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		line := prompt(ctx, msg)
		if line == "" && ctx.Err() != nil {
			return false, ctx.Err()
		}
		trimmed := strings.TrimSpace(strings.ToLower(line))
		switch trimmed {
		case "":
			return defaultVal, nil
		case "o", "y", "oui", "yes":
			return true, nil
		case "n", "non", "no":
			return false, nil
		default:
			fmt.Printf("  %s Reponse non comprise. Tapez O ou N.\n", yellow("?"))
		}
	}
	fmt.Println(red("Trop de tentatives invalides. Setup annule."))
	return false, fmt.Errorf("trop de tentatives invalides")
}

// stdinReader is the io.Reader used by prompt(). It defaults to os.Stdin but
// tests can substitute a pipe by calling initStdinScanner with a new reader.
var stdinScanner *bufio.Scanner

// initStdinScanner resets the package-level stdin scanner to read from r.
// Called once at startup and by tests to inject a fake stdin.
func initStdinScanner() {
	stdinScanner = bufio.NewScanner(os.Stdin)
}

func init() {
	initStdinScanner()
}

// prompt prints msg and reads a line from stdin. Returns "" if ctx is done.
func prompt(ctx context.Context, msg string) string {
	fmt.Print(msg)
	ch := make(chan string, 1)
	go func() {
		if stdinScanner.Scan() {
			ch <- stdinScanner.Text()
		} else {
			ch <- ""
		}
	}()
	select {
	case <-ctx.Done():
		return ""
	case line := <-ch:
		return line
	}
}

// promptChoice prompts until the user enters one of the valid choices.
// Returns "" if ctx is cancelled.
func promptChoice(ctx context.Context, msg string, valid ...[]string) string {
	var allowed []string
	if len(valid) > 0 {
		allowed = valid[0]
	} else {
		allowed = []string{"1", "2"}
	}
	for {
		line := prompt(ctx, msg)
		if line == "" {
			return ""
		}
		line = strings.TrimSpace(line)
		for _, v := range allowed {
			if line == v {
				return line
			}
		}
		fmt.Printf("  %s Entrez %s.\n", yellow("?"), strings.Join(allowed, " ou "))
	}
}
