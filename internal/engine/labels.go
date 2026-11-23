package engine

import (
	"sort"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// GitHub shows each lane's state to people as labels on its ticket, the
// labels the Principal's workflow has always used, so that a person and the
// workflow's other commands see the same thing. Only the engine writes them;
// a person who changes them gives the engine an instruction (reconciliation,
// task 1.6), and Infer reads what they say.

// Projection is what GitHub should show of a lane: the workflow's labels the
// ticket has, and whether it is closed. Labels the workflow does not own are
// left as they are.
type Projection struct {
	Labels []string
	Closed bool
}

// managed are the labels the workflow owns.
func managed(names config.Labels) []string {
	return []string{names.Task, names.InProgress, names.PRReady, names.ReviewFailed, names.MergeConflict, names.NeedsRebase, names.NeedsHumanReview, names.WorkerFailed, names.Completed}
}

// Project is what GitHub should show of a lane.
func Project(l Lane, names config.Labels) Projection {
	state := l.State
	if state == Paused && l.Resume != "" {
		// Pausing changes no label: the person is told in a comment.
		state = l.Resume
	}
	task := names.Task
	with := func(extra ...string) []string { return append([]string{task}, extra...) }
	switch state {
	case Blocked, Ready, Rework, Paused:
		return Projection{Labels: with()}
	case Implementing, Checking:
		return Projection{Labels: with(names.InProgress)}
	case Reviewing, Queued:
		return Projection{Labels: with(names.PRReady)}
	case ReviewRetry:
		return Projection{Labels: with(names.ReviewFailed)}
	case Resolving:
		if l.Resolve == ResolveRebase {
			return Projection{Labels: with(names.NeedsRebase)}
		}
		return Projection{Labels: with(names.MergeConflict)}
	case AwaitingApproval:
		return Projection{Labels: with(names.NeedsHumanReview)}
	case NeedsYou:
		if l.Why == WhyWorkerFailed {
			return Projection{Labels: with(names.WorkerFailed)}
		}
		return Projection{Labels: with(names.NeedsHumanReview)}
	case Merged:
		return Projection{Labels: with(), Closed: true}
	case Completed:
		return Projection{Labels: []string{names.Completed}, Closed: true}
	case Cancelled:
		// No longer a ticket: no run takes it up.
		return Projection{Labels: []string{}}
	}
	return Projection{Labels: with()}
}

// Ops are the changes that bring a ticket to a lane's projection.
type Ops struct {
	Add    []string
	Remove []string
	Close  bool
	Reopen bool
}

// Empty says whether nothing needs changing.
func (o Ops) Empty() bool {
	return len(o.Add) == 0 && len(o.Remove) == 0 && !o.Close && !o.Reopen
}

// Diff is what to change on a ticket that has labels and is closed or open,
// for it to show the lane: the workflow's labels it lacks, those it has and
// should not, and whether to close or reopen it. A cancelled lane leaves the
// ticket open or closed as the person left it.
func Diff(l Lane, labels []string, closed bool, names config.Labels) Ops {
	want := Project(l, names)
	have := map[string]bool{}
	for _, label := range labels {
		have[label] = true
	}
	wanted := map[string]bool{}
	var ops Ops
	for _, label := range want.Labels {
		wanted[label] = true
		if !have[label] {
			ops.Add = append(ops.Add, label)
		}
	}
	for _, label := range managed(names) {
		if have[label] && !wanted[label] {
			ops.Remove = append(ops.Remove, label)
		}
	}
	sort.Strings(ops.Add)
	sort.Strings(ops.Remove)
	if l.State != Cancelled {
		ops.Close = want.Closed && !closed
		ops.Reopen = !want.Closed && closed
	}
	return ops
}

// Inferred is a lane read from a ticket's labels: where the Principal's
// workflow, or a person, left it.
type Inferred struct {
	State   State
	Resolve string
	Why     string
}

// Infer reads a lane from a ticket's labels and whether it is closed, in the
// order the Principal's workflow reads them (internal/analyzer): a ticket
// with a worker at work first, then one in review, and so on. ok is false
// for an open issue that is no ticket. landed says whether a closed ticket's
// work is on the integration branch.
func Infer(labels []string, closed, landed bool, names config.Labels) (in Inferred, ok bool) {
	has := map[string]bool{}
	for _, label := range labels {
		has[label] = true
	}
	if closed {
		switch {
		case has[names.Completed]:
			return Inferred{State: Completed}, true
		case landed:
			return Inferred{State: Merged}, true
		default:
			return Inferred{State: Cancelled}, true
		}
	}
	switch {
	case has[names.InProgress]:
		return Inferred{State: Implementing}, true
	case has[names.PRReady]:
		return Inferred{State: Reviewing}, true
	case has[names.ReviewFailed]:
		return Inferred{State: ReviewRetry}, true
	case has[names.MergeConflict]:
		return Inferred{State: Resolving, Resolve: ResolveConflict}, true
	case has[names.NeedsRebase]:
		return Inferred{State: Resolving, Resolve: ResolveRebase}, true
	case has[names.WorkerFailed]:
		return Inferred{State: NeedsYou, Why: WhyWorkerFailed}, true
	case has[names.NeedsHumanReview]:
		return Inferred{State: NeedsYou, Why: WhyNeedsHumanReview}, true
	case has[names.Task]:
		return Inferred{State: Ready}, true
	}
	return Inferred{}, false
}
