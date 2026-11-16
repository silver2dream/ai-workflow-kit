// Package config reads a project's configuration file: one file, one reader,
// one set of defaults.
//
// Every part of AWK that needs the configuration gets it from here. A value
// the file leaves out has the same default whoever asks, and a file that
// cannot be read is the same error for every command.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// File is where a project keeps its configuration, relative to its root.
const File = layout.ConfigFile

// Path returns the configuration file of the project at root.
func Path(root string) string { return layout.Config(root) }

// Config is the configuration of a project.
type Config struct {
	Version    string     `yaml:"version"`
	Project    Project    `yaml:"project"`
	Repos      []Repo     `yaml:"repos"`
	Git        Git        `yaml:"git"`
	GitHub     GitHub     `yaml:"github"`
	Rules      Rules      `yaml:"rules"`
	Agents     Agents     `yaml:"agents"`
	Review     Review     `yaml:"review"`
	Escalation Escalation `yaml:"escalation"`
	Timeouts   Timeouts   `yaml:"timeouts"`
	Feedback   Feedback   `yaml:"feedback"`
	Hooks      Hooks      `yaml:"hooks"`
	Worker     Worker     `yaml:"worker"`
	Lessons    Lessons    `yaml:"lessons"`
}

// Project says what the project is.
type Project struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Type is monorepo or single-repo.
	Type string `yaml:"type"`
}

// Repo is a part of the project a ticket can be for.
type Repo struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
	// Type is root, directory or submodule.
	Type     string `yaml:"type"`
	Language string `yaml:"language"`
	// PackageManager picks the command that installs a Node repo's
	// dependencies when verify.setup names none: npm, yarn or pnpm.
	PackageManager string `yaml:"package_manager"`
	Verify         Verify `yaml:"verify"`
	// Rules is read from the file and used by nothing.
	Rules []string `yaml:"rules"`
}

// Verify holds the commands that set up, build and test a repo.
type Verify struct {
	Setup string `yaml:"setup"`
	Build string `yaml:"build"`
	Test  string `yaml:"test"`
}

// Git names the branches work goes to.
type Git struct {
	IntegrationBranch string `yaml:"integration_branch"`
	ReleaseBranch     string `yaml:"release_branch"`
	CommitFormat      string `yaml:"commit_format"`
}

// GitHub holds what AWK needs to know about the project on GitHub.
type GitHub struct {
	// Repo is owner/name. Empty means the repo gh resolves by itself.
	Repo   string `yaml:"repo"`
	Labels Labels `yaml:"labels"`
}

// Labels are the names of the labels the workflow puts on issues and pull
// requests.
type Labels struct {
	Task             string `yaml:"task"`
	InProgress       string `yaml:"in_progress"`
	PRReady          string `yaml:"pr_ready"`
	WorkerFailed     string `yaml:"worker_failed"`
	NeedsHumanReview string `yaml:"needs_human_review"`
	ReviewFailed     string `yaml:"review_failed"`
	MergeConflict    string `yaml:"merge_conflict"`
	NeedsRebase      string `yaml:"needs_rebase"`
	Completed        string `yaml:"completed"`

	// ReviewFail is the name review_failed had in version 1.0 of the file.
	// It is read into ReviewFailed and then cleared.
	ReviewFail string `yaml:"review_fail,omitempty"`
}

// DefaultLabels returns the label names used when the file names none.
func DefaultLabels() Labels {
	return Labels{
		Task:             "ai-task",
		InProgress:       "in-progress",
		PRReady:          "pr-ready",
		WorkerFailed:     "worker-failed",
		NeedsHumanReview: "needs-human-review",
		ReviewFailed:     "review-failed",
		MergeConflict:    "merge-conflict",
		NeedsRebase:      "needs-rebase",
		Completed:        "completed",
	}
}

// Rules names the rule files agents are told to read.
type Rules struct {
	Kit    []string `yaml:"kit"`
	Custom []string `yaml:"custom"`
}

// Agents names the subagents of the project.
type Agents struct {
	Builtin []string      `yaml:"builtin"`
	Custom  []CustomAgent `yaml:"custom"`
}

// CustomAgent is a subagent the project defines itself.
type CustomAgent struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tools       string `yaml:"tools"`
	Model       string `yaml:"model"`
	Trigger     string `yaml:"trigger"`
	Condition   string `yaml:"condition"`
}

