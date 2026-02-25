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

// TestConfigValidation_YAMLSyntaxError tests that malformed YAML fails to parse
func TestConfigValidation_YAMLSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Write malformed YAML (invalid indentation and syntax)
	malformedYAML := `
project:
  name: test
  type: monorepo
  invalid yaml here: [unclosed bracket
git
  integration_branch: develop
`
	if err := os.WriteFile(configPath, []byte(malformedYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := loadConfig(configPath)
	if err == nil {
		t.Error("loadConfig() expected error for malformed YAML")
	}
}

// TestConfigValidation_MissingProject tests that missing project section fails validation
func TestConfigValidation_MissingProject(t *testing.T) {
	config := Config{
		Git: GitConfig{IntegrationBranch: "develop"},
		Repos: []RepoConfig{
			{Name: "backend", Path: "backend/", Type: "directory"},
		},
	}

	errors := config.Validate()
	if len(errors) < 2 {
		t.Errorf("Validate() got %d errors, want at least 2 (project.name, project.type)", len(errors))
	}

	// Check that project.name and project.type errors are present
	var hasNameErr, hasTypeErr bool
	for _, e := range errors {
		if e.Field == "project.name" {
			hasNameErr = true
		}
		if e.Field == "project.type" {
			hasTypeErr = true
		}
	}
	if !hasNameErr {
		t.Error("expected error for project.name")
	}
	if !hasTypeErr {
		t.Error("expected error for project.type")
	}
}

// TestConfigValidation_MissingRepos tests that an empty repos section is handled
func TestConfigValidation_MissingRepos(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Repos:   nil, // empty repos
	}

	// Config with no repos should still pass validation
	// (repos being empty is valid - just means no repo-specific validation)
	errors := config.Validate()
	if len(errors) != 0 {
		t.Errorf("Validate() got %d errors, want 0 for config with no repos", len(errors))
		for _, e := range errors {
			t.Logf("  error: %s", e.Error())
		}
	}
}

// TestConfigValidation_InvalidRepoType tests that invalid repo type fails validation
func TestConfigValidation_InvalidRepoType(t *testing.T) {
	tests := []struct {
		name     string
		repoType string
		wantErr  bool
	}{
		{"valid root", "root", false},
		{"valid directory", "directory", false},
		{"valid submodule", "submodule", false},
		{"invalid type", "invalid_type", true},
		{"empty type", "", false}, // empty is allowed (not validated)
		{"typo in type", "directry", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Project: ProjectConfig{Name: "test", Type: "monorepo"},
				Git:     GitConfig{IntegrationBranch: "develop"},
				Repos: []RepoConfig{
					{Name: "backend", Path: "backend/", Type: tt.repoType},
				},
			}

			errors := config.Validate()
			hasRepoTypeErr := false
			for _, e := range errors {
				if strings.Contains(e.Field, "repos[0].type") {
					hasRepoTypeErr = true
					break
				}
			}

			if tt.wantErr && !hasRepoTypeErr {
				t.Errorf("expected repo type validation error for type %q", tt.repoType)
			}
			if !tt.wantErr && hasRepoTypeErr {
				t.Errorf("unexpected repo type validation error for type %q", tt.repoType)
			}
		})
	}
}

// TestConfigValidation_ValidConfig tests that a complete valid config passes validation
func TestConfigValidation_ValidConfig(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test-project", Type: "monorepo"},
		Git: GitConfig{
			IntegrationBranch: "develop",
			ReleaseBranch:     "main",
			CommitFormat:      "[type] subject",
		},
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend/",
				Type:     "directory",
				Language: "go",
				Verify: VerifyConfig{
					Build: "go build ./...",
					Test:  "go test ./...",
				},
			},
			{
				Name:     "frontend",
				Path:     "frontend/",
				Type:     "directory",
				Language: "typescript",
				Verify: VerifyConfig{
					Build: "npm run build",
					Test:  "npm test",
				},
			},
		},
		Rules: RulesConfig{
			Kit:    []string{"git-workflow"},
			Custom: []string{},
		},
		Specs: SpecsConfig{
			BasePath: ".ai/specs",
		},
	}

	errors := config.Validate()
	if len(errors) != 0 {
		t.Errorf("Validate() got %d errors, want 0 for valid config", len(errors))
		for _, e := range errors {
			t.Logf("  error: %s", e.Error())
		}
	}
}

