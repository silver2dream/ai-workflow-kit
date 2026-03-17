package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// GenerateReport (report.go)
// ---------------------------------------------------------------------------

func TestGenerateReport_ContainsExitReason(t *testing.T) {
	stats := &WorkflowStats{TotalIssues: 5, ClosedIssues: 3, OpenIssues: 2}
	report := GenerateReport("user_stopped", stats, "session-abc")

	if !strings.Contains(report.Content, "user_stopped") {
		t.Error("report should contain exit reason")
	}
	if !strings.Contains(report.Content, "session-abc") {
		t.Error("report should contain session ID")
	}
}

func TestGenerateReport_NoSessionID(t *testing.T) {
	stats := &WorkflowStats{}
	report := GenerateReport("all_tasks_complete", stats, "")

	if !strings.Contains(report.Content, "N/A") {
		t.Error("report should contain N/A when no session ID")
	}
	if !strings.Contains(report.Content, "All tasks completed successfully") {
		t.Error("report should contain completion message")
	}
}

func TestGenerateReport_WithFailures(t *testing.T) {
	stats := &WorkflowStats{WorkerFailed: 2, NeedsReview: 1}
	report := GenerateReport("max_consecutive_failures", stats, "")

	if !strings.Contains(report.Content, "2") {
		t.Error("report should mention failed count")
	}
	if !strings.Contains(report.Content, "Attention Required") {
		t.Error("report should have attention required section")
	}
}

func TestGenerateReport_HasTimestamp(t *testing.T) {
	stats := &WorkflowStats{}
	before := time.Now().Truncate(time.Second)
	report := GenerateReport("none", stats, "")
	after := time.Now().Add(time.Second)

	if report.GeneratedAt.Before(before) || report.GeneratedAt.After(after) {
		t.Errorf("GeneratedAt = %v, should be between %v and %v",
			report.GeneratedAt, before, after)
	}
}

func TestGenerateReport_PRReady(t *testing.T) {
	stats := &WorkflowStats{PRReady: 3}
	report := GenerateReport("user_stopped", stats, "")
	if !strings.Contains(report.Content, "PRs Ready") {
		t.Error("report should mention PRs ready section when PRReady > 0")
	}
}

// ---------------------------------------------------------------------------
// formatExitReasonDetails (report.go) — via GenerateReport
// ---------------------------------------------------------------------------

