package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPath(t *testing.T) {
	if got, want := Path("proj"), filepath.Join("proj", ".ai", "config", "workflow.yaml"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// A file that says nothing still gives a whole configuration: every default
// is decided here, and nowhere else.
func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, "# nothing\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := &Config{
		Git:        Git{ReleaseBranch: "main", CommitFormat: "[type] subject"},
		GitHub:     GitHub{Labels: DefaultLabels()},
		Rules:      Rules{Kit: []string{"git-workflow"}},
		Agents:     Agents{Builtin: []string{"pr-reviewer", "conflict-resolver"}},
		Review:     Review{ScoreThreshold: 7, MergeStrategy: "squash", JiTTest: JiTTest{MaxTests: 5, TimeoutSeconds: 120, FailurePolicy: "warn", Model: "claude-sonnet-4-6"}},
		Escalation: Escalation{MaxReviewAttempts: 3, RetryCount: 2, RetryDelaySeconds: 5},
		Timeouts:   Timeouts{GitSeconds: 120, GHSeconds: 60, CodexMinutes: 30, GHRetryCount: 3, GHRetryBaseDelay: 2},
		Feedback:   Feedback{MaxHistoryInPrompt: 10},
		Worker:     Worker{Backend: "codex", Codex: Codex{MaxAttempts: 1}, ClaudeCode: ClaudeCode{Model: "sonnet", MaxTurns: 50}},
		Lessons:    Lessons{MaxActive: 30, InjectTopK: 3, InjectMaxChars: 800, Distiller: LessonsDistiller{Model: "sonnet", TimeoutSeconds: 60}},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("the configuration of an empty file =\n%+v\nwant\n%+v", cfg, want)
	}
	if !reflect.DeepEqual(Default(), want) {
		t.Errorf("Default() =\n%+v\nwant what an empty file gives", Default())
	}
	if labels := DefaultLabels(); labels.Task != "ai-task" || labels.NeedsHumanReview != "needs-human-review" || labels.Completed != "completed" {
		t.Errorf("default labels = %+v", labels)
	}

	// What is on unless the file turns it off, and what is off unless it turns it on.
	if !cfg.Review.SeverityCheckEnabled() || !cfg.Review.HumanFeedbackEnabled() || !cfg.Feedback.IsEnabled() || !cfg.Lessons.IsEnabled() || !cfg.Worker.Codex.IsFullAuto() {
		t.Error("a check that is on by default is off in an empty file")
	}
	if cfg.Review.JiTTest.IsEnabled() {
		t.Error("JiT tests are on in an empty file")
	}
	if got := cfg.BaseBranch(); got != "develop" {
		t.Errorf("BaseBranch of a file that names none = %q, want develop", got)
	}
}

const full = `
version: "1.2"
project: {name: demo, description: A demo, type: monorepo}
repos:
  - name: Backend
    path: backend/
    type: directory
    language: go
    package_manager: npm
    verify: {setup: make deps, build: go build ./..., test: go test ./...}
    rules: [backend-go]
  - {name: frontend, path: frontend/, type: submodule, language: godot}
git: {integration_branch: trunk, release_branch: stable, commit_format: "type: subject"}
github:
  repo: someone/demo
  labels: {task: todo, in_progress: doing, pr_ready: ready, worker_failed: broke, needs_human_review: human, review_failed: rejected, merge_conflict: conflict, needs_rebase: rebase, completed: done}
rules: {kit: [git-workflow, extra], custom: [mine]}
agents:
  builtin: [pr-reviewer]
  custom:
    - {name: sec, description: Security, tools: Read, model: haiku, trigger: review_pr, condition: always}
review:
  score_threshold: 9
  merge_strategy: rebase
  multi_model: true
  secondary_reviewers: [{backend: codex, model: o3, focus_area: security}]
  jittest: {enabled: true, max_tests: 2, timeout_seconds: 30, failure_policy: block, model: haiku}
  severity_consistency: false
  human_feedback: false
escalation: {max_review_attempts: 5, retry_count: 4, retry_delay_seconds: 9}
timeouts: {git_seconds: 11, gh_seconds: 12, codex_minutes: 13, gh_retry_count: 14, gh_retry_base_delay: 15}
feedback: {enabled: false, max_history_in_prompt: 4}
hooks:
  pre_dispatch: [{command: echo pre, timeout: 5s, on_failure: abort, env: {A: b}}]
  post_dispatch: [{command: echo post}]
  pre_review: [{command: echo pre-review}]
  post_review: [{command: echo post-review}]
  on_merge: [{command: echo merged}]
  on_failure: [{command: echo failed}]
worker:
  backend: claude-code
  codex: {full_auto: false, max_attempts: 3}
  claude_code: {model: opus, max_turns: 80, dangerously_skip_permissions: true}
  knowledge_graph: "off"
lessons:
  enabled: false
  max_active: 7
  inject_top_k: 2
  inject_max_chars: 300
  distiller: {backend: claude, model: haiku, timeout_seconds: 20}
`

