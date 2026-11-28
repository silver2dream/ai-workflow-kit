package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/lanenote"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
)

// The owner's gate (design.md §10, task 2.5).

// A change the review approves that is plainly high risk waits for its
// owner: the lane says why, what the owner is shown is kept, and each
// reason is a verdict. One that is not goes to the queue.
func TestReview_AHighRiskChangeWaitsForItsOwner(t *testing.T) {
	queued := reviewer.SubmitReviewResult{Result: "approved", Next: reviewer.NextQueued, Head: "abc123"}

	f := newReviewFixture(t)
	f.result = queued
	if tr := f.review(); tr.To != Queued || tr.Approved != "abc123" {
		t.Fatalf("ordinary work: went %s: %s", tr.To, tr.Note)
	}

	f = newReviewFixture(t)
	f.result = queued
	f.diff += "diff --git a/go.mod b/go.mod\n--- a/go.mod\n+++ b/go.mod\n@@ -1 +1 @@\n+require x v1\n"
	tr := f.review()
	if tr.To != AwaitingApproval || tr.Cause != CauseApprovalNeeded || tr.Approved != "abc123" || f.lane.Approved != "abc123" {
		t.Fatalf("a dependency: %+v", tr)
	}
	if !strings.HasPrefix(f.lane.Note, "high risk: it changes what the project depends on") || !strings.Contains(f.lane.Note, "(go.mod)") {
		t.Errorf("note: %q", f.lane.Note)
	}
	req, ok := ReadApprovalRequest(f.root, 12)
	if !ok || req.PR != 30 || req.Head != "abc123" || req.Score != 9 || req.Summary == "" || len(req.Reasons) != 1 || req.Reasons[0].Rule != "high-risk/1#dependency" {
		t.Errorf("what the owner is shown: %+v, %v", req, ok)
	}

	// The ticket says its risk is high.
	f = newReviewFixture(t)
	f.result = queued
	f.rc.Ticket += "\n```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [score.go]\nrisk: high\n```\n"
	if tr := f.review(); tr.To != AwaitingApproval || !strings.Contains(tr.Note, "its ticket says its risk is high") {
		t.Errorf("a ticket of high risk: %+v", tr)
	}

	// The project's own paths, from its configuration.
	f = newReviewFixture(t)
	f.result = queued
	writeFile(t, f.root+"/.ai/config.yaml", reviewConfig+"guardrails:\n  high_risk: {paths: [\"score.go\"]}\n")
	if tr := f.review(); tr.To != AwaitingApproval || !strings.Contains(tr.Note, "marks as high risk") {
		t.Errorf("the project's paths: %+v", tr)
	}

	// A review that does not approve is not held to the risk.
	f = newReviewFixture(t)
	f.result = reviewer.SubmitReviewResult{Result: "changes_requested", Next: reviewer.NextRework}
	f.diff += "diff --git a/go.mod b/go.mod\n"
	if tr := f.review(); tr.To != Rework {
		t.Errorf("sent back: %+v", tr)
	}
}

// awaiting records a lane whose change waits for its owner's approval.
func awaiting(t *testing.T, f *engineFixture, issue, pr int) {
	t.Helper()
	at := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	for _, tr := range []Transition{
		{Issue: issue, To: Reviewing, Cause: CauseImported, PR: pr, At: at},
		{Issue: issue, From: Reviewing, To: AwaitingApproval, Cause: CauseApprovalNeeded, Approved: "abc", Note: "high risk: it changes what the project depends on (go.mod)", At: at},
	} {
		if _, err := f.e.Journal.Record(tr); err != nil {
			t.Fatal(err)
		}
	}
}

// The engine asks the owner on the issue, once; a lane that waits for its
// owner stops alone, and the run ends for the person when nothing else is
// left.
func TestEngine_AChangeThatWaitsForItsOwner(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12), {NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonNoActionableTasks}}
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.review = []func(Lane) Transition{goTo(AwaitingApproval, CauseApprovalNeeded, func(t *Transition) {
		t.Approved, t.Note = "abc", "high risk: it changes what the project depends on (go.mod)"
	})}
	f.e.Review = func(ctx context.Context, l Lane) (Transition, error) {
		f.steps = append(f.steps, "review #12")
		return f.review[0](l), nil
	}
	f.e.Shadow = nil

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if l := f.lane(12); l.State != AwaitingApproval || l.Approved != "abc" || l.Note == "" {
		t.Errorf("lane: %+v", l)
	}
	if got := f.gh.labels(12); got != "ai-task,needs-human-review" {
		t.Errorf("labels: %s", got)
	}
	if len(f.gh.comments) != 1 || !strings.Contains(f.gh.comments[0], "要你核可才會合併") || !strings.Contains(f.gh.comments[0], "it changes what the project depends on (go.mod)") ||
		!strings.Contains(f.gh.comments[0], "awkit lane approve 12") {
		t.Errorf("comments: %q", f.gh.comments)
	}
}

