package team

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// MaxTeamWorkers is the maximum number of workers allowed in a single team.
const MaxTeamWorkers = 20

// TeamDepthEnvVar is the environment variable that tracks nested team depth.
const TeamDepthEnvVar = "CLUE_CODE_TEAM_DEPTH"

// MaxTeamDepth is the maximum allowed nesting depth (0-indexed: 0 = top-level,
// 1 = one level deep — disallowed in Phase 4).
const MaxTeamDepth = 1

var (
	// ErrTooManyWorkers is returned when the requested worker count exceeds MaxTeamWorkers.
	ErrTooManyWorkers = errors.New("team: too many workers (max 20)")

	// ErrTeamDepthExceeded is returned when the current process is already
	// running inside a team (nested teams are not supported in Phase 4).
	ErrTeamDepthExceeded = errors.New("team: depth exceeded (max 1, no nested teams in Phase 4)")
)

// CheckTeamSize returns ErrTooManyWorkers when workers exceeds MaxTeamWorkers.
func CheckTeamSize(workers int) error {
	if workers > MaxTeamWorkers {
		return fmt.Errorf("%w: requested %d", ErrTooManyWorkers, workers)
	}
	return nil
}

// CheckDepth reads the CLUE_CODE_TEAM_DEPTH environment variable and returns
// ErrTeamDepthExceeded when the current depth is already at or above MaxTeamDepth.
func CheckDepth() error {
	raw := os.Getenv(TeamDepthEnvVar)
	if raw == "" {
		return nil // depth 0 — top-level, allowed
	}
	depth, err := strconv.Atoi(raw)
	if err != nil {
		// Malformed value — treat as 0 to avoid hard failures on env corruption.
		return nil
	}
	if depth >= MaxTeamDepth {
		return fmt.Errorf("%w: current depth %d", ErrTeamDepthExceeded, depth)
	}
	return nil
}

// IncrementedDepthEnv returns a slice of "KEY=VALUE" strings that should be
// added to a subprocess environment so that nested workers see the incremented
// depth. Callers should merge this into os.Environ() before exec.
func IncrementedDepthEnv() []string {
	raw := os.Getenv(TeamDepthEnvVar)
	current := 0
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			current = n
		}
	}
	return []string{fmt.Sprintf("%s=%d", TeamDepthEnvVar, current+1)}
}