// TestConfigValidation_RootTypePath tests that root type validation works correctly
func TestConfigValidation_RootTypePath(t *testing.T) {
	// Note: Current implementation doesn't validate root type path constraint
	// This test documents current behavior and can be updated if path validation is added
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "single-repo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Repos: []RepoConfig{
			{Name: "root", Path: "./", Type: "root"},
		},
	}

	errors := config.Validate()
	if len(errors) != 0 {
		t.Errorf("Validate() got %d errors, want 0 for valid root config", len(errors))
	}
}

// TestLoadConfig_YAMLSyntaxVariants tests various YAML syntax issues
func TestLoadConfig_YAMLSyntaxVariants(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid minimal",
			yaml: `
project:
  name: test
  type: single-repo
git:
  integration_branch: develop
repos:
  - name: root
    path: ./
    type: root
`,
			wantErr: false,
		},
		{
			name: "unclosed bracket in value",
			yaml: `
project:
  name: "test [unclosed"
git:
  integration_branch: develop
`,
			// Unquoted brackets are parsed as strings, quoted brackets are fine
			wantErr: false,
		},
		{
			name: "invalid indentation",
			yaml: `
project:
name: test
  type: monorepo
`,
			wantErr: true,
		},
		{
			name: "duplicate key",
			yaml: `
project:
  name: test
  name: duplicate
  type: monorepo
`,
			// Go YAML library treats duplicate keys as an error
			wantErr: true,
		},
		{
			name: "tabs in indentation",
			yaml: "project:\n\tname: test\n",
			// Go YAML library doesn't allow tabs for indentation
			wantErr: true,
		},
		{
			name: "completely malformed",
			yaml: `:::invalid:::yaml`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "workflow.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			_, err := loadConfig(configPath)
			if tt.wantErr && err == nil {
				t.Error("loadConfig() expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("loadConfig() unexpected error: %v", err)
			}
		})
	}
}

// TestConfigValidation_MultipleErrors tests that multiple validation errors are reported
func TestConfigValidation_MultipleErrors(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "", Type: "invalid-type"},
		Git:     GitConfig{IntegrationBranch: ""},
		Repos: []RepoConfig{
			{Name: "", Path: "", Type: "invalid"},
		},
	}

	errors := config.Validate()
	// Should have at least:
	// - project.name missing
	// - project.type invalid
	// - git.integration_branch missing
	// - repos[0].name missing
	// - repos[0].path missing
	// - repos[0].type invalid
	if len(errors) < 5 {
		t.Errorf("Validate() got %d errors, want at least 5", len(errors))
		for _, e := range errors {
			t.Logf("  error: %s", e.Error())
		}
	}
}

// =============================================================================
// Path Traversal Prevention Tests (migrated from test_config_validation_extended.py)
// Property 15: Path Traversal Prevention
// =============================================================================

func TestValidateRepoPath_RejectDoubleDotPath(t *testing.T) {
	// Test paths with .. are rejected (Req 22.1)
	isValid, err := ValidateRepoPath("directory", "../outside")

	if isValid {
		t.Fatal("ValidateRepoPath should reject path with ..")
	}
	if !strings.Contains(err, "Path traversal") {
		t.Errorf("Error should mention path traversal, got: %s", err)
	}
}

func TestValidateRepoPath_RejectEmbeddedDoubleDot(t *testing.T) {
	// Test paths with embedded .. are rejected (Req 22.2)
	isValid, err := ValidateRepoPath("directory", "backend/../../../etc")

	if isValid {
		t.Fatal("ValidateRepoPath should reject path with embedded ..")
	}
	if !strings.Contains(err, "Path traversal") {
		t.Errorf("Error should mention path traversal, got: %s", err)
	}
}

func TestValidateRepoPath_AcceptValidPath(t *testing.T) {
	// Test valid paths are accepted
	isValid, err := ValidateRepoPath("directory", "backend/internal")

	if !isValid {
		t.Fatalf("ValidateRepoPath should accept valid path, got error: %s", err)
	}
	if err != "" {
		t.Errorf("Error should be empty for valid path, got: %s", err)
	}
}

