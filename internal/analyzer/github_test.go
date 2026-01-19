package analyzer

import (
	"testing"
	"time"
)

func TestIssue_HasLabel(t *testing.T) {
	issue := Issue{
		Number: 1,
		Body:   "test",
		Labels: []Label{
			{Name: "bug"},
			{Name: "in-progress"},
			{Name: "priority-high"},
		},
	}

	tests := []struct {
		label string
		want  bool
	}{
		{"bug", true},
		{"in-progress", true},
		{"priority-high", true},
		{"feature", false},
		{"", false},
		{"BUG", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := issue.HasLabel(tt.label)
			if got != tt.want {
				t.Errorf("HasLabel(%q) = %v, want %v", tt.label, got, tt.want)
			}
		})
	}
}

func TestIssue_HasLabel_Empty(t *testing.T) {
	issue := Issue{
		Number: 1,
		Body:   "test",
		Labels: []Label{},
	}

	if issue.HasLabel("any") {
		t.Error("HasLabel() should return false for empty labels")
	}
}

func TestNewGitHubClient(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{"zero timeout uses default", 0, 30 * time.Second},
		{"custom timeout", 60 * time.Second, 60 * time.Second},
		{"short timeout", 5 * time.Second, 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewGitHubClient(tt.timeout)
			if client.Timeout != tt.want {
				t.Errorf("Timeout = %v, want %v", client.Timeout, tt.want)
			}
		})
	}
}

func TestExtractPRNumber_Extended(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		// Full GitHub URLs
		{"full URL", "https://github.com/owner/repo/pull/123", 123},
		{"full URL with trailing path", "https://github.com/owner/repo/pull/456/files", 456},
		{"full URL with query", "https://github.com/owner/repo/pull/789?diff=split", 789},
		{"full URL in text", "See https://github.com/owner/repo/pull/101 for details", 101},

		// Relative URLs
		{"relative URL", "/pull/200", 200},
		{"relative URL in text", "Check /pull/201 please", 201},

		// PR references
		{"PR hash ref", "PR #300 is ready", 300},
		{"PR no space", "PR#301", 301},
		{"lowercase pr", "pr #302", 302},
		{"Pull request ref", "pull request #303", 303},
		{"PULL REQUEST ref", "PULL REQUEST #304", 304},

		// No match cases
		{"no PR", "no pull request here", 0},
		{"issue reference", "Fixes #123", 0}, // Issue refs should not match
		{"empty string", "", 0},
		{"just number", "123", 0},
		{"pulls list endpoint", "/pulls/123", 0}, // Should not match /pulls/ (list endpoint)

		// Complex cases
		{"multiple URLs takes first", "https://github.com/owner/repo/pull/500 and https://github.com/owner/repo/pull/501", 500},
		{"PR in markdown", "[PR #600](https://github.com/owner/repo/pull/600)", 600},
		{"issue body with PR", "This PR fixes issue #10.\n\nSee https://github.com/owner/repo/pull/700", 700},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPRNumber(tt.input)
			if got != tt.want {
				t.Errorf("ExtractPRNumber(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGitHubClientInterface(t *testing.T) {
	// Test that GitHubClient implements GitHubClientInterface
	var _ GitHubClientInterface = (*GitHubClient)(nil)
}