// What the file says is what the configuration holds: no default writes over
// a value that is there.
func TestLoad_WhatTheFileSays(t *testing.T) {
	cfg, err := Load(writeConfig(t, full))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	no, yes := false, true
	want := &Config{
		Version: "1.2",
		Project: Project{Name: "demo", Description: "A demo", Type: "monorepo"},
		Repos: []Repo{
			{Name: "Backend", Path: "backend/", Type: "directory", Language: "go", PackageManager: "npm",
				Verify: Verify{Setup: "make deps", Build: "go build ./...", Test: "go test ./..."}, Rules: []string{"backend-go"}},
			{Name: "frontend", Path: "frontend/", Type: "submodule", Language: "godot"},
		},
		Git: Git{IntegrationBranch: "trunk", ReleaseBranch: "stable", CommitFormat: "type: subject"},
		GitHub: GitHub{Repo: "someone/demo", Labels: Labels{Task: "todo", InProgress: "doing", PRReady: "ready", WorkerFailed: "broke",
			NeedsHumanReview: "human", ReviewFailed: "rejected", MergeConflict: "conflict", NeedsRebase: "rebase", Completed: "done"}},
		Rules:  Rules{Kit: []string{"git-workflow", "extra"}, Custom: []string{"mine"}},
		Agents: Agents{Builtin: []string{"pr-reviewer"}, Custom: []CustomAgent{{Name: "sec", Description: "Security", Tools: "Read", Model: "haiku", Trigger: "review_pr", Condition: "always"}}},
		Review: Review{ScoreThreshold: 9, MergeStrategy: "rebase", MultiModel: true,
			SecondaryReviewers:  []SecondaryReviewer{{Backend: "codex", Model: "o3", FocusArea: "security"}},
			JiTTest:             JiTTest{Enabled: &yes, MaxTests: 2, TimeoutSeconds: 30, FailurePolicy: "block", Model: "haiku"},
			SeverityConsistency: &no, HumanFeedback: &no},
		Escalation: Escalation{MaxReviewAttempts: 5, RetryCount: 4, RetryDelaySeconds: 9},
		Timeouts:   Timeouts{GitSeconds: 11, GHSeconds: 12, CodexMinutes: 13, GHRetryCount: 14, GHRetryBaseDelay: 15},
		Feedback:   Feedback{Enabled: &no, MaxHistoryInPrompt: 4},
		Hooks: Hooks{
			PreDispatch:  []Hook{{Command: "echo pre", Timeout: "5s", OnFailure: "abort", Env: map[string]string{"A": "b"}}},
			PostDispatch: []Hook{{Command: "echo post"}}, PreReview: []Hook{{Command: "echo pre-review"}}, PostReview: []Hook{{Command: "echo post-review"}},
			OnMerge: []Hook{{Command: "echo merged"}}, OnFailure: []Hook{{Command: "echo failed"}},
		},
		Worker: Worker{Backend: "claude-code", Codex: Codex{FullAuto: &no, MaxAttempts: 3},
			ClaudeCode: ClaudeCode{Model: "opus", MaxTurns: 80, DangerouslySkipPermissions: true}, KnowledgeGraph: "off"},
		Lessons: Lessons{Enabled: &no, MaxActive: 7, InjectTopK: 2, InjectMaxChars: 300, Distiller: LessonsDistiller{Backend: "claude", Model: "haiku", TimeoutSeconds: 20}},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("the configuration =\n%+v\nwant\n%+v", cfg, want)
	}

	if cfg.Review.SeverityCheckEnabled() || cfg.Review.HumanFeedbackEnabled() || cfg.Feedback.IsEnabled() || cfg.Lessons.IsEnabled() || cfg.Worker.Codex.IsFullAuto() {
		t.Error("something the file turns off is on")
	}
	if !cfg.Review.JiTTest.IsEnabled() {
		t.Error("JiT tests the file turns on are off")
	}
	if got := cfg.BaseBranch(); got != "trunk" {
		t.Errorf("BaseBranch = %q, want the integration branch", got)
	}
	for event, want := range map[string]string{"pre_dispatch": "echo pre", "post_dispatch": "echo post", "pre_review": "echo pre-review",
		"post_review": "echo post-review", "on_merge": "echo merged", "on_failure": "echo failed"} {
		if hooks := cfg.Hooks.GetHooks(event); len(hooks) != 1 || hooks[0].Command != want {
			t.Errorf("hooks of %s = %+v, want %q", event, hooks, want)
		}
	}
	if hooks := cfg.Hooks.GetHooks("on_holiday"); hooks != nil {
		t.Errorf("an event that does not exist has hooks: %+v", hooks)
	}
}

