package analyzer

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestScopeFromEnv(t *testing.T) {
	for value, want := range map[string]int{"": 0, "  ": 0, "12": 12, " 12 ": 12} {
		t.Setenv(ScopeEnv, value)
		got, err := ScopeFromEnv()
		if err != nil || got != want {
			t.Errorf("%s=%q: ScopeFromEnv() = %d, %v; want %d", ScopeEnv, value, got, err, want)
		}
	}
	// A run meant for one ticket must not become a run for all of them.
	for _, value := range []string{"abc", "0", "-3", "1.5", "12 13", "#12"} {
		t.Setenv(ScopeEnv, value)
		got, err := ScopeFromEnv()
		if err == nil || got != 0 || !strings.Contains(err.Error(), ScopeEnv) {
			t.Errorf("%s=%q: ScopeFromEnv() = %d, %v; want an error that names the variable", ScopeEnv, value, got, err)
		}
	}
}

// decideFor runs one decision limited to an issue.
func decideFor(t *testing.T, stateRoot string, mock *MockGitHubClient, issue int) *Decision {
	t.Helper()
	a := newTestAnalyzer(stateRoot, defaultTestConfig(), mock)
	a.LimitTo(issue)
	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	return decision
}

// untouched fails the test when the run changed an issue other than its own.
func untouched(t *testing.T, mock *MockGitHubClient, own int) {
	t.Helper()
	for issue := range mock.AddedLabels {
		if issue != own {
			t.Errorf("a label was added to #%d: %v", issue, mock.AddedLabels[issue])
		}
	}
	for issue := range mock.RemovedLabels {
		if issue != own {
			t.Errorf("a label was removed from #%d: %v", issue, mock.RemovedLabels[issue])
		}
	}
	for issue := range mock.Comments {
		if issue != own {
			t.Errorf("a comment was left on #%d: %v", issue, mock.Comments[issue])
		}
	}
	for _, issue := range append(append([]int{}, mock.ReopenedIssues...), mock.ClosedIssues...) {
		if issue != own {
			t.Errorf("#%d was reopened or closed", issue)
		}
	}
}

// A run limited to a ticket works on that ticket, whatever else is waiting
// and whatever state the other issues are in.
func TestDecide_LimitedToOneTicket(t *testing.T) {
	labels := DefaultLabels()
	other := func(number int) Issue { return Issue{Number: number, Body: "https://github.com/owner/repo/pull/900"} }

	tests := []struct {
		name       string
		setup      func(mock *MockGitHubClient)
		wantAction string
		wantIssue  int
		wantPR     int
		wantReason string
	}{
		{
			name: "the ticket is pending, behind others",
			setup: func(mock *MockGitHubClient) {
				mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 5, Body: ticketBody()}, {Number: 7, Body: ticketBody()}}
			},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name: "another issue is in progress",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.InProgress] = []Issue{other(3)}
				mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody()}}
			},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name: "the ticket is in progress",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.InProgress] = []Issue{other(3), {Number: 5}}
			},
			wantAction: ActionCheckResult, wantIssue: 5,
		},
		{
			name: "another issue waits for its review",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.PRReady] = []Issue{other(3)}
				mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody()}}
			},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name: "the ticket waits for its review",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.PRReady] = []Issue{other(3), {Number: 5, Body: "https://github.com/owner/repo/pull/50"}}
			},
			wantAction: ActionReviewPR, wantIssue: 5, wantPR: 50,
		},
		{
			name: "another issue needs a person",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.NeedsHumanReview] = []Issue{other(3)}
				mock.IssuesByLabel[labels.WorkerFailed] = []Issue{other(4)}
				mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody()}}
			},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name: "the ticket's Worker gave up",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.WorkerFailed] = []Issue{other(4), {Number: 5}}
			},
			wantAction: ActionNone, wantIssue: 5, wantReason: ReasonWorkerFailed,
		},
		{
			name: "another ticket cannot be used",
			setup: func(mock *MockGitHubClient) {
				mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody("priority: high")}, {Number: 5, Body: ticketBody()}}
			},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name: "the ticket cannot be used",
			setup: func(mock *MockGitHubClient) {
				mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 5, Body: ticketBody("priority: high")}}
			},
			wantAction: ActionNone, wantIssue: 5, wantReason: ReasonInvalidTicket,
		},
		{
			name: "the ticket waits for an issue that is open",
			setup: func(mock *MockGitHubClient) {
				mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 5, Body: ticketBody(dependsOn(3))}}
				mock.TaskStatuses = map[int]IssueStatus{3: open, 5: open}
				mock.IssueStatuses[3] = open
			},
			wantAction: ActionNone, wantIssue: 5, wantReason: ReasonBlockedByDependencies,
		},
		{
			name: "what the ticket waits for is done",
			setup: func(mock *MockGitHubClient) {
				mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
				mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}
				mock.IssueStatuses[3] = completed
			},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name: "the ticket is done, others are not",
			setup: func(mock *MockGitHubClient) {
				mock.IssuesByLabel[labels.Task] = []Issue{{Number: 3}, {Number: 7}}
				mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 7, Body: ticketBody()}}
				mock.OpenIssueCount = 2
				mock.TaskStatuses = map[int]IssueStatus{3: open, 5: completed, 7: open}
			},
			wantAction: ActionAllComplete,
		},
		{
			name: "the ticket is open and nothing can be done about it",
			setup: func(mock *MockGitHubClient) {
				// Open, with the task label, and held out of the pending ones
				// by a label the workflow does not act on.
				mock.IssuesByLabel[labels.Task] = []Issue{{Number: 3}, {Number: 5}}
				mock.TaskStatuses = map[int]IssueStatus{3: open, 5: open}
			},
			wantAction: ActionNone, wantReason: ReasonNoActionableTasks,
		},
	}
	for _, tt := range tests {
		mock := NewMockGitHubClient()
		tt.setup(mock)

		got := decideFor(t, t.TempDir(), mock, 5)

		if got.NextAction != tt.wantAction || got.IssueNumber != tt.wantIssue || got.PRNumber != tt.wantPR || got.ExitReason != tt.wantReason {
			t.Errorf("%s: decision = %s #%d PR %d (%q), want %s #%d PR %d (%q)", tt.name,
				got.NextAction, got.IssueNumber, got.PRNumber, got.ExitReason, tt.wantAction, tt.wantIssue, tt.wantPR, tt.wantReason)
		}
		t.Run(tt.name, func(t *testing.T) { untouched(t, mock, 5) })
	}
}

