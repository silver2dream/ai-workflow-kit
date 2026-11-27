package engine

import (
	"bytes"
	"context"

	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// fakeGitHub keeps tickets in memory, and what was done to them.
type fakeGitHub struct {
	tickets   map[int]*Ticket
	mergeable map[int]string // by pull request; "" is MERGEABLE
	comments  []string
}

func newFakeGitHub(tickets ...Ticket) *fakeGitHub {
	g := &fakeGitHub{tickets: map[int]*Ticket{}, mergeable: map[int]string{}}
	for i := range tickets {
		t := tickets[i]
		g.tickets[t.Number] = &t
	}
	return g
}

func (g *fakeGitHub) Tickets(ctx context.Context) ([]Ticket, error) {
	var out []Ticket
	for _, t := range g.tickets {
		if !t.Closed && slices.Contains(t.Labels, "ai-task") {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (g *fakeGitHub) Ticket(ctx context.Context, issue int) (Ticket, error) {
	t, ok := g.tickets[issue]
	if !ok {
		return Ticket{}, fmt.Errorf("no issue #%d", issue)
	}
	return *t, nil
}

func (g *fakeGitHub) Edit(ctx context.Context, issue int, ops Ops) error {
	t := g.tickets[issue]
	for _, r := range ops.Remove {
		t.Labels = slices.DeleteFunc(t.Labels, func(l string) bool { return l == r })
	}
	t.Labels = append(t.Labels, ops.Add...)
	sort.Strings(t.Labels)
	if ops.Close {
		t.Closed = true
	}
	if ops.Reopen {
		t.Closed = false
	}
	return nil
}

func (g *fakeGitHub) Comment(ctx context.Context, issue int, body string) error {
	g.comments = append(g.comments, fmt.Sprintf("#%d %s", issue, body))
	return nil
}

func (g *fakeGitHub) Mergeable(ctx context.Context, pr int) (string, error) {
	if s := g.mergeable[pr]; s != "" {
		return s, nil
	}
	return "MERGEABLE", nil
}

func (g *fakeGitHub) labels(issue int) string {
	t := g.tickets[issue]
	s := strings.Join(t.Labels, ",")
	if t.Closed {
		s += " (closed)"
	}
	return s
}

// fakeAnalyzer answers the engine's questions from a script.
type fakeAnalyzer struct {
	precheck *analyzer.Decision
	next     []*analyzer.Decision // NextTicket answers, in turn; the last repeats
	asked    int
	prs      map[int]int
}

func (a *fakeAnalyzer) Precheck() *analyzer.Decision { return a.precheck }
func (a *fakeAnalyzer) NextTicket(ctx context.Context) (*analyzer.Decision, error) {
	a.asked++
	if len(a.next) == 0 {
		return &analyzer.Decision{NextAction: analyzer.ActionAllComplete}, nil
	}
	d := a.next[0]
	if len(a.next) > 1 {
		a.next = a.next[1:]
	}
	return d, nil
}
func (a *fakeAnalyzer) PRNumber(issue int, body string) int { return a.prs[issue] }

type engineFixture struct {
	t     *testing.T
	e     *Engine
	gh    *fakeGitHub
	an    *fakeAnalyzer
	steps []string
	log   bytes.Buffer
	// The steps' answers, by step, in turn; the last repeats.
	implement, check, review, resolve []func(Lane) Transition
}

func dispatch(issue int) *analyzer.Decision {
	return &analyzer.Decision{NextAction: analyzer.ActionDispatchWorker, IssueNumber: issue}
}

func newEngineFixture(t *testing.T, tickets ...Ticket) *engineFixture {
	t.Helper()
	f := &engineFixture{t: t, gh: newFakeGitHub(tickets...), an: &fakeAnalyzer{prs: map[int]int{}}}
	root := t.TempDir()
	j, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	answer := func(name string, answers *[]func(Lane) Transition) Step {
		return func(ctx context.Context, l Lane) (Transition, error) {
			f.steps = append(f.steps, fmt.Sprintf("%s #%d", name, l.Issue))
			// The Worker takes a ticket it is given labeled in-progress for
			// one another Worker is at work on, and does nothing.
			if t := f.gh.tickets[l.Issue]; name == "implement" && t != nil && slices.Contains(t.Labels, "in-progress") {
				f.t.Errorf("#%d is given to the Worker labeled %v", l.Issue, t.Labels)
			}
			if len(*answers) == 0 {
				return Transition{}, fmt.Errorf("no answer for %s", name)
			}
			a := (*answers)[0]
			if len(*answers) > 1 {
				*answers = (*answers)[1:]
			}
			return a(l), nil
		}
	}
	f.e = &Engine{
		Root: root, Journal: j, GitHub: f.gh, Analyzer: f.an, Names: config.DefaultLabels(), MaxReviews: 3,
		Implement: answer("implement", &f.implement), Check: answer("check", &f.check),
		Review: answer("review", &f.review), Resolve: answer("resolve", &f.resolve),
		Log: &f.log, now: func() time.Time { return time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC) },
	}
	shadowed(f)
	return f
}

// to answers a move of the lane, from where it is.
func goTo(state State, cause Cause, set ...func(*Transition)) func(Lane) Transition {
	return func(l Lane) Transition {
		t := Transition{Issue: l.Issue, From: l.State, To: state, Cause: cause}
		for _, s := range set {
			s(&t)
		}
		return t
	}
}

func withPR(pr int) func(*Transition) { return func(t *Transition) { t.PR = pr } }
func withWhy(why string) func(*Transition) {
	return func(t *Transition) { t.Why, t.Note = why, "because" }
}

func (f *engineFixture) run() string {
	f.t.Helper()
	end, err := f.e.Run(context.Background())
	if err != nil {
		f.t.Fatalf("Run: %v\n%s", err, f.log.String())
	}
	return end
}

func (f *engineFixture) lane(issue int) Lane {
	l := f.e.Journal.Lane(issue)
	if l == nil {
		f.t.Fatalf("#%d has no lane", issue)
	}
	return *l
}

func task(n int, labels ...string) Ticket {
	return Ticket{Number: n, Labels: append([]string{"ai-task"}, labels...)}
}

// A ticket from the queue to the integration branch: written, reviewed,
// merged, and the labels follow the lane.
func TestEngine_ATicketToItsMerge(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12), {NextAction: analyzer.ActionAllComplete}}
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.review = []func(Lane) Transition{goTo(Merged, CauseMerged)}

	if end := f.run(); end != EndComplete {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "implement #12, review #12" {
		t.Errorf("steps: %s", got)
	}
	if l := f.lane(12); l.State != Merged || l.PR != 30 {
		t.Errorf("lane: %+v", l)
	}
	if got := f.gh.labels(12); got != "ai-task (closed)" {
		t.Errorf("labels: %s", got)
	}
	if len(f.gh.comments) != 0 {
		t.Errorf("comments: %v", f.gh.comments)
	}
}

// A review that cannot be completed is tried again as often as it may be,
// then the ticket waits for a person, who is told why.
func TestEngine_ReviewTriedAgain(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12)}
	f.e.MaxReviews = 2
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.review = []func(Lane) Transition{goTo(ReviewRetry, CauseReviewFailed)}

	if end := f.run(); end != analyzer.ReasonReviewMaxRetries {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "implement #12, review #12, review #12, review #12" {
		t.Errorf("steps: %s", got)
	}
	l := f.lane(12)
	if l.State != NeedsYou || l.Why != WhyReviewMaxRetries || l.Attempts.Reviews != 2 {
		t.Errorf("lane: %+v", l)
	}
	if got := f.gh.labels(12); got != "ai-task,needs-human-review" {
		t.Errorf("labels: %s", got)
	}
	if len(f.gh.comments) != 1 || !strings.Contains(f.gh.comments[0], "移除 `needs-human-review`") {
		t.Errorf("comments: %v", f.gh.comments)
	}
	// The next run stops on it at once.
	f.steps = nil
	if end := f.run(); end != analyzer.ReasonNeedsHumanReview || len(f.steps) != 0 {
		t.Errorf("the next run: %q, %v", end, f.steps)
	}
}

// A pull request that conflicts with its base is resolved before it is
// reviewed, as often as it may be.
func TestEngine_ConflictBeforeReview(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12), {NextAction: analyzer.ActionAllComplete}}
	f.gh.mergeable[30] = "CONFLICTING"
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.resolve = []func(Lane) Transition{func(l Lane) Transition {
		f.gh.mergeable[30] = "MERGEABLE"
		return goTo(Reviewing, CauseResolved)(l)
	}}
	f.review = []func(Lane) Transition{goTo(Merged, CauseMerged)}

	if end := f.run(); end != EndComplete {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if got := strings.Join(f.steps, ", "); got != "implement #12, resolve #12, review #12" {
		t.Errorf("steps: %s", got)
	}
	if l := f.lane(12); l.Attempts.Conflicts != 1 {
		t.Errorf("conflicts: %+v", l.Attempts)
	}

	// One that still conflicts after as many resolutions as it may have
	// waits for a person.
	f = newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12)}
	f.e.MaxReviews = 2
	f.gh.mergeable[30] = "CONFLICTING"
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.resolve = []func(Lane) Transition{goTo(Reviewing, CauseResolved)}
	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if l := f.lane(12); l.State != NeedsYou || l.Why != WhyConflictRetries || l.Attempts.Conflicts != 2 {
		t.Errorf("lane: %+v\n%s", l, f.log.String())
	}
	if len(f.gh.comments) != 1 {
		t.Errorf("comments: %v", f.gh.comments)
	}
}

