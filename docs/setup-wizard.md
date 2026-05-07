# CLUE CODE Setup Wizard

`clue-code setup` is an interactive wizard that guides non-developers through
configuring their first AI model in 3 questions.

## Quick start

```bash
clue-code setup
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

## Decision Tree

```
Sensitive=yes OR Offline=yes
  └── Apple Silicon (M1/M2/M3/M4) AND quality priority?
        ├── YES → MLX  (fastest local GPU inference)
        └── NO  → Ollama (llama3.2, free, cross-platform)

Sensitive=no AND Offline=no
  ├── Cost priority?  → DeepSeek ($0.14/M tokens)
  └── Quality priority → Anthropic Claude (best reasoning)
```

## Providers

| Provider  | Privacy | Cost           | Quality | Offline |
|-----------|---------|----------------|---------|---------|
| Ollama    | Local   | Free           | Good    | Yes     |
| MLX       | Local   | Free           | Good    | Yes     |
| DeepSeek  | Cloud   | $0.14/M tokens | Very good | No   |
| Anthropic | Cloud   | ~$3/M tokens   | Best    | No      |

## Crash Recovery

If the wizard is interrupted (Ctrl+C, network drop, crash), progress is saved
to `~/.clue-code/setup-progress.json`. The next `clue-code setup` invocation
will detect this file and offer to resume.

To start fresh, decline the resume prompt or delete the file:

```bash
rm ~/.clue-code/setup-progress.json
```

## FAQ

### Et si Ollama echoue a l'installation ?

Run `ollama serve` manually after installing:

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama serve &
ollama pull llama3.2
clue-code chat "hello"
```

### Et si je n'ai pas Internet ?

Choose **Oui** for Question 3. The wizard will guide you through Ollama or MLX
installation (the initial model download requires internet, but inference is
fully offline after that).

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
