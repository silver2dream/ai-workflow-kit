package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// createMinimalMockFS creates a mock FS with minimal required files
func createMinimalMockFS() fstest.MapFS {
	return fstest.MapFS{
		".ai/config/workflow.schema.json": &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":         &fstest.MapFile{Data: []byte("#!/bin/bash\necho test")},
		".ai/rules/.gitkeep":              &fstest.MapFile{Data: []byte("")},
		".ai/commands/.gitkeep":           &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":          &fstest.MapFile{Data: []byte("")},
	}
}

func TestInstall_CurrentDirectory(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	// Test install with absolute path
	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Verify .ai directory was created
	aiDir := filepath.Join(tmpDir, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		t.Error(".ai directory was not created")
	}

	// Verify config directory was created
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error(".ai/config directory was not created")
	}
}

func TestInstall_DotPath(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	mockFS := createMinimalMockFS()

	// Test install with "."
	result, err := Install(mockFS, ".", Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install with '.' failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Verify .ai directory was created
	aiDir := filepath.Join(tmpDir, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		t.Error(".ai directory was not created")
	}
}

func TestInstall_InvalidPreset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "invalid-preset",
		NoGenerate: true,
	})
	if err == nil {
		t.Error("expected error for invalid preset, got nil")
	}
}

func TestInstall_NonExistentDirectory(t *testing.T) {
	mockFS := createMinimalMockFS()

	_, err := Install(mockFS, "/nonexistent/path/that/does/not/exist", Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

func TestInstall_ConfigSkipped(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml
	configDir := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("existing: true"), 0o644)

	mockFS := createMinimalMockFS()

	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}
	if !result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be true")
	}

	// Verify original content preserved
	content, _ := os.ReadFile(filepath.Join(configDir, "workflow.yaml"))
	if string(content) != "existing: true" {
		t.Error("existing workflow.yaml was overwritten")
	}
}

func TestInstall_ForceConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml
	configDir := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("existing: true"), 0o644)

	mockFS := createMinimalMockFS()

	result, err := Install(mockFS, tmpDir, Options{
		Preset:      "generic",
		ForceConfig: true,
		NoGenerate:  true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}
	if result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be false with --force-config")
	}

	// Verify content was overwritten
	content, _ := os.ReadFile(filepath.Join(configDir, "workflow.yaml"))
	if string(content) == "existing: true" {
		t.Error("workflow.yaml was not overwritten with --force-config")
	}
}

