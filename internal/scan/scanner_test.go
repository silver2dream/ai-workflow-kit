package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage_Go(t *testing.T) {
	// Create temp directory with go.mod
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	lang := DetectLanguage(tmpDir)
	if lang != "go" {
		t.Errorf("expected 'go', got '%s'", lang)
	}
}

func TestDetectLanguage_Unity(t *testing.T) {
	// Create temp directory with ProjectSettings/
	tmpDir := t.TempDir()
	projectSettings := filepath.Join(tmpDir, "ProjectSettings")
	if err := os.MkdirAll(projectSettings, 0755); err != nil {
		t.Fatalf("failed to create ProjectSettings: %v", err)
	}

	lang := DetectLanguage(tmpDir)
	if lang != "unity" {
		t.Errorf("expected 'unity', got '%s'", lang)
	}
}

func TestDetectLanguage_Unknown(t *testing.T) {
	// Create empty temp directory
	tmpDir := t.TempDir()

	lang := DetectLanguage(tmpDir)
	if lang != "unknown" {
		t.Errorf("expected 'unknown', got '%s'", lang)
	}
}

func TestDetectLanguage_GoTakesPrecedence(t *testing.T) {
	// Create temp directory with both go.mod and ProjectSettings/
	tmpDir := t.TempDir()

	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module test\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	projectSettings := filepath.Join(tmpDir, "ProjectSettings")
	if err := os.MkdirAll(projectSettings, 0755); err != nil {
		t.Fatalf("failed to create ProjectSettings: %v", err)
	}

	// go.mod should take precedence since it's checked first
	lang := DetectLanguage(tmpDir)
	if lang != "go" {
		t.Errorf("expected 'go' to take precedence, got '%s'", lang)
	}
}

func TestCountTestFiles_Go(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create test files
	testFiles := []string{
		"main_test.go",
		"handler_test.go",
		"utils_test.go",
	}
	for _, f := range testFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("package test\n"), 0644); err != nil {
			t.Fatalf("failed to create %s: %v", f, err)
		}
	}

	// Create a non-test file (should not be counted)
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create a subdirectory with more test files
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "sub_test.go"), []byte("package sub\n"), 0644); err != nil {
		t.Fatalf("failed to create sub_test.go: %v", err)
	}

	count := CountTestFiles(tmpDir, "go")
	expected := 4 // 3 in root + 1 in subdirectory
	if count != expected {
		t.Errorf("expected %d test files, got %d", expected, count)
	}
}

func TestCountTestFiles_Go_SkipsVendor(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file in root
	if err := os.WriteFile(filepath.Join(tmpDir, "main_test.go"), []byte("package test\n"), 0644); err != nil {
		t.Fatalf("failed to create main_test.go: %v", err)
	}

	// Create vendor directory with test file (should be skipped)
	vendorDir := filepath.Join(tmpDir, "vendor", "pkg")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatalf("failed to create vendor directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "vendor_test.go"), []byte("package pkg\n"), 0644); err != nil {
		t.Fatalf("failed to create vendor_test.go: %v", err)
	}

	count := CountTestFiles(tmpDir, "go")
	expected := 1 // only the root test file
	if count != expected {
		t.Errorf("expected %d test files (vendor should be skipped), got %d", expected, count)
	}
}

func TestCountTestFiles_Unity(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Unity project structure
	if err := os.MkdirAll(filepath.Join(tmpDir, "ProjectSettings"), 0755); err != nil {
		t.Fatalf("failed to create ProjectSettings: %v", err)
	}

	// Create Assets/Tests directory with test files
	testsDir := filepath.Join(tmpDir, "Assets", "Tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		t.Fatalf("failed to create Assets/Tests: %v", err)
	}

	testFiles := []string{
		"PlayerTests.cs",
		"InventoryTests.cs",
	}
	for _, f := range testFiles {
		if err := os.WriteFile(filepath.Join(testsDir, f), []byte("using NUnit.Framework;\n"), 0644); err != nil {
			t.Fatalf("failed to create %s: %v", f, err)
		}
	}

	count := CountTestFiles(tmpDir, "unity")
	expected := 2
	if count != expected {
		t.Errorf("expected %d test files, got %d", expected, count)
	}
}

