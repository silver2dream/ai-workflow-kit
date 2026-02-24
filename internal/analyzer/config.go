package analyzer

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the workflow configuration
type Config struct {
	Specs      SpecsConfig      `yaml:"specs"`
	GitHub     GitHubConfig     `yaml:"github"`
	Repos      []RepoConfig     `yaml:"repos"`
	Escalation EscalationConfig `yaml:"escalation"`
}

// EscalationConfig represents escalation/retry limits
type EscalationConfig struct {
	MaxReviewAttempts int `yaml:"max_review_attempts"`
}

// RepoConfig represents a repository configuration
type RepoConfig struct {
	Name     string       `yaml:"name"`
	Path     string       `yaml:"path"`
	Type     string       `yaml:"type"`     // root, directory, submodule
	Language string       `yaml:"language"` // go, node, unity, python, etc.
	Verify   VerifyConfig `yaml:"verify"`
}

// VerifyConfig represents verification commands for a repo
type VerifyConfig struct {
	Build string `yaml:"build"`
	Test  string `yaml:"test"`
}

// GetRepoByName returns the repo config by name
func (c *Config) GetRepoByName(name string) *RepoConfig {
	for i := range c.Repos {
		if c.Repos[i].Name == name {
			return &c.Repos[i]
		}
	}
	return nil
}

// GetVerifyCommands returns build and test commands for a repo
func (c *Config) GetVerifyCommands(repoName string) []string {
	repo := c.GetRepoByName(repoName)
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

// TrackingMode constants
const (
	TrackingModeTasksMd   = "tasks_md"
	TrackingModeGitHubEpic = "github_epic"
)

// TrackingConfig represents task tracking configuration
type TrackingConfig struct {
	Mode       string         `yaml:"mode"`        // "tasks_md" (default) | "github_epic"
	EpicIssues map[string]int `yaml:"epic_issues"` // spec_name → tracking issue number
	Audit      EpicAuditConfig `yaml:"audit"`       // epic audit settings
}

// EpicAuditConfig represents epic audit settings for gap detection.
// If the audit section is absent from config, audit is enabled by default.
type EpicAuditConfig struct {
	Enabled             *bool `yaml:"enabled"`               // nil = true (default enabled)
	MilestoneInterval   int   `yaml:"milestone_interval"`    // trigger every N% completion (default: 25)
	MaxAdditionsPerAudit int  `yaml:"max_additions_per_audit"` // max tasks to add per audit (default: 5)
}

// IsAuditEnabled returns whether epic audit is enabled (default: true)
func (c *EpicAuditConfig) IsAuditEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// SpecsConfig represents the specs section
type SpecsConfig struct {
	BasePath string         `yaml:"base_path"`
	Active   []string       `yaml:"active"`
	Tracking TrackingConfig `yaml:"tracking"`
}

// IsEpicMode returns true if the config uses GitHub Epic tracking mode
func (c *Config) IsEpicMode() bool {
	return c.Specs.Tracking.Mode == TrackingModeGitHubEpic
}

// GetEpicIssue returns the tracking issue number for a spec, or 0 if not configured
func (c *Config) GetEpicIssue(specName string) int {
	if c.Specs.Tracking.EpicIssues == nil {
		return 0
	}
	return c.Specs.Tracking.EpicIssues[specName]
}

// GitHubConfig represents the github section
type GitHubConfig struct {
	Repo   string       `yaml:"repo"`
	Labels LabelsConfig `yaml:"labels"`
}

// LabelsConfig represents GitHub labels configuration
type LabelsConfig struct {
	Task             string `yaml:"task"`
	InProgress       string `yaml:"in_progress"`
	PRReady          string `yaml:"pr_ready"`
	WorkerFailed     string `yaml:"worker_failed"`
	NeedsHumanReview string `yaml:"needs_human_review"`
	ReviewFailed     string `yaml:"review_failed"`
	MergeConflict    string `yaml:"merge_conflict"`
	NeedsRebase      string `yaml:"needs_rebase"`
	Completed        string `yaml:"completed"`

	// Deprecated: old key name, kept for backward compat with v1.0 configs
	ReviewFail string `yaml:"review_fail,omitempty"`
}

// DefaultLabels returns default label names
func DefaultLabels() LabelsConfig {
	return LabelsConfig{
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

// LoadConfig loads the workflow configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Backward compat: support old "review_fail" key from v1.0 configs
	if cfg.GitHub.Labels.ReviewFailed == "" && cfg.GitHub.Labels.ReviewFail != "" {
		cfg.GitHub.Labels.ReviewFailed = cfg.GitHub.Labels.ReviewFail
		fmt.Fprintf(os.Stderr, "warning: config uses deprecated 'review_fail' label key; run 'awkit upgrade' to migrate\n")
	}
	cfg.GitHub.Labels.ReviewFail = "" // clear deprecated field

	// Apply defaults
	if cfg.Specs.BasePath == "" {
		cfg.Specs.BasePath = ".ai/specs"
	}
	if cfg.GitHub.Labels.Task == "" {
		cfg.GitHub.Labels.Task = "ai-task"
	}
	if cfg.GitHub.Labels.InProgress == "" {
		cfg.GitHub.Labels.InProgress = "in-progress"
	}
	if cfg.GitHub.Labels.PRReady == "" {
		cfg.GitHub.Labels.PRReady = "pr-ready"
	}
	if cfg.GitHub.Labels.WorkerFailed == "" {
		cfg.GitHub.Labels.WorkerFailed = "worker-failed"
	}
	if cfg.GitHub.Labels.NeedsHumanReview == "" {
		cfg.GitHub.Labels.NeedsHumanReview = "needs-human-review"
	}
	if cfg.GitHub.Labels.ReviewFailed == "" {
		cfg.GitHub.Labels.ReviewFailed = "review-failed"
	}
	if cfg.GitHub.Labels.MergeConflict == "" {
		cfg.GitHub.Labels.MergeConflict = "merge-conflict"
	}
	if cfg.GitHub.Labels.NeedsRebase == "" {
		cfg.GitHub.Labels.NeedsRebase = "needs-rebase"
	}
	if cfg.GitHub.Labels.Completed == "" {
		cfg.GitHub.Labels.Completed = "completed"
	}
	if cfg.Escalation.MaxReviewAttempts <= 0 {
		cfg.Escalation.MaxReviewAttempts = 3
	}
	if cfg.Specs.Tracking.Mode == "" {
		cfg.Specs.Tracking.Mode = TrackingModeTasksMd
	}
	if cfg.Specs.Tracking.Audit.MilestoneInterval <= 0 {
		cfg.Specs.Tracking.Audit.MilestoneInterval = 25
	}
	if cfg.Specs.Tracking.Audit.MaxAdditionsPerAudit <= 0 {
		cfg.Specs.Tracking.Audit.MaxAdditionsPerAudit = 5
	}

	return &cfg, nil
}
