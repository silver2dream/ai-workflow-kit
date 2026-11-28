package engine

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// The shadow (design.md §5.1, task 1.7): before each step of a run, the
// Principal's analyzer decides, from the same state of GitHub, what it
// would do with the tickets in the workflow's hands, and the engine's
// decision is compared with it. The engine does what it decided; the
// analyzer decides and does nothing (it changes no label and counts no
// attempt). What a run's checks and the choice of the next ticket decide
// (Precheck, and NextTicket, which the engine asks as Candidates) is the
// analyzer's own in both, and is not compared; the paths a lane holds, which
// only the engine looks at, can make it start another ticket (design.md
// §5.3).
//
// Both read one state: the analyzer reads the tickets from the snapshot the
// shadow takes, asks GitHub nothing the engine asked in the same step (a
// pull request's mergeability), and counts the attempts in the engine's
// lanes, which a run of the engine keeps in the place of the analyzer's
// files.

// TypeShadow is the event of one comparison.
const TypeShadow = "engine_shadow"

// KnownOrder is the known difference of the order: of several tickets in
// the same state, the engine takes the one with the lowest number and the
// analyzer the first that `gh issue list` gives (the newest).
const KnownOrder = "order"

// KnownCheckFirst is the known difference of a ticket the engine dispatched
// whose labels show no Worker at work (the Worker has not put its label on
// yet, or ended without a result and took it off): the engine asks for the
// result the Worker may have left before it dispatches the ticket again;
// the analyzer, which reads only the labels, has nothing in hand and picks
// the ticket again at once.
const KnownCheckFirst = "check_first"

// KnownQueue is the known difference of the merge queue (task 2.3), which
// the analyzer does not have: the engine merges the lane at the head of its
// queue, or the analyzer takes a lane in the queue, whose ticket is labeled
// pr-ready, for one to review.
const KnownQueue = "queue"

// Shadow decides as the Principal's analyzer, from the tickets given, what
// to do with the tickets in the workflow's hands (analyzer.LabelDecision):
// nil is none in its hands. mergeable answers what the engine was told of a
// pull request; attempts counts what the engine counted.
type Shadow func(ctx context.Context, tickets []Ticket, mergeable Mergeability, attempts func(kind string, pr int) (int, bool)) (*analyzer.Decision, error)

// Mergeability is GitHub's word on a pull request, asked once a step.
type Mergeability func(ctx context.Context, pr int) (string, error)

// Comparison is one step's comparison, the data of its event.
type Comparison struct {
	// Engine and Analyzer are the two decisions; nil is none of the
	// tickets in the workflow's hands (the next ticket is picked).
	Engine   *analyzer.Decision `json:"engine"`
	Analyzer *analyzer.Decision `json:"analyzer"`
	Same     bool               `json:"same"`
	// Known names a known difference (KnownOrder); a difference without
	// one is to be looked into.
	Known string `json:"known,omitempty"`
	// Error is why the analyzer could not decide: nothing was compared.
	Error string `json:"error,omitempty"`
}

// Differs says whether the comparison found a difference that is not known.
func (c Comparison) Differs() bool {
	return c.Error == "" && !c.Same && c.Known == ""
}

// NewShadow is the shadow of a project's analyzer: it reads the tickets
// given in the place of asking GitHub for their labels, and asks client
// the rest (the pull request of a ticket, and whether it is merged).
func NewShadow(root string, cfg *config.Config, only int, client analyzer.GitHubClientInterface) Shadow {
	return func(ctx context.Context, tickets []Ticket, mergeable Mergeability, attempts func(string, int) (int, bool)) (*analyzer.Decision, error) {
		a := analyzer.New(root, cfg)
		a.GHClient = &shadowGitHub{tickets: tickets, mergeable: mergeable, GitHubClientInterface: client}
		a.ReadOnly = true
		a.AttemptsOf = attempts
		if only > 0 {
			a.LimitTo(only)
		}
		return a.LabelDecision(ctx)
	}
}

// errShadowWrites is what the shadow's analyzer is told when it tries to
// change GitHub: it is read-only, and does not.
var errShadowWrites = errors.New("the shadow changes nothing on GitHub")

