package engine

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)

// walk applies transitions in order, failing the test at the first that
// does not apply.
func walk(t *testing.T, issue int, steps ...Transition) *Lane {
	t.Helper()
	var lane *Lane
	for i, s := range steps {
		s.Issue = issue
		if lane != nil {
			s.From = lane.State
		}
		if s.At.IsZero() {
			s.At = t0.Add(time.Duration(i) * time.Minute)
		}
		next, err := Apply(lane, s)
		if err != nil {
			t.Fatalf("step %d (%s → %s): %v", i, s.From, s.To, err)
		}
		lane = next
	}
	return lane
}

func to(state State, cause Cause) Transition { return Transition{To: state, Cause: cause} }

func TestLane_FromTheQueueToTheMerge(t *testing.T) {
	lane := walk(t, 12,
		to(Blocked, CauseOpened),
		to(Ready, CauseUnblocked),
		to(Implementing, CauseDispatched),
		Transition{To: Reviewing, Cause: CauseImplemented, PR: 30},
		to(Merged, CauseMerged),
	)
	if lane.State != Merged || lane.PR != 30 || lane.Issue != 12 || !lane.Since.Equal(t0.Add(4*time.Minute)) {
		t.Errorf("lane = %+v", lane)
	}
	if !lane.State.Terminal() {
		t.Error("merged is not over")
	}
}

// The workflow as it is today: a change requested sends the ticket back to
// the implementer; a review that could not be completed is tried again; a
// conflict is resolved by an implementer and reviewed again.
func TestLane_TheWaysBack(t *testing.T) {
	lane := walk(t, 5,
		to(Ready, CauseOpened),
		to(Implementing, CauseDispatched),
		Transition{To: Reviewing, Cause: CauseImplemented, PR: 9},
		to(Rework, CauseChangesWanted),
		to(Implementing, CauseDispatched),
		to(Reviewing, CauseImplemented),
		to(ReviewRetry, CauseReviewFailed),
		to(Reviewing, CauseReviewRetried),
		Transition{To: Resolving, Cause: CauseConflicting, Resolve: ResolveRebase},
		to(Implementing, CauseDispatched),
	)
	if lane.State != Implementing || lane.Resolve != ResolveRebase || lane.PR != 9 {
		t.Fatalf("resolving a rebase: %+v", lane)
	}
	if lane.Attempts.Reviews != 1 || lane.Attempts.Conflicts != 0 {
		t.Errorf("attempts = %+v: a rebase is no conflict", lane.Attempts)
	}
	lane2, err := Apply(lane, Transition{Issue: 5, From: Implementing, To: Resolving, Cause: CauseResolveFailed, At: t0})
	if err != nil || lane2.Resolve != ResolveRebase {
		t.Fatalf("a failed resolution keeps what it resolves: %+v, %v", lane2, err)
	}
	lane3, err := Apply(lane2, Transition{Issue: 5, From: Resolving, To: Reviewing, Cause: CauseResolved, At: t0})
	if err != nil || lane3.Resolve != "" {
		t.Fatalf("resolved: %+v, %v", lane3, err)
	}
}

func TestLane_Attempts(t *testing.T) {
	lane := walk(t, 3,
		to(Ready, CauseOpened),
		to(Implementing, CauseDispatched),
		to(Rework, CauseImplementFail),
		to(Implementing, CauseDispatched),
		to(Rework, CauseImplementFail),
	)
	if lane.Attempts.Failures != 2 {
		t.Fatalf("failures = %d", lane.Attempts.Failures)
	}
	lane = walk(t, 3,
		to(Ready, CauseOpened),
		to(Implementing, CauseDispatched),
		to(Rework, CauseImplementFail),
		to(Implementing, CauseDispatched),
		Transition{To: Reviewing, Cause: CauseImplemented, PR: 4},
		to(Resolving, CauseConflicting),
		to(Implementing, CauseDispatched),
		to(Reviewing, CauseResolved),
	)
	if lane.State != Reviewing {
		t.Fatalf("lane = %+v", lane)
	}
	if lane.Attempts.Failures != 0 || lane.Attempts.Conflicts != 1 {
		t.Errorf("attempts = %+v: a success clears the failures, a conflict counts", lane.Attempts)
	}
}

func TestLane_NeedsYouAndRelease(t *testing.T) {
	lane := walk(t, 8,
		to(Ready, CauseOpened),
		to(Implementing, CauseDispatched),
		Transition{To: NeedsYou, Cause: CauseEscalated, Why: WhyGuardBlocked, Note: "an injection in the review comments"},
	)
	if lane.Why != WhyGuardBlocked || lane.Note == "" {
		t.Fatalf("lane = %+v", lane)
	}
	released, err := Apply(lane, Transition{Issue: 8, From: NeedsYou, To: Ready, Cause: CauseReleased, At: t0})
	if err != nil || released.Why != "" || released.Note != "" {
		t.Errorf("released: %+v, %v", released, err)
	}
	if _, err := Apply(lane, Transition{Issue: 8, From: NeedsYou, To: Merged, Cause: CauseReleased, At: t0}); err == nil {
		t.Error("released straight to merged")
	}
}