func TestValidateRepoPath_RootTypeRequiresDotPath(t *testing.T) {
	// Test root type requires ./ or . path (Req 9.3)
	isValid, err := ValidateRepoPath("root", "backend")

	if isValid {
		t.Fatal("ValidateRepoPath should reject non-dot path for root type")
	}
	if !strings.Contains(err, "Root type path must be") {
		t.Errorf("Error should mention root type path requirement, got: %s", err)
	}
}

func TestValidateRepoPath_RootTypeAcceptsDot(t *testing.T) {
	// Test root type accepts . path
	isValid, err := ValidateRepoPath("root", ".")

	if !isValid {
		t.Fatalf("ValidateRepoPath should accept '.' for root type, got error: %s", err)
	}
}

func TestValidateRepoPath_RootTypeAcceptsDotSlash(t *testing.T) {
	// Test root type accepts ./ path
	isValid, err := ValidateRepoPath("root", "./")

	if !isValid {
		t.Fatalf("ValidateRepoPath should accept './' for root type, got error: %s", err)
	}
}

// =============================================================================
// Submodule Config Validation Tests (migrated from test_config_validation_extended.py)
// Property 7: Config Validation Completeness
// =============================================================================

func TestValidateSubmoduleConfig_MissingGitmodules(t *testing.T) {
	// Test missing .gitmodules is detected (Req 1.5)
	isValid, errors := ValidateSubmoduleConfig("backend", "", true)

	if isValid {
		t.Fatal("ValidateSubmoduleConfig should reject missing .gitmodules")
	}

	hasMissingError := false
	for _, e := range errors {
		if strings.Contains(e, "Missing .gitmodules") {
			hasMissingError = true
			break
		}
	}
	if !hasMissingError {
		t.Errorf("Errors should mention missing .gitmodules, got: %v", errors)
	}
}

func TestValidateSubmoduleConfig_PathNotInGitmodules(t *testing.T) {
	// Test path not in .gitmodules is detected (Req 9.1)
	gitmodules := `[submodule "frontend"]
    path = frontend
    url = https://github.com/test/frontend.git
`
	isValid, errors := ValidateSubmoduleConfig("backend", gitmodules, true)

	if isValid {
		t.Fatal("ValidateSubmoduleConfig should reject path not in .gitmodules")
	}

	hasNotFoundError := false
	for _, e := range errors {
		if strings.Contains(e, "not found in .gitmodules") {
			hasNotFoundError = true
			break
		}
	}
	if !hasNotFoundError {
		t.Errorf("Errors should mention path not found, got: %v", errors)
	}
}

func TestValidateSubmoduleConfig_MissingGitDirectory(t *testing.T) {
	// Test missing .git is detected (Req 9.2)
	gitmodules := `[submodule "backend"]
    path = backend
    url = https://github.com/test/backend.git
`
	isValid, errors := ValidateSubmoduleConfig("backend", gitmodules, false)

	if isValid {
		t.Fatal("ValidateSubmoduleConfig should reject missing .git")
	}

	hasNoGitError := false
	for _, e := range errors {
		if strings.Contains(e, "no .git") {
			hasNoGitError = true
			break
		}
	}
	if !hasNoGitError {
		t.Errorf("Errors should mention no .git, got: %v", errors)
	}
}

func TestValidateSubmoduleConfig_ValidConfig(t *testing.T) {
	// Test valid submodule config passes
	gitmodules := `[submodule "backend"]
    path = backend
    url = https://github.com/test/backend.git
`
	isValid, errors := ValidateSubmoduleConfig("backend", gitmodules, true)

	if !isValid {
		t.Fatalf("ValidateSubmoduleConfig should accept valid config, got errors: %v", errors)
	}
	if len(errors) != 0 {
		t.Errorf("Errors should be empty for valid config, got: %v", errors)
	}
}

// =============================================================================
// Directory Git File Warning Tests
// =============================================================================

