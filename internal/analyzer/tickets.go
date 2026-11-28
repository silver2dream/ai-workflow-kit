package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// ticketLookup answers, for one Decide call, what GitHub knows about the
// tickets a decision depends on. Every answer is fetched at most once.
type ticketLookup struct {
	a        *Analyzer
	statuses map[int]IssueStatus // the tickets, by issue number; nil until loaded
	loadErr  error
	loaded   bool
}

func (a *Analyzer) newTicketLookup() *ticketLookup {
	return &ticketLookup{a: a}
}

// taskStatuses returns the status of every issue that carries the task label.
func (l *ticketLookup) taskStatuses(ctx context.Context) (map[int]IssueStatus, error) {
	if !l.loaded {
		l.statuses, l.loadErr = l.a.GHClient.TaskIssueStatuses(ctx, l.a.taskLabel())
		l.loaded = true
	}
	return l.statuses, l.loadErr
}

// status returns the status of one issue: a ticket from the list, or any
// other issue asked for by number.
func (l *ticketLookup) status(ctx context.Context, issue int) (IssueStatus, error) {
	if statuses, err := l.taskStatuses(ctx); err == nil {
		if status, ok := statuses[issue]; ok {
			return status, nil
		}
	}
	return l.a.GHClient.IssueStatus(ctx, issue)
}

func (a *Analyzer) taskLabel() string {
	if a.Config != nil && a.Config.GitHub.Labels.Task != "" {
		return a.Config.GitHub.Labels.Task
	}
	return DefaultLabels().Task
}

// Skip says whether a ticket that could start now is passed over: the engine
// passes over one whose paths overlap the paths a lane holds (design.md
// §5.3). meta is the ticket's block; found is false for a ticket without one.
// It is asked last: a ticket it does not pass over is picked, so the tickets
// picked after it can be kept off what it holds.
type Skip func(issue int, meta ticket.Meta, found bool) bool

// Picks is what Candidates found among the pending tickets.
type Picks struct {
	// Tickets are the tickets to start, in the order to start them: lowest
	// number first, each with its dependencies done.
	Tickets []*Decision
	// Then is what was decided past them, or instead of them: a ticket
	// handed to a person, a dependency closed without its work landing that
	// is done again; and when there is no ticket to start, what the run comes
	// to (every ticket waits for a dependency, all done, none actionable, a
	// lookup that failed). nil when tickets were passed over and nothing else
	// was decided.
	Then *Decision
	// Skipped are the tickets that could start and were passed over.
	Skipped []int
}

// walk looks at the pending tickets, lowest number first, for up to max that
// can start, passing over those skip says to. It stops at max, and at the
// first ticket it hands to a person or dependency it does again, so that it
// changes nothing on GitHub for a ticket past those (as picking one ticket
// never did). apiErrors counts the lookups that failed and kept a ticket from
// being judged.
//
// A ticket whose block cannot be used is handed to a person: one that cannot
// be read, names a repo the config does not have, or depends on itself or on
// an issue that does not exist. A dependency that was closed without its work
// landing is reopened and dispatched first. When every pending ticket waits
// for a dependency, Then says so instead of calling the run complete.
func (a *Analyzer) walk(ctx context.Context, pending []Issue, lookup *ticketLookup, max int, skip Skip) (picks Picks, apiErrors int) {
	sort.Slice(pending, func(i, j int) bool { return pending[i].Number < pending[j].Number })

	firstWaiting := 0
	for _, issue := range pending {
		// An issue whose pull request is already merged was left open by an
		// interrupted run: close it and move on.
		if prNumber := a.extractPRNumberForIssue(issue.Number, issue.Body); prNumber > 0 {
			if merged, err := a.GHClient.IsPRMerged(ctx, prNumber); err == nil && merged {
				_ = a.GHClient.CloseIssue(ctx, issue.Number)
				MarkLanded(a.StateRoot, issue.Number)
				continue
			}
		}

		meta, found, err := ticket.Parse(issue.Body)
		if err == nil && found {
			err = a.ticketProblem(issue.Number, meta)
		}
		if err != nil {
			picks.Then = a.rejectTicket(ctx, issue.Number, err)
			return picks, apiErrors
		}

		waitingFor, unknown := 0, false
		for _, dependency := range meta.DependsOn {
			done, redo, err := a.dependencyDone(ctx, dependency, lookup)
			if errors.Is(err, ErrIssueNotFound) {
				problem := fmt.Errorf("the awk-ticket block is not valid: depends_on lists #%d, and the repository has no such issue", dependency)
				picks.Then = a.rejectTicket(ctx, issue.Number, problem)
				return picks, apiErrors
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "[analyzer] warning: #%d depends on #%d, whose state could not be read: %v\n", issue.Number, dependency, err)
				apiErrors++
				unknown = true
				break
			}
			if redo != nil {
				picks.Then = redo
				return picks, apiErrors
			}
			if !done {
				waitingFor = dependency
				break
			}
		}
		if unknown {
			// Not known to be blocked: the caller reports the failed lookup.
			continue
		}
		if waitingFor != 0 {
			fmt.Fprintf(os.Stderr, "[analyzer] #%d waits for #%d\n", issue.Number, waitingFor)
			if firstWaiting == 0 {
				firstWaiting = issue.Number
			}
			continue
		}

		if skip != nil && skip(issue.Number, meta, found) {
			picks.Skipped = append(picks.Skipped, issue.Number)
			continue
		}
		picks.Tickets = append(picks.Tickets, &Decision{NextAction: ActionDispatchWorker, IssueNumber: issue.Number})
		if len(picks.Tickets) >= max {
			return picks, apiErrors
		}
	}

	// Every ticket that is not started waits for a dependency: a ticket
	// passed over does not, and one to start makes the run go on.
	if firstWaiting != 0 && len(picks.Tickets) == 0 && len(picks.Skipped) == 0 {
		picks.Then = &Decision{NextAction: ActionNone, IssueNumber: firstWaiting, ExitReason: ReasonBlockedByDependencies}
	}
	return picks, apiErrors
}

