package analyzer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// pathsBody is a schema 2 ticket for the root with the given paths.
func pathsBody(paths string, more ...string) string {
	body := "## Summary\nwork\n\n```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [" + paths + "]\nrisk: low\n"
	for _, line := range more {
		body += line + "\n"
	}
	return body + "```\n"
}

// candidateCases are the situations a pick is made in: each makes the GitHub
// of one, afresh, and the state folder it is made in.
var candidateCases = []struct {
	name  string
	setup func(t *testing.T, dir string) *MockGitHubClient
}{
	{"three that can start", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 9, Body: ticketBody()}, {Number: 3, Body: ticketBody()}, {Number: 5, Body: ticketBody()}}
		m.OpenIssueCount = 3
		return m
	}},
	{"the lowest waits", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 3, Body: ticketBody(dependsOn(20))}, {Number: 5, Body: ticketBody()}, {Number: 9, Body: ticketBody()}}
		m.OpenIssueCount = 4
		m.TaskStatuses = map[int]IssueStatus{3: open, 5: open, 9: open, 20: open}
		return m
	}},
	{"every one waits", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 3, Body: ticketBody(dependsOn(20))}, {Number: 5, Body: ticketBody(dependsOn(3))}}
		m.OpenIssueCount = 3
		m.TaskStatuses = map[int]IssueStatus{3: open, 5: open, 20: open}
		return m
	}},
	{"one that cannot be used, between two", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 4, Body: ticketBody("priority: high")}, {Number: 5, Body: ticketBody()}}
		m.OpenIssueCount = 3
		return m
	}},
	{"one that cannot be used, first", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 2, Body: ticketBodyFor("web")}, {Number: 3, Body: ticketBody()}}
		m.OpenIssueCount = 2
		return m
	}},
	{"one whose pull request is merged", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 3, Body: ticketBody() + "\nPR: #40\n"}, {Number: 5, Body: ticketBody()}}
		m.PRByBranch["feat/ai-issue-3"] = 40
		m.PRMerged[40] = true
		m.OpenIssueCount = 2
		return m
	}},
	{"a dependency closed without landing", func(t *testing.T, dir string) *MockGitHubClient {
		recordWorkedOn(t, dir, 2)
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(2))}, {Number: 7, Body: ticketBody()}}
		m.OpenIssueCount = 2
		m.TaskStatuses = map[int]IssueStatus{2: completed, 5: open, 7: open}
		return m
	}},
	{"a dependency that is not there", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(99))}}
		m.OpenIssueCount = 1
		m.TaskStatuses = map[int]IssueStatus{5: open}
		return m
	}},
	{"nothing open", func(t *testing.T, dir string) *MockGitHubClient {
		return NewMockGitHubClient()
	}},
	{"nothing pending, one open", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.OpenIssueCount = 1
		return m
	}},
	{"the list cannot be read", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssuesError = errors.New("HTTP 502")
		m.OpenIssueCount = 1
		return m
	}},
	{"a dependency whose state cannot be read", func(t *testing.T, dir string) *MockGitHubClient {
		m := NewMockGitHubClient()
		m.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
		m.OpenIssueCount = 1
		m.IssueStatusesError = errors.New("HTTP 502")
		m.IssueStatusError = errors.New("HTTP 502")
		return m
	}},
}

// changes is what a pick changed on GitHub.
func changes(m *MockGitHubClient) map[string]any {
	return map[string]any{"closed": m.ClosedIssues, "reopened": m.ReopenedIssues, "comments": m.Comments, "added": m.AddedLabels, "removed": m.RemovedLabels}
}