// What the Principal's workflow left on GitHub is taken up where it is, in
// the order its analyzer takes it up.
func TestEngine_TakenUpWhereTheLabelsLeftIt(t *testing.T) {
	f := newEngineFixture(t,
		task(3, "pr-ready"), task(4, "merge-conflict"), task(5, "in-progress"), task(6),
		task(7, "needs-rebase"), task(8, "review-failed"), task(9, "worker-failed"),
		Ticket{Number: 10, Labels: []string{"bug"}},
	)
	f.an.prs = map[int]int{3: 33, 4: 44, 7: 77, 8: 88}
	if err := f.e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := map[int]string{3: "reviewing", 4: "resolving/conflict", 5: "implementing", 6: "ready", 7: "resolving/rebase", 8: "review_retry", 9: "needs_you/worker_failed"}
	for issue, w := range want {
		l := f.lane(issue)
		got := string(l.State)
		if l.Resolve != "" {
			got += "/" + l.Resolve
		}
		if l.Why != "" {
			got += "/" + l.Why
		}
		if got != w {
			t.Errorf("#%d: %s, want %s", issue, got, w)
		}
	}
	if f.e.Journal.Lane(10) != nil {
		t.Error("an issue that is no ticket became a lane")
	}
	if l := f.lane(3); l.PR != 33 {
		t.Errorf("#3's pull request: %d", l.PR)
	}

	// The analyzer's order: a Worker at work, a review, a review again, a
	// conflict, a rebase, then the people.
	order := []string{}
	f.check = []func(Lane) Transition{goTo(Rework, CauseImplementFail)}
	f.review = []func(Lane) Transition{goTo(Rework, CauseChangesWanted)}
	f.resolve = []func(Lane) Transition{goTo(Reviewing, CauseResolved)}
	for i := 0; i < 8; i++ {
		d, err := f.e.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		order = append(order, fmt.Sprintf("%s#%d%s", d.NextAction, d.IssueNumber, d.MergeIssue))
		if d.NextAction == analyzer.ActionNone {
			break
		}
		if _, err := f.e.Step(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(order, " "); got != "check_result#5 review_pr#3 review_pr#8 dispatch_worker#4conflict review_pr#4 dispatch_worker#7rebase review_pr#7 none#9" {
		t.Errorf("order: %s", got)
	}
}

// The Worker's failures: done again, then a person, whom the Worker told.
func TestEngine_WorkerFailures(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12)}
	f.implement = []func(Lane) Transition{
		goTo(Rework, CauseImplementFail),
		goTo(NeedsYou, CauseEscalated, withWhy(WhyWorkerFailed)),
	}
	if end := f.run(); end != analyzer.ReasonWorkerFailed {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if l := f.lane(12); l.Attempts.Failures != 1 || l.State != NeedsYou {
		t.Errorf("lane: %+v", l)
	}
	if got := f.gh.labels(12); got != "ai-task,worker-failed" {
		t.Errorf("labels: %s", got)
	}
	if len(f.gh.comments) != 0 {
		t.Errorf("the engine commented over the Worker: %v", f.gh.comments)
	}
}

// What a review step hands to a person without saying so is said.
func TestEngine_TellsWhatAStepDidNot(t *testing.T) {
	for why, told := range map[string]bool{WhyGuardBlocked: false, WhyAgentRefused: false, WhyMergeBlocked: true, WhyGuardOutput: true} {
		f := newEngineFixture(t, task(12, "pr-ready"))
		f.an.prs[12] = 30
		f.review = []func(Lane) Transition{goTo(NeedsYou, CauseEscalated, withWhy(why))}
		f.run()
		if (len(f.gh.comments) == 0) != told {
			t.Errorf("%s: comments %v", why, f.gh.comments)
		}
	}
}

func TestEngine_Stops(t *testing.T) {
	// A stop asked for, before anything is done.
	f := newEngineFixture(t, task(12))
	f.an.precheck = &analyzer.Decision{NextAction: analyzer.ActionNone, ExitReason: "stop_requested"}
	if end := f.run(); end != "stop_requested" || len(f.steps) != 0 {
		t.Errorf("stop: %q, %v", end, f.steps)
	}

	// A run limited to one ticket takes up that one.
	f = newEngineFixture(t, task(12, "pr-ready"), task(13))
	f.e.Only = 13
	if err := f.e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.e.Journal.Lane(12) != nil || f.e.Journal.Lane(13) == nil {
		t.Error("the run took up a ticket outside it")
	}

	// A stopped engine moves nothing more.
	f = newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12)}
	ctx, cancel := context.WithCancel(context.Background())
	f.e.Implement = func(context.Context, Lane) (Transition, error) {
		cancel()
		return Transition{}, context.Canceled
	}
	if end, err := f.e.Run(ctx); end != EndInterrupted || err != nil {
		t.Errorf("interrupted: %q, %v", end, err)
	}

	// The analyzer's word on the next ticket ends a run, and moves the lane.
	f = newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionNone, IssueNumber: 12, ExitReason: analyzer.ReasonBlockedByDependencies}}
	if end := f.run(); end != analyzer.ReasonBlockedByDependencies || f.lane(12).State != Blocked {
		t.Errorf("blocked: %q, %+v", end, f.lane(12))
	}
	f.an.next = []*analyzer.Decision{dispatch(12)}
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.review = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	f.an.next = append(f.an.next, &analyzer.Decision{NextAction: analyzer.ActionAllComplete})
	if end := f.run(); end != EndComplete || f.lane(12).State != Merged {
		t.Errorf("unblocked: %q, %+v\n%s", end, f.lane(12), f.log.String())
	}
}

