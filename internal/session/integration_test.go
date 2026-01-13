package session

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Integration tests for Session Manager
// Converted from .ai/tests/test_session_manager.sh

// findAwkitBinaryFromSession locates the awkit binary for testing
func findAwkitBinaryFromSession(t *testing.T) string {
	t.Helper()

	// First try to find in repo root
	repoRoot := getSessionTestRepoRoot(t)
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

// getSessionTestRepoRoot returns the repository root directory for tests
func getSessionTestRepoRoot(t *testing.T) string {
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

// sessionIDFormatRegex matches the expected session ID format
var sessionIDFormatRegex = regexp.MustCompile(`^principal-\d{8}-\d{6}-[a-f0-9]{4}$`)

// iso8601Regex matches ISO 8601 timestamp format
var iso8601Regex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

func TestIntegration_Property1_SessionIDFormatConsistency(t *testing.T) {
	awkit := findAwkitBinaryFromSession(t)

	testDir := t.TempDir()
	setupIntegrationTestEnv(t, testDir)

	t.Run("Principal session ID format", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "init")
		cmd.Dir = testDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("session init failed: %v", err)
		}

		sessionID := strings.TrimSpace(string(output))
		if !sessionIDFormatRegex.MatchString(sessionID) {
			t.Errorf("session ID %q does not match format principal-YYYYMMDD-HHMMSS-xxxx", sessionID)
		}
	})

	t.Run("Date component is valid (today's date)", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "init")
		cmd.Dir = testDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("session init failed: %v", err)
		}

		sessionID := strings.TrimSpace(string(output))
		today := time.Now().UTC().Format("20060102")

		if !strings.HasPrefix(sessionID, "principal-"+today+"-") {
			t.Errorf("session ID should contain today's date %s, got %s", today, sessionID)
		}
	})
}

func TestIntegration_Property2_SessionIDUniqueness(t *testing.T) {
	awkit := findAwkitBinaryFromSession(t)

	testDir := t.TempDir()
	setupIntegrationTestEnv(t, testDir)

	cmd := exec.Command(awkit, "session", "init")
	cmd.Dir = testDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("session init failed: %v", err)
	}

	sessionID := strings.TrimSpace(string(output))

	t.Run("Random hex suffix is 4 characters", func(t *testing.T) {
		parts := strings.Split(sessionID, "-")
		if len(parts) != 4 {
			t.Errorf("session ID should have 4 parts, got %d", len(parts))
			return
		}

		hexSuffix := parts[3]
		if len(hexSuffix) != 4 {
			t.Errorf("hex suffix should be 4 characters, got %d: %s", len(hexSuffix), hexSuffix)
		}
	})
}