// The same states, without the limit: the run follows the queue. This is what
// the limit changes.
func TestDecide_NotLimited(t *testing.T) {
	labels := DefaultLabels()

	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 5, Body: ticketBody()}}
	wantDecision(t, decide(t, t.TempDir(), mock), ActionDispatchWorker, 3, "")

	mock = NewMockGitHubClient()
	mock.IssuesByLabel[labels.NeedsHumanReview] = []Issue{{Number: 3}}
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody()}}
	wantDecision(t, decide(t, t.TempDir(), mock), ActionNone, 3, ReasonNeedsHumanReview)

	// LimitTo(0) is no limit.
	mock = NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 3, Body: ticketBody()}, {Number: 5, Body: ticketBody()}}
	a := newTestAnalyzer(t.TempDir(), defaultTestConfig(), mock)
	a.LimitTo(0)
	if a.GHClient != GitHubClientInterface(mock) {
		t.Error("LimitTo(0) changed what the analyzer sees")
	}
	got, err := a.Decide(context.Background())
	if err != nil || got.NextAction != ActionDispatchWorker || got.IssueNumber != 3 {
		t.Errorf("decision = %+v, err %v; want the first ticket of the queue", got, err)
	}
}

// What the ticket depends on is outside the run. A dependency that was closed
// without its work landing is not reopened by a run limited to another ticket:
// the ticket waits, and the run says so.
func TestDecide_LimitedRunLeavesADependencyAlone(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 3)
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}
	mock.IssueStatuses[3] = completed

	got := decideFor(t, dir, mock, 5)

	wantDecision(t, got, ActionNone, 5, ReasonBlockedByDependencies)
	untouched(t, mock, 5)
	if len(mock.ReopenedIssues) != 0 {
		t.Errorf("reopened %v", mock.ReopenedIssues)
	}

	// Without the limit, the dependency is done again first.
	mock = NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}
	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 3, "")
}

// The ticket itself, closed without its work landing, is done again: the run
// is about it.
func TestDecide_LimitedRunRedoesItsOwnTicket(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 5)
	recordWorkedOn(t, dir, 3) // another ticket in the same state, outside the run
	mock := NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: completed}

	got := decideFor(t, dir, mock, 5)

	wantDecision(t, got, ActionDispatchWorker, 5, "")
	if !reflect.DeepEqual(mock.ReopenedIssues, []int{5}) {
		t.Errorf("reopened %v, want the ticket of the run and no other", mock.ReopenedIssues)
	}
	untouched(t, mock, 5)
}

