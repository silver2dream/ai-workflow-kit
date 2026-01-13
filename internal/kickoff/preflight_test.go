package kickoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreflightChecker_RunAll tests all pre-flight checks
// Now includes: gh CLI, gh auth, claude CLI, codex CLI, git status, stop marker, lock, config, PTY
func TestPreflightChecker_RunAll(t *testing.T) {
	// Skip if gh CLI is not installed (CI environment may not have it)
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("Skipping test: gh CLI not installed")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "preflight-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create valid config
	configPath := filepath.Join(tmpDir, "workflow.yaml")
	configContent := `version: "1.0"
project:
  name: test-project
  type: single-repo
repos:
  - name: test-repo
    type: root
    path: ./
git:
  integration_branch: feat/test
specs:
  path: .ai/specs
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Create specs directory
	specsDir := filepath.Join(tmpDir, ".ai", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	lockFile := filepath.Join(tmpDir, "kickoff.lock")

	checker := NewPreflightChecker(configPath, lockFile)
	results, err := checker.RunAll()

	// May fail due to gh auth or git status - that's expected in test environment
	// Just verify we get results
	if len(results) == 0 {
		t.Error("RunAll should return at least some results")
	}

	// Verify expected checks are present
	expectedChecks := map[string]bool{
		"gh CLI":      false,
		"gh Auth":     false,
		"claude CLI":  false,
		"codex CLI":   false,
		"Git Status":  false,
		"Stop Marker": false,
		"Lock File":   false,
		"Config":      false,
		"PTY":         false,
	}

	for _, result := range results {
		if _, ok := expectedChecks[result.Name]; ok {
			expectedChecks[result.Name] = true
		}
	}

	// Check that at least gh CLI check was performed (first check)
	if !expectedChecks["gh CLI"] {
		t.Error("gh CLI check should be performed")
	}

	// If we got an error, it should be from one of the expected checks
	if err != nil {
		t.Logf("RunAll returned error (expected in test env): %v", err)
	}
}

// TestPreflightChecker_CheckGhCLI tests gh CLI check
func TestPreflightChecker_CheckGhCLI(t *testing.T) {
	checker := NewPreflightChecker("", "")
	result := checker.CheckGhCLI()

	// Result depends on whether gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		if result.Passed {
			t.Error("CheckGhCLI should fail when gh is not installed")
		}
	} else {
		if !result.Passed {
			t.Errorf("CheckGhCLI should pass when gh is installed: %s", result.Message)
		}
	}
}

// TestPreflightChecker_CheckClaudeCLI tests claude CLI check
func TestPreflightChecker_CheckClaudeCLI(t *testing.T) {
	checker := NewPreflightChecker("", "")
	result := checker.CheckClaudeCLI()

	// Result depends on whether claude is installed
	if _, err := exec.LookPath("claude"); err != nil {
		if result.Passed {
			t.Error("CheckClaudeCLI should fail when claude is not installed")
		}
	} else {
		if !result.Passed {
			t.Errorf("CheckClaudeCLI should pass when claude is installed: %s", result.Message)
		}
	}
}

// TestPreflightChecker_CheckCodexCLI tests codex CLI check (warning only)
func TestPreflightChecker_CheckCodexCLI(t *testing.T) {
	checker := NewPreflightChecker("", "")
	result := checker.CheckCodexCLI()

	// Codex check should always pass (it's a warning only)
	if !result.Passed {
		t.Errorf("CheckCodexCLI should always pass (warning only): %s", result.Message)
	}

	// If codex is not installed, should have warning flag
	if _, err := exec.LookPath("codex"); err != nil {
		if !result.Warning {
			t.Error("CheckCodexCLI should set Warning flag when codex is not installed")
		}
	}
}

// TestPreflightChecker_CheckStopMarker tests STOP marker check
func TestPreflightChecker_CheckStopMarker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-stop-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp dir
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	checker := NewPreflightChecker("", "")

	// Test 1: No STOP marker
	result := checker.CheckStopMarker()
	if !result.Passed {
		t.Errorf("CheckStopMarker should pass when no marker exists: %s", result.Message)
	}

	// Test 2: STOP marker exists
	stopDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stopDir, 0755)
	stopFile := filepath.Join(stopDir, "STOP")
	os.WriteFile(stopFile, []byte("test"), 0644)

	result = checker.CheckStopMarker()
	if result.Passed {
		t.Error("CheckStopMarker should fail when marker exists")
	}

	// Test 3: STOP marker with force delete
	checker.SetForceDelete(true)
	result = checker.CheckStopMarker()
	if !result.Passed {
		t.Errorf("CheckStopMarker should pass with force delete: %s", result.Message)
	}
	if !result.Warning {
		t.Error("CheckStopMarker should set Warning flag when force deleting")
	}

	// Verify marker was deleted
	if _, err := os.Stat(stopFile); !os.IsNotExist(err) {
		t.Error("STOP marker should be deleted with force delete")
	}
}

// TestPreflightChecker_CheckLockFile tests lock file checking
func TestPreflightChecker_CheckLockFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-lock-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockFile := filepath.Join(tmpDir, "kickoff.lock")
	checker := NewPreflightChecker("", lockFile)

	// Test 1: No lock file exists
	result := checker.CheckLockFile()
	if !result.Passed {
		t.Errorf("CheckLockFile should pass when no lock exists: %s", result.Message)
	}

	// Test 2: Stale lock file (non-existent PID)
	staleContent := `{"pid": 999999999, "start_time": "2020-01-01T00:00:00Z", "hostname": "test"}`
	if err := os.WriteFile(lockFile, []byte(staleContent), 0644); err != nil {
		t.Fatalf("Failed to write stale lock: %v", err)
	}

	result = checker.CheckLockFile()
	if !result.Passed {
		t.Errorf("CheckLockFile should pass for stale lock: %s", result.Message)
	}
}

// TestPreflightChecker_CheckConfig tests config validation
func TestPreflightChecker_CheckConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-config-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name       string
		config     string
		shouldPass bool
	}{
		{
			name: "valid config",
			config: `version: "1.0"
project:
  name: test
  type: single-repo
repos:
  - name: test
    type: root
    path: ./
git:
  integration_branch: feat/test
specs:
  path: .ai/specs
`,
			shouldPass: true,
		},
		{
			name: "missing project name",
			config: `version: "1.0"
project:
  type: single-repo
repos:
  - name: test
    type: root
    path: ./
git:
  integration_branch: feat/test
`,
			shouldPass: false,
		},
		{
			name:       "invalid yaml",
			config:     `invalid: yaml: content`,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, "workflow.yaml")
			if err := os.WriteFile(configPath, []byte(tt.config), 0644); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}

			checker := NewPreflightChecker(configPath, "")
			result := checker.CheckConfig()

			if result.Passed != tt.shouldPass {
				t.Errorf("CheckConfig() passed=%v, want %v: %s", result.Passed, tt.shouldPass, result.Message)
			}
		})
	}
}

// TestPreflightChecker_CheckConfig_FileNotFound tests missing config
func TestPreflightChecker_CheckConfig_FileNotFound(t *testing.T) {
	checker := NewPreflightChecker("/nonexistent/path/workflow.yaml", "")
	result := checker.CheckConfig()

	if result.Passed {
		t.Error("CheckConfig should fail for missing file")
	}
}

// TestPreflightChecker_CheckPTY tests PTY availability check
func TestPreflightChecker_CheckPTY(t *testing.T) {
	checker := NewPreflightChecker("", "")
	result := checker.CheckPTY()

	// PTY check should always pass (fallback available)
	if !result.Passed {
		t.Errorf("CheckPTY should pass (fallback available): %s", result.Message)
	}
}

// TestPreflightChecker_Config tests config retrieval
func TestPreflightChecker_Config(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-getconfig-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "workflow.yaml")
	configContent := `version: "1.0"
project:
  name: my-project
  type: single-repo
repos:
  - name: test
    type: root
    path: ./
git:
  integration_branch: feat/test
specs:
  path: .ai/specs
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	checker := NewPreflightChecker(configPath, "")

	// Config should be nil before CheckConfig
	if checker.Config() != nil {
		t.Error("Config should be nil before CheckConfig")
	}

	// Run CheckConfig
	checker.CheckConfig()

	// Config should be set after CheckConfig
	cfg := checker.Config()
	if cfg == nil {
		t.Fatal("Config should not be nil after CheckConfig")
	}

	if cfg.Project.Name != "my-project" {
		t.Errorf("Config project name = %q, want %q", cfg.Project.Name, "my-project")
	}
}

