package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// A lane that waits for a person stops alone (design.md §6, task 2.4).

// A decision that hands one lane to a person moves that lane, and the round
// goes on: here a review tried as often as it may be, while another ticket
// waits to start. The run ends once nothing else is left, for the person.
func TestEngine_AHandedLaneLetsTheRoundGoOn(t *testing.T) {
	f := newEngineFixture(t, task(3, "review-failed"), task(5))
	f.an.prs = map[int]int{3: 30}
	f.an.next = []*analyzer.Decision{dispatch(5), {NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonNoActionableTasks}}
	at := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	for _, tr := range []Transition{
		{Issue: 3, To: Reviewing, Cause: CauseImported, PR: 30, At: at},
		{Issue: 3, From: Reviewing, To: ReviewRetry, Cause: CauseReviewFailed, At: at},
		{Issue: 3, From: ReviewRetry, To: Reviewing, Cause: CauseReviewRetried, At: at},
		{Issue: 3, From: Reviewing, To: ReviewRetry, Cause: CauseReviewFailed, At: at},
		{Issue: 3, From: ReviewRetry, To: Reviewing, Cause: CauseReviewRetried, At: at},
		{Issue: 3, From: Reviewing, To: ReviewRetry, Cause: CauseReviewFailed, At: at},
		{Issue: 3, From: ReviewRetry, To: Reviewing, Cause: CauseReviewRetried, At: at},
		{Issue: 3, From: Reviewing, To: ReviewRetry, Cause: CauseReviewFailed, At: at},
	} {
		if _, err := f.e.Journal.Record(tr); err != nil {
			t.Fatal(err)
		}
	}
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(50))}
	f.review = []func(Lane) Transition{approve}
	f.merge = []func(Lane) Transition{goTo(Merged, CauseMerged)}

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "implement #5, review #5, merge #5" {
		t.Errorf("steps: %s", got)
	}
	if l := f.lane(3); l.State != NeedsYou || l.Why != WhyReviewMaxRetries {
		t.Errorf("#3: %+v", l)
	}
	if l := f.lane(5); l.State != Merged {
		t.Errorf("#5: %+v", l)
	}
	// The engine told the person of #3; nobody of #5.
	if len(f.gh.comments) != 1 || !strings.HasPrefix(f.gh.comments[0], "#3 ") {
		t.Errorf("comments: %q", f.gh.comments)
	}
}

// A ticket the analyzer hands to a person (its block cannot be used) stops
// alone: the next ticket is picked in the same round.
func TestEngine_AnUnusableTicketStopsAlone(t *testing.T) {
	f := newEngineFixture(t, task(4), task(6))
	f.an.next = []*analyzer.Decision{
		{NextAction: analyzer.ActionNone, IssueNumber: 4, ExitReason: analyzer.ReasonInvalidTicket},
		dispatch(6),
		{NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonNoActionableTasks},
	}
	f.implement = []func(Lane) Transition{goTo(Completed, CauseNoChanges)}

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "implement #6" {
		t.Errorf("steps: %s", got)
	}
	if l := f.lane(4); l.State != NeedsYou || l.Why != WhyInvalidTicket {
		t.Errorf("#4: %+v", l)
	}
	if f.an.asked != 3 {
		t.Errorf("the analyzer was asked %d times", f.an.asked)
	}
}

// A ticket the analyzer hands to a person (and tells them of) stops alone
// too; the engine does not tell them again.
func TestEngine_ATicketTheAnalyzerHandsOver(t *testing.T) {
	f := newEngineFixture(t, task(4), task(6))
	f.an.next = []*analyzer.Decision{
		{NextAction: analyzer.ActionNone, IssueNumber: 4, ExitReason: analyzer.ReasonNeedsHumanReview},
		dispatch(6),
		{NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonNoActionableTasks},
	}
	f.implement = []func(Lane) Transition{goTo(Completed, CauseNoChanges)}

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "implement #6" {
		t.Errorf("steps: %s", got)
	}
	if l := f.lane(4); l.State != NeedsYou || l.Why != WhyNeedsHumanReview {
		t.Errorf("#4: %+v", l)
	}
	if len(f.gh.comments) != 0 {
		t.Errorf("comments: %q", f.gh.comments)
	}
}