// Numbers that make no sense get the default; retries are the exception,
// where a negative number is how a file asks for none.
func TestLoad_NumbersOutOfRange(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
review: {score_threshold: -1, jittest: {max_tests: 0, timeout_seconds: -5}}
escalation: {max_review_attempts: -2, retry_count: -1, retry_delay_seconds: -3}
timeouts: {git_seconds: -1, gh_seconds: 0, codex_minutes: -9, gh_retry_count: -1, gh_retry_base_delay: 0}
worker: {codex: {max_attempts: -1}, claude_code: {max_turns: -4}}
lessons: {max_active: -1, inject_top_k: -1, inject_max_chars: -1, distiller: {timeout_seconds: -1}}
feedback: {max_history_in_prompt: -6}
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Escalation.RetryCount != 0 {
		t.Errorf("a negative retry_count gives %d retries, want none", cfg.Escalation.RetryCount)
	}
	cfg.Escalation.RetryCount = Default().Escalation.RetryCount
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("numbers out of range did not get their defaults:\n%+v", cfg)
	}

	for retries, want := range map[string]int{"0": 2, "1": 1, "6": 6} {
		cfg, err := Load(writeConfig(t, "escalation: {retry_count: "+retries+"}\n"))
		if err != nil || cfg.Escalation.RetryCount != want {
			t.Errorf("retry_count %s gives %d (err %v), want %d", retries, cfg.Escalation.RetryCount, err, want)
		}
	}
}

// The label that was called review_fail in version 1.0 of the file is still
// read, unless the file names review_failed too.
func TestLoad_TheOldNameOfALabel(t *testing.T) {
	cfg, err := Load(writeConfig(t, "github: {labels: {review_fail: old-name}}\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GitHub.Labels.ReviewFailed != "old-name" || cfg.GitHub.Labels.ReviewFail != "" {
		t.Errorf("labels = %+v, want the old key read into review_failed and cleared", cfg.GitHub.Labels)
	}
	cfg, err = Load(writeConfig(t, "github: {labels: {review_fail: old-name, review_failed: new-name}}\n"))
	if err != nil || cfg.GitHub.Labels.ReviewFailed != "new-name" || cfg.GitHub.Labels.ReviewFail != "" {
		t.Errorf("labels = %+v (err %v), want review_failed to win", cfg.GitHub.Labels, err)
	}
}

// A project without a configuration is told apart from one whose
// configuration is broken; a broken one is never half used.
func TestLoad_FilesThatCannotBeUsed(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !os.IsNotExist(err) || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a project without a configuration: err = %v, want one that says the file is not there", err)
	}
	var parse *ParseError
	if errors.As(err, &parse) {
		t.Errorf("a file that is not there is reported as one that is not valid: %v", err)
	}

	for name, content := range map[string]string{
		"not YAML":                  "project: [unclosed\n",
		"a number where a word is":  "repos: 3\n",
		"a word where a number is":  "timeouts: {gh_seconds: soon}\n",
		"a list where a section is": "review: [squash]\n",
	} {
		root := writeConfig(t, content)
		cfg, err := Load(root)
		if err == nil || cfg != nil {
			t.Errorf("%s: read as %+v, err %v; want an error and no configuration", name, cfg, err)
			continue
		}
		if !errors.As(err, &parse) || os.IsNotExist(err) {
			t.Errorf("%s: err = %v (%T), want a *ParseError", name, err, err)
			continue
		}
		if !strings.Contains(err.Error(), ".ai/config/workflow.yaml is not valid: ") || strings.Contains(err.Error(), "yaml: ") {
			t.Errorf("%s: the error should name the file and say it is not valid: %v", name, err)
		}
		if parse.Unwrap() == nil {
			t.Errorf("%s: the error does not carry what the parser said", name)
		}
	}
}