func TestCheckDirectoryHasGitFile_WithGitFile(t *testing.T) {
	// Test warning when directory has .git file (Req 9.4)
	hasWarning, warning := CheckDirectoryHasGitFile("backend", true)

	if !hasWarning {
		t.Fatal("CheckDirectoryHasGitFile should return warning when has .git file")
	}
	if !strings.Contains(warning, "WARNING") {
		t.Errorf("Warning should contain 'WARNING', got: %s", warning)
	}
	if !strings.Contains(warning, "might be a submodule") {
		t.Errorf("Warning should mention 'might be a submodule', got: %s", warning)
	}
}

func TestCheckDirectoryHasGitFile_WithoutGitFile(t *testing.T) {
	// Test no warning when directory has no .git file
	hasWarning, warning := CheckDirectoryHasGitFile("backend", false)

	if hasWarning {
		t.Fatal("CheckDirectoryHasGitFile should not return warning when no .git file")
	}
	if warning != "" {
		t.Errorf("Warning should be empty, got: %s", warning)
	}
}

// =============================================================================
// Submodule Remote Validation Tests (migrated from test_config_validation_extended.py)
// Property 18: Submodule Remote Validation
// =============================================================================

func TestValidateSubmoduleRemote_MatchingURLs(t *testing.T) {
	// Test matching URLs pass validation (Req 26.1)
	isValid, err := ValidateSubmoduleRemote(
		"backend",
		"https://github.com/test/backend.git",
		"https://github.com/test/backend.git",
	)

	if !isValid {
		t.Fatalf("ValidateSubmoduleRemote should accept matching URLs, got error: %s", err)
	}
	if err != "" {
		t.Errorf("Error should be empty for matching URLs, got: %s", err)
	}
}

func TestValidateSubmoduleRemote_MatchingURLsWithoutGitSuffix(t *testing.T) {
	// Test URLs match even without .git suffix
	isValid, err := ValidateSubmoduleRemote(
		"backend",
		"https://github.com/test/backend.git",
		"https://github.com/test/backend",
	)

	if !isValid {
		t.Fatalf("ValidateSubmoduleRemote should accept URLs with/without .git suffix, got error: %s", err)
	}
}

func TestValidateSubmoduleRemote_MismatchedURLs(t *testing.T) {
	// Test mismatched URLs fail validation (Req 26.2)
	isValid, err := ValidateSubmoduleRemote(
		"backend",
		"https://github.com/test/backend.git",
		"https://github.com/other/backend.git",
	)

	if isValid {
		t.Fatal("ValidateSubmoduleRemote should reject mismatched URLs")
	}
	if !strings.Contains(err, "mismatch") {
		t.Errorf("Error should mention mismatch, got: %s", err)
	}
}

func TestValidateSubmoduleRemote_MissingGitmodulesURL(t *testing.T) {
	// Test missing .gitmodules URL fails (Req 26.3)
	isValid, err := ValidateSubmoduleRemote(
		"backend",
		"",
		"https://github.com/test/backend.git",
	)

	if isValid {
		t.Fatal("ValidateSubmoduleRemote should reject missing .gitmodules URL")
	}
	if !strings.Contains(err, "No URL found in .gitmodules") {
		t.Errorf("Error should mention no URL in .gitmodules, got: %s", err)
	}
}

func TestValidateSubmoduleRemote_MissingActualRemote(t *testing.T) {
	// Test missing actual remote fails (Req 26.4)
	isValid, err := ValidateSubmoduleRemote(
		"backend",
		"https://github.com/test/backend.git",
		"",
	)

	if isValid {
		t.Fatal("ValidateSubmoduleRemote should reject missing actual remote")
	}
	if !strings.Contains(err, "No remote URL configured") {
		t.Errorf("Error should mention no remote URL configured, got: %s", err)
	}
}

// =============================================================================
// Custom Agents Validation Tests
// =============================================================================

func TestConfigValidate_CustomAgents_Valid(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Builtin: []string{"pr-reviewer", "conflict-resolver"},
			Custom: []CustomAgentDef{
				{Name: "code-analyzer", Description: "Analyzes code quality"},
				{Name: "doc-generator", Description: "Generates docs", Model: "sonnet", Trigger: "check_result"},
			},
		},
	}

	errors := config.Validate()
	for _, e := range errors {
		if strings.Contains(e.Field, "agents.custom") {
			t.Errorf("unexpected agent validation error: %s", e.Error())
		}
	}
}

