# CLUE CODE — Phase 4 Implementation Plan

**Status:** DRAFT for RALPLAN consensus (deliberate mode)
**Author:** Planner agent
**Date:** 2026-05-05
**Scope:** Track A (Hooks / L6) · Track B (State primitives / L0+L5) · Track C (Bubble Tea TUI / L8) · Track D (Team primitives / L5)
**Repo:** `github.com/clue-code/clue-code` · Go 1.22 · single static binary
**Surface preserved:** Phase 1-3 commands (`version`, `doctor`), `internal/orchestrator` API, `agents/*.md`, `skills/*/SKILL.md` loader.

---

## 0.1 Scope and timeline acknowledgment

**Scope:** This Phase 4 spans 4 tracks (B/A/D/C) plus Track A.5 (skillrunner) — 53 files (incl. fixtures + clock pkg), 6 sub-packages (incl. `internal/clock`), 38 acceptance criteria (A1-A6=6, E1-E5=5, B1-B8=8, C1-C7=7, D1-D12=12), 6.5-week timeline per Architect estimate. Consensus considered splitting into Phase 4 (B+A+A.5) and Phase 4.5 (D+C) per Critic ADV-1; the user explicitly scoped all 4 tracks together because they form one architectural surface (state/hooks/team/TUI all consume each other through `*orchestrator.Registry`, `state.Store`, `*hooks.Manager`). We accept the timeline risk: Sub-phase 4.4 (TUI golden frames) is the most likely overrun candidate; if it slips, ship Phase 4 minus 4.4, tag a release, and land 4.4 as a post-Phase-4 hardening pass. Track D (team) is NOT a candidate for slip — it unlocks the codex/gemini parity story per Architect REV-A1 reasoning.

---

## 1. RALPLAN-DR Summary (deliberate mode)

### 1.1 Principles (5)

1. **Local-first.** Every Phase 4 surface must work offline on an M1 / 8 GB box with zero external services. Cloud features remain opt-in (Phase 5+).
2. **Single static binary.** No CGo, no Python, no SQLite C bindings, no shared libraries. `go build ./cmd/clue-code` must produce one self-contained binary.
3. **Graceful degradation.** Hooks, TUI, and IPC must each degrade safely when their backing resource is unavailable (corrupted file, missing config, narrow terminal, etc.) — never bring down the orchestrator core.
4. **Composable core.** TUI, CLI, and skills call the *same* orchestrator package surface. No business logic inside `cmd/` or TUI views — they are thin shells over `internal/`.
5. **Append-mostly state.** Concurrent state writes default to append-only event journals with periodic compaction; mutable index files are guarded by an OS-level file lock. Avoid in-place rewrites of large blobs.

### 1.2 Decision Drivers (top 3)

1. **Must run on macOS / Linux laptop with 8 GB RAM, no daemons.** Rules out separate state daemons or services that hold long-lived RAM.
2. **Must remain a single CGo-free static binary.** Rules out `mattn/go-sqlite3` (CGo) and any IPC scheme that needs system services beyond what the OS already exposes (Unix sockets and `os/exec` are fine; D-Bus / kqueue daemons are not).
3. **Must support 4–8 concurrent agent processes coordinating without the user noticing latency.** Rules out poll-only schemes >100 ms; pushes toward in-process or socket-based fan-out for the hot path.

### 1.3 Viable Options (≥2 per major decision)

#### Decision 1 — Hook execution model

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **A. `os/exec` subprocess + stdout capture + optional context injection (chosen)** | Matches OMC mental model. Per-event timeout. Works with any shell command. Easy to log. | Subprocess startup ~5–15 ms on macOS. Need allowlist to prevent arbitrary code from `~/.config/`. | Chosen. |
| B. In-process Go plugins (`plugin.Open`) | Zero startup cost. Type-safe. | `plugin` is broken on macOS for cross-version builds, breaks "single binary" principle, ecosystem is dead. | Eliminated — violates Principle 2 (single binary) and is unmaintained on darwin/arm64. |
| C. Embedded Lua / Starlark interpreter | Sandboxable. No fork cost. | Adds a non-trivial dep, second language, smaller user base, no parity with OMC's shell-hook expectations. | Eliminated — violates parity with OMC and adds language surface area Phase 4 cannot afford. |