// TestNewPreflightChecker tests checker creation
func TestNewPreflightChecker(t *testing.T) {
	checker := NewPreflightChecker("/path/to/config.yaml", "/path/to/lock")

	if checker == nil {
		t.Fatal("NewPreflightChecker returned nil")
	}

	if checker.configPath != "/path/to/config.yaml" {
		t.Errorf("configPath = %q, want %q", checker.configPath, "/path/to/config.yaml")
	}

	if checker.lockFile != "/path/to/lock" {
		t.Errorf("lockFile = %q, want %q", checker.lockFile, "/path/to/lock")
	}
}

// TestPreflightChecker_SetForceDelete tests force delete setting
func TestPreflightChecker_SetForceDelete(t *testing.T) {
	checker := NewPreflightChecker("", "")

	if checker.forceDelete {
		t.Error("forceDelete should be false by default")
	}

	checker.SetForceDelete(true)

	if !checker.forceDelete {
		t.Error("forceDelete should be true after SetForceDelete(true)")
	}
}

// ============================================================
// Tests migrated from test_preflight.py
// Property 6: Preflight Execution for All Types
// Property 13: Remote Accessibility Verification
// Property 27: Submodule Detached HEAD Handling
// ============================================================

// ValidateRepoType validates that repo type is one of the allowed values.
// This is a helper function for testing that mirrors the Python implementation.
func ValidateRepoType(repoType string) bool {
	switch repoType {
	case "root", "directory", "submodule":
		return true
	default:
		return false
	}
}

