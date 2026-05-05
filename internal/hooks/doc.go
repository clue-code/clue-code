// Package hooks implements the five lifecycle hook points for clue-code:
//
//   - SessionStart  – fired once when a session is initialized
//   - PreToolUse    – fired before every tool invocation
//   - PostToolUse   – fired after every tool invocation (success or failure)
//   - UserPromptSubmit – fired each time the user submits a prompt
//   - Stop          – fired when the session is about to terminate
//
// Hooks are shell commands defined in ~/.config/clue-code/hooks.yaml.
// Each hook is executed as a subprocess via "sh -c <command>".
// Blocking hooks (blocking: true) halt the calling operation until they
// complete or time out.  Non-blocking hooks fire-and-forget.
//
// Threat model (trusted-hook): hook commands are read from a user-owned config
// file (~/.config/clue-code/hooks.yaml) and executed with the user's own
// credentials.  No untrusted input flows into the command string at runtime —
// only structured JSON context is passed via stdin.  The recursion guard
// (CLUE_CODE_HOOK_DEPTH) prevents hook-spawned processes from re-triggering
// hooks without bound.  Path values derived from config are sanitized before
// any filesystem join to prevent traversal attacks.
package hooks
