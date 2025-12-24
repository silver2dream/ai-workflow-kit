package kickoff

import (
	"os"
	"os/exec"
	"path/filepath"
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