// A step whose move the state machine refuses stops the run with the error.
func TestEngine_AMoveRefused(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12)}
	f.implement = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	if end, err := f.e.Run(context.Background()); err == nil || end != "error_exit" {
		t.Errorf("%q, %v", end, err)
	}
}

func TestWorkerSteps(t *testing.T) {
	lane := Lane{Issue: 12, State: Implementing}
	steps := func(dispatched worker.DispatchOutput, checked worker.CheckResultOutput, fails int, stage string) *WorkerSteps {
		return &WorkerSteps{
			Root: t.TempDir(), MaxRetries: 3, Log: &bytes.Buffer{},
			now: func() time.Time { return time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC) },
			dispatch: func(context.Context, worker.DispatchOptions) (*worker.DispatchOutput, error) {
				return &dispatched, nil
			},
			check: func(context.Context, worker.CheckResultOptions) (*worker.CheckResultOutput, error) {
				return &checked, nil
			},
			learn:     func(context.Context, string, time.Duration, io.Writer) {},
			failCount: func(string, int) int { return fails },
			stage:     func(string, int) string { return stage },
		}
	}
	for _, c := range []struct {
		out   worker.DispatchOutput
		fails int
		stage string
		want  string
	}{
		{worker.DispatchOutput{Status: "success", PRURL: "https://github.com/o/r/pull/30"}, 0, "", "reviewing pr 30"},
		{worker.DispatchOutput{Status: "success_no_changes"}, 0, "", "completed pr 0"},
		{worker.DispatchOutput{Status: "failed", Error: "tests failed"}, 1, "", "rework pr 0"},
		{worker.DispatchOutput{Status: "failed", Error: "tests failed"}, 3, "", "needs_you/worker_failed pr 0"},
		{worker.DispatchOutput{Status: "blocked"}, 0, "guard", "needs_you/guard_blocked pr 0"},
		{worker.DispatchOutput{Status: "blocked"}, 0, worker.FailureStageGuardOutput, "needs_you/guard_output pr 0"},
		{worker.DispatchOutput{Status: "in_progress"}, 0, "", " pr 0"},
	} {
		w := steps(c.out, worker.CheckResultOutput{}, c.fails, c.stage)
		tr, err := w.Implement(context.Background(), lane)
		got := fmt.Sprintf("%s pr %d", tr.To, tr.PR)
		if tr.Why != "" {
			got = fmt.Sprintf("%s/%s pr %d", tr.To, tr.Why, tr.PR)
		}
		if err != nil || got != c.want {
			t.Errorf("%+v: %s (%v), want %s", c.out, got, err, c.want)
		}
	}
	for status, want := range map[string]State{
		"success": Reviewing, "success_no_changes": Completed, "failed_will_retry": Rework,
		"crashed": Rework, "timeout": Rework, "failed_max_retries": NeedsYou, "not_found": "",
	} {
		w := steps(worker.DispatchOutput{}, worker.CheckResultOutput{Status: status, PRNumber: "30"}, 0, "")
		tr, err := w.Check(context.Background(), lane)
		if err != nil || tr.To != want {
			t.Errorf("check %s: %s (%v), want %s", status, tr.To, err, want)
		}
	}

	// No result yet: the ticket says whether a Worker is at work.
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name   string
		labels []string
		since  time.Duration // before now
		status string
		want   string
	}{
		{"at work", []string{"ai-task", "in-progress"}, 10 * time.Minute, "not_found", ""},
		{"at work a long while", []string{"ai-task", "in-progress"}, 70 * time.Minute, "not_found", ""},
		{"none at work", []string{"ai-task"}, time.Minute, "not_found", "rework/implement_failed"},
		{"a result it cannot read", []string{"ai-task"}, time.Minute, "error", "rework/implement_failed"},
		{"said at work, none for too long", []string{"ai-task", "in-progress"}, 80 * time.Minute, "not_found", "needs_you/escalated/worker_lost"},
	} {
		w := steps(worker.DispatchOutput{}, worker.CheckResultOutput{Status: c.status}, 0, "")
		w.GitHub = newFakeGitHub(Ticket{Number: 12, Labels: c.labels})
		at := lane
		at.Since = now.Add(-c.since)
		tr, err := w.Check(context.Background(), at)
		got := ""
		if tr.To != "" {
			got = string(tr.To) + "/" + string(tr.Cause)
			if tr.Why != "" {
				got += "/" + tr.Why
			}
		}
		if err != nil || got != c.want || (got != "" && !Allowed(tr.From, tr.To, tr.Cause)) {
			t.Errorf("%s: %q (%v), want %q", c.name, got, err, c.want)
		}
	}
	// A ticket that cannot be read is looked at again.
	w := steps(worker.DispatchOutput{}, worker.CheckResultOutput{Status: "not_found"}, 0, "")
	w.GitHub = newFakeGitHub()
	if tr, err := w.Check(context.Background(), lane); err != nil || tr.To != "" {
		t.Errorf("an unreadable ticket: %+v (%v)", tr, err)
	}
	if _, err := steps(worker.DispatchOutput{}, worker.CheckResultOutput{}, 0, "").Implement(context.Background(), Lane{Issue: 12, State: Ready}); err == nil {
		t.Error("implemented a lane that was not dispatched")
	}

}

