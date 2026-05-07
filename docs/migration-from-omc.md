# Migration from oh-my-claudecode (OMC) to CLUE CODE

This guide maps every OMC concept to its CLUE CODE equivalent so you can
switch in under 10 minutes (acceptance criterion M2).

---

## Concept map

| oh-my-claudecode | CLUE CODE | Notes |
|---|---|---|
| `/oh-my-claudecode:autopilot` | `clue-code skill run autopilot` | Same loop semantics |
| `/oh-my-claudecode:ralph` | `clue-code skill run ralph` | Self-referential execution loop |
| `/oh-my-claudecode:ultrawork` | `clue-code skill run ultrawork` | Parallel task engine |
| `/team N:executor "task"` | `clue-code team demo` / `clue-code team list` | Team primitives with DAG scheduler |
| `state_read` / `state_write` MCP tools | `clue-code state get <key>` / `clue-code state set <key> <val>` | Local JSON store |
| `state_list_active` | `clue-code state list-active` | Active session scopes |
| Hook settings in `settings.json` | `~/.clue-code/hooks.yaml` | YAML format, same event model |
| `.omc/notepad.md` | `.clue-code/notepad.md` | Project-local scratch pad |
| `.omc/project-memory.json` | `.clue-code/project-memory.json` | Persistent memory across sessions |
| `CLUE_CODE_AGENTS_DIR` env var | `CLUE_CODE_AGENTS_DIR` | Same env var name |
| `agents/*.md` agent definitions | `agents/*.md` | Drop-in compatible |
| `skills/*.md` skill definitions | `skills/*.md` | Drop-in compatible |

---

## 5-step migration

### Step 1 — Install CLUE CODE

```bash
# Option A: build from source (current, while binary releases are pending)
git clone https://github.com/clue-code/clue-code.git
cd clue-code
go build -o clue-code ./cmd/clue-code
sudo mv clue-code /usr/local/bin/

# Option B: one-liner (after v1.0.0 is published)
bash <(curl -sSL https://github.com/clue-code/clue-code/releases/latest/download/install.sh)
```

Verify:

```bash
clue-code version
# clue-code 1.0.0 (abc1234, 2026-05-07)
```

---

### Step 2 — Set your API key

CLUE CODE supports 6 cloud providers. Set at least one:

```bash
# DeepSeek (recommended: best cost/performance ratio)
export DEEPSEEK_API_KEY="sk-..."

# Or OpenAI
export OPENAI_API_KEY="sk-..."

# Or Groq (fast inference, free tier available)
export GROQ_API_KEY="gsk_..."

# Add to your shell profile to persist:
echo 'export DEEPSEEK_API_KEY="sk-..."' >> ~/.zshrc
```

For local-only mode (no API key required):

```bash
# Install Ollama then pull a model
curl -fsSL https://ollama.com/install.sh | sh
ollama pull qwen2.5-coder:7b

clue-code mode local
```

---

### Step 3 — Run doctor

```bash
clue-code doctor
```

Expected output (green checkmarks for installed deps):

```
CLUE CODE — doctor
====================
Build:       clue-code 1.0.0 (abc1234, 2026-05-07)
OS / arch:   darwin / arm64
Go runtime:  go1.25.0
Logical CPU: 12

System resources:
  ✓ RAM                   36.0 GB
  ✓ disk free             245.3 GB free

External dependencies:
  ✓ aider                 aider 0.82.0
  ⚠ ollama               not running (optional — start with `ollama serve`)
  ✓ network (deepseek)    api.deepseek.com:443 reachable
  ⚠ mlx (inference)      not found (optional, Apple Silicon only)
  ✗ python3               NOT found  — required for LoRA pipeline (Phase 5+)
  ✓ git                   found at /usr/bin/git
```

Fix any `✗` items before continuing. `⚠` items are optional.

---

### Step 4 — Test a skill

```bash
# Dry-run autopilot to confirm skill engine works without executing
clue-code skill run autopilot --dry-run

# Run a real skill
clue-code skill run autopilot "refactor the auth module for clarity"
```

Skill files are loaded from `./skills/` (project) or the binary's sibling
`skills/` directory. OMC skill files are 100% compatible — copy them as-is.

---

### Step 5 — Import OMC project memory (optional)

If you have existing OMC state (`.omc/` directory), import it:

```bash
clue-code import-omc /path/to/project/.omc/
```

This copies:
- `.omc/project-memory.json` → `.clue-code/project-memory.json`
- `.omc/notepad.md` → `.clue-code/notepad.md`
- `.omc/plans/` → `.clue-code/plans/`

Agent and skill Markdown files can be copied directly — the format is identical.

---

## Hooks migration

OMC uses `settings.json` for hooks. CLUE CODE uses `~/.clue-code/hooks.yaml`.

**OMC (settings.json):**
```json
{
  "hooks": {
    "PreToolUse": [{ "matcher": "Bash", "hooks": [{"type": "command", "command": "echo pre"}] }]
  }
}
```

**CLUE CODE (hooks.yaml):**
```yaml
hooks:
  PreToolUse:
    - matcher: Bash
      command: echo pre
  PostToolUse:
    - matcher: Write
      command: echo post
```

Run `clue-code hooks list` to verify loaded hooks.

---

## Mode differences

| Concern | OMC | CLUE CODE |
|---|---|---|
| Requires Claude Code | Yes (runs inside it) | No (standalone binary) |
| Model providers | Anthropic only | 6 providers + local |
| Local model support | No | Yes (Ollama + MLX-LM) |
| Agent execution | Claude sub-agents | Go goroutines + Aider |
| Skill files | `skills/*.md` | `skills/*.md` (same) |
| Agent files | `agents/*.md` | `agents/*.md` (same) |
| State store | MCP tools | `clue-code state` CLI |
| Hook events | Claude Code hooks | Same event names |

---

## Troubleshooting

- **`clue-code skill run` not found**: ensure binary is in PATH (`which clue-code`).
- **`import-omc` not found**: feature available in v1.0.0+, build from main.
- **Agent not loading**: check `CLUE_CODE_AGENTS_DIR` or run from project root.
- **Model errors**: run `clue-code doctor` to confirm API key and connectivity.

Found a bug or missing mapping? Open an issue:
https://github.com/clue-code/clue-code/issues

Join the discussion:
https://github.com/clue-code/clue-code/discussions
