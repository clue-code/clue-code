# Changelog

All notable changes to CLUE CODE will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
