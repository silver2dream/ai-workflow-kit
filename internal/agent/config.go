package agent

import (
	"fmt"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// Agents are the settings of the engine's agents, as a project's
// configuration gives them.
type Agents struct {
	Implementer Profile
	Reviewer    Profile
	Resolver    Profile
}

// FromConfig gives the engine's agents their settings: the implementer's
// from the project's worker, and the reviewer's and the resolver's from
// agents.reviewer and agents.resolver on their roles' defaults. Settings
// that go past a role's limits are refused, naming the key. The engine
// starts with these; the settings page and `awkit validate` check with them
// before a run would.
func FromConfig(cfg *config.Config) (Agents, error) {
	a := Agents{Implementer: ImplementerOf(cfg)}
	var err error
	if a.Reviewer, err = Resolve(Reviewer, a.Implementer, OverrideOf(cfg.Agents.Reviewer)); err != nil {
		return Agents{}, fmt.Errorf("agents.reviewer: %w", err)
	}
	if a.Resolver, err = Resolve(Resolver, a.Implementer, OverrideOf(cfg.Agents.Resolver)); err != nil {
		return Agents{}, fmt.Errorf("agents.resolver: %w", err)
	}
	return a, nil
}

// ImplementerOf is the Worker's settings, as the roles that follow it read
// them: its backend, and, for Claude Code, its model and effort.
func ImplementerOf(cfg *config.Config) Profile {
	p := Profile{Role: Implementer, Backend: cfg.Worker.Backend}
	if p.Backend == "" {
		p.Backend = Codex
	}
	if p.Backend == ClaudeCode {
		p.Model, p.Effort = cfg.Worker.ClaudeCode.Model, cfg.Worker.ClaudeCode.Effort
	}
	return p
}

// OverrideOf is a role's settings as the configuration gives them.
func OverrideOf(r config.AgentRole) Override {
	return Override{Backend: r.Backend, Model: r.Model, Effort: r.Effort, Minutes: r.Minutes, USD: r.USD}
}

// MaxMinutes is how long one run of a role may be set to take; 0 for a role
// that is not one.
func MaxMinutes(role Role) int {
	return limits[role].maxMinutes
}

// CodexEfforts are the reasoning efforts codex takes.
func CodexEfforts() []string {
	return append([]string(nil), codexEfforts...)
}
