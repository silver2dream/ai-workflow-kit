package engine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

var names = config.DefaultLabels()

func TestProject(t *testing.T) {
	for _, c := range []struct {
		lane   Lane
		labels string
		closed bool
	}{
		{Lane{State: Blocked}, "ai-task", false},
		{Lane{State: Ready}, "ai-task", false},
		{Lane{State: Rework}, "ai-task", false},
		{Lane{State: Implementing}, "ai-task,in-progress", false},
		{Lane{State: Implementing, Resolve: ResolveConflict}, "ai-task,in-progress", false},
		{Lane{State: Checking}, "ai-task,in-progress", false},
		{Lane{State: Reviewing}, "ai-task,pr-ready", false},
		{Lane{State: Queued}, "ai-task,pr-ready", false},
		{Lane{State: ReviewRetry}, "ai-task,review-failed", false},
		{Lane{State: Resolving, Resolve: ResolveConflict}, "ai-task,merge-conflict", false},
		{Lane{State: Resolving, Resolve: ResolveRebase}, "ai-task,needs-rebase", false},
		{Lane{State: AwaitingApproval}, "ai-task,needs-human-review", false},
		{Lane{State: NeedsYou, Why: WhyWorkerFailed}, "ai-task,worker-failed", false},
		{Lane{State: NeedsYou, Why: WhyGuardBlocked}, "ai-task,needs-human-review", false},
		{Lane{State: Paused, Resume: Reviewing}, "ai-task,pr-ready", false},
		{Lane{State: Merged}, "ai-task", true},
		{Lane{State: Completed}, "completed", true},
		{Lane{State: Cancelled}, "", false},
	} {
		got := Project(c.lane, names)
		if strings.Join(got.Labels, ",") != c.labels || got.Closed != c.closed {
			t.Errorf("%s/%s: %v closed=%v, want %s closed=%v", c.lane.State, c.lane.Resolve+c.lane.Why, got.Labels, got.Closed, c.labels, c.closed)
		}
	}
}

func TestDiff(t *testing.T) {
	// A person's own labels stay; the workflow's are brought to the lane's.
	ops := Diff(Lane{State: Reviewing}, []string{"ai-task", "in-progress", "bug", "needs-human-review"}, false, names)
	if !reflect.DeepEqual(ops, Ops{Add: []string{"pr-ready"}, Remove: []string{"in-progress", "needs-human-review"}}) {
		t.Errorf("reviewing: %+v", ops)
	}
	if ops := Diff(Lane{State: Merged}, []string{"ai-task", "pr-ready"}, false, names); !reflect.DeepEqual(ops, Ops{Remove: []string{"pr-ready"}, Close: true}) {
		t.Errorf("merged: %+v", ops)
	}
	if ops := Diff(Lane{State: Completed}, []string{"ai-task", "in-progress"}, false, names); !reflect.DeepEqual(ops, Ops{Add: []string{"completed"}, Remove: []string{"ai-task", "in-progress"}, Close: true}) {
		t.Errorf("completed: %+v", ops)
	}
	if ops := Diff(Lane{State: Ready}, []string{"ai-task"}, true, names); !reflect.DeepEqual(ops, Ops{Reopen: true}) {
		t.Errorf("reopened: %+v", ops)
	}
	// Cancelled: no longer a ticket, and left open or closed as the person left it.
	if ops := Diff(Lane{State: Cancelled}, []string{"ai-task", "pr-ready", "bug"}, true, names); !reflect.DeepEqual(ops, Ops{Remove: []string{"ai-task", "pr-ready"}}) {
		t.Errorf("cancelled: %+v", ops)
	}
	if !Diff(Lane{State: Implementing}, []string{"in-progress", "ai-task"}, false, names).Empty() {
		t.Error("a ticket that shows its lane needs changing")
	}
}

func TestInfer(t *testing.T) {
	for _, c := range []struct {
		labels         string
		closed, landed bool
		want           Inferred
		ok             bool
	}{
		{"ai-task", false, false, Inferred{State: Ready}, true},
		{"ai-task,in-progress", false, false, Inferred{State: Implementing}, true},
		// In the order the Principal reads them: a worker at work first.
		{"ai-task,pr-ready,in-progress", false, false, Inferred{State: Implementing}, true},
		{"ai-task,pr-ready,needs-human-review", false, false, Inferred{State: Reviewing}, true},
		{"ai-task,review-failed", false, false, Inferred{State: ReviewRetry}, true},
		{"ai-task,merge-conflict", false, false, Inferred{State: Resolving, Resolve: ResolveConflict}, true},
		{"ai-task,needs-rebase", false, false, Inferred{State: Resolving, Resolve: ResolveRebase}, true},
		{"ai-task,worker-failed", false, false, Inferred{State: NeedsYou, Why: WhyWorkerFailed}, true},
		{"ai-task,needs-human-review", false, false, Inferred{State: NeedsYou, Why: WhyNeedsHumanReview}, true},
		{"completed", true, false, Inferred{State: Completed}, true},
		{"ai-task", true, true, Inferred{State: Merged}, true},
		{"ai-task", true, false, Inferred{State: Cancelled}, true},
		{"bug", false, false, Inferred{}, false},
	} {
		got, ok := Infer(strings.Split(c.labels, ","), c.closed, c.landed, names)
		if got != c.want || ok != c.ok {
			t.Errorf("%s closed=%v landed=%v: %+v %v, want %+v %v", c.labels, c.closed, c.landed, got, ok, c.want, c.ok)
		}
	}
}

// What the engine shows is what it reads back, for every state labels can
// tell apart.
func TestProjectThenInfer(t *testing.T) {
	for _, l := range []Lane{
		{State: Ready}, {State: Implementing}, {State: Reviewing}, {State: ReviewRetry},
		{State: Resolving, Resolve: ResolveConflict}, {State: Resolving, Resolve: ResolveRebase},
		{State: NeedsYou, Why: WhyWorkerFailed}, {State: NeedsYou, Why: WhyNeedsHumanReview},
		{State: Merged}, {State: Completed},
	} {
		p := Project(l, names)
		got, ok := Infer(p.Labels, p.Closed, l.State == Merged, names)
		if !ok || got.State != l.State || got.Resolve != l.Resolve || got.Why != l.Why {
			t.Errorf("%+v → %v → %+v", l, p.Labels, got)
		}
	}
}

// A project that names its labels otherwise is shown in its own names.
func TestProject_TheProjectsNames(t *testing.T) {
	own := names
	own.Task, own.PRReady = "awk", "ready-for-review"
	if got := Project(Lane{State: Reviewing}, own); strings.Join(got.Labels, ",") != "awk,ready-for-review" {
		t.Errorf("labels = %v", got.Labels)
	}
	if got, ok := Infer([]string{"awk", "ready-for-review"}, false, false, own); !ok || got.State != Reviewing {
		t.Errorf("infer = %+v %v", got, ok)
	}
}
