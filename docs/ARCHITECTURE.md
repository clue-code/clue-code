# CLUE CODE — Architecture

This document describes the high-level architecture of CLUE CODE, the responsibilities of each layer, and the rationale behind the design.

## Layered overview

```
┌─────────────────────────────────────────────────────────────┐
│  L8 — User interface                                         │
│       CLI (Go/Cobra), TUI (Bubble Tea, Phase 2+),            │
│       IDE plugins (VS Code/JetBrains, Phase 7+)              │
├─────────────────────────────────────────────────────────────┤
│  L7 — Skill engine                                           │
│       autopilot, ralph, ultrawork, ccg, team, ralplan, …     │
├─────────────────────────────────────────────────────────────┤
│  L6 — Hook system                                            │
│       SessionStart, PreToolUse, PostToolUse,                 │
│       UserPromptSubmit, Stop                                 │
├─────────────────────────────────────────────────────────────┤
│  L5 — Orchestrator core                                      │
│       Router (4-tier), Agent registry, MoA aggregator,       │
│       State manager, Memory manager, Telemetry               │
├─────────────────────────────────────────────────────────────┤
│  L4 — Token engine                                           │
│       Counter, Cache (3 levels), Optimizer, Analytics,       │
│       Guardrails (budget, alerts)                            │
├─────────────────────────────────────────────────────────────┤
│  L3 — Execution adapters                                     │
│       Aider sub-process, MCP bridge, OpenCode tools          │
├─────────────────────────────────────────────────────────────┤
│  L2 — Model proxy                                            │
│       LiteLLM (cloud), MLX-LM / llama.cpp (local)            │
├─────────────────────────────────────────────────────────────┤
│  L1 — Models                                                 │
│       Local: Qwen3-Coder 7B/30B, DeepSeek-R1 32B (MLX/GGUF)  │
│       Cloud: DeepSeek V3.2/R1, Groq Llama 4, OpenRouter      │
├─────────────────────────────────────────────────────────────┤
│  L0 — Storage & state                                        │
│       .clue-code/ (state, sessions, plans), Postgres/Redis   │
│       (cloud only, Phase 3+)                                 │
└─────────────────────────────────────────────────────────────┘
```

## Layer responsibilities

### L8 — User interface
The user-facing surface. The CLI is the canonical entry point. A TUI built on Bubble Tea provides an interactive REPL. IDE plugins are deferred to Phase 7+.

### L7 — Skill engine
Skills are higher-level workflows that compose multiple agents and tool calls. Examples:

- `autopilot`: end-to-end idea → spec → plan → code → tests → validation
- `ralph`: self-iterative loop until a goal is reached
- `ultrawork`: parallel execution engine for independent tasks
- `ccg`: tri-model consensus (e.g. DeepSeek + Qwen + Llama)
- `team`: N coordinated agents on a shared task list

Skills are defined in `skills/<name>/SKILL.md` and loaded dynamically.

### L6 — Hook system
Hooks let users (and the system) inject behavior at well-defined lifecycle points. They are configured in `~/.config/clue-code/hooks.yaml` and run as shell commands.

Hook points:
- `SessionStart` — when a session begins
- `PreToolUse` — before any tool call
- `PostToolUse` — after a tool call completes
- `UserPromptSubmit` — when the user submits a prompt
- `Stop` — at session end

### L5 — Orchestrator core
The brain of CLUE CODE.

- **Router**: classifies each task by complexity, criticality, and required capabilities, then dispatches to the right agent + model tier.
- **Agent registry**: loads agents from `agents/*.md`, parses YAML frontmatter, exposes lookup by name.
- **MoA aggregator**: for critical decisions, runs N models in parallel and aggregates with a vote or synthesis layer.
- **State manager**: persists session state, plans, and notepads under `.clue-code/state/`.
- **Memory manager**: loads project-specific LoRAs and context (Phase 5+).
- **Telemetry** (opt-in): anonymous usage metrics for product improvement.

