package analyzer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var (
	open       = IssueStatus{State: "OPEN"}
	completed  = IssueStatus{State: "CLOSED", Reason: "COMPLETED"}
	notPlanned = IssueStatus{State: "CLOSED", Reason: "NOT_PLANNED"}
	// What an older GitHub record looks like: closed, no reason given.
	closedNoReason = IssueStatus{State: "CLOSED"}
)

// ticketBody is an issue body with an awk-ticket block for the root repo.
func ticketBody(lines ...string) string {
	return ticketBodyFor("root", lines...)
}

func ticketBodyFor(repo string, lines ...string) string {
	return "## Summary\nwork\n\n```yaml awk-ticket\nschema: 1\nrepo: " + repo + "\n" + strings.Join(lines, "\n") + "\n```\n"
}

func dependsOn(issues ...int) string {
	parts := make([]string, len(issues))
	for i, issue := range issues {
		parts[i] = fmt.Sprint(issue)
	}
	return "depends_on: [" + strings.Join(parts, ", ") + "]"
}

// recordWorkedOn records that the workflow dispatched a Worker for the issue,
// and that the Worker reported a pull request.
func recordWorkedOn(t *testing.T, stateRoot string, issue int) {
	t.Helper()
	recordResult(t, stateRoot, issue, `{"status":"success"}`)
}

// recordResult writes the result file a Worker leaves for an issue.
func recordResult(t *testing.T, stateRoot string, issue int, content string) {
	t.Helper()
	path := filepath.Join(stateRoot, ".ai", "results", fmt.Sprintf("issue-%d.json", issue))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decide(t *testing.T, stateRoot string, mock *MockGitHubClient) *Decision {
	t.Helper()
	decision, err := newTestAnalyzer(stateRoot, defaultTestConfig(), mock).Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	return decision
}

func wantDecision(t *testing.T, got *Decision, action string, issue int, reason string) {
	t.Helper()
	if got.NextAction != action || got.IssueNumber != issue || got.ExitReason != reason {
		t.Errorf("decision = %s #%d (%q), want %s #%d (%q)", got.NextAction, got.IssueNumber, got.ExitReason, action, issue, reason)
	}
}

// Tickets are taken in the order of their numbers, whatever order GitHub
// lists them in.
func TestDecide_TicketsInNumberOrder(t *testing.T) {
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 9, Body: "c"}, {Number: 3, Body: "a"}, {Number: 5, Body: "b"}}

	wantDecision(t, decide(t, t.TempDir(), mock), ActionDispatchWorker, 3, "")
}

