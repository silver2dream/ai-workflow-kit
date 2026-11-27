package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// defaultsCase is a case of testdata/defaults.json: the workbench's settings
// page reads the same cases, so that the defaults it shows are the ones the
// engine gives.
type defaultsCase struct {
	Name   string `json:"name"`
	Worker struct {
		Backend string `json:"backend"`
		Model   string `json:"model"`
		Effort  string `json:"effort"`
	} `json:"worker"`
	ReviewerSet *shown `json:"reviewer_set"`
	ResolverSet *shown `json:"resolver_set"`
	Reviewer    shown  `json:"reviewer"`
	Resolver    shown  `json:"resolver"`
}

// shown is a role's settings: as a case sets them, or as it gets them.
type shown struct {
	Backend string  `json:"backend"`
	Model   string  `json:"model"`
	Effort  string  `json:"effort"`
	Minutes int     `json:"minutes"`
	USD     float64 `json:"usd"`
}

func (s shown) role() config.AgentRole {
	return config.AgentRole{Backend: s.Backend, Model: s.Model, Effort: s.Effort, Minutes: s.Minutes, USD: s.USD}
}

// configOf is the configuration a project's file of a case gives, read as a
// file is: what it leaves out gets its default.
func configOf(t *testing.T, c defaultsCase) *config.Config {
	t.Helper()
	var b strings.Builder
	b.WriteString("version: \"1.0\"\nproject: {name: p, type: single-repo}\nrepos: [{name: root, path: ./, type: root}]\ngit: {integration_branch: develop, release_branch: main}\n")
	if c.Worker.Backend != "" || c.Worker.Model != "" || c.Worker.Effort != "" {
		b.WriteString("worker:\n")
		if c.Worker.Backend != "" {
			fmt.Fprintf(&b, "  backend: %s\n", c.Worker.Backend)
		}
		if c.Worker.Model != "" || c.Worker.Effort != "" {
			fmt.Fprintf(&b, "  claude_code: {model: %q, effort: %q}\n", c.Worker.Model, c.Worker.Effort)
		}
	}
	cfg, err := config.Parse("config.yaml", []byte(b.String()))
	if err != nil {
		t.Fatalf("%s: %v", c.Name, err)
	}
	if c.ReviewerSet != nil {
		cfg.Agents.Reviewer = c.ReviewerSet.role()
	}
	if c.ResolverSet != nil {
		cfg.Agents.Resolver = c.ResolverSet.role()
	}
	return cfg
}

// The agents' settings from a configuration, for every case the settings
// page shows the defaults of.
func TestFromConfig_TheDefaultsCases(t *testing.T) {
	data, err := os.ReadFile("testdata/defaults.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []defaultsCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 5 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, c := range cases {
		a, err := FromConfig(configOf(t, c))
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		for _, got := range []struct {
			role string
			p    Profile
			want shown
		}{{"reviewer", a.Reviewer, c.Reviewer}, {"resolver", a.Resolver, c.Resolver}} {
			if have := (shown{got.p.Backend, got.p.Model, got.p.Effort, got.p.Minutes, got.p.USD}); have != got.want {
				t.Errorf("%s, the %s: %+v, want %+v", c.Name, got.role, have, got.want)
			}
		}
	}
}

// The Worker's settings as the roles that follow it read them; settings
// past a role's limits are refused, naming the key.
func TestFromConfig(t *testing.T) {
	cfg := config.Default()
	if p := ImplementerOf(cfg); p.Backend != Codex || p.Model != "" {
		t.Errorf("the default Worker: %+v", p)
	}
	cfg.Worker.Backend, cfg.Worker.ClaudeCode.Model, cfg.Worker.ClaudeCode.Effort = ClaudeCode, "opus", "high"
	if p := ImplementerOf(cfg); p.Backend != ClaudeCode || p.Model != "opus" || p.Effort != "high" {
		t.Errorf("a Claude Code Worker: %+v", p)
	}
	for _, c := range []struct {
		set  func(*config.Config)
		want string
	}{
		{func(c *config.Config) { c.Agents.Reviewer.Minutes = 500 }, "agents.reviewer: the reviewer's time limit is 500 minutes: it must be 1 to 120"},
		{func(c *config.Config) { c.Agents.Resolver.Backend = "nobody" }, "agents.resolver: the resolver cannot run on \"nobody\""},
		{func(c *config.Config) { c.Agents.Reviewer.Effort = "minimal" }, "agents.reviewer: the reviewer's effort \"minimal\" is not one the claude CLI takes"},
		{func(c *config.Config) { c.Agents.Resolver.Backend, c.Agents.Resolver.Effort = Codex, "max" }, "agents.resolver: the resolver's effort \"max\" is not one codex takes"},
		{func(c *config.Config) { c.Agents.Reviewer.USD = -1 }, "agents.reviewer: the reviewer's spend limit is negative"},
	} {
		cfg := config.Default()
		c.set(cfg)
		if _, err := FromConfig(cfg); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("err = %v, want %q", err, c.want)
		}
	}
	if MaxMinutes(Reviewer) != 120 || MaxMinutes(Resolver) != 120 || MaxMinutes("nobody") != 0 {
		t.Errorf("max minutes: %d %d", MaxMinutes(Reviewer), MaxMinutes(Resolver))
	}
	if e := CodexEfforts(); len(e) == 0 || e[0] != "minimal" {
		t.Errorf("codex efforts: %v", e)
	}
}

// The configuration names the backends whose steps engine.max_per_backend
// bounds: they are the runtime's.
func TestAgentBackendsAreTheRuntimes(t *testing.T) {
	if got, want := config.AgentBackends, []string{ClaudeCode, Codex}; !slices.Equal(got, want) {
		t.Errorf("config.AgentBackends = %v, want %v", got, want)
	}
}
