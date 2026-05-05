---
name: cancel
description: Cancel any active CLUE CODE mode (autopilot, ralph, ultrawork, ultraqa, swarm, ultrapilot, pipeline, team)
level: 2
---

# Cancel Skill

Intelligent cancellation that detects and cancels the active CLUE CODE mode.

**The cancel skill is the standard way to complete and exit any CLUE CODE mode.**
When the stop hook detects work is complete, it instructs the LLM to invoke
this skill for proper state cleanup. If cancel fails or is interrupted,
retry with `--force` flag, or wait for the 2-hour staleness timeout as
a last resort.

## What It Does

Automatically detects which mode is active and cancels it:
- **Autopilot**: Stops workflow, preserves progress for resume
- **Ralph**: Stops persistence loop, clears linked ultrawork if applicable
- **Ultrawork**: Stops parallel execution (standalone or linked)
- **UltraQA**: Stops QA cycling workflow
- **Swarm**: Stops coordinated agent swarm, releases claimed tasks
- **Ultrapilot**: Stops parallel autopilot workers
- **Pipeline**: Stops sequential agent pipeline
- **Team**: Sends shutdown_request to all teammates, waits for responses, deletes team, clears linked ralph if present
- **Team+Ralph (linked)**: Cancels team first (graceful shutdown), then clears ralph state. Cancelling ralph when linked also cancels team first.

## Usage

```
/clue-code:cancel
```

Or say: "cancelclue", "stopclue".

## State Tooling

The orchestrator's state-management primitives (`state_clear`, `state_read`, `state_write`,
`state_list_active`, `state_get_status`) are provided by the CLUE CODE Phase 4 hook + state
system. They operate against `.clue-code/state/sessions/{sessionId}/...` for per-session
data, and `.clue-code/state/*.json` for legacy / shared modes.

If the state primitives are unavailable (e.g. running in a stripped-down mode), use this
**bash fallback** as an emergency escape from the stop-hook loop. This is NOT a full
replacement for the cancel flow — it only removes state files to unblock the session.
Linked modes (e.g. ralph→ultrawork, autopilot→ralph/ultraqa) must be cleared separately
by running the fallback once per mode.

Replace `MODE` with the specific mode (e.g. `ralplan`, `ralph`, `ultrawork`, `ultraqa`).

**WARNING:** Do NOT use this fallback for `autopilot` or `clue-teams`. Autopilot requires
`state_write(active=false)` to preserve resume data. clue-teams requires tmux session
cleanup that cannot be done via file deletion alone.

```bash
# Fallback: direct file removal when state_clear is unavailable
SESSION_ID="${CLUE_CODE_SESSION_ID:-}"
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || { d="$PWD"; while [ "$d" != "/" ] && [ ! -d "$d/.clue-code" ]; do d="$(dirname "$d")"; done; echo "$d"; })"

# Cross-platform SHA-256 (macOS: shasum, Linux: sha256sum)
sha256portable() { printf '%s' "$1" | (sha256sum 2>/dev/null || shasum -a 256) | cut -c1-16; }

# Resolve state directory (supports CLUE_CODE_STATE_DIR centralized storage)
if [ -n "${CLUE_CODE_STATE_DIR:-}" ]; then
  SOURCE="$(git remote get-url origin 2>/dev/null || echo "$REPO_ROOT")"
  HASH="$(sha256portable "$SOURCE")"
  DIR_NAME="$(basename "$REPO_ROOT" | sed 's/[^a-zA-Z0-9_-]/_/g')"
  CLUE_STATE="$CLUE_CODE_STATE_DIR/${DIR_NAME}-${HASH}/state"
  [ ! -d "$CLUE_STATE" ] && { echo "ERROR: State dir not found at $CLUE_STATE" >&2; exit 1; }
elif [ "$REPO_ROOT" != "/" ] && [ -d "$REPO_ROOT/.clue-code" ]; then
  CLUE_STATE="$REPO_ROOT/.clue-code/state"
else
  echo "ERROR: Could not locate .clue-code state directory" >&2
  exit 1
fi
MODE="ralplan"  # <-- replace with the target mode

# Clear session-scoped state for the specific mode
if [ -n "$SESSION_ID" ] && [ -d "$CLUE_STATE/sessions/$SESSION_ID" ]; then
  rm -f "$CLUE_STATE/sessions/$SESSION_ID/${MODE}-state.json"
  rm -f "$CLUE_STATE/sessions/$SESSION_ID/${MODE}-stop-breaker.json"
  # Write cancel signal so stop hook detects cancellation in progress
  NOW_ISO="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  printf '{"active":true,"requested_at":"%s","mode":"%s","source":"bash_fallback"}' \
    "$NOW_ISO" "$MODE" > "$CLUE_STATE/sessions/$SESSION_ID/cancel-signal-state.json"
fi

# Clear legacy state only if no session ID (avoid clearing another session's state)
if [ -z "$SESSION_ID" ]; then
  rm -f "$CLUE_STATE/${MODE}-state.json"
fi
```

