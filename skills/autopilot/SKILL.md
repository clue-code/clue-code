---
name: autopilot
description: Full autonomous execution from idea to working code
---

You are running as the **autopilot** skill inside clue-code.

Session: {{.SessionID}}
Project: {{.ProjectRoot}}
Shell: {{.UserShell}}
Task: {{range .SkillArgs}}{{.}} {{end}}

<purpose>
Autopilot takes a brief product idea and autonomously handles the full lifecycle:
requirements analysis, technical design, planning, parallel implementation, QA cycling,
and multi-perspective validation. It produces working, verified code from a short description.
</purpose>

<when_to_use>
- User wants end-to-end autonomous execution from an idea to working code
- Task requires multiple coordinated phases: planning, coding, testing, and validation
- User wants hands-off execution and is willing to let the system run to completion
</when_to_use>

<do_not_use_when>
- User wants to explore options or brainstorm — respond conversationally
- User wants a single focused code change — use ralph or delegate directly to executor
- Task is a quick fix or small bug — use direct executor delegation
</do_not_use_when>

<execution_policy>
- Each phase must complete before the next begins
- Parallel execution is used within phases where possible
- QA cycles repeat up to 5 times; if the same error persists 3 times, stop and report
- Validation requires approval from all reviewers; fix and re-validate on rejection
- Cancel gracefully at any time; progress is preserved for resume
</execution_policy>

<steps>

## Phase 0 — Expansion

Turn the user task into a detailed specification.

- If a ralplan consensus plan exists at `.clue-code/plans/ralplan-*.md` or
  `.clue-code/plans/consensus-*.md`: **skip Phase 0 and Phase 1** — jump directly
  to Phase 2. The plan has already been Planner/Architect/Critic validated.
- If a deep-interview spec exists at `.clue-code/specs/deep-interview-*.md`:
  skip analyst+architect expansion and use it directly as Phase 0 output.
- If the task is vague (no file paths, function names, or concrete anchors):
  offer a redirect to the deep-interview skill for Socratic clarification first.
- Otherwise: extract requirements (analyst role), then create technical specification
  (architect role). Output: `.clue-code/autopilot/spec.md`.

## Phase 1 — Planning

Create an implementation plan from the Phase 0 spec.

- If a ralplan consensus plan exists: skip — already done.
- Architect role: create plan in direct mode (no interview).
- Critic role: validate the plan.
- Output: `.clue-code/plans/autopilot-impl.md`.

## Phase 2 — Execution

Implement the plan. Run independent tasks in parallel.

- Simple tasks: executor (L0)
- Standard tasks: executor (L1)
- Complex tasks: executor (L2)

## Phase 3 — QA

Cycle until all tests pass.

- Build, lint, test, fix failures in a loop.
- Repeat up to 5 cycles.
- Stop early if the same error recurs 3 times — this indicates a fundamental issue.

## Phase 4 — Validation

Run multi-perspective review in parallel:

- Architect: functional completeness
- Security-reviewer: vulnerability check
- Code-reviewer: quality review

All must approve. Fix rejected items and re-validate (up to 3 rounds).

## Phase 5 — Cleanup

On successful completion:

- Remove `.clue-code/state/autopilot-state.json`, `ralph-state.json`,
  `ultrawork-state.json`, `ultraqa-state.json`.
- Signal clean exit to the caller.

</steps>

<stop_conditions>
- Same QA error persists across 3 cycles → stop and report fundamental issue
- Validation keeps failing after 3 re-validation rounds → stop and report
- User says "stop", "cancel", or "abort" → stop immediately
- Task is vague and expansion produces an unclear spec → offer deep-interview redirect
</stop_conditions>

<final_checklist>
- [ ] All 5 phases completed (Expansion, Planning, Execution, QA, Validation)
- [ ] All validators approved in Phase 4
- [ ] Tests pass (verified with fresh test run output, not assumed)
- [ ] Build succeeds (verified with fresh build output)
- [ ] State files cleaned up
- [ ] User informed of completion with a summary of what was built
</final_checklist>

<configuration>
The following settings in `clue-code.yaml` control this skill:

  autopilot.maxIterations: 10
  autopilot.maxQaCycles: 5
  autopilot.maxValidationRounds: 3
  autopilot.pauseAfterExpansion: false
  autopilot.pauseAfterPlanning: false
  autopilot.skipQa: false
  autopilot.skipValidation: false

If autopilot was cancelled or failed, invoke the skill again with the same task
to resume from the last completed phase.
</configuration>
