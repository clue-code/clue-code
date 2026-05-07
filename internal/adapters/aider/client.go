// Package aider provides an optional adapter for the Aider AI coding assistant.
// All operations degrade gracefully when the aider binary is absent.
package aider

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
)

// ErrAiderNotAvailable is returned when the aider binary cannot be found.
var ErrAiderNotAvailable = errors.New("aider: binary not available")

// Client wraps an optional aider installation. A zero-value Client is safe to
// use; Available() returns false and Apply() returns ErrAiderNotAvailable.
type Client struct {
	available bool
	version   string
	binPath   string
}

// NewClient probes the PATH for the aider binary and captures its version.
// If aider is not installed, the returned Client is still valid — callers must
// check Available() before relying on Apply().
func NewClient() *Client {
	c := &Client{}

	path, err := exec.LookPath("aider")
	if err != nil {
		slog.Warn("aider not found (optional, fallback edit available)",
			"error", err)
		return c
	}
	c.binPath = path

	// Capture version string; ignore errors — availability already confirmed.
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		slog.Warn("aider --version failed; treating as unavailable",
			"path", path,
			"error", err)
		return c
	}

	c.version = strings.TrimSpace(string(out))
	c.available = true
	slog.Info("aider detected", "version", c.version, "path", path)
	return c
}

// Available reports whether the aider binary was found and is usable.
func (c *Client) Available() bool {
	return c.available
}

// Version returns the aider version string (e.g. "aider 0.40.1").
// Returns an empty string when not available.
func (c *Client) Version() string {
	return c.version
}

// Apply invokes aider with the given instruction inside repoRoot.
// It returns the list of files that aider reported as changed and a short
// summary extracted from the output.
//
// Returns ErrAiderNotAvailable immediately when the binary is absent so callers
// can implement a clean fallback without inspecting error strings.
func (c *Client) Apply(ctx context.Context, instruction, repoRoot string) (filesChanged []string, summary string, err error) {
	if !c.available {
		return nil, "", ErrAiderNotAvailable
	}

	output, err := runAider(ctx, c.binPath, repoRoot, instruction)
	if err != nil {
		slog.Warn("aider subprocess failed",
			"repoRoot", repoRoot,
			"error", err)
		return nil, "", err
	}

	filesChanged, summary, err = ParseAiderOutput(output)
	if err != nil {
		slog.Warn("aider output parse error (non-fatal)",
			"error", err)
		// Return what we have even if parsing was partially successful.
	}
	return filesChanged, summary, err
}