// theWorker stands in for the Worker as the engine meets it: it labels its
// ticket in-progress while it works, takes a ticket labeled so for one
// another Worker is at work on and does nothing, and takes the label off
// when it ends without a result. idle is how many dispatches find nothing
// to do though the ticket is not labeled (the first of the run that found
// the engine's bug, as the engine saw it).
type theWorker struct {
	f                *engineFixture
	idle             int
	dispatched, seen int // dispatches, and those that found the ticket labeled
	checked          int
	advance          time.Duration // the clock moves by it on each check
	clock            time.Time
}

func (tw *theWorker) steps() *WorkerSteps {
	gh := tw.f.gh
	return &WorkerSteps{
		Root: tw.f.e.Root, GitHub: gh, MaxRetries: 3, Log: &tw.f.log, WorkerTimeout: time.Hour,
		now: func() time.Time { return tw.clock },
		dispatch: func(ctx context.Context, o worker.DispatchOptions) (*worker.DispatchOutput, error) {
			tw.dispatched++
			if slices.Contains(gh.tickets[o.IssueNumber].Labels, "in-progress") {
				tw.seen++
				_ = gh.Edit(ctx, o.IssueNumber, Ops{Remove: []string{"in-progress"}})
				return &worker.DispatchOutput{Status: "in_progress"}, nil
			}
			if tw.idle > 0 {
				tw.idle--
				return &worker.DispatchOutput{Status: "in_progress"}, nil
			}
			_ = gh.Edit(ctx, o.IssueNumber, Ops{Add: []string{"pr-ready"}})
			return &worker.DispatchOutput{Status: "success", PRURL: "https://github.com/o/r/pull/30"}, nil
		},
		check: func(ctx context.Context, o worker.CheckResultOptions) (*worker.CheckResultOutput, error) {
			tw.checked++
			tw.clock = tw.clock.Add(tw.advance)
			return &worker.CheckResultOutput{Status: "not_found"}, nil
		},
		learn:     func(context.Context, string, time.Duration, io.Writer) {},
		failCount: func(string, int) int { return 0 },
		stage:     func(string, int) string { return "" },
	}
}

