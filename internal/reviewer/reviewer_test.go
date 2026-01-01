package reviewer

import (
	"strings"
	"testing"
)

func TestReviewContextFormatOutput(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:           123,
		IssueNumber:        456,
		PrincipalSessionID: "principal-test-123",
		CIStatus:           "passed",
		ReviewDir:          "/path/to/review",
		WorktreePath:       "/path/to/worktree",
		TestCommand:        "go test -v ./...",
		Ticket:             "## Acceptance Criteria\n- [ ] Feature works",
		IssueJSON:          `{"title": "Test Issue", "body": "ticket body"}`,
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

	// Check worktree path
	if !strings.Contains(output, "WORKTREE_PATH: /path/to/worktree") {
		t.Error("output should contain worktree path")
	}

	// Check test command
	if !strings.Contains(output, "TEST_COMMAND: go test -v ./...") {
		t.Error("output should contain test command")
	}

	// Check ticket is included
	if !strings.Contains(output, "Feature works") {
		t.Error("output should contain ticket content")
	}
}

func TestReviewContextToJSON(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    123,
		IssueNumber: 456,
		CIStatus:    "passed",
		TestCommand: "go test ./...",
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

	if !strings.Contains(jsonStr, `"test_command": "go test ./..."`) {
		t.Error("JSON should contain test_command")
	}
}

func TestReviewContextFormatOutputWithTicket(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    123,
		IssueNumber: 456,
		Ticket:      "This is the ticket body with acceptance criteria",
		IssueJSON:   `{"title": "Test", "body": "fallback"}`,
	}

	output := rc.FormatOutput()

	// When Ticket is set, it should be used instead of IssueJSON
	if !strings.Contains(output, "This is the ticket body") {
		t.Error("output should contain ticket body when set")
	}
}

func TestReviewContextFormatOutputFallbackToIssueJSON(t *testing.T) {
	rc := &ReviewContext{
		PRNumber:    123,
		IssueNumber: 456,
		Ticket:      "",
		IssueJSON:   `{"title": "Test", "body": "fallback body"}`,
	}

	output := rc.FormatOutput()

	// When Ticket is empty, it should fall back to IssueJSON
	if !strings.Contains(output, "fallback body") {
		t.Error("output should contain IssueJSON when Ticket is empty")
	}
}

func TestExtractIssueBody(t *testing.T) {
	tests := []struct {
		name      string
		issueJSON string
		want      string
	}{
		{
			name:      "valid JSON",
			issueJSON: `{"title": "Test", "body": "The issue body content"}`,
			want:      "The issue body content",
		},
		{
			name:      "empty body",
			issueJSON: `{"title": "Test", "body": ""}`,
			want:      "",
		},
		{
			name:      "invalid JSON",
			issueJSON: "not json",
			want:      "",
		},
		{
			name:      "error message",
			issueJSON: "ERROR: Cannot fetch issue",
			want:      "",
		},
		{
			name:      "empty string",
			issueJSON: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIssueBody(tt.issueJSON)
			if got != tt.want {
				t.Errorf("extractIssueBody() = %q, want %q", got, tt.want)
			}
		})
	}
}
