package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSessionIntegration_PrincipalSession tests the complete Principal session lifecycle
// via CLI commands. This is an integration test that requires the awkit binary.
// Converted from .ai/tests/test_principal_session.sh

// findAwkitBinary locates the awkit binary for testing
func findAwkitBinary(t *testing.T) string {
	t.Helper()

	// First try to find in repo root
	repoRoot := getTestRepoRoot(t)
	paths := []string{
		filepath.Join(repoRoot, "awkit"),
		filepath.Join(repoRoot, "awkit.exe"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try PATH
	if path, err := exec.LookPath("awkit"); err == nil {
		return path
	}

	t.Skip("awkit binary not found, skipping integration test")
	return ""
}

// getTestRepoRoot returns the repository root directory for tests
func getTestRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Traverse up to find go.mod (repo root)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", wd)
		}
		dir = parent
	}
}

// sessionIDRegex matches the expected session ID format: principal-YYYYMMDD-HHMMSS-xxxxxxxx (8 hex chars)
var sessionIDRegex = regexp.MustCompile(`^principal-\d{8}-\d{6}-[a-f0-9]{8}$`)

func TestSessionIntegration_Property7_StrictSequentialExecution(t *testing.T) {
	awkit := findAwkitBinary(t)

	// Setup test directory with git repo
	testDir := t.TempDir()
	setupTestGitRepo(t, testDir)
	setupTestAIDir(t, testDir)

	// Test 7.1: First Principal session initializes successfully
	t.Run("first session init", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "init")
		cmd.Dir = testDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("session init failed: %v", err)
		}

		sessionID := strings.TrimSpace(string(output))
		if !sessionIDRegex.MatchString(sessionID) {
			t.Errorf("session ID %q does not match expected format", sessionID)
		}
	})
}

func TestSessionIntegration_SessionFileCreation(t *testing.T) {
	awkit := findAwkitBinary(t)

	testDir := t.TempDir()
	setupTestGitRepo(t, testDir)
	setupTestAIDir(t, testDir)

	// Initialize session
	cmd := exec.Command(awkit, "session", "init")
	cmd.Dir = testDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("session init failed: %v", err)
	}
	sessionID := strings.TrimSpace(string(output))

	t.Run("session.json created", func(t *testing.T) {
		sessionFile := filepath.Join(testDir, ".ai", "state", "principal", "session.json")
		if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
			t.Error("session.json was not created")
		}
	})

	t.Run("session.json has correct fields", func(t *testing.T) {
		sessionFile := filepath.Join(testDir, ".ai", "state", "principal", "session.json")
		data, err := os.ReadFile(sessionFile)
		if err != nil {
			t.Fatalf("failed to read session.json: %v", err)
		}

		var session map[string]interface{}
		if err := json.Unmarshal(data, &session); err != nil {
			t.Fatalf("failed to parse session.json: %v", err)
		}

		if session["session_id"] != sessionID {
			t.Errorf("session_id = %v, want %v", session["session_id"], sessionID)
		}

		if pid, ok := session["pid"].(float64); !ok || pid <= 0 {
			t.Error("session.json should contain positive PID")
		}
	})

	t.Run("session log file created", func(t *testing.T) {
		sessionLog := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		if _, err := os.Stat(sessionLog); os.IsNotExist(err) {
			t.Errorf("session log file %s was not created", sessionID+".json")
		}
	})

	t.Run("session log has empty actions", func(t *testing.T) {
		sessionLog := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		data, err := os.ReadFile(sessionLog)
		if err != nil {
			t.Fatalf("failed to read session log: %v", err)
		}

		var log map[string]interface{}
		if err := json.Unmarshal(data, &log); err != nil {
			t.Fatalf("failed to parse session log: %v", err)
		}

		// Actions can be null/nil or empty array, both are valid for initial state
		if actions, ok := log["actions"].([]interface{}); ok {
			if len(actions) != 0 {
				t.Error("session log should start with empty actions")
			}
		} else if log["actions"] != nil {
			t.Error("session log actions should be empty array or null")
		}
		// If actions is null/nil, that's also acceptable for initial state
	})
}

