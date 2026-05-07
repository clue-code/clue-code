---
name: ultrawork
description: Parallel execution engine for high-throughput task completion
level: 4
---

<Purpose>
Ultrawork is a parallel execution engine that runs multiple agents simultaneously for independent tasks. It is a component, not a standalone persistence mode -- it provides parallelism and smart model routing but not persistence, verification loops, or state management.
</Purpose>

<Use_When>
- Multiple independent tasks can run simultaneously
- User says "ulw", "ultrawork", or wants parallel execution
- You need to delegate work to multiple agents at once
- Task benefits from concurrent execution but the user will manage completion themselves
</Use_When>

<Do_Not_Use_When>
- Task requires guaranteed completion with verification -- use `ralph` instead (ralph includes ultrawork)
- Task requires a full autonomous pipeline -- use `autopilot` instead (autopilot includes ralph which includes ultrawork)
- There is only one sequential task with no parallelism opportunity -- delegate directly to an executor agent
- User needs session persistence for resume -- use `ralph` which adds persistence on top of ultrawork
</Do_Not_Use_When>

<Why_This_Exists>
Sequential task execution wastes time when tasks are independent. Ultrawork enables firing multiple agents simultaneously and routing each to the right model tier, reducing total execution time while controlling token costs. It is designed as a composable component that ralph and autopilot layer on top of.
</Why_This_Exists>

<Execution_Policy>
- Fire all independent agent calls simultaneously -- never serialize independent work
- Always pass the `model` parameter explicitly when delegating
- Read `docs/agent-tiers.md` before first delegation for agent selection guidance
- Use `run_in_background: true` for operations over ~30 seconds (installs, builds, tests)
- Run quick commands (git status, file reads, simple checks) in the foreground
</Execution_Policy>

<Steps>
1. **Read agent reference**: Load `docs/agent-tiers.md` for tier selection
2. **Classify tasks by independence**: Identify which tasks can run in parallel vs which have dependencies
3. **Route to correct tiers**:
   - Simple lookups/definitions: L0 tier
   - Standard implementation: L1 tier
   - Complex analysis/refactoring: L2 tier
4. **Fire independent tasks simultaneously**: Launch all parallel-safe tasks at once
5. **Run dependent tasks sequentially**: Wait for prerequisites before launching dependent work
6. **Background long operations**: Builds, installs, and test suites use `run_in_background: true`
7. **Verify when all tasks complete** (lightweight):
   - Build/typecheck passes
   - Affected tests pass
   - No new errors introduced
</Steps>

<Tool_Usage>
- Delegate to the executor agent at L0 for simple changes
- Delegate to the executor agent at L1 for standard work
- Delegate to the executor agent at L2 for complex work
- Use `run_in_background: true` for package installs, builds, and test suites
- Use foreground execution for quick status checks and file operations
</Tool_Usage>

<Examples>
<Good>
Three independent tasks fired simultaneously:
```
delegate to executor (L0) "Add missing type export for Config interface"
delegate to executor (L1) "Implement the /api/users endpoint with validation"
delegate to executor (L1) "Add integration tests for the auth middleware"
```
Why good: Independent tasks at appropriate tiers, all fired at once.
</Good>

<Good>
Correct use of background execution:
```
delegate to executor (L1) "npm install && npm run build" run_in_background=true
delegate to executor (L0) "Update the README with new API endpoints"
```
Why good: Long build runs in background while short task runs in foreground.
</Good>

<Bad>
Sequential execution of independent work:
```
result1 = delegate(executor, "Add type export")  # wait...
result2 = delegate(executor, "Implement endpoint")     # wait...
result3 = delegate(executor, "Add tests")              # wait...
```
Why bad: These tasks are independent. Running them sequentially wastes time.
</Bad>

<Bad>
Wrong tier selection:
```
delegate to executor (L2) "Add a missing semicolon"
```
Why bad: L2 is expensive overkill for a trivial fix. Use executor at L0 instead.
</Bad>
</Examples>

<Escalation_And_Stop_Conditions>
- When ultrawork is invoked directly (not via ralph), apply lightweight verification only -- build passes, tests pass, no new errors
- For full persistence and comprehensive architect verification, recommend switching to `ralph` mode
- If a task fails repeatedly across retries, report the issue rather than retrying indefinitely
- Escalate to the user when tasks have unclear dependencies or conflicting requirements
</Escalation_And_Stop_Conditions>

<Final_Checklist>
- [ ] All parallel tasks completed
- [ ] Build/typecheck passes
- [ ] Affected tests pass
- [ ] No new errors introduced
</Final_Checklist>

<Team_Integration>
## Using internal/team from Ultrawork

When ultrawork fires parallel executor agents that need inter-agent messaging,
they can use the `internal/team` API directly. Key entry points:

```go
import "github.com/clue-code/clue-code/internal/team"

// Create a team for the parallel batch.
t, err := team.TeamCreate(team.Spec{Workers: n, ProjectRoot: root})

// Deliver a message from one worker to another (non-blocking).
err = t.SendMessage(from, to, payload)

// Read from a worker's inbox.
inbox, err := t.Inbox(agentRef)
msg := <-inbox

// Tear down when done.
t.Close()
```

See `docs/team-transport.md` for wire format, journal layout, and operator CLI.
The `clue-code team list|inspect|tail|demo` commands provide read-only views
into any running team created by an ultrawork batch.
</Team_Integration>

<Advanced>
## Relationship to Other Modes

```
ralph (persistence wrapper)
 \-- includes: ultrawork (this skill)
     \-- provides: parallel execution only

autopilot (autonomous execution)
 \-- includes: ralph
     \-- includes: ultrawork (this skill)
```

Ultrawork is the parallelism layer. Ralph adds persistence and verification. Autopilot adds the full lifecycle pipeline.
</Advanced>