func TestDecide_DependenciesDecideWhenATicketStarts(t *testing.T) {
	tests := []struct {
		name       string
		pending    []Issue
		tickets    map[int]IssueStatus // issues that carry the task label
		others     map[int]IssueStatus // issues GitHub knows that are not tickets
		wantAction string
		wantIssue  int
		wantReason string
	}{
		{
			name:       "the dependency is pending too: it goes first",
			pending:    []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}, {Number: 3, Body: ticketBody()}},
			tickets:    map[int]IssueStatus{3: open, 5: open},
			wantAction: ActionDispatchWorker, wantIssue: 3,
		},
		{
			name:       "the dependency has a higher number: the ticket waits and the dependency goes first",
			pending:    []Issue{{Number: 3, Body: ticketBody(dependsOn(5))}, {Number: 5, Body: ticketBody()}},
			tickets:    map[int]IssueStatus{3: open, 5: open},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name:       "a dependency closed as completed is done",
			pending:    []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}},
			tickets:    map[int]IssueStatus{3: completed, 5: open},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name:       "closed without a reason counts as completed",
			pending:    []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}},
			tickets:    map[int]IssueStatus{3: closedNoReason, 5: open},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name:       "every dependency must be done",
			pending:    []Issue{{Number: 7, Body: ticketBody(dependsOn(3, 4))}},
			tickets:    map[int]IssueStatus{3: completed, 4: open, 7: open},
			wantAction: ActionNone, wantIssue: 7, wantReason: ReasonBlockedByDependencies,
		},
		{
			name:       "a dependency that is open and not pending holds the ticket",
			pending:    []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}},
			tickets:    map[int]IssueStatus{3: open, 5: open},
			wantAction: ActionNone, wantIssue: 5, wantReason: ReasonBlockedByDependencies,
		},
		{
			name:       "a dependency closed as not planned will never be done",
			pending:    []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}},
			tickets:    map[int]IssueStatus{3: notPlanned, 5: open},
			wantAction: ActionNone, wantIssue: 5, wantReason: ReasonBlockedByDependencies,
		},
		{
			name:       "a waiting ticket does not hold up one that is ready",
			pending:    []Issue{{Number: 4, Body: ticketBody(dependsOn(9))}, {Number: 6, Body: ticketBody()}},
			tickets:    map[int]IssueStatus{4: open, 6: open, 9: open},
			wantAction: ActionDispatchWorker, wantIssue: 6,
		},
		{
			name:       "tickets that wait for each other",
			pending:    []Issue{{Number: 4, Body: ticketBody(dependsOn(6))}, {Number: 6, Body: ticketBody(dependsOn(4))}},
			tickets:    map[int]IssueStatus{4: open, 6: open},
			wantAction: ActionNone, wantIssue: 4, wantReason: ReasonBlockedByDependencies,
		},
		{
			name:       "a dependency that is not a ticket is asked for by number",
			pending:    []Issue{{Number: 5, Body: ticketBody(dependsOn(2))}},
			tickets:    map[int]IssueStatus{5: open},
			others:     map[int]IssueStatus{2: completed},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
		{
			name:       "a ticket without the block has no dependencies",
			pending:    []Issue{{Number: 5, Body: "## Summary\n- **Repo**: backend\n"}},
			tickets:    map[int]IssueStatus{5: open},
			wantAction: ActionDispatchWorker, wantIssue: 5,
		},
	}
	for _, tt := range tests {
		mock := NewMockGitHubClient()
		mock.PendingIssues = tt.pending
		mock.OpenIssueCount = len(tt.pending)
		mock.TaskStatuses = tt.tickets
		for issue, status := range tt.others {
			mock.IssueStatuses[issue] = status
		}

		got := decide(t, t.TempDir(), mock)

		if got.NextAction != tt.wantAction || got.IssueNumber != tt.wantIssue || got.ExitReason != tt.wantReason {
			t.Errorf("%s: decision = %s #%d (%q), want %s #%d (%q)", tt.name,
				got.NextAction, got.IssueNumber, got.ExitReason, tt.wantAction, tt.wantIssue, tt.wantReason)
		}
		if len(mock.ReopenedIssues) != 0 || len(mock.Comments) != 0 || len(mock.AddedLabels) != 0 {
			t.Errorf("%s: an issue was changed: reopened %v, comments %v, labels %v", tt.name, mock.ReopenedIssues, mock.Comments, mock.AddedLabels)
		}
		// However many tickets and dependencies, the list is fetched once.
		lists := 0
		for _, call := range mock.Calls {
			if call == "TaskIssueStatuses" {
				lists++
			}
		}
		if lists > 1 {
			t.Errorf("%s: the list of tickets was fetched %d times for one decision: %v", tt.name, lists, mock.Calls)
		}
	}
}

func TestConfig_TicketRepos(t *testing.T) {
	tests := []struct {
		name  string
		repos []RepoConfig
		want  []string
	}{
		{"no repos: the root alone", nil, []string{"root"}},
		{"the repos of the config, after the root", []RepoConfig{{Name: "backend"}, {Name: "frontend"}}, []string{"root", "backend", "frontend"}},
		{"a config that lists the root itself", []RepoConfig{{Name: "root"}, {Name: "backend"}}, []string{"root", "backend"}},
		{"a repo without a name is not one", []RepoConfig{{Name: ""}, {Name: "backend"}}, []string{"root", "backend"}},
	}
	for _, tt := range tests {
		config := &Config{Repos: tt.repos}
		if got := config.TicketRepos(); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: TicketRepos() = %v, want %v", tt.name, got, tt.want)
		}
	}

	config := &Config{Repos: []RepoConfig{{Name: "backend"}}}
	for name, want := range map[string]bool{"backend": true, "Backend": true, "root": true, "ROOT": true, "frontend": false, "": false, "back": false} {
		if got := config.HasTicketRepo(name); got != want {
			t.Errorf("HasTicketRepo(%q) = %v, want %v", name, got, want)
		}
	}
}