func withTheWorker(f *engineFixture, idle int, advance time.Duration) *theWorker {
	tw := &theWorker{f: f, idle: idle, advance: advance, clock: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)}
	f.e.now = func() time.Time { return tw.clock }
	w := tw.steps()
	f.e.Implement, f.e.Check = w.Implement, w.Check
	return tw
}

// The run that found it: the engine gives the Worker its ticket as it is
// (not labeled in-progress), and a lane its Worker left without a result
// and is not at work on is dispatched again, not waited on.
func TestEngine_AWorkerThatDidNothing(t *testing.T) {
	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12), dispatch(12), {NextAction: analyzer.ActionAllComplete}}
	f.review = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	tw := withTheWorker(f, 1, time.Minute)

	if end := f.run(); end != EndComplete {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if tw.dispatched != 2 || tw.seen != 0 || tw.checked != 1 {
		t.Errorf("dispatched %d (%d found it labeled), checked %d\n%s", tw.dispatched, tw.seen, tw.checked, f.log.String())
	}
	if l := f.lane(12); l.State != Merged || l.PR != 30 {
		t.Errorf("lane: %+v", l)
	}
	if !strings.Contains(f.log.String(), "implementing → rework (implement_failed): no Worker is at work on it") {
		t.Errorf("log:\n%s", f.log.String())
	}
}

