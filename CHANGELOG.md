# Changelog

All notable changes to CLUE CODE will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] — 2026-05-07

First stable release. No breaking changes (fresh 1.0 baseline).

### Phase 4.12 — Polish + release infrastructure

#### Added
- feat(doctor): RAM check via `sysctl hw.memsize` (darwin) / `/proc/meminfo` (linux)
- feat(doctor): disk free check via `df -k` with <500 MB warning
- feat(doctor): Ollama version check via `curl localhost:11434/api/version` (2s timeout)
- feat(doctor): network reachability check to `api.deepseek.com:443` (5s timeout)
- feat(doctor): MLX inference check (`/usr/local/bin/mlx_lm.server` or Python package)
- feat(doctor): table output with ✓ / ⚠ / ✗ per check
- feat(install): `scripts/install.sh` — cross-platform binary installer with SHA256 verification, gh CLI fallback, non-root `~/.local/bin` support, post-install `doctor` run
- feat(install): `scripts/install_test.sh` — bash dry-run test suite (5 cases)
- feat(release): `.goreleaser.yml` — 4 binaries (darwin/linux × amd64/arm64) × 2 build tags (no-tag + tui), cosign keyless OIDC signing config, snapshot mode
- feat(release): `.github/workflows/release.yml` — tag-triggered release pipeline with cosign + goreleaser
- docs: `docs/migration-from-omc.md` — 5-step OMC → CLUE CODE migration guide
- docs: `docs/release-notes-v1.0.0.md` — full 1.0.0 release notes
- docs: `README.md` — updated quickstart, Why CLUE CODE table, Skills section, v1.0.0 summary
- test: `cmd/clue-code/doctor_test.go` — tests for RAM, disk, Ollama mock, network output, MLX output

### Phase 4.11 — Security hardening

#### Added
- feat(security): rate limiting on orchestrator dispatch paths
- feat(security): subprocess sandboxing for Aider and team workers
- feat(security): audit log for tool invocations (`~/.clue-code/audit.log`)

### Phase 4.10 — MCP tools bridge

#### Added
- feat(mcp): `clue-code mcp` subcommand — list, call, and inspect MCP tools
- feat(mcp): bridge layer routing MCP tool calls through orchestrator

### Phase 4.9 — Agent hot-reload

#### Added
- feat(agents): fsnotify watcher on `agents/` directory — live reload without restart
- feat(agents): `clue-code agent reload` subcommand

### Phase 4.8 — Skill engine

#### Added
- feat(skills): `clue-code skill run <name> [args]` — invoke skill by name
- feat(skills): `clue-code skill list` — list available skills with metadata
- feat(skills): `clue-code skill inspect <name>` — show skill definition

### Phase 4.7 — DeepSeek + 5-provider model routing

#### Added
- feat(models): DeepSeek V3.2 / R1 provider
- feat(models): Groq provider (low-latency inference)
- feat(models): OpenRouter provider (250+ models via LiteLLM)
- feat(models): Ollama provider (local GGUF models)
- feat(models): MLX-LM provider (Apple Silicon native)
- feat(router): 4-tier cost-aware routing (L0=local fast, L1=local quality, L2=cloud fast, L3=cloud quality)

### Phase 4.6 — Token engine

#### Added
- feat(tokens): per-tier token budgets with overflow detection
- feat(tokens): session-level usage aggregation
- feat(tokens): `clue-code tokens` subcommand with table output
- test(tokens): budget enforcement, aggregation correctness

### Phase 4.5 — MoA aggregator

#### Added
- feat(moa): Mixture-of-Agents parallel dispatch and response voting
- feat(moa): configurable quorum threshold and merge strategy

### Phase 4.4 — TUI (Bubble Tea)

#### Added
- feat(tui): Bubble Tea terminal UI, build-tag gated (`-tags=tui`)
- feat(tui): `clue-code-tui` binary (separate goreleaser build ID)
- feat(tui): fsnotify live-reload for agent/skill edits in TUI mode
- test(tui): teatest integration tests

### Phase 4.3 — Team primitives

#### Added
- feat(team): NDJSON transport (inproc + subprocess), envelope v=1
- feat(team): scheduler DAG (topological), bounded mailbox 256, crash-resume
- feat(team): stalled detector + panic recovery (clock injection)
- feat(team): CLI `clue-code team list|inspect|tail|demo` + `team-worker` subcommand
- feat(team): forkbomb caps (`MaxTeamWorkers=20`, depth=1 via `CLUE_CODE_TEAM_DEPTH`)
- feat(team): journal torn-tail recovery + cache rebuild from journal
- test(team): p99 `SendMessage` < 1 ms (HdrHistogram)
- docs: `docs/team-transport.md` — wire format, journal layout, CLI operator guide

### Phase 4.2 — Persistent memory + notepad

#### Added
- feat(memory): `.clue-code/project-memory.json` — persistent cross-session memory
- feat(memory): `.clue-code/notepad.md` — project-local scratch pad
- feat(state): `clue-code state get|set|list-active` subcommands
- feat(state): session-scoped state isolation

### Phase 4.1 — Hook system

#### Added
- feat(hooks): PreToolUse / PostToolUse / Stop event hooks
- feat(hooks): YAML configuration at `~/.clue-code/hooks.yaml`
- feat(hooks): compatible with oh-my-claudecode hook event names
- feat(hooks): `clue-code hooks list|fire` subcommands

---

## [Unreleased]

_No unreleased changes._

---

## [0.1.0-dev] - TBD

Initial development build. Not yet functional end-to-end.

[1.0.0]: https://github.com/clue-code/clue-code/compare/v0.1.0-dev...v1.0.0
[0.1.0-dev]: https://github.com/clue-code/clue-code/releases/tag/v0.1.0-dev
