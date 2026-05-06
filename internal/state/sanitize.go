package state

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrInvalidKey is returned by SanitizeKey/SanitizeIdentifier when input
// would escape its scope root via path traversal, an absolute path, or other
// unsafe content.
var ErrInvalidKey = errors.New("state: invalid key")

// SanitizeKey validates and normalizes a state key so it cannot escape the
// scope root via path traversal. Used as the canonical sanitizer across
// internal/* packages that join user-supplied keys into filesystem paths.
//
// Allowed: namespace-style keys with forward slashes (e.g. "team/abc/worker/1"),
// dotted segments (e.g. "config.json"), and Unicode alphanumerics.
// Rejected: empty strings, absolute paths ("/etc/passwd"), parent-traversal
// segments ("../escape"), and embedded NUL bytes.
//
// The returned cleaned form is what callers should use for filesystem joins.
func SanitizeKey(key string) (string, error) {
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

// SanitizeIdentifier is a stricter validator for flat identifiers (session IDs,
// skill names, agent names, team names), which must NOT contain any path
// separators or traversal segments. Use this for any name that becomes a
// single directory or filename component.
func SanitizeIdentifier(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty identifier", ErrInvalidKey)
	}
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("%w: identifier %q contains path separator", ErrInvalidKey, name)
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: identifier %q contains traversal", ErrInvalidKey, name)
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: identifier contains NUL byte", ErrInvalidKey)
	}
	return name, nil
}
