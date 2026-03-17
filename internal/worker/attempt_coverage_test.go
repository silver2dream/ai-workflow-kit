package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// AttemptGuard.Check
// ---------------------------------------------------------------------------

func TestCov_AttemptGuard_Check(t *testing.T) {
	t.Run("first attempt succeeds", func(t *testing.T) {
		dir := t.TempDir()
		guard := &AttemptGuard{
			StateRoot:   dir,
			IssueID:     1,
			MaxAttempts: 3,
		}

		result, err := guard.Check()
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if !result.CanProceed {
			t.Error("expected CanProceed=true for first attempt")
		}
		if result.AttemptNumber != 1 {
			t.Errorf("expected attempt 1, got %d", result.AttemptNumber)
		}
		if result.Reason != "ok" {
			t.Errorf("expected reason 'ok', got %q", result.Reason)
		}
	})

	t.Run("increment on successive attempts", func(t *testing.T) {
		dir := t.TempDir()
		guard := &AttemptGuard{
			StateRoot:   dir,
			IssueID:     2,
			MaxAttempts: 5,
		}

		// First attempt
		r1, _ := guard.Check()
		if r1.AttemptNumber != 1 {
			t.Errorf("expected 1, got %d", r1.AttemptNumber)
		}

		// Second attempt
		r2, _ := guard.Check()
		if r2.AttemptNumber != 2 {
			t.Errorf("expected 2, got %d", r2.AttemptNumber)
		}
	})

	t.Run("max attempts reached", func(t *testing.T) {
		dir := t.TempDir()
		guard := &AttemptGuard{
			StateRoot:   dir,
			IssueID:     3,
			MaxAttempts: 2,
		}

		// Exhaust attempts
		guard.Check()
		guard.Check()

		// Should be blocked now
		result, err := guard.Check()
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if result.CanProceed {
			t.Error("expected CanProceed=false after max attempts")
		}
		if result.Reason != "max_attempts" {
			t.Errorf("expected reason 'max_attempts', got %q", result.Reason)
		}
	})
}

// ---------------------------------------------------------------------------
// AttemptGuard.RecordFailure
// ---------------------------------------------------------------------------

func TestCov_AttemptGuard_RecordFailure(t *testing.T) {
	dir := t.TempDir()
	guard := &AttemptGuard{
		StateRoot:   dir,
		IssueID:     10,
		MaxAttempts: 3,
	}

	// Do a check first to create directories
	guard.Check()

	err := guard.RecordFailure("build_error", true)
	if err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}

	// Verify the history file exists and has content
	historyPath := filepath.Join(dir, ".ai", "state", "failure_history.jsonl")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("history file not found: %v", err)
	}

	var entry FailureEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("failed to parse history entry: %v", err)
	}

	if entry.IssueID != 10 {
		t.Errorf("expected IssueID 10, got %d", entry.IssueID)
	}
	if entry.Type != "build_error" {
		t.Errorf("expected type 'build_error', got %q", entry.Type)
	}
	if !entry.Retryable {
		t.Error("expected retryable=true")
	}
	if entry.PatternID != "go-impl" {
		t.Errorf("expected PatternID 'go-impl', got %q", entry.PatternID)
	}
}

// ---------------------------------------------------------------------------
// AttemptGuard.Reset
// ---------------------------------------------------------------------------

func TestCov_AttemptGuard_Reset(t *testing.T) {
	dir := t.TempDir()
	guard := &AttemptGuard{
		StateRoot:   dir,
		IssueID:     20,
		MaxAttempts: 3,
	}

	// Check twice to increment
	guard.Check()
	guard.Check()

	// Reset
	err := guard.Reset()
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Should be back to first attempt
	result, _ := guard.Check()
	if result.AttemptNumber != 1 {
		t.Errorf("expected attempt 1 after reset, got %d", result.AttemptNumber)
	}
}

// ---------------------------------------------------------------------------
// NewAttemptGuard with env var
// ---------------------------------------------------------------------------

func TestCov_NewAttemptGuard_EnvOverride(t *testing.T) {
	os.Setenv("AI_MAX_ATTEMPTS", "7")
	defer os.Unsetenv("AI_MAX_ATTEMPTS")

	guard := NewAttemptGuard(t.TempDir(), 1)
	if guard.MaxAttempts != 7 {
		t.Errorf("expected MaxAttempts=7, got %d", guard.MaxAttempts)
	}
}

func TestCov_NewAttemptGuard_InvalidEnv(t *testing.T) {
	os.Setenv("AI_MAX_ATTEMPTS", "not-a-number")
	defer os.Unsetenv("AI_MAX_ATTEMPTS")

	guard := NewAttemptGuard(t.TempDir(), 1)
	if guard.MaxAttempts != 3 {
		t.Errorf("expected default MaxAttempts=3, got %d", guard.MaxAttempts)
	}
}

func TestCov_NewAttemptGuard_ZeroEnv(t *testing.T) {
	os.Setenv("AI_MAX_ATTEMPTS", "0")
	defer os.Unsetenv("AI_MAX_ATTEMPTS")

	guard := NewAttemptGuard(t.TempDir(), 1)
	if guard.MaxAttempts != 3 {
		t.Errorf("expected default MaxAttempts=3 for zero env, got %d", guard.MaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// FailureEntry JSON
// ---------------------------------------------------------------------------

func TestCov_FailureEntryJSON(t *testing.T) {
	entry := FailureEntry{
		Timestamp: "2024-01-15T09:30:00Z",
		IssueID:   42,
		Attempt:   2,
		PatternID: "go-impl",
		Type:      "test_failure",
		Retryable: true,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded FailureEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if loaded.IssueID != 42 {
		t.Errorf("IssueID mismatch: %d", loaded.IssueID)
	}
	if loaded.Type != "test_failure" {
		t.Errorf("Type mismatch: %q", loaded.Type)
	}
}