// dependencyDone reports whether the issue a ticket depends on is done.
//
// Done means closed, and not closed as "not planned". For an issue this
// workflow worked on, the work must also have landed: a closed issue with no
// merged pull request is a false completion, and redo is the decision to do
// it again.
func (a *Analyzer) dependencyDone(ctx context.Context, issue int, lookup *ticketLookup) (done bool, redo *Decision, err error) {
	status, err := lookup.status(ctx, issue)
	if err != nil {
		return false, nil, err
	}
	if !status.Closed() {
		return false, nil, nil
	}
	if status.Dropped() {
		fmt.Fprintf(os.Stderr, "[analyzer] #%d was closed as not planned; a ticket that depends on it cannot start\n", issue)
		return false, nil, nil
	}
	landed, err := a.landed(ctx, issue)
	if err != nil {
		return false, nil, err
	}
	if !landed {
		redo, err := a.redoFalseCompletion(ctx, issue)
		return false, redo, err
	}
	return true, nil, nil
}

// falseCompletion looks for a ticket that was closed although its work never
// landed, and returns the decision to do it again. It is asked before a run is
// called complete.
//
// Only issues whose closing has to be backed by a merge are looked at (see
// unconfirmed).
func (a *Analyzer) falseCompletion(ctx context.Context, lookup *ticketLookup) (redo *Decision, err error) {
	statuses, err := lookup.taskStatuses(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []int
	for issue, status := range statuses {
		if status.Closed() && !status.Dropped() && a.unconfirmed(issue) {
			candidates = append(candidates, issue)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Ints(candidates)

	// One call answers for most of them; the rest are asked one by one.
	if merged, err := a.GHClient.MergedIssueBranches(ctx); err == nil {
		for _, issue := range candidates {
			if merged[issueBranch(issue)] {
				MarkLanded(a.StateRoot, issue)
			}
		}
	}
	for _, issue := range candidates {
		landed, err := a.landed(ctx, issue)
		if err != nil {
			return nil, err
		}
		if landed {
			continue
		}
		// A limited run leaves an issue outside it alone: look further.
		if redo, err := a.redoFalseCompletion(ctx, issue); redo != nil || err != nil {
			return redo, err
		}
	}
	return nil, nil
}

// redoFalseCompletion reopens an issue that was closed without its work
// landing and returns the decision to dispatch it. An issue that cannot be
// reopened is an error: dispatching a closed issue fails, again and again.
// A run limited to another ticket leaves the issue as it is, and decides
// nothing: whatever depends on it waits.
func (a *Analyzer) redoFalseCompletion(ctx context.Context, issue int) (*Decision, error) {
	if a.outOfScope(issue) {
		fmt.Fprintf(os.Stderr, "[analyzer] issue #%d is closed but has no merged pull request; this run is limited to #%d and leaves it as it is\n", issue, a.only)
		return nil, nil
	}
	fmt.Fprintf(os.Stderr, "[analyzer] false completion: issue #%d is closed but has no merged pull request; reopening and dispatching it again\n", issue)
	if err := a.GHClient.ReopenIssue(ctx, issue); err != nil {
		return nil, fmt.Errorf("issue #%d was closed without its work landing, and could not be reopened: %w", issue, err)
	}
	// It is a ticket again, and one that is not done.
	var done []string
	if label := a.Config.GitHub.Labels.Completed; label != "" {
		done = []string{label}
	}
	a.updateIssueLabels(ctx, issue, []string{a.taskLabel()}, done)
	return &Decision{NextAction: ActionDispatchWorker, IssueNumber: issue}, nil
}

// ticketProblem checks a ticket's block against what only the workflow knows:
// the ticket's own number, the repos of the config and their folders.
func (a *Analyzer) ticketProblem(issue int, meta ticket.Meta) error {
	for _, dependency := range meta.DependsOn {
		if dependency == issue {
			return fmt.Errorf("the awk-ticket block is not valid: depends_on lists the ticket itself (#%d)", issue)
		}
	}
	if !a.Config.HasTicketRepo(meta.Repo) {
		return fmt.Errorf("the awk-ticket block is not valid: repo %q is not a repo of "+layout.ConfigFile+" (it has: %s)",
			meta.Repo, strings.Join(a.Config.TicketRepos(), ", "))
	}
	if err := meta.CheckRepo(a.Config.TicketRepoDir(meta.Repo)); err != nil {
		return fmt.Errorf("the awk-ticket block is not valid: %w", err)
	}
	return nil
}

// rejectTicket hands a ticket the workflow cannot work on to a person: the
// issue gets a comment that says what is wrong, and the label that keeps the
// workflow off it until someone has looked.
func (a *Analyzer) rejectTicket(ctx context.Context, issue int, problem error) *Decision {
	fmt.Fprintf(os.Stderr, "[analyzer] ticket #%d cannot be used: %v\n", issue, problem)
	label := a.Config.GitHub.Labels.NeedsHumanReview
	comment := fmt.Sprintf("AWK cannot work on this ticket: %v.\n\n"+
		"Correct the `%s` block in the issue body, then remove the `%s` label and the workflow will pick the ticket up again.",
		problem, ticket.Info, label)
	if err := a.GHClient.CommentIssue(ctx, issue, comment); err != nil {
		fmt.Fprintf(os.Stderr, "[analyzer] warning: could not comment on #%d: %v\n", issue, err)
	}
	a.updateIssueLabels(ctx, issue, []string{label}, nil)
	return &Decision{NextAction: ActionNone, IssueNumber: issue, ExitReason: ReasonInvalidTicket}
}

func issueBranch(issue int) string {
	return fmt.Sprintf("feat/ai-issue-%d", issue)
}

// resultNoChanges is what a Worker reports when its ticket needed no change.
const resultNoChanges = "success_no_changes"

// workerResult returns the status the Worker last reported for an issue. worked
// is false when this workflow never dispatched a Worker for it.
func (a *Analyzer) workerResult(issue int) (status string, worked bool) {
	data, err := os.ReadFile(layout.Result(a.StateRoot, issue))
	if os.IsNotExist(err) {
		return "", false
	}
	var result struct {
		Status string `json:"status"`
	}
	if err == nil {
		_ = json.Unmarshal(data, &result)
	}
	return result.Status, true
}

// unconfirmed reports whether the closing of an issue still has to be backed by
// a merged pull request: this workflow worked on it, its Worker changed
// something, and no merge is on record.
//
// An issue the workflow never worked on is not its business: how it got closed
// is up to whoever closed it. An issue whose Worker found nothing to change has
// nothing to land.
func (a *Analyzer) unconfirmed(issue int) bool {
	status, worked := a.workerResult(issue)
	return worked && status != resultNoChanges && !a.knownLanded(issue)
}

// landed reports whether a closed issue is really done: its work was merged,
// or its closing needs no merge behind it (see unconfirmed). A merge that was
// found is remembered, so GitHub is asked once per issue.
func (a *Analyzer) landed(ctx context.Context, issue int) (bool, error) {
	if !a.unconfirmed(issue) {
		return true, nil
	}
	merged, err := a.GHClient.HasMergedPR(ctx, issueBranch(issue))
	if err != nil {
		return false, err
	}
	if merged {
		MarkLanded(a.StateRoot, issue)
	}
	return merged, nil
}

func landedMarker(stateRoot string, issue int) string {
	return filepath.Join(stateRoot, ".ai", "state", "landed", fmt.Sprintf("issue-%d", issue))
}

// KnownLanded says whether an issue's work is known to be on the integration
// branch: its merge was recorded (MarkLanded). It asks GitHub nothing.
func KnownLanded(stateRoot string, issue int) bool {
	_, err := os.Stat(landedMarker(stateRoot, issue))
	return err == nil
}

func (a *Analyzer) knownLanded(issue int) bool {
	_, err := os.Stat(landedMarker(a.StateRoot, issue))
	return err == nil
}

// MarkLanded records that the pull request of an issue was merged. The review
// calls it when it merges; the decision engine then knows without asking
// GitHub. Recording it twice is harmless, and a failure to record only costs
// one question to GitHub later.
func MarkLanded(stateRoot string, issue int) {
	marker := landedMarker(stateRoot, issue)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(marker, nil, 0o644)
}