## Auto-Detection

`/clue-code:cancel` follows the session-aware state contract:
- By default the command inspects the current session via `state_list_active` and `state_get_status`, navigating `.clue-code/state/sessions/{sessionId}/…` to discover which mode is active.
- When a session id is provided or already known, that session-scoped path is authoritative. Legacy files in `.clue-code/state/*.json` are consulted only as a compatibility fallback if the session id is missing or empty.
- Swarm is a shared SQLite/marker mode (`.clue-code/state/swarm.db` / `.clue-code/state/swarm-active.marker`) and is not session-scoped.
- The default cleanup flow calls `state_clear` with the session id to remove only the matching session files; modes stay bound to their originating session.

Active modes are cancelled in dependency order:
1. Autopilot (includes linked ralph/ultraqa cleanup)
2. Ralph (cleans its linked ultrawork)
3. Ultrawork (standalone)
4. UltraQA (standalone)
5. Swarm (standalone)
6. Ultrapilot (standalone)
7. Pipeline (standalone)
8. Team (CLUE CODE native)
9. CLUE Teams (tmux CLI workers)
10. Plan Consensus (standalone)

## Force Clear All

Use `--force` or `--all` when you need to erase every session plus legacy artifacts, e.g., to reset the workspace entirely.

```
/clue-code:cancel --force
```

```
/clue-code:cancel --all
```

Steps under the hood:
1. `state_list_active` enumerates `.clue-code/state/sessions/{sessionId}/…` to find every known session.
2. `state_clear` runs once per session to drop that session’s files.
3. A global `state_clear` without `session_id` removes legacy files under `.clue-code/state/*.json`, `.clue-code/state/swarm*.db`, and compatibility artifacts (see list).
4. Team artifacts (`~/.clue-code/teams/*/`, `~/.clue-code/tasks/*/`, `.clue-code/state/team-state.json`) are best-effort cleared as part of the legacy fallback.
   - Cancel for native team does NOT affect clue-teams state, and vice versa.

Every `state_clear` command honors the `session_id` argument, so even force mode still uses the session-aware paths first before deleting legacy files.

Legacy compatibility list (removed only under `--force`/`--all`):
- `.clue-code/state/autopilot-state.json`
- `.clue-code/state/ralph-state.json`
- `.clue-code/state/ralph-plan-state.json`
- `.clue-code/state/ralph-verification.json`
- `.clue-code/state/ultrawork-state.json`
- `.clue-code/state/ultraqa-state.json`
- `.clue-code/state/swarm.db`
- `.clue-code/state/swarm.db-wal`
- `.clue-code/state/swarm.db-shm`
- `.clue-code/state/swarm-active.marker`
- `.clue-code/state/swarm-tasks.db`
- `.clue-code/state/ultrapilot-state.json`
- `.clue-code/state/ultrapilot-ownership.json`
- `.clue-code/state/pipeline-state.json`
- `.clue-code/state/clue-teams-state.json`
- `.clue-code/state/plan-consensus.json`
- `.clue-code/state/ralplan-state.json`
- `.clue-code/state/boulder.json`
- `.clue-code/state/hud-state.json`
- `.clue-code/state/subagent-tracking.json`
- `.clue-code/state/subagent-tracker.lock`
- `.clue-code/state/rate-limit-daemon.pid`
- `.clue-code/state/rate-limit-daemon.log`
- `.clue-code/state/checkpoints/` (directory)
- `.clue-code/state/sessions/` (empty directory cleanup after clearing sessions)

## Implementation Steps

When you invoke this skill:

### 1. Parse Arguments

```bash
# Check for --force or --all flags
FORCE_MODE=false
if [[ "$*" == *"--force"* ]] || [[ "$*" == *"--all"* ]]; then
  FORCE_MODE=true
fi
```

### 2. Detect Active Modes

The skill relies on the session-aware state contract rather than hard-coded file paths:
1. Call `state_list_active` to enumerate `.clue-code/state/sessions/{sessionId}/…` and discover every active session.
2. For each session id, call `state_get_status` to learn which mode is running (`autopilot`, `ralph`, `ultrawork`, etc.) and whether dependent modes exist.
3. If a `session_id` was supplied to `/clue-code:cancel`, skip legacy fallback entirely and operate solely within that session path; otherwise, consult legacy files in `.clue-code/state/*.json` only if the state tools report no active session. Swarm remains a shared SQLite/marker mode outside session scoping.
4. Any cancellation logic in this doc mirrors the dependency order discovered via state tools (autopilot → ralph → …).

