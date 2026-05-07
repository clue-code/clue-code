package aider

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
)

// ErrMalformedOutput is returned when the aider output cannot be parsed at all.
var ErrMalformedOutput = errors.New("aider: malformed output")

// ParseAiderOutput scans aider's stdout for edited-file markers and assembles
// a short summary from the remaining lines.
//
// Aider emits lines of the form:
//
//	Edited file: path/to/file.go
//
// Everything else is treated as summary material.
//
// The function never panics; malformed or empty input is handled gracefully.
func ParseAiderOutput(output []byte) (filesChanged []string, summary string, err error) {
	if len(output) == 0 {
		return nil, "", nil
	}

	var summaryLines []string
	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()

		// Primary marker used by modern aider versions.
		if after, ok := trimPrefix(line, "Edited file:"); ok {
			f := strings.TrimSpace(after)
			if f != "" {
				filesChanged = append(filesChanged, f)
			}
			continue
		}

		// Secondary marker found in some aider builds.
		if after, ok := trimPrefix(line, "Modified file:"); ok {
			f := strings.TrimSpace(after)
			if f != "" {
				filesChanged = append(filesChanged, f)
			}
			continue
		}

		summaryLines = append(summaryLines, line)
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return filesChanged, strings.Join(summaryLines, "\n"), ErrMalformedOutput
	}

	summary = buildSummary(summaryLines)
	return filesChanged, summary, nil
}

// trimPrefix reports whether s starts with prefix (case-sensitive) and returns
// the remainder of the string after the prefix.
func trimPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

// buildSummary collapses the non-file lines into a short summary string.
// It strips blank lines and limits output to the first 5 meaningful lines.
func buildSummary(lines []string) string {
	const maxLines = 5
	var kept []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		kept = append(kept, trimmed)
		if len(kept) >= maxLines {
			break
		}
	}
	return strings.Join(kept, " | ")
}