### L4 — Token engine
Differentiator vs Claude Code. Provides:

- Real-time token counter per agent/task/project
- 3-level cache (prompt, tool result, embedding)
- Compression and summarization optimizers
- Analytics dashboards (CLI + Web)
- Budget guardrails and alerts

### L3 — Execution adapters
Wraps battle-tested external tools rather than reimplementing them.

- **Aider** (sub-process): edit engine, repo-map, git integration
- **MCP bridge**: translates MCP server protocol to OpenAI-style tool calls so non-Anthropic models can use existing MCP servers
- **OpenCode tools**: optional integration for richer tooling

### L2 — Model proxy
Unifies access to 250+ models behind a single OpenAI-compatible interface.

- **LiteLLM** for cloud routing (DeepSeek, Together, Groq, OpenRouter, etc.)
- **MLX-LM** for Apple Silicon native inference (preferred on M-series)
- **llama.cpp / Ollama** as cross-platform fallback

### L1 — Models
Open-source models are first-class. Recommended defaults per tier:

| Tier | Model | Use case |
|------|-------|----------|
| L0 | Qwen3-Coder 7B Q8 (local) | Read/Glob/lookup, completion |
| L1 | Qwen3-Coder 30B Q4 (local) | Edit standard, refactor |
| L1-bis | DeepSeek-R1-Distill 32B Q4 (local) | Local reasoning |
| L2 | DeepSeek V3.2 (cloud) | Architecture, complex multi-file |
| L2-bis | Groq Llama-4 Scout (cloud) | Speed-critical paths |
| L3 | MoA (R1 + V3.2 + Qwen3) | Security, critical decisions |

### L0 — Storage & state
Local-first. State lives under `.clue-code/` per project:

```
.clue-code/
├── state/          # session state, locks
├── sessions/       # per-session transcripts
├── plans/          # generated plans (autopilot, ralplan)
├── notepad.md      # working memory
└── memory.json     # project memory
```

Cloud-mode users get optional sync to a managed backend (Postgres + Redis).

## Three runtime modes

CLUE CODE ships as a single binary supporting three modes via `clue-code mode <mode>`:

| Mode | Description | Default for |
|------|-------------|-------------|
| `local` | 100% local inference, no network | Privacy-sensitive code |
| `cloud` | 100% cloud APIs, no local models | Travel / non-Mac machines |
| `hybrid` | Smart routing — local + cloud | Daily driver (default) |

The same agents, skills, and hooks work across all three modes — only the model dispatch changes.

## Design principles

1. **Reuse over reinvention** — Aider, LiteLLM, MLX-LM are great; we don't rebuild them.
2. **Local-first** — your code never leaves your machine unless you choose otherwise.
3. **Open-core** — the orchestrator is Apache 2.0 forever; cloud services and Hub are proprietary.
4. **Single binary** — one Go binary, no Python deps for the core (Python only for optional LoRA training).
5. **Multi-model from day one** — never lock to a single provider.
6. **Explicit cost** — token counts and dollar costs are visible, not hidden.
7. **Graceful degradation** — if cloud is down, fall back to local; if local is OOM, fall back to cloud.

## What is NOT in scope

- Re-implementing Aider's edit engine (we use it as a sub-process)
- Re-implementing LiteLLM's model proxy
- A custom tokenizer (we use `tiktoken-go` ports)
- A new MCP server protocol (we bridge the existing one)

## Future roadmap (post-Phase 5)

- IDE plugins (VS Code, JetBrains)
- Mobile companion app
- Cloud-hosted LoRA training service
- Hub Marketplace (agents/skills with revenue share)
- Enterprise SSO/SAML + audit + compliance (SOC 2, ISO 27001)
- Internationalization (10+ languages)

---

*This is a living document. Updates land in `docs/ARCHITECTURE.md` as the design evolves.*
