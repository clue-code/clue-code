## Description
What does this PR change and why?

## Type of change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality not to work as expected)
- [ ] Documentation update
- [ ] Refactor / chore

## Testing
- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes
- [ ] Manually tested on `<macOS / Linux>`
- [ ] New tests added for new functionality

## Checklist
- [ ] Code follows the existing style (`gofmt`, `go vet`)
- [ ] Self-reviewed the diff
- [ ] Updated documentation where relevant (`README.md`, `docs/`, `CHANGELOG.md`)
- [ ] Linked related issues (`Fixes #123` or `Refs #123`)
- [ ] No leftover debug code (`fmt.Println`, `// TODO`, `// HACK`, `panic("debug")`)

## Security checklist
- [ ] No secrets, tokens, API keys, or credentials in this diff (env, code, fixtures, comments)
- [ ] Any new file path joins use sanitized inputs (`sanitizeKey`/`sanitizeSessionID` or equivalent)
- [ ] New file modes are `0o600` (files) / `0o700` (dirs) unless world-readable is intentional
- [ ] Any new external input (CLI args, hook output, network) is validated before use
- [ ] No new `os/exec` invocations spawn binaries from `$PATH`; use `os.Executable()` for self-invocation
- [ ] If touching auth, crypto, or session state, requested a security-reviewer pass

## Related issues
Fixes #
Refs #
