package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowStats(t *testing.T) {
	stats := &WorkflowStats{
		TotalIssues:  10,
		OpenIssues:   5,
		ClosedIssues: 5,
		InProgress:   2,
		PRReady:      1,
		WorkerFailed: 1,
		NeedsReview:  1,
	}

	str := stats.String()
	if !strings.Contains(str, "Total: 10") {
		t.Errorf("String() = %q, should contain Total: 10", str)
	}
	if !strings.Contains(str, "Open: 5") {
		t.Errorf("String() = %q, should contain Open: 5", str)
	}
}

func TestCollectGitHubStatsWithMock(t *testing.T) {
	mockStats := &WorkflowStats{
		TotalIssues:  20,
		OpenIssues:   10,
		ClosedIssues: 10,
	}

	result := CollectGitHubStatsWithMock(mockStats)

	if result.TotalIssues != 20 {
		t.Errorf("TotalIssues = %d, want 20", result.TotalIssues)
	}
}

func TestGenerateReport(t *testing.T) {
	stats := &WorkflowStats{
		TotalIssues:  10,
		OpenIssues:   3,
		ClosedIssues: 7,
		InProgress:   1,
		PRReady:      1,
		WorkerFailed: 1,
		NeedsReview:  0,
	}

	report := GenerateReport("all_tasks_complete", stats, "principal-test-123")

	if report.Content == "" {
		t.Error("report content should not be empty")
	}

	if !strings.Contains(report.Content, "AWK Workflow Report") {
		t.Error("report should contain header")
	}

	if !strings.Contains(report.Content, "principal-test-123") {
		t.Error("report should contain session ID")
	}

	if !strings.Contains(report.Content, "Total Issues") {
		t.Error("report should contain summary")
	}

	if !strings.Contains(report.Content, "all_tasks_complete") {
		t.Error("report should contain exit reason")
	}
}

func TestGenerateReportWithFailures(t *testing.T) {
	stats := &WorkflowStats{
		TotalIssues:  10,
		OpenIssues:   5,
		ClosedIssues: 5,
		WorkerFailed: 2,
		NeedsReview:  1,
	}

	report := GenerateReport("max_failures", stats, "")

	if !strings.Contains(report.Content, "Attention Required") {
		t.Error("report should contain attention required section when there are failures")
	}

	if !strings.Contains(report.Content, "2") && !strings.Contains(report.Content, "worker-failed") {
		t.Error("report should mention failed issues count")
	}
}

func TestFormatExitReasonDetails(t *testing.T) {
	tests := []struct {
		reason   string
		contains string
	}{
		{"all_tasks_complete", "All tasks completed"},
		{"user_stopped", "stopped by user"},
		{"max_loop_reached", "maximum loop count"},
		{"max_consecutive_failures", "consecutive failures"},
		{"contract_violation", "contract violation"},
		{"error_exit", "error"},
		{"max_failures", "maximum failures"},
		{"escalation_triggered", "escalation"},
		{"interrupted", "interrupted"},
		{"unknown_reason", "unknown_reason"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			result := formatExitReasonDetails(tt.reason)
			if !strings.Contains(strings.ToLower(result), strings.ToLower(tt.contains)) {
				t.Errorf("formatExitReasonDetails(%q) = %q, should contain %q", tt.reason, result, tt.contains)
			}
		})
	}
}

func TestSaveReport(t *testing.T) {
	tmpDir := t.TempDir()

	stats := &WorkflowStats{TotalIssues: 5}
	report := GenerateReport("test", stats, "test-session")

	path, err := SaveReport(tmpDir, report)
	if err != nil {
		t.Fatalf("SaveReport() error = %v", err)
	}

	if path == "" {
		t.Error("SaveReport() returned empty path")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("report file not created at %s", path)
	}

	// Verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	if !strings.Contains(string(content), "AWK Workflow Report") {
		t.Error("saved report should contain header")
	}
}

func TestFormatSummary(t *testing.T) {
	stats := &WorkflowStats{
		TotalIssues:  10,
		OpenIssues:   3,
		ClosedIssues: 7,
		WorkerFailed: 1,
	}

	summary := FormatSummary("user_stopped", stats, "/path/to/report.md")

	if !strings.Contains(summary, "AWK Workflow Stopped") {
		t.Error("summary should contain header")
	}

	if !strings.Contains(summary, "user_stopped") {
		t.Error("summary should contain exit reason")
	}

	if !strings.Contains(summary, "Total Issues: 10") {
		t.Error("summary should contain total issues")
	}

	if !strings.Contains(summary, "Attention Required") {
		t.Error("summary should contain attention required when there are failures")
	}
}

func TestValidExitReasons(t *testing.T) {
	reasons := ValidExitReasons()

	if len(reasons) == 0 {
		t.Error("ValidExitReasons() should return non-empty list")
	}

	expectedReasons := []string{
		"all_tasks_complete",
		"user_stopped",
		"error_exit",
		"max_failures",
	}

	for _, expected := range expectedReasons {
		found := false
		for _, r := range reasons {
			if r == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidExitReasons() should contain %q", expected)
		}
	}
}

func TestIsValidExitReason(t *testing.T) {
	tests := []struct {
		reason string
		valid  bool
	}{
		{"all_tasks_complete", true},
		{"user_stopped", true},
		{"error_exit", true},
		{"invalid_reason", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			if got := IsValidExitReason(tt.reason); got != tt.valid {
				t.Errorf("IsValidExitReason(%q) = %v, want %v", tt.reason, got, tt.valid)
			}
		})
	}
}

func TestCleanupStateFiles(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)

	// Create test files
	loopCountPath := filepath.Join(stateDir, "loop_count")
	failuresPath := filepath.Join(stateDir, "consecutive_failures")

	os.WriteFile(loopCountPath, []byte("10"), 0644)
	os.WriteFile(failuresPath, []byte("3"), 0644)

	// Verify files exist
	if _, err := os.Stat(loopCountPath); os.IsNotExist(err) {
		t.Fatal("loop_count should exist before cleanup")
	}

	// Cleanup
	cleanupStateFiles(tmpDir)

	// Verify files removed
	if _, err := os.Stat(loopCountPath); !os.IsNotExist(err) {
		t.Error("loop_count should be removed after cleanup")
	}
	if _, err := os.Stat(failuresPath); !os.IsNotExist(err) {
		t.Error("consecutive_failures should be removed after cleanup")
	}
}
