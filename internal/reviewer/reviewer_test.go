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

// ============================================================
// Tests migrated from test_review.py
// Property 26: Review Submodule Changes
// Validates: Requirements 21.1, 21.2, 21.3, 21.4
// ============================================================

// SubmoduleChange represents a submodule change in a PR.
type SubmoduleChange struct {
	Path     string
	OldSHA   string
	NewSHA   string
	IsPushed bool
}

// IdentifySubmoduleChanges identifies which changed files are submodule references.
//
// Property 26: Review Submodule Changes
// For any PR that includes submodule changes, the system SHALL
// identify the submodule path (Req 21.1).
func IdentifySubmoduleChanges(changedFiles, gitmodulesPaths []string) []string {
	submoduleChanges := []string{}

	for _, file := range changedFiles {
		for _, submodule := range gitmodulesPaths {
			if file == submodule {
				submoduleChanges = append(submoduleChanges, file)
				break
			}
		}
	}

	return submoduleChanges
}

// GetSubmoduleDiff gets diff for submodule commits.
//
// Property 26: Review Submodule Changes
// Fetch and display the submodule's commit diff (Req 21.2).
func GetSubmoduleDiff(submodulePath, oldSHA, newSHA string) string {
	return "Submodule " + submodulePath + " changed from " + oldSHA[:7] + " to " + newSHA[:7]
}

// CheckSubmodulePushed checks if submodule commit is pushed to remote.
//
// Property 26: Review Submodule Changes
// Warn if the reference points to an unpushed commit (Req 21.3, 21.4).
func CheckSubmodulePushed(submodulePath, newSHA string, remoteSHAs []string) (bool, string) {
	for _, sha := range remoteSHAs {
		if sha == newSHA {
			return true, ""
		}
	}
	// Truncate SHA to 7 chars if longer, otherwise use full SHA
	shaDisplay := newSHA
	if len(newSHA) > 7 {
		shaDisplay = newSHA[:7]
	}
	return false, "WARNING: Submodule '" + submodulePath + "' references unpushed commit " + shaDisplay
}

// ReviewSubmoduleChanges reviews all submodule changes.
func ReviewSubmoduleChanges(changes []SubmoduleChange) ([]string, []string) {
	var diffs []string
	var warnings []string

	for _, change := range changes {
		// Get diff (Req 21.2)
		diff := GetSubmoduleDiff(change.Path, change.OldSHA, change.NewSHA)
		diffs = append(diffs, diff)

		// Check if pushed (Req 21.3, 21.4)
		if !change.IsPushed {
			warnings = append(warnings, "WARNING: Submodule '"+change.Path+"' references unpushed commit")
		}
	}

	return diffs, warnings
}

// TestReviewSubmoduleChanges tests review submodule changes.
// Property 26: Review Submodule Changes
func TestReviewSubmoduleChanges(t *testing.T) {
	t.Run("identify_submodule_changes", func(t *testing.T) {
		// Test identifying submodule changes (Req 21.1)
		changedFiles := []string{"backend", "README.md", "frontend"}
		gitmodulesPaths := []string{"backend", "frontend"}

		submoduleChanges := IdentifySubmoduleChanges(changedFiles, gitmodulesPaths)

		if !contains(submoduleChanges, "backend") {
			t.Error("submodule changes should include 'backend'")
		}
		if !contains(submoduleChanges, "frontend") {
			t.Error("submodule changes should include 'frontend'")
		}
		if contains(submoduleChanges, "README.md") {
			t.Error("submodule changes should NOT include 'README.md'")
		}
	})

	t.Run("get_submodule_diff", func(t *testing.T) {
		// Test getting submodule diff (Req 21.2)
		diff := GetSubmoduleDiff("backend", "abc1234567890", "def1234567890")

		if !strings.Contains(diff, "backend") {
			t.Error("diff should contain submodule path 'backend'")
		}
		if !strings.Contains(diff, "abc1234") {
			t.Error("diff should contain old SHA prefix")
		}
		if !strings.Contains(diff, "def1234") {
			t.Error("diff should contain new SHA prefix")
		}
	})

	t.Run("check_pushed_commit", func(t *testing.T) {
		// Test checking pushed commit (Req 21.3)
		isPushed, warning := CheckSubmodulePushed("backend", "abc123", []string{"abc123", "def456"})

		if !isPushed {
			t.Error("should report as pushed when SHA exists in remote")
		}
		if warning != "" {
			t.Error("should have no warning when pushed")
		}
	})

	t.Run("check_unpushed_commit", func(t *testing.T) {
		// Test checking unpushed commit (Req 21.4)
		isPushed, warning := CheckSubmodulePushed("backend", "xyz789", []string{"abc123", "def456"})

		if isPushed {
			t.Error("should report as NOT pushed when SHA not in remote")
		}
		if !strings.Contains(warning, "WARNING") {
			t.Error("should have WARNING in message")
		}
		if !strings.Contains(warning, "unpushed") {
			t.Error("should mention 'unpushed' in warning")
		}
	})
}