func TestFormatExitReasonDetails_AllCases(t *testing.T) {
	cases := []struct {
		reason   string
		expected string
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
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			stats := &WorkflowStats{}
			report := GenerateReport(tc.reason, stats, "")
			if !strings.Contains(strings.ToLower(report.Content), strings.ToLower(tc.expected)) {
				t.Errorf("GenerateReport(%q) content should contain %q", tc.reason, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SaveReport (report.go)
// ---------------------------------------------------------------------------

func TestSaveReport_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	report := &Report{
		Content:     "# Test Report\n\nContent here.\n",
		GeneratedAt: time.Now(),
	}

	path, err := SaveReport(dir, report)
	if err != nil {
		t.Fatalf("SaveReport: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("report file not created at %s: %v", path, err)
	}
	if report.Path != path {
		t.Errorf("report.Path = %q, want %q", report.Path, path)
	}

	data, _ := os.ReadFile(path)
	if string(data) != report.Content {
		t.Error("report file content mismatch")
	}
}

func TestSaveReport_FilenameContainsTimestamp(t *testing.T) {
	dir := t.TempDir()
	report := &Report{
		Content:     "content",
		GeneratedAt: time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
	}

	path, err := SaveReport(dir, report)
	if err != nil {
		t.Fatalf("SaveReport: %v", err)
	}
	base := filepath.Base(path)
	if !strings.Contains(base, "20240315") {
		t.Errorf("filename %q should contain date 20240315", base)
	}
}

// ---------------------------------------------------------------------------
// cleanupStateFiles (stop.go)
// ---------------------------------------------------------------------------

func TestCleanupStateFiles_RemovesCounterFiles(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)

	loopCount := filepath.Join(stateDir, "loop_count")
	failures := filepath.Join(stateDir, "consecutive_failures")
	os.WriteFile(loopCount, []byte("5"), 0644)
	os.WriteFile(failures, []byte("2"), 0644)

	cleanupStateFiles(dir)

	if _, err := os.Stat(loopCount); !os.IsNotExist(err) {
		t.Error("loop_count should be removed")
	}
	if _, err := os.Stat(failures); !os.IsNotExist(err) {
		t.Error("consecutive_failures should be removed")
	}
}

func TestCleanupStateFiles_NoDirNoPanic(t *testing.T) {
	dir := t.TempDir()
	// No .ai/state dir — should not panic
	cleanupStateFiles(dir)
}

// ---------------------------------------------------------------------------
// ValidExitReasons / IsValidExitReason (stop.go)
// ---------------------------------------------------------------------------

func TestValidExitReasons_NotEmpty(t *testing.T) {
	reasons := ValidExitReasons()
	if len(reasons) == 0 {
		t.Error("ValidExitReasons should return non-empty list")
	}
}

func TestIsValidExitReason_ValidReasons(t *testing.T) {
	for _, reason := range ValidExitReasons() {
		if !IsValidExitReason(reason) {
			t.Errorf("IsValidExitReason(%q) = false, want true", reason)
		}
	}
}

func TestIsValidExitReason_InvalidReason(t *testing.T) {
	if IsValidExitReason("not_a_real_reason") {
		t.Error("IsValidExitReason(invalid) should return false")
	}
}

func TestIsValidExitReason_EmptyString(t *testing.T) {
	if IsValidExitReason("") {
		t.Error("IsValidExitReason('') should return false")
	}
}

// ---------------------------------------------------------------------------
// WorkflowStats.String (stats.go)
// ---------------------------------------------------------------------------

func TestWorkflowStats_String(t *testing.T) {
	stats := &WorkflowStats{TotalIssues: 10, OpenIssues: 3, ClosedIssues: 7}
	s := stats.String()
	if !strings.Contains(s, "10") {
		t.Errorf("String() = %q, should contain TotalIssues=10", s)
	}
	if !strings.Contains(s, "3") {
		t.Errorf("String() = %q, should contain OpenIssues=3", s)
	}
	if !strings.Contains(s, "7") {
		t.Errorf("String() = %q, should contain ClosedIssues=7", s)
	}
}

// ---------------------------------------------------------------------------
// FormatSummary (report.go)
// ---------------------------------------------------------------------------

func TestFormatSummary_ContainsExitReason(t *testing.T) {
	stats := &WorkflowStats{TotalIssues: 5, ClosedIssues: 4, OpenIssues: 1}
	summary := FormatSummary("user_stopped", stats, "/some/report.md")
	if !strings.Contains(summary, "user_stopped") {
		t.Error("FormatSummary should contain exit reason")
	}
	if !strings.Contains(summary, "5") {
		t.Error("FormatSummary should contain total issues count")
	}
}

func TestFormatSummary_WithReportPath(t *testing.T) {
	stats := &WorkflowStats{}
	summary := FormatSummary("none", stats, "/tmp/report.md")
	if !strings.Contains(summary, "/tmp/report.md") {
		t.Error("FormatSummary should contain the report path")
	}
}

func TestFormatSummary_WithNeedsReview(t *testing.T) {
	stats := &WorkflowStats{NeedsReview: 2}
	summary := FormatSummary("user_stopped", stats, "")
	if !strings.Contains(summary, "2") {
		t.Error("FormatSummary with NeedsReview should mention the count")
	}
	if !strings.Contains(summary, "Attention Required") {
		t.Error("FormatSummary with NeedsReview should mention Attention Required")
	}
}

func TestFormatSummary_BothFailedAndNeedsReview(t *testing.T) {
	stats := &WorkflowStats{WorkerFailed: 1, NeedsReview: 3}
	summary := FormatSummary("error_exit", stats, "")
	if !strings.Contains(summary, "1") {
		t.Error("FormatSummary should mention WorkerFailed count")
	}
	if !strings.Contains(summary, "3") {
		t.Error("FormatSummary should mention NeedsReview count")
	}
}