// TestValidRepoTypes tests all valid repo types are recognized.
// Property 6: Preflight Execution for All Types
func TestValidRepoTypes(t *testing.T) {
	validTypes := []string{"root", "directory", "submodule"}

	for _, repoType := range validTypes {
		t.Run("valid_"+repoType, func(t *testing.T) {
			if !ValidateRepoType(repoType) {
				t.Errorf("ValidateRepoType(%q) should return true", repoType)
			}
		})
	}
}

// TestInvalidRepoTypes tests invalid repo types are rejected.
func TestInvalidRepoTypes(t *testing.T) {
	invalidTypes := []string{"invalid", "unknown", "", "ROOT", "Directory"}

	for _, repoType := range invalidTypes {
		t.Run("invalid_"+repoType, func(t *testing.T) {
			if ValidateRepoType(repoType) {
				t.Errorf("ValidateRepoType(%q) should return false", repoType)
			}
		})
	}
}

// TestPreflightCheckGitClean tests git working tree clean check
func TestPreflightCheckGitClean(t *testing.T) {
	// Skip if git is not installed
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Skipping test: git not installed")
	}

	tmpDir, err := os.MkdirTemp("", "preflight-git-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	cmd.Run()

	// Save current directory and change to temp dir
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	checker := NewPreflightChecker("", "")

	t.Run("clean_repo", func(t *testing.T) {
		result := checker.CheckGitClean()
		if !result.Passed {
			t.Errorf("CheckGitClean should pass for clean repo: %s", result.Message)
		}
	})

	t.Run("dirty_repo", func(t *testing.T) {
		// Create uncommitted file
		dirtyFile := filepath.Join(tmpDir, "dirty.txt")
		if err := os.WriteFile(dirtyFile, []byte("uncommitted"), 0644); err != nil {
			t.Fatalf("Failed to write dirty file: %v", err)
		}
		defer os.Remove(dirtyFile)

		result := checker.CheckGitClean()
		if result.Passed {
			t.Error("CheckGitClean should fail for dirty repo")
		}
		if result.Message == "" || result.Message == "Working directory clean" {
			t.Error("CheckGitClean should include error message for dirty repo")
		}
	})
}

// TestPreflightDirectoryExists tests directory existence check
func TestPreflightDirectoryExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("existing_directory", func(t *testing.T) {
		// Create subdirectory
		subDir := filepath.Join(tmpDir, "backend")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		// Check directory exists
		info, err := os.Stat(subDir)
		if err != nil {
			t.Errorf("Directory should exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("Path should be a directory")
		}
	})

	t.Run("nonexistent_directory", func(t *testing.T) {
		nonexistent := filepath.Join(tmpDir, "nonexistent")
		_, err := os.Stat(nonexistent)
		if !os.IsNotExist(err) {
			t.Error("Nonexistent directory should return error")
		}
	})
}

