package reviewer

import (
	"strings"
	"testing"
)

func TestSha256_16(t *testing.T) {
	result := sha256_16("test data")

	if len(result) != 16 {
		t.Errorf("sha256_16() length = %d, want 16", len(result))
	}

	// Same input should produce same output
	result2 := sha256_16("test data")
	if result != result2 {
		t.Error("sha256_16() should be deterministic")
	}

	// Different input should produce different output
	result3 := sha256_16("different data")
	if result == result3 {
		t.Error("sha256_16() should produce different hashes for different inputs")
	}
}

func TestReviewContextFormatOutput(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:           123,
		IssueNumber:        456,
		PrincipalSessionID: "principal-test-123",
		CIStatus:           "passed",
		DiffHash:           "abc123",
		DiffBytes:          1000,
		ReviewDir:          "/path/to/review",
		WorktreePath:       "/path/to/worktree",
		Diff:               "diff content here",
		IssueJSON:          `{"title": "Test Issue"}`,
		CommitsJSON:        "- abc1234 Test commit",
	}

	output := rc.FormatOutput()

	// Check header
	if !strings.Contains(output, "AWK PR REVIEW CONTEXT") {
		t.Error("output should contain header")
	}

	// Check PR number
	if !strings.Contains(output, "PR_NUMBER: 123") {
		t.Error("output should contain PR number")
	}

	// Check issue number
	if !strings.Contains(output, "ISSUE_NUMBER: 456") {
		t.Error("output should contain issue number")
	}

	// Check session ID
	if !strings.Contains(output, "principal-test-123") {
		t.Error("output should contain session ID")
	}

	// Check CI status
	if !strings.Contains(output, "CI_STATUS: passed") {
		t.Error("output should contain CI status")
	}

	// Check diff
	if !strings.Contains(output, "diff content here") {
		t.Error("output should contain diff")
	}
}

func TestReviewContextToJSON(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    123,
		IssueNumber: 456,
		CIStatus:    "passed",
	}

	jsonStr, err := rc.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	if !strings.Contains(jsonStr, `"pr_number": 123`) {
		t.Error("JSON should contain pr_number")
	}

	if !strings.Contains(jsonStr, `"ci_status": "passed"`) {
		t.Error("JSON should contain ci_status")
	}
}

func TestReviewContextFormatOutputWithMissingDiff(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    123,
		IssueNumber: 456,
		Diff:        "",
	}

	output := rc.FormatOutput()

	if !strings.Contains(output, "ERROR: Diff not available") {
		t.Error("output should indicate diff not available when empty")
	}
}