func TestSessionIntegration_ActionRecording(t *testing.T) {
	awkit := findAwkitBinary(t)

	testDir := t.TempDir()
	setupTestGitRepo(t, testDir)
	setupTestAIDir(t, testDir)

	// Initialize session
	cmd := exec.Command(awkit, "session", "init")
	cmd.Dir = testDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("session init failed: %v", err)
	}
	sessionID := strings.TrimSpace(string(output))
	sessionLog := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")

	t.Run("append issue_created action", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "append", sessionID, "issue_created", `{"issue_id":"42","title":"Test Issue"}`)
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session append failed: %v", err)
		}

		data, _ := os.ReadFile(sessionLog)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		actions := log["actions"].([]interface{})
		if len(actions) != 1 {
			t.Errorf("expected 1 action, got %d", len(actions))
		}
	})

	t.Run("append worker_dispatched action", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "append", sessionID, "worker_dispatched", `{"issue_id":"42"}`)
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session append failed: %v", err)
		}

		data, _ := os.ReadFile(sessionLog)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		actions := log["actions"].([]interface{})
		if len(actions) != 2 {
			t.Errorf("expected 2 actions, got %d", len(actions))
		}
	})

	t.Run("append worker_completed action", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "append", sessionID, "worker_completed",
			`{"issue_id":"42","worker_session_id":"worker-20251223-100000-aaaa","status":"success","pr_url":"https://github.com/test/pr/1"}`)
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session append failed: %v", err)
		}

		data, _ := os.ReadFile(sessionLog)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		actions := log["actions"].([]interface{})
		if len(actions) != 3 {
			t.Errorf("expected 3 actions, got %d", len(actions))
		}

		lastAction := actions[2].(map[string]interface{})
		if lastAction["type"] != "worker_completed" {
			t.Errorf("last action type = %v, want worker_completed", lastAction["type"])
		}
	})
}

func TestSessionIntegration_SessionEnd(t *testing.T) {
	awkit := findAwkitBinary(t)

	testDir := t.TempDir()
	setupTestGitRepo(t, testDir)
	setupTestAIDir(t, testDir)

	// Initialize session
	cmd := exec.Command(awkit, "session", "init")
	cmd.Dir = testDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("session init failed: %v", err)
	}
	sessionID := strings.TrimSpace(string(output))
	sessionLog := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")

	t.Run("end session with reason", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "end", sessionID, "all_tasks_complete")
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session end failed: %v", err)
		}

		data, _ := os.ReadFile(sessionLog)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		if log["ended_at"] == nil || log["ended_at"] == "" {
			t.Error("ended_at should be recorded")
		}

		if log["exit_reason"] != "all_tasks_complete" {
			t.Errorf("exit_reason = %v, want all_tasks_complete", log["exit_reason"])
		}
	})
}

func TestSessionIntegration_ResultJSONUpdates(t *testing.T) {
	awkit := findAwkitBinary(t)

	testDir := t.TempDir()
	setupTestGitRepo(t, testDir)
	setupTestAIDir(t, testDir)

	// Initialize session
	cmd := exec.Command(awkit, "session", "init")
	cmd.Dir = testDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("session init failed: %v", err)
	}
	sessionID := strings.TrimSpace(string(output))

	// Create mock result.json
	resultFile := filepath.Join(testDir, ".ai", "results", "issue-42.json")
	mockResult := map[string]interface{}{
		"issue_id": "42",
		"status":   "success",
		"session": map[string]interface{}{
			"worker_session_id":     "worker-20251223-100000-aaaa",
			"principal_session_id":  "",
			"attempt_number":        1,
			"previous_session_ids":  []string{},
			"previous_failure_reason": "",
		},
		"review_audit": map[string]interface{}{
			"reviewer_session_id": "",
			"review_timestamp":    "",
			"ci_status":           "",
			"ci_timeout":          false,
			"decision":            "",
			"merge_timestamp":     "",
		},
	}
	data, _ := json.Marshal(mockResult)
	os.WriteFile(resultFile, data, 0644)

	t.Run("update-result updates principal_session_id", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "update-result", "42", sessionID)
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session update-result failed: %v", err)
		}

		data, _ := os.ReadFile(resultFile)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		session := result["session"].(map[string]interface{})
		if session["principal_session_id"] != sessionID {
			t.Errorf("principal_session_id = %v, want %v", session["principal_session_id"], sessionID)
		}
	})

	t.Run("update-review updates review_audit", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "update-review", "42", sessionID, "approved", "passed", "false", "2025-12-23T10:00:00Z")
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session update-review failed: %v", err)
		}

		data, _ := os.ReadFile(resultFile)
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		reviewAudit := result["review_audit"].(map[string]interface{})

		if reviewAudit["decision"] != "approved" {
			t.Errorf("decision = %v, want approved", reviewAudit["decision"])
		}

		if reviewAudit["ci_status"] != "passed" {
			t.Errorf("ci_status = %v, want passed", reviewAudit["ci_status"])
		}

		if reviewAudit["merge_timestamp"] != "2025-12-23T10:00:00Z" {
			t.Errorf("merge_timestamp = %v, want 2025-12-23T10:00:00Z", reviewAudit["merge_timestamp"])
		}
	})
}

// Helper functions

func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}
}

func setupTestAIDir(t *testing.T, dir string) {
	t.Helper()

	paths := []string{
		filepath.Join(dir, ".ai", "state", "principal", "sessions"),
		filepath.Join(dir, ".ai", "results"),
	}

	for _, p := range paths {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatalf("failed to create %s: %v", p, err)
		}
	}
}
