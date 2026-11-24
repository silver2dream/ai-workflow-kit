// Package engine is the deterministic engine that takes over from the LLM
// Principal (docs/developer/specs/agent-engine). This file is its unit of
// work: a lane, one ticket on its way from the queue to the integration
// branch, and the states it moves through.
//
// A lane's history is its transitions, kept in the event log; its state is
// what they add up to. GitHub's labels show the state to people
// (labels.go) and are not where it is kept.
package engine

import (
	"fmt"
	"slices"
	"time"
)

// State is where a lane is.
type State string

const (
	// Blocked: a ticket it depends on has not landed.
	Blocked State = "blocked"
	// Ready: it can start, when a place is free.
	Ready State = "ready"
	// Implementing: an implementer works on it, writing the ticket or
	// resolving its pull request's conflict with the base (Resolve says).
	Implementing State = "implementing"
	// Checking: the engine runs the deterministic checks on its change.
	Checking State = "checking"
	// Reviewing: a reviewer reviews its pull request.
	Reviewing State = "reviewing"
	// ReviewRetry: the review could not be completed (the checks failed, its
	// evidence did not hold, the reviewer failed): it is reviewed again.
	ReviewRetry State = "review_retry"
	// Rework: it waits to be implemented again, with what the review or a
	// failure said.
	Rework State = "rework"
	// AwaitingApproval: the change is high risk and waits for the owner.
	AwaitingApproval State = "awaiting_approval"
	// Queued: approved, it waits its turn to be merged.
	Queued State = "queued"
	// Resolving: its pull request conflicts with the base, or is behind it,
	// and waits to be resolved (Resolve says which).
	Resolving State = "resolving"
	// Merged: its pull request is merged; the ticket is closed.
	Merged State = "merged"
	// Completed: done with nothing to merge (the implementer found nothing to
	// change); the ticket is closed.
	Completed State = "completed"
	// NeedsYou: only a person can decide what happens next (Why says why).
	NeedsYou State = "needs_you"
	// Paused: a person paused it; it goes back to Resume.
	Paused State = "paused"
	// Cancelled: a person cancelled it, or closed its ticket.
	Cancelled State = "cancelled"
)

// States lists every state, in the order a lane goes through them.
var States = []State{Blocked, Ready, Implementing, Checking, Reviewing, ReviewRetry, Rework, AwaitingApproval, Queued, Resolving, Merged, Completed, NeedsYou, Paused, Cancelled}

// Terminal says whether a lane is over: nothing moves it but a reopened
// ticket.
func (s State) Terminal() bool {
	return s == Merged || s == Completed || s == Cancelled
}

// Known says whether s is a state of a lane.
func (s State) Known() bool {
	return slices.Contains(States, s)
}

// What resolving does to a pull request.
const (
	ResolveConflict = "conflict"
	ResolveRebase   = "rebase"
)

// Why a lane needs a person. Each is a reason the Principal's workflow
// stops on today (internal/analyzer), or one the engine adds.
const (
	WhyWorkerFailed     = "worker_failed"        // the implementer failed as often as it may
	WhyNeedsHumanReview = "needs_human_review"   // the workflow handed it to a person
	WhyReviewMaxRetries = "review_max_retries"   // the review could not be completed as often as it may
	WhyConflictRetries  = "conflict_max_retries" // the conflict could not be resolved as often as it may
	WhyNoPR             = "no_pr"                // the pull request of a lane that has one cannot be found
	WhyMergeBlocked     = "merge_blocked"        // GitHub refused the merge (protection, checks)
	WhyGuardBlocked     = "guard_blocked"        // a guardrail blocked what the implementer was to read
	WhyGuardOutput      = "guard_output"         // a guardrail blocked what was to be written to GitHub
	WhyInvalidTicket    = "invalid_ticket"       // the ticket's awk-ticket block cannot be read
	WhyDependencyCycle  = "dependency_cycle"     // its dependencies go round in a circle
	WhyPerson           = "person"               // a person marked it on GitHub
	WhyAgentRefused     = "agent_refused"        // an agent's backend cannot be held to its role's settings: it was not run
	WhyWorkerLost       = "worker_lost"          // its ticket says a Worker is at work, and none has left a result in the time a Worker may take
)

// Attempts are the counts a lane's limits are checked against, as the
// Principal's workflow counts them today.
type Attempts struct {
	// Failures counts the implementer's failures since it last succeeded.
	Failures int `json:"failures,omitempty"`
	// Reviews counts the times the review was tried again (ReviewRetry →
	// Reviewing).
	Reviews int `json:"reviews,omitempty"`
	// Conflicts counts the times a conflict of the pull request was sent to
	// be resolved.
	Conflicts int `json:"conflicts,omitempty"`
}

