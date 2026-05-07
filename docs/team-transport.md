# Team Transport — Developer & Operator Guide

> Phase 4.3 · `internal/team/` · Go 1.22+

---

## Wire Format

All team messages are exchanged as **NDJSON envelopes** — one JSON object per line, terminated by `\n`. Each envelope carries a version field so readers can reject forward-incompatible traffic immediately.

### Envelope Schema (v=1)

```json
{
  "v":       1,
  "seq":     42,
  "from":    "worker-0",
  "to":      "worker-1",
  "kind":    "ping",
  "payload": {"n": 42},
  "ts":      "2026-05-06T12:00:00.123456789Z"
}
```

| Field     | Type            | Required | Description |
|-----------|-----------------|----------|-------------|
| `v`       | `uint8`         | yes      | Envelope version. Always `1` for Phase 4. Readers must reject `v > 1` with `ErrUnsupportedEnvelopeVersion`. |
| `seq`     | `uint64`        | yes      | Monotonic sequence number per team. Used for journal ordering and replay deduplication. |
| `from`    | `string`        | yes      | Sender identity, e.g. `"worker-0"`, `"team-lead"`. |
| `to`      | `string`        | yes      | Recipient identity. Use `"broadcast:*"` for fan-out. |
| `kind`    | `string`        | yes      | Message type. Conventions: `"ping"`, `"ack"`, `"task-update"`, `"agent-down"`, `"team-event:stalled"`. |
| `payload` | `json.RawMessage` | no     | Arbitrary JSON. Omitted when empty. |
| `ts`      | RFC 3339 nano   | yes      | UTC timestamp at send time. |

**Wire constants (Go):**

```go
const EnvelopeVersion uint8 = 1
const maxScanTokenSize = 10 * 1024 * 1024 // 10 MiB per line
```

**Codec helpers:**

```go
// Serialise one envelope as "JSON\n" into w.
func EncodeEnvelope(w io.Writer, e Envelope) error

// Advance scanner and decode the next envelope.
func DecodeNext(s *bufio.Scanner) (Envelope, error)
```

---

## Journal Layout

Every team owns a directory under the **project root**:

```
<project>/.clue-code/teams/<team-id>/
  journal.ndjson          # append-only event log — source of truth
  team.json               # derived cache: team metadata snapshot
  tasks.json              # derived cache: task-graph snapshot
  workers/
    <worker-id>/
      stderr.log          # child stderr, rotated at 4 MiB, keep 2 copies
```

### Invariant

> `journal.ndjson` is truth. `team.json` and `tasks.json` are **derived caches**. On conflict or corruption, both cache files are rebuilt from the journal by `team.Open(id)`.

### Torn-Tail Handling

On `Open(id)`, the journal is scanned byte-by-byte. The last `\n` is the **high-water mark**. Any bytes after it (a partial write interrupted by crash) are discarded. A `slog warn` is emitted with `truncation_offset_bytes` so operators can observe the event.

### Unsupported Version

A journal line with `"v": 99` (or any value `> 1`) causes `Open()` to return `ErrUnsupportedEnvelopeVersion` immediately and refuse to start the team.

---

## Transports

### Inproc Transport

```go
t1, t2 := team.NewInprocPair()
```

- Implemented via `io.Pipe()` pairs (stdlib only, zero allocations on the fast path).
- Both ends live **in the same process** — used for unit tests and the `team demo --transport=inproc` hello-world.
- `Send` serialises via `EncodeEnvelope`; `Recv` deserialises via `DecodeNext`.
- `Close()` closes the underlying pipe, unblocking any pending `Recv`.

### Subprocess Transport

```go
tr, err := team.NewSubprocessTransport(teamID, workerID, projectRoot)
```

- Spawns `os.Executable()` (resolved once at `TeamCreate`) with subcommand `team-worker --team-id=<id> --worker-id=<n>`.
- **PATH lookup is forbidden** — the binary identity is the calling process itself.
- Child **stdout** is the NDJSON wire: parsed by `subprocess.Recv()`.
- Child **stdin** receives outbound `SendEnvelope` calls.
- Child **stderr** is redirected to `<project>/.clue-code/teams/<team-id>/workers/<worker-id>/stderr.log` (rotated at 4 MiB, keep 2, via `gopkg.in/natefinch/lumberjack.v2`).
- `Close()` sends SIGTERM; escalates to SIGKILL after 3 seconds if the child has not exited.

