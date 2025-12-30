package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		wantErrors int
	}{
		{
			name: "valid config",
			config: Config{
				Project: ProjectConfig{Name: "test-project", Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
				Repos: []RepoConfig{
					{Name: "backend", Path: "backend/", Type: "directory"},
				},
			},
			wantErrors: 0,
		},
		{
			name:       "empty config",
			config:     Config{},
			wantErrors: 3, // project.name, project.type, git.integration_branch
		},
		{
			name: "invalid project type",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "invalid"},
				Git:     GitConfig{IntegrationBranch: "develop"},
			},
			wantErrors: 1,
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
			wantErrors: 1,
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
			wantErrors: 1,
		},
		{
			name: "missing repo path",
			config: Config{
				Project: ProjectConfig{Name: "test", Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
				Repos: []RepoConfig{
					{Name: "backend", Type: "directory"},
				},
			},
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.config.Validate()
			if len(errors) != tt.wantErrors {
				t.Errorf("Validate() got %d errors, want %d", len(errors), tt.wantErrors)
				for _, e := range errors {
					t.Logf("  error: %s", e.Error())
				}
			}
		})
	}
}

func TestValidationErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		contains string
	}{
		{
			name:     "with expected",
			err:      ValidationError{Field: "project.type", Message: "invalid", Expected: "monorepo"},
			contains: "expected: monorepo",
		},
		{
			name:     "without expected",
			err:      ValidationError{Field: "project.name", Message: "required"},
			contains: "project.name: required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Error() = %q, want to contain %q", result, tt.contains)
			}
		})
	}
}

func TestBuildContext(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		wantSubmodules bool
		wantDirs       bool
		wantSingleRepo bool
	}{
		{
			name: "monorepo with submodules",
			config: &Config{
				Project: ProjectConfig{Type: "monorepo"},
				Repos: []RepoConfig{
					{Type: "submodule"},
				},
			},
			wantSubmodules: true,
		},
		{
			name: "monorepo with directories",
			config: &Config{
				Project: ProjectConfig{Type: "monorepo"},
				Repos: []RepoConfig{
					{Type: "directory"},
				},
			},
			wantDirs: true,
		},
		{
			name: "single repo",
			config: &Config{
				Project: ProjectConfig{Type: "single-repo"},
				Repos: []RepoConfig{
					{Type: "root"},
				},
			},
			wantSingleRepo: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := buildContext(tt.config)
			if ctx.HasSubmodules != tt.wantSubmodules {
				t.Errorf("HasSubmodules = %v, want %v", ctx.HasSubmodules, tt.wantSubmodules)
			}
			if ctx.HasDirectories != tt.wantDirs {
				t.Errorf("HasDirectories = %v, want %v", ctx.HasDirectories, tt.wantDirs)
			}
			if ctx.IsSingleRepo != tt.wantSingleRepo {
				t.Errorf("IsSingleRepo = %v, want %v", ctx.IsSingleRepo, tt.wantSingleRepo)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create temp dir
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Write test config
	configContent := `
project:
  name: test-project
  type: monorepo
git:
  integration_branch: develop
  release_branch: main
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: go build ./...
      test: go test ./...
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if config.Project.Name != "test-project" {
		t.Errorf("Project.Name = %q, want %q", config.Project.Name, "test-project")
	}
	if config.Git.IntegrationBranch != "develop" {
		t.Errorf("Git.IntegrationBranch = %q, want %q", config.Git.IntegrationBranch, "develop")
	}
	if len(config.Repos) != 1 {
		t.Errorf("len(Repos) = %d, want 1", len(config.Repos))
	}

	// Check defaults
	if config.Specs.BasePath != ".ai/specs" {
		t.Errorf("Specs.BasePath = %q, want %q", config.Specs.BasePath, ".ai/specs")
	}
	if config.Git.CommitFormat != "[type] subject" {
		t.Errorf("Git.CommitFormat = %q, want %q", config.Git.CommitFormat, "[type] subject")
	}
}

func TestLoadConfigNotFound(t *testing.T) {
	_, err := loadConfig("/nonexistent/path/workflow.yaml")
	if err == nil {
		t.Error("loadConfig() expected error for nonexistent file")
	}
}

func TestGenerateDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup config
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
project:
  name: test-project
  type: single-repo
git:
  integration_branch: develop
repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: go build ./...
      test: go test ./...
`
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Generate(Options{
		StateRoot: tmpDir,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(result.GeneratedFiles) == 0 {
		t.Error("Generate() returned no files in dry-run mode")
	}

	// Verify files were NOT created in dry-run mode
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); err == nil {
		t.Error("CLAUDE.md should not exist in dry-run mode")
	}
}

func TestGenerateCreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup config
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
project:
  name: test-project
  type: single-repo
git:
  integration_branch: develop
  release_branch: main
repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: go build ./...
      test: go test ./...
rules:
  kit:
    - git-workflow
`
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Generate(Options{
		StateRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(result.GeneratedFiles) == 0 {
		t.Error("Generate() returned no files")
	}

	// Verify files were created
	expectedFiles := []string{
		filepath.Join(tmpDir, "CLAUDE.md"),
		filepath.Join(tmpDir, "AGENTS.md"),
		filepath.Join(tmpDir, ".ai", "rules", "_kit", "git-workflow.md"),
		filepath.Join(tmpDir, ".claude", "settings.local.json"),
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("expected file not created: %s", f)
		}
	}
}

func TestGenerateRequiresStateRoot(t *testing.T) {
	_, err := Generate(Options{})
	if err == nil {
		t.Error("Generate() expected error when StateRoot is empty")
	}
}

func TestGenerateValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup invalid config (missing required fields)
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
project:
  name: ""
  type: invalid
`
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Generate(Options{StateRoot: tmpDir})
	if err == nil {
		t.Error("Generate() expected validation error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error should mention validation: %v", err)
	}
}

func TestGenerateCIContent(t *testing.T) {
	repo := RepoConfig{
		Name:     "backend",
		Path:     "backend/",
		Type:     "directory",
		Language: "go",
		Verify: VerifyConfig{
			Build: "go build ./...",
			Test:  "go test ./...",
		},
	}
	git := GitConfig{
		IntegrationBranch: "develop",
		ReleaseBranch:     "main",
	}

	content := generateCIContent(repo, git, "go")

	if !strings.Contains(content, "backend CI") {
		t.Error("CI content should contain repo name")
	}
	if !strings.Contains(content, "setup-go") {
		t.Error("Go CI should use setup-go action")
	}
	if !strings.Contains(content, "go build") {
		t.Error("CI content should contain build command")
	}
}

func TestGenerateUnityCIContent(t *testing.T) {
	repo := RepoConfig{
		Name:     "frontend",
		Language: "unity",
	}
	git := GitConfig{
		IntegrationBranch: "develop",
		ReleaseBranch:     "main",
	}

	content := generateUnityCIContent(repo, git)

	if !strings.Contains(content, "frontend CI") {
		t.Error("Unity CI should contain repo name")
	}
	if !strings.Contains(content, "manifest.json") {
		t.Error("Unity CI should check manifest.json")
	}
	if !strings.Contains(content, ".meta") {
		t.Error("Unity CI should check .meta files")
	}
}