// With one asked for and nothing passed over, Candidates is NextTicket: the
// same decision, and the same changes on GitHub.
func TestCandidates_OneIsNextTicket(t *testing.T) {
	for _, tc := range candidateCases {
		dirA, dirB := t.TempDir(), t.TempDir()
		mockA, mockB := tc.setup(t, dirA), tc.setup(t, dirB)

		want, err := newTestAnalyzer(dirA, defaultTestConfig(), mockA).NextTicket(context.Background())
		if err != nil {
			t.Fatalf("%s: NextTicket: %v", tc.name, err)
		}
		picks, err := newTestAnalyzer(dirB, defaultTestConfig(), mockB).Candidates(context.Background(), 1, nil)
		if err != nil {
			t.Fatalf("%s: Candidates: %v", tc.name, err)
		}

		got := picks.Then
		if len(picks.Tickets) > 0 {
			got = picks.Tickets[0]
			if len(picks.Tickets) > 1 || picks.Then != nil {
				t.Errorf("%s: one was asked for, and Candidates found %d and then %+v", tc.name, len(picks.Tickets), picks.Then)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Candidates = %+v, NextTicket = %+v", tc.name, got, want)
		}
		if len(picks.Skipped) != 0 {
			t.Errorf("%s: passed over %v with nothing to pass over", tc.name, picks.Skipped)
		}
		if a, b := changes(mockA), changes(mockB); !reflect.DeepEqual(a, b) {
			t.Errorf("%s: Candidates changed %v on GitHub, NextTicket %v", tc.name, b, a)
		}
	}
}

func pickedIssues(p Picks) []int {
	var issues []int
	for _, d := range p.Tickets {
		if d.NextAction != ActionDispatchWorker {
			return append(issues, -1)
		}
		issues = append(issues, d.IssueNumber)
	}
	return issues
}

// Several are picked lowest number first, past the ones that wait.
func TestCandidates_Several(t *testing.T) {
	for _, tt := range []struct {
		name     string
		max      int
		want     []int
		thenIs   string // the Then's exit reason, or its action
		thenOf   int
		comments []int
	}{
		{"three that can start", 3, []int{3, 5, 9}, "", 0, nil},
		{"three that can start", 2, []int{3, 5}, "", 0, nil},
		{"three that can start", 10, []int{3, 5, 9}, "", 0, nil},
		{"the lowest waits", 3, []int{5, 9}, "", 0, nil},
		{"every one waits", 3, nil, ReasonBlockedByDependencies, 3, nil},
		// It stops at the one it hands to a person: the one after it is not
		// looked at, nor anything changed for it.
		{"one that cannot be used, between two", 3, []int{3}, ReasonInvalidTicket, 4, []int{4}},
		{"one that cannot be used, between two", 1, []int{3}, "", 0, nil},
		{"a dependency closed without landing", 3, nil, ActionDispatchWorker, 2, nil},
		{"nothing pending, one open", 3, nil, ReasonNoActionableTasks, 0, nil},
		// Closed on the way, and not picked.
		{"one whose pull request is merged", 3, []int{5}, "", 0, nil},
	} {
		var setup func(*testing.T, string) *MockGitHubClient
		for _, tc := range candidateCases {
			if tc.name == tt.name {
				setup = tc.setup
			}
		}
		dir := t.TempDir()
		mock := setup(t, dir)

		picks, err := newTestAnalyzer(dir, defaultTestConfig(), mock).Candidates(context.Background(), tt.max, nil)

		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := pickedIssues(picks); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s, %d asked for: picked %v, want %v", tt.name, tt.max, got, tt.want)
		}
		switch {
		case tt.thenIs == "" && picks.Then != nil:
			t.Errorf("%s, %d asked for: then %+v, want nothing", tt.name, tt.max, picks.Then)
		case tt.thenIs != "" && (picks.Then == nil || (picks.Then.ExitReason != tt.thenIs && picks.Then.NextAction != tt.thenIs) || picks.Then.IssueNumber != tt.thenOf):
			t.Errorf("%s, %d asked for: then %+v, want %s #%d", tt.name, tt.max, picks.Then, tt.thenIs, tt.thenOf)
		}
		if tt.name == "one whose pull request is merged" && !reflect.DeepEqual(mock.ClosedIssues, []int{3}) {
			t.Errorf("closed %v, want #3, whose pull request is merged", mock.ClosedIssues)
		}
		var commented []int
		for issue := range mock.Comments {
			commented = append(commented, issue)
		}
		if len(commented) != len(tt.comments) || (len(tt.comments) > 0 && !reflect.DeepEqual(commented, tt.comments)) {
			t.Errorf("%s, %d asked for: comments on %v, want on %v", tt.name, tt.max, commented, tt.comments)
		}
	}
}

// What skip passes over is listed, is not a ticket that waits, and is told
// each ticket's block.
func TestCandidates_Skip(t *testing.T) {
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{
		{Number: 3, Body: pathsBody("a/**")},
		{Number: 5, Body: "## Summary\nno block\n"},
		{Number: 7, Body: pathsBody("c/**", "depends_on: [20]")},
		{Number: 9, Body: pathsBody("b/x.go")},
	}
	mock.OpenIssueCount = 5
	mock.TaskStatuses = map[int]IssueStatus{3: open, 5: open, 7: open, 9: open, 20: open}
	type seen struct {
		paths []string
		found bool
	}
	told := map[int]seen{}
	skipA := func(issue int, meta ticket.Meta, found bool) bool {
		told[issue] = seen{meta.Paths, found}
		return issue == 3
	}

	picks, err := newTestAnalyzer(t.TempDir(), defaultTestConfig(), mock).Candidates(context.Background(), 5, skipA)

	if err != nil {
		t.Fatal(err)
	}
	if got := pickedIssues(picks); !reflect.DeepEqual(got, []int{5, 9}) || !reflect.DeepEqual(picks.Skipped, []int{3}) || picks.Then != nil {
		t.Errorf("picked %v, skipped %v, then %+v; want 5 and 9, 3 passed over, nothing then", got, picks.Skipped, picks.Then)
	}
	want := map[int]seen{3: {[]string{"a/**"}, true}, 5: {nil, false}, 9: {[]string{"b/x.go"}, true}}
	if !reflect.DeepEqual(told, want) {
		t.Errorf("skip was told %+v, want %+v (not #7, which waits)", told, want)
	}

	// Every one passed over, or waiting: not blocked by dependencies, for
	// the ones passed over can start once the paths are free.
	all := func(int, ticket.Meta, bool) bool { return true }
	picks, _ = newTestAnalyzer(t.TempDir(), defaultTestConfig(), mock).Candidates(context.Background(), 5, all)
	if len(picks.Tickets) != 0 || !reflect.DeepEqual(picks.Skipped, []int{3, 5, 9}) || picks.Then != nil {
		t.Errorf("all passed over: picked %v, skipped %v, then %+v", pickedIssues(picks), picks.Skipped, picks.Then)
	}

	// Once enough are picked, the ones after are not looked at.
	told = map[int]seen{}
	picks, _ = newTestAnalyzer(t.TempDir(), defaultTestConfig(), mock).Candidates(context.Background(), 1, skipA)
	if got := pickedIssues(picks); !reflect.DeepEqual(got, []int{5}) || len(told) != 2 {
		t.Errorf("one asked for: picked %v, skip told about %v; want 5, and only 3 and 5 looked at", got, told)
	}

	if _, err := newTestAnalyzer(t.TempDir(), defaultTestConfig(), mock).Candidates(context.Background(), 0, nil); err == nil {
		t.Error("none asked for was taken")
	}
}
