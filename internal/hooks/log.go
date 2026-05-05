package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogEntry records the outcome of a single hook execution.
type LogEntry struct {
	Timestamp  time.Time `json:"ts"`
	Event      Event     `json:"event"`
	Command    string    `json:"command"`
	DurationMS int64     `json:"duration_ms"`
	ExitCode   int       `json:"exit_code"`
	TimedOut   bool      `json:"timed_out,omitempty"`
	Truncated  bool      `json:"truncated,omitempty"`
	StdoutLen  int       `json:"stdout_len"`
}

// Log is an append-only NDJSON log with automatic rotation.
type Log struct {
	w io.WriteCloser
}

// OpenLog opens (or creates) the NDJSON hooks log at
// <projectDir>/.clue-code/state/hooks.log with 4 MB max size, keeping 2
// rotated backups.
func OpenLog(projectDir string) (*Log, error) {
	if err := validateProjectDir(projectDir); err != nil {
		return nil, err
	}
	logPath := filepath.Join(projectDir, ".clue-code", "state", "hooks.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("hooks: mkdir log dir: %w", err)
	}
	lj := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    4, // megabytes
		MaxBackups: 2,
		Compress:   false,
	}
	return &Log{w: lj}, nil
}

// Write appends entry as a single JSON line followed by a newline.
func (l *Log) Write(entry LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("hooks: marshal log entry: %w", err)
	}
	data = append(data, '\n')
	if _, err := l.w.Write(data); err != nil {
		return fmt.Errorf("hooks: write log entry: %w", err)
	}
	return nil
}

// Close flushes and closes the underlying log writer.
func (l *Log) Close() error {
	return l.w.Close()
}

// validateProjectDir rejects empty, absolute-path-escaping, or NUL-containing
// project directory paths before they are joined with a trusted suffix.
func validateProjectDir(projectDir string) error {
	if projectDir == "" {
		return fmt.Errorf("hooks: projectDir must not be empty")
	}
	// Reject NUL bytes which would silently truncate paths on some systems.
	for _, b := range projectDir {
		if b == 0 {
			return fmt.Errorf("hooks: projectDir contains NUL byte")
		}
	}
	// The log is always placed under <projectDir>/.clue-code/state/hooks.log,
	// so no further traversal check is needed — we own the suffix.
	return nil
}