func TestConfigValidate_CustomAgents_Duplicate(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Builtin: []string{"pr-reviewer"},
			Custom: []CustomAgentDef{
				{Name: "my-agent", Description: "First"},
				{Name: "my-agent", Description: "Duplicate"},
			},
		},
	}

	errors := config.Validate()
	hasDuplicate := false
	for _, e := range errors {
		if strings.Contains(e.Message, "duplicate") {
			hasDuplicate = true
		}
	}
	if !hasDuplicate {
		t.Error("expected duplicate agent name error")
	}
}

func TestConfigValidate_CustomAgents_BuiltinCollision(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Builtin: []string{"pr-reviewer", "conflict-resolver"},
			Custom: []CustomAgentDef{
				{Name: "pr-reviewer", Description: "Collides with built-in"},
			},
		},
	}

	errors := config.Validate()
	hasCollision := false
	for _, e := range errors {
		if strings.Contains(e.Message, "collides with built-in") {
			hasCollision = true
		}
	}
	if !hasCollision {
		t.Error("expected built-in collision error")
	}
}

func TestConfigValidate_CustomAgents_InvalidModel(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Custom: []CustomAgentDef{
				{Name: "my-agent", Description: "Test", Model: "gpt4"},
			},
		},
	}

	errors := config.Validate()
	hasModelErr := false
	for _, e := range errors {
		if strings.Contains(e.Message, "invalid model") {
			hasModelErr = true
		}
	}
	if !hasModelErr {
		t.Error("expected invalid model error")
	}
}

func TestConfigValidate_CustomAgents_InvalidTrigger(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Custom: []CustomAgentDef{
				{Name: "my-agent", Description: "Test", Trigger: "on_commit"},
			},
		},
	}

	errors := config.Validate()
	hasTriggerErr := false
	for _, e := range errors {
		if strings.Contains(e.Message, "invalid trigger") {
			hasTriggerErr = true
		}
	}
	if !hasTriggerErr {
		t.Error("expected invalid trigger error")
	}
}

func TestConfigValidate_CustomAgents_InvalidName(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Custom: []CustomAgentDef{
				{Name: "My Agent", Description: "Invalid name"},
			},
		},
	}

	errors := config.Validate()
	hasNameErr := false
	for _, e := range errors {
		if strings.Contains(e.Message, "invalid name") {
			hasNameErr = true
		}
	}
	if !hasNameErr {
		t.Error("expected invalid name error")
	}
}

func TestConfigValidate_CustomAgents_MissingDescription(t *testing.T) {
	config := Config{
		Project: ProjectConfig{Name: "test", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Agents: AgentsConfig{
			Custom: []CustomAgentDef{
				{Name: "my-agent", Description: ""},
			},
		},
	}

	errors := config.Validate()
	hasDescErr := false
	for _, e := range errors {
		if strings.Contains(e.Field, "description") {
			hasDescErr = true
		}
	}
	if !hasDescErr {
		t.Error("expected missing description error")
	}
}

// =============================================================================
// InstallAgentsDir Tests
// =============================================================================

func TestInstallAgentsDir_WithCustomAgents(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	agents := AgentsConfig{
		Builtin: []string{"pr-reviewer", "conflict-resolver"},
		Custom: []CustomAgentDef{
			{Name: "code-analyzer", Description: "Analyzes code quality", Model: "sonnet"},
		},
	}

	if err := installAgentsDir(agentsDir, agents); err != nil {
		t.Fatalf("installAgentsDir() error = %v", err)
	}

	// Check built-in agents exist
	for _, name := range []string{"pr-reviewer.md", "conflict-resolver.md"} {
		path := filepath.Join(agentsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("built-in agent not created: %s", name)
		}
	}

	// Check custom agent exists
	customPath := filepath.Join(agentsDir, "code-analyzer.md")
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Error("custom agent not created: code-analyzer.md")
	}

	// Check custom agent content
	data, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("failed to read custom agent: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "name: code-analyzer") {
		t.Error("custom agent should contain name in frontmatter")
	}
	if !strings.Contains(content, "model: sonnet") {
		t.Error("custom agent should contain model in frontmatter")
	}
}

