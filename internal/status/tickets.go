package status

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// The stages a ticket is shown in, in the order a person wants them: what
// needs them first.
const (
	StageNeedsYou   = "needs_you"
	StageInReview   = "in_review"
	StageInProgress = "in_progress"
	StageRework     = "rework"
	StageReady      = "ready"
	StageWaiting    = "waiting"
)

// Stages lists the stages in the order they are shown, with their titles.
var Stages = []struct{ Stage, Title string }{
	{StageNeedsYou, "needs you"},
	{StageInReview, "in review"},
	{StageInProgress, "in progress"},
	{StageRework, "back to a Worker"},
	{StageReady, "ready"},
	{StageWaiting, "waiting"},
}

// Ticket is an open ticket as `awkit status` shows it.
type Ticket struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Stage    string `json:"stage"`
	Why      string `json:"why,omitempty"` // what puts it in its stage, where that is not plain from the stage
	WaitsFor []int  `json:"waits_for,omitempty"`
}

// Tickets is the work of a project: the tickets that are open, in the order
// they are shown, and how many are done.
type Tickets struct {
	Open []Ticket `json:"open"`
	Done int      `json:"done"`
}

// In returns the open tickets of a stage.
func (t *Tickets) In(stage string) []Ticket {
	var in []Ticket
	for _, ticket := range t.Open {
		if ticket.Stage == stage {
			in = append(in, ticket)
		}
	}
	return in
}

// IssueLister asks GitHub for the issues that carry a label, open or closed,
// and returns the JSON `gh issue list` prints for the fields.
type IssueLister func(ctx context.Context, label, state, fields string) ([]byte, error)

// ListWithGh is the IssueLister that asks `gh`.
func ListWithGh(ctx context.Context, label, state, fields string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "gh", "issue", "list", "--label", label, "--state", state, "--limit", "1000", "--json", fields).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

// CollectTickets asks for the project's tickets and says what stage each is
// in. The stage is read from the labels the workflow moves an issue through;
// the decision engine is what acts on them, and this is the view of a person.
//
// A ticket without one of those labels is ready, unless it waits for another
// ticket that is still open. An issue it waits for that is not among the open
// tickets is taken as done: whether it was closed as completed is the
// decision engine's question, asked when the ticket's turn comes.
func CollectTickets(ctx context.Context, labels config.Labels, list IssueLister) (*Tickets, error) {
	raw, err := list(ctx, labels.Task, "open", "number,title,labels,body")
	if err != nil {
		return nil, err
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(raw, &issues); err != nil {
		return nil, fmt.Errorf("the list of open tickets could not be read: %w", err)
	}

	open := map[int]bool{}
	for _, issue := range issues {
		open[issue.Number] = true
	}

	tickets := &Tickets{}
	for _, issue := range issues {
		has := map[string]bool{}
		for _, label := range issue.Labels {
			has[label.Name] = true
		}
		t := Ticket{Number: issue.Number, Title: issue.Title}
		switch {
		case has[labels.NeedsHumanReview]:
			t.Stage, t.Why = StageNeedsYou, "marked "+labels.NeedsHumanReview+"; the issue's comments say why"
		case has[labels.WorkerFailed]:
			t.Stage, t.Why = StageNeedsYou, "its Worker failed on every attempt; the issue has the last error"
		case has[labels.InProgress]:
			t.Stage, t.Why = StageInProgress, "a Worker is on it"
		case has[labels.PRReady]:
			t.Stage, t.Why = StageInReview, "its pull request waits for the review"
		case has[labels.ReviewFailed]:
			t.Stage, t.Why = StageInReview, "its pull request is reviewed again"
		case has[labels.MergeConflict]:
			t.Stage, t.Why = StageRework, "its pull request conflicts with the branch it goes to"
		case has[labels.NeedsRebase]:
			t.Stage, t.Why = StageRework, "its pull request is behind the branch it goes to"
		default:
			meta, _, err := ticket.Parse(issue.Body)
			if err != nil {
				t.Stage, t.Why = StageNeedsYou, "its awk-ticket block cannot be read: "+err.Error()
				break
			}
			for _, dep := range meta.DependsOn {
				if open[dep] {
					t.WaitsFor = append(t.WaitsFor, dep)
				}
			}
			if len(t.WaitsFor) > 0 {
				waits := make([]string, len(t.WaitsFor))
				for i, dep := range t.WaitsFor {
					waits[i] = fmt.Sprintf("#%d", dep)
				}
				t.Stage, t.Why = StageWaiting, "waits for "+strings.Join(waits, ", ")
			} else {
				t.Stage = StageReady
			}
		}
		tickets.Open = append(tickets.Open, t)
	}

	rank := map[string]int{}
	for i, stage := range Stages {
		rank[stage.Stage] = i
	}
	sort.SliceStable(tickets.Open, func(i, j int) bool {
		a, b := tickets.Open[i], tickets.Open[j]
		if rank[a.Stage] != rank[b.Stage] {
			return rank[a.Stage] < rank[b.Stage]
		}
		return a.Number < b.Number
	})

	// How many are done: the closed issues that still carry the task label
	// (their pull request was merged), and the ones closed with the completed
	// label in its place (no change was needed). A list that cannot be had is
	// not worth failing the whole view for: the open tickets are what a
	// person came for.
	done := map[int]bool{}
	for _, label := range []string{labels.Task, labels.Completed} {
		if label == "" {
			continue
		}
		raw, err := list(ctx, label, "closed", "number")
		if err != nil {
			continue
		}
		var closed []struct {
			Number int `json:"number"`
		}
		if json.Unmarshal(raw, &closed) == nil {
			for _, issue := range closed {
				done[issue.Number] = true
			}
		}
	}
	tickets.Done = len(done)
	return tickets, nil
}
