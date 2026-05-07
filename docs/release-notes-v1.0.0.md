# Release Notes — CLUE CODE v1.0.0

**Released:** 2026-05-07

This is the first stable release of CLUE CODE, the open-source multi-agent AI
orchestration OS for developers. It marks the completion of Phase 4 (12
sub-phases) and represents a production-ready baseline.

---

## Highlights

### 19 specialized agents

A full typed agent catalog covering every stage of the development lifecycle:

`explore` · `analyst` · `planner` · `architect` · `debugger` · `executor` ·
`verifier` · `tracer` · `security-reviewer` · `code-reviewer` · `test-engineer` ·
`designer` · `writer` · `qa-tester` · `scientist` · `document-specialist` ·
`git-master` · `code-simplifier` · `critic`

Each agent ships with a Markdown definition file in `agents/` and is
independently loadable at runtime.

### 6 orchestrated skills

High-level workflows that coordinate multiple agents automatically:

| Skill | Description |
|---|---|
| `autopilot` | Full autonomous execution from idea to working code |
| `ralph` | Self-referential loop until task completion |
| `ultrawork` | Parallel execution engine for high-throughput tasks |
| `team` | N coordinated agents on a shared task list |
| `ccg` | Claude-Codex-Gemini tri-model orchestration |
| `ralplan` | Consensus planning with gating before execution |

### 6 model providers

Switch between providers with a single env var:

- **DeepSeek** V3.2 / R1 (recommended for cost)
- **OpenAI** GPT-4o / o3
- **Groq** (low-latency inference)
- **OpenRouter** (250+ models via LiteLLM)
- **Ollama** (local, any GGUF model)
- **MLX-LM** (Apple Silicon native, fastest local inference)

### 3 runtime modes

```bash
clue-code mode local    # 100% on-device, $0, max privacy
clue-code mode cloud    # API-backed, works on any laptop
clue-code mode hybrid   # smart cost-aware routing (default)
```

### Team primitives (Phase 4.3)

- NDJSON transport (in-process + subprocess)
- DAG scheduler with topological ordering
- Bounded mailbox (256 msgs), crash-resume via journal
- Stalled-task detector with clock injection for testing
- CLI: `clue-code team list | inspect | tail | demo`
- Forkbomb protection: `MaxTeamWorkers=20`, depth cap via `CLUE_CODE_TEAM_DEPTH`

### Token engine (Phase 4.6)

- Per-tier token budgets with overflow detection
- Session-level usage aggregation
- `clue-code tokens` subcommand with table output

### TUI (Phase 4.4)

- Bubble Tea terminal UI, build-tag gated (`-tags=tui`)
- Separate `clue-code-tui` binary in releases
- fsnotify live-reload for agent/skill edits

### Hook system (Phase 4.1)

- `PreToolUse` / `PostToolUse` / `Stop` event hooks
- YAML configuration at `~/.clue-code/hooks.yaml`
- Compatible with oh-my-claudecode hook event names

### State and memory (Phase 4.2)

- Persistent key/value store: `clue-code state get|set|list-active`
- Project memory: `.clue-code/project-memory.json`
- Session notepad: `.clue-code/notepad.md`

### Doctor (Phase 4.12)

Extended health check covering:

- RAM (sysctl / /proc/meminfo), disk free space
- Ollama API version (2s timeout)
- Network reachability to api.deepseek.com:443
- MLX inference binary or Python package
- All loaded agents with count

### Release infrastructure (Phase 4.12)

- `.goreleaser.yml`: 4 binaries (darwin/linux × amd64/arm64) × 2 tags
- Cosign keyless signing via Sigstore OIDC
- `scripts/install.sh`: cross-platform one-liner with SHA256 verification
- GitHub Actions release workflow (tag-triggered)

---

## What changed since the last dev build

All 12 sub-phases of Phase 4 are merged into this release:

| Phase | Feature |
|---|---|
| 4.1 | Hook system + state primitives |
| 4.2 | Persistent memory + notepad |
| 4.3 | Team primitives (DAG, journal, transport) |
| 4.4 | Bubble Tea TUI (build-tag gated) |
| 4.5 | MoA (Mixture-of-Agents) aggregator |
| 4.6 | Token engine (budgets, usage tracking) |
| 4.7 | DeepSeek + 5-provider model routing |
| 4.8 | Skill engine (run/list/inspect) |
| 4.9 | Agent hot-reload (fsnotify) |
| 4.10 | MCP tools bridge |
| 4.11 | Security hardening (rate limiting, sandboxing) |
| 4.12 | Polish + release infra (this release) |

---

## Breaking changes

None. This is the first stable release; there are no prior stable APIs to break.

---

## Migration from oh-my-claudecode

See [docs/migration-from-omc.md](migration-from-omc.md) for a complete
step-by-step guide, including concept mapping and hooks migration.

---

## Installation

```bash
# Build from source
git clone https://github.com/clue-code/clue-code.git
cd clue-code
go build -o clue-code ./cmd/clue-code
sudo mv clue-code /usr/local/bin/

# One-liner (after binary release is published)
bash <(curl -sSL https://github.com/clue-code/clue-code/releases/latest/download/install.sh)

# Verify
clue-code doctor
```

---

## Hardware requirements

| Configuration | Use case |
|---|---|
| MacBook Pro M-series 16 GB+ | Cloud or hybrid mode |
| MacBook Pro M-series 32 GB+ | Local mode (7B models), recommended sweet spot |
| Mac Studio 64 GB+ | Local mode (30B+ models) |
| Linux + NVIDIA 24 GB+ | Local mode (any GPU-accelerated model) |
| Any laptop | Cloud mode (no local model required) |

---

## Acknowledgments

CLUE CODE builds on exceptional open-source foundations:

- [Aider](https://aider.chat) — edit engine and repo-map
- [LiteLLM](https://github.com/BerriAI/litellm) — unified model proxy
- [MLX](https://github.com/ml-explore/mlx) — Apple Silicon inference
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Cobra](https://github.com/spf13/cobra) — CLI patterns
- [HdrHistogram](https://github.com/HdrHistogram/hdrhistogram-go) — latency measurement
- [Sigstore / cosign](https://sigstore.dev) — supply-chain signing
- [GoReleaser](https://goreleaser.com) — release automation

The agent and skill design draws inspiration from the
[oh-my-claudecode](https://github.com/clue-code/oh-my-claudecode) project.

---

## Feedback and contributions

- Issues: https://github.com/clue-code/clue-code/issues
- Discussions: https://github.com/clue-code/clue-code/discussions
- Contributing guide: [CONTRIBUTING.md](../CONTRIBUTING.md)