// TestReviewAllChanges tests reviewing all submodule changes.
func TestReviewAllChanges(t *testing.T) {
	t.Run("review_multiple_changes", func(t *testing.T) {
		changes := []SubmoduleChange{
			{Path: "backend", OldSHA: "abc1234567890", NewSHA: "def4567890123", IsPushed: true},
			{Path: "frontend", OldSHA: "1112223334445", NewSHA: "3334445556667", IsPushed: false},
		}

		diffs, warnings := ReviewSubmoduleChanges(changes)

		if len(diffs) != 2 {
			t.Errorf("should have 2 diffs, got %d", len(diffs))
		}
		if len(warnings) != 1 {
			t.Errorf("should have 1 warning, got %d", len(warnings))
		}
		if !strings.Contains(warnings[0], "frontend") {
			t.Error("warning should mention 'frontend'")
		}
	})

	t.Run("review_no_changes", func(t *testing.T) {
		changes := []SubmoduleChange{}

		diffs, warnings := ReviewSubmoduleChanges(changes)

		if len(diffs) != 0 {
			t.Errorf("should have 0 diffs, got %d", len(diffs))
		}
		if len(warnings) != 0 {
			t.Errorf("should have 0 warnings, got %d", len(warnings))
		}
	})

	t.Run("review_all_pushed", func(t *testing.T) {
		changes := []SubmoduleChange{
			{Path: "backend", OldSHA: "abc1234567890", NewSHA: "def4567890123", IsPushed: true},
			{Path: "frontend", OldSHA: "1112223334445", NewSHA: "3334445556667", IsPushed: true},
		}

		diffs, warnings := ReviewSubmoduleChanges(changes)

		if len(diffs) != 2 {
			t.Errorf("should have 2 diffs, got %d", len(diffs))
		}
		if len(warnings) != 0 {
			t.Errorf("should have 0 warnings, got %d", len(warnings))
		}
	})
}

// TestSubmoduleChangeStruct tests SubmoduleChange struct.
func TestSubmoduleChangeStruct(t *testing.T) {
	t.Run("change_has_path", func(t *testing.T) {
		change := SubmoduleChange{Path: "backend", OldSHA: "abc", NewSHA: "def"}

		if change.Path != "backend" {
			t.Errorf("Path should be 'backend', got %q", change.Path)
		}
	})

	t.Run("change_has_shas", func(t *testing.T) {
		change := SubmoduleChange{Path: "backend", OldSHA: "abc123", NewSHA: "def456"}

		if change.OldSHA != "abc123" {
			t.Errorf("OldSHA should be 'abc123', got %q", change.OldSHA)
		}
		if change.NewSHA != "def456" {
			t.Errorf("NewSHA should be 'def456', got %q", change.NewSHA)
		}
	})

	t.Run("change_default_pushed", func(t *testing.T) {
		// In Go, bool defaults to false, so we need to explicitly set true
		change := SubmoduleChange{Path: "backend", OldSHA: "abc", NewSHA: "def", IsPushed: true}

		if !change.IsPushed {
			t.Error("IsPushed should be true when explicitly set")
		}
	})

	t.Run("change_unpushed", func(t *testing.T) {
		change := SubmoduleChange{Path: "backend", OldSHA: "abc", NewSHA: "def", IsPushed: false}

		if change.IsPushed {
			t.Error("IsPushed should be false")
		}
	})
}

// TestIdentifyChanges tests identifying submodule changes.
func TestIdentifyChanges(t *testing.T) {
	t.Run("no_submodule_changes", func(t *testing.T) {
		changedFiles := []string{"README.md", "main.go"}
		gitmodulesPaths := []string{"backend", "frontend"}

		submoduleChanges := IdentifySubmoduleChanges(changedFiles, gitmodulesPaths)

		if len(submoduleChanges) != 0 {
			t.Errorf("should have 0 submodule changes, got %d", len(submoduleChanges))
		}
	})

	t.Run("all_submodule_changes", func(t *testing.T) {
		changedFiles := []string{"backend", "frontend"}
		gitmodulesPaths := []string{"backend", "frontend"}

		submoduleChanges := IdentifySubmoduleChanges(changedFiles, gitmodulesPaths)

		if len(submoduleChanges) != 2 {
			t.Errorf("should have 2 submodule changes, got %d", len(submoduleChanges))
		}
	})

	t.Run("empty_changed_files", func(t *testing.T) {
		changedFiles := []string{}
		gitmodulesPaths := []string{"backend"}

		submoduleChanges := IdentifySubmoduleChanges(changedFiles, gitmodulesPaths)

		if len(submoduleChanges) != 0 {
			t.Errorf("should have 0 submodule changes, got %d", len(submoduleChanges))
		}
	})
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
