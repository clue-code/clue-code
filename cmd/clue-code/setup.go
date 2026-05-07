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
func runSetup(ctx context.Context, _ []string) int {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

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
				return resumeSetup(ctx, prog)
			}
			// User declined — clear and start fresh.
			_ = setup.ClearProgress()
		}
	}

	return runWizard(ctx)
}

// runWizard executes the full 3-question wizard from the beginning.
func runWizard(ctx context.Context) int {
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

	return runInstallPhase(ctx, answers, &prog)
}

// resumeSetup restores wizard state from a Progress snapshot.
func resumeSetup(ctx context.Context, prog setup.Progress) int {
	fmt.Printf("\n%s\n\n", bold("Reprise du wizard depuis l'etape : "+prog.Stage))
	answers := prog.PartialAnswers
	answers.HasMacM = runtime.GOARCH == "arm64" && runtime.GOOS == "darwin"
	return runInstallPhase(ctx, answers, &prog)
}

// runInstallPhase shows the recommendation and executes the install.
func runInstallPhase(ctx context.Context, answers setup.Answers, prog *setup.Progress) int {
	rec := setup.Recommend(answers)

	fmt.Println()
	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Printf("%s  Recommandation : %s\n", bold(">>"), bold(cyan(strings.ToUpper(rec.Provider))))
	fmt.Printf("   Modele       : %s\n", rec.Model)
	fmt.Printf("   Cout         : %s\n", rec.Cost)
	fmt.Println()
	fmt.Printf("   %s\n", rec.Justification)
	fmt.Println()
	fmt.Println("   Etapes :")
	for i, step := range rec.Steps {
		fmt.Printf("     %d. %s\n", i+1, step)
	}
	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Println()

	ans := prompt(ctx, "Voulez-vous installer/configurer maintenant ? [O/n] : ")
	if !isYes(ans) {
		fmt.Println()
		fmt.Println(yellow("Setup annule. Relancez 'clue-code setup' quand vous etes pret."))
		_ = setup.ClearProgress()
		return 0
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
			fmt.Println(cyan("Navigateur ouvert → https://platform.deepseek.com/api_keys"))
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
			fmt.Println(cyan("Navigateur ouvert → https://console.anthropic.com/settings/keys"))
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
		fmt.Fprintf(os.Stderr, "setup: provider inconnu %q\n", rec.Provider)
		return 1
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

// prompt prints msg and reads a line from stdin. Returns "" if ctx is done.
func prompt(ctx context.Context, msg string) string {
	fmt.Print(msg)
	ch := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		if sc.Scan() {
			ch <- sc.Text()
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