// The lists the engine reads hold the one ticket, or nothing.
func TestScopedClient(t *testing.T) {
	labels := DefaultLabels()
	mock := NewMockGitHubClient()
	mock.IssuesByLabel[labels.Task] = []Issue{{Number: 3}, {Number: 5}, {Number: 7}}
	mock.IssuesByLabel[labels.PRReady] = []Issue{{Number: 3}}
	mock.PendingIssues = []Issue{{Number: 7}, {Number: 5}}
	mock.OpenIssueCount = 3
	mock.TaskStatuses = map[int]IssueStatus{3: open, 5: open, 7: completed}
	mock.IssueStatuses[3] = open
	ctx := context.Background()

	client := &scopedClient{GitHubClientInterface: mock, issue: 5}

	if got, err := client.ListIssuesByLabel(ctx, labels.Task); err != nil || !reflect.DeepEqual(got, []Issue{{Number: 5}}) {
		t.Errorf("ListIssuesByLabel(task) = %v, %v", got, err)
	}
	if got, err := client.ListIssuesByLabel(ctx, labels.PRReady); err != nil || len(got) != 0 {
		t.Errorf("ListIssuesByLabel(pr-ready) = %v, %v; want nothing: the ticket is not among them", got, err)
	}
	if got, err := client.ListPendingIssues(ctx, labels); err != nil || !reflect.DeepEqual(got, []Issue{{Number: 5}}) {
		t.Errorf("ListPendingIssues = %v, %v", got, err)
	}
	if got, err := client.CountOpenIssues(ctx, labels.Task); err != nil || got != 1 {
		t.Errorf("CountOpenIssues = %d, %v; want 1: the ticket is open", got, err)
	}
	if got, err := client.TaskIssueStatuses(ctx, labels.Task); err != nil || !reflect.DeepEqual(got, map[int]IssueStatus{5: open}) {
		t.Errorf("TaskIssueStatuses = %v, %v", got, err)
	}
	// What is asked for by number is answered as it is.
	if got, err := client.IssueStatus(ctx, 3); err != nil || got != open {
		t.Errorf("IssueStatus(3) = %v, %v", got, err)
	}

	// The ticket closed: none open, whatever the count of the others.
	mock.IssuesByLabel[labels.Task] = []Issue{{Number: 3}, {Number: 7}}
	if got, err := client.CountOpenIssues(ctx, labels.Task); err != nil || got != 0 {
		t.Errorf("CountOpenIssues = %d, %v; want 0: the ticket is closed", got, err)
	}
	delete(mock.TaskStatuses, 5)
	if got, err := client.TaskIssueStatuses(ctx, labels.Task); err != nil || len(got) != 0 {
		t.Errorf("TaskIssueStatuses = %v, %v; want nothing: the ticket no longer carries the label", got, err)
	}

	// A failed lookup stays a failed lookup.
	mock.ListIssuesError = context.DeadlineExceeded
	if _, err := client.ListIssuesByLabel(ctx, labels.Task); err == nil {
		t.Error("ListIssuesByLabel swallowed the error")
	}
	if _, err := client.ListPendingIssues(ctx, labels); err == nil {
		t.Error("ListPendingIssues swallowed the error")
	}
	if _, err := client.CountOpenIssues(ctx, labels.Task); err == nil {
		t.Error("CountOpenIssues swallowed the error")
	}
	mock.IssueStatusesError = context.DeadlineExceeded
	if _, err := client.TaskIssueStatuses(ctx, labels.Task); err == nil {
		t.Error("TaskIssueStatuses swallowed the error")
	}
}

// Looking for a ticket that never landed, a limited run passes over one that
// is outside it and goes on to its own.
func TestFalseCompletion_PassesOverWhatIsOutsideTheRun(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 3)
	recordWorkedOn(t, dir, 5)
	mock := NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: completed}
	// The limit without the filter on the lists: the engine sees both tickets.
	a := newTestAnalyzer(dir, defaultTestConfig(), mock)
	a.only = 5

	redo, err := a.falseCompletion(context.Background(), a.newTicketLookup())

	if err != nil || redo == nil || redo.NextAction != ActionDispatchWorker || redo.IssueNumber != 5 {
		t.Fatalf("falseCompletion = %+v, %v; want the run's own ticket done again", redo, err)
	}
	if !reflect.DeepEqual(mock.ReopenedIssues, []int{5}) {
		t.Errorf("reopened %v, want only the run's own ticket", mock.ReopenedIssues)
	}
}

func TestLimit(t *testing.T) {
	a := newTestAnalyzer(t.TempDir(), defaultTestConfig(), NewMockGitHubClient())
	if a.Limit() != 0 {
		t.Errorf("Limit() = %d before any limit", a.Limit())
	}
	a.LimitTo(-4)
	if a.Limit() != 0 {
		t.Errorf("Limit() = %d after LimitTo(-4)", a.Limit())
	}
	a.LimitTo(9)
	if a.Limit() != 9 {
		t.Errorf("Limit() = %d after LimitTo(9)", a.Limit())
	}
}
