package analyzer

import "github.com/silver2dream/ai-workflow-kit/internal/config"

// The configuration is read by package config, for everyone. These are the
// names the decision engine and its callers have for it.
type (
	Config                  = config.Config
	HookDef                 = config.Hook
	HooksConfig             = config.Hooks
	WorkerConfig            = config.Worker
	CodexConfig             = config.Codex
	ClaudeCodeConfig        = config.ClaudeCode
	ReviewConfig            = config.Review
	LessonsConfig           = config.Lessons
	LessonsDistillerConfig  = config.LessonsDistiller
	SecondaryReviewerConfig = config.SecondaryReviewer
	JiTTestConfig           = config.JiTTest
	FeedbackConfig          = config.Feedback
	EscalationConfig        = config.Escalation
	RepoConfig              = config.Repo
	VerifyConfig            = config.Verify
	GitHubConfig            = config.GitHub
	LabelsConfig            = config.Labels
)

// DefaultLabels returns the label names used when the configuration names
// none.
func DefaultLabels() LabelsConfig {
	return config.DefaultLabels()
}

// LoadConfig reads the configuration file at path.
func LoadConfig(path string) (*Config, error) {
	return config.LoadFile(path)
}
