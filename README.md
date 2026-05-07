# CLUE CODE

> **CLUE CODE — multi-agent AI coding OSS. Local-first. Privacy-first. Open-source.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25%2B-00ADD8.svg)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/clue-code/clue-code.svg)](https://github.com/clue-code/clue-code/releases)

## Quickstart (30 seconds)

```bash
# Build from source (binary releases available after v1.0.0 tag)
git clone https://github.com/clue-code/clue-code.git && cd clue-code
go build -o clue-code ./cmd/clue-code && sudo mv clue-code /usr/local/bin/

# Then chat:
clue-code chat "hello"
```

Once the v1.0.0 binary is published, the one-liner will be:

```bash
bash <(curl -sSL https://github.com/clue-code/clue-code/releases/latest/download/install.sh) && clue-code chat "hello"
```

> Demo: see `demo.cast` (coming soon)

---

## Why CLUE CODE

| Concern | Claude Code | Cursor | **CLUE CODE** |
|---|---|---|---|
| Cost / month | $200-500 | $20 | **$0-15** |
| Code privacy | Cloud-only | Cloud-only | **Local-first** |
| Open-source | No | No | **Yes (Apache 2.0)** |
| Multi-agent typed | Yes (1 provider) | Limited | **19 agents, 6 providers** |
| Self-hosted | No | No | **Yes** |
| Mode switch | No | No | **local / cloud / hybrid** |

---

## Modes

```bash
clue-code mode local    # 100% on your machine — $0, privacy max
clue-code mode cloud    # via APIs (DeepSeek, OpenAI, Groq, OpenRouter)
clue-code mode hybrid   # smart cost-aware routing (default)
```

Switch at any time. Settings persist in `~/.clue-code/config.yaml`.

---

## Skills

High-level workflows that coordinate multiple agents automatically:

| Skill | What it does |
|---|---|
| `autopilot` | Full autonomous execution from idea to working code |
| `ralph` | Self-referential loop — keeps going until the task is done |
| `team` | Spin up N coordinated agents on a shared task list |
| `ultrawork` | Parallel execution engine for high-throughput tasks |
| `ccg` | Claude-Codex-Gemini tri-model orchestration |
| `ralplan` | Consensus planning with gating before execution |

```bash
clue-code skill list
clue-code skill run autopilot "refactor auth module for clarity"
clue-code skill run ralph "fix all failing tests"
```

---

## Architecture in 10 seconds

```
┌─────────────────────────────────────────────┐
│ CLI: clue-code (Go binary, CGO=0)           │
└────────────────┬────────────────────────────┘
                 ▼
┌─────────────────────────────────────────────┐
│ Orchestrator                                │
│  • Router (4-tier cost-aware)               │
│  • Agent registry (19 typed agents)         │
│  • Skill engine (autopilot/ralph/team/…)    │
│  • Hook system + state + memory             │
│  • MoA aggregator (multi-model voting)      │
│  • Token engine (budgets + usage)           │
└────────────────┬────────────────────────────┘
                 ▼
┌─────────────────────────────────────────────┐
│ Execution layer                             │
│  • Aider (edit + repo-map + git)            │
│  • LiteLLM (250+ model proxy)               │
│  • MLX-LM (Apple Silicon native)            │
└────────────────┬────────────────────────────┘
                 ▼
┌─────────────────────────────────────────────┐
│ Models                                      │
│  Local: Qwen3-Coder 7B/30B, DeepSeek-R1     │
│  Cloud: DeepSeek V3.2, Groq, OpenRouter     │
└─────────────────────────────────────────────┘
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the deep dive.

---

## What's in v1.0.0

- **19 agents** — full typed catalog (executor, planner, architect, debugger, …)
- **6 skills** — autopilot, ralph, ultrawork, team, ccg, ralplan
- **6 model providers** — DeepSeek, OpenAI, Groq, OpenRouter, Ollama, MLX-LM
- **3 runtime modes** — local / cloud / hybrid
- **Team primitives** — DAG scheduler, NDJSON transport, journal crash-resume
- **Token engine** — per-tier budgets, session usage tracking
- **TUI** — Bubble Tea terminal UI (`clue-code-tui` binary, `-tags=tui`)
- **Hook system** — PreToolUse / PostToolUse / Stop hooks via YAML
- **Doctor** — RAM, disk, Ollama, network, MLX health checks
- **Release infra** — goreleaser, cosign keyless signing, install.sh

See [`CHANGELOG.md`](CHANGELOG.md) and [`docs/release-notes-v1.0.0.md`](docs/release-notes-v1.0.0.md).

---

## Hardware recommendations

- **MacBook Pro M-series, 32 GB+** — recommended sweet spot (local + cloud)
- Mac Studio / Mac mini 64 GB+ — local 30B+ models
- Linux + NVIDIA 24 GB+ (RTX 4090, A6000) — GPU local inference
- Any laptop in **cloud mode** (no local model required)

---

## The CLUE Ecosystem

CLUE CODE is the developer-tooling brick of the broader **CLUE** product family:

- **CLUE MONEY** — personal finance app (mobile)
- **CLUE ENTERPRISE** — HCM SaaS platform (web)
- **CLUE INTELLIGENCE** — data & analytics layer
- **CLUE CODE** — multi-agent AI orchestration OS *(you are here)*

---

## Migrating from oh-my-claudecode?

See [`docs/migration-from-omc.md`](docs/migration-from-omc.md) — 5 steps, under 10 minutes.

---

## Contributing

Issues, PRs, agent submissions, and skill contributions are all welcome.
See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).

## Acknowledgements

Built on: [Aider](https://aider.chat) · [LiteLLM](https://github.com/BerriAI/litellm) · [MLX](https://github.com/ml-explore/mlx) · [Bubble Tea](https://github.com/charmbracelet/bubbletea) · [GoReleaser](https://goreleaser.com) · [Sigstore](https://sigstore.dev)

---

*Built for developers who value sovereignty, productivity, and privacy.*
