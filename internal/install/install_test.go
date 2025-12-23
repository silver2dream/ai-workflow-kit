package install

import (
	"errors"
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
		WithCI:     false,
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
		WithCI:     false,
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
		WithCI:     false,
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
		WithCI:      false,
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
	// This test ensures that when using a preset (e.g., react-go),
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
		".ai/config/workflow.yaml":              &fstest.MapFile{Data: []byte("language: unity\n")},
		".ai/config/workflow.schema.json":       &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":               &fstest.MapFile{Data: []byte("#!/bin/bash\necho test")},
		".ai/rules/.gitkeep":                    &fstest.MapFile{Data: []byte("")},
		".ai/rules/_examples/backend-go.md":     &fstest.MapFile{Data: []byte("# Go rules")},
		".ai/rules/_examples/frontend-react.md": &fstest.MapFile{Data: []byte("# React rules")},
		".ai/commands/.gitkeep":                 &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":                &fstest.MapFile{Data: []byte("")},
	}

	// Install with react-go preset and force mode
	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "react-go",
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

	// It should contain "typescript" (from react-go preset), NOT "unity" (from embedded default)
	if strings.Contains(string(content), "language: unity") {
		t.Error("workflow.yaml was overwritten by copyDir - should preserve preset config")
	}
	if !strings.Contains(string(content), "language: typescript") && !strings.Contains(string(content), "language: go") {
		t.Error("workflow.yaml should contain react-go preset languages (typescript/go)")
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
		WithCI:     false,
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
		WithCI:     false,
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
		WithCI:     false,
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
		WithCI:     false,
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
		WithCI:     false,
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

// TestPresetWorkflowYAML uses table-driven tests to verify all presets generate valid workflow.yaml
func TestPresetWorkflowYAML(t *testing.T) {
	testCases := []struct {
		name           string
		preset         string
		category       string
		expectLanguage []string // languages expected in workflow.yaml
	}{
		{"Go", "go", "single-repo", []string{"go"}},
		{"Python", "python", "single-repo", []string{"python"}},
		{"Rust", "rust", "single-repo", []string{"rust"}},
		{"Dotnet", "dotnet", "single-repo", []string{"dotnet"}},
		{"Node", "node", "single-repo", []string{"typescript"}},
		{"Generic", "generic", "single-repo", []string{"typescript"}},
		{"ReactGo", "react-go", "monorepo", []string{"typescript", "go"}},
		{"ReactPython", "react-python", "monorepo", []string{"typescript", "python"}},
		{"UnityGo", "unity-go", "monorepo", []string{"unity", "go"}},
		{"GodotGo", "godot-go", "monorepo", []string{"gdscript", "go"}},
		{"UnrealGo", "unreal-go", "monorepo", []string{"unreal", "go"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "awkit-preset-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			mockFS := createMinimalMockFS()

			_, err = Install(mockFS, tmpDir, Options{
				Preset:     tc.preset,
				NoGenerate: true,
			})
			if err != nil {
				t.Fatalf("Install with preset %q failed: %v", tc.preset, err)
			}

			// Verify workflow.yaml was created
			configPath := filepath.Join(tmpDir, ".ai", "config", "workflow.yaml")
			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("failed to read workflow.yaml: %v", err)
			}

			// Verify expected languages are present
			for _, lang := range tc.expectLanguage {
				if !strings.Contains(string(content), lang) {
					t.Errorf("workflow.yaml should contain language %q for preset %q", lang, tc.preset)
				}
			}
		})
	}
}