// A ticket labeled in-progress that no Worker of this engine is at work on
// is waited on as long as a Worker may take, then handed to a person, who
// is told; once they take the label off, it is dispatched.
func TestEngine_AWorkerNoneIsAtWork(t *testing.T) {
	f := newEngineFixture(t, task(12, "in-progress"))
	f.an.next = []*analyzer.Decision{dispatch(12), {NextAction: analyzer.ActionAllComplete}}
	f.review = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	tw := withTheWorker(f, 0, 40*time.Minute)

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	if l := f.lane(12); l.State != NeedsYou || l.Why != WhyWorkerLost || tw.checked != 2 || tw.dispatched != 0 {
		t.Errorf("lane: %+v, checked %d, dispatched %d", l, tw.checked, tw.dispatched)
	}
	if got := f.gh.labels(12); got != "ai-task,needs-human-review" {
		t.Errorf("labels: %s", got)
	}
	if len(f.gh.comments) != 1 || !strings.Contains(f.gh.comments[0], "none has left a result") {
		t.Errorf("comments: %v", f.gh.comments)
	}

	f.gh.tickets[12].Labels = []string{"ai-task"}
	if end := f.run(); end != EndComplete {
		t.Fatalf("the next run ended %q\n%s", end, f.log.String())
	}
	if l := f.lane(12); l.State != Merged || tw.dispatched != 1 || tw.seen != 0 {
		t.Errorf("lane: %+v, dispatched %d (%d found it labeled)", l, tw.dispatched, tw.seen)
	}
}