// Lane is a ticket's way to the integration branch, as its transitions left
// it.
type Lane struct {
	Issue int   `json:"issue"`
	State State `json:"state"`
	// PR is the lane's pull request, once there is one.
	PR int `json:"pr,omitempty"`
	// Resolve is what resolving does (ResolveConflict, ResolveRebase): set
	// while a lane is Resolving, and while an implementer resolves it.
	Resolve string `json:"resolve,omitempty"`
	// Why says why a lane needs a person (the Why constants).
	Why string `json:"why,omitempty"`
	// Note says the same for a person to read, or why a lane was paused or
	// cancelled.
	Note string `json:"note,omitempty"`
	// Resume is the state a paused lane goes back to.
	Resume   State    `json:"resume,omitempty"`
	Attempts Attempts `json:"attempts"`
	// Priority orders the lanes that can start: lower first; 0 is none.
	Priority int `json:"priority,omitempty"`
	// Since is when the lane came to its state.
	Since time.Time `json:"since"`
	// Seq is the sequence number, in the event log, of its last transition.
	Seq int `json:"seq"`
}

// Cause is what moved a lane.
type Cause string

const (
	CauseOpened         Cause = "opened"           // a ticket became a lane
	CauseImported       Cause = "imported"         // a ticket the engine did not have became a lane where its labels left it (Infer)
	CauseUnblocked      Cause = "unblocked"        // what it depends on landed
	CauseBlocked        Cause = "blocked"          // something it depends on is open again
	CauseDispatched     Cause = "dispatched"       // an implementer took it
	CauseImplemented    Cause = "implemented"      // the implementer's pull request is up
	CauseNoChanges      Cause = "no_changes"       // the implementer found nothing to change
	CauseImplementFail  Cause = "implement_failed" // the implementer failed, or was lost
	CauseResolveFailed  Cause = "resolve_failed"   // resolving the pull request failed
	CauseChecked        Cause = "checked"          // the engine's checks passed
	CauseCheckFailed    Cause = "check_failed"     // the engine's checks failed
	CauseApproved       Cause = "approved"         // the review approved
	CauseChangesWanted  Cause = "changes_requested"
	CauseReviewFailed   Cause = "review_failed"   // the review could not be completed
	CauseReviewRetried  Cause = "review_retried"  // the review is tried again
	CauseConflicting    Cause = "conflicting"     // the pull request conflicts with the base, or is behind it
	CauseResolved       Cause = "resolved"        // the conflict is resolved
	CauseApprovalNeeded Cause = "approval_needed" // high risk: the owner approves
	CauseOwnerApproved  Cause = "owner_approved"
	CauseOwnerReturned  Cause = "owner_returned"
	CauseMerged         Cause = "merged"
	CauseLanded         Cause = "landed"    // its work landed outside the lane's steps: a person merged it, or an earlier run did
	CauseEscalated      Cause = "escalated" // it needs a person (Why says why)
	CauseReleased       Cause = "released"  // a person took it out of NeedsYou
	CausePaused         Cause = "paused"
	CauseResumed        Cause = "resumed"
	CauseCancelled      Cause = "cancelled"
	CauseReopened       Cause = "reopened" // a closed ticket was opened again
	CausePrioritized    Cause = "prioritized"
)

// Transition is one move of a lane: what the event log keeps.
type Transition struct {
	Issue int   `json:"issue"`
	From  State `json:"from,omitempty"` // "" when the lane is opened
	To    State `json:"to"`
	Cause Cause `json:"cause"`
	// PR, Resolve, Why, Note and Priority set the lane's fields of the same
	// names; zero leaves them as they are, except as Apply says.
	PR       int       `json:"pr,omitempty"`
	Resolve  string    `json:"resolve,omitempty"`
	Why      string    `json:"why,omitempty"`
	Note     string    `json:"note,omitempty"`
	Priority int       `json:"priority,omitempty"`
	At       time.Time `json:"at"`
}

// edge is a move the state machine allows.
type edge struct {
	from  State
	to    State
	cause Cause
}

// anyLive stands for every state that is not terminal.
const anyLive State = "*"