// Review configures the automated review of a pull request.
type Review struct {
	ScoreThreshold     int                 `yaml:"score_threshold"`
	MergeStrategy      string              `yaml:"merge_strategy"`
	MultiModel         bool                `yaml:"multi_model"`
	SecondaryReviewers []SecondaryReviewer `yaml:"secondary_reviewers"`
	JiTTest            JiTTest             `yaml:"jittest"`
	// SeverityConsistency gates the check that a verdict and the severity
	// of its findings agree: a score below the threshold lists at least one
	// Critical or Important finding, and a passing score carries no Critical
	// one. nil means on.
	SeverityConsistency *bool `yaml:"severity_consistency"`
	// HumanFeedback makes a maintainer's review of a pull request outrank
	// the automated verdict: the pull request is not merged over open
	// feedback, the Worker is sent back with it, and it is recorded as human
	// evidence for the learning loop. Merging never waits for a review;
	// feedback written after the merge is picked up before the next dispatch
	// and distilled into lessons. nil means on.
	HumanFeedback *bool `yaml:"human_feedback"`
}

// SeverityCheckEnabled reports whether the severity check is on (default).
func (c *Review) SeverityCheckEnabled() bool {
	return c.SeverityConsistency == nil || *c.SeverityConsistency
}

// HumanFeedbackEnabled reports whether a maintainer's review feedback is read
// and acted on (default).
func (c *Review) HumanFeedbackEnabled() bool {
	return c.HumanFeedback == nil || *c.HumanFeedback
}

// MergeStrategies are the ways a pull request can be merged.
var MergeStrategies = []string{"squash", "merge", "rebase"}

// Strategy returns the way pull requests are merged: the configured one when
// it is one of MergeStrategies, squash otherwise.
func (c *Review) Strategy() string {
	for _, known := range MergeStrategies {
		if c.MergeStrategy == known {
			return known
		}
	}
	return "squash"
}

// SecondaryReviewer is a further reviewer asked when review.multi_model is on.
type SecondaryReviewer struct {
	Backend   string `yaml:"backend"`
	Model     string `yaml:"model"`
	FocusArea string `yaml:"focus_area"`
}