func TestCountTestFiles_Unknown(t *testing.T) {
	tmpDir := t.TempDir()

	count := CountTestFiles(tmpDir, "unknown")
	if count != 0 {
		t.Errorf("expected 0 test files for unknown language, got %d", count)
	}
}

func TestHasSubmodules_True(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .gitmodules file
	gitModulesPath := filepath.Join(tmpDir, ".gitmodules")
	if err := os.WriteFile(gitModulesPath, []byte("[submodule \"lib\"]\n\tpath = lib\n\turl = https://example.com/lib.git\n"), 0644); err != nil {
		t.Fatalf("failed to create .gitmodules: %v", err)
	}

	if !HasSubmodules(tmpDir) {
		t.Error("expected HasSubmodules to return true")
	}
}

func TestHasSubmodules_False(t *testing.T) {
	tmpDir := t.TempDir()

	if HasSubmodules(tmpDir) {
		t.Error("expected HasSubmodules to return false")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	// Test with a non-git directory should return empty string
	tmpDir := t.TempDir()

	branch := GetCurrentBranch(tmpDir)
	if branch != "" {
		t.Errorf("expected empty string for non-git directory, got '%s'", branch)
	}
}

func TestGetCurrentBranch_GitRepo(t *testing.T) {
	// Use the actual repo we're in
	// This test will work when run from within the ai-workflow-kit repo
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get working directory")
	}

	// Walk up to find .git directory
	repoRoot := wd
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Skip("not running within a git repository")
		}
		repoRoot = parent
	}

	branch := GetCurrentBranch(repoRoot)
	if branch == "" {
		t.Error("expected non-empty branch name for git repository")
	}
}

func TestScanRepo_Integration(t *testing.T) {
	// Create a complete test repository structure
	tmpDir := t.TempDir()

	// Create go.mod
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "main_test.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create main_test.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "handler_test.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create handler_test.go: %v", err)
	}

	// Create CLAUDE.md
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte("# Claude Instructions\n"), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	// Create AGENTS.md
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Agents Instructions\n"), 0644); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	// Create .ai/config/workflow.yaml
	aiConfigDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(aiConfigDir, 0755); err != nil {
		t.Fatalf("failed to create .ai/config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aiConfigDir, "workflow.yaml"), []byte("version: 1\n"), 0644); err != nil {
		t.Fatalf("failed to create workflow.yaml: %v", err)
	}

	// Scan the repository
	result, err := ScanRepo(tmpDir)
	if err != nil {
		t.Fatalf("ScanRepo failed: %v", err)
	}

	// Verify results
	if result.Language != "go" {
		t.Errorf("expected language 'go', got '%s'", result.Language)
	}

	if result.TestFileCount != 2 {
		t.Errorf("expected 2 test files, got %d", result.TestFileCount)
	}

	if result.HasSubmodules {
		t.Error("expected HasSubmodules to be false")
	}

	// Branch will be empty since this is not a real git repo
	if result.Branch != "" {
		t.Errorf("expected empty branch for non-git directory, got '%s'", result.Branch)
	}

	if !result.HasClaudeMD {
		t.Error("expected HasClaudeMD to be true")
	}

	if !result.HasAgentsMD {
		t.Error("expected HasAgentsMD to be true")
	}

	if !result.HasAIConfig {
		t.Error("expected HasAIConfig to be true")
	}
}

func TestScanRepo_NonExistentPath(t *testing.T) {
	_, err := ScanRepo("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestScanRepo_FileInsteadOfDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-a-directory")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	_, err := ScanRepo(filePath)
	if err == nil {
		t.Error("expected error when scanning a file instead of directory")
	}
}