// The owner approves: the lane goes to the queue, its approval with it, and
// is merged. A lane that does not wait is not moved.
func TestEngine_TheOwnerApproves(t *testing.T) {
	f := newEngineFixture(t, task(12, "needs-human-review"), task(13))
	f.e.Shadow = nil
	awaiting(t, f, 12, 30)
	reply := func(c Command) Reply {
		t.Helper()
		sent, err := Send(f.e.Root, c)
		if err != nil {
			t.Fatal(err)
		}
		f.e.takeCommands(nil)
		r, ok := ReplyOf(f.e.Root, sent.ID)
		if !ok {
			t.Fatalf("%s was not answered", c.Kind)
		}
		return r
	}
	if r := reply(Command{Kind: CommandApprove, Issue: 13, By: "awkit lane"}); r.Result != Rejected || !strings.Contains(r.Reason, "#13 has no lane") {
		t.Errorf("a ticket without a lane: %+v", r)
	}
	if r := reply(Command{Kind: CommandApprove, Issue: 12, By: "awkit lane"}); r.Result != Applied || r.Reason != "it goes to the merge queue" {
		t.Fatalf("approved: %+v", r)
	}
	if l := f.lane(12); l.State != Queued || l.Approved != "abc" || l.Note != "" {
		t.Errorf("lane: %+v", l)
	}
	if r := reply(Command{Kind: CommandApprove, Issue: 12}); r.Result != Rejected || !strings.Contains(r.Reason, "#12 is queued") {
		t.Errorf("approved again: %+v", r)
	}
	f.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionAllComplete}}
	f.merge = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	if end := f.run(); end != EndComplete || strings.Join(f.steps, ", ") != "merge #12" {
		t.Errorf("ended %q, steps %v\n%s", end, f.steps, f.log.String())
	}
}

// The owner returns the change: the lane goes back to its implementer, and
// the reason is kept for the next dispatch as the owner's note.
func TestEngine_TheOwnerReturns(t *testing.T) {
	f := newEngineFixture(t, task(12, "needs-human-review"))
	f.e.Shadow = nil
	awaiting(t, f, 12, 30)
	sent, err := Send(f.e.Root, Command{Kind: CommandReject, Issue: 12, By: "the workbench", Args: map[string]string{"note": "Pin the version instead of upgrading it."}})
	if err != nil {
		t.Fatal(err)
	}
	f.e.takeCommands(nil)
	if r, _ := ReplyOf(f.e.Root, sent.ID); r.Result != Applied || r.Reason != "it goes back to its implementer, with the reason" {
		t.Fatalf("returned: %+v", r)
	}
	if l := f.lane(12); l.State != Rework || l.Approved != "" {
		t.Errorf("lane: %+v", l)
	}
	notes, err := lanenote.Read(f.e.Root, 12)
	if err != nil || len(notes) != 1 || notes[0].Kind != lanenote.Returned || notes[0].By != "the workbench" || notes[0].Text != "Pin the version instead of upgrading it." {
		t.Errorf("notes: %+v, %v", notes, err)
	}
	if got := f.gh.labels(12); got != "ai-task" {
		t.Errorf("labels: %s", got)
	}
}

// A run takes what was asked of it before its first round: an approval given
// while no run went is not lost to a run that finds nothing else to do.
func TestServe_TakesTheInboxFirst(t *testing.T) {
	f := newEngineFixture(t, task(12, "needs-human-review"))
	f.e.Shadow = nil
	awaiting(t, f, 12, 30)
	if _, err := Send(f.e.Root, Command{Kind: CommandApprove, Issue: 12, By: "awkit lane"}); err != nil {
		t.Fatal(err)
	}
	f.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonNoActionableTasks}}
	f.merge = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	end, err := Serve(t.Context(), f.e, ServeOptions{Beat: time.Hour, Poll: time.Hour})
	if err != nil || end != analyzer.ReasonNoActionableTasks {
		t.Fatalf("ended %q, %v\n%s", end, err, f.log.String())
	}
	if l := f.lane(12); l.State != Merged {
		t.Errorf("lane: %+v\n%s", l, f.log.String())
	}
}

// An approval while the run waits for its steps starts the merge at once,
// not when a step ends.
func TestEngine_AnApprovalWakesTheRun(t *testing.T) {
	p := newParallelFixture(t, 1, apart(1)...)
	p.e.Shadow = nil
	p.holdMerges = true
	p.gh.mu.Lock()
	p.gh.tickets[2] = &Ticket{Number: 2, Labels: []string{"ai-task", "needs-human-review"}, Body: body("part2/**")}
	p.gh.mu.Unlock()
	awaiting(t, p.engineFixture, 2, 102)
	done := p.runAsync(t.Context())

	p.expect("implement #1")
	if _, err := Send(p.e.Root, Command{Kind: CommandApprove, Issue: 2, By: "awkit lane"}); err != nil {
		t.Fatal(err)
	}
	p.e.takeCommands(nil)
	p.expect("merge #2")
	p.let("merge #2", "implement #1")
	p.expect("review #1")
	p.let("review #1")
	p.expect("merge #1")
	p.let("merge #1")
	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
}