func TestImportedLanes(t *testing.T) {
	for _, c := range []struct {
		from, to State
		ok       bool
	}{{"", Reviewing, true}, {"", NeedsYou, true}, {"", Merged, false}, {"", Paused, false}, {Ready, Reviewing, false}} {
		if Allowed(c.from, c.to, CauseImported) != c.ok {
			t.Errorf("%s → %s imported: %v", show(c.from), c.to, !c.ok)
		}
	}
}

func TestPRFromURL(t *testing.T) {
	for url, want := range map[string]int{"https://github.com/o/r/pull/30": 30, "https://github.com/o/r/pull/30/": 30, "": 0, "https://github.com/o/r/issues/3": 0} {
		if got := prFromURL(url); got != want {
			t.Errorf("%q: %d", url, got)
		}
	}
}

// The engine of a project, from its configuration: its steps, its
// analyzer, the limit the configuration sets.
func TestNew(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root+"/.ai/config.yaml", reviewConfig+"escalation:\n  max_review_attempts: 5\n")
	e, err := New(Options{Root: root, Only: 12})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Journal.Close()
	if e.MaxReviews != 5 || e.Only != 12 || e.Implement == nil || e.Check == nil || e.Review == nil || e.Resolve == nil || e.Names.Task != "ai-task" {
		t.Errorf("engine: %+v", e)
	}
	if _, err := New(Options{Root: t.TempDir()}); err == nil {
		t.Error("an engine without a configuration")
	}
}