func TestInstall_PresetConfigNotOverwrittenByCopyDir(t *testing.T) {
	// This test ensures that when using a preset (e.g., godot-go),
	// the preset's workflow.yaml is NOT overwritten by copyDir
	// which copies the embedded default workflow.yaml.
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock FS that includes a default workflow.yaml
	// (simulating the embedded kit files with a different config)
	mockFS := fstest.MapFS{
		".ai/config/workflow.yaml":                &fstest.MapFile{Data: []byte("language: unity\n")},
		".ai/config/workflow.schema.json":         &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":                 &fstest.MapFile{Data: []byte("#!/bin/bash\necho test")},
		".ai/rules/.gitkeep":                      &fstest.MapFile{Data: []byte("")},
		".ai/rules/_examples/testing-strategy.md": &fstest.MapFile{Data: []byte("# Testing rules")},
		".ai/rules/_examples/api-design.md":       &fstest.MapFile{Data: []byte("# API rules")},
		".ai/commands/.gitkeep":                   &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":                  &fstest.MapFile{Data: []byte("")},
	}

	// Install with the godot-go preset and force mode
	result, err := Install(mockFS, tmpDir, Options{
		Preset:     PresetGodotGo,
		Force:      true,
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Read the resulting workflow.yaml
	content, err := os.ReadFile(filepath.Join(tmpDir, ".ai", "config", "workflow.yaml"))
	if err != nil {
		t.Fatalf("failed to read workflow.yaml: %v", err)
	}

	// It should hold the preset's languages, NOT "unity" (from the embedded default)
	if strings.Contains(string(content), "language: unity") {
		t.Error("workflow.yaml was overwritten by copyDir - should preserve preset config")
	}
	if !strings.Contains(string(content), "language: csharp") || !strings.Contains(string(content), "language: go") {
		t.Error("workflow.yaml should contain the godot-go preset languages (go and csharp)")
	}
}

func TestInstall_Upgrade(t *testing.T) {
	// Test upgrade scenario: existing config should be preserved
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml with custom content
	configDir := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(configDir, 0o755)
	customConfig := "project:\n  name: my-custom-project\n  type: monorepo\n"
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(customConfig), 0o644)

	// Create existing scripts directory with old content
	scriptsDir := filepath.Join(tmpDir, ".ai", "scripts")
	os.MkdirAll(scriptsDir, 0o755)
	os.WriteFile(filepath.Join(scriptsDir, "old-script.sh"), []byte("old content"), 0o644)

	mockFS := fstest.MapFS{
		".ai/config/workflow.schema.json": &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":         &fstest.MapFile{Data: []byte("#!/bin/bash\necho new")},
		".ai/scripts/kickoff.sh":          &fstest.MapFile{Data: []byte("#!/bin/bash\necho kickoff")},
		".ai/rules/.gitkeep":              &fstest.MapFile{Data: []byte("")},
		".ai/commands/.gitkeep":           &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":          &fstest.MapFile{Data: []byte("")},
	}

	// Simulate upgrade: Force=true with SkipConfig=true
	result, err := Install(mockFS, tmpDir, Options{
		Force:      true,
		SkipConfig: true,
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install (upgrade) failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Config should be skipped (preserved)
	if !result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be true during upgrade")
	}

	// Verify original config preserved
	content, _ := os.ReadFile(filepath.Join(configDir, "workflow.yaml"))
	if string(content) != customConfig {
		t.Error("workflow.yaml was overwritten during upgrade")
	}

	// Verify new scripts were added
	if _, err := os.Stat(filepath.Join(scriptsDir, "kickoff.sh")); os.IsNotExist(err) {
		t.Error("new script kickoff.sh was not added during upgrade")
	}
}

func TestInstall_GitIgnoreContainsClaudeSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify .gitignore contains claude settings entry
	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	if !strings.Contains(string(content), ".claude/settings.local.json") {
		t.Error(".gitignore should contain .claude/settings.local.json")
	}
}

func TestInstall_GitIgnoreContainsCacheFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	// Verify common cache files are ignored to prevent audit P1 findings
	expectedEntries := []string{
		"__pycache__/",
		"*.pyc",
		"node_modules/",
		".pytest_cache/",
	}
	for _, entry := range expectedEntries {
		if !strings.Contains(string(content), entry) {
			t.Errorf(".gitignore should contain %s", entry)
		}
	}
}

func TestInstall_ClaudeDirectoryCreated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify .claude directory was created
	claudeDir := filepath.Join(tmpDir, ".claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		t.Error(".claude directory was not created")
	}
}