func TestLane_PauseAndResume(t *testing.T) {
	lane := walk(t, 6,
		to(Ready, CauseOpened),
		to(Implementing, CauseDispatched),
		Transition{To: Reviewing, Cause: CauseImplemented, PR: 2},
		Transition{To: Resolving, Cause: CauseConflicting, Resolve: ResolveRebase},
		Transition{To: Paused, Cause: CausePaused, Note: "wait for the release"},
	)
	if lane.Resume != Resolving || lane.Resolve != ResolveRebase || lane.Note == "" {
		t.Fatalf("paused: %+v", lane)
	}
	if _, err := Apply(lane, Transition{Issue: 6, From: Paused, To: Ready, Cause: CauseResumed, At: t0}); err == nil {
		t.Error("resumed into another state than it was paused in")
	}
	back, err := Apply(lane, Transition{Issue: 6, From: Paused, To: Resolving, Cause: CauseResumed, At: t0})
	if err != nil || back.Resolve != ResolveRebase || back.Resume != "" || back.Note != "" {
		t.Errorf("resumed: %+v, %v", back, err)
	}
}

func TestLane_Priority(t *testing.T) {
	lane := walk(t, 4, to(Ready, CauseOpened), Transition{To: Ready, Cause: CausePrioritized, Priority: 1})
	if lane.Priority != 1 {
		t.Fatalf("priority = %d", lane.Priority)
	}
	lane, _ = Apply(lane, Transition{Issue: 4, From: Ready, To: Ready, Cause: CausePrioritized, At: t0})
	if lane.Priority != 0 {
		t.Errorf("a priority cleared: %d", lane.Priority)
	}
	if _, err := Apply(lane, Transition{Issue: 4, From: Ready, To: Implementing, Cause: CausePrioritized, At: t0}); err == nil {
		t.Error("a priority moved the lane")
	}
}

func TestLane_ReopenedStartsOver(t *testing.T) {
	lane := walk(t, 9,
		to(Ready, CauseOpened),
		to(Implementing, CauseDispatched),
		to(Rework, CauseImplementFail),
		to(Cancelled, CauseCancelled),
		to(Ready, CauseReopened),
	)
	if lane.State != Ready || lane.Attempts != (Attempts{}) {
		t.Errorf("reopened: %+v", lane)
	}
}

func TestApply_Refuses(t *testing.T) {
	ready := &Lane{Issue: 1, State: Ready}
	merged := &Lane{Issue: 1, State: Merged}
	for name, c := range map[string]struct {
		lane *Lane
		t    Transition
	}{
		"skips the work":           {ready, Transition{Issue: 1, From: Ready, To: Merged, Cause: CauseMerged}},
		"wrong cause":              {ready, Transition{Issue: 1, From: Ready, To: Implementing, Cause: CauseImplemented}},
		"stale from":               {ready, Transition{Issue: 1, From: Reviewing, To: Merged, Cause: CauseMerged}},
		"opened twice":             {ready, Transition{Issue: 1, To: Ready, Cause: CauseOpened}},
		"no lane to move":          {nil, Transition{Issue: 1, From: Ready, To: Implementing, Cause: CauseDispatched}},
		"over is over":             {merged, Transition{Issue: 1, From: Merged, To: NeedsYou, Cause: CauseEscalated}},
		"cancel what is merged":    {merged, Transition{Issue: 1, From: Merged, To: Cancelled, Cause: CauseCancelled}},
		"paused into paused":       {&Lane{Issue: 1, State: Paused, Resume: Ready}, Transition{Issue: 1, From: Paused, To: Paused, Cause: CausePaused}},
		"escalate needs-you again": {&Lane{Issue: 1, State: NeedsYou}, Transition{Issue: 1, From: NeedsYou, To: NeedsYou, Cause: CauseEscalated}},
		"unknown state":            {ready, Transition{Issue: 1, From: Ready, To: "somewhere", Cause: CauseResumed}},
	} {
		if _, err := Apply(c.lane, c.t); err == nil {
			t.Errorf("%s: applied", name)
		}
	}
}

// Every edge of the table is a move Apply makes, from every state it names.
func TestEdges_AllApply(t *testing.T) {
	for _, e := range edges {
		froms := []State{e.from}
		if e.from == anyLive {
			froms = nil
			for _, s := range States {
				if !s.Terminal() && s != e.to {
					froms = append(froms, s)
				}
			}
		}
		for _, from := range froms {
			var lane *Lane
			if from != "" {
				lane = &Lane{Issue: 2, State: from}
			}
			if _, err := Apply(lane, Transition{Issue: 2, From: from, To: e.to, Cause: e.cause, At: t0}); err != nil {
				t.Errorf("%s → %s (%s): %v", from, e.to, e.cause, err)
			}
		}
	}
	if !strings.Contains(show(""), "none") {
		t.Error("show")
	}
}