### Future: Socket Transport (Phase 6)

Phase 6 will add a socket-based transport for cross-machine or cross-process workers. The `Transport` interface is designed to accommodate this without changes to callers:

```go
type Transport interface {
    Send(env Envelope) error
    Recv() (Envelope, error)
    Close() error
}
```

`TeamSpec.WorkerSpecs []WorkerSpec` (Phase 6 addition) will carry per-worker `Cmd`, `WorkingDirectory`, `WorktreePath`, and `Branch`. Phase 4 callers using `Workers int` continue to work unchanged.

---

## Fork-Bomb Caps

Two independent guards prevent runaway process trees:

| Guard | Constant / Env Var | Value | Error Returned |
|-------|--------------------|-------|----------------|
| Worker count cap | `MaxTeamWorkers` | **20** | `ErrTooManyWorkers` |
| Nesting depth cap | `CLUE_CODE_TEAM_DEPTH` | **1** | `ErrTeamDepthExceeded` |

`MaxTeamWorkers = 20` is enforced in `TeamCreate`. Passing `Workers: 25` returns `ErrTooManyWorkers` immediately.

`CLUE_CODE_TEAM_DEPTH` is an env-propagated counter. A team-worker process inherits the variable from its parent. At depth 1, any attempt to call `TeamCreate` returns `ErrTeamDepthExceeded`. Phase 6+ may raise the cap; the env var is the lever — no code change required.

**Test:** `internal/team.TestTeamForkBombGuard` (acceptance criterion D10).

---

## Operator Guide — CLI

The `clue-code team` subcommand provides read-only operator views plus a demo runner. Team **creation** is driven by skills; the CLI does not create teams (except `team demo`).

### `clue-code team list [project-root]`

Lists all teams found under `<project-root>/.clue-code/teams/`.

```
$ clue-code team list
ID              WORKERS  CREATED_AT
fix-ts-errors   4        2026-05-06T12:00:00Z
refactor-auth   2        2026-05-06T11:30:00Z
```

If no teams exist:

```
$ clue-code team list
no teams found
```

Project root defaults to `$CLUE_CODE_PROJECT_ROOT` or `cwd`.

### `clue-code team inspect <team-id> [project-root]`

Opens the team (replaying journal if caches are missing) and prints a summary.

```
$ clue-code team inspect fix-ts-errors
Team:    fix-ts-errors
Workers: 4
Tasks:   7 total
  pending    2
  running    1
  blocked    1
  completed  3
```

If the team is stalled (no progress for 60 s):

```
$ clue-code team inspect fix-ts-errors
Team:    fix-ts-errors
Workers: 4
Tasks:   3 total
  running    3
State:   stalled
Last progress age: 62s
```

### `clue-code team tail <team-id> [project-root]`

Follows `journal.ndjson` in real time (seeks to end, then polls every 100 ms). Each new envelope is printed as a raw NDJSON line. Exit with `Ctrl-C`.

```
$ clue-code team tail fix-ts-errors
{"v":1,"seq":201,"from":"worker-0","to":"team-lead","kind":"task-update","payload":{"status":"done"},"ts":"2026-05-06T12:01:00Z"}
{"v":1,"seq":202,"from":"worker-1","to":"team-lead","kind":"task-update","payload":{"status":"done"},"ts":"2026-05-06T12:01:01Z"}
^C
```

### `clue-code team demo --transport={inproc,subprocess}`

Runs a self-contained demonstration:

- Spawns **2 workers**, each exchanges **100 messages** (ping + ack), then exits cleanly.
- `inproc`: workers run as goroutines in the same process — fast, no subprocess overhead.
- `subprocess`: workers are spawned as child `clue-code team-worker` processes; journal accumulates exactly 200 NDJSON entries.

```
$ clue-code team demo --transport=inproc
demo (inproc): 2 workers, 200 sent, 200 acks received

$ clue-code team demo --transport=subprocess
demo (subprocess): 2 workers, 200 sent, 200 acks received
```

Exit code 0 on success, 1 on any error.

---

## Stalled Detection

A **stalled-team detector** runs as one goroutine per `*Team`, started in `TeamCreate` / `team.Open`, cancelled when `Team.Close()` is called.

