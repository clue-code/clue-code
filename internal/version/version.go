// Package version exposes the CLUE CODE build metadata.
package version

// Build metadata populated via -ldflags at build time.
// Default values represent an unreleased/dev build.
var (
	// Version is the semantic version of the build.
	Version = "0.1.0-dev"
	// Commit is the git commit SHA of the build.
	Commit = "unknown"
	// Date is the ISO-8601 build timestamp.
	Date = "unknown"
)

// String returns a human-readable representation of the build.
func String() string {
	return "clue-code " + Version + " (" + Commit + ", " + Date + ")"
}