// A dependency that is a pull request is done once the pull request is merged.
func TestDecide_DependencyThatIsAPullRequest(t *testing.T) {
	for state, want := range map[string]string{"MERGED": ActionDispatchWorker, "OPEN": ActionNone} {
		mock := NewMockGitHubClient()
		mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(2))}}
		mock.OpenIssueCount = 1
		mock.TaskStatuses = map[int]IssueStatus{5: open}
		mock.IssueStatuses[2] = IssueStatus{State: state}

		if got := decide(t, t.TempDir(), mock); got.NextAction != want || got.IssueNumber != 5 {
			t.Errorf("pull request %s: decision = %s #%d (%q), want %s #5", state, got.NextAction, got.IssueNumber, got.ExitReason, want)
		}
	}
}

// A dependency whose state cannot be read is a failed lookup, not a blocked
// ticket: the run stops saying GitHub could not be asked.
func TestDecide_DependencyThatCannotBeRead(t *testing.T) {
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(99))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{5: open}
	mock.IssueStatusError = errors.New("HTTP 502")

	wantDecision(t, decide(t, t.TempDir(), mock), ActionNone, 0, ReasonGitHubAPIError)
	if len(mock.Comments) != 0 || len(mock.AddedLabels) != 0 {
		t.Errorf("the ticket was handed to a person over a failed lookup: comments %v, labels %v", mock.Comments, mock.AddedLabels)
	}

	// The same when the list of tickets itself cannot be read.
	mock = NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.IssueStatusesError = errors.New("HTTP 502")
	mock.IssueStatusError = errors.New("HTTP 502")

	wantDecision(t, decide(t, t.TempDir(), mock), ActionNone, 0, ReasonGitHubAPIError)
}

// A dependency this workflow worked on is done only when its pull request was
// merged. Closed without that, it is done again before anything builds on it.
func TestDecide_DependencyClosedWithoutLanding(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 3)
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}

	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 3, "")

	if !reflect.DeepEqual(mock.ReopenedIssues, []int{3}) {
		t.Errorf("reopened %v, want the issue that never landed", mock.ReopenedIssues)
	}
	if !reflect.DeepEqual(mock.AddedLabels[3], []string{"ai-task"}) {
		t.Errorf("labels added to #3 = %v, want the task label so that it is a ticket again", mock.AddedLabels[3])
	}
	if !reflect.DeepEqual(mock.RemovedLabels[3], []string{"completed"}) {
		t.Errorf("labels removed from #3 = %v, want the one that says it is done", mock.RemovedLabels[3])
	}
}

// An issue whose Worker found nothing to change is closed without a pull
// request, and rightly so. It is done; doing it again would find nothing to
// change again, for ever.
func TestDecide_ClosedWithNothingToChange(t *testing.T) {
	dir := t.TempDir()
	recordResult(t, dir, 3, `{"status":"success_no_changes"}`)

	// As a dependency. The dispatch took the task label off it when it closed it.
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{5: open}
	mock.IssueStatuses[3] = completed

	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 5, "")

	// And before the run is called complete, should it still carry the label.
	mock2 := NewMockGitHubClient()
	mock2.TaskStatuses = map[int]IssueStatus{3: completed}

	wantDecision(t, decide(t, dir, mock2), ActionAllComplete, 0, "")

	for _, m := range []*MockGitHubClient{mock, mock2} {
		if len(m.ReopenedIssues) != 0 {
			t.Errorf("reopened %v: an issue that needed no change", m.ReopenedIssues)
		}
		for _, call := range m.Calls {
			if strings.HasPrefix(call, "HasMergedPR") || call == "MergedIssueBranches" {
				t.Errorf("asked for the pull request of an issue that needed none: %v", m.Calls)
			}
		}
	}
}

// A result file that cannot be read still says the workflow worked on the
// issue: its closing needs a merge behind it.
func TestDecide_ResultThatCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	recordResult(t, dir, 3, `{"status": "succ`)
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}

	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 3, "")
}

