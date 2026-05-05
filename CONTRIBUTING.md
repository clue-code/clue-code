# Contributing to CLUE CODE

Thank you for considering a contribution. CLUE CODE is open-source under the Apache 2.0 license, and we welcome issues, pull requests, agent/skill submissions, and feedback.

## Getting started

### Prerequisites

- Go 1.22 or later (`brew install go` on macOS)
- Git
- (Optional, for runtime use): Aider, Ollama or MLX-LM, LiteLLM

### Setup

```bash
git clone https://github.com/clue-code/clue-code.git
cd clue-code
go mod download
go build ./cmd/clue-code
./clue-code version
./clue-code doctor
```

## Tests

```bash
go test ./...        # all tests
go vet ./...         # static analysis
go build ./...       # compile check
```

PRs that break the build or fail tests will not be merged.

## Pull Requests

1. Fork the repository and create a feature branch from `main` (`git checkout -b feat/my-change`)
2. Keep changes focused: one PR = one logical change
3. Add tests for new code paths
4. Run `go fmt ./...` and `go vet ./...` before committing
5. Use [Conventional Commits](https://www.conventionalcommits.org/) format:
   - `feat:` new feature
   - `fix:` bug fix
   - `docs:` documentation only
   - `refactor:` code change that neither fixes a bug nor adds a feature
   - `test:` adding/fixing tests
   - `chore:` build/tooling/etc.
6. Open a PR against `main` with a clear description of what changed and why

## Code style

- Follow standard Go conventions (`gofmt`, `golint`, `go vet`)
- Keep functions small and single-purpose
- Prefer composition over inheritance
- Write godoc comments on exported types and functions
- Avoid premature abstraction

## Submitting an agent (for the future Hub)

Agents live in `agents/<name>.md` with frontmatter:

```yaml
---
name: my-agent
description: Short description of what the agent does
model: qwen3-coder:30b   # or any LiteLLM-supported model id
level: L1                # L0 (fast/cheap), L1 (standard), L2 (cloud), L3 (MoA critical)
---
```

Followed by the agent prompt body (Markdown). See `agents/executor.md` for a reference.

## Submitting a skill

Skills are higher-level workflows orchestrating multiple agents. They live in `skills/<name>/SKILL.md` (one folder per skill so resources can be co-located).

## Reporting bugs

Open a GitHub issue with:
- CLUE CODE version (`clue-code version`)
- OS / arch (`uname -a`)
- Doctor output (`clue-code doctor`)
- Steps to reproduce
- Expected vs actual behavior

## Getting help

- GitHub Discussions for design questions
- Discord (link in README) for chat
- Twitter/X: `@cluecode`

## License

By contributing to CLUE CODE you agree that your contributions are licensed under the Apache License 2.0.
