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
| anthropic  | claude-sonnet-4-6  | 4       | 2    | 10      | 0       | $15.00      |
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

  [1] Qualite avant tout     → anthropic/claude-sonnet-4-6
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