**Threshold:** 60 seconds of no progress (no `TaskUpdate`, no `SendMessage`).

When stalled:
1. `team-event:stalled` envelope is broadcast on a side-channel.
2. `clue-code team inspect <id>` reports `state=stalled` and `last_progress_age=<N>s`.
3. Per-worker mailbox depths are included in the inspect output.

**Clock injection:** The detector uses a `clock.Clock` interface (from `internal/clock/clock.go`) so CI tests can inject a synthetic clock and trigger the 60 s threshold without sleeping. Tests do not actually wait 60 s.

**Goroutine cleanup:** `Team.Close()` closes a `done chan struct{}` that cancels the detector goroutine. The goroutine exits within 100 ms (verified via `runtime.NumGoroutine()` delta in `TestStalledTeamDetector`).

**Crash-resume:** On `team.Open`, the detector is re-armed using the journal's last-progress timestamp as the baseline, so a crash 45 s into a stalled period correctly fires after only 15 more seconds.

**Test:** `internal/team.TestStalledTeamDetector` (acceptance criterion D8).

---

## Panic Recovery

Every subprocess worker goroutine is wrapped in a `RunWorker` panic handler. When a panic is recovered:

1. The worker's owning task is marked `failed`.
2. An `agent-down` envelope is broadcast to all other workers.
3. The panicking goroutine exits; other workers continue unaffected.

No cascade — a single panicking worker cannot bring down the team.

**Test:** `internal/team.TestPanicRecovery` (acceptance criterion D4).

---

## Crash Resume

On `team.Open(id)`:

1. `journal.ndjson` is replayed from byte 0.
2. Torn tail (bytes after last `\n`) is discarded with a `slog warn`.
3. Task statuses are reconstructed from journal events — `max(seq)` is restored as the sequence counter baseline.
4. `team.json` and `tasks.json` caches are rebuilt if absent or inconsistent.
5. Stalled detector is re-armed using the journal's last-progress timestamp.

After `Open` returns, `TaskList()` returns results identical to the pre-crash state.

**Test:** `internal/team.TestCrashResume` (D3) and `internal/team.TestRebuildFromJournalAlone` (D12).

---

## Acceptance Checklist D1–D12

| ID | Test Name | Description |
|----|-----------|-------------|
| D1 | `TestFanOut` | 4 workers each create tasks and send messages; journal has exactly `4*(n_create+n_messages)` lines. |
| D2 | `TestScheduler_Topological` | Task graph `B→A`, `C→B`; A completes → B running, C blocked; B completes → C running. |
| D3 | `TestCrashResume` | Process killed mid-run; `team.Open` replays journal; task statuses match pre-crash truth. |
| D4 | `TestPanicRecovery` | Worker panic → owning task `failed`, `agent-down` broadcast, other workers continue. |
| D5 | `TestBackpressure` | Mailbox bounded at 256; 257th send returns `ErrMailboxFull` immediately (non-blocking). |
| D6 | `TestSendMessage_P99Latency` | 8 workers × 1000 messages over 10 s; p99 latency `< 1 ms` via HdrHistogram. |
| D7 | `TestSubprocessTransport_Demo` | `team demo --transport=subprocess`; 2 child processes, 100 msgs each direction, 200 journal entries, all exit 0. |
| D8 | `TestStalledTeamDetector` | Deadlocked workers + injected clock; `team-event:stalled` broadcast after 60 s; goroutine exits within 100 ms of `Close()`. |
| D9 | `TestSubprocessTransport_StderrIsolation` | Worker writes 1 MiB to stderr; parent stdout/stderr unaffected; bytes appear in `workers/<n>/stderr.log`. |
| D10 | `TestTeamForkBombGuard` | `Workers:25` → `ErrTooManyWorkers`; nested `TeamCreate` at depth 1 → `ErrTeamDepthExceeded`. |
| D11 | `TestUnsupportedEnvelopeVersion` | Journal line with `"v":99`; `Open` returns `ErrUnsupportedEnvelopeVersion`. |
| D12 | `TestRebuildFromJournalAlone` | 10-task team, 200-entry journal; delete `team.json`+`tasks.json`; `Open` rebuilds both; `TaskList()` identical to pre-deletion. |
