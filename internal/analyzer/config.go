package analyzer

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the workflow configuration
type Config struct {
	Specs  SpecsConfig  `yaml:"specs"`
	GitHub GitHubConfig `yaml:"github"`
}

// SpecsConfig represents the specs section
type SpecsConfig struct {
	BasePath string   `yaml:"base_path"`
	Active   []string `yaml:"active"`
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

	return &cfg, nil
}