### 3A. Force Mode (if --force or --all)

Use force mode to clear every session plus legacy artifacts via `state_clear`. Direct file removal is reserved for legacy cleanup when the state tools report no active sessions.

### 3B. Smart Cancellation (default)

#### If Team Active (CLUE CODE native)

Teams are detected by checking for config files in `~/.clue-code/teams/`:

```bash
# Check for active teams
TEAM_CONFIGS=$(find ~/.clue-code/teams -name config.json -maxdepth 2 2>/dev/null)
```

**Two-pass cancellation protocol:**

**Pass 1: Graceful Shutdown**
```
For each team found in ~/.clue-code/teams/:
  1. Read config.json to get team_name and members list
  2. For each non-lead member:
     a. Send shutdown_request via team messaging
     b. Wait up to 15 seconds for shutdown_response
     c. If response received: member terminates and is auto-removed
     d. If timeout: mark member as unresponsive, continue to next
  3. Log: "Graceful pass: X/Y members responded"
```

**Pass 2: Reconciliation**
```
After graceful pass:
  1. Re-read config.json to check remaining members
  2. If only lead remains (or config is empty): proceed to team deletion
  3. If unresponsive members remain:
     a. Wait 5 more seconds (they may still be processing)
     b. Re-read config.json again
     c. If still stuck: attempt team deletion anyway
     d. If team deletion fails: report manual cleanup path
```

**Team deletion + Cleanup:**
```
  1. Delete team — removes ~/.clue-code/teams/{name}/ and ~/.clue-code/tasks/{name}/
  2. Clear team state: state_clear(mode="team")
  3. Check for linked ralph: state_read(mode="ralph") — if linked_team is true:
     a. Clear ralph state: state_clear(mode="ralph")
     b. Clear linked ultrawork if present: state_clear(mode="ultrawork")
  4. Run orphan scan (see below)
  5. Emit structured cancel report
```

**Orphan Detection (Post-Cleanup):**

After team deletion, verify no agent processes remain:
```bash
node "${CLUE_CODE_HOME}/scripts/cleanup-orphans.mjs" --team-name "{team_name}"
```

The orphan scanner:
1. Checks `ps aux` (Unix) or `tasklist` (Windows) for processes with `--team-name` matching the deleted team
2. For each orphan whose team config no longer exists: sends SIGTERM, waits 5s, sends SIGKILL if still alive
3. Reports cleanup results as JSON

Use `--dry-run` to inspect without killing. The scanner is safe to run multiple times.

**Structured Cancel Report:**
```
Team "{team_name}" cancelled:
  - Members signaled: N
  - Responses received: M
  - Unresponsive: K (list names if any)
  - Team deletion: success/failed
  - Manual cleanup needed: yes/no
    Path: ~/.clue-code/teams/{name}/ and ~/.clue-code/tasks/{name}/
```

**Implementation note:** The cancel skill is executed by the LLM, not as a bash script. When you detect an active team:
1. Read `~/.clue-code/teams/*/config.json` to find active teams
2. If multiple teams exist, cancel oldest first (by `createdAt`)
3. For each non-lead member, send `shutdown_request` via the team messaging primitive
4. Wait briefly for shutdown responses (15s per member timeout)
5. Re-read config.json to check for remaining members (reconciliation pass)
6. Delete the team to clean up
7. Clear team state: `state_clear(mode="team", session_id)`
8. Report structured summary to user

#### If Autopilot Active

Autopilot handles its own cleanup including linked ralph and ultraqa.

1. Read autopilot state via `state_read(mode="autopilot", session_id)` to get current phase
2. Check for linked ralph via `state_read(mode="ralph", session_id)`:
   - If ralph is active and has `linked_ultrawork: true`, clear ultrawork first: `state_clear(mode="ultrawork", session_id)`
   - Clear ralph: `state_clear(mode="ralph", session_id)`
3. Check for linked ultraqa via `state_read(mode="ultraqa", session_id)`:
   - If active, clear it: `state_clear(mode="ultraqa", session_id)`
4. Mark autopilot inactive (preserve state for resume) via `state_write(mode="autopilot", session_id, state={active: false, ...existing})`

#### If Ralph Active (but not Autopilot)

1. Read ralph state via `state_read(mode="ralph", session_id)` to check for linked ultrawork
2. If `linked_ultrawork: true`:
   - Read ultrawork state to verify `linked_to_ralph: true`
   - If linked, clear ultrawork: `state_clear(mode="ultrawork", session_id)`