// TestPresetGenericEqualsNode verifies generic preset produces same output as node
func TestPresetGenericEqualsNode(t *testing.T) {
	// Create two temp directories
	genericDir, err := os.MkdirTemp("", "awkit-generic-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(genericDir)

	nodeDir, err := os.MkdirTemp("", "awkit-node-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(nodeDir)

	mockFS := createMinimalMockFS()

	// Use same project name for both to ensure identical output
	projectName := "test-project"

	// Install with generic preset
	_, err = Install(mockFS, genericDir, Options{
		Preset:      "generic",
		ProjectName: projectName,
		NoGenerate:  true,
	})
	if err != nil {
		t.Fatalf("Install with generic preset failed: %v", err)
	}

	// Install with node preset
	_, err = Install(mockFS, nodeDir, Options{
		Preset:      "node",
		ProjectName: projectName,
		NoGenerate:  true,
	})
	if err != nil {
		t.Fatalf("Install with node preset failed: %v", err)
	}

	// Compare workflow.yaml content
	genericConfig, _ := os.ReadFile(filepath.Join(genericDir, ".ai", "config", "workflow.yaml"))
	nodeConfig, _ := os.ReadFile(filepath.Join(nodeDir, ".ai", "config", "workflow.yaml"))

	if string(genericConfig) != string(nodeConfig) {
		t.Error("generic and node presets should produce identical workflow.yaml")
	}
}

// TestBackwardCompatibilityGeneric verifies generic preset behavior is unchanged
func TestBackwardCompatibilityGeneric(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-compat-generic-*")
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
		t.Fatalf("Install with generic preset failed: %v", err)
	}

	// Verify expected structure
	configPath := filepath.Join(tmpDir, ".ai", "config", "workflow.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read workflow.yaml: %v", err)
	}

	// Generic should be single-repo with typescript
	if !strings.Contains(string(content), "type: root") {
		t.Error("generic preset should have type: root")
	}
	if !strings.Contains(string(content), "language: typescript") {
		t.Error("generic preset should have language: typescript")
	}
}

// TestBackwardCompatibilityReactGo verifies react-go preset behavior is unchanged
func TestBackwardCompatibilityReactGo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-compat-reactgo-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := fstest.MapFS{
		".ai/config/workflow.schema.json":       &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":               &fstest.MapFile{Data: []byte("#!/bin/bash\necho test")},
		".ai/rules/.gitkeep":                    &fstest.MapFile{Data: []byte("")},
		".ai/rules/_examples/backend-go.md":     &fstest.MapFile{Data: []byte("# Go rules")},
		".ai/rules/_examples/frontend-react.md": &fstest.MapFile{Data: []byte("# React rules")},
		".ai/commands/.gitkeep":                 &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":                &fstest.MapFile{Data: []byte("")},
	}

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "react-go",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install with react-go preset failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".ai", "config", "workflow.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read workflow.yaml: %v", err)
	}

	// react-go should be monorepo with frontend and backend
	if !strings.Contains(string(content), "frontend") {
		t.Error("react-go preset should have frontend repo")
	}
	if !strings.Contains(string(content), "backend") {
		t.Error("react-go preset should have backend repo")
	}
	if !strings.Contains(string(content), "language: typescript") {
		t.Error("react-go preset should have typescript for frontend")
	}
	if !strings.Contains(string(content), "language: go") {
		t.Error("react-go preset should have go for backend")
	}
}

// =============================================================================
// Task 22: Scaffold Tests
// =============================================================================

// TestScaffoldSingleRepo uses table-driven tests for single-repo scaffold
func TestScaffoldSingleRepo(t *testing.T) {
	testCases := []struct {
		name          string
		preset        string
		expectedFiles []string
	}{
		{
			name:   "Go",
			preset: "go",
			expectedFiles: []string{
				"go.mod",
				"main.go",
				"README.md",
			},
		},
		{
			name:   "Python",
			preset: "python",
			expectedFiles: []string{
				"pyproject.toml",
				filepath.Join("src", "__init__.py"),
				filepath.Join("src", "main.py"),
				filepath.Join("tests", "__init__.py"),
				filepath.Join("tests", "test_placeholder.py"),
				"README.md",
			},
		},
		{
			name:   "Rust",
			preset: "rust",
			expectedFiles: []string{
				"Cargo.toml",
				filepath.Join("src", "main.rs"),
				"README.md",
			},
		},
		{
			name:   "Dotnet",
			preset: "dotnet",
			expectedFiles: []string{
				"Program.cs",
				"README.md",
				// .csproj uses project name, checked separately
			},
		},
		{
			name:   "Node",
			preset: "node",
			expectedFiles: []string{
				"package.json",
				"tsconfig.json",
				filepath.Join("src", "index.ts"),
				"README.md",
			},
		},
		{
			name:   "Generic",
			preset: "generic",
			expectedFiles: []string{
				"package.json",
				"tsconfig.json",
				filepath.Join("src", "index.ts"),
				"README.md",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "awkit-scaffold-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			result, err := Scaffold(tmpDir, ScaffoldOptions{
				Preset:      tc.preset,
				TargetDir:   tmpDir,
				ProjectName: "test-project",
				Force:       false,
				DryRun:      false,
			})
			if err != nil {
				t.Fatalf("Scaffold failed: %v", err)
			}

			// Verify expected files were created
			for _, expectedFile := range tc.expectedFiles {
				fullPath := filepath.Join(tmpDir, expectedFile)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Errorf("expected file %q was not created", expectedFile)
				}
			}

			// Verify result.Created contains expected files
			if len(result.Created) == 0 {
				t.Error("result.Created should not be empty")
			}
		})
	}
}

