# Security Policy

## Supported versions

CLUE CODE is in alpha. Only `main` and the latest tagged release receive
security fixes during this phase. Pinned dependencies are kept current via
Dependabot.

| Version  | Supported          |
| -------- | ------------------ |
| `main`   | :white_check_mark: |
| latest tag | :white_check_mark: |
| older tags | :x:              |

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Instead, report via one of the following private channels:

1. **GitHub Security Advisory** (preferred):
   <https://github.com/clue-code/clue-code/security/advisories/new>
2. **Email**: `security@clue-code.dev` (PGP key on request).

We acknowledge receipt within **3 business days** and aim for a triaged
response within **7 business days**. Fix timelines depend on severity:

| Severity | Target fix window |
| -------- | ----------------- |
| Critical (RCE, privilege escalation, secret exposure) | 7 days |
| High (path traversal, injection, auth bypass) | 30 days |
| Medium (info disclosure, DoS) | 60 days |
| Low (hardening, defense-in-depth) | 90 days or next release |

## Coordinated disclosure

We follow **90-day coordinated disclosure**:

1. You report privately.
2. We acknowledge and triage.
3. We work with you on a fix; you can request CVE assignment.
4. Once a patch ships (or after 90 days, whichever is sooner), we publish a
   GitHub Security Advisory crediting you (unless you prefer anonymity).

## Scope

In scope:
- The `clue-code` Go binary and `internal/*` packages.
- The `agents/`, `skills/`, and `hooks/` directories.
- Build and CI configuration (`.github/workflows/`, `scripts/`).

Out of scope:
- Vulnerabilities in dependencies (please report upstream first; we will fast-track upgrades).
- Social engineering, physical attacks, or attacks against contributor accounts.
- Denial of service via resource exhaustion (CPU/RAM) on local machines — CLUE CODE is a local-first tool by design.
- Issues in third-party models (Qwen, DeepSeek, Llama) or model proxies (LiteLLM).

## Hardening commitments

- All commits to `main` go through pull requests with required CI checks and review.
- GitHub Actions are pinned to commit SHAs (not mutable tags).
- GitHub secret scanning + push protection are enabled.
- Dependabot watches Go modules and GitHub Actions for known CVEs.
- Releases will be signed with Sigstore once Phase 4 ships v0.2.0.

## What we will not do

- We will not threaten legal action against good-faith security researchers.
- We will not pay bounties during the alpha phase, but we will publicly
  credit you in the advisory and `CHANGELOG.md`.
- We will not require an NDA before discussing vulnerabilities.

## Hall of fame

Researchers who report valid vulnerabilities will be listed here (with their
permission).

_No reports yet._