// JiTTest configures the tests that are generated from a pull request's diff
// at review time, run once and thrown away.
type JiTTest struct {
	Enabled        *bool  `yaml:"enabled"`
	MaxTests       int    `yaml:"max_tests"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	FailurePolicy  string `yaml:"failure_policy"`
	Model          string `yaml:"model"`
}

// IsEnabled reports whether JiT tests are on. They are off by default.
func (c *JiTTest) IsEnabled() bool {
	return c.Enabled != nil && *c.Enabled
}

// Escalation bounds how often the workflow tries again before it asks for a
// person.
type Escalation struct {
	MaxReviewAttempts int `yaml:"max_review_attempts"`
	RetryCount        int `yaml:"retry_count"`
	RetryDelaySeconds int `yaml:"retry_delay_seconds"`
}

// Timeouts bound the external commands the Worker runs.
type Timeouts struct {
	GitSeconds       int `yaml:"git_seconds"`
	GHSeconds        int `yaml:"gh_seconds"`
	CodexMinutes     int `yaml:"codex_minutes"`
	GHRetryCount     int `yaml:"gh_retry_count"`
	GHRetryBaseDelay int `yaml:"gh_retry_base_delay"`
}

// Feedback configures what a Worker is told about earlier reviews.
type Feedback struct {
	Enabled            *bool `yaml:"enabled"`
	MaxHistoryInPrompt int   `yaml:"max_history_in_prompt"`
}

// IsEnabled reports whether review feedback is recorded (default).
func (c *Feedback) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// Hook is one command run at a point of the workflow.
type Hook struct {
	Command string `yaml:"command"`
	// Timeout is a duration such as "30s". Empty means none.
	Timeout string `yaml:"timeout"`
	// OnFailure is warn (default), abort or ignore.
	OnFailure string            `yaml:"on_failure"`
	Env       map[string]string `yaml:"env"`
}

// Hooks holds the commands run at each point of the workflow.
type Hooks struct {
	PreDispatch  []Hook `yaml:"pre_dispatch"`
	PostDispatch []Hook `yaml:"post_dispatch"`
	PreReview    []Hook `yaml:"pre_review"`
	PostReview   []Hook `yaml:"post_review"`
	OnMerge      []Hook `yaml:"on_merge"`
	OnFailure    []Hook `yaml:"on_failure"`
}

// GetHooks returns the hooks of an event, by its name in the file.
func (c *Hooks) GetHooks(event string) []Hook {
	switch event {
	case "pre_dispatch":
		return c.PreDispatch
	case "post_dispatch":
		return c.PostDispatch
	case "pre_review":
		return c.PreReview
	case "post_review":
		return c.PostReview
	case "on_merge":
		return c.OnMerge
	case "on_failure":
		return c.OnFailure
	default:
		return nil
	}
}

// Worker configures the agent that writes the code.
type Worker struct {
	// Backend is codex (default) or claude-code.
	Backend    string     `yaml:"backend"`
	Codex      Codex      `yaml:"codex"`
	ClaudeCode ClaudeCode `yaml:"claude_code"`
	// KnowledgeGraph controls whether a ticket's slice of
	// .understand-anything/knowledge-graph.json goes into the Worker's
	// prompt: auto (default, when the file is there) or off.
	KnowledgeGraph string `yaml:"knowledge_graph"`
}

// Codex holds the settings of the codex backend.
type Codex struct {
	FullAuto    *bool `yaml:"full_auto"`
	MaxAttempts int   `yaml:"max_attempts"`
}

// IsFullAuto reports whether codex runs in full-auto mode (default).
func (c *Codex) IsFullAuto() bool {
	return c.FullAuto == nil || *c.FullAuto
}

// ClaudeCode holds the settings of the claude-code backend.
type ClaudeCode struct {
	Model                      string `yaml:"model"`
	MaxTurns                   int    `yaml:"max_turns"`
	DangerouslySkipPermissions bool   `yaml:"dangerously_skip_permissions"`
}

// Lessons configures the learning loop: record, distill, inject, verify.
type Lessons struct {
	Enabled        *bool            `yaml:"enabled"`
	MaxActive      int              `yaml:"max_active"`
	InjectTopK     int              `yaml:"inject_top_k"`
	InjectMaxChars int              `yaml:"inject_max_chars"`
	Distiller      LessonsDistiller `yaml:"distiller"`
}

// IsEnabled reports whether the learning loop is on (default).
func (c *Lessons) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// LessonsDistiller configures the model that turns reviews into lessons.
type LessonsDistiller struct {
	Backend        string `yaml:"backend"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// warnReviewFail says once per process that the file uses the old name of a
// label. A command reads the configuration several times.
var warnReviewFail sync.Once

// LoadFile reads the configuration at path and fills in what the file leaves
// out.
//
// A file that cannot be read gives the error of the read as it is, so that
// os.IsNotExist tells a project without a configuration from one whose
// configuration is broken. A file that is not valid YAML, or holds a value of
// the wrong kind, is an error whoever asks: a configuration that is half read
// is not used.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, &ParseError{File: filepath.ToSlash(path), Err: err}
	}
	if cfg.GitHub.Labels.ReviewFailed == "" && cfg.GitHub.Labels.ReviewFail != "" {
		cfg.GitHub.Labels.ReviewFailed = cfg.GitHub.Labels.ReviewFail
		warnReviewFail.Do(func() {
			fmt.Fprintln(os.Stderr, "warning: config uses deprecated 'review_fail' label key; run 'awkit upgrade' to migrate")
		})
	}
	cfg.GitHub.Labels.ReviewFail = ""
	cfg.applyDefaults()
	return &cfg, nil
}

// Load reads the configuration of the project at root.
func Load(root string) (*Config, error) {
	return LoadFile(Path(root))
}

// ParseError says that the configuration was read and is not valid.
type ParseError struct {
	File string
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s is not valid: %s", e.File, strings.TrimPrefix(e.Err.Error(), "yaml: "))
}

func (e *ParseError) Unwrap() error { return e.Err }

// Default returns the configuration of a file that says nothing: every
// default, no repo. It is what a command that goes on without a readable
// configuration works with.
func Default() *Config {
	var cfg Config
	cfg.applyDefaults()
	return &cfg
}

