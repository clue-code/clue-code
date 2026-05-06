# CLUE CODE — Master Plan Phase 4.5 → 1.0

**Status:** APPROVED 2026-05-06 (post Phase-4.2 ship)
**Scope:** Tier 1 — OSS Core 1.0, daily-driver replacement for OMC.
**Timeline:** ~14 weeks calendar (~3.5 months).
**Predecessor:** [docs/PHASE_4_PLAN.md](PHASE_4_PLAN.md) (locked plan covering 4.1–4.4).

---

## §0. Scope & status

### 0.1 What's done (2026-05-06)

| Phase | Track | Status |
|---|---|---|
| Phase 0 | Scaffolding (CLI shell, agents, skills as MD, CI, docs) | ✅ |
| Phase 4.1 | B — State primitives | ✅ merged on main |
| Phase 4.2 | A + A.5 — Hooks + skillrunner | ✅ merged `d6e001b` |

107 tests pass race-clean across 8 packages. Build matrix clean (4 tag combos: no-tag, tui, test, tui+test).

### 0.2 What remains (this plan)

| Sub-phase | Track | Estimate | Acceptance | Status |
|---|---|---|---|---|
| **4.5** | **Model proxy (L2)** | **1.5 sem** | **F1-F5** | **STARTING NOW** |
| 4.6 | Skill execution real | 2 sem | G1-G6 | NOT STARTED |
| 4.7 | Agent invocation | 1.5 sem | H1-H4 | NOT STARTED |
| 4.8 | Token engine (L4) | 1 sem | I1-I5 | NOT STARTED |
| 4.3 | Team primitives (Track D) | 2 sem | D1-D12 | NOT STARTED — see PHASE_4_PLAN.md §3.D |
| 4.9 | Aider integration (L3) | 1.5 sem | J1-J3 | NOT STARTED |
| 4.10 | MCP bridge (L3) | 1.5 sem | K1-K3 | NOT STARTED |
| 4.11 | 3 runtime modes | 1 sem | L1-L4 | NOT STARTED |
| 4.4 | TUI full (Track C) | 1.5 sem | C1-C7 | NOT STARTED — see PHASE_4_PLAN.md §3.C |
| 4.12 | Polish + 1.0 release | 1 sem | M1-M3 | NOT STARTED |

**Total remaining: 14.5 weeks of focused work** with intentional 0.5 sem buffer.

### 0.3 Out of scope for Tier 1 (deferred to Tier 2 post-1.0)

- Hub Marketplace (paid agents/skills, revenue share)
- LoRA training pipeline (Phase 5 in original README)
- Cloud Sync / multi-device session
- IDE plugins (VS Code, JetBrains)
- Mobile companion app
- Enterprise SSO/SAML, SOC 2

---

## §1. RALPLAN-DR Summary

### 1.1 Principles (carried from PHASE_4_PLAN.md, refined)

1. **Local-first.** Every feature must work on M1/8GB without external services in `mode local`.
2. **Single static binary.** No CGo, no Python in core (Python only for optional Phase 5+ LoRA training).
3. **Graceful degradation.** Cloud down → local fallback. Local OOM → cloud fallback. Aider absent → direct edit.
4. **Composable core.** TUI/CLI/skills share the same `internal/*` primitives via constructors.
5. **Append-mostly state.** Journals append-only; index files derived from journal.
6. **NEW — Streaming-first model I/O.** Every model call streams; never wait for full response before display.
7. **NEW — Cost-aware by default.** Token counter wired into every model call; budget guardrails on by default.

### 1.2 Decision drivers (top 3)

1. Daily-driver-quality on M1/16GB by week 14.
2. CGo-free static binary across darwin-{arm64,amd64} + linux-{amd64,arm64}.
3. Cost ≤ $0.50/day for typical user dogfood (achievable with DeepSeek V3.2 + cache).

### 1.3 Default model choice

**DeepSeek V3.2** via DeepSeek API.
- Pricing: $0.27/M input, $1.10/M output (vs Anthropic Sonnet $3/M input).
- Context: 64K.
- OpenAI-compatible endpoint: simplifies Phase 4.5 client.
- Tool use: native function-calling support.
- Strong code generation per LiveCodeBench v6 (Mar 2026 data).

