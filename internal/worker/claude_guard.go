package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// A Worker's claude runs with settings awkit writes for the run and no
// others: not the person's, not the project's. They allow the tools a Worker
// writes code with, refuse the rest without asking, and run every call past
// `awkit guard tool`, which holds it to the implementer's policy. Claude Code
// runs a PreToolUse hook whatever the permission mode, so the policy holds
// even where dangerously_skip_permissions is set.

// workerTools are the tools a Worker is allowed without being asked.
var workerTools = []string{"Read", "Edit", "Write", "MultiEdit", "NotebookEdit", "Glob", "Grep", "LS", "Bash", "PowerShell", "TodoWrite", "BashOutput", "KillShell"}

// workerDenied are refused by Claude Code itself as well, before the hook.
var workerDenied = []string{"WebFetch", "WebSearch", "Bash(gh:*)", "Bash(git push:*)", "Bash(curl:*)", "Bash(wget:*)"}

// awkitExecutable is the awkit the hook runs: this one. It and what is asked
// of the claude CLI are variables, so tests do not depend on what is
// installed.
var (
	awkitExecutable = os.Executable
	claudeSupports  = claudecli.Supports
	claudeMentions  = claudecli.Mentions
	claudeAtLeast   = claudecli.AtLeast
)

// workerHook is the Worker's command-policy hook: the implementer's.
func workerHook(exe string, g *WorkerGuard) guard.Hook {
	return guard.Hook{Awkit: exe, Role: guard.RoleImplementer, StateRoot: g.StateRoot, Protected: g.Protected, AllowControl: g.AllowControl}
}

// workerClaudeSettings are the settings of a Worker's claude for one run.
// blockOnFailure makes a hook that fails or times out refuse the call; the
// CLI knows it from 2.1.295 on.
func workerClaudeSettings(hook guard.Hook, blockOnFailure bool) ([]byte, error) {
	settings := map[string]any{
		"permissions": map[string]any{"allow": workerTools, "deny": workerDenied},
		"hooks":       map[string]any{"PreToolUse": []any{hook.PreToolUse(blockOnFailure)}},
	}
	return json.MarshalIndent(settings, "", "  ")
}

// guardArgs write the run's settings and return the arguments that hold the
// Worker's claude to them. A claude CLI that cannot be held so is refused:
// the Worker does not run unguarded.
func (b *ClaudeCodeBackend) guardArgs(g *WorkerGuard) ([]string, error) {
	missing := []string{}
	for _, option := range []string{"--settings", "--setting-sources"} {
		if !claudeSupports(option) {
			missing = append(missing, option)
		}
	}
	if !b.DangerouslySkipPermissions && !claudeMentions("dontAsk") {
		missing = append(missing, "the dontAsk permission mode")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("this claude CLI cannot be held to the Worker's settings (it lacks %s): update Claude Code", strings.Join(missing, ", "))
	}
	exe, err := awkitExecutable()
	if err != nil {
		return nil, fmt.Errorf("awkit cannot find itself for the Worker's hook: %w", err)
	}
	blockOnFailure := claudeAtLeast(2, 1, 295)
	if !blockOnFailure {
		fmt.Fprintln(os.Stderr, "[worker] warning: this claude CLI is older than 2.1.295: a check of the Worker's tool calls that fails or times out lets the call through")
	}
	data, err := workerClaudeSettings(workerHook(exe, g), blockOnFailure)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(g.SettingsFile), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(g.SettingsFile, data, 0o644); err != nil {
		return nil, err
	}
	args := []string{"--settings", g.SettingsFile, "--setting-sources", ""}
	if claudeSupports("--strict-mcp-config") {
		args = append(args, "--strict-mcp-config")
	}
	if !b.DangerouslySkipPermissions {
		args = append(args, "--permission-mode", "dontAsk")
	}
	return args, nil
}