// edges are the moves of a lane (design.md §6). A cause from anyLive may
// come from any state that is not terminal.
var edges = []edge{
	{"", Blocked, CauseOpened},
	{"", Ready, CauseOpened},
	{Blocked, Ready, CauseUnblocked},
	{Ready, Blocked, CauseBlocked},
	{Rework, Blocked, CauseBlocked},

	{Ready, Implementing, CauseDispatched},
	{Rework, Implementing, CauseDispatched},
	{Resolving, Implementing, CauseDispatched},

	{Implementing, Checking, CauseImplemented},
	{Implementing, Reviewing, CauseImplemented},
	{Implementing, Completed, CauseNoChanges},
	{Implementing, Rework, CauseImplementFail},
	{Implementing, Resolving, CauseResolveFailed},
	// An implementer that resolved the pull request's conflict, as the
	// Principal's workflow has it done: the pull request is reviewed again.
	{Implementing, Reviewing, CauseResolved},

	{Checking, Reviewing, CauseChecked},
	{Checking, Rework, CauseCheckFailed},

	{Reviewing, Queued, CauseApproved},
	{Reviewing, Merged, CauseMerged},
	{Reviewing, Rework, CauseChangesWanted},
	{Reviewing, ReviewRetry, CauseReviewFailed},
	{ReviewRetry, Reviewing, CauseReviewRetried},
	{Reviewing, Resolving, CauseConflicting},
	{Reviewing, AwaitingApproval, CauseApprovalNeeded},

	{AwaitingApproval, Queued, CauseOwnerApproved},
	{AwaitingApproval, Rework, CauseOwnerReturned},

	{Queued, Merged, CauseMerged},
	{Queued, Resolving, CauseConflicting},
	{Queued, Rework, CauseCheckFailed},
	{Resolving, Queued, CauseResolved},
	{Resolving, Reviewing, CauseResolved},

	{anyLive, NeedsYou, CauseEscalated},
	{anyLive, Merged, CauseLanded},
	{NeedsYou, Ready, CauseReleased},
	{NeedsYou, Rework, CauseReleased},
	{NeedsYou, Reviewing, CauseReleased},
	{NeedsYou, Resolving, CauseReleased},

	{anyLive, Paused, CausePaused},
	{anyLive, Cancelled, CauseCancelled},

	{Merged, Ready, CauseReopened},
	{Merged, Rework, CauseReopened},
	{Completed, Ready, CauseReopened},
	{Cancelled, Ready, CauseReopened},
	{Cancelled, Rework, CauseReopened},
}

// Allowed says whether the state machine has the move.
func Allowed(from, to State, cause Cause) bool {
	if cause == CausePrioritized {
		return from == to && from != "" && !from.Terminal()
	}
	if cause == CauseResumed {
		return from == Paused && to != Paused && !to.Terminal() && to.Known()
	}
	if cause == CauseImported {
		// A ticket the Principal's workflow, or a person, left somewhere:
		// the lane starts there. A closed ticket is not taken in.
		return from == "" && to.Known() && !to.Terminal() && to != Paused
	}
	for _, e := range edges {
		if e.to != to || e.cause != cause {
			continue
		}
		if e.from == from || (e.from == anyLive && from != "" && !from.Terminal() && from != to) {
			return true
		}
	}
	return false
}

// Apply moves a lane by a transition: the lane it was (nil for a lane that
// is opened) and the one it is now. A move the state machine does not have,
// or one that starts from another state than the lane's, is refused.
func Apply(lane *Lane, t Transition) (*Lane, error) {
	from := State("")
	if lane != nil {
		from = lane.State
	}
	if t.From != from {
		return nil, fmt.Errorf("lane #%d is %s, not %s: %s → %s (%s) does not apply", t.Issue, show(from), show(t.From), show(t.From), t.To, t.Cause)
	}
	if !Allowed(from, t.To, t.Cause) {
		return nil, fmt.Errorf("lane #%d cannot go from %s to %s (%s)", t.Issue, show(from), t.To, t.Cause)
	}
	if t.Cause == CauseResumed && lane.Resume != "" && t.To != lane.Resume {
		return nil, fmt.Errorf("lane #%d was paused in %s, not %s", t.Issue, lane.Resume, t.To)
	}

	next := Lane{Issue: t.Issue}
	if lane != nil {
		next = *lane
	}
	next.State = t.To
	next.Since = t.At
	if t.PR != 0 {
		next.PR = t.PR
	}
	if t.Priority != 0 || t.Cause == CausePrioritized {
		next.Priority = t.Priority
	}

	// What a person is told belongs to the state that needs them.
	switch t.To {
	case NeedsYou, Paused, Cancelled:
		next.Why, next.Note = t.Why, t.Note
	default:
		next.Why, next.Note = "", ""
	}
	if t.To == Paused {
		next.Resume = from
	} else {
		next.Resume = ""
	}

	// Resolving, and an implementer at work on a resolution, know what
	// they resolve; nothing else does.
	switch {
	case t.To == Resolving:
		// A lane that comes back to resolving (resumed, released, failed)
		// still resolves what it did.
		if t.Resolve != "" {
			next.Resolve = t.Resolve
		}
		if next.Resolve == "" {
			next.Resolve = ResolveConflict
		}
	case t.To == Implementing && from == Resolving:
	case t.To == Paused || t.To == NeedsYou:
	default:
		next.Resolve = ""
	}

	switch t.Cause {
	case CauseImplementFail:
		next.Attempts.Failures++
	case CauseImplemented, CauseNoChanges, CauseResolved:
		next.Attempts.Failures = 0
	case CauseReviewRetried:
		next.Attempts.Reviews++
	case CauseConflicting:
		if t.Resolve != ResolveRebase {
			next.Attempts.Conflicts++
		}
	case CauseReopened:
		next.Attempts = Attempts{}
	}
	return &next, nil
}

func show(s State) string {
	if s == "" {
		return "(none)"
	}
	return string(s)
}