// TestScaffoldMonorepo uses table-driven tests for monorepo scaffold
func TestScaffoldMonorepo(t *testing.T) {
	testCases := []struct {
		name          string
		preset        string
		expectedFiles []string
	}{
		{
			name:   "ReactGo",
			preset: "react-go",
			expectedFiles: []string{
				filepath.Join("backend", "go.mod"),
				filepath.Join("backend", "main.go"),
				filepath.Join("frontend", "package.json"),
				filepath.Join("frontend", "tsconfig.json"),
				filepath.Join("frontend", "vite.config.ts"),
				filepath.Join("frontend", "index.html"),
				filepath.Join("frontend", "src", "main.tsx"),
				filepath.Join("frontend", "src", "App.tsx"),
			},
		},
		{
			name:   "ReactPython",
			preset: "react-python",
			expectedFiles: []string{
				filepath.Join("backend", "pyproject.toml"),
				filepath.Join("backend", "src", "main.py"),
				filepath.Join("frontend", "package.json"),
				filepath.Join("frontend", "vite.config.ts"),
			},
		},
		{
			name:   "UnityGo",
			preset: "unity-go",
			expectedFiles: []string{
				filepath.Join("backend", "go.mod"),
				filepath.Join("backend", "main.go"),
				filepath.Join("frontend", ".gitkeep"),
				filepath.Join("frontend", "README.md"),
			},
		},
		{
			name:   "GodotGo",
			preset: "godot-go",
			expectedFiles: []string{
				filepath.Join("backend", "go.mod"),
				filepath.Join("backend", "main.go"),
				filepath.Join("frontend", "project.godot"),
				filepath.Join("frontend", "README.md"),
			},
		},
		{
			name:   "UnrealGo",
			preset: "unreal-go",
			expectedFiles: []string{
				filepath.Join("backend", "go.mod"),
				filepath.Join("backend", "main.go"),
				filepath.Join("frontend", ".gitkeep"),
				filepath.Join("frontend", "README.md"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "awkit-scaffold-monorepo-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			result, err := Scaffold(tmpDir, ScaffoldOptions{
				Preset:      tc.preset,
				TargetDir:   tmpDir,
				ProjectName: "test-project",
				Force:       false,
				DryRun:      false,
			})
			if err != nil {
				t.Fatalf("Scaffold failed: %v", err)
			}

			// Verify expected files were created
			for _, expectedFile := range tc.expectedFiles {
				fullPath := filepath.Join(tmpDir, expectedFile)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Errorf("expected file %q was not created", expectedFile)
				}
			}

			if len(result.Created) == 0 {
				t.Error("result.Created should not be empty")
			}
		})
	}
}

