package aider

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BuildRepoMap walks repoRoot and returns a map of relative file path → SHA-256
// hex digest for every file whose extension is in exts.
//
// This lightweight map is used as a fallback when aider is absent and a caller
// needs to track which files changed after a direct edit.
//
// The function tolerates unreadable files by skipping them (no error returned).
// An error is only returned when repoRoot itself cannot be walked.
func BuildRepoMap(repoRoot string, exts []string) (map[string]string, error) {
	extSet := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		// Normalise: ensure each extension starts with a dot.
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		extSet[strings.ToLower(e)] = struct{}{}
	}

	result := make(map[string]string)

	walkErr := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries rather than aborting the whole walk.
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories (e.g. .git, .omc).
			if strings.HasPrefix(d.Name(), ".") && path != repoRoot {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if len(extSet) > 0 {
			if _, ok := extSet[ext]; !ok {
				return nil
			}
		}

		hash, hashErr := hashFile(path)
		if hashErr != nil {
			// Unreadable file — skip silently.
			return nil
		}

		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		result[rel] = hash
		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("aider: repomap walk failed: %w", walkErr)
	}
	return result, nil
}

// hashFile computes the SHA-256 hex digest of the file at path.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