// applyDefaults fills in what the file left out. This is the one place a
// default is decided.
func (c *Config) applyDefaults() {
	labels, defaults := &c.GitHub.Labels, DefaultLabels()
	for _, label := range []struct {
		value    *string
		fallback string
	}{
		{&labels.Task, defaults.Task}, {&labels.InProgress, defaults.InProgress}, {&labels.PRReady, defaults.PRReady},
		{&labels.WorkerFailed, defaults.WorkerFailed}, {&labels.NeedsHumanReview, defaults.NeedsHumanReview},
		{&labels.ReviewFailed, defaults.ReviewFailed}, {&labels.MergeConflict, defaults.MergeConflict},
		{&labels.NeedsRebase, defaults.NeedsRebase}, {&labels.Completed, defaults.Completed},
	} {
		if *label.value == "" {
			*label.value = label.fallback
		}
	}

	if c.Git.ReleaseBranch == "" {
		c.Git.ReleaseBranch = "main"
	}
	if c.Git.CommitFormat == "" {
		c.Git.CommitFormat = "[type] subject"
	}
	if len(c.Rules.Kit) == 0 {
		c.Rules.Kit = []string{"git-workflow"}
	}
	if len(c.Agents.Builtin) == 0 {
		c.Agents.Builtin = []string{"pr-reviewer", "conflict-resolver"}
	}

	positive(&c.Review.ScoreThreshold, 7)
	if c.Review.MergeStrategy == "" {
		c.Review.MergeStrategy = "squash"
	}
	positive(&c.Review.JiTTest.MaxTests, 5)
	positive(&c.Review.JiTTest.TimeoutSeconds, 120)
	if c.Review.JiTTest.FailurePolicy == "" {
		c.Review.JiTTest.FailurePolicy = "warn"
	}
	if c.Review.JiTTest.Model == "" {
		c.Review.JiTTest.Model = "claude-sonnet-4-6"
	}

	positive(&c.Escalation.MaxReviewAttempts, 3)
	// Zero retries cannot be asked for: a file that leaves the key out reads
	// as zero. A negative number asks for none.
	switch {
	case c.Escalation.RetryCount == 0:
		c.Escalation.RetryCount = 2
	case c.Escalation.RetryCount < 0:
		c.Escalation.RetryCount = 0
	}
	positive(&c.Escalation.RetryDelaySeconds, 5)

	positive(&c.Timeouts.GitSeconds, 120)
	positive(&c.Timeouts.GHSeconds, 60)
	positive(&c.Timeouts.CodexMinutes, 30)
	positive(&c.Timeouts.GHRetryCount, 3)
	positive(&c.Timeouts.GHRetryBaseDelay, 2)

	positive(&c.Feedback.MaxHistoryInPrompt, 10)

	if c.Worker.Backend == "" {
		c.Worker.Backend = "codex"
	}
	positive(&c.Worker.Codex.MaxAttempts, 1)
	if c.Worker.ClaudeCode.Model == "" {
		c.Worker.ClaudeCode.Model = "sonnet"
	}
	positive(&c.Worker.ClaudeCode.MaxTurns, 50)

	positive(&c.Lessons.MaxActive, 30)
	positive(&c.Lessons.InjectTopK, 3)
	positive(&c.Lessons.InjectMaxChars, 800)
	if c.Lessons.Distiller.Model == "" {
		c.Lessons.Distiller.Model = "sonnet"
	}
	positive(&c.Lessons.Distiller.TimeoutSeconds, 60)
}

// positive sets a number that is zero or less to its default.
func positive(value *int, fallback int) {
	if *value <= 0 {
		*value = fallback
	}
}

// BaseBranch returns the branch pull requests go to. A file that names none
// fails validation; a Worker that runs all the same works on develop.
func (c *Config) BaseBranch() string {
	if c.Git.IntegrationBranch == "" {
		return "develop"
	}
	return c.Git.IntegrationBranch
}

// FindRepo returns the repo a ticket names, or nil. The case of the name does
// not matter: a ticket's repo is written by a person, or by a model.
func (c *Config) FindRepo(name string) *Repo {
	for i := range c.Repos {
		if strings.EqualFold(c.Repos[i].Name, name) {
			return &c.Repos[i]
		}
	}
	return nil
}

// TicketRepos lists the repos a ticket can be for: the root of the
// repository, always, and the repos of the file. A name the file writes
// twice, in whatever case, is one repo.
func (c *Config) TicketRepos() []string {
	names := []string{ticket.RootRepo}
	for _, repo := range c.Repos {
		if repo.Name != "" && !containsFold(names, repo.Name) {
			names = append(names, repo.Name)
		}
	}
	return names
}

// HasTicketRepo reports whether a ticket can be for the named repo, whatever
// the case of the name.
func (c *Config) HasTicketRepo(name string) bool {
	return containsFold(c.TicketRepos(), name)
}

func containsFold(names []string, name string) bool {
	for _, known := range names {
		if strings.EqualFold(known, name) {
			return true
		}
	}
	return false
}

// GetVerifyCommands returns the build and test commands of the repo a ticket
// names.
func (c *Config) GetVerifyCommands(repoName string) []string {
	repo := c.FindRepo(repoName)
	if repo == nil {
		return nil
	}
	var commands []string
	if repo.Verify.Build != "" {
		commands = append(commands, repo.Verify.Build)
	}
	if repo.Verify.Test != "" {
		commands = append(commands, repo.Verify.Test)
	}
	return commands
}