// TestScaffoldNoOverwrite verifies scaffold doesn't overwrite existing files
func TestScaffoldNoOverwrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-nooverwrite-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing file
	existingContent := "existing content"
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(existingContent), 0o644)

	result, err := Scaffold(tmpDir, ScaffoldOptions{
		Preset:      "go",
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	// Verify existing file was not overwritten
	content, _ := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	if string(content) != existingContent {
		t.Error("existing README.md should not be overwritten")
	}

	// Verify README.md is in Skipped list
	found := false
	for _, skipped := range result.Skipped {
		if strings.HasSuffix(skipped, "README.md") {
			found = true
			break
		}
	}
	if !found {
		t.Error("README.md should be in Skipped list")
	}
}

// TestScaffoldForceOverwrite verifies --force overwrites existing files
func TestScaffoldForceOverwrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-force-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing file
	existingContent := "existing content"
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(existingContent), 0o644)

	_, err = Scaffold(tmpDir, ScaffoldOptions{
		Preset:      "go",
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	// Verify existing file was overwritten
	content, _ := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	if string(content) == existingContent {
		t.Error("README.md should be overwritten with --force")
	}
}

// TestScaffoldDryRun verifies dry-run doesn't create files
func TestScaffoldDryRun(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-dryrun-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	result, err := Scaffold(tmpDir, ScaffoldOptions{
		Preset:      "go",
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Scaffold dry-run failed: %v", err)
	}

	// Verify no files were created
	if _, err := os.Stat(filepath.Join(tmpDir, "go.mod")); !os.IsNotExist(err) {
		t.Error("go.mod should not be created in dry-run mode")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "main.go")); !os.IsNotExist(err) {
		t.Error("main.go should not be created in dry-run mode")
	}

	// Verify result.Created contains expected files (what would be created)
	if len(result.Created) == 0 {
		t.Error("result.Created should contain files that would be created")
	}
}

// TestScaffoldDryRunWithExistingFiles verifies dry-run reports skipped files
func TestScaffoldDryRunWithExistingFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-dryrun-existing-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing file
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("existing"), 0o644)

	result, err := Scaffold(tmpDir, ScaffoldOptions{
		Preset:      "go",
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Scaffold dry-run failed: %v", err)
	}

	// Verify README.md is in Skipped list
	found := false
	for _, skipped := range result.Skipped {
		if strings.HasSuffix(skipped, "README.md") {
			found = true
			break
		}
	}
	if !found {
		t.Error("README.md should be in Skipped list for dry-run")
	}
}

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

// TestScaffoldDefaultPreset verifies empty preset uses generic (node)
func TestScaffoldDefaultPreset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-default-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = Scaffold(tmpDir, ScaffoldOptions{
		Preset:      "", // empty preset
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	// Verify node/generic files were created
	if _, err := os.Stat(filepath.Join(tmpDir, "package.json")); os.IsNotExist(err) {
		t.Error("package.json should be created for default preset")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "tsconfig.json")); os.IsNotExist(err) {
		t.Error("tsconfig.json should be created for default preset")
	}
}

// TestScaffoldFailureDoesNotAffectKit verifies scaffold failure doesn't affect AWK kit
func TestScaffoldFailureDoesNotAffectKit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-failure-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	// Install with scaffold and invalid project name
	result, err := Install(mockFS, tmpDir, Options{
		Preset:      "go",
		ProjectName: "invalid/name", // invalid project name
		NoGenerate:  true,
		Scaffold:    true,
	})

	// AWK kit should still be installed even if scaffold fails
	if err != nil {
		t.Fatalf("Install should not fail even if scaffold fails: %v", err)
	}

	// Verify AWK kit was installed
	if _, err := os.Stat(filepath.Join(tmpDir, ".ai", "config")); os.IsNotExist(err) {
		t.Error(".ai/config should exist even if scaffold fails")
	}

	// Verify scaffold error was recorded
	if result.ScaffoldError == nil {
		t.Error("ScaffoldError should be set when scaffold fails")
	}
}

// TestScaffoldUnknownPreset verifies unknown preset returns error
func TestScaffoldUnknownPreset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-scaffold-unknown-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = Scaffold(tmpDir, ScaffoldOptions{
		Preset:      "unknown-preset",
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      false,
	})

	if err == nil {
		t.Error("expected error for unknown preset")
	}
	if !errors.Is(err, ErrUnknownPreset) {
		t.Errorf("expected ErrUnknownPreset, got: %v", err)
	}
}
