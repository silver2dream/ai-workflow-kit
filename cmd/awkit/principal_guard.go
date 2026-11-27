package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// The Principal of AWKIT_ENGINE=principal, with its reviewer and conflict
// resolver, has every tool call checked against the principal's policy
// (internal/guard): it cannot merge a pull request on its own, push where
// pull requests are merged, read credentials or reach other hosts. The
// check comes in the settings the session is started with, as a Worker's
// and the engine's agents' do, and holds that session alone: the project's
// own settings, which every Claude Code session of the project reads, a
// person's own among them, have no hook of awkit's (internal/generate).
// The project's settings stay the session's too: these only add the check.

// What awkit asks of the claude CLI, and where it is itself: variables, so
// that tests do not depend on what is installed.
var (
	principalAwkit    = os.Executable
	principalSupports = claudecli.Supports
	principalAtLeast  = claudecli.AtLeast
)

// principalSettingsFile is where the settings of the Principal's session are
// written, under the project.
func principalSettingsFile(root string) string {
	return filepath.Join(root, layout.StateDir, "principal", "claude-settings.json")
}

// principalGuardArgs write the settings that hold the Principal's session to
// the principal's policy, with the project's branches protected, and return
// the arguments that give them to it. A claude CLI that cannot be given
// settings is refused: the Principal does not run unchecked.
func principalGuardArgs(root string, cfg *config.Config) ([]string, error) {
	if !principalSupports("--settings") {
		return nil, fmt.Errorf("this claude CLI cannot be given the Principal's settings (it lacks --settings), and the Principal does not run unchecked: update Claude Code")
	}
	exe, err := principalAwkit()
	if err != nil {
		return nil, fmt.Errorf("awkit cannot find itself for the Principal's command-policy hook: %w", err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	hook := guard.Hook{Awkit: exe, Role: guard.RolePrincipal, StateRoot: abs}
	if cfg != nil {
		hook.Protected = []string{cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch}
	}
	block := principalAtLeast(2, 1, 295)
	if !block {
		fmt.Fprintln(os.Stderr, "[run] warning: this claude CLI is older than 2.1.295: a check of the Principal's tool calls that fails or times out lets the call through")
	}
	data, err := json.MarshalIndent(map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{hook.PreToolUse(block)}},
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	path := principalSettingsFile(abs)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return []string{"--settings", path}, nil
}
