package guard

import (
	"path/filepath"
	"strings"
)

// The command-policy hook (design.md §9.3) is a Claude Code PreToolUse hook:
// every tool call of a session goes past `awkit guard tool` first, which
// holds it to a role's policy. awkit gives it to each session it starts in
// the settings it starts the session with (--settings): a Worker, an agent
// of the engine, and the Principal of AWKIT_ENGINE=principal. It is never
// written into a project's own settings: there it would hold every session
// of the project to the role, a person's own among them, and stop every one
// of them when the awkit it names cannot check.

// HookTimeoutSeconds bounds one check: the policy is a few regular
// expressions, so a check that takes longer has gone wrong.
const HookTimeoutSeconds = 30

// Hook is whom a hook checks the calls of: the role, the project, its
// branches.
type Hook struct {
	// Awkit is the awkit that checks, by its full path.
	Awkit string
	Role  string
	// StateRoot is the project, where verdicts are recorded; "" lets the
	// check find it from the call.
	StateRoot string
	// Protected are branches nothing is pushed to, beside the ones that
	// always are.
	Protected []string
	// AllowControl lets the role change .claude and .ai (a Worker whose
	// ticket allows it).
	AllowControl bool
}

// Command is the hook's command line. It names the awkit that checks by its
// full path, so that the check does not depend on which awkit, if any, is
// first on PATH.
func (h Hook) Command() string {
	quote := func(s string) string { return `"` + filepath.ToSlash(s) + `"` }
	parts := []string{quote(h.Awkit), "guard", "tool", "--role", h.Role}
	if h.StateRoot != "" {
		parts = append(parts, "--state-root", quote(h.StateRoot))
	}
	for _, branch := range h.Protected {
		if branch != "" {
			parts = append(parts, "--protect", quote(branch))
		}
	}
	if h.AllowControl {
		parts = append(parts, "--allow-control")
	}
	return strings.Join(parts, " ")
}

// PreToolUse is the hook's entry among the PreToolUse hooks of a settings
// file: for every tool. blockOnFailure makes a check that fails or times out
// refuse the call; Claude Code knows it from 2.1.295 on.
func (h Hook) PreToolUse(blockOnFailure bool) map[string]any {
	hook := map[string]any{"type": "command", "command": h.Command(), "timeout": HookTimeoutSeconds}
	if blockOnFailure {
		hook["onFailure"] = "block"
	}
	return map[string]any{"matcher": "*", "hooks": []any{hook}}
}