Fallbacks: Anthropic Claude Sonnet 4.6 (via API), Groq Llama-4 Scout (speed), OpenRouter (multi-provider). Local: Qwen3-Coder via Ollama.

---

## §2. Phase 4.5 — Model proxy (L2) — DETAILED SPEC

### 2.1 Decision (ADR)

**Decision:** Implement a single `internal/model.Client` interface, with one provider per file. All providers speak streaming + non-streaming chat completion via OpenAI-compatible JSON over HTTP.

**Drivers:** Single binary, OSS LiteLLM-compatible wire format, easy provider extension.

**Alternatives considered:**
- LiteLLM as Python sub-process — eliminated: violates Principle 2 (no Python in core).
- Direct go-openai library only — eliminated: locks us to OpenAI shape; Anthropic uses different request body.
- Custom protocol — eliminated: OpenAI shape is the dominant LCD.

**Why chosen:** Native Go HTTP client, streaming via Server-Sent Events with `bufio.Scanner`, provider differences absorbed via small adapters. Cloud + local backends share the same interface.

**Consequences (good):** Forward-compat with any future OpenAI-compatible provider. Anthropic adapter handles their body shape. Ollama already speaks OpenAI shape.

**Consequences (bad):** Anthropic streaming SSE format differs slightly from OpenAI; need adapter logic. MLX-LM subprocess wrapping is unique.

**Follow-ups:** Phase 4.7 will add MoA (mixture of agents) on top of this client.

### 2.2 Public API

```go
// internal/model/types.go
package model

type Role string
const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

type Message struct {
    Role    Role   `json:"role"`
    Content string `json:"content"`
}

type ChatRequest struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    Stream      bool      `json:"stream,omitempty"`
    Temperature float64   `json:"temperature,omitempty"`
    MaxTokens   int       `json:"max_tokens,omitempty"`
}

type Chunk struct {
    Delta string // streamed token delta
    Done  bool   // final chunk marker
    Usage *Usage // populated on Done=true
}

type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

// internal/model/client.go
type Client interface {
    Chat(ctx context.Context, req ChatRequest) (Response, error)         // non-streaming
    ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error) // streaming
}

type Response struct {
    Content string
    Usage   Usage
}

var (
    ErrNoAPIKey       = errors.New("model: no API key configured")
    ErrModelNotFound  = errors.New("model: model id not found")
    ErrRateLimit      = errors.New("model: rate limit")
    ErrUpstream       = errors.New("model: upstream error")
)
```

### 2.3 Provider files (one per backend)

```
internal/model/
├── doc.go
├── types.go              — Message, ChatRequest, Chunk, Usage
├── client.go             — Client interface + factory NewClient(cfg)
├── config.go             — yaml.v3 parser for ~/.config/clue-code/config.yaml
├── http.go               — shared HTTP base (timeouts, retry, error mapping)
├── deepseek.go           — DeepSeek client (default cloud)
├── anthropic.go          — Claude client
├── groq.go               — Groq client
├── openrouter.go         — OpenRouter passthrough
├── ollama.go             — Ollama HTTP client (local)
├── mlx.go                — MLX-LM subprocess wrapper (Apple Silicon local)
└── *_test.go             — table-driven tests with httptest.NewServer
testdata/
├── deepseek-stream.sse   — golden SSE response
├── anthropic-stream.sse
└── ollama-stream.ndjson
```

### 2.4 Config file

`~/.config/clue-code/config.yaml`:

```yaml
default_model: deepseek/deepseek-chat
budget_usd_per_day: 5.0      # 4.8 will enforce; 4.5 records intent
models:
  - id: deepseek/deepseek-chat
    provider: deepseek
    endpoint: https://api.deepseek.com/v1
    api_key_env: DEEPSEEK_API_KEY
    max_tokens: 4096
  - id: anthropic/claude-sonnet-4-6
    provider: anthropic
    endpoint: https://api.anthropic.com/v1
    api_key_env: ANTHROPIC_API_KEY
    max_tokens: 8192
  - id: ollama/qwen3-coder:30b
    provider: ollama
    endpoint: http://localhost:11434/v1
    max_tokens: 8192
  - id: mlx/qwen3-coder-7b-q8
    provider: mlx
    bin: /opt/homebrew/bin/mlx_lm.server
    max_tokens: 4096
```

### 2.5 CLI