// Every ticket waiting for another hands none to a person: the run ends as
// it did, and the analyzer is not asked again for the same answer.
func TestEngine_EveryTicketWaits(t *testing.T) {
	f := newEngineFixture(t, task(3))
	f.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionNone, IssueNumber: 3, ExitReason: analyzer.ReasonBlockedByDependencies}}
	if end := f.run(); end != analyzer.ReasonBlockedByDependencies {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if l := f.lane(3); l.State != Blocked || f.an.asked != 1 {
		t.Errorf("#3: %+v; the analyzer asked %d times", l, f.an.asked)
	}
}

// The shadow's lane that stops alone is the analyzer stopping the run on a
// ticket labeled for a person, whose lane waits for one: not another stop,
// nor one on a lane that does not wait.
func TestShadow_ALaneThatStopsAlone(t *testing.T) {
	f := newEngineFixture(t, task(7))
	f.e.Shadow = nil
	if _, err := f.e.Journal.Record(Transition{Issue: 7, To: NeedsYou, Cause: CauseImported, Why: WhyPerson, At: time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Journal.Record(Transition{Issue: 8, To: Ready, Cause: CauseImported, At: time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	stop := func(issue int, reason string) *analyzer.Decision {
		return &analyzer.Decision{NextAction: analyzer.ActionNone, IssueNumber: issue, ExitReason: reason}
	}
	for _, c := range []struct {
		name   string
		mine   *analyzer.Decision
		theirs *analyzer.Decision
		known  string
	}{
		{"needs a person", nil, stop(7, analyzer.ReasonNeedsHumanReview), KnownLaneStop},
		{"a Worker failed", dispatch(8), stop(7, analyzer.ReasonWorkerFailed), KnownLaneStop},
		{"another stop", nil, stop(7, analyzer.ReasonReviewMaxRetries), ""},
		{"a lane that does not wait", nil, stop(8, analyzer.ReasonNeedsHumanReview), ""},
		{"the same", stop(7, analyzer.ReasonNeedsHumanReview), stop(7, analyzer.ReasonNeedsHumanReview), ""},
	} {
		same, known := f.e.judge(c.mine, c.theirs, nil)
		if known != c.known || (c.name == "the same") != same {
			t.Errorf("%s: same %v, known %q", c.name, same, known)
		}
	}
}

// A decision that moves no lane is one the run stops on: the lane already
// waits for a person, and the decision is not made again and again.
func TestEngine_ADecisionThatMovesNoLane(t *testing.T) {
	f := newEngineFixture(t, task(4, "needs-human-review"))
	f.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionNone, IssueNumber: 4, ExitReason: analyzer.ReasonInvalidTicket}}
	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if f.an.asked != 1 {
		t.Errorf("the analyzer was asked %d times", f.an.asked)
	}
}

// Tickets in a circle are found at the first pick, while another ticket can
// start: they are handed to a person, and the other is written. A ticket of
// the circle that started already is left to go on.
func TestEngine_ACircleFoundAtOnce(t *testing.T) {
	on := func(n int, deps string) Ticket {
		return withBody(task(n), "## Summary\nx\n\n```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [part"+fmt.Sprint(n)+"/**]\nrisk: low\ndepends_on: ["+deps+"]\n```\n")
	}
	f := newEngineFixture(t, on(1, "2"), on(2, "1"), on(3, ""), on(4, "5"), on(5, "4"))
	f.gh.tickets[5].Labels = append(f.gh.tickets[5].Labels, "pr-ready")
	f.an.prs = map[int]int{5: 50}
	f.an.pending = f.gh.pendingOf
	f.implement = []func(Lane) Transition{goTo(Completed, CauseNoChanges)}
	f.review = []func(Lane) Transition{func(l Lane) Transition {
		// #5's review: the circle is still there, and it was not handed over.
		if l.State != Reviewing {
			t.Errorf("#5 is %s while it is reviewed", l.State)
		}
		return approve(l)
	}}
	f.merge = []func(Lane) Transition{goTo(Merged, CauseMerged)}

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "review #5, merge #5, implement #3" {
		t.Errorf("steps: %s", got)
	}
	for _, issue := range []int{1, 2, 4} {
		if l := f.lane(issue); l.State != NeedsYou || l.Why != WhyDependencyCycle {
			t.Errorf("#%d: %+v", issue, l)
		}
	}
	if l := f.lane(5); l.State != Merged {
		t.Errorf("#5, which started, was handed over: %+v", l)
	}
}

// How a run ends that has nothing more to do: for a person when lanes wait
// for one, a Worker that failed when each of them is one.
func TestForAPerson(t *testing.T) {
	at := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name  string
		lanes []Transition
		end   string
		want  string
	}{
		{"none waits", nil, analyzer.ReasonNoActionableTasks, analyzer.ReasonNoActionableTasks},
		{"complete", nil, EndComplete, EndComplete},
		{"a Worker failed", []Transition{{Issue: 1, To: NeedsYou, Cause: CauseImported, Why: WhyWorkerFailed}}, analyzer.ReasonNoActionableTasks, analyzer.ReasonWorkerFailed},
		{"a Worker failed, and a review", []Transition{{Issue: 1, To: NeedsYou, Cause: CauseImported, Why: WhyWorkerFailed}, {Issue: 2, To: NeedsYou, Cause: CauseImported, Why: WhyReviewMaxRetries}}, analyzer.ReasonBlockedByDependencies, analyzer.ReasonNeedsHumanReview},
		{"a person marked one", []Transition{{Issue: 2, To: NeedsYou, Cause: CauseImported, Why: WhyPerson}}, EndComplete, analyzer.ReasonNeedsHumanReview},
		{"a Worker failed, the end said a person", []Transition{{Issue: 1, To: NeedsYou, Cause: CauseImported, Why: WhyWorkerFailed}}, analyzer.ReasonNeedsHumanReview, analyzer.ReasonWorkerFailed},
		{"paused", []Transition{{Issue: 1, To: Ready, Cause: CauseImported}, {Issue: 1, From: Ready, To: Paused, Cause: CausePaused}}, analyzer.ReasonNoActionableTasks, analyzer.ReasonNeedsHumanReview},
		{"a stop is a stop", []Transition{{Issue: 2, To: NeedsYou, Cause: CauseImported, Why: WhyPerson}}, analyzer.ReasonStopRequested, analyzer.ReasonStopRequested},
		{"too many failures", []Transition{{Issue: 2, To: NeedsYou, Cause: CauseImported, Why: WhyPerson}}, analyzer.ReasonMaxConsecutiveFailures, analyzer.ReasonMaxConsecutiveFailures},
		{"an unusable ticket that waits", []Transition{{Issue: 2, To: NeedsYou, Cause: CauseImported, Why: WhyInvalidTicket}}, analyzer.ReasonInvalidTicket, analyzer.ReasonNeedsHumanReview},
		{"GitHub", []Transition{{Issue: 2, To: NeedsYou, Cause: CauseImported, Why: WhyPerson}}, analyzer.ReasonGitHubAPIError, analyzer.ReasonGitHubAPIError},
	} {
		f := newEngineFixture(t)
		f.e.Shadow = nil
		for _, tr := range c.lanes {
			tr.At = at
			if _, err := f.e.Journal.Record(tr); err != nil {
				t.Fatal(err)
			}
		}
		if got := f.e.forAPerson(c.end); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