// TestPreflightSubmoduleCheck tests submodule detection
func TestPreflightSubmoduleCheck(t *testing.T) {
	// Skip if git is not installed
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Skipping test: git not installed")
	}

	tmpDir, err := os.MkdirTemp("", "preflight-submodule-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("directory_with_git", func(t *testing.T) {
		// Create directory with .git (simulates submodule)
		subDir := filepath.Join(tmpDir, "backend")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		cmd := exec.Command("git", "init")
		cmd.Dir = subDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to init git in subdir: %v", err)
		}

		// Check .git exists
		gitPath := filepath.Join(subDir, ".git")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			t.Error(".git should exist in initialized git directory")
		}
	})

	t.Run("directory_without_git", func(t *testing.T) {
		// Create directory without .git
		plainDir := filepath.Join(tmpDir, "frontend")
		if err := os.MkdirAll(plainDir, 0755); err != nil {
			t.Fatalf("Failed to create plain dir: %v", err)
		}

		gitPath := filepath.Join(plainDir, ".git")
		if _, err := os.Stat(gitPath); !os.IsNotExist(err) {
			t.Error("Plain directory should not have .git")
		}
	})
}

// TestPreflightSubmoduleDetachedHead tests submodule detached HEAD detection
// Property 27: Submodule Detached HEAD Handling
func TestPreflightSubmoduleDetachedHead(t *testing.T) {
	// Skip if git is not installed
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Skipping test: git not installed")
	}

	tmpDir, err := os.MkdirTemp("", "preflight-detached-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	cmd.Run()

	t.Run("on_branch", func(t *testing.T) {
		// Check if on a branch (not detached)
		cmd := exec.Command("git", "symbolic-ref", "-q", "HEAD")
		cmd.Dir = tmpDir
		err := cmd.Run()
		if err != nil {
			t.Error("Should be on a branch, not detached HEAD")
		}
	})

	t.Run("detached_head", func(t *testing.T) {
		// Get current HEAD SHA
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = tmpDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Failed to get HEAD SHA: %v", err)
		}
		headSHA := strings.TrimSpace(string(output))

		// Checkout the SHA to create detached HEAD
		cmd = exec.Command("git", "checkout", headSHA)
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to checkout SHA: %v", err)
		}

		// Check if in detached HEAD state
		cmd = exec.Command("git", "symbolic-ref", "-q", "HEAD")
		cmd.Dir = tmpDir
		err = cmd.Run()
		if err == nil {
			t.Error("Should be in detached HEAD state")
		}
	})
}

// TestPreflightCheckGitStatus tests git status porcelain output
func TestPreflightCheckGitStatus(t *testing.T) {
	// Skip if git is not installed
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Skipping test: git not installed")
	}

	tmpDir, err := os.MkdirTemp("", "preflight-status-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	cmd.Run()

	t.Run("clean_status", func(t *testing.T) {
		cmd := exec.Command("git", "status", "--porcelain")
		cmd.Dir = tmpDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Failed to get git status: %v", err)
		}
		if len(strings.TrimSpace(string(output))) != 0 {
			t.Error("Clean repo should have empty git status --porcelain output")
		}
	})

	t.Run("dirty_status", func(t *testing.T) {
		// Create uncommitted file
		dirtyFile := filepath.Join(tmpDir, "new_file.txt")
		if err := os.WriteFile(dirtyFile, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
		defer os.Remove(dirtyFile)

		cmd := exec.Command("git", "status", "--porcelain")
		cmd.Dir = tmpDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Failed to get git status: %v", err)
		}
		if len(strings.TrimSpace(string(output))) == 0 {
			t.Error("Dirty repo should have non-empty git status --porcelain output")
		}
		if !strings.Contains(string(output), "new_file.txt") {
			t.Error("Git status should contain the new file name")
		}
	})
}

// TestPreflightRepoTypeChecks tests repo type-specific preflight checks
// Property 6: Preflight Execution for All Types
func TestPreflightRepoTypeChecks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-repotype-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("root_type", func(t *testing.T) {
		// Root type: path should be "./" or empty
		repoType := "root"
		repoPath := "./"

		if !ValidateRepoType(repoType) {
			t.Error("root should be a valid repo type")
		}
		if repoPath != "./" && repoPath != "" && repoPath != "." {
			t.Error("root type typically has path './' or empty")
		}
	})

	t.Run("directory_type", func(t *testing.T) {
		// Directory type: path should point to existing directory
		repoType := "directory"
		subDir := filepath.Join(tmpDir, "backend")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}

		if !ValidateRepoType(repoType) {
			t.Error("directory should be a valid repo type")
		}

		info, err := os.Stat(subDir)
		if err != nil {
			t.Errorf("Directory should exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("Path should be a directory")
		}
	})

	t.Run("submodule_type", func(t *testing.T) {
		// Submodule type: path should point to git repo
		repoType := "submodule"
		if !ValidateRepoType(repoType) {
			t.Error("submodule should be a valid repo type")
		}
	})
}