`cmd/clue-code/chat.go`:
- `clue-code chat <prompt>` — single-turn chat with default model, streams to stdout
- `clue-code chat --model=<id> <prompt>` — pick model
- `clue-code chat --no-stream <prompt>` — buffered output (for piping)
- `clue-code chat --json <prompt>` — emit chunks as NDJSON (for tooling)

`cmd/clue-code/main.go` adds `case "chat": runChat(args)`.

### 2.6 Acceptance criteria F1–F5

**F1 — Cloud chat round-trip.** GIVEN `DEEPSEEK_API_KEY` set in env, WHEN `clue-code chat "hello"` runs, THEN exits 0 within 10s, stdout contains a non-empty response. Verified by `internal/model.TestDeepSeekChat_Live` (skipped if no API key).

**F2 — Streaming display.** GIVEN streaming enabled (default), WHEN `clue-code chat "count 1 to 5"` runs, THEN stdout receives at least 3 separate writes (assertable via tee + write timestamps in `cmd/clue-code/chat_test.go`). Verified by `TestChat_StreamingChunks`.

**F3 — Ollama local fallback.** GIVEN `mode local` set in config AND Ollama running on localhost:11434 with `qwen3-coder:30b` pulled, WHEN `clue-code chat "hello"` runs, THEN no network call to *.deepseek.com, response from local model. Verified by `TestOllama_LocalOnly` with httptest stub.

**F4 — Model selection flag.** GIVEN multiple models in config, WHEN `clue-code chat --model=anthropic/claude-sonnet-4-6 "hello"` runs, THEN client invoked is anthropic, not deepseek. Verified by `TestModelFlag_Routing`.

**F5 — Missing API key.** GIVEN `default_model: deepseek/...` AND `DEEPSEEK_API_KEY` unset, WHEN `clue-code chat "hello"` runs, THEN exits 2 with stderr `model: no API key configured (DEEPSEEK_API_KEY): set it in your environment`. Verified by `TestNoAPIKey_ClearError`.

### 2.7 Phasing (within Phase 4.5)

```
Day 1-2  : Foundation (worker-1) — types + config + http base
Day 3-5  : Cloud providers (worker-2) — deepseek + anthropic + groq + openrouter
Day 3-5  : Local providers (worker-3, parallel with -2) — ollama + mlx
Day 6-8  : CLI integration (worker-4, depends on 1+2+3) — chat command + smoke tests
Day 9    : Verifier pass + bug fixes
Day 10   : PR + merge
```

Checkpoint: at end of Day 5, run F1+F2 with stub HTTP server before integrating CLI.

---

## §3. Phase 4.6 — Skill execution real (~2 sem)

### 3.1 Goal

Replace the test-seam `RunFunc` in `internal/skillrunner/engine.go` with a real executor that:
1. Loads SKILL.md body (Markdown after YAML frontmatter)
2. Renders it as a system prompt with context (skill args, project metadata)
3. Combines with user task → ChatRequest
4. Streams output to stdout
5. Persists transcript to `state.Store` for `clue-code state status <sid>`
6. Fires lifecycle hooks (already wired in 4.2)

### 3.2 Files

- `internal/skillrunner/run_real.go` (replace stub)
- `internal/skillrunner/template.go` — Go text/template for prompt rendering
- `internal/skillrunner/transcript.go` — turn-by-turn JSONL persistence
- `internal/skillrunner/run_real_test.go`
- 6 SKILL.md bodies retravaillés (or auto-generated from existing skills) — see §3.3

### 3.3 Skills to fully implement

| Skill | Behavior | Body source |
|---|---|---|
| `autopilot` | idea → spec → plan → code → tests pipeline | New body |
| `ralph` | self-iterative loop until goal | New body |
| `ultrawork` | parallel agents on shared task list (uses 4.3) | New body, depends 4.3 |
| `ccg` | tri-model consensus (Claude+Codex+Gemini) | New body, calls 3 models |
| `team` | wrap 4.3 team primitives | New body, depends 4.3 |
| `cancel` | terminate active skill cleanly | Simple terminal op |

### 3.4 Acceptance criteria G1–G6

