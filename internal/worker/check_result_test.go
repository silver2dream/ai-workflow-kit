package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResult(t *testing.T) {
	// Create a temp directory with test data
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test case 1: Valid result file
	resultJSON := `{
		"issue_id": "123",
		"status": "success",
		"pr_url": "https://github.com/owner/repo/pull/456"
	}`
	resultPath := filepath.Join(resultsDir, "issue-123.json")
	if err := os.WriteFile(resultPath, []byte(resultJSON), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadResult(tmpDir, 123)
	if err != nil {
		t.Errorf("LoadResult failed: %v", err)
	}
	if result.IssueID != "123" {
		t.Errorf("Expected IssueID '123', got '%s'", result.IssueID)
	}
	if result.Status != "success" {
		t.Errorf("Expected Status 'success', got '%s'", result.Status)
	}
	if result.PRURL != "https://github.com/owner/repo/pull/456" {
		t.Errorf("Expected PRURL 'https://github.com/owner/repo/pull/456', got '%s'", result.PRURL)
	}

	// Test case 2: Non-existent result file
	_, err = LoadResult(tmpDir, 999)
	if !os.IsNotExist(err) {
		t.Errorf("Expected os.IsNotExist error, got %v", err)
	}
}

func TestLoadTrace(t *testing.T) {
	tmpDir := t.TempDir()
	tracesDir := filepath.Join(tmpDir, ".ai", "state", "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Fatal(err)
	}

	traceJSON := `{
		"trace_id": "issue-123-20251228T120000Z",
		"issue_id": "123",
		"status": "running",
		"started_at": "2025-12-28T12:00:00Z",
		"worker_pid": 12345,
		"worker_start_time": 1735387200
	}`
	tracePath := filepath.Join(tracesDir, "issue-123.json")
	if err := os.WriteFile(tracePath, []byte(traceJSON), 0644); err != nil {
		t.Fatal(err)
	}

	trace, err := LoadTrace(tmpDir, 123)
	if err != nil {
		t.Errorf("LoadTrace failed: %v", err)
	}
	if trace.IssueID != "123" {
		t.Errorf("Expected IssueID '123', got '%s'", trace.IssueID)
	}
	if trace.WorkerPID != 12345 {
		t.Errorf("Expected WorkerPID 12345, got %d", trace.WorkerPID)
	}
}

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/owner/repo/pull/123", "123"},
		{"https://github.com/owner/repo/pulls/456", "456"},
		{"https://github.com/owner/repo/pull/1", "1"},
		{"", ""},
		{"https://github.com/owner/repo", ""},
		{"https://github.com/owner/repo/issues/789", ""},
	}

	for _, tt := range tests {
		result := ExtractPRNumber(tt.url)
		if result != tt.expected {
			t.Errorf("ExtractPRNumber(%q) = %q, want %q", tt.url, result, tt.expected)
		}
	}
}

func TestWriteResultAtomic(t *testing.T) {
	tmpDir := t.TempDir()

	result := &IssueResult{
		IssueID:      "123",
		Status:       "success",
		PRURL:        "https://github.com/owner/repo/pull/456",
		TimestampUTC: "2025-12-28T12:00:00Z",
	}

	if err := WriteResultAtomic(tmpDir, 123, result); err != nil {
		t.Errorf("WriteResultAtomic failed: %v", err)
	}

	// Verify the file was written
	loaded, err := LoadResult(tmpDir, 123)
	if err != nil {
		t.Errorf("LoadResult failed after write: %v", err)
	}
	if loaded.IssueID != result.IssueID {
		t.Errorf("IssueID mismatch: got %s, want %s", loaded.IssueID, result.IssueID)
	}
	if loaded.Status != result.Status {
		t.Errorf("Status mismatch: got %s, want %s", loaded.Status, result.Status)
	}
}

func TestCheckResultOutput_FormatBashOutput(t *testing.T) {
	output := &CheckResultOutput{
		Status:   "success",
		PRNumber: "123",
	}

	expected := "CHECK_RESULT_STATUS=success\nWORKER_STATUS=success\nPR_NUMBER=123\n"
	result := output.FormatBashOutput()
	if result != expected {
		t.Errorf("FormatBashOutput() = %q, want %q", result, expected)
	}

	// Test without PR number
	output2 := &CheckResultOutput{
		Status: "not_found",
	}
	expected2 := "CHECK_RESULT_STATUS=not_found\nWORKER_STATUS=not_found\nPR_NUMBER=\n"
	result2 := output2.FormatBashOutput()
	if result2 != expected2 {
		t.Errorf("FormatBashOutput() = %q, want %q", result2, expected2)
	}
}

func TestIssueInfo_HasLabel(t *testing.T) {
	info := &IssueInfo{
		Labels: []string{"bug", "in-progress", "P0"},
	}

	if !info.HasLabel("bug") {
		t.Error("HasLabel should return true for 'bug'")
	}
	if !info.HasLabel("BUG") { // Case insensitive
		t.Error("HasLabel should be case insensitive")
	}
	if !info.HasLabel("in-progress") {
		t.Error("HasLabel should return true for 'in-progress'")
	}
	if info.HasLabel("feature") {
		t.Error("HasLabel should return false for 'feature'")
	}
}

func TestIssueInfo_IsOpen(t *testing.T) {
	tests := []struct {
		state    string
		expected bool
	}{
		{"OPEN", true},
		{"open", true},
		{"Open", true},
		{"CLOSED", false},
		{"closed", false},
		{"", false},
	}

	for _, tt := range tests {
		info := &IssueInfo{State: tt.state}
		if info.IsOpen() != tt.expected {
			t.Errorf("IsOpen() for state %q = %v, want %v", tt.state, info.IsOpen(), tt.expected)
		}
	}
}

func TestReadFailCount(t *testing.T) {
	tmpDir := t.TempDir()

	// Test non-existent file
	count := ReadFailCount(tmpDir, 123)
	if count != 0 {
		t.Errorf("ReadFailCount for non-existent file = %d, want 0", count)
	}

	// Create fail count file
	runsDir := filepath.Join(tmpDir, ".ai", "runs", "issue-123")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "fail_count.txt"), []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	count = ReadFailCount(tmpDir, 123)
	if count != 2 {
		t.Errorf("ReadFailCount = %d, want 2", count)
	}
}
