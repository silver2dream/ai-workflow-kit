package kickoff

import (
	"errors"
	"fmt"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// The configuration is read by package config, for everyone. These are the
// names the pre-flight checks and their callers have for it.
type (
	Config          = config.Config
	ProjectConfig   = config.Project
	RepoConfig      = config.Repo
	VerifyConfig    = config.Verify
	GitConfig       = config.Git
	GitHubConfig    = config.GitHub
	ValidationError = config.ValidationError
)

// LoadConfig reads the configuration file at path, and says whether it could
// not be read or could not be understood.
func LoadConfig(path string) (*Config, error) {
	cfg, err := config.LoadFile(path)
	if err != nil {
		var parse *config.ParseError
		if errors.As(err, &parse) {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return cfg, nil
}