// An issue that cannot be reopened is not dispatched: the dispatch of a closed
// issue fails, and the run would go round and round.
func TestDecide_FalseCompletionThatCannotBeReopened(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 3)

	// As a dependency.
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}
	mock.ReopenError = errors.New("HTTP 403")

	wantDecision(t, decide(t, dir, mock), ActionNone, 0, ReasonGitHubAPIError)

	// Before the run is called complete.
	mock = NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{3: completed}
	mock.ReopenError = errors.New("HTTP 403")

	wantDecision(t, decide(t, dir, mock), ActionNone, 0, ReasonGitHubAPIError)
	if len(mock.AddedLabels) != 0 {
		t.Errorf("labels were added to an issue that stayed closed: %v", mock.AddedLabels)
	}
}

func TestDecide_DependencyThatLanded(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 3)
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}
	mock.MergedBranches["feat/ai-issue-3"] = true

	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 5, "")
	if len(mock.ReopenedIssues) != 0 {
		t.Errorf("reopened %v although the work was merged", mock.ReopenedIssues)
	}

	// The answer is remembered: GitHub is not asked about #3 again.
	mock.Calls = nil
	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 5, "")
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "HasMergedPR") {
			t.Errorf("GitHub was asked again whether #3 landed: %v", mock.Calls)
		}
	}
}

// An issue the workflow never touched is done when someone closes it. Nobody
// asks for its pull request.
func TestDecide_DependencyClosedByAPerson(t *testing.T) {
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 5, Body: ticketBody(dependsOn(3))}}
	mock.OpenIssueCount = 1
	mock.TaskStatuses = map[int]IssueStatus{3: completed, 5: open}

	wantDecision(t, decide(t, t.TempDir(), mock), ActionDispatchWorker, 5, "")

	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "HasMergedPR") {
			t.Errorf("asked for the pull request of an issue the workflow never worked on: %v", mock.Calls)
		}
	}
}

// A ticket whose block cannot be used is handed to a person, with the reason
// on the issue. It is not dispatched, and nothing is guessed from its prose.
func TestDecide_TicketThatCannotBeUsed(t *testing.T) {
	tests := []struct {
		name, body, wantInComment string
	}{
		{"unknown field", ticketBody("priority: high"), `unknown field "priority"`},
		{"schema from the future", "```yaml awk-ticket\nschema: 2\nrepo: root\n```\n", "schema 2"},
		{"no repo", "```yaml awk-ticket\nschema: 1\n```\n", "repo is missing"},
		{"depends on itself", ticketBody(dependsOn(3, 7)), "depends_on lists the ticket itself (#7)"},
		{"depends on an issue that does not exist", ticketBody(dependsOn(3, 99)), "depends_on lists #99, and the repository has no such issue"},
		{"a repo the config does not have", ticketBodyFor("web"), `repo "web" is not a repo of .ai/config/workflow.yaml (it has: root)`},
		{"block not closed", "```yaml awk-ticket\nschema: 1\nrepo: root\n", "is not closed"},
	}
	for _, tt := range tests {
		mock := NewMockGitHubClient()
		mock.PendingIssues = []Issue{{Number: 7, Body: tt.body}, {Number: 8, Body: ticketBody()}}
		mock.OpenIssueCount = 2
		mock.TaskStatuses = map[int]IssueStatus{3: completed, 7: open, 8: open}

		got := decide(t, t.TempDir(), mock)

		if got.NextAction != ActionNone || got.IssueNumber != 7 || got.ExitReason != ReasonInvalidTicket {
			t.Errorf("%s: decision = %s #%d (%q), want none #7 (%q)", tt.name, got.NextAction, got.IssueNumber, got.ExitReason, ReasonInvalidTicket)
		}
		if len(mock.Comments[7]) != 1 {
			t.Errorf("%s: %d comments on the ticket, want one that says what is wrong", tt.name, len(mock.Comments[7]))
			continue
		}
		comment := mock.Comments[7][0]
		for _, want := range []string{tt.wantInComment, "yaml awk-ticket", "needs-human-review"} {
			if !strings.Contains(comment, want) {
				t.Errorf("%s: the comment should mention %q:\n%s", tt.name, want, comment)
			}
		}
		if !reflect.DeepEqual(mock.AddedLabels[7], []string{"needs-human-review"}) {
			t.Errorf("%s: labels added = %v, want needs-human-review", tt.name, mock.AddedLabels[7])
		}
	}
}

