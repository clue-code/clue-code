package state

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrInvalidKey is returned by sanitizeKey when a key would escape its scope
// root via path traversal, an absolute path, or other unsafe input.
var ErrInvalidKey = errors.New("state: invalid key")

// sanitizeKey validates and normalizes a state key so it cannot escape the
// scope root via path traversal.
//
// Allowed: namespace-style keys with forward slashes (e.g. "team/abc/worker/1"),
// dotted segments (e.g. "config.json"), and Unicode alphanumerics.
// Rejected: empty strings, absolute paths ("/etc/passwd"), parent-traversal
// segments ("../escape"), and embedded NUL bytes.
//
// The returned cleaned form is what callers should use for filesystem joins.
func sanitizeKey(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: empty key", ErrInvalidKey)
	}
	if strings.ContainsRune(key, 0) {
		return "", fmt.Errorf("%w: key contains NUL byte", ErrInvalidKey)
	}
	// Reject any ".." segment in the raw input, even if filepath.Clean would
	// cancel it later. "team/../etc/passwd" is semantically lateral movement
	// and shows malicious intent — legitimate callers never write parent
	// traversal. Defense-in-depth.
	for _, sep := range []byte{'/', '\\'} {
		for _, segment := range strings.Split(key, string(sep)) {
			if segment == ".." {
				return "", fmt.Errorf("%w: %q contains parent-traversal segment", ErrInvalidKey, key)
			}
		}
	}
	cleaned := filepath.Clean(key)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: %q is an absolute path", ErrInvalidKey, key)
	}
	// Belt-and-suspenders: also check the cleaned form.
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes scope root", ErrInvalidKey, key)
	}
	return cleaned, nil
}

// sanitizeSessionID is a stricter validator for session identifiers, which
// must not contain any path separators or traversal segments at all.
func sanitizeSessionID(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("%w: empty sessionID", ErrInvalidKey)
	}
	if strings.ContainsAny(sessionID, "/\\") {
		return "", fmt.Errorf("%w: sessionID %q contains path separator", ErrInvalidKey, sessionID)
	}
	if strings.Contains(sessionID, "..") {
		return "", fmt.Errorf("%w: sessionID %q contains traversal", ErrInvalidKey, sessionID)
	}
	if strings.ContainsRune(sessionID, 0) {
		return "", fmt.Errorf("%w: sessionID contains NUL byte", ErrInvalidKey)
	}
	return sessionID, nil
}
