// Package state manages persistent agent state for CLUE CODE using a
// JSON-on-disk store with POSIX flock-based mutual exclusion.
//
// State is partitioned into three scopes:
//   - ScopeGlobal  — ~/.clue-code/state/ (shared across projects)
//   - ScopeProject — <project-root>/.clue-code/state/ (per-repo)
//   - ScopeSession — <project-root>/.clue-code/sessions/<id>/ (per-run)
//
// All writes are atomic (write-tmp + rename) and protected by an exclusive
// flock. Readers acquire a shared lock so concurrent reads are allowed.
package state