func TestInstallAgentsDir_CleanStaleAgents(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Create a stale agent file
	stalePath := filepath.Join(agentsDir, "old-agent.md")
	if err := os.WriteFile(stalePath, []byte("stale"), 0644); err != nil {
		t.Fatalf("write stale file error: %v", err)
	}

	agents := AgentsConfig{
		Builtin: []string{"pr-reviewer", "conflict-resolver"},
		Custom:  []CustomAgentDef{},
	}

	if err := installAgentsDir(agentsDir, agents); err != nil {
		t.Fatalf("installAgentsDir() error = %v", err)
	}

	// Stale file should be removed
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Error("stale agent file should have been removed")
	}

	// Built-in should still exist
	if _, err := os.Stat(filepath.Join(agentsDir, "pr-reviewer.md")); os.IsNotExist(err) {
		t.Error("built-in agent should still exist after cleanup")
	}
}

// =============================================================================
// ValidateCustomRules Tests
// =============================================================================

func TestValidateCustomRules_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	rules := RulesConfig{
		Custom: []string{"nonexistent-rule"},
	}

	warnings := validateCustomRules(tmpDir, rules)
	if len(warnings) == 0 {
		t.Error("expected warning for missing rule file")
	}
	hasNotFound := false
	for _, w := range warnings {
		if strings.Contains(w, "not found") {
			hasNotFound = true
		}
	}
	if !hasNotFound {
		t.Errorf("expected 'not found' warning, got: %v", warnings)
	}
}

func TestValidateCustomRules_MissingRoleGoal(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".ai", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	// Create rule file missing Role: and Goal:
	rulePath := filepath.Join(rulesDir, "bad-rule.md")
	if err := os.WriteFile(rulePath, []byte("# Bad Rule\n\nNo role or goal here.\n"), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	rules := RulesConfig{
		Custom: []string{"bad-rule"},
	}

	warnings := validateCustomRules(tmpDir, rules)
	if len(warnings) < 2 {
		t.Errorf("expected at least 2 warnings (missing Role + Goal), got %d: %v", len(warnings), warnings)
	}
}

func TestValidateCustomRules_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".ai", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	rulePath := filepath.Join(rulesDir, "good-rule.md")
	content := "# Good Rule\n\nRole: Senior Engineer\nGoal: Write good code\n"
	if err := os.WriteFile(rulePath, []byte(content), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	rules := RulesConfig{
		Custom: []string{"good-rule"},
	}

	warnings := validateCustomRules(tmpDir, rules)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid rule, got: %v", warnings)
	}
}

// =============================================================================
// LoadConfig Agents Default Tests
// =============================================================================

func TestLoadConfig_AgentsDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Config without agents section
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
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if len(config.Agents.Builtin) != 2 {
		t.Errorf("expected 2 default builtin agents, got %d", len(config.Agents.Builtin))
	}
	if config.Agents.Builtin[0] != "pr-reviewer" {
		t.Errorf("expected first builtin agent to be pr-reviewer, got %s", config.Agents.Builtin[0])
	}
}

// =============================================================================
// Path Validation by Repo Type (Table-Driven Tests)
// =============================================================================

func TestValidateRepoPath_AllRepoTypes(t *testing.T) {
	tests := []struct {
		name        string
		repoType    string
		repoPath    string
		expectValid bool
	}{
		{"root with ./", "root", "./", true},
		{"root with .", "root", ".", true},
		{"root with backend", "root", "backend", false},
		{"directory with backend", "directory", "backend", true},
		{"directory with nested path", "directory", "libs/shared", true},
		{"directory with path traversal", "directory", "../outside", false},
		{"submodule with backend", "submodule", "backend", true},
		{"submodule with nested path", "submodule", "libs/shared", true},
		{"submodule with path traversal", "submodule", "../outside", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid, _ := ValidateRepoPath(tt.repoType, tt.repoPath)
			if isValid != tt.expectValid {
				t.Errorf("ValidateRepoPath(%q, %q) = %v, want %v",
					tt.repoType, tt.repoPath, isValid, tt.expectValid)
			}
		})
	}
}