// shadowGitHub is GitHub as the shadow's analyzer sees it: the tickets of the
// step, the engine's word on mergeability, and no change.
type shadowGitHub struct {
	analyzer.GitHubClientInterface
	tickets   []Ticket
	mergeable Mergeability
}

// ListIssuesByLabel lists the open tickets with a label, in the order GitHub
// gave the tickets.
func (s *shadowGitHub) ListIssuesByLabel(_ context.Context, label string) ([]analyzer.Issue, error) {
	var issues []analyzer.Issue
	for _, t := range s.tickets {
		if t.Closed || !slices.Contains(t.Labels, label) {
			continue
		}
		issue := analyzer.Issue{Number: t.Number, Body: t.Body, State: "OPEN"}
		for _, name := range t.Labels {
			issue.Labels = append(issue.Labels, analyzer.Label{Name: name})
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (s *shadowGitHub) PRMergeable(ctx context.Context, pr int) (string, error) {
	return s.mergeable(ctx, pr)
}

func (s *shadowGitHub) RemoveLabel(context.Context, int, string) error  { return errShadowWrites }
func (s *shadowGitHub) AddLabel(context.Context, int, string) error     { return errShadowWrites }
func (s *shadowGitHub) CloseIssue(context.Context, int) error           { return errShadowWrites }
func (s *shadowGitHub) ReopenIssue(context.Context, int) error          { return errShadowWrites }
func (s *shadowGitHub) CommentIssue(context.Context, int, string) error { return errShadowWrites }

// mergeability asks GitHub once per pull request: the engine and its shadow
// are told the same.
func (e *Engine) mergeability() Mergeability {
	type answer struct {
		state string
		err   error
	}
	asked := map[int]answer{}
	return func(ctx context.Context, pr int) (string, error) {
		if a, ok := asked[pr]; ok {
			return a.state, a.err
		}
		state, err := e.GitHub.Mergeable(ctx, pr)
		asked[pr] = answer{state, err}
		return state, err
	}
}

// attempts are the engine's counts, by pull request, for the shadow's
// analyzer: the times a review was tried again and a conflict was sent to
// be resolved. A pull request no lane has was tried none.
func (e *Engine) attempts(kind string, pr int) (int, bool) {
	for _, l := range e.live() {
		if l.PR != pr {
			continue
		}
		switch kind {
		case "review":
			return l.Attempts.Reviews, true
		case "conflict":
			return l.Attempts.Conflicts, true
		}
	}
	return 0, true
}

// compare asks the shadow what the analyzer decides, compares the engine's
// decision with it, and writes the comparison to the run's events.
func (e *Engine) compare(ctx context.Context, mine *analyzer.Decision, mergeable Mergeability) Comparison {
	c := Comparison{Engine: mine}
	tickets, err := e.GitHub.Tickets(ctx)
	if err == nil {
		c.Analyzer, err = e.Shadow(ctx, tickets, mergeable, e.attempts)
	}
	if err != nil {
		c.Error = err.Error()
	} else {
		c.Same, c.Known = e.judge(mine, c.Analyzer, tickets)
	}
	level, issue := trace.LevelInfo, 0
	if mine != nil {
		issue = mine.IssueNumber
	}
	if c.Differs() {
		level = trace.LevelWarn
		e.logf("shadow: the engine decided %s; the Principal's workflow would have decided %s", decided(mine), decided(c.Analyzer))
	}
	if err := e.Journal.Note(TypeShadow, level, issue, c); err != nil {
		e.logf("shadow: the comparison could not be written: %v", err)
	}
	return c
}

// judge says whether two decisions are the same, and when they are not,
// which known difference they are.
func (e *Engine) judge(mine, theirs *analyzer.Decision, tickets []Ticket) (same bool, known string) {
	if mine != nil && theirs == nil && mine.NextAction == analyzer.ActionCheckResult {
		l := e.Journal.Lane(mine.IssueNumber)
		i := slices.IndexFunc(tickets, func(t Ticket) bool { return t.Number == mine.IssueNumber })
		if l != nil && l.State == Implementing && l.Resolve == "" && i >= 0 && !slices.Contains(tickets[i].Labels, e.Names.InProgress) {
			return false, KnownCheckFirst
		}
	}
	if mine != nil && mine.NextAction == ActionMerge {
		return false, KnownQueue
	}
	if theirs != nil && theirs.IssueNumber != 0 && (mine == nil || *mine != *theirs) {
		if l := e.Journal.Lane(theirs.IssueNumber); l != nil && l.State == Queued {
			return false, KnownQueue
		}
	}
	if mine == nil || theirs == nil {
		return mine == nil && theirs == nil, ""
	}
	if *mine == *theirs {
		return true, ""
	}
	// The same thing done to another ticket the analyzer cannot tell from
	// it (the same labels of the workflow): the order.
	if mine.NextAction == theirs.NextAction && mine.ExitReason == theirs.ExitReason && mine.MergeIssue == theirs.MergeIssue && mine.IssueNumber != theirs.IssueNumber {
		if a, b := e.workflowLabels(tickets, mine.IssueNumber), e.workflowLabels(tickets, theirs.IssueNumber); a != "" && a == b {
			return false, KnownOrder
		}
	}
	return false, ""
}

// decided is a decision of the comparison as a person reads it.
func decided(d *analyzer.Decision) string {
	if d == nil {
		return "nothing of the tickets in hand (the next ticket is picked)"
	}
	return d.NextAction + describe(d)
}

// workflowLabels are the workflow's labels of a ticket of the snapshot, as
// one string; "" is no such ticket.
func (e *Engine) workflowLabels(tickets []Ticket, issue int) string {
	i := slices.IndexFunc(tickets, func(t Ticket) bool { return t.Number == issue })
	if i < 0 {
		return ""
	}
	var labels []string
	for _, label := range managed(e.Names) {
		if slices.Contains(tickets[i].Labels, label) {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ",")
}

// ShadowRun is how the decisions of the last run that had a shadow
// compared, as `awkit status` shows it.
type ShadowRun struct {
	Started time.Time `json:"started"`
	// Steps are the decisions compared; Same of them were the same, Known
	// differed as is known, Failed could not be compared.
	Steps  int `json:"steps"`
	Same   int `json:"same"`
	Known  int `json:"known"`
	Failed int `json:"failed"`
	// Differences are the comparisons that differed as nothing known
	// explains, the latest last.
	Differences []ShadowDifference `json:"differences"`
}

// ShadowDifference is one decision the two did not agree on.
type ShadowDifference struct {
	At         time.Time  `json:"at"`
	Comparison Comparison `json:"comparison"`
}

// LastShadow reads how the last run of the engine compared with its
// shadow. ok is false when the last run had none.
func LastShadow(root string) (run ShadowRun, ok bool, err error) {
	// Only the starts of runs and the comparisons are decoded: the engine's
	// file holds every transition of every run, and grows with them. A
	// project the engine has not run has no file.
	file := filepath.Join(root, filepath.FromSlash(layout.StateDir), "events", Session+".jsonl")
	err = scanEvents(file, [][]byte{[]byte(TypeRun), []byte(TypeShadow)}, func(ev trace.Event) {
		if ev.Component != Component {
			return
		}
		switch ev.Type {
		case TypeRun:
			var data struct {
				Event string `json:"event"`
			}
			if decode(ev.Data, &data) == nil && data.Event == "started" {
				run = ShadowRun{Started: ev.Timestamp}
			}
		case TypeShadow:
			var c Comparison
			if decode(ev.Data, &c) != nil {
				return
			}
			run.Steps++
			switch {
			case c.Error != "":
				run.Failed++
			case c.Same:
				run.Same++
			case c.Known != "":
				run.Known++
			default:
				run.Differences = append(run.Differences, ShadowDifference{At: ev.Timestamp, Comparison: c})
			}
		}
	})
	if err != nil {
		return ShadowRun{}, false, err
	}
	// No difference is an empty list, not none: a page reads it as a list.
	if run.Differences == nil {
		run.Differences = []ShadowDifference{}
	}
	return run, run.Steps > 0, nil
}

// decode reads an event's data into v.
func decode(data any, v any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// Decided is a decision of a comparison as a person reads it.
func Decided(d *analyzer.Decision) string {
	return decided(d)
}