func TestInstall_GitIgnoreUpdatedOnUpgrade(t *testing.T) {
	// Test that upgrade replaces old AWK gitignore section with new entries
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing .gitignore with OLD AWK section (missing __pycache__)
	oldGitignore := `# User entries
*.bak

# >>> AI Workflow Kit >>>
# Runtime state (do not commit)
.ai/state/
.ai/results/
.ai/runs/
.ai/exe-logs/
.worktrees/
# <<< AI Workflow Kit <<<

# More user entries
*.tmp
`
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(oldGitignore), 0o644)

	mockFS := createMinimalMockFS()

	// Simulate upgrade
	_, err = Install(mockFS, tmpDir, Options{
		Force:      true,
		SkipConfig: true,
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install (upgrade) failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	// Should now contain new entries
	if !strings.Contains(string(content), "__pycache__/") {
		t.Error(".gitignore should contain __pycache__/ after upgrade")
	}
	if !strings.Contains(string(content), ".ai/temp/") {
		t.Error(".gitignore should contain .ai/temp/ after upgrade")
	}
	if !strings.Contains(string(content), ".ai/logs/") {
		t.Error(".gitignore should contain .ai/logs/ after upgrade")
	}

	// Should preserve user entries
	if !strings.Contains(string(content), "*.bak") {
		t.Error(".gitignore should preserve user entries (*.bak)")
	}
	if !strings.Contains(string(content), "*.tmp") {
		t.Error(".gitignore should preserve user entries (*.tmp)")
	}

	// Should only have ONE AWK section (not duplicated)
	count := strings.Count(string(content), "# >>> AI Workflow Kit >>>")
	if count != 1 {
		t.Errorf(".gitignore should have exactly 1 AWK section, got %d", count)
	}
}

// =============================================================================
// Task 21: Preset Tests
// =============================================================================

// =============================================================================
// Task 22: Scaffold Tests
// =============================================================================

// TestValidateProjectName verifies project name validation
func TestValidateProjectName(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{"ValidSimple", "my-project", false},
		{"ValidWithNumbers", "project123", false},
		{"ValidWithUnderscore", "my_project", false},
		{"ValidWithDot", "my.project", false},
		{"InvalidSlash", "my/project", true},
		{"InvalidBackslash", "my\\project", true},
		{"InvalidDollar", "my$project", true},
		{"InvalidBacktick", "my`project", true},
		{"InvalidDoubleQuote", `my"project`, true},
		{"InvalidSingleQuote", "my'project", true},
		{"InvalidSemicolon", "my;project", true},
		{"InvalidAmpersand", "my&project", true},
		{"InvalidPipe", "my|project", true},
		{"InvalidGreaterThan", "my>project", true},
		{"InvalidLessThan", "my<project", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProjectName(tc.input)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tc.input, err)
			}
		})
	}
}

// TestCIWorkflowMigration verifies that upgrade removes deprecated awk job
func TestCIWorkflowMigration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-ci-migration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create old CI file with deprecated awk job
	ciDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(ciDir, 0755); err != nil {
		t.Fatalf("failed to create .github/workflows dir: %v", err)
	}

	oldCI := `name: ci

on:
  push:
    branches: ["feat/example"]
  pull_request:
    branches: ["feat/example", "main"]

jobs:
  awk:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Run AWK tests
        run: bash .ai/scripts/evaluate.sh

  backend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Go test
        working-directory: backend
        run: go test ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Build
        working-directory: frontend
        run: npm run build
`
	ciPath := filepath.Join(ciDir, "ci.yml")
	if err := os.WriteFile(ciPath, []byte(oldCI), 0644); err != nil {
		t.Fatalf("failed to write old CI file: %v", err)
	}

	// Test migration function directly
	migrated, newContent := migrateCIWorkflow([]byte(oldCI))
	if !migrated {
		t.Error("migrateCIWorkflow should return migrated=true for CI with awk job")
	}

	newContentStr := string(newContent)

	// Verify awk job is removed
	if strings.Contains(newContentStr, "awk:") {
		t.Error("Migrated CI should NOT contain 'awk:' job")
	}
	if strings.Contains(newContentStr, "evaluate.sh") {
		t.Error("Migrated CI should NOT contain 'evaluate.sh'")
	}

	// Verify other jobs are preserved
	if !strings.Contains(newContentStr, "backend:") {
		t.Error("Migrated CI should preserve 'backend:' job")
	}
	if !strings.Contains(newContentStr, "frontend:") {
		t.Error("Migrated CI should preserve 'frontend:' job")
	}
	if !strings.Contains(newContentStr, "go test") {
		t.Error("Migrated CI should preserve backend test command")
	}
	if !strings.Contains(newContentStr, "npm run build") {
		t.Error("Migrated CI should preserve frontend build command")
	}
}

// TestCIWorkflowNoMigrationNeeded verifies that CI without awk job is not modified
func TestCIWorkflowNoMigrationNeeded(t *testing.T) {
	cleanCI := `name: ci

on: [push]

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - run: npm test
`
	migrated, newContent := migrateCIWorkflow([]byte(cleanCI))
	if migrated {
		t.Error("migrateCIWorkflow should return migrated=false for CI without awk job")
	}
	if string(newContent) != cleanCI {
		t.Error("Content should be unchanged when no migration needed")
	}
}
