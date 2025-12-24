package kickoff

import (
	"testing"
)

func TestOutputParser_ParseSTEP3(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		expectStart   bool
		expectedIssue int
	}{
		{
			name:          "standard STEP-3 format",
			line:          "[PRINCIPAL] 10:43:45 | STEP-3    | 派工給 Worker (issue #42, repo: backend)...",
			expectStart:   true,
			expectedIssue: 42,
		},
		{
			name:          "STEP-3 with different spacing",
			line:          "[PRINCIPAL] 10:43:45 | STEP-3 | issue #123",
			expectStart:   true,
			expectedIssue: 123,
		},
		{
			name:          "STEP-3 with large issue number",
			line:          "[PRINCIPAL] 10:43:45 | STEP-3    | Working on issue #9999",
			expectStart:   true,
			expectedIssue: 9999,
		},
		{
			name:        "STEP-4 should not trigger start",
			line:        "[PRINCIPAL] 10:44:30 | STEP-4    | ✓ Worker 成功，PR: #123",
			expectStart: false,
		},
		{
			name:        "STEP-3 without issue number",
			line:        "[PRINCIPAL] 10:43:45 | STEP-3    | Starting work...",
			expectStart: false,
		},
		{
			name:        "issue number in non-STEP-3 line",
			line:        "[PRINCIPAL] 10:43:45 | STEP-1    | Found issue #42",
			expectStart: false,
		},
		{
			name:        "empty line",
			line:        "",
			expectStart: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var startCalled bool
			var receivedIssue int

			parser := NewOutputParser(
				func(issueID int) {
					startCalled = true
					receivedIssue = issueID
				},
				func() {},
			)

			parser.Parse(tt.line)

			if startCalled != tt.expectStart {
				t.Errorf("Expected start called=%v, got %v", tt.expectStart, startCalled)
			}

			if tt.expectStart && receivedIssue != tt.expectedIssue {
				t.Errorf("Expected issue %d, got %d", tt.expectedIssue, receivedIssue)
			}
		})
	}
}

func TestOutputParser_ParseSTEP4(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		expectEnd bool
	}{
		{
			name:      "standard STEP-4 format",
			line:      "[PRINCIPAL] 10:44:30 | STEP-4    | ✓ Worker 成功，PR: #123",
			expectEnd: true,
		},
		{
			name:      "STEP-4 with different content",
			line:      "[PRINCIPAL] 10:44:30 | STEP-4 | Worker completed",
			expectEnd: true,
		},
		{
			name:      "STEP-3 should not trigger end",
			line:      "[PRINCIPAL] 10:43:45 | STEP-3    | 派工給 Worker (issue #42)",
			expectEnd: false,
		},
		{
			name:      "STEP-5 should not trigger end",
			line:      "[PRINCIPAL] 10:45:00 | STEP-5    | Review complete",
			expectEnd: false,
		},
		{
			name:      "empty line",
			line:      "",
			expectEnd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var endCalled bool

			parser := NewOutputParser(
				func(issueID int) {},
				func() {
					endCalled = true
				},
			)

			parser.Parse(tt.line)

			if endCalled != tt.expectEnd {
				t.Errorf("Expected end called=%v, got %v", tt.expectEnd, endCalled)
			}
		})
	}
}

func TestOutputParser_NilCallbacks(t *testing.T) {
	// Should not panic with nil callbacks
	parser := NewOutputParser(nil, nil)

	// These should not panic
	parser.Parse("[PRINCIPAL] 10:43:45 | STEP-3    | issue #42")
	parser.Parse("[PRINCIPAL] 10:44:30 | STEP-4    | done")
}

func TestOutputParser_MultipleIssues(t *testing.T) {
	var issues []int

	parser := NewOutputParser(
		func(issueID int) {
			issues = append(issues, issueID)
		},
		func() {},
	)

	// Parse multiple STEP-3 lines
	parser.Parse("[PRINCIPAL] 10:43:45 | STEP-3    | issue #42")
	parser.Parse("[PRINCIPAL] 10:44:00 | STEP-3    | issue #43")
	parser.Parse("[PRINCIPAL] 10:44:15 | STEP-3    | issue #44")

	if len(issues) != 3 {
		t.Errorf("Expected 3 issues, got %d", len(issues))
	}

	expected := []int{42, 43, 44}
	for i, issue := range issues {
		if issue != expected[i] {
			t.Errorf("Expected issue %d at index %d, got %d", expected[i], i, issue)
		}
	}
}
