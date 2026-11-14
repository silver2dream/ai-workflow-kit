package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// issue is an issue as `gh issue list --json` prints it.
type issue struct {
	Number int                 `json:"number"`
	Title  string              `json:"title,omitempty"`
	Body   string              `json:"body,omitempty"`
	Labels []map[string]string `json:"labels,omitempty"`
}

func labelled(number int, title, body string, labels ...string) issue {
	i := issue{Number: number, Title: title, Body: body}
	for _, name := range labels {
		i.Labels = append(i.Labels, map[string]string{"name": name})
	}
	return i
}

func ticketBody(dependsOn ...int) string {
	body := "## Summary\nwork\n\n```yaml awk-ticket\nschema: 1\nrepo: root\n"
	if len(dependsOn) > 0 {
		deps := make([]string, len(dependsOn))
		for i, dep := range dependsOn {
			deps[i] = fmt.Sprint(dep)
		}
		body += "depends_on: [" + strings.Join(deps, ", ") + "]\n"
	}
	return body + "```\n"
}

// lister answers from a table of issues by label and state, and writes down
// what was asked.
type lister struct {
	issues map[string][]issue // "label/state"
	fail   map[string]error
	asked  []string
}

func (l *lister) list(_ context.Context, label, state, fields string) ([]byte, error) {
	key := label + "/" + state
	l.asked = append(l.asked, key+" "+fields)
	if err := l.fail[key]; err != nil {
		return nil, err
	}
	issues := l.issues[key]
	if issues == nil {
		issues = []issue{}
	}
	return json.Marshal(issues)
}

// A ticket's stage is read from the label the workflow has put on it; one
// without such a label is ready, or waits for another open ticket. They are
// shown with what needs a person first.
func TestCollectTickets_Stages(t *testing.T) {
	l := &lister{issues: map[string][]issue{
		"ai-task/open": {
			labelled(21, "[feat] receipts", ticketBody(16, 3)), // 3 is not an open ticket: taken as done
			labelled(16, "[feat] shop checkout", ticketBody()),
			labelled(13, "[feat] shop page", ticketBody(), "ai-task", "in-progress"),
			labelled(12, "[feat] daily reward", ticketBody(), "pr-ready"),
			labelled(11, "[fix] flaky test", ticketBody(), "review-failed"),
			labelled(14, "[fix] race in the queue", ticketBody(), "needs-human-review"),
			labelled(9, "[feat] add login", ticketBody(), "worker-failed"),
			labelled(10, "[feat] sign up", ticketBody(), "merge-conflict"),
			labelled(8, "[feat] profile", ticketBody(), "needs-rebase"),
			labelled(22, "[feat] by hand", "a ticket written before the block existed"),
			labelled(23, "[feat] bad block", "```yaml awk-ticket\nschema: 1\nrepo: root\nseverity: urgent\n```\n"),
		},
		"ai-task/closed":   {{Number: 1}, {Number: 2}, {Number: 3}},
		"completed/closed": {{Number: 3}, {Number: 4}},
	}}

	tickets, err := CollectTickets(context.Background(), config.DefaultLabels(), l.list)
	if err != nil {
		t.Fatalf("CollectTickets: %v", err)
	}

	type shown struct {
		Number int
		Stage  string
	}
	var got []shown
	for _, ticket := range tickets.Open {
		got = append(got, shown{ticket.Number, ticket.Stage})
	}
	want := []shown{
		{9, StageNeedsYou}, {14, StageNeedsYou}, {23, StageNeedsYou},
		{11, StageInReview}, {12, StageInReview},
		{13, StageInProgress},
		{8, StageRework}, {10, StageRework},
		{16, StageReady}, {22, StageReady},
		{21, StageWaiting},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the tickets, in the order shown:\n%v\nwant\n%v", got, want)
	}

	why := map[int]string{}
	for _, ticket := range tickets.Open {
		why[ticket.Number] = ticket.Why
	}
	for number, want := range map[int]string{
		9:  "its Worker failed on every attempt",
		14: "marked needs-human-review",
		23: "its awk-ticket block cannot be read",
		12: "waits for the review",
		11: "reviewed again",
		13: "a Worker is on it",
		10: "conflicts with the branch it goes to",
		8:  "is behind the branch it goes to",
		21: "waits for #16",
	} {
		if !strings.Contains(why[number], want) {
			t.Errorf("#%d: %q, want it to say %q", number, why[number], want)
		}
	}
	if why[16] != "" || why[22] != "" {
		t.Errorf("a ready ticket needs no reason: %q, %q", why[16], why[22])
	}
	if strings.Contains(why[21], "#3") {
		t.Errorf("#21 is said to wait for #3, which is not an open ticket: %q", why[21])
	}
	if waiting := tickets.In(StageWaiting); len(waiting) != 1 || !reflect.DeepEqual(waiting[0].WaitsFor, []int{16}) {
		t.Errorf("the waiting tickets: %+v, want #21 waiting for #16", waiting)
	}

	// Done: the merged ones still carry the task label, the ones that needed
	// no change carry the completed label; an issue with both counts once.
	if tickets.Done != 4 {
		t.Errorf("Done = %d, want 4", tickets.Done)
	}
	if len(l.asked) != 3 || l.asked[0] != "ai-task/open number,title,labels,body" {
		t.Errorf("GitHub was asked %v", l.asked)
	}
}

