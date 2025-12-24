package kickoff

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the workflow.yaml configuration
type Config struct {
	Version string        `yaml:"version"`
	Project ProjectConfig `yaml:"project"`
	Repos   []RepoConfig  `yaml:"repos"`
	Git     GitConfig     `yaml:"git"`
	Specs   SpecsConfig   `yaml:"specs"`
	GitHub  GitHubConfig  `yaml:"github"`
}

// ProjectConfig holds project-level settings
type ProjectConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"` // monorepo, single-repo
}

// RepoConfig holds repository configuration
type RepoConfig struct {
	Name     string       `yaml:"name"`
	Path     string       `yaml:"path"`
	Type     string       `yaml:"type"`     // root, directory, submodule
	Language string       `yaml:"language"` // go, node, python, etc.
	Verify   VerifyConfig `yaml:"verify"`
}

// VerifyConfig holds build/test commands
type VerifyConfig struct {
	Build string `yaml:"build"`
	Test  string `yaml:"test"`
}

// GitConfig holds git-related settings
type GitConfig struct {
	IntegrationBranch string `yaml:"integration_branch"`
	ReleaseBranch     string `yaml:"release_branch"`
	CommitFormat      string `yaml:"commit_format"`
}

// SpecsConfig holds specs directory settings
type SpecsConfig struct {
	BasePath string   `yaml:"base_path"`
	Active   []string `yaml:"active"`
}

// GitHubConfig holds GitHub-related settings
type GitHubConfig struct {
	Repo string `yaml:"repo"`
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field    string
	Message  string
	Expected string
}

func (e ValidationError) Error() string {
	if e.Expected != "" {
		return fmt.Sprintf("%s: %s (expected: %s)", e.Field, e.Message, e.Expected)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// LoadConfig reads and parses the workflow.yaml configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// Validate checks if the configuration has all required fields
func (c *Config) Validate() []ValidationError {
	var errors []ValidationError

	if c.Project.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "project.name",
			Message: "required field is missing",
		})
	}

	if c.Project.Type == "" {
		errors = append(errors, ValidationError{
			Field:    "project.type",
			Message:  "required field is missing",
			Expected: "monorepo or single-repo",
		})
	} else if c.Project.Type != "monorepo" && c.Project.Type != "single-repo" {
		errors = append(errors, ValidationError{
			Field:    "project.type",
			Message:  fmt.Sprintf("invalid value: %s", c.Project.Type),
			Expected: "monorepo or single-repo",
		})
	}

	if c.Git.IntegrationBranch == "" {
		errors = append(errors, ValidationError{
			Field:   "git.integration_branch",
			Message: "required field is missing",
		})
	}

	// Validate repos
	for i, repo := range c.Repos {
		if repo.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("repos[%d].name", i),
				Message: "required field is missing",
			})
		}
		if repo.Path == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("repos[%d].path", i),
				Message: "required field is missing",
			})
		}
		if repo.Type != "" && repo.Type != "root" && repo.Type != "directory" && repo.Type != "submodule" {
			errors = append(errors, ValidationError{
				Field:    fmt.Sprintf("repos[%d].type", i),
				Message:  fmt.Sprintf("invalid value: %s", repo.Type),
				Expected: "root, directory, or submodule",
			})
		}
	}

	return errors
}

// ValidatePaths checks if referenced paths exist
func (c *Config) ValidatePaths(baseDir string) []ValidationError {
	var errors []ValidationError

	for i, repo := range c.Repos {
		repoPath := filepath.Join(baseDir, repo.Path)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("repos[%d].path", i),
				Message: fmt.Sprintf("path does not exist: %s", repo.Path),
			})
		}
	}

	if c.Specs.BasePath != "" {
		specsPath := filepath.Join(baseDir, c.Specs.BasePath)
		if _, err := os.Stat(specsPath); os.IsNotExist(err) {
			errors = append(errors, ValidationError{
				Field:   "specs.base_path",
				Message: fmt.Sprintf("path does not exist: %s", c.Specs.BasePath),
			})
		}
	}

	return errors
}