#### Decision 2 — State storage

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **A. JSON files + `flock` per-scope (chosen)** | Pure stdlib, CGo-free, human-debuggable, trivial backup, plays nicely with `git diff`. Append journal for sessions; index files for plans/notepad. | Concurrent multi-writer requires advisory lock discipline. Listing across all projects requires a global session registry file. | Chosen. |
| B. SQLite via `mattn/go-sqlite3` | Real transactions, SQL queries, mature. | Requires CGo — kills the static-binary principle on darwin/arm64 cross-compile and bloats the release matrix. | Eliminated — violates Principle 2. |
| C. SQLite via `modernc.org/sqlite` (pure Go) | Real transactions, no CGo. | ~7 MB extra binary size. Slower than CGo SQLite by ~2–3×. License is BSD (compatible). Schema migrations become a permanent debt. | Deferred for Phase 4, NOT eliminated by Principle 2 (it is pure Go). Chosen against on dogfood-affordance grounds — see ADR-2 honesty paragraph. **Re-evaluation gate at Phase 5:** if `state_list_active` p99 latency >50 ms across 10 projects × 5 sessions, switch to `modernc.org/sqlite`. |
| D. Bolt / bbolt (`go.etcd.io/bbolt`) | Pure Go, ACID, B+tree, no CGo. | Single-writer per file. Binary format opaque to users (can't `cat .clue-code/state/*`). | Eliminated — violates the "human-debuggable" axis of Principle 1 (local-first). |
| E. Hybrid — JSON for kv + `modernc.org/sqlite` for cross-project session index | Faster `state_list_active` at scale, still pure-Go, keeps human-debuggable kv path. | Two storage models, harder migration once ossified, dual locking story. | Deferred (not eliminated). Re-considered at Phase 5 perf gate alongside Option C. |

#### Decision 3 — Team / IPC primitives

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **A. Wire-first `Transport` (NDJSON over `io.ReadWriter`) with TWO impls in Phase 4 — `inproc.go` (over `io.Pipe`/`net.Pipe()`) AND `subprocess.go` (over child `os/exec.Cmd` stdin/stdout) (chosen)** | Same wire format for in-proc and cross-process. NDJSON envelopes are grep-able. Subprocess transport unblocks the existing `skills/team/SKILL.md` codex_worker / gemini_worker pattern (today driven by tmux). In-proc keeps microsecond latency for the hot path. | Two impls double test surface vs. an in-proc-only Phase 4. Subprocess transport must handle child lifetime + stderr piping. | Chosen. Both impls land in Phase 4. |
| B. In-process goroutines + Go-channel `Transport` (no wire format) | Zero serialization. Simplest possible. | `chan Message` is not a wire — Phase 6 cross-process work would have to rewrite the contract. Cannot interoperate with the tmux-driven codex/gemini workers `skills/team/SKILL.md:36-40, 587-643, 916` already ships. | Eliminated — defers a wire-format decision that Phase 4 can absorb cheaply now. |
| C. Unix domain socket transport only | Process isolation; OOM in one agent doesn't kill the orchestrator. | Socket lifecycle, restart semantics, Windows portability all become Phase-4 work for no near-term benefit (subprocess via stdin/stdout is sufficient for `codex` / `gemini` CLI workers). | Eliminated for Phase 4 — pushed to Phase 6 if cross-machine teams arrive. The same NDJSON wire format will drop into a socket transport unchanged. |
| D. SQLite-backed message queue | Survives process death; pollable from any tool. | Inherits Decision 2's SQLite trade-offs; adds polling latency (≥10 ms) on the hot path; over-engineered for in-process fan-out. | Eliminated — wrong trade-off for in-process workers. |

### 1.4 Pre-mortem — three failure scenarios

**Scenario 1 — "The hook killed the orchestrator."**
- *What went wrong:* A `PreToolUse` hook triggered another `clue-code` invocation that itself had `PreToolUse` configured → infinite recursion → fork bomb.
- *Leading indicator:* `.clue-code/state/hooks.log` shows monotonically growing `event_id` from one root session within a single second; OS load average climbs.
- *Mitigation in plan:* (i) Re-entrancy guard: `CLUE_CODE_HOOK_DEPTH` env var incremented on every hook spawn, hard cap at 3. (ii) Per-event timeout default 5 s, hard cap 30 s. (iii) `clue-code doctor --hooks` flags any hook whose command contains `clue-code` itself. (iv) Allowlist in config defaults to **deny** for `clue-code` self-invocation unless `allow_self_invoke: true` is set explicitly.

**Scenario 2 — "Two skills wrote the same notepad and one won."**
- *What went wrong:* `autopilot` and `ralph` both ran `state_write("notepad", …, project)` simultaneously. Last writer clobbered the other's working memory.
- *Leading indicator:* Users report "I added a note in autopilot and now it's gone." `git log .clue-code/notepad.md` shows back-to-back overwrites within 200 ms.
- *Mitigation in plan:* (i) Per-key `flock` (advisory) on every mutating op, with 2 s acquisition timeout. (ii) Notepad is structured as append-only sections separated by `## <skill> @ <iso8601>` headers — `state_write(notepad, …)` *appends* by default, only `state_clear` truncates. (iii) `state_write` returns `(version uint64, err error)` so callers can do CAS via `WriteIfVersion`.

**Scenario 3 — "The TUI showed stale data after a CLI run."**
- *What went wrong:* User ran `clue-code tui` in one terminal, then `clue-code skill autopilot` in another. The TUI's session list never refreshed and showed the old session as "active" forever.
- *Leading indicator:* `state_list_active` returns sessions whose `last_activity` is hours old. TUI status bar disagrees with `clue-code status`.
- *Mitigation in plan:* (i) Session registry uses `fsnotify` (`gopkg.in/fsnotify.v1`, pure Go) to push changes into the TUI's `tea.Msg` queue. (ii) Every session writes a heartbeat to `state/sessions/<id>/heartbeat` every 5 s; sessions with `now - heartbeat > 30 s` are reported as `stale`, not `active`. (iii) TUI has an explicit `r` keybind for manual reload as a fallback.

### 1.5 Expanded test plan (per track)

| Lane | Track A (Hooks) | Track B (State) | Track C (TUI) | Track D (Team) |
|---|---|---|---|---|
| **Unit** | hook config parser; matcher; timeout enforcement; depth guard; output truncation. Table-driven in `hooks_test.go`. | scope resolution; flock acquire/release; CAS path; session descriptor (de)serialization. | model `Update` per message type with `tea.NewProgram` in headless mode + `teatest`. | task graph topological order; channel routing; journal replay. |
| **Integration** | spawn real subprocess that writes to stdout/stderr, asserts capture + injection envelope; cross-platform on darwin + linux runners. | concurrent multi-process write contention test using two `os/exec` clue-code subprocesses on the same key. | full `tea.Program` startup, drives synthetic key events, asserts view snapshots. | spawn 8 worker goroutines, each posts 100 messages, verify ordering + journal completeness. |
| **E2E (`go test -tags=e2e`)** | run a sample skill that fires all 5 hook events end-to-end and asserts `hooks.log` contents. | run `clue-code state list-active` against a populated `~/.clue-code/`, assert output. | drive TUI through a 6-step golden script with `expect`-style harness; compare ANSI-stripped frames to fixtures. | run `clue-code team demo` to spin up 3 agents that exchange messages and exit cleanly; assert journal + final state. |
| **Observability** | every hook fires writes a structured `slog` event with `event`, `command`, `duration_ms`, `exit_code`, `truncated`. Counter exported via `clue-code doctor --json`. | `state` ops emit `slog` events with `op`, `scope`, `key_hash` (not key — privacy), `bytes`, `duration_us`. | TUI emits a startup `slog` event with `term`, `width`, `height`; key events are *not* logged (privacy). | every `SendMessage` and `TaskUpdate` emits an `slog` event with `team_id`, `task_id`, `from`, `to`, `payload_bytes`. |
| **Negative / fault-injection** | hook with infinite loop (timeout fires); hook that prints 100 MB to stdout (output truncated to 64 KiB); recursive hook (depth guard fires). | corrupt JSON; permission-denied dir; lock held by zombie PID. | terminal width=20; SIGWINCH during render; non-TTY stdout (TUI must refuse cleanly with helpful error). | dropped channel; agent panics mid-task; team file deleted while running. |

---

## 2. Architecture Decision Records (ADRs)

### ADR-1 — Hook execution model: `os/exec` subprocess + capture + opt-in context injection

- **Decision:** Run hooks as `os/exec.CommandContext` subprocesses with bounded stdout/stderr capture, per-event timeout, re-entrancy depth guard, and an opt-in mechanism to inject the captured stdout back into the calling skill as an additional context block (mirroring OMC's `<system-reminder>` envelope).
- **Drivers:** Parity with OMC (Driver 3 — agents already understand "shell hook" semantics); single-binary (Driver 2); local-first (Driver 1).
- **Alternatives considered:** Go `plugin` (eliminated, broken on darwin), embedded Starlark/Lua (eliminated, parity cost too high).
- **Why chosen:** Matches existing OMC user expectations one-to-one. Trivial to debug (`echo` works). Subprocess isolation means a malformed hook cannot corrupt orchestrator state. Phase 4 cost is one Go package and one YAML schema.
- **Consequences (good):** Users can write hooks in any language. Subprocess crash never propagates. Cross-platform with zero conditional code (`os/exec` works identically on darwin/linux/windows for our use cases).
- **Consequences (bad):** ~5–15 ms cold-start cost per hook on macOS. Not suitable for the *hot path* of every token (acceptable — hooks fire on lifecycle points only, not per token). Allowlist must be designed carefully or hooks become a privilege-escalation vector.
- **Follow-ups:** Phase 5 may add an in-process fast path for built-in hooks (e.g. metrics collection) once the metrics primitives exist; the public hook YAML stays unchanged.

### ADR-2 — State storage: JSON files + advisory `flock`, with a `modernc.org/sqlite` upgrade gate at Phase 5

- **Decision:** Phase 4 ships JSON-on-disk for all state with `golang.org/x/sys/unix` advisory file locks (`flock`) for mutating ops. Schema is documented in this file (Section 4.B). Phase 5 re-evaluates against a measured budget; if `state_list_active` exceeds 50 ms p99 across **10 projects × 5 sessions**, migrate to `modernc.org/sqlite` (pure-Go, CGo-free) using the same public Go API.
- **Drivers:** Single-binary (Driver 2); local-first / human-debuggable (Driver 1).
- **Alternatives considered:** SQLite (CGo) — eliminated by Principle 2. SQLite (`modernc.org/sqlite`) — **deferred, NOT eliminated by Principle 2**. bbolt — eliminated (opaque format). Hybrid JSON kv + `modernc` session index — deferred.
- **Why chosen — honest rationale:** `modernc.org/sqlite` satisfies Principle 2 (CGo-free pure Go) and is therefore not eliminated by Principle 2. We choose JSON+flock for Phase 4 because (a) human-debuggability via `cat | jq` and `git diff` is a stronger affordance for an alpha-stage open-source tool than SQL, (b) the Phase 4 KV surface is ~5 keys with no relational queries, (c) the Phase-5 perf gate (10 projects × 5 sessions, p99 > 50 ms) is a realistic dogfood ceiling that will trigger a real perf signal — not a never-fire safety blanket. Migration cost when the gate triggers: refactor of `state.Store` interface (currently shaped around JSON CAS) into transactional shape — estimated 2–3 days work tracked as a Phase-5 follow-up.
- **Consequences (good):** Zero new external deps for L0 storage. Trivial backup story (it's just files). Test fixtures are plain JSON.
- **Consequences (bad):** No SQL. `state_list_active` requires walking the global `~/.clue-code/sessions/index.json` file rather than a query. Concurrent-writer story relies on `flock` discipline — every mutating call site must use `state.With*` helpers, never raw file I/O. The `state.Store` interface is shaped around JSON CAS today; the Phase-5 SQLite swap is mechanical only for the surface, transactional for the impl.
- **Follow-ups:** Phase 5 perf gate (10 projects × 5 sessions, p99 < 50 ms). If gate fails → swap backend or adopt Hybrid Option E. The `internal/state.Backend` interface is added in Phase 4 to make the swap mechanical at the call-site layer; impl rewrite is the 2–3 day cost noted above.

### ADR-3 — Team / IPC: wire-first NDJSON `Transport` over `io.ReadWriter`, with two Phase-4 impls (in-proc + subprocess)

- **Decision:** The `Transport` interface is **wire-first**, not channel-shaped. The wire format is NDJSON envelopes `{v uint8, seq uint64, from string, to string, kind string, payload []byte, ts time.Time}` over `io.ReadWriter`. The `v` (Version) field is `1` for Phase 4; lines with `v > 1` cause `team.Open(id)` to return `ErrUnsupportedEnvelopeVersion` immediately and refuse to start the team. Phase 4 ships **both** impls: `internal/team/inproc.go` (over `io.Pipe()` / `net.Pipe()`) AND `internal/team/subprocess.go` (over child `os/exec.Cmd` stdin/stdout). The persisted NDJSON event journal at `.clue-code/teams/<team_id>/journal.ndjson` uses the same envelope format, so journal entries are valid wire frames. The Go-channel `<-chan Message` mailbox is a **consumer-side abstraction** over `Recv()`, NOT the wire contract itself. Subprocess transport is chosen alongside in-proc (not deferred) because `skills/team/SKILL.md:36-40, 587-643, 916` already ships codex_worker / gemini_worker via tmux — Phase 4 lifts that into a typed Go transport.
- **Wire-first interface (canonical):**
  ```go
  type Envelope struct {
      Version   uint8     `json:"v"`   // Phase 4 = 1; lines with v > 1 trigger ErrUnsupportedEnvelopeVersion on replay
      Seq       uint64
      From, To  AgentRef
      Kind      string
      Payload   []byte
      Timestamp time.Time
  }
  type Transport interface {
      SendEnvelope(env Envelope) error // serializes to underlying wire
      Recv() (Envelope, error)         // blocks; returns io.EOF on close
      Close() error
  }
  ```
- **Drivers:** Laptop-scale concurrency (Driver 3); single-binary (Driver 2); parity with existing skills/team subprocess workers.
- **Alternatives considered:** Channel-shaped transport (eliminated — defers wire-format decision); Unix domain sockets (deferred to Phase 6, cross-machine teams); SQLite-backed queue (eliminated, wrong trade-off).
- **Why chosen:** One wire format means in-proc and subprocess transports share serialization, journal format, and replay code. NDJSON envelopes are grep-able and `jq`-friendly. 4–8 in-proc workers exchanging 100 messages/second is a non-event for the Go runtime; subprocess workers add a small framing cost (NDJSON line per message) acceptable for codex/gemini-style CLI participants. Crash-resume replays the journal as if it were a `Transport.Recv()` stream.
- **Consequences (good):** Wire format settled in Phase 4 — Phase 6 socket transport drops in unchanged. Subprocess transport ready for codex/gemini CLI workers. Journal is grep-able and replay-compatible with the wire.
- **Consequences (bad):** Two impls double the test surface in Phase 4. Subprocess transport must own child lifetime, stderr piping, and signal forwarding. A panic in any goroutine still requires `recover()` at every `Transport` boundary. Subprocess transport `Recv()` returns `io.EOF` on clean child exit (Close), `ErrTransportDead` (typed) on unclean exit (signal, segfault, oom-kill). In-flight messages on the child's stdin queue are lost; replay must come from the journal. Parent must reconcile journal's last entry against `ErrTransportDead` to mark partially-completed tasks correctly.
- **Follow-ups:** Phase 6 adds `socket.go` over Unix domain sockets implementing the same wire format, no callsite changes. Phase 7+ "Hub" handles cross-machine.

---

## 3. File-level Implementation Plan

### Track A — Hook system (L6)

**New package:** `internal/hooks/`
**New deps:** none (`gopkg.in/yaml.v3` already needed by Phase 5; for Phase 4 we hand-parse the small `hooks.yaml` schema with a minimal scanner OR introduce `gopkg.in/yaml.v3` here as the first external dep — see Open Question Q1).

**Files (8):**
- `internal/hooks/doc.go` — package doc.
- `internal/hooks/event.go` — `Event` enum (`SessionStart`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`), `Context` struct.
- `internal/hooks/config.go` — `Config`, `Spec`, parser for `hooks.yaml`, allowlist resolution, env var depth guard constants.
- `internal/hooks/runner.go` — `Runner` (subprocess driver, timeout, output capture, depth guard).
- `internal/hooks/runner_test.go` — table-driven runner tests with fake commands.
- `internal/hooks/manager.go` — `Manager` (orchestrator-facing API: `Fire(ctx, event, payload)`).
- `internal/hooks/log.go` — structured event logger writing `.clue-code/state/hooks.log` (NDJSON).
- `internal/hooks/manager_test.go` — integration tests covering all 5 events, depth guard, timeout, fail-open vs fail-closed.
- `internal/hooks/testdata/trivial-hook.yaml` — minimal one-event fixture for A1.
- `internal/hooks/testdata/recursive-hook.yaml` — self-invoke fixture for A3 (hermetic).
- `internal/hooks/testdata/infinite-loop-hook.yaml` — `sleep 60` fixture for A2 timeout.

**New CLI command:** `cmd/clue-code/hooks.go` — `clue-code hooks list|test|tail|fire-test` (`fire-test` is a synthetic helper that re-fires a hook by event name; only available behind `-tags=test` build tag so it does not pollute the production binary).

**Public Go API:**
```go
package hooks

// Event is the lifecycle point at which hooks fire.
type Event string

const (
    EventSessionStart     Event = "SessionStart"
    EventPreToolUse       Event = "PreToolUse"
    EventPostToolUse      Event = "PostToolUse"
    EventUserPromptSubmit Event = "UserPromptSubmit"
    EventStop             Event = "Stop"
)

// Spec is one hook entry in hooks.yaml.
type Spec struct {
    Command  string        `yaml:"command"`            // shell command, required
    Matcher  string        `yaml:"matcher,omitempty"`  // regex matched against payload
    Timeout  time.Duration `yaml:"timeout,omitempty"`  // default 5s, hard cap 30s
    Blocking bool          `yaml:"blocking,omitempty"` // if true, non-zero exit aborts the calling action
    Inject   bool          `yaml:"inject,omitempty"`   // if true, stdout becomes <hook-context> for the caller
}

// Config holds the parsed ~/.config/clue-code/hooks.yaml file.
type Config struct {
    Events       map[Event][]Spec `yaml:"events"`
    Allowlist    []string         `yaml:"allowlist,omitempty"`     // command prefixes allowed (default: all)
    AllowSelfInv bool             `yaml:"allow_self_invoke"`        // default false
}

// LoadConfig reads ~/.config/clue-code/hooks.yaml (respecting XDG and CLUE_CODE_HOOKS_CONFIG).
func LoadConfig() (*Config, error)

// Manager dispatches lifecycle events to configured hooks.
type Manager struct{ /* unexported */ }

// NewManager builds a Manager from the given config and log destination.
func NewManager(cfg *Config, logPath string) (*Manager, error)

// Fire runs every hook configured for ev. Payload is the structured context
// the caller wants matchers to see (e.g. tool name, prompt). Returns the
// concatenated injected output (empty if no hook had Inject:true), and any
// blocking error returned by a Blocking:true hook.
func (m *Manager) Fire(ctx context.Context, ev Event, payload map[string]any) (injected string, err error)

// FireAndForget runs hooks asynchronously, used for non-blocking events
// (SessionStart, Stop). Errors are logged, not returned.
func (m *Manager) FireAndForget(ev Event, payload map[string]any)
```

**Hook re-entrancy guard:**
```go
const (
    EnvHookDepth   = "CLUE_CODE_HOOK_DEPTH"
    MaxHookDepth   = 3
    DefaultTimeout = 5 * time.Second
    MaxTimeout     = 30 * time.Second
    MaxOutputBytes = 64 * 1024
)
```

**Failure semantics:**
- `Blocking: false` (default) → log error, continue.
- `Blocking: true` → return wrapped error to caller; caller decides whether to abort the user-facing action.

#### §3.A.1 — Hook threat model (Phase 4 scope)

Hooks are user-written commands trusted at the same level as the user's shell. The Phase 4 trust boundary is: **anything executable from `~/.config/clue-code/hooks.yaml` is treated as trusted-by-the-user.** Phase 4 does NOT defend against shell-injection in user-written hooks.

Known limitations:
- `unset CLUE_CODE_HOOK_DEPTH` is a known bypass for malicious hooks. **Defense-in-depth** via in-process per-session recursion counter: a `sessionID`-keyed `map[string]int` inside `*hooks.Manager` is incremented on every spawn and decremented on subprocess exit. The env-var counter is the cross-process signal; the map is the in-process signal — both must allow the spawn.
- Phase 5 follow-up: skill-shipped hooks (i.e. hooks installed by skills, not by the user) need stronger isolation — likely a sandboxed subprocess profile (no network, restricted FS) and a separate trust class.

#### §3.A.2 — Allowlist truth table (defaults)

| `Allowlist` | `AllowSelfInv` | Behavior |
|---|---|---|
| nil/empty | true | All commands allowed (insecure but explicit) |
| nil/empty | false | All commands allowed EXCEPT `clue-code` self-invoke |
| non-empty | true | Only allowlist matches; self-invoke allowed |
| non-empty | false | Only allowlist matches AND no `clue-code` self-invoke |

**Default config:** `Allowlist: nil`, `AllowSelfInv: false` (loop guard active by default; arbitrary commands permitted; `clue-code` self-invoke blocked unless explicitly enabled).

---

### Track A.5 — Skill runner (in-Phase-4.2)

**New package:** `internal/skillrunner/`
**New deps:** none (reuses `internal/orchestrator`, `internal/hooks`, `internal/state`).

**Files (4):**
- `internal/skillrunner/loader.go` — discovers and parses `skills/<name>/SKILL.md` frontmatter.
- `internal/skillrunner/engine.go` — orchestrates a skill run (lifecycle hooks, state binding, agent dispatch).
- `internal/skillrunner/hooks_glue.go` — wires `*hooks.Manager` lifecycle calls into the engine.
- `internal/skillrunner/engine_test.go` — coverage for SessionStart/Stop hook firing, `Inject:true` envelope wrapping, ErrHookDepthExceeded propagation, lifecycle ordering (E5), recursion depth (E2), graceful cancel (E4).
- `internal/skillrunner/testdata/synthetic-skill/SKILL.md` — minimal lifecycle-exercising skill (E5 ordering test).
- `internal/skillrunner/testdata/malformed-yaml-skill/SKILL.md` — invalid frontmatter for E1 skip-malformed test.
- `internal/skillrunner/testdata/recursive-skill/SKILL.md` — self-invoking skill for E2 depth-guard test.

**Public Go API:**
```go
type Engine struct{ /* unexported */ }

func NewEngine(reg *orchestrator.Registry, hm *hooks.Manager, store state.Store) *Engine

// Load discovers all skills under skillsDir. Per-skill parse errors are
// returned as a slice (one bad SKILL.md should not block the others).
func (e *Engine) Load(skillsDir string) (errs []error)

// Run executes the named skill end-to-end with the given args. Fires the
// SessionStart/Stop hook lifecycle around execution. Returns when the
// skill exits or ctx is cancelled.
func (e *Engine) Run(ctx context.Context, name string, args []string) error
```

Track A.5 lands inside Sub-phase 4.2 *after* the Track A merge — same PR or fast-follow PR — because it is the consumer of `*hooks.Manager.Fire(...)`.

---

### Track B — State tooling primitives (L0 + L5)

**New package:** `internal/state/`
**New deps:** `golang.org/x/sys/unix` (already implicit via Go toolchain; explicit dep) for `flock`. `github.com/fsnotify/fsnotify` for live session list updates (~250 LOC, pure Go, BSD-3 license).

**Files (12):**
- `internal/state/doc.go`
- `internal/state/scope.go` — `Scope` enum, path resolution.
- `internal/state/path.go` — `~/.clue-code/`, `./.clue-code/`, registry index path resolution.
- `internal/state/lock.go` — `withLock(path, fn)` helper, advisory flock with timeout.
- `internal/state/store.go` — `Store` interface + `jsonStore` impl. CAS via version counter. `WriteWithRetry` (exp backoff 10ms, 30ms, 90ms, 270ms, 500ms cap, then repeat at 500ms until ctx). `Append` (`O_APPEND|O_CREATE` + `fcntl(LOCK_EX)`).
- `internal/state/store_test.go` — covers B1, B4, B6, B7 (`TestWriteContention_NoBusyErrors`), B8 (`TestAppend_TwoWritersIntact`).
- `internal/state/session.go` — `SessionDescriptor`, `SessionStatus`, heartbeat write/read.
- `internal/state/session_test.go`
- `internal/state/registry.go` — global cross-project session index (`~/.clue-code/sessions/index.json`) with `fsnotify` change feed.
- `internal/state/registry_test.go`
- `internal/state/metrics.go` — `state_write_contention_total{key,scope}` counter (incremented on every `WriteWithRetry` retry due to flock contention), `session_stale_lag_seconds` gauge (max age of any session whose last heartbeat is `> 30 s` ago), and `state_watch_dropped_total` counter (incremented when `Watch()`'s 64-buffered channel overflows and an event is dropped). Exposed via `cmd/clue-code/state.go metrics` subcommand.
- `internal/state/log.go` — pinned `slog` sink wiring: default to `<project>/.clue-code/state/clue-code.log` (rotated via `gopkg.in/natefinch/lumberjack.v2` at 10 MB, keep 3); `CLUE_CODE_LOG=stderr` opts back to stderr; `CLUE_CODE_LOG=<path>` redirects.
- `internal/state/testdata/home-fixture/global.json` — fixture global KV (B3 list-active test).
- `internal/state/testdata/home-fixture/sessions/index.json` — registry fixture for B3.
- `internal/state/testdata/home-fixture/sessions/<sid>/...` — per-session fixture data (heartbeat, kv.json) for B3.

**Shared clock package (CRITIC-5):** `internal/clock/clock.go` — 5 LOC `Clock` interface plus `realClock` and `fakeClock` impls. Used by `internal/team/stalled.go` (D8) and `internal/state/session.go` (heartbeat) so tests can inject deterministic time without `time.Sleep`.

**New CLI command:** `cmd/clue-code/state.go` — `clue-code state read|write|clear|list-active|status|metrics`.

The `metrics` subcommand prints leading-indicator counters as JSON:
- `state_write_contention_total{key,scope}` — incremented every time `WriteWithRetry` retries due to flock contention.
- `session_stale_lag_seconds` — max age of any session whose last heartbeat is `> 30 s` ago.

These counters make Pre-mortem #2 (notepad clobber) and Pre-mortem #3 (TUI staleness) programmatically detectable, not user-reported.

**Default `slog` sink pin:** the orchestrator's default `slog` writer is `<project>/.clue-code/state/clue-code.log` (rotated at 10 MB, keep 3 oldest), NOT stderr. This is required so the TUI can render without interleaved log noise. Setting `CLUE_CODE_LOG=stderr` opts back to stderr (development workflow). Setting `CLUE_CODE_LOG=<path>` redirects to a custom file.

**Public Go API:**
```go
package state

type Scope string

const (
    ScopeSession Scope = "session"  // .clue-code/state/sessions/<sid>/kv.json
    ScopeProject Scope = "project"  // .clue-code/state/project.json
    ScopeGlobal  Scope = "global"   // ~/.clue-code/global.json
)

// Store is the abstract state API; implementations are jsonStore now,
// sqliteStore later (ADR-2 Phase 5 gate).
type Store interface {
    Read(ctx context.Context, key string, scope Scope) (value []byte, version uint64, exists bool, err error)
    Write(ctx context.Context, key string, value []byte, scope Scope) (version uint64, err error)
    WriteIfVersion(ctx context.Context, key string, value []byte, expected uint64, scope Scope) (version uint64, err error)
    Clear(ctx context.Context, scope Scope, prefix string) (removed int, err error)

    // WriteWithRetry retries Write on ErrStateBusy with exponential backoff.
    // Schedule: 10ms, 30ms, 90ms, 270ms, 500ms cap then repeat at 500ms
    // until ctx deadline. Returns the final version on success, or the
    // last error if ctx fires.
    WriteWithRetry(ctx context.Context, key string, value []byte, scope Scope) (version uint64, err error)

    // Append opens key in O_APPEND|O_CREATE mode under fcntl(LOCK_EX) and
    // writes value to the tail. Used for notepad-style log files where
    // multiple writers concatenate sections (each section header is its
    // own atomic append). Last-byte newline is the writer's responsibility.
    Append(ctx context.Context, key string, value []byte, scope Scope) error
}

// Open returns the default Store for this process.
// sessionID is required for ScopeSession ops.
func Open(sessionID string) (Store, error)

// SessionDescriptor describes a session visible across all projects.
type SessionDescriptor struct {
    ID           string    `json:"id"`
    ProjectPath  string    `json:"project_path"`
    StartedAt    time.Time `json:"started_at"`
    LastActivity time.Time `json:"last_activity"`
    PID          int       `json:"pid"`
    Skill        string    `json:"current_skill,omitempty"`
}

// SessionStatus is the live view of one session.
type SessionStatus struct {
    Descriptor   SessionDescriptor `json:"descriptor"`
    State        string            `json:"state"`         // "active" | "stale" | "ended"
    PendingTasks int               `json:"pending_tasks"` // from teams journal
}

// ListActive returns all sessions across all projects on this host.
// Sessions whose last heartbeat is older than 30s are reported as state=stale.
func ListActive() ([]SessionDescriptor, error)

// GetStatus returns the live status for one session id.
// Returns ErrSessionNotFound if no descriptor exists.
func GetStatus(sessionID string) (SessionStatus, error)

// Watch returns a channel that receives a SessionDescriptor every time a
// session is created, heartbeats, or ends. Used by the TUI.
//
// Channel is buffered at 64. On overflow, oldest events are dropped and
// the `state_watch_dropped_total` counter is incremented (exposed via
// `clue-code state metrics`). TUI reload (`r` keybind) reseeds full state.
func Watch(ctx context.Context) (<-chan SessionDescriptor, error)
```

**On-disk schema:**
```
~/.clue-code/
├── global.json                       # global scope KV
├── sessions/
│   └── index.json                    # registry: [SessionDescriptor]
└── plans/                            # cross-project plan store (Phase 5+)

<project>/.clue-code/
├── state/
│   ├── project.json                  # project scope KV
│   ├── sessions/
│   │   └── <sid>/
│   │       ├── kv.json               # session scope KV
│   │       ├── heartbeat              # touch file, mtime is heartbeat
│   │       └── transcript.ndjson     # session events (Phase 5+)
│   └── hooks.log                     # hook event log (NDJSON)
├── teams/
│   └── <team_id>/
│       ├── team.json
│       ├── tasks.json
│       └── journal.ndjson            # Track D
├── notepad.md
├── memory.json
└── plans/
```

**Locking discipline:**
- Every mutating call wraps the target file's `<file>.lock` sentinel under `flock(LOCK_EX)`.
- 2 s acquisition timeout; on contention return `ErrStateBusy` so callers can retry or surface to user.
- CAS via integer `version` field stored alongside payload; `WriteIfVersion` is the only safe way to do read-modify-write across multiple processes.

---

### Track C — Bubble Tea TUI (L8)

**New package:** `internal/tui/` (model layer) + `cmd/clue-code/tui.go` (entry point).
**New deps (go.mod, gated by `tui` build tag):**
- `github.com/charmbracelet/bubbletea` v1.x (MIT)
- `github.com/charmbracelet/bubbles` v0.x (MIT)
- `github.com/charmbracelet/lipgloss` v0.x (MIT)
- (transitive: `github.com/muesli/termenv`, `github.com/charmbracelet/x/...` — all MIT)

**Apache-2.0 friendliness:** MIT is permissive and compatible with Apache-2.0 distribution. Confirmed.

**Build-tag gating (CRITICAL — prevents charmbracelet bloat in headless binary):**
- Every file under `internal/tui/**.go` starts with `//go:build tui`.
- `cmd/clue-code/tui.go` is split into TWO files:
  - `cmd/clue-code/tui.go` (`//go:build tui`) — the real entry point that calls `internal/tui.Run(...)`.
  - `cmd/clue-code/tui_stub.go` (`//go:build !tui`) — prints `clue-code was built without TUI support; rebuild with -tags=tui` and exits 2.
- charmbracelet deps are listed in `go.mod`, but `go build ./cmd/clue-code` (no tag) does NOT link them — the stub satisfies the unused-import constraint.
- CI workflow ships TWO artifacts:
  - `clue-code` (no tag, target `< 12 MB`).
  - `clue-code-tui` (`-tags=tui`, target `< 18 MB`).

**Files (13, all gated `//go:build tui` except where noted):**
- `internal/tui/doc.go`
- `internal/tui/app.go` — root model, view router.
- `internal/tui/keys.go` — keybindings.
- `internal/tui/style.go` — lipgloss themes.
- `internal/tui/views/sessions.go` — session list view.
- `internal/tui/views/agents.go` — agent picker.
- `internal/tui/views/skills.go` — skill runner.
- `internal/tui/views/tokens.go` — live token counter (placeholder bound to `internal/tokens` once Phase 5 lands; until then renders `--`).
- `internal/tui/views/state.go` — state inspector (read-only browser of `state.Store`). **Lands in Sub-phase 4.1 alongside Track B as the TUI vertical slice (~100 LOC); headless 4.1 binary stays charmbracelet-free because of the `tui` build tag.**
- `internal/tui/views/hooks.go` — hook event stream (tails `hooks.log`).
- `internal/tui/app_test.go` — uses `teatest` for headless model tests.
- `cmd/clue-code/tui.go` (`//go:build tui`) — TUI entry point.
- `cmd/clue-code/tui_stub.go` (`//go:build !tui`) — refusal message + exit 2.
- `internal/tui/views/testdata/sessions-24x80.golden.ans` — golden frame for C1.
- `internal/tui/views/testdata/agents-24x80.golden.ans` — golden frame for C1 + §1.5 E2E.
- `internal/tui/views/testdata/skills-24x80.golden.ans` — golden frame for C1 + §1.5 E2E.
- `internal/tui/views/testdata/tokens-24x80.golden.ans` — golden frame for C1 + §1.5 E2E.
- `internal/tui/views/testdata/state-24x80.golden.ans` — golden frame for C1 + §1.5 E2E.
- `internal/tui/views/testdata/hooks-24x80.golden.ans` — golden frame for C1 + §1.5 E2E.

**`runTUI` under both build tags (REV-A6):** Both `cmd/clue-code/tui.go` and `cmd/clue-code/tui_stub.go` define `runTUI(args []string) int` with identical signature; `cmd/clue-code/main.go` dispatches `case "tui": runTUI(args)` under both build configurations. CI runs `go vet ./...` AND `go vet -tags=tui ./...` on every PR to catch build-tag drift before binary build.

**Public entry point:**
```go
package tui

// Options configure the TUI. All fields optional; sensible defaults applied.
type Options struct {
    Registry    *orchestrator.Registry
    Hooks       *hooks.Manager
    State       state.Store
    SessionID   string  // current session
}

// Run starts the Bubble Tea program against the user's terminal.
// Returns when the user quits or ctx is cancelled.
func Run(ctx context.Context, opts Options) error
```

**Composition rule:** TUI views read `*orchestrator.Registry`, `state.Store`, `hooks.Manager` via the same constructors the CLI uses. No view holds business logic — it only translates state to lipgloss-rendered strings and key events to method calls.

**Live updates:** `views/sessions.go` subscribes to `state.Watch()` and converts `SessionDescriptor` events into `tea.Msg`. `views/hooks.go` tails `hooks.log` via `fsnotify`. No polling > 1 Hz anywhere.

**Coexistence with non-interactive CLI:** `tui` command checks `term.IsTerminal(int(os.Stdout.Fd()))`; if false, emits `clue-code tui requires a TTY` and exits 2. Non-TTY scripts continue using subcommands.

---

### Track D — Team primitives (L5)

**New package:** `internal/team/`
**New deps:** `github.com/HdrHistogram/hdrhistogram-go` (BSD-2, no CGo) for D6 latency-quantile assertion. (Otherwise stdlib only — `os/exec` for subprocess transport, `io.Pipe`/`net.Pipe` for inproc transport.) NDJSON envelopes are written with stdlib `encoding/json`.

**Files (12):**
- `internal/team/doc.go`
- `internal/team/transport.go` — `Transport` interface (wire-first, `io.ReadWriter`-shaped) + `Envelope` struct + NDJSON codec helpers.
- `internal/team/inproc.go` — `Transport` over `io.Pipe()` / `net.Pipe()`. Phase 4.
- `internal/team/subprocess.go` — `Transport` over child `os/exec.Cmd` stdin/stdout (NDJSON-framed). Phase 4. Owns child lifetime, stderr piping, and signal forwarding (SIGTERM on `Close()`, escalating to SIGKILL after 3 s).
- `internal/team/journal.go` — NDJSON journaler with rotation via `gopkg.in/natefinch/lumberjack.v2` (Apache-2.0, no CGo) at 16 MiB. Journal lines ARE valid wire envelopes — replay feeds them through `Transport.Recv()`-shaped iterator. Includes torn-tail handling: on `Open(id)`, scans `journal.ndjson` byte-by-byte; the LAST `\n` is the high-water mark; everything after is a torn write and discarded; emits `slog warn` with `truncation_offset_bytes`. On replay, lines with `v > 1` cause `Open()` to return `ErrUnsupportedEnvelopeVersion` immediately.
- `internal/team/team.go` — `Team`, `TeamSpec`, `TeamID`, lifecycle.
- `internal/team/task.go` — `Task`, `TaskID`, dependency graph, topological scheduler.
- `internal/team/message.go` — consumer-side `Message` view, mailbox `<-chan Message` adapter over `Transport.Recv()`.
- `internal/team/stalled.go` — stalled-team detector. Tracks per-team "last progress" (any `TaskUpdate` or `SendMessage`); when 60 s passes with no progress, broadcasts `team-event:stalled` on a side-channel. Time injected via `clock.Clock` interface (from `internal/clock/clock.go`) for testability. **Lifetime:** one goroutine per `*Team`, started in `TeamCreate`/`team.Open`, cancelled by a `done chan struct{}` closed in `Team.Close()`. On crash-resume, `team.Open` re-arms the detector using the journal's last-progress timestamp as the baseline.
- `internal/team/team_test.go`
- `internal/team/journal_test.go`
- `internal/team/testdata/torn-journal.ndjson` — fixture: 5 valid lines + a partial 6th (no trailing newline).
- `internal/team/testdata/ten-task-team/journal.ndjson` — fixture: 200-entry journal (D12 rebuild test).
- `internal/team/testdata/ten-task-team/team.json` — pre-deletion cache snapshot for D12.
- `internal/team/testdata/ten-task-team/tasks.json` — pre-deletion cache snapshot for D12.

**Invariant:** journal is truth; `team.json` and `tasks.json` are derived caches. On conflict during `Open(id)`, the cache files are rebuilt from the journal.

**Subprocess transport binary identity + stderr handling:**

> Subprocess transport spawns `os.Executable()` (resolved once at `TeamCreate`) with subcommand `team-worker --team-id=<id> --worker-id=<n>`. PATH lookup is forbidden. Phase 4 `TeamSpec` does NOT carry a user-specified `Cmd`. Arbitrary-binary subprocess workers are a Phase-5 feature gated behind an allowlist analogous to `hooks.Config.Allowlist`. Child stderr is redirected to `<project>/.clue-code/teams/<team_id>/workers/<worker_id>/stderr.log` (rotated at 4 MiB, keep 2, via `gopkg.in/natefinch/lumberjack.v2`). Child stdout is the NDJSON wire and is parsed by `subprocess.Recv()`. Child stdin receives outbound `SendEnvelope`.

**Subprocess fork-bomb guard (REV-A2):** `MaxTeamWorkers = 20` enforced in `TeamCreate` (returns `ErrTooManyWorkers`). `CLUE_CODE_TEAM_DEPTH` env-propagated counter, **cap = 1** (no nested teams in Phase 4). Phase 6+ may raise the cap; the env var is the lever.

**TeamSpec extensibility forward-compat note:**

> Forward-compat: Phase 6 will add `TeamSpec.WorkerSpecs []WorkerSpec` for tmux/socket transports needing per-worker `Cmd []string`, `WorkingDirectory`, `WorktreePath`, `Branch`. This is additive — Phase 4 callers using `Workers int` continue to work.

**New CLI command:** `cmd/clue-code/team.go` — `clue-code team list|inspect <id>|tail <id>|demo --transport={inproc,subprocess}` (read-only operator views; team creation is driven by skills, *except* `team demo`). The `cmd/clue-code/team_worker.go` binary entry implements the `team-worker --team-id=<id> --worker-id=<n>` subcommand spawned by subprocess transport.

`team demo --transport={inproc,subprocess}` runs a 2-worker example team that exchanges N messages and exits cleanly. Used by acceptance criterion D7 and as a user-facing "hello world" for the team API.

**Public Go API:**
```go
package team

type TeamID string
type TaskID string
type AgentRef string // e.g. "agent:executor", "user", "team-lead"

// TeamSpec configures a team at creation.
type TeamSpec struct {
    Name        string            // human-readable
    Workers     int               // number of parallel agents (1..20)
    Description string
    SessionID   string            // owning session
    Metadata    map[string]string // free-form
}

// Team is the runtime handle.
type Team struct{ /* unexported */ }

// TeamCreate creates a team. The team owns a directory under
// .clue-code/teams/<id>/, including journal.ndjson.
func TeamCreate(ctx context.Context, spec TeamSpec) (*Team, error)

// TeamDelete tears down a team and removes its on-disk state.
func TeamDelete(id TeamID) error

// TaskCreate adds a task to the team's task graph. Returns its id.
// DependsOn must reference existing TaskIDs in the same team.
func (t *Team) TaskCreate(task Task) (TaskID, error)

type Task struct {
    ID          TaskID    // empty on input, populated on return
    Title       string
    Description string
    Owner       AgentRef
    DependsOn   []TaskID
    Status      TaskStatus
    Result      string    // free-form
}

type TaskStatus string

const (
    TaskPending  TaskStatus = "pending"
    TaskRunning  TaskStatus = "running"
    TaskBlocked  TaskStatus = "blocked"
    TaskDone     TaskStatus = "done"
    TaskFailed   TaskStatus = "failed"
)

func (t *Team) TaskList() []Task
func (t *Team) TaskGet(id TaskID) (Task, error)
func (t *Team) TaskUpdate(id TaskID, fn func(*Task) error) error

// SendMessage delivers payload from one party to another.
// to may be a single AgentRef or "broadcast:*" for fan-out.
// Calls are non-blocking; full mailboxes return ErrMailboxFull.
func (t *Team) SendMessage(from, to AgentRef, payload []byte) error

// Inbox returns the receive channel for the given agent. The channel is
// closed when the team is deleted or the agent is unregistered.
func (t *Team) Inbox(agent AgentRef) (<-chan Message, error)

type Message struct {
    From      AgentRef
    To        AgentRef
    Payload   []byte
    Timestamp time.Time
    Sequence  uint64 // monotonic per team, for journal ordering
}

// Open re-attaches to an existing team after a crash, replaying journal.
func Open(id TeamID) (*Team, error)
```

**Transport — wire-first interface (canonical, ADR-3):**
```go
// Envelope is the on-wire frame for one message. Encoded as a single
// NDJSON line on every transport (in-proc, subprocess, future socket).
type Envelope struct {
    Version   uint8     `json:"v"`           // Phase 4 = 1; future versions bump this; replay rejects v > 1 with ErrUnsupportedEnvelopeVersion
    Seq       uint64    `json:"seq"`
    From      AgentRef  `json:"from"`
    To        AgentRef  `json:"to"`
    Kind      string    `json:"kind"`        // "message" | "task-update" | "panic" | "team-event"
    Payload   []byte    `json:"payload"`     // opaque to transport
    Timestamp time.Time `json:"ts"`
}

// Transport is io.ReadWriter-shaped, NOT chan-shaped. The chan Message
// mailbox exposed via Team.Inbox(...) is a consumer-side adapter over Recv().
type Transport interface {
    SendEnvelope(env Envelope) error // serializes one envelope to underlying wire
    Recv() (Envelope, error)         // blocks; returns io.EOF on close
    Close() error
}

// Phase 4 ships TWO impls:
//   inproc.go    — over io.Pipe() / net.Pipe()
//   subprocess.go — over child os/exec.Cmd stdin/stdout
// Phase 6 adds socket.go (Unix domain sockets); same wire format.
```

**Recovery from panics (scope, see acceptance D4):** every goroutine spawned by `Team` runs inside a wrapper that `recover()`s, logs the panic with stack to `slog`, writes a `kind: "panic"` envelope to `journal.ndjson`, marks the owning task `failed` (or the journal/transport as failed), and broadcasts a `team-event:agent-down` message. Required coverage: (a) worker goroutine, (b) journal-writer goroutine, (c) transport-receive goroutine. In all three cases the Team itself remains operable for further `SendMessage` / `TaskUpdate` calls.

---

## 4. Integration Points

### 4.1 Hooks fire from inside skills + CLI
- The skill engine (Phase 3, `skills/<name>/SKILL.md` loader — to be wrapped in Phase 4 by `internal/skillrunner/`) gets a `*hooks.Manager` injected at construction.
- Lifecycle:
  - `clue-code skill <name>` start → `hooks.FireAndForget(SessionStart, …)` after session registry insert.
  - Before any tool dispatch → `hooks.Fire(ctx, PreToolUse, payload)` (blocking-aware; `Inject:true` output is appended to the next prompt as `<hook-context>...</hook-context>`).
  - After tool dispatch → `hooks.Fire(ctx, PostToolUse, payload)`.
  - User submits a prompt → `hooks.Fire(ctx, UserPromptSubmit, payload)`.
  - Skill exits → `hooks.FireAndForget(Stop, …)`.
- The CLI itself fires `SessionStart` / `Stop` for direct subcommand invocations that open a session (so `clue-code state write` from a script also triggers user hooks if configured).
- Re-entrancy guard env (`CLUE_CODE_HOOK_DEPTH`) is propagated to every subprocess so a hook spawning `clue-code` recursively self-aborts at depth 3.

### 4.2 State primitives called from skills
- Skills construct one `state.Store` per session via `state.Open(sessionID)`.
- `autopilot` writes phase progress to `state_write("autopilot/phase", ..., ScopeSession)` and reads ralplan plan paths from `ScopeProject`.
- `ralph` persists `prd.json` digest + iteration counter under `ScopeSession` and reads `--critic` selection from `ScopeProject`.
- `team` skill: each worker reads the team manifest from `team.Open(teamID)` and writes per-worker checkpoints under `state.ScopeSession` with key prefix `team/<team_id>/worker/<n>/`.
- `clue-code state list-active` is the operator's "what's going on" view; the TUI's session list is the same data via `state.Watch()`.

### 4.3 TUI shares orchestrator state with CLI (single binary, two entry points)
- `cmd/clue-code/main.go` already dispatches by subcommand. Add `case "tui": runTUI(args)`.
- `runTUI` constructs the same `*orchestrator.Registry`, `*hooks.Manager`, and `state.Store` the CLI builds. There is exactly one source of truth per process.
- Session registry is process-shared via the `~/.clue-code/sessions/index.json` file + `fsnotify`. Two `clue-code` processes (one TUI, one CLI subcommand) see each other's sessions in real time.
- TUI never writes business state directly — every mutation goes through `state.Store` so locking is uniform.

### 4.4 Team primitives use the chosen IPC + state store
- `Team.journal.ndjson` lives under `<project>/.clue-code/teams/<id>/`.
- Task graph state is mirrored to `tasks.json` (atomic write via `state.lock`).
- `SendMessage` writes to the journal *before* it pushes to the recipient channel — guarantees crash-resume can replay messages.
- `team list|inspect|tail` CLI commands read these files directly (no IPC) — they are operator views, not participants.
- When Phase 6 swaps in the socket transport, the journal format stays identical so existing `tail` works unchanged.

---

## 5. Acceptance Criteria

### Track A — Hooks
- **A1.** GIVEN `~/.config/clue-code/hooks.yaml` with one `SessionStart` hook running `echo hello`, WHEN `clue-code skill autopilot` starts, THEN `hooks.log` contains exactly one NDJSON line with `event=SessionStart, exit_code=0, stdout="hello"`. *Verified by `go test -tags=e2e ./internal/hooks/...`.*
- **A2.** GIVEN a `PreToolUse` hook with `command: "sleep 60"` and default 5 s timeout, WHEN it fires, THEN it is killed at 5 s and emits `exit_code=-1, timed_out=true`. *Verified by `go test ./internal/hooks/...`.*
- **A3.** GIVEN a hook with `command: "sh -c 'export CLUE_CODE_HOOK_DEPTH=3; clue-code hooks fire-test'"`, WHEN it fires, THEN at depth 3 the `Manager.Fire()` call returns `ErrHookDepthExceeded` without spawning a subprocess. *Verified by `internal/hooks.TestDepthGuard_Hermetic`.*
- **A4.** GIVEN a `PreToolUse` hook with `inject: true` printing `BUDGET=10000` to stdout, WHEN it fires, THEN the calling skill receives a `<hook-context>BUDGET=10000</hook-context>` block in its next prompt. *Verified by skillrunner integration test.*
- **A5.** GIVEN a `Blocking: true` `PreToolUse` hook that exits non-zero, WHEN it fires, THEN the calling tool dispatch is aborted with a wrapped error referencing the hook spec. *Verified by integration test.*
- **A6.** GIVEN no `hooks.yaml`, WHEN any event fires, THEN the orchestrator (a) exits 0, (b) `stderr` byte-count is 0, (c) `<project>/.clue-code/state/hooks.log` does not exist. *Verified by `clue-code skill autopilot` smoke test on a clean machine; assertions are scripted, not visual.*

### Track A.5 — Skill runner
- **E1.** GIVEN `skills/foo/SKILL.md` with malformed YAML AND `skills/bar/SKILL.md` well-formed, WHEN `Engine.Load(skillsDir)` runs, THEN `bar` is loadable and `errs` contains exactly one error referencing `foo`. *Verified by `internal/skillrunner.TestLoad_SkipMalformedSkills`.*
- **E2.** GIVEN a skill that invokes itself, WHEN the 5th call would push depth to 5, THEN `Engine.Run` returns `ErrSkillDepthExceeded`. Calls at depth 1..4 succeed. The cap = 4 is justified by the real OMC chain `autopilot → ralph → executor → verifier` (depth 4). *Verified by `internal/skillrunner.TestRun_RecursionGuard`.*
- **E3.** GIVEN `Engine.Run(name)`, THEN `hooks.SessionStart` fires before the first agent dispatch and `hooks.Stop` fires after the last, in that order, even if the skill panics mid-run (recovered). *Verified by `internal/skillrunner.TestRun_LifecycleHooksAroundPanic`.*
- **E4.** GIVEN `ctx.Cancel()` while a skill is running, THEN `Engine.Run` returns within 200 ms with `ctx.Err()` AND `Stop` hooks have fired. *Verified by `internal/skillrunner.TestRun_GracefulCancel`.*
- **E5.** GIVEN a synthetic skill registered via test fixture that performs (start → tool dispatch → user prompt → tool dispatch → stop), WHEN it runs through `Engine.Run`, THEN `hooks.log` contains exactly one of each {SessionStart, PreToolUse, UserPromptSubmit, PostToolUse, Stop} in that exact order. *Verified by `internal/skillrunner.TestEngine_AllLifecycleHooksFire`.*

### Track B — State
- **B1.** GIVEN two processes calling `state.Write("k", v, ScopeProject)` simultaneously, WHEN both return successfully, THEN final stored value belongs to one of them and the version counter equals 2. *Verified by `internal/state.TestConcurrentWrite`.*
- **B2.** GIVEN a session writes a heartbeat every 5 s, WHEN the session crashes, THEN within 30 s `state.ListActive()` reports it as `state=stale`. *Verified by `e2e_test.go`.*
- **B3.** GIVEN three sessions running in three project directories, WHEN `clue-code state list-active` is executed from any working directory, THEN all three SessionDescriptors are listed. *Verified by `clue-code state list-active` against fixture `~/.clue-code/`.*
- **B4.** GIVEN `WriteIfVersion(k, v, expected=5)` and current version is 7, WHEN called, THEN it returns `ErrVersionMismatch` and does not write. *Verified by table test.*
- **B5.** GIVEN a session with 3 pending team tasks, WHEN `state.GetStatus(sid)` is called, THEN `PendingTasks=3`. *Verified by integration test against Track D.*
- **B6.** GIVEN `state.Clear(ScopeSession, "team/")`, WHEN the session has 5 keys with prefix `team/` and 2 without, THEN exactly 5 keys are removed and the 2 untouched. *Verified by table test.*
- **B7.** GIVEN 8 concurrent goroutines calling `WriteWithRetry(ctx5s, key, val, ScopeProject)`, WHEN they run simultaneously, THEN ALL 8 succeed within the 5 s deadline AND zero `ErrStateBusy` errors are surfaced to callers. *Verified by `internal/state.TestWriteContention_NoBusyErrors` (8-writer benchmark).*
- **B8.** GIVEN two skills calling `state.Append(ctx, "notepad", "## skill-X @ T")` and `state.Append(ctx, "notepad", "## skill-Y @ T+1ms")` simultaneously, WHEN both succeed, THEN final `notepad.md` contains both sections in file order with intact headers (verified by regex match on `## skill-(X|Y) @ \d+`); zero interleaved bytes. *Verified by `internal/state.TestAppend_TwoWritersIntact`.*

### Track C — TUI
- **C1.** GIVEN a 24×80 terminal, WHEN `clue-code tui` starts, THEN the session list view renders within 200 ms with no layout overflow AND no rendered line exceeds 80 ANSI-stripped columns (measured via `lipgloss.Width()`). Snapshot fixture: `internal/tui/views/testdata/sessions-24x80.golden.ans`. *Verified by `teatest` snapshot test.*
- **C2.** GIVEN three running sessions, WHEN a second `clue-code` subprocess (spawned via `os/exec`) creates a project at `t=0`, writes a session heartbeat at `t=200ms`, and ends at `t=600ms`, THEN the TUI session list converges within 1 s of both the create AND the removal events without any fsnotify mocking. *Verified by `internal/tui.TestSessionsView_RealFsnotifyConvergence` (real backend integration test).*
- **C3.** GIVEN stdout is not a TTY, WHEN `clue-code tui` is invoked, THEN it exits with code 2 and prints `clue-code tui requires a TTY (try running this in a terminal, not a pipe)`. *Verified by `cmd/clue-code/tui_test.go`.*
- **C4.** GIVEN the user presses `q`, WHEN any view is active, THEN the program exits 0 within 100 ms. *Verified by `teatest`.*
- **C5.** GIVEN the hooks log has 50 events, WHEN the user opens the hook event stream view, THEN the most recent 20 are rendered, oldest scrolls into view via `j/k` navigation. *Verified by `teatest`.*
- **C6.** GIVEN the build matrix, WHEN CI builds both artifacts, THEN `go build ./cmd/clue-code` (no tag) produces a binary `< 12 MB` AND `go build -tags=tui ./cmd/clue-code` produces a binary `< 18 MB`. *Verified by size-delta CI check.*
- **C7.** GIVEN the TUI is running with active `slog` writes (default sink `<project>/.clue-code/state/clue-code.log`), WHEN the `teatest` harness captures rendered frames, THEN no frame contains ANSI-corrupted output (no stray escape sequences, no log-line bytes interleaved with the model's render output). *Verified by `internal/tui.TestSlog_NoFrameCorruption`.*

### Track D — Team
- **D1.** GIVEN `TeamCreate(spec{Workers:4})`, WHEN four workers each `TaskCreate` and `SendMessage`, THEN all messages are delivered and the journal contains exactly `4 * (n_create + n_messages)` NDJSON lines. *Verified by `internal/team.TestFanOut`.*
- **D2.** GIVEN a task graph with `B depends_on A` and `C depends_on B`, WHEN A completes, THEN B becomes `running` and C remains `blocked` until B completes. *Verified by `TestScheduler_Topological`.*
- **D3.** GIVEN the orchestrator process is killed mid-team-run, WHEN it restarts and calls `team.Open(id)`, THEN the journal is replayed and task statuses match the pre-crash truth. *Verified by `TestCrashResume`.*
- **D4.** GIVEN one worker panics inside its goroutine, WHEN the panic is recovered, THEN its owning task is marked `failed`, an `agent-down` message is broadcast, and other workers continue. *Verified by `TestPanicRecovery`.*
- **D5.** GIVEN a mailbox bounded at 256, WHEN the 257th message is sent without the recipient draining, THEN `SendMessage` returns `ErrMailboxFull` immediately (no blocking). *Verified by `TestBackpressure`.*
- **D6.** GIVEN 8 workers exchanging 1000 messages each over 10 s of steady-state, WHEN measured by `TestSendMessage_P99Latency` (regular `*testing.T`, NOT `*testing.B` — bench can't fail CI on threshold), THEN `hist.Quantile(0.99) < 1*time.Millisecond` using `github.com/HdrHistogram/hdrhistogram-go` (BSD-2, no CGo). *Verified by `internal/team.TestSendMessage_P99Latency`.*
- **D7.** GIVEN `clue-code team demo --transport=subprocess`, WHEN it runs, THEN the parent spawns 2 child `clue-code team-worker` processes, the workers exchange 100 messages each direction, the parent's journal contains exactly 200 NDJSON entries, AND all child exit codes are 0. *Verified by `internal/team.TestSubprocessTransport_Demo` (e2e).*
- **D8.** GIVEN a team with worker A blocked on `Inbox(B)` waiting for B's reply AND worker B blocked on `Inbox(A)` waiting for A's reply, WHEN 60 s pass with no `TaskUpdate` and no `SendMessage` (clock injected via `clock.Clock` interface so CI does not actually sleep 60 s), THEN `team-event:stalled` is broadcast on a side-channel AND `clue-code team inspect <id>` reports `state=stalled`, `last_progress_age=60s+`, AND mailbox depths visible per worker. AND GIVEN `Team.Close()`, THEN the stalled-detector goroutine exits within 100 ms (verified via `runtime.NumGoroutine()` delta). *Verified by `internal/team.TestStalledTeamDetector`.*
- **D9.** GIVEN a subprocess worker that writes 1 MiB to stderr, THEN parent stderr/stdout are unaffected and bytes appear in `<team>/workers/<n>/stderr.log`. *Verified by `internal/team.TestSubprocessTransport_StderrIsolation`.*
- **D10.** GIVEN `TeamCreate(spec{Workers:25})`, THEN it returns `ErrTooManyWorkers`. AND GIVEN a team-worker that calls `TeamCreate`, THEN at depth 1 it returns `ErrTeamDepthExceeded`. *Verified by `internal/team.TestTeamForkBombGuard`.*
- **D11.** GIVEN a journal line with `"v": 99`, WHEN `team.Open(id)` is called, THEN it returns `ErrUnsupportedEnvelopeVersion` and refuses to start the team. *Verified by `internal/team.TestUnsupportedEnvelopeVersion`.*
- **D12.** GIVEN a team with 10 completed tasks AND a 200-entry journal, WHEN `team.json` and `tasks.json` are deleted from disk and `team.Open(id)` is called, THEN both cache files are reconstructed and `TaskList()` returns identical results to the pre-deletion state. *Verified by `internal/team.TestRebuildFromJournalAlone`.*

---

## 6. Risk Register (top 6)

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | **Hook-induced infinite loop** (a hook invokes `clue-code` which re-fires the same hook) | Medium | Critical (fork bomb, system unresponsive) | (a) `CLUE_CODE_HOOK_DEPTH` env-propagated counter, hard cap 3; (b) `doctor --hooks` flags self-invocation; (c) allowlist defaults to deny `clue-code` self-invoke unless `allow_self_invoke: true`; (d) hermetic A3 acceptance test (CRITIC-8) verifies depth guard returns `ErrHookDepthExceeded` without spawning a subprocess. ADR-1, Pre-mortem #1. |
| R2 | **State corruption on concurrent writes** (two skills clobber the notepad) | Medium | High (silent data loss) | (a) `flock` on every mutating op; (b) version-counter CAS via `WriteIfVersion`; (c) notepad is append-only by structure (sections), not in-place rewrite; (d) `state.Clear` is the only truncation path and requires explicit scope. ADR-2, Pre-mortem #2. |
| R3 | **TUI/CLI state divergence** (TUI shows a session that ended an hour ago) | High | Medium (user trust) | (a) `state.Watch()` via `fsnotify` pushes changes; (b) heartbeat + 30 s stale threshold; (c) explicit `r` reload key; (d) integration test C2 enforces 1 s convergence. Pre-mortem #3. |
| R4 | **IPC deadlock** (worker A waits on B, B waits on A) | Low | High (team hangs forever) | (a) Bounded mailboxes (256) — backpressure surfaces as `ErrMailboxFull` instead of blocking; (b) every blocking op accepts a `context.Context` with deadline; (c) `internal/team/stalled.go` detects no progress for 60 s and broadcasts `team-event:stalled` on a side-channel (clock injected via `clock.Clock` for testability — see D8); (d) `clue-code team inspect <id>` reports `state=stalled`, `last_progress_age`, and per-worker mailbox depth. |
| R5 | **Schema lock-in** (JSON layout becomes a compatibility contract before we know the right shape) | Medium | High (forces costly migrations later) | (a) Every state file has a top-level `"version": N` field; (b) loaders accept versions ≤ current and reject newer with helpful error; (c) public Go API (`state.Store`, `team.*`) is the contract — file layout is documented as "subject to change before v1.0"; (d) Phase 5 SQLite migration tool drafted in Phase 4 ADR follow-ups; (e) **leading-indicator counters** `state_write_contention_total{key,scope}` and `session_stale_lag_seconds` exposed via `clue-code state metrics` make the Pre-mortem #2 / #3 signals programmatically detectable, not user-reported. |
| R6 | **Envelope wire-format lock-in** (NDJSON wire format becomes ossified before we learn from real deployments) | Medium | Medium (forces protocol break) | (a) `Envelope.Version uint8` (json `"v"`) field defaults to `1` for Phase 4; (b) journal replay rejects `v > 1` with `ErrUnsupportedEnvelopeVersion` (D11); (c) Version field reserves header space for forward-compatible wire upgrades without breaking existing journals; (d) Phase 6 socket transport reuses the same envelope and inherits versioning for free. |

---

## 7. Phasing within Phase 4 (merge order + checkpoints)

**Track ordering rationale:** Track B (state) is the foundation Tracks A, C, and D all read from; Track A's `hooks.log` and Track D's `journal.ndjson` both live next to state; Track C is the only one that doesn't gate any other track and thus ships last so it can show off the others.

```
   ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
   │  B       │ →  │  A       │ →  │  D       │ →  │  C       │
   │  state   │    │  hooks   │    │  team    │    │  TUI     │
   └──────────┘    └──────────┘    └──────────┘    └──────────┘
        │              │               │                │
   Checkpoint α   Checkpoint β    Checkpoint γ      Checkpoint δ
```

- **Sub-phase 4.1 — Track B (state) + read-only TUI state inspector behind `tui` build tag** — 1 PR. Unblocks A, C, D. Includes `internal/state/**` AND `internal/tui/views/state.go` (~100 LOC, gated `//go:build tui`). Headless 4.1 binary stays charmbracelet-free because of the build tag (verified by C6 size-delta CI check).
  - **Checkpoint α (consensus re-validate):** Architect reviews `state.Store` interface (incl. `WriteWithRetry`, `Append`); Critic confirms ADR-2 honesty rationale and the Phase 5 SQLite gate (10 projects × 5 sessions) is documented. Required before merging A/C/D.
- **Sub-phase 4.2 — Track A (hooks) + Track A.5 (`internal/skillrunner/`)** — 1 PR (or fast-follow). Depends on B for `hooks.log` location and session registry. Includes `internal/hooks/**` (incl. `testdata/{trivial,recursive,infinite-loop}-hook.yaml`) AND `internal/skillrunner/{loader.go, engine.go, hooks_glue.go, engine_test.go, testdata/synthetic-skill, testdata/malformed-yaml-skill, testdata/recursive-skill}`. Acceptance criteria E1-E5 (Track A.5 lifecycle, recursion guard, panic-around-lifecycle, graceful cancel, ordered-events) merge in this PR.
  - **Checkpoint β:** Critic verifies depth guard implementation matches Pre-mortem #1 + the in-process per-session recursion counter (defense-in-depth against `unset CLUE_CODE_HOOK_DEPTH`); security-reviewer audits allowlist defaults against the §3.A.2 truth table; A3 hermetic test (CRITIC-8) lands.
- **Sub-phase 4.3 — Track D (team)** — 1 PR. Depends on B (locking helpers) and A (fires `PreToolUse`/`PostToolUse` around `TaskUpdate`). Ships BOTH `inproc.go` AND `subprocess.go` transports + `team demo` CLI + `cmd/clue-code/team_worker.go` subcommand. Acceptance criteria D1-D12 merge in this PR (D9 stderr isolation, D10 fork-bomb guard, D11 envelope version rejection, D12 journal-rebuild).
  - **Checkpoint γ:** Architect reviews wire-first `Transport` interface (NDJSON envelopes over `io.ReadWriter` with `Version` field); test-engineer confirms D6 latency assertion AND D7 subprocess demo AND D8 stalled-team detector AND D9-D12 are in CI.
- **Sub-phase 4.4 — Track C (TUI, full views)** — 1 PR. Depends on B/A/D being merged so all views have real data sources. Adds the remaining `internal/tui/views/{sessions,agents,skills,tokens,hooks}.go` and `cmd/clue-code/tui.go` + `tui_stub.go`.
  - **Checkpoint δ (final consensus):** All three reviewers (Architect/Critic/Security) sign off; release notes drafted; `CHANGELOG.md` updated; doctor command extended (`clue-code doctor` reports hook count, session count, team count); CI publishes BOTH artifacts (`clue-code` + `clue-code-tui`).

**Merge gate per checkpoint:** zero failing tests on `darwin/arm64` and `linux/amd64`, race detector clean (`go test -race ./...`), no new external deps beyond those listed in this plan, `go vet` and `staticcheck` clean.

---

## 8. Open Questions (for ralplan step 2 alignment)

- **Q1.** ~~Adopt `gopkg.in/yaml.v3` vs hand-roll YAML parser?~~ **CLOSED — adopt `gopkg.in/yaml.v3` (Apache-2.0).** Already in the dependency closure once Track C lands via charmbracelet; hand-rolling YAML invites bugs. Decision recorded.
- **Q2.** ~~`Inject:true` hook output verbatim or wrapped?~~ **CLOSED — wrap in `<hook-context source="<command-name>">…</hook-context>` for OMC parity.** Decision recorded.
- **Q3.** ~~Should `state.Clear(ScopeGlobal, "")` (clear-all-global) require an explicit confirmation flag at the CLI layer?~~ **CLOSED — yes, the `clue-code state clear --scope=global` invocation requires `--yes-i-know` flag; absent that flag, the CLI prints a refusal message and exits 2.** Decision recorded.
- **Q4.** ~~TUI default theme: lipgloss `adaptive` (auto light/dark) or hard-pinned dark?~~ **CLOSED — `lipgloss.AdaptiveColor` is the default; users may opt into hard-pinned themes via a future `~/.config/clue-code/tui.yaml` (Phase 5+).** Decision recorded.
- **Q5.** ~~Should `team.Team.SendMessage` to a non-existent recipient be `ErrUnknownAgent` (strict) or silently dropped + journaled (lenient)?~~ **CLOSED — strict: returns `ErrUnknownAgent`. Lenient hides bugs and complicates the wire-format invariant.** Decision recorded.
- **Q6.** ~~Hook threat model — what is Phase 4 actually defending against?~~ **CLOSED — Phase 4 trusts user-written hooks at shell-level (see §3.A.1). Defense-in-depth via env-var depth counter + in-process per-session map. Phase 5 follow-up: skill-shipped hook isolation.** Decision recorded.

All ralplan step-2 alignment questions (Q1-Q6) are CLOSED by this revision. No open questions remain.

---

## 9. Final Architecture Decision Record summary (for ralplan output)

| ID | Decision | Drivers | Alternatives | Why chosen | Consequences | Follow-ups |
|---|---|---|---|---|---|---|
| ADR-1 | Subprocess hooks with capture + opt-in injection | OMC parity, single binary, isolation | `plugin`, Lua/Starlark | Matches user mental model; trivial to debug; CGo-free | +5–15 ms cold-start; needs allowlist | Phase 5 fast-path for built-in hooks |
| ADR-2 | JSON files + flock for state (modernc SQLite NOT eliminated by Principle 2 — deferred on dogfood-affordance grounds) | Single binary, human-debuggable, alpha-stage debug ergonomics | CGo SQLite (eliminated), modernc SQLite (deferred), bbolt (eliminated), Hybrid kv+sqlite (deferred) | `cat | jq` and `git diff` are stronger affordances than SQL for an alpha-stage tool with ~5 keys; perf gate at 10 projects × 5 sessions is realistic, not theatrical | No SQL; manual locking discipline; 2-3 day Store interface refactor cost when gate triggers | Phase 5 perf gate → swap to modernc SQLite or Hybrid Option E |
| ADR-3 | Wire-first NDJSON `Transport` over `io.ReadWriter`, with TWO Phase-4 impls (`inproc.go` + `subprocess.go`); `Envelope` carries a `Version uint8` field defaulting to 1 | Laptop scale, single binary, parity with existing skills/team subprocess workers | Channel-shaped transport (eliminated — defers wire decision), Unix sockets (Phase 6), SQLite queue (eliminated) | One wire format spans in-proc, subprocess, journal, and replay — Phase 6 socket transport drops in unchanged; Version field reserves header space for forward-compat upgrades | Two impls double Phase-4 test surface; subprocess transport owns child lifetime + signals; recover() required at every boundary; subprocess `Recv()` returns `ErrTransportDead` on unclean child exit | Phase 6 socket transport (Unix domain sockets, same wire); Phase 7+ Hub for cross-machine; Phase 5+ may bump envelope version when adding fields |

---

*End of plan. Ready for Architect review (step 2) and Critic review (step 3) per the RALPLAN deliberate-mode protocol.*
