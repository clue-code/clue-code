# CLUE CODE Setup Wizard

`clue-code setup` is an interactive wizard that guides non-developers through
configuring their first AI model in 3 questions.

## Quick start

```bash
clue-code setup
# With detailed scoring breakdown:
clue-code setup --explain
```

## The 3 Questions

### Question 1 — Data Privacy

> "Vos prompts contiendront-ils des donnees sensibles ou confidentielles ?"

**[1] Oui** — Your data stays entirely on your machine.
**[2] Non** — Cloud services are acceptable.

If you handle proprietary source code, personal data, or business secrets,
choose **Oui**. The wizard will recommend a fully local model (Ollama or MLX).

---

### Question 2 — Priority

> "Qu'est-ce qui compte le plus pour vous ?"

**[1] Cout minimal** — cheapest option, great for high-volume use.
**[2] Meilleure qualite** — best reasoning and code quality.

---

### Question 3 — Connectivity

> "Avez-vous besoin que CLUE CODE fonctionne sans connexion internet ?"

**[1] Oui** — fully offline, air-gapped support.
**[2] Non** — always connected.

---

## Multi-criteria Scoring

The wizard no longer uses a binary decision tree. Instead, every provider is
scored across **4 dimensions** (0–10 each) with weights derived from your answers:

| Dimension | Weight when priority | Weight otherwise |
|-----------|---------------------|-----------------|
| Privacy   | ×3 (if Sensitive=Oui) | ×1 |
| Cost      | ×3 (if Priority=Cout) | ×1 |
| Quality   | ×3 (if Priority=Qualite) | ×1 |
| Offline   | ×3 (if Offline=Oui) | ×1 |

**Total score** = Privacy×wP + Cost×wC + Quality×wQ + Offline×wO

The top-3 providers by score are presented. When answers conflict, the wizard
surfaces explicit arbitration options instead of imposing a single choice.

### Provider Score Table

| Provider   | Model              | Privacy | Cost | Quality | Offline | $/1M tokens |
|------------|--------------------|---------|------|---------|---------|-------------|
| ollama     | llama3.2           | 10      | 10   | 5       | 10      | free        |
| ollama     | qwen2.5-coder:32b  | 10      | 10   | 8       | 10      | free        |
| ollama     | deepseek-r1:7b     | 10      | 10   | 7       | 10      | free        |
| mlx        | Llama-3.2-3B       | 10      | 10   | 6       | 10      | free        |
| deepseek   | deepseek-chat      | 4       | 9    | 8       | 0       | $0.28       |
| anthropic  | claude-sonnet-4-5  | 4       | 2    | 10      | 0       | $15.00      |
| groq       | llama-3.3-70b      | 4       | 7    | 8       | 0       | $0.59       |
| openrouter | various            | 4       | 6    | 9       | 0       | ~$1.50      |

MLX is only included in rankings when running on Apple Silicon (darwin/arm64).

### Example: Quality + Offline (the conflict case)

```
Answers: Sensitive=Non, Priority=Qualite, Offline=Oui

Weights: Privacy=1, Cost=1, Quality=3, Offline=3

Scores:
  ollama/qwen2.5-coder:32b : 10+10+24+30 = 74  ← local winner
  anthropic/claude-sonnet  : 4+2+30+0    = 36  ← cloud quality winner

! CONFLIT DETECTE: Qualite maximale vs Hors-ligne
  Les meilleurs modeles (Claude, GPT-4) sont cloud.
  Les modeles locaux atteignent 60-80% de la qualite cloud.

  [1] Qualite avant tout     → anthropic/claude-sonnet-4-5
  [2] Hors-ligne avant tout  → ollama/qwen2.5-coder:32b
  [3] Compromis intelligent  → hybrid:ollama+anthropic
```

Instead of silently picking Ollama llama3.2 (lightweight), the wizard
detects the tension and lets the user choose explicitly.

---

## Conflict Resolution

Conflicts are detected when user priorities are inherently in tension:

### Conflict 1: Quality vs Offline

Triggered when: Priority=Qualite AND Offline=Oui

Best-quality models (Claude, GPT-4) are cloud-only. Local models reach
60-80 % of cloud quality. Three options are presented:

1. **Quality first** — cloud provider, blocked offline
2. **Offline first** — best local model (qwen2.5-coder:32b), ~80 % quality
3. **Smart compromise** — hybrid mode: local primary, cloud fallback

### Conflict 2: Privacy vs Quality

Triggered when: Sensitive=Oui AND Priority=Qualite

Top cloud models receive your prompts. Options:

