package settings

import (
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// configFile is a configuration as awkit init writes it.
const configFile = `# The configuration of this project for AWK. A key that is left out takes its
# default.
version: "1.2"

project:
  name: "tennis"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      # The commands that build and test this project.
      build: ""
      test: ""

git:
  # Pull requests are merged into the integration branch.
  integration_branch: "develop"
  release_branch: "main"

worker:
  # Who writes the code: codex or claude-code.
  backend: codex

review:
  # The score out of 10 a review has to give.
  score_threshold: 7
  merge_strategy: squash
`

func readConfig(t *testing.T, data string) *Config {
	t.Helper()
	cfg, err := ReadConfig([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestReadConfig_ReadsWhatTheFileSays(t *testing.T) {
	cfg := readConfig(t, configFile)
	if cfg.Name != "tennis" || cfg.Integration != "develop" || cfg.Release != "main" || cfg.Worker.Backend != "codex" || cfg.Worker.Model != "" {
		t.Errorf("config = %+v", cfg)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Name != "root" || cfg.Repos[0].Language != "go" || cfg.Repos[0].Build != "" {
		t.Errorf("repos = %+v", cfg.Repos)
	}
	if cfg.Review.ScoreThreshold == nil || *cfg.Review.ScoreThreshold != 7 || cfg.Retries.ReviewAttempts != nil || cfg.Retries.Worker != nil || cfg.VerifyMinutes != nil {
		t.Errorf("numbers = %+v %+v %v", cfg.Review, cfg.Retries, cfg.VerifyMinutes)
	}
	// No retries are written -1, and read as none.
	if cfg := readConfig(t, "escalation:\n  retry_count: -1\n"); cfg.Retries.Worker == nil || *cfg.Retries.Worker != 0 {
		t.Errorf("retries = %+v", cfg.Retries)
	}
}

func TestApplyConfig_WhatWasReadWritesTheSameFile(t *testing.T) {
	out, err := ApplyConfig([]byte(configFile), *readConfig(t, configFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != configFile {
		t.Errorf("the file changed:\n%s", out)
	}
}

func TestApplyConfig_ChangesInPlaceAndKeepsTheRest(t *testing.T) {
	cfg := readConfig(t, configFile)
	cfg.Name = "Tennis Arena"
	cfg.Description = "A tennis game"
	cfg.Repos[0].Build = "go build ./..."
	cfg.Repos[0].Test = "go test ./..."
	cfg.Worker = Worker{Backend: "claude-code", Model: "opus", Effort: "high"}
	cfg.Review.ScoreThreshold = intOf(8)
	cfg.Retries.Worker = intOf(0)
	cfg.VerifyMinutes = intOf(20)
	// Shown, not changed: the branches stay as the file has them.
	cfg.Integration, cfg.Release = "trunk", "trunk"

	out, err := ApplyConfig([]byte(configFile), *cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `# The configuration of this project for AWK. A key that is left out takes its
# default.
version: "1.2"

project:
  name: "Tennis Arena"
  type: "single-repo"
  description: A tennis game

repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      # The commands that build and test this project.
      build: go build ./...
      test: go test ./...

git:
  # Pull requests are merged into the integration branch.
  integration_branch: "develop"
  release_branch: "main"

worker:
  # Who writes the code: codex or claude-code.
  backend: claude-code
  claude_code:
    model: opus
    effort: high

review:
  # The score out of 10 a review has to give.
  score_threshold: 8
  merge_strategy: squash

escalation:
  retry_count: -1

timeouts:
  verify_minutes: 20
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	parsed, err := config.Parse("config.yaml", out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Escalation.RetryCount != 0 || parsed.Worker.ClaudeCode.Effort != "high" || parsed.Timeouts.VerifyMinutes != 20 {
		t.Errorf("parsed = %+v %+v %+v", parsed.Escalation, parsed.Worker, parsed.Timeouts)
	}
}

// A value set back to the default is taken out, and a section left empty
// with it.
func TestApplyConfig_TheDefaultIsNotWrittenDown(t *testing.T) {
	cfg := readConfig(t, configFile)
	cfg.Worker = Worker{Backend: "claude-code", Model: "opus"}
	cfg.VerifyMinutes = intOf(20)
	changed, err := ApplyConfig([]byte(configFile), *cfg)
	if err != nil {
		t.Fatal(err)
	}
	back := readConfig(t, string(changed))
	back.Worker = Worker{Backend: "codex"}
	back.VerifyMinutes = nil
	out, err := ApplyConfig(changed, *back)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != configFile {
		t.Errorf("got:\n%s", out)
	}
}

func TestApplyConfig_RefusesWhatARunCannotUse(t *testing.T) {
	for name, change := range map[string]func(c *Config){
		"no name":      func(c *Config) { c.Name = " " },
		"bad effort":   func(c *Config) { c.Worker.Effort = "huge" },
		"unknown repo": func(c *Config) { c.Repos[0].Name = "client" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := readConfig(t, configFile)
			change(cfg)
			if out, err := ApplyConfig([]byte(configFile), *cfg); err == nil {
				t.Errorf("accepted:\n%s", out)
			}
		})
	}
	if err := CheckConfig([]byte("project:\n  name: x\n")); err == nil || !strings.Contains(err.Error(), "project.type") {
		t.Errorf("check = %v", err)
	}
}

func TestDefaultsOfConfig(t *testing.T) {
	defaults := DefaultsOfConfig()
	if defaults.Backend != "codex" || defaults.ScoreThreshold != 7 || defaults.WorkerRetries != 2 || defaults.VerifyMinutes != 10 || len(defaults.Strategies) != 3 {
		t.Errorf("defaults = %+v", defaults)
	}
}

// The engine's reviewer and resolver: read as the file has them, written
// under agents beside what else is there, and taken out again when set back
// to the defaults.
func TestApplyConfig_TheAgents(t *testing.T) {
	file := configFile + `
agents:
  # The subagents the Principal starts.
  builtin: [pr-reviewer, conflict-resolver]
  reviewer:
    model: sonnet
`
	cfg := readConfig(t, file)
	if cfg.Agents.Reviewer.Model != "sonnet" || cfg.Agents.Reviewer.Minutes != nil || cfg.Agents.Resolver != (AgentRole{}) {
		t.Errorf("agents = %+v", cfg.Agents)
	}
	usd := 1.5
	cfg.Agents.Reviewer = AgentRole{Backend: "claude-code", Model: "opus", Effort: "high", Minutes: intOf(45), USD: &usd}
	whole := 2.0
	cfg.Agents.Resolver = AgentRole{Backend: "codex", Effort: "xhigh", USD: &whole}
	out, err := ApplyConfig([]byte(file), *cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"agents:\n  # The subagents the Principal starts.\n  builtin: [pr-reviewer, conflict-resolver]\n",
		"  reviewer:\n    backend: claude-code\n    model: opus\n    effort: high\n    minutes: 45\n    usd: 1.5\n",
		"  resolver:\n    backend: codex\n    effort: xhigh\n    usd: 2\n",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the file has no %q:\n%s", want, out)
		}
	}
	back := readConfig(t, string(out))
	if back.Agents.Reviewer.USD == nil || *back.Agents.Reviewer.USD != 1.5 || *back.Agents.Reviewer.Minutes != 45 || back.Agents.Resolver.Effort != "xhigh" {
		t.Errorf("read back = %+v", back.Agents)
	}
	// Back to the defaults: the roles go, and what else is under agents
	// stays. Zero is the default too.
	zero, none := 0, 0.0
	back.Agents = Agents{Reviewer: AgentRole{Minutes: &zero, USD: &none}}
	out, err = ApplyConfig(out, *back)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "reviewer:") || strings.Contains(string(out), "resolver:") || !strings.Contains(string(out), "builtin: [pr-reviewer, conflict-resolver]") {
		t.Errorf("defaults are written down:\n%s", out)
	}
	// A file with no agents gets none written when none are set.
	if out, _ := ApplyConfig([]byte(configFile), *readConfig(t, configFile)); strings.Contains(string(out), "agents:") {
		t.Errorf("agents written:\n%s", out)
	}
}

// What the engine would refuse to start with is refused before it is saved,
// naming the key.
func TestApplyConfig_RefusesAgentsTheEngineWouldNot(t *testing.T) {
	negative := -1.0
	for want, role := range map[string]AgentRole{
		"agents.reviewer: the reviewer's time limit is 500 minutes": {Minutes: intOf(500)},
		"agents.reviewer: the reviewer cannot run on \"gpt\"":       {Backend: "gpt"},
		"agents.reviewer: the reviewer's effort \"minimal\"":        {Effort: "minimal"},
		"agents.reviewer: the reviewer's spend limit is negative":   {USD: &negative},
	} {
		cfg := readConfig(t, configFile)
		cfg.Agents.Reviewer = role
		if _, err := ApplyConfig([]byte(configFile), *cfg); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: %v, want %q", role, err, want)
		}
	}
	cfg := readConfig(t, configFile)
	cfg.Agents.Resolver = AgentRole{Backend: "codex", Effort: "max"}
	if _, err := ApplyConfig([]byte(configFile), *cfg); err == nil || !strings.Contains(err.Error(), "agents.resolver") {
		t.Errorf("a resolver on codex thinking at max: %v", err)
	}
	if err := CheckConfig([]byte(configFile + "agents:\n  resolver: {minutes: 121}\n")); err == nil || !strings.Contains(err.Error(), "agents.resolver") {
		t.Errorf("check = %v", err)
	}
}

// What the page is told the agents' settings may be: their default minutes,
// their limits and codex's efforts, from the roles themselves.
func TestDefaultsOfConfig_TheAgents(t *testing.T) {
	a := DefaultsOfConfig().Agents
	if a.Minutes["reviewer"] != 20 || a.Minutes["resolver"] != 20 || a.MaxMinutes["reviewer"] != 120 || a.MaxMinutes["resolver"] != 120 {
		t.Errorf("minutes = %v, max %v", a.Minutes, a.MaxMinutes)
	}
	if len(a.CodexEfforts) == 0 || a.CodexEfforts[0] != "minimal" {
		t.Errorf("codex efforts = %v", a.CodexEfforts)
	}
}