// A ticket is for one of the repos of the workflow config, or for the root of
// the repository. The name is compared the way the Worker compares it.
func TestDecide_TicketNamesARepoOfTheConfig(t *testing.T) {
	config := defaultTestConfig()
	config.Repos = []RepoConfig{{Name: "backend"}, {Name: "frontend"}}

	for _, repo := range []string{"backend", "frontend", "Backend", "root", "ROOT"} {
		mock := NewMockGitHubClient()
		mock.PendingIssues = []Issue{{Number: 7, Body: ticketBodyFor(repo)}}
		mock.OpenIssueCount = 1

		got, err := newTestAnalyzer(t.TempDir(), config, mock).Decide(context.Background())

		if err != nil || got.NextAction != ActionDispatchWorker || got.IssueNumber != 7 {
			t.Errorf("repo %q: decision = %+v, err %v; want the ticket dispatched", repo, got, err)
		}
	}

	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 7, Body: ticketBodyFor("web")}}
	mock.OpenIssueCount = 1

	got, err := newTestAnalyzer(t.TempDir(), config, mock).Decide(context.Background())

	if err != nil || got.NextAction != ActionNone || got.ExitReason != ReasonInvalidTicket {
		t.Fatalf("decision = %+v, err %v; want the ticket refused", got, err)
	}
	if len(mock.Comments[7]) != 1 || !strings.Contains(mock.Comments[7][0], "(it has: root, backend, frontend)") {
		t.Errorf("the comment should list the repos of the config: %v", mock.Comments[7])
	}
}

// The comment is how the author learns what is wrong; when it cannot be
// posted the ticket is still held back.
func TestDecide_TicketThatCannotBeUsed_CommentFails(t *testing.T) {
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{{Number: 7, Body: ticketBody("priority: high")}}
	mock.OpenIssueCount = 1
	mock.CommentError = errors.New("HTTP 403")

	wantDecision(t, decide(t, t.TempDir(), mock), ActionNone, 7, ReasonInvalidTicket)
	if !reflect.DeepEqual(mock.AddedLabels[7], []string{"needs-human-review"}) {
		t.Errorf("labels added = %v, want needs-human-review", mock.AddedLabels[7])
	}
}

// An issue whose pull request is already merged was left open by an
// interrupted run. It is closed, and its merge is on record from then on.
func TestDecide_PendingIssueAlreadyMerged(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockGitHubClient()
	mock.PendingIssues = []Issue{
		{Number: 60, Body: "https://github.com/owner/repo/pull/600"},
		{Number: 61, Body: ticketBody(dependsOn(60))},
	}
	mock.OpenIssueCount = 2
	mock.PRMerged[600] = true
	recordWorkedOn(t, dir, 60)
	// What GitHub says once #60 has been closed.
	mock.TaskStatuses = map[int]IssueStatus{60: completed, 61: open}

	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 61, "")

	if !reflect.DeepEqual(mock.ClosedIssues, []int{60}) {
		t.Errorf("closed %v, want the issue whose pull request is merged", mock.ClosedIssues)
	}
	for _, call := range mock.Calls {
		if strings.HasPrefix(call, "HasMergedPR") {
			t.Errorf("the merge was just seen, yet GitHub was asked again: %v", mock.Calls)
		}
	}
}

// Before a run is called complete: a ticket the workflow worked on that is
// closed with nothing merged is done again.
func TestDecide_NotCompleteWhileATicketNeverLanded(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 12)
	recordWorkedOn(t, dir, 14)
	mock := NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{12: completed, 14: completed}
	mock.MergedBranches["feat/ai-issue-12"] = true

	wantDecision(t, decide(t, dir, mock), ActionDispatchWorker, 14, "")

	if !reflect.DeepEqual(mock.ReopenedIssues, []int{14}) {
		t.Errorf("reopened %v, want only the ticket with nothing merged", mock.ReopenedIssues)
	}
}

