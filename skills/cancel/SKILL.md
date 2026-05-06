---
name: cancel
description: Terminate the active CLUE CODE mode with a structured ack and state cleanup
level: 2
---

# Cancel

Terminates the active CLUE CODE mode and clears its state.

## Steps

1. **Detect active mode** — call `state_list_active` to find the current session's
   active mode (`autopilot`, `ralph`, `ultrawork`, `ultraqa`, `team`, etc.).
   If none, report and stop.

2. **Emit structured ack** — before touching any state, print:

   ```
   Cancelling <mode> (session: <session_id>)...
   ```

3. **Clear state** — call `state_clear(mode=<mode>, session_id=<session_id>)`.
   For autopilot only: call `state_write(mode=autopilot, state={active:false})`
   instead, to preserve resume data.

4. **Confirm** — print the mode-specific success message from the table below
   and stop execution.

## Success messages

| Mode | Message |
|------|---------|
| autopilot | Autopilot cancelled. Progress preserved — run `/clue-code:autopilot` to resume. |
| ralph | Ralph cancelled. |
| ultrawork | Ultrawork cancelled. |
| ultraqa | UltraQA cancelled. |
| team | Team cancelled. Teammates shut down. |
| (none) | No active CLUE CODE mode detected. |
| (force) | All CLUE CODE state cleared. |

## Force clear

Pass `--force` to clear every session and all legacy state files regardless of
what is currently active:

```
/clue-code:cancel --force
```

## Invocation

```
/clue-code:cancel
/clue-code:cancel --force
```

Or say: "cancel clue", "stop clue".