3. Clear ralph: `state_clear(mode="ralph", session_id)`

#### If Ultrawork Active (standalone, not linked)

1. Read ultrawork state via `state_read(mode="ultrawork", session_id)`
2. If `linked_to_ralph: true`, warn user to cancel ralph instead (which cascades)
3. Otherwise clear: `state_clear(mode="ultrawork", session_id)`

#### If UltraQA Active (standalone)

Clear directly: `state_clear(mode="ultraqa", session_id)`

#### No Active Modes

Report: "No active CLUE CODE modes detected. Use --force to clear all state files anyway."

## Implementation Notes

The cancel skill runs as follows:
1. Parse the `--force` / `--all` flags, tracking whether cleanup should span every session or stay scoped to the current session id.
2. Use `state_list_active` to enumerate known session ids and `state_get_status` to learn the active mode (`autopilot`, `ralph`, `ultrawork`, etc.) for each session.
3. When operating in default mode, call `state_clear` with that session_id to remove only the session’s files, then run mode-specific cleanup (autopilot → ralph → …) based on the state tool signals.
4. In force mode, iterate every active session, call `state_clear` per session, then run a global `state_clear` without `session_id` to drop legacy files (`.clue-code/state/*.json`, compatibility artifacts) and report success. Swarm remains a shared SQLite/marker mode outside session scoping.
5. Team artifacts (`~/.clue-code/teams/*/`, `~/.clue-code/tasks/*/`, `.clue-code/state/team-state.json`) remain best-effort cleanup items invoked during the legacy/global pass.

State tools always honor the `session_id` argument, so even force mode still clears the session-scoped paths before deleting compatibility-only legacy state.

Mode-specific subsections above describe what extra cleanup each handler performs after the state-wide operations finish.

## Messages Reference

| Mode | Success Message |
|------|-----------------|
| Autopilot | "Autopilot cancelled at phase: {phase}. Progress preserved for resume." |
| Ralph | "Ralph cancelled. Persistent mode deactivated." |
| Ultrawork | "Ultrawork cancelled. Parallel execution mode deactivated." |
| UltraQA | "UltraQA cancelled. QA cycling workflow stopped." |
| Swarm | "Swarm cancelled. Coordinated agents stopped." |
| Ultrapilot | "Ultrapilot cancelled. Parallel autopilot workers stopped." |
| Pipeline | "Pipeline cancelled. Sequential agent chain stopped." |
| Team | "Team cancelled. Teammates shut down and cleaned up." |
| Plan Consensus | "Plan Consensus cancelled. Planning session ended." |
| Force | "All CLUE CODE modes cleared. You are free to start fresh." |
| None | "No active CLUE CODE modes detected." |

## What Gets Preserved

| Mode | State Preserved | Resume Command |
|------|-----------------|----------------|
| Autopilot | Yes (phase, files, spec, plan, verdicts) | `/clue-code:autopilot` |
| Ralph | No | N/A |
| Ultrawork | No | N/A |
| UltraQA | No | N/A |
| Swarm | No | N/A |
| Ultrapilot | No | N/A |
| Pipeline | No | N/A |
| Plan Consensus | Yes (plan file path preserved) | N/A |

## Notes

- **Dependency-aware**: Autopilot cancellation cleans up Ralph and UltraQA
- **Link-aware**: Ralph cancellation cleans up linked Ultrawork
- **Safe**: Only clears linked Ultrawork, preserves standalone Ultrawork
- **Local-only**: Clears state files in `.clue-code/state/` directory
- **Resume-friendly**: Autopilot state is preserved for seamless resume
- **Team-aware**: Detects native CLUE CODE teams and performs graceful shutdown

## MCP Worker Cleanup

When cancelling modes that may have spawned worker daemons (team bridge), the cancel skill should also:

1. **Check for active workers**: Look for heartbeat files at `.clue-code/state/team-bridge/{team}/*.heartbeat.json`
2. **Send shutdown signals**: Write shutdown signal files for each active worker
3. **Kill tmux sessions**: Run `tmux kill-session -t clue-team-{team}-{worker}` for each worker
4. **Clean up heartbeat files**: Remove all heartbeat files for the team
5. **Clean up shadow registry**: Remove `.clue-code/state/team-workers.json`

### Force Clear Addition

When `--force` is used, also clean up:
```bash
rm -rf .clue-code/state/team-bridge/       # Heartbeat files
rm -f .clue-code/state/team-workers.json   # Shadow registry
# Kill all clue-team-* tmux sessions
tmux list-sessions -F '#{session_name}' 2>/dev/null | grep '^clue-team-' | while read s; do tmux kill-session -t "$s" 2>/dev/null; done
```