func TestIntegration_SessionLifecycle(t *testing.T) {
	awkit := findAwkitBinaryFromSession(t)

	testDir := t.TempDir()
	setupIntegrationTestEnv(t, testDir)

	// Initialize session
	cmd := exec.Command(awkit, "session", "init")
	cmd.Dir = testDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("session init failed: %v", err)
	}
	sessionID := strings.TrimSpace(string(output))

	t.Run("init creates session.json", func(t *testing.T) {
		sessionFile := filepath.Join(testDir, ".ai", "state", "principal", "session.json")
		if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
			t.Error("session.json was not created")
		}
	})

	t.Run("init creates session log file", func(t *testing.T) {
		logFile := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Errorf("session log file %s was not created", sessionID+".json")
		}
	})

	t.Run("session.json contains session_id", func(t *testing.T) {
		sessionFile := filepath.Join(testDir, ".ai", "state", "principal", "session.json")
		data, err := os.ReadFile(sessionFile)
		if err != nil {
			t.Fatalf("failed to read session.json: %v", err)
		}

		var session map[string]interface{}
		if err := json.Unmarshal(data, &session); err != nil {
			t.Fatalf("failed to parse session.json: %v", err)
		}

		if session["session_id"] == nil {
			t.Error("session.json should contain session_id")
		}
	})

	t.Run("session.json contains pid", func(t *testing.T) {
		sessionFile := filepath.Join(testDir, ".ai", "state", "principal", "session.json")
		data, _ := os.ReadFile(sessionFile)
		var session map[string]interface{}
		json.Unmarshal(data, &session)

		if session["pid"] == nil {
			t.Error("session.json should contain pid")
		}
	})

	t.Run("session.json contains pid_start_time", func(t *testing.T) {
		sessionFile := filepath.Join(testDir, ".ai", "state", "principal", "session.json")
		data, _ := os.ReadFile(sessionFile)
		var session map[string]interface{}
		json.Unmarshal(data, &session)

		if session["pid_start_time"] == nil {
			t.Error("session.json should contain pid_start_time")
		}
	})

	t.Run("get-id returns correct ID", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "get-id")
		cmd.Dir = testDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("session get-id failed: %v", err)
		}

		retrievedID := strings.TrimSpace(string(output))
		if retrievedID != sessionID {
			t.Errorf("get-id returned %q, want %q", retrievedID, sessionID)
		}
	})

	t.Run("append adds action to log", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "append", sessionID, "test_action", `{"test": "data"}`)
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session append failed: %v", err)
		}

		logFile := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		data, _ := os.ReadFile(logFile)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		actions := log["actions"].([]interface{})
		if len(actions) != 1 {
			t.Errorf("expected 1 action, got %d", len(actions))
		}
	})

	t.Run("action has correct type", func(t *testing.T) {
		logFile := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		data, _ := os.ReadFile(logFile)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		actions := log["actions"].([]interface{})
		action := actions[0].(map[string]interface{})

		if action["type"] != "test_action" {
			t.Errorf("action type = %v, want test_action", action["type"])
		}
	})

	t.Run("action has ISO 8601 timestamp", func(t *testing.T) {
		logFile := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		data, _ := os.ReadFile(logFile)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		actions := log["actions"].([]interface{})
		action := actions[0].(map[string]interface{})
		timestamp := action["timestamp"].(string)

		if !iso8601Regex.MatchString(timestamp) {
			t.Errorf("timestamp %q is not ISO 8601 format", timestamp)
		}
	})

	t.Run("end adds ended_at", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "end", sessionID, "test_complete")
		cmd.Dir = testDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("session end failed: %v", err)
		}

		logFile := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		data, _ := os.ReadFile(logFile)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		if log["ended_at"] == nil || log["ended_at"] == "" {
			t.Error("ended_at should be set after end")
		}
	})

	t.Run("end sets exit_reason", func(t *testing.T) {
		logFile := filepath.Join(testDir, ".ai", "state", "principal", "sessions", sessionID+".json")
		data, _ := os.ReadFile(logFile)
		var log map[string]interface{}
		json.Unmarshal(data, &log)

		if log["exit_reason"] != "test_complete" {
			t.Errorf("exit_reason = %v, want test_complete", log["exit_reason"])
		}
	})
}

func TestIntegration_PreserveEndedSessionExitReason(t *testing.T) {
	awkit := findAwkitBinaryFromSession(t)

	testDir := t.TempDir()
	setupIntegrationTestEnv(t, testDir)

	// Simulate a previous session that ended normally, but whose PID is now stale
	prevSessionID := "principal-20250101-000000-dead"
	sessionsDir := filepath.Join(testDir, ".ai", "state", "principal", "sessions")

	prevSessionLog := map[string]interface{}{
		"session_id":  prevSessionID,
		"started_at":  "2025-01-01T00:00:00Z",
		"ended_at":    "2025-01-01T00:10:00Z",
		"exit_reason": "all_tasks_complete",
		"actions":     []interface{}{},
	}
	data, _ := json.Marshal(prevSessionLog)
	os.WriteFile(filepath.Join(sessionsDir, prevSessionID+".json"), data, 0644)

	prevSessionInfo := map[string]interface{}{
		"session_id":     prevSessionID,
		"started_at":     "2025-01-01T00:00:00Z",
		"pid":            99999,
		"pid_start_time": 0,
	}
	data, _ = json.Marshal(prevSessionInfo)
	os.WriteFile(filepath.Join(testDir, ".ai", "state", "principal", "session.json"), data, 0644)

	t.Run("session init succeeds with stale previous PID", func(t *testing.T) {
		cmd := exec.Command(awkit, "session", "init")
		cmd.Dir = testDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("session init failed: %v", err)
		}

		newSessionID := strings.TrimSpace(string(output))
		if newSessionID == "" {
			t.Error("session init should return new session ID")
		}
	})

	t.Run("previous ended session exit_reason preserved", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(sessionsDir, prevSessionID+".json"))
		if err != nil {
			t.Fatalf("failed to read previous session log: %v", err)
		}

		var log map[string]interface{}
		json.Unmarshal(data, &log)

		if log["exit_reason"] != "all_tasks_complete" {
			t.Errorf("previous session exit_reason = %v, want all_tasks_complete", log["exit_reason"])
		}
	})
}

// Helper functions

func setupIntegrationTestEnv(t *testing.T, dir string) {
	t.Helper()

	// Initialize git repo
	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create required directories
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
