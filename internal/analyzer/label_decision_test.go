package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// LabelDecision is what Decide decides from the labels of the tickets in
// hand, and nil when there is none: Decide then picks a ticket. Read-only,
// it decides the same and changes nothing: no label, no attempt file. The
// engine's shadow decides so (internal/engine/shadow.go).
func TestLabelDecision_ReadOnlyDecidesTheSameAndChangesNothing(t *testing.T) {
	labels := DefaultLabels()
	issue := func(n int) Issue { return Issue{Number: n, Body: "", State: "OPEN"} }
	tests := []struct {
		name  string
		setup func(m *MockGitHubClient, a *Analyzer)
	}{
		{"a Worker at work", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.InProgress] = []Issue{issue(5)}
		}},
		{"a review", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.PRReady] = []Issue{issue(5)}
			m.PRByBranch["feat/ai-issue-5"] = 30
		}},
		{"a review whose pull request is not found", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.PRReady] = []Issue{issue(5)}
		}},
		{"a conflict to resolve", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.PRReady] = []Issue{issue(5)}
			m.PRByBranch["feat/ai-issue-5"] = 30
			m.PRMergeableStatus[30] = "CONFLICTING"
		}},
		{"a conflict resolved as often as it may be", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.PRReady] = []Issue{issue(5)}
			m.PRByBranch["feat/ai-issue-5"] = 30
			m.PRMergeableStatus[30] = "CONFLICTING"
			a.AttemptsOf = func(kind string, pr int) (int, bool) { return 3, kind == "conflict" }
		}},
		{"a review tried again", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.ReviewFailed] = []Issue{issue(5)}
			m.PRByBranch["feat/ai-issue-5"] = 30
		}},
		{"a review tried as often as it may be", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.ReviewFailed] = []Issue{issue(5)}
			m.PRByBranch["feat/ai-issue-5"] = 30
			a.AttemptsOf = func(kind string, pr int) (int, bool) { return 3, kind == "review" }
		}},
		{"a conflict whose pull request is not found", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.MergeConflict] = []Issue{issue(5)}
		}},
		{"a ticket that needs a person", func(m *MockGitHubClient, a *Analyzer) {
			m.IssuesByLabel[labels.NeedsHumanReview] = []Issue{issue(5)}
		}},
		{"none in hand", func(m *MockGitHubClient, a *Analyzer) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// As the Principal decides: the decision, and what it changes.
			m := NewMockGitHubClient()
			a := newTestAnalyzer(t.TempDir(), defaultTestConfig(), m)
			tt.setup(m, a)
			want, err := a.LabelDecision(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			// Read-only, in a project of its own.
			m = NewMockGitHubClient()
			root := t.TempDir()
			a = newTestAnalyzer(root, defaultTestConfig(), m)
			tt.setup(m, a)
			a.ReadOnly = true
			got, err := a.LabelDecision(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if (want == nil) != (got == nil) || (want != nil && *want != *got) {
				t.Errorf("read-only it decides %+v, otherwise %+v", got, want)
			}
			if len(m.AddedLabels) != 0 || len(m.RemovedLabels) != 0 {
				t.Errorf("read-only it changed labels: +%v -%v", m.AddedLabels, m.RemovedLabels)
			}
			if entries, _ := os.ReadDir(filepath.Join(root, ".ai", "state", "attempts")); len(entries) != 0 {
				t.Errorf("read-only it counted attempts: %v", entries)
			}
		})
	}
}

// The counts AttemptsOf gives are the ones the limits are checked against;
// a count it does not have is read from the file.
func TestAttemptsOf(t *testing.T) {
	a := New(t.TempDir(), defaultTestConfig())
	if err := a.incrementAttempts("review", 30); err != nil {
		t.Fatal(err)
	}
	if got := a.getAttempts("review", 30); got != 1 {
		t.Fatalf("from the file: %d", got)
	}
	a.AttemptsOf = func(kind string, pr int) (int, bool) { return 7, kind == "conflict" }
	if got := a.getAttempts("conflict", 30); got != 7 {
		t.Errorf("given: %d", got)
	}
	if got := a.getAttempts("review", 30); got != 1 {
		t.Errorf("not given, from the file: %d", got)
	}
}

// Decide is Precheck, LabelDecision, then the next ticket: the decision on
// the labels comes before any ticket is picked.
func TestDecide_LabelsBeforeTheNextTicket(t *testing.T) {
	m := NewMockGitHubClient()
	m.IssuesByLabel[DefaultLabels().NeedsHumanReview] = []Issue{{Number: 5, State: "OPEN"}}
	m.PendingIssues = []Issue{{Number: 6, State: "OPEN"}}
	a := newTestAnalyzer(t.TempDir(), defaultTestConfig(), m)
	d, err := a.Decide(context.Background())
	if err != nil || d.NextAction != ActionNone || d.IssueNumber != 5 || d.ExitReason != ReasonNeedsHumanReview {
		t.Errorf("Decide: %+v, %v", d, err)
	}
}
