# Changelog

All notable changes to CLUE CODE will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Phase 4.3 — Team primitives

#### Added
- feat(team): NDJSON transport (inproc + subprocess), envelope v=1 — `internal/team/transport.go`, `inproc.go`, `subprocess.go`
- feat(team): scheduler DAG (topological), bounded mailbox 256, crash-resume — `internal/team/task.go`, `journal.go`
- feat(team): stalled detector + panic recovery (clock injection) — `internal/team/stalled.go`
- feat(team): CLI `clue-code team list|inspect|tail|demo` + `team-worker` subcommand — `cmd/clue-code/team.go`, `team_worker.go`
- feat(team): forkbomb caps (`MaxTeamWorkers=20`, depth=1 via `CLUE_CODE_TEAM_DEPTH`) — `internal/team/team.go`
- feat(team): journal torn-tail recovery + cache rebuild from journal — `internal/team/journal.go`
- test(team): p99 `SendMessage` < 1 ms (HdrHistogram) — `internal/team.TestSendMessage_P99Latency`
- skills: ultrawork + team re-enabled (was DEFERRED post-Phase 4.6, now wired to `internal/team` API)
- docs: `docs/team-transport.md` — wire format, journal layout, CLI operator guide, acceptance checklist D1-D12

### Added
- Initial repository scaffolding (Phase 0 — Fondations)
- Go module structure with `cmd/`, `internal/`, `agents/`, `skills/`, `hooks/`
- CLI entry-point (`clue-code` binary) with `version` and `doctor` commands
- Orchestrator skeleton: agent registry + router stub
- Configuration system (YAML + env override) with sensible defaults for M-series Macs
- First agent ported from OMC: `executor` (adapted to local model `qwen3-coder:30b`)
- Install script (`scripts/install.sh`) with OS detection and `--dry-run` mode
- Apache 2.0 LICENSE
- Contributor Covenant 2.1 Code of Conduct
- Contribution guide with test/PR/style sections
- GitHub Actions CI (build, vet, test) on macOS + Linux, Go 1.22 + 1.23
- Issue and Pull Request templates
- High-level architecture documentation

### Architecture decisions
- Reuse Aider as edit engine (sub-process), not reimplemented from scratch
- MLX-LM as primary local inference (Apple Silicon) with Ollama fallback
- LiteLLM as cloud model proxy (250+ providers unified)
- Go for the orchestrator core (single binary, no Python deps)
- Apache 2.0 license for community-friendly distribution

## [0.1.0-dev] - TBD

Initial development build. Not yet functional end-to-end.

[Unreleased]: https://github.com/clue-code/clue-code/compare/v0.1.0...HEAD
[0.1.0-dev]: https://github.com/clue-code/clue-code/releases/tag/v0.1.0-dev