// How pull requests are merged: what the file says when that is a way AWK
// knows, squash otherwise.
func TestReview_Strategy(t *testing.T) {
	for configured, want := range map[string]string{"": "squash", "squash": "squash", "merge": "merge", "rebase": "rebase", "fast-forward": "squash", "Merge": "squash"} {
		review := Review{MergeStrategy: configured}
		if got := review.Strategy(); got != want {
			t.Errorf("Strategy of %q = %q, want %q", configured, got, want)
		}
	}
}

// A ticket's repo is found whatever its case; the root of the repository is
// always one, whether the file names it or not.
func TestRepos(t *testing.T) {
	cfg := &Config{Repos: []Repo{
		{Name: "Backend", Verify: Verify{Build: "make", Test: "make test"}},
		{Name: "frontend", Verify: Verify{Test: "dotnet test"}},
		{Name: "docs"},
		{Name: ""},
		{Name: "ROOT"},
	}}
	if got := cfg.FindRepo("backend"); got == nil || got.Name != "Backend" {
		t.Errorf("FindRepo(backend) = %+v, want the repo named Backend", got)
	}
	if got := cfg.FindRepo("FRONTEND"); got == nil || got.Name != "frontend" {
		t.Errorf("FindRepo(FRONTEND) = %+v", got)
	}
	if got := cfg.FindRepo("web"); got != nil {
		t.Errorf("FindRepo(web) = %+v, want none", got)
	}
	if got := cfg.GetRepoByName("backend"); got != nil {
		t.Errorf("GetRepoByName(backend) = %+v; it takes the name as it is written", got)
	}
	if got := cfg.GetRepoByName("Backend"); got == nil {
		t.Error("GetRepoByName(Backend) found nothing")
	}
	// What is returned is the repo of the configuration, not a copy.
	cfg.FindRepo("docs").Language = "markdown"
	if cfg.Repos[2].Language != "markdown" {
		t.Error("FindRepo returns a copy")
	}

	if got, want := cfg.TicketRepos(), []string{"root", "Backend", "frontend", "docs"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TicketRepos = %v, want %v", got, want)
	}
	for name, want := range map[string]bool{"root": true, "Root": true, "backend": true, "FRONTEND": true, "web": false, "": false} {
		if got := cfg.HasTicketRepo(name); got != want {
			t.Errorf("HasTicketRepo(%q) = %v, want %v", name, got, want)
		}
	}
	if got := (&Config{}).TicketRepos(); !reflect.DeepEqual(got, []string{"root"}) {
		t.Errorf("TicketRepos of a file without repos = %v, want the root alone", got)
	}

	for repo, want := range map[string][]string{"Backend": {"make", "make test"}, "frontend": {"dotnet test"}, "docs": nil, "web": nil} {
		if got := cfg.GetVerifyCommands(repo); !reflect.DeepEqual(got, want) {
			t.Errorf("GetVerifyCommands(%s) = %v, want %v", repo, got, want)
		}
	}
}

func valid() *Config {
	return &Config{
		Project: Project{Name: "demo", Type: "monorepo"},
		Git:     Git{IntegrationBranch: "develop"},
		Repos:   []Repo{{Name: "backend", Path: "backend/", Type: "directory"}, {Name: "root", Path: "./"}},
		Agents: Agents{Builtin: []string{"pr-reviewer"}, Custom: []CustomAgent{
			{Name: "sec-check", Description: "Security", Model: "opus", Trigger: "review_pr"},
			{Name: "perf", Description: "Performance"},
		}},
	}
}

