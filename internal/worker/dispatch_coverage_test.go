package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// DispatchOutput.FormatBashOutput
// ---------------------------------------------------------------------------

func TestCov_DispatchOutput_FormatBashOutput(t *testing.T) {
	tests := []struct {
		name   string
		output DispatchOutput
		checks []string
	}{
		{
			name: "success with PR",
			output: DispatchOutput{
				Status: "success",
				PRURL:  "https://github.com/org/repo/pull/42",
			},
			checks: []string{
				"WORKER_STATUS=success",
				"PR_URL=",
			},
		},
		{
			name: "failed with error",
			output: DispatchOutput{
				Status: "failed",
				Error:  "some error message",
			},
			checks: []string{
				"WORKER_STATUS=failed",
				"WORKER_ERROR=",
			},
		},
		{
			name: "in_progress",
			output: DispatchOutput{
				Status: "in_progress",
			},
			checks: []string{
				"WORKER_STATUS=in_progress",
			},
		},
		{
			name: "needs_conflict_resolution",
			output: DispatchOutput{
				Status:       "needs_conflict_resolution",
				WorktreePath: "/tmp/worktree",
				IssueNumber:  42,
				PRNumber:     99,
			},
			checks: []string{
				"WORKER_STATUS=needs_conflict_resolution",
				"WORKTREE_PATH=",
				"ISSUE_NUMBER=42",
				"PR_NUMBER=99",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.output.FormatBashOutput()
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("FormatBashOutput() missing %q in:\n%s", check, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DispatchOutput no conflict fields for non-conflict status
// ---------------------------------------------------------------------------

func TestCov_DispatchOutput_NoConflictFieldsForSuccess(t *testing.T) {
	output := DispatchOutput{
		Status: "success",
		PRURL:  "https://github.com/org/repo/pull/42",
	}
	got := output.FormatBashOutput()
	if strings.Contains(got, "WORKTREE_PATH=") {
		t.Error("unexpected WORKTREE_PATH in success output")
	}
	if strings.Contains(got, "ISSUE_NUMBER=") {
		t.Error("unexpected ISSUE_NUMBER in success output")
	}
}

// ---------------------------------------------------------------------------
// DispatchLogger
// ---------------------------------------------------------------------------

func TestCov_DispatchLogger(t *testing.T) {
	t.Run("logs to file", func(t *testing.T) {
		dir := t.TempDir()
		logDir := filepath.Join(dir, ".ai", "exe-logs")
		os.MkdirAll(logDir, 0755)

		logger := NewDispatchLogger(dir, 42)
		logger.Log("test message %d", 123)
		if err := logger.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		logPath := filepath.Join(logDir, "principal.log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("log file not found: %v", err)
		}
		if !strings.Contains(string(data), "test message 123") {
			t.Error("expected message in log file")
		}
		if !strings.Contains(string(data), "[PRINCIPAL]") {
			t.Error("expected [PRINCIPAL] prefix in log")
		}
	})

	t.Run("nil file does not panic", func(t *testing.T) {
		logger := &DispatchLogger{}
		logger.Log("should not panic")
		if err := logger.Close(); err != nil {
			t.Errorf("Close should return nil for empty logger: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// DispatchCleanup
// ---------------------------------------------------------------------------

func TestCov_DispatchCleanup_RunOnce(t *testing.T) {
	dir := t.TempDir()
	dc := NewDispatchCleanup(42, dir, nil) // nil ghClient to avoid real gh calls

	// First run should work
	dc.Run()

	// Second run should be no-op (idempotent)
	dc.Run()

	// Verify done flag
	dc.mu.Lock()
	if !dc.done {
		t.Error("expected done to be true after Run()")
	}
	dc.mu.Unlock()
}

func TestCov_DispatchCleanup_SkipsOnSuccess(t *testing.T) {
	dir := t.TempDir()
	resultDir := filepath.Join(dir, ".ai", "results")
	os.MkdirAll(resultDir, 0700)

	// Write a success result
	result := &IssueResult{
		IssueID: "42",
		Status:  "success",
	}
	data, _ := json.Marshal(result)
	os.WriteFile(filepath.Join(resultDir, "issue-42.json"), data, 0600)

	dc := NewDispatchCleanup(42, dir, nil)
	dc.Run() // Should return early because result is success
}

// ---------------------------------------------------------------------------
// SaveTicketFile / LoadTicketFile / CleanupTicketFile
// ---------------------------------------------------------------------------

func TestCov_TicketFileLifecycle(t *testing.T) {
	dir := t.TempDir()
	body := "# [feat] test task\n- Repo: backend"

	// Save
	path, err := SaveTicketFile(dir, 42, body)
	if err != nil {
		t.Fatalf("SaveTicketFile failed: %v", err)
	}
	if !strings.Contains(path, "ticket-42.md") {
		t.Errorf("expected ticket-42.md in path, got %q", path)
	}

	// Load
	loaded, err := LoadTicketFile(dir, 42)
	if err != nil {
		t.Fatalf("LoadTicketFile failed: %v", err)
	}
	if loaded != body {
		t.Errorf("loaded content mismatch: got %q, want %q", loaded, body)
	}

	// Cleanup
	err = CleanupTicketFile(dir, 42)
	if err != nil {
		t.Fatalf("CleanupTicketFile failed: %v", err)
	}

	// Verify deleted
	_, err = LoadTicketFile(dir, 42)
	if err == nil {
		t.Error("expected error after cleanup")
	}

	// Cleanup again should not error
	err = CleanupTicketFile(dir, 42)
	if err != nil {
		t.Errorf("second CleanupTicketFile should not error: %v", err)
	}
}