- **G1** `clue-code skill run autopilot "build a hello world in Go"` produces compilable Go code.
- **G2** `clue-code skill run ralph "fix all lint issues"` loops until lint passes (or max-iter 10).
- **G3** Skill execution fires the 5 hooks in order (regression of E5 from 4.2, but with real model calls).
- **G4** `clue-code skill run X --` (no args) errors with usage.
- **G5** Ctrl-C during long skill returns within 200ms with Stop hook fired.
- **G6** Skill chaining respects `CLUE_CODE_SKILL_DEPTH` cap=4 (regression of E2 with real exec).

---

## §4. Phase 4.7 — Agent invocation (~1.5 sem)

### 4.1 Goal

Make the 19 markdown agents (`agents/*.md`) callable as primitives. The router exists since Phase 1; now wire actual model dispatch.

### 4.2 Files

- `internal/orchestrator/dispatch.go` — `Dispatch(ctx, agentName, task) (output string, err error)`
- `internal/orchestrator/moa.go` — Mixture of Agents aggregator (3 models in parallel + synthesis)
- `cmd/clue-code/agent.go` — `clue-code agent {list,run}`

### 4.3 Acceptance criteria H1–H4

- **H1** `clue-code agent run executor "fix this code"` returns a diff via the executor agent prompt.
- **H2** `clue-code agent run` (no name) → router auto-selects from task description.
- **H3** `clue-code agent moa "design X"` runs 3 models in parallel and synthesizes.
- **H4** Model unavailable → fallback chain with clear log.

---

## §5. Phase 4.8 — Token engine (L4) (~1 sem)

### 5.1 Goal

The differentiator vs Claude Code: real-time token counter, 3-level cache, budget guardrails.

### 5.2 Files

- `internal/tokens/{counter,cache,budget,analytics}.go`
- `cmd/clue-code/tokens.go` — `clue-code tokens {summary,top,clear-cache}`
- `tiktoken-go` port for Anthropic/OpenAI tokenizer

### 5.3 Acceptance criteria I1–I5

- **I1** Token count ±2% vs upstream API usage report.
- **I2** Prompt cache hit rate >30% on warm dogfood.
- **I3** `budget_usd_per_day: 5.0` exceeded → block call, log alert.
- **I4** `clue-code tokens summary` shows daily breakdown.
- **I5** Cache invalidation on SKILL.md change (mtime-based).

---

## §6. Phase 4.3 — Team primitives (Track D) (~2 sem)

**Reference:** PHASE_4_PLAN.md §3 Track D + §5 D1-D12. Plan unchanged from ralplan-locked spec. Wire-first NDJSON Transport, dual impl (inproc + subprocess), stalled detector, journal torn-tail recovery, fork-bomb guard cap=1.

---

## §7. Phase 4.9 — Aider integration (L3) (~1.5 sem)

### 7.1 Goal

Reuse Aider's edit engine + repo-map instead of reimplementing.

### 7.2 Files

- `internal/adapters/aider/{client,subprocess,parser,repomap}.go`
- `clue-code doctor` extended to detect Aider via `aider --version`

### 7.3 Acceptance criteria J1–J3

- **J1** Skill autopilot with `--use-aider` produces git-clean diff via Aider.
- **J2** Aider absent → fallback direct edit, warn user.
- **J3** Aider crash → our process survives, log error.

---

## §8. Phase 4.10 — MCP bridge (L3) (~1.5 sem)

### 8.1 Goal

Translate MCP server protocol to OpenAI-style tool calls so non-Anthropic models can use existing MCP servers.

### 8.2 Files

- `internal/adapters/mcp/{client,bridge,translator}.go`
- `cmd/clue-code/mcp.go` — `clue-code mcp {list,call}`

### 8.3 Acceptance criteria K1–K3

- **K1** MCP filesystem server callable from DeepSeek (non-Anthropic model).
- **K2** Tool errors translated cleanly.
- **K3** MCP server crash → our process survives.

---

## §9. Phase 4.11 — 3 runtime modes (~1 sem)

### 9.1 Goal

`clue-code mode {local,cloud,hybrid}` switching with smart routing per mode.

### 9.2 Files

- `internal/orchestrator/mode.go`
- `cmd/clue-code/mode.go`

### 9.3 Acceptance criteria L1–L4

- **L1** `mode local` blocks all network egress (verified via `httptest.NewTLSServer` audit).
- **L2** `mode cloud` skips Ollama/MLX entirely.
- **L3** `mode hybrid` routes per task tier (read/edit local, architecture cloud).
- **L4** Cloud down + hybrid → automatic local fallback, no user-facing error.