// The labels are the project's: a project that named its own is read by them.
func TestCollectTickets_TheProjectsLabels(t *testing.T) {
	labels := config.DefaultLabels()
	labels.Task, labels.InProgress, labels.NeedsHumanReview, labels.Completed = "work", "doing", "human", "finished"
	l := &lister{issues: map[string][]issue{
		"work/open": {
			labelled(5, "a", ticketBody(), "doing"),
			labelled(6, "b", ticketBody(), "human"),
			labelled(7, "c", ticketBody(), "in-progress"), // the default name means nothing here
		},
		"finished/closed": {{Number: 1}},
	}}
	tickets, err := CollectTickets(context.Background(), labels, l.list)
	if err != nil {
		t.Fatalf("CollectTickets: %v", err)
	}
	stage := map[int]string{}
	for _, ticket := range tickets.Open {
		stage[ticket.Number] = ticket.Stage
	}
	if want := map[int]string{5: StageInProgress, 6: StageNeedsYou, 7: StageReady}; !reflect.DeepEqual(stage, want) {
		t.Errorf("stages = %v, want %v", stage, want)
	}
	if tickets.Done != 1 {
		t.Errorf("Done = %d, want 1", tickets.Done)
	}
	if !strings.Contains(tickets.In(StageNeedsYou)[0].Why, "marked human") {
		t.Errorf("the reason names the default label: %q", tickets.In(StageNeedsYou)[0].Why)
	}
}

// Without the open tickets there is no view; without the closed ones there
// is one that does not say how many are done.
func TestCollectTickets_WhenGitHubCannotBeAsked(t *testing.T) {
	down := errors.New("gh: could not connect")
	if _, err := CollectTickets(context.Background(), config.DefaultLabels(), (&lister{fail: map[string]error{"ai-task/open": down}}).list); !errors.Is(err, down) {
		t.Errorf("the open tickets cannot be had: err = %v, want the error", err)
	}

	l := &lister{
		issues: map[string][]issue{"ai-task/open": {labelled(5, "a", ticketBody())}},
		fail:   map[string]error{"ai-task/closed": down, "completed/closed": down},
	}
	tickets, err := CollectTickets(context.Background(), config.DefaultLabels(), l.list)
	if err != nil || len(tickets.Open) != 1 || tickets.Done != 0 {
		t.Errorf("the closed tickets cannot be had: %+v (err %v), want the open one and no count", tickets, err)
	}

	garbled := func(context.Context, string, string, string) ([]byte, error) { return []byte("not json"), nil }
	if _, err := CollectTickets(context.Background(), config.DefaultLabels(), garbled); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Errorf("an answer that is not JSON: err = %v", err)
	}
}
