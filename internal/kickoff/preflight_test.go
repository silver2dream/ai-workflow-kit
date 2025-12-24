package kickoff

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPreflightChecker_RunAll tests Property 1: Pre-flight checks scope
// awkit only checks: lock file, config, PTY (NOT gh auth, working directory)
func TestPreflightChecker_RunAll(t *testing.T) {
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

	if err != nil {
		t.Fatalf("RunAll failed: %v", err)
	}

	// Property 1: Should only have awkit-level checks
	expectedChecks := map[string]bool{
		"Lock File": false,
		"Config":    false,
		"PTY":       false,
	}

	for _, result := range results {
		if _, ok := expectedChecks[result.Name]; ok {
			expectedChecks[result.Name] = true
		} else {
			t.Errorf("Unexpected check: %s (awkit should not check this)", result.Name)
		}
	}

	// Verify all expected checks were performed
	for name, found := range expectedChecks {
		if !found {
			t.Errorf("Missing expected check: %s", name)
		}
	}

	// Verify NO gh auth or working directory checks
	for _, result := range results {
		if result.Name == "gh auth" || result.Name == "GitHub Auth" {
			t.Error("awkit should NOT check gh auth (Principal's responsibility)")
		}
		if result.Name == "Working Directory" || result.Name == "Git Status" {
			t.Error("awkit should NOT check working directory (Principal's responsibility)")
		}
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