1. **Privacy first** — fully local (ollama/qwen2.5-coder:32b)
2. **Quality first** — Anthropic Claude (zero-retention policy)
3. **Hybrid** — local for sensitive projects, cloud for others

---

## Choisir une alternative (Phase 5.3)

After the recommendation is displayed, the wizard presents an interactive
numbered menu instead of a simple O/N confirm. This applies to the
**conflict-free path** only; conflict arbitration (Phase 5.1) still takes
precedence when tensions are detected.

### Menu format

```
Quel provider voulez-vous installer ?

  [1] anthropic     claude-sonnet-4-5         Top qualite, cher  ← recommande
  [2] openrouter    various                   Acces 100+ modeles
  [3] deepseek      deepseek-chat             Cloud, 53x moins cher que Claude

  [s] Voir le detail du scoring
  [r] Refaire les questions
  [n] Annuler

Votre choix [1] : _
```

### Actions

| Input | Effect |
|-------|--------|
| Enter (vide) | Accepte la recommandation primaire [1] (P7 default) |
| `1` | Installe le provider recommande |
| `2` | Installe la 2e option (1ere alternative) |
| `3` | Installe la 3e option (2e alternative, si disponible) |
| `s` | Affiche le tableau de scoring detaille, puis re-affiche le menu |
| `r` | Efface les reponses en memoire et relance le wizard depuis Q1 |
| `n` | Annule proprement — aucune modification effectuee |

### Validation

- Invalid input (e.g. `4`, `abc`, `!`) triggers re-prompt with hint
- After **3 consecutive invalid** responses the wizard exits with an error
- `[r]` (restart) resets answers in memory only — `setup-progress.json` is
  cleared so the next run starts fresh from Q1

### Walkthrough example (answers 2/2/2 — quality, cloud, connected)

```
>> Recommandation : ANTHROPIC
   Modele       : claude-sonnet-4-5
   Cout         : $15.00/M tokens

   qualite top niveau

   Alternatives (Top 3 selon vos reponses) :
     2. openrouter    various                   Acces 100+ modeles
     3. groq          llama-3.3-70b             Ultra-rapide cloud

   Etapes :
     1. Creer un compte Anthropic : https://console.anthropic.com
     ...
─────────────────────────────────────────────────────

Quel provider voulez-vous installer ?

  [1] anthropic     claude-sonnet-4-5         Top qualite, cher  ← recommande
  [2] openrouter    various                   Acces 100+ modeles
  [3] groq          llama-3.3-70b             Ultra-rapide cloud

  [s] Voir le detail du scoring
  [r] Refaire les questions
  [n] Annuler

Votre choix [1] : 2        ← user picks openrouter
```

The wizard then continues with the openrouter install flow.

---

## Strict Y/N Validation (O7)

All yes/no prompts use strict validation with re-prompt on invalid input:

- Accepted: `O`, `o`, `Y`, `y`, `oui`, `yes`, Enter (= default)
- Accepted: `N`, `n`, `non`, `no`
- Rejected: `0`, `x`, digits, random letters → re-prompt with explanation
- After 3 consecutive invalid responses → wizard exits with error

---

## Crash Recovery

If the wizard is interrupted (Ctrl+C, network drop, crash), progress is saved
to `~/.clue-code/setup-progress.json`. The next `clue-code setup` invocation
will detect this file and offer to resume.

To start fresh, decline the resume prompt or delete the file:

```bash
rm ~/.clue-code/setup-progress.json
```

---

## FAQ

### Et si Ollama echoue a l'installation ?

Run `ollama serve` manually after installing:

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama serve &
ollama pull qwen2.5-coder:32b
clue-code chat "hello"
```

### Et si je n'ai pas Internet ?

Choose **Oui** for Question 3. The wizard will guide you through Ollama or MLX
installation (the initial model download requires internet, but inference is
fully offline after that).

### Comment voir le detail du scoring ?

Run with `--explain` flag or answer `O` when the wizard asks:

```bash
clue-code setup --explain
```

This prints the full weighted score table for all providers.

### Comment changer de provider plus tard ?

Run `clue-code setup` again. Your previous configuration is preserved;
a new one is written on top.

### Comment verifier que tout fonctionne ?

```bash
clue-code doctor        # full health report
clue-code doctor --brief # compact 3-line summary
clue-code chat "hello"  # end-to-end smoke test
```

### Est-ce que mes cles API sont stockees en clair ?

Keys are stored in `~/.config/clue-code/config.json` with permissions `0600`
(readable only by the owner). For production environments, prefer exporting
keys via environment variables (`DEEPSEEK_API_KEY`, `ANTHROPIC_API_KEY`).
