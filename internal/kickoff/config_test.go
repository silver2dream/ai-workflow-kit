package kickoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	configContent := `
version: "1.0"
project:
  name: "test-project"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
git:
  integration_branch: "develop"
  release_branch: "main"
specs:
  base_path: ".ai/specs"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", config.Version)
	}

	if config.Project.Name != "test-project" {
		t.Errorf("Expected project name test-project, got %s", config.Project.Name)
	}

	if config.Project.Type != "monorepo" {
		t.Errorf("Expected project type monorepo, got %s", config.Project.Type)
	}

	if len(config.Repos) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(config.Repos))
	}

	if config.Repos[0].Name != "backend" {
		t.Errorf("Expected repo name backend, got %s", config.Repos[0].Name)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/workflow.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorField  string
	}{
		{
			name: "valid config",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
				Repos: []RepoConfig{
					{Name: "backend", Path: "backend/", Type: "directory"},
				},
			},
			expectError: false,
		},
		{
			name: "missing project name",
			config: Config{
				Project: ProjectConfig{Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
			},
			expectError: true,
			errorField:  "project.name",
		},
		{
			name: "missing project type",
			config: Config{
				Project: ProjectConfig{Name: "test"},
				Git:     GitConfig{IntegrationBranch: "develop"},
			},
			expectError: true,
			errorField:  "project.type",
		},
		{
			name: "invalid project type",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "invalid"},
				Git:     GitConfig{IntegrationBranch: "develop"},
			},
			expectError: true,
			errorField:  "project.type",
		},
		{
			name: "missing integration branch",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "monorepo"},
			},
			expectError: true,
			errorField:  "git.integration_branch",
		},
		{
			name: "invalid repo type",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
				Repos: []RepoConfig{
					{Name: "backend", Path: "backend/", Type: "invalid"},
				},
			},
			expectError: true,
			errorField:  "repos[0].type",
		},
		{
			name: "missing repo name",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
				Repos: []RepoConfig{
					{Path: "backend/", Type: "directory"},
				},
			},
			expectError: true,
			errorField:  "repos[0].name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.config.Validate()
			if tt.expectError {
				if len(errors) == 0 {
					t.Error("Expected validation error, got none")
				} else if errors[0].Field != tt.errorField {
					t.Errorf("Expected error field %s, got %s", tt.errorField, errors[0].Field)
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("Expected no errors, got %v", errors)
				}
			}
		})
	}
}

func TestConfig_ValidatePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create backend directory
	backendDir := filepath.Join(tmpDir, "backend")
	if err := os.MkdirAll(backendDir, 0755); err != nil {
		t.Fatalf("Failed to create backend dir: %v", err)
	}

	// Create specs directory
	specsDir := filepath.Join(tmpDir, ".ai", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorField  string
	}{
		{
			name: "valid paths",
			config: Config{
				Repos: []RepoConfig{
					{Name: "backend", Path: "backend/"},
				},
				Specs: SpecsConfig{BasePath: ".ai/specs"},
			},
			expectError: false,
		},
		{
			name: "nonexistent repo path",
			config: Config{
				Repos: []RepoConfig{
					{Name: "frontend", Path: "frontend/"},
				},
			},
			expectError: true,
			errorField:  "repos[0].path",
		},
		{
			name: "nonexistent specs path",
			config: Config{
				Specs: SpecsConfig{BasePath: "nonexistent/specs"},
			},
			expectError: true,
			errorField:  "specs.base_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.config.ValidatePaths(tmpDir)
			if tt.expectError {
				if len(errors) == 0 {
					t.Error("Expected validation error, got none")
				} else if errors[0].Field != tt.errorField {
					t.Errorf("Expected error field %s, got %s", tt.errorField, errors[0].Field)
				}
			} else {
				if len(errors) > 0 {
					t.Errorf("Expected no errors, got %v", errors)
				}
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name:     "without expected",
			err:      ValidationError{Field: "project.name", Message: "required field is missing"},
			expected: "project.name: required field is missing",
		},
		{
			name:     "with expected",
			err:      ValidationError{Field: "project.type", Message: "invalid value", Expected: "monorepo or single-repo"},
			expected: "project.type: invalid value (expected: monorepo or single-repo)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, got)
			}
		})
	}
}