func TestDecide_AllCompleteWhenEverythingLanded(t *testing.T) {
	dir := t.TempDir()
	for _, issue := range []int{12, 14, 15} {
		recordWorkedOn(t, dir, issue)
	}
	mock := NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{
		12: completed,  // merged, found in the list of merged branches
		14: completed,  // merged, but not in that list: asked one by one
		15: notPlanned, // someone decided against it: not the workflow's to reopen
		16: completed,  // never worked on by the workflow: closed by a person
	}
	mock.MergedBranches["feat/ai-issue-12"] = true
	mock.MergedUnlisted = map[string]bool{"feat/ai-issue-14": true}

	wantDecision(t, decide(t, dir, mock), ActionAllComplete, 0, "")
	if len(mock.ReopenedIssues) != 0 {
		t.Errorf("reopened %v", mock.ReopenedIssues)
	}
	// One call answers for what it lists; only the rest is asked for.
	if want := []string{"TaskIssueStatuses", "MergedIssueBranches", "HasMergedPR feat/ai-issue-14"}; !reflect.DeepEqual(mock.Calls, want) {
		t.Errorf("lookups = %v, want %v", mock.Calls, want)
	}

	// Asked once. The next time everything is on record, and GitHub is asked
	// for the list of tickets only.
	mock.Calls = nil
	wantDecision(t, decide(t, dir, mock), ActionAllComplete, 0, "")
	if !reflect.DeepEqual(mock.Calls, []string{"TaskIssueStatuses"}) {
		t.Errorf("lookups on the second run = %v, want only the list of tickets", mock.Calls)
	}
}

// A run is not called complete when it cannot be checked.
func TestDecide_NotCompleteWhenTheCheckFails(t *testing.T) {
	// The list of tickets cannot be read.
	mock := NewMockGitHubClient()
	mock.IssueStatusesError = errors.New("HTTP 502")
	wantDecision(t, decide(t, t.TempDir(), mock), ActionNone, 0, ReasonGitHubAPIError)

	// Whether a closed ticket landed cannot be read.
	dir := t.TempDir()
	recordWorkedOn(t, dir, 12)
	mock = NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{12: completed}
	mock.MergedBranchesError = errors.New("HTTP 502")
	mock.HasMergedPRError = errors.New("HTTP 502")
	wantDecision(t, decide(t, dir, mock), ActionNone, 0, ReasonGitHubAPIError)
	if len(mock.ReopenedIssues) != 0 {
		t.Errorf("reopened %v on a failed lookup", mock.ReopenedIssues)
	}

	// The list of merged branches failing alone costs nothing: each ticket is
	// then asked for.
	mock = NewMockGitHubClient()
	mock.TaskStatuses = map[int]IssueStatus{12: completed}
	mock.MergedBranchesError = errors.New("HTTP 502")
	mock.MergedBranches["feat/ai-issue-12"] = true
	wantDecision(t, decide(t, dir, mock), ActionAllComplete, 0, "")
}

func TestMarkLanded(t *testing.T) {
	dir := t.TempDir()
	a := newTestAnalyzer(dir, defaultTestConfig(), NewMockGitHubClient())

	if a.knownLanded(7) {
		t.Fatal("nothing is on record yet")
	}
	MarkLanded(dir, 7)
	MarkLanded(dir, 7) // recording it twice is harmless
	if !a.knownLanded(7) {
		t.Error("the merge of #7 is not on record")
	}
	if a.knownLanded(70) || a.knownLanded(8) {
		t.Error("another issue is on record")
	}
}

// The review records a merge where the decision engine reads it.
func TestLanded_ReadsWhatTheReviewRecorded(t *testing.T) {
	dir := t.TempDir()
	recordWorkedOn(t, dir, 7)
	mock := NewMockGitHubClient()
	a := newTestAnalyzer(dir, defaultTestConfig(), mock)
	MarkLanded(dir, 7)

	landed, err := a.landed(context.Background(), 7)

	if err != nil || !landed {
		t.Errorf("landed = %v, %v; want true from the record", landed, err)
	}
	if len(mock.Calls) != 0 {
		t.Errorf("GitHub was asked although the merge is on record: %v", mock.Calls)
	}
}