func TestValidate(t *testing.T) {
	if errs := valid().Validate(); len(errs) != 0 {
		t.Fatalf("a valid configuration has problems: %v", errs)
	}
	tests := []struct {
		name   string
		change func(c *Config)
		want   string
	}{
		{"no project name", func(c *Config) { c.Project.Name = "" }, "project.name: required field is missing"},
		{"no project type", func(c *Config) { c.Project.Type = "" }, "project.type: required field is missing (expected: monorepo or single-repo)"},
		{"a project type that is none", func(c *Config) { c.Project.Type = "multi" }, "project.type: invalid value: multi (expected: monorepo or single-repo)"},
		{"no integration branch", func(c *Config) { c.Git.IntegrationBranch = "" }, "git.integration_branch: required field is missing"},
		{"a repo without a name", func(c *Config) { c.Repos[1].Name = "" }, "repos[1].name: required field is missing"},
		{"a repo without a path", func(c *Config) { c.Repos[0].Path = "" }, "repos[0].path: required field is missing"},
		{"a repo type that is none", func(c *Config) { c.Repos[0].Type = "folder" }, "repos[0].type: invalid value: folder (expected: root, directory, or submodule)"},
		{"an agent without a name", func(c *Config) { c.Agents.Custom[0].Name = "" }, "agents.custom[0].name: required field is missing"},
		{"an agent name that is no name", func(c *Config) { c.Agents.Custom[1].Name = "Perf Check" },
			"agents.custom[1].name: invalid name: Perf Check (expected: lowercase letters, digits, hyphens (must start with letter))"},
		{"an agent named like a built-in one", func(c *Config) { c.Agents.Custom[0].Name = "pr-reviewer" }, "agents.custom[0].name: name 'pr-reviewer' collides with built-in agent"},
		{"two agents of one name", func(c *Config) { c.Agents.Custom[1].Name = "sec-check" }, "agents.custom[1].name: duplicate agent name: sec-check"},
		{"an agent without a description", func(c *Config) { c.Agents.Custom[1].Description = "" }, "agents.custom[1].description: required field is missing"},
		{"an agent with a model that is none", func(c *Config) { c.Agents.Custom[0].Model = "gpt" }, "agents.custom[0].model: invalid model: gpt (expected: haiku, sonnet, or opus)"},
		{"an agent with a trigger that is none", func(c *Config) { c.Agents.Custom[0].Trigger = "generate_tasks" },
			"agents.custom[0].trigger: invalid trigger: generate_tasks (expected: review_pr, check_result, or dispatch_worker)"},
	}
	for _, tt := range tests {
		cfg := valid()
		tt.change(cfg)
		errs := cfg.Validate()
		if len(errs) != 1 || errs[0].Error() != tt.want {
			t.Errorf("%s: problems = %v, want exactly %q", tt.name, errs, tt.want)
		}
	}

	// Every problem is reported, in the order of the file: project, git,
	// agents, repos.
	errs := (&Config{Repos: []Repo{{}}, Agents: Agents{Custom: []CustomAgent{{}}}}).Validate()
	var fields []string
	for _, err := range errs {
		fields = append(fields, err.Field)
	}
	if want := []string{"project.name", "project.type", "git.integration_branch", "agents.custom[0].name", "agents.custom[0].description", "repos[0].name", "repos[0].path"}; !reflect.DeepEqual(fields, want) {
		t.Errorf("problems of an empty configuration = %v, want %v", fields, want)
	}
	// A repo may leave its type out, and every known model and trigger is accepted.
	for _, model := range AgentModels {
		for _, trigger := range AgentTriggers {
			cfg := valid()
			cfg.Agents.Custom[0].Model, cfg.Agents.Custom[0].Trigger = model, trigger
			if errs := cfg.Validate(); len(errs) != 0 {
				t.Errorf("model %s with trigger %s: %v", model, trigger, errs)
			}
		}
	}
	for _, kind := range []string{"", "root", "directory", "submodule"} {
		cfg := valid()
		cfg.Repos[0].Type = kind
		if errs := cfg.Validate(); len(errs) != 0 {
			t.Errorf("repo type %q: %v", kind, errs)
		}
	}
}

func TestValidatePaths(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Repos: []Repo{{Name: "root", Path: "./"}, {Name: "backend", Path: "backend/"}, {Name: "frontend", Path: "frontend/"}}}

	errs := cfg.ValidatePaths(base)

	if len(errs) != 1 || errs[0].Error() != "repos[2].path: path does not exist: frontend/" {
		t.Errorf("problems = %v, want the one path that is not there", errs)
	}
}