// What a person does on GitHub between the engine's steps moves the lanes:
// it is an instruction, not an error to put back.
func TestReconcile_APersonsHand(t *testing.T) {
	ctx := context.Background()
	setLabels := func(f *engineFixture, issue int, closed bool, labels ...string) {
		f.gh.tickets[issue].Labels, f.gh.tickets[issue].Closed = labels, closed
	}
	reconcile := func(f *engineFixture) {
		t.Helper()
		if err := f.e.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	state := func(l Lane) string {
		s := string(l.State)
		if l.Why != "" {
			s += "/" + l.Why
		}
		return s
	}

	// A person takes needs-human-review off a lane that waited for them.
	f := newEngineFixture(t, task(12, "needs-human-review"), task(13, "needs-human-review"))
	f.an.prs = map[int]int{12: 30}
	reconcile(f)
	setLabels(f, 12, false, "ai-task")
	setLabels(f, 13, false, "ai-task", "pr-ready")
	reconcile(f)
	if got := state(f.lane(12)) + " " + state(f.lane(13)); got != "rework reviewing" {
		t.Errorf("released: %s", got)
	}

	// A person puts needs-human-review on a lane under review: the run stops
	// on it, and nobody is told what the person did.
	setLabels(f, 13, false, "ai-task", "pr-ready", "needs-human-review")
	reconcile(f)
	if got := state(f.lane(13)); got != "needs_you/person" || len(f.gh.comments) != 0 {
		t.Errorf("marked: %s, comments %v", got, f.gh.comments)
	}
	if d, _ := f.e.Next(ctx); d.NextAction != analyzer.ActionNone || d.IssueNumber != 13 {
		t.Errorf("the run goes on past a lane a person took: %+v", d)
	}

	// A person closes tickets: one whose work landed, one whose did not, and
	// one is no longer a ticket.
	f = newEngineFixture(t, task(20, "pr-ready"), task(21, "pr-ready"), task(22))
	reconcile(f)
	analyzer.MarkLanded(f.e.Root, 20)
	setLabels(f, 20, true, "ai-task", "pr-ready")
	setLabels(f, 21, true, "ai-task", "pr-ready")
	setLabels(f, 22, false, "bug")
	reconcile(f)
	if got := state(f.lane(20)) + " " + state(f.lane(21)) + " " + state(f.lane(22)); got != "merged cancelled cancelled" {
		t.Errorf("closed: %s", got)
	}
	if got := f.gh.labels(21); got != " (closed)" {
		t.Errorf("a cancelled ticket keeps the workflow's labels: %q", got)
	}
	if got := f.gh.labels(22); got != "bug" {
		t.Errorf("a person's own label was touched: %q", got)
	}

	// A ticket that ended is opened again, and a new one appears.
	setLabels(f, 20, false, "ai-task")
	f.gh.tickets[23] = &Ticket{Number: 23, Labels: []string{"ai-task"}}
	reconcile(f)
	if got := state(f.lane(20)) + " " + state(f.lane(23)); got != "ready ready" {
		t.Errorf("reopened and new: %s", got)
	}

	// Any other change of the labels is left as the person left it.
	setLabels(f, 23, false, "ai-task", "priority:high")
	reconcile(f)
	if got := f.gh.labels(23); got != "ai-task,priority:high" || f.lane(23).State != Ready {
		t.Errorf("other labels: %q, %+v", got, f.lane(23))
	}
}

// The agents' settings come from the configuration, on the roles' defaults;
// what goes past a role's limits keeps the engine from starting.
func TestNew_TheAgentsSettings(t *testing.T) {
	start := func(agents string) (*Engine, string, error) {
		root := t.TempDir()
		writeFile(t, root+"/.ai/config.yaml", reviewConfig+"worker:\n  backend: claude-code\n  claude_code: {model: opus}\n"+agents)
		var log bytes.Buffer
		e, err := New(Options{Root: root, Log: &log})
		if e != nil {
			e.Journal.Close()
		}
		return e, log.String(), err
	}
	if _, log, err := start(""); err != nil || log != "" {
		t.Errorf("the defaults: %v, %q", err, log)
	}
	if _, _, err := start("agents:\n  reviewer: {minutes: 500}\n"); err == nil || !strings.Contains(err.Error(), "agents.reviewer") {
		t.Errorf("a reviewer past its limit: %v", err)
	}
	if _, _, err := start("agents:\n  resolver: {backend: nobody}\n"); err == nil || !strings.Contains(err.Error(), "agents.resolver") {
		t.Errorf("a resolver on no backend: %v", err)
	}
	if _, log, err := start("agents:\n  reviewer: {model: opus}\n"); err != nil || !strings.Contains(log, "same model as the Worker") {
		t.Errorf("the reviewer is the Worker's model: %v, %q", err, log)
	}
}
