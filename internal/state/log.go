package state

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logFileName = "clue-code.log"
	logMaxMB    = 10
	logKeep     = 3
)

// InitLogger configures the default slog handler.
//
// Priority order:
//  1. CLUE_CODE_LOG=stderr → write to stderr.
//  2. CLUE_CODE_LOG=<path> → write to that path (rotated via lumberjack).
//  3. default → <project>/.clue-code/state/clue-code.log (rotated).
//
// This must be called once at process start, before any slog calls.
func InitLogger() {
	var w io.Writer

	switch v := os.Getenv("CLUE_CODE_LOG"); v {
	case "stderr":
		w = os.Stderr
	case "":
		w = defaultLogWriter()
	default:
		w = rotatedWriter(v)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
}

// defaultLogWriter returns a lumberjack writer targeting the project log file.
// Falls back to stderr if the project root cannot be determined.
func defaultLogWriter() io.Writer {
	root, err := findProjectRoot()
	if err != nil {
		return os.Stderr
	}
	logPath := filepath.Join(root, ".clue-code", "state", logFileName)
	return rotatedWriter(logPath)
}

func rotatedWriter(path string) io.Writer {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return os.Stderr
	}
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    logMaxMB,
		MaxBackups: logKeep,
		Compress:   false,
	}
}
