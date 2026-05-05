# CLUE CODE

> **The open-source multi-agent AI orchestration OS for developers and teams. Local-first. Privacy-first. Evolving at the weights level.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25%2B-00ADD8.svg)](https://go.dev)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)](#status)

CLUE CODE replaces (or augments) Claude Code, Cursor, and GitHub Copilot with **19 specialized agents**, **8 orchestrated workflows**, and **3 runtime modes** (local, cloud, hybrid) — running on **open-source models** (Qwen3-Coder, DeepSeek V3.2/R1, Llama 4) with **continual learning** at the weights level (LoRA).

## Why CLUE CODE

| Concern | Claude Code | Aider | Cursor | **CLUE CODE** |
|---------|-------------|-------|--------|---------------|
| Cost / month | $200-500 | $5-50 | $20 | **$0-15** |
| Code privacy | Cloud-only | Cloud-only by default | Cloud-only | **Local-first** |
| Open-source | ❌ | ✅ | ❌ | ✅ Apache 2.0 |
| Multi-agent typed | ✅ | ❌ | ⚠️ Limited | ✅ 19 agents |
| Continual learning | ❌ | ❌ | ❌ | ✅ Weekly LoRA |
| Self-hosted | ❌ | ✅ | ❌ | ✅ |
| Mode switch (local/cloud) | ❌ | ⚠️ | ❌ | ✅ 3 modes |

## The CLUE Ecosystem

CLUE CODE is the developer-tooling brick of the broader **CLUE** product family:

- **CLUE MONEY** — personal finance app (mobile)
- **CLUE ENTERPRISE** — HCM SaaS platform (web)
- **CLUE INTELLIGENCE** — data & analytics layer
- **CLUE CODE** — multi-agent AI orchestration OS *(you are here)*

## Quick start

> **Status**: Alpha — actively developed. APIs may change before 1.0.

```bash
# One-liner install (planned, Phase 2):
# curl -fsSL https://cluecode.dev/install.sh | sh

# Manual install (current):
git clone https://github.com/clue-code/clue-code.git
cd clue-code
go build -o clue-code ./cmd/clue-code

./clue-code version
./clue-code doctor   # checks OS, arch, RAM, deps
```

## Architecture in 10 seconds

```
┌─────────────────────────────────────────────┐
│ CLI: clue-code (Go binary, 1 file)          │
└────────────────┬────────────────────────────┘
                 ▼
┌─────────────────────────────────────────────┐
│ Orchestrator (Go ~3000 LOC)                 │
│  • Router (4-tier cost-aware)               │
│  • Agent registry (19 typed agents)         │
│  • Skill engine (autopilot/ralph/ultrawork) │
│  • Hook system + state + memory             │
│  • MoA aggregator (multi-model voting)      │
└────────────────┬────────────────────────────┘
                 ▼
┌─────────────────────────────────────────────┐
│ Execution layer (reuses battle-tested OSS)  │
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

## Three runtime modes

```bash
clue-code mode local    # 100% on your machine — privacy max, $0
clue-code mode cloud    # 100% via APIs — works on any laptop
clue-code mode hybrid   # smart routing (default) — best of both
```

## Roadmap

- **Phase 1 — MVP** (in progress): install + 5 agents + autopilot
- **Phase 2 — Core skills**: ralph, ultrawork, ccg, team
- **Phase 3 — Smart routing**: 4-tier router + MoA + hot LoRA swap
- **Phase 4 — Hooks + state**: full hook system + persistent memory
- **Phase 5 — Continual learning** (opt-in): weekly LoRA + per-project models

See [`CHANGELOG.md`](CHANGELOG.md) and the milestones tab for details.

## Status

**Alpha — early development.** Not yet a daily driver replacement. Watch / star the repo for updates.

## Hardware recommendations

CLUE CODE is designed to run well on:

- **MacBook Pro M-series, 32 GB+ RAM** (target sweet spot)
- Mac Studio / Mac mini with 64 GB+
- Linux + NVIDIA GPU 24 GB+ (RTX 4090, A6000)
- Any laptop in **cloud mode** (no local model required)

## Contributing

We welcome issues, PRs, agent and skill submissions, and feedback. See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).

## Acknowledgements

CLUE CODE stands on the shoulders of giants. We gratefully reuse and integrate:

- [Aider](https://aider.chat) — edit engine and repo-map
- [LiteLLM](https://github.com/BerriAI/litellm) — unified model proxy
- [MLX](https://github.com/ml-explore/mlx) — Apple Silicon inference
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI (Phase 2+)

The agent and skill design draws inspiration from the **oh-my-claudecode** project, adapted for open-source models.

---

*Built for developers who value sovereignty, productivity, and privacy.*