---

## §10. Phase 4.4 — TUI full (Track C) (~1.5 sem)

**Reference:** PHASE_4_PLAN.md §3 Track C + §5 C1-C7. Plan unchanged. 6 views (sessions, agents, skills, tokens, state, hooks) behind `tui` build tag.

---

## §11. Phase 4.12 — Polish + 1.0 release (~1 sem)

### 11.1 Goal

Ship public 1.0.

### 11.2 Deliverables

- `clue-code doctor` complete (RAM, disk, Aider, Ollama, network, MLX)
- `scripts/install.sh` cross-platform with auto-deps detection
- README quickstart in 30 seconds
- Migration guide from OMC
- `goreleaser` config for darwin-arm64/amd64 + linux-amd64/arm64
- Tag `v1.0.0` signed via Sigstore
- Release notes
- Demo GIF (asciinema preferred)

### 11.3 Acceptance criteria M1–M3

- **M1** Mac M1 vierge: install one-liner → `clue-code chat "hello"` works in <2 min.
- **M2** OMC user migration tested in <10 min.
- **M3** `goreleaser` produces 4 signed binaries.

---

## §12. Critical path & timeline

```
Week 1-2  : Phase 4.5 (model proxy)        ★ critical bottleneck
Week 3-4  : Phase 4.6 (skill execution)
Week 5-6  : Phase 4.7 (agent invocation)
                + Phase 4.3 starts in parallel (worker pool 2)
Week 7    : Phase 4.8 (token engine)
                + Phase 4.3 finishes
Week 8-9  : Phase 4.9 (Aider) + Phase 4.10 (MCP) parallel
Week 10   : Phase 4.11 (modes)
Week 11-12: Phase 4.4 (TUI full)
Week 13   : Phase 4.12 (polish)
Week 14   : v1.0 SHIP

Buffer: 0.5 week absorbed across late phases for unforeseen bugs.
```

**Stop conditions:**
1. 2+ week slip on critical path 4.5/4.6 → cut scope (drop 4.10 MCP, drop 4.4 TUI full to read-only).
2. Trademark Klue Labs forces rebrand → all README/docs/module path rework, +2-3 weeks.
3. Open-source models insufficient for daily driver → relegate `mode local` to Phase 5.

---

## §13. Risk register

| # | Risk | P × I | Mitigation |
|---|---|---|---|
| R1 | Phase 4.6 (real skill exec) more complex than estimated | 70% × High | Time-box at 2.5 sem; simplify skills if exceed |
| R2 | Aider sub-process integration fragile | 50% × Medium | Fallback direct edit; MCP as alternative |
| R3 | DeepSeek API rate limits during dogfood | 60% × Medium | Aggressive prompt cache (4.8) + Ollama fallback |
| R4 | TUI flakiness (golden frames) blocks release | 40% × Medium | TUI = "slip candidate" per PHASE_4_PLAN §0.1 |
| R5 | MCP protocol evolutions | 30% × High | Pin MCP version, isolate adapter |
| R6 | Klue Labs trademark forces rebrand | 20% × Critical | Decide MAY 2026, in parallel with 4.5 |
| R7 | Worker stalling pattern (cf. Phase 4.2 worker-2) | 60% × Low | Watchdog + reassign at 30min idle |

---

## §14. Open Questions

- **Q1.** Anthropic Claude API key required for which fallback chain? Recommendation: optional — DeepSeek + Ollama covers daily driver.
- **Q2.** Token tokenizer choice: tiktoken-go port vs native tokenizer per provider? Recommendation: tiktoken-go for OpenAI-shape; per-provider adapters for Anthropic counts.
- **Q3.** Should `clue-code chat` persist history? Recommendation: yes — `~/.clue-code/state/sessions/<sid>/transcript.ndjson`. Clear via `clue-code chat --new`.
- **Q4.** MoA aggregation strategy: vote-majority vs synthesis? Recommendation: synthesis via dedicated agent (`agents/critic.md` + special prompt).

---

*This document is updated incrementally as each sub-phase ships. The next sub-phase (4.6, 4.7, etc.) will receive a detailed §X.Y expansion when it becomes the critical path.*
