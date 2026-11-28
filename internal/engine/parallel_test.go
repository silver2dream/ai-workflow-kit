package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// parallelFixture runs an engine whose steps wait to be let go, and counts
// how many of a kind run at once.
type parallelFixture struct {
	*engineFixture
	mu sync.Mutex
	// at and most count the steps of a kind that run now, and the most that
	// ran at once; "all" counts every kind that takes a slot (all but the
	// merge).
	at, most map[string]int
	gates    map[string]chan struct{}
	started  chan string
	// answers are where a kind of step takes its lane; set before the run.
	answers map[string]func(Lane) Transition
	// holdMerges has the merges wait to be let go, and say they started, as
	// the other steps do; else a merge ends at once, unseen.
	holdMerges bool
}

func newParallelFixture(t *testing.T, max int, tickets ...Ticket) *parallelFixture {
	t.Helper()
	p := &parallelFixture{
		engineFixture: newEngineFixture(t, tickets...),
		at:            map[string]int{}, most: map[string]int{},
		gates: map[string]chan struct{}{}, started: make(chan string, 64),
		answers: map[string]func(Lane) Transition{
			"implement": goTo(Reviewing, CauseImplemented, func(t *Transition) { t.PR = 100 + t.Issue }),
			"review":    approve,
			"resolve":   resolvedBack,
			"check":     goTo(Reviewing, CauseImplemented, func(t *Transition) { t.PR = 100 + t.Issue }),
			"merge":     goTo(Merged, CauseMerged),
		},
	}
	p.e.MaxParallel = max
	p.an.pending = p.gh.pendingOf
	p.e.Implement, p.e.Review, p.e.Resolve, p.e.Check = p.gated("implement"), p.gated("review"), p.gated("resolve"), p.gated("check")
	p.e.Merge = p.gated("merge")
	return p
}

// approve is a review that approves: the lane goes to the merge queue.
func approve(l Lane) Transition {
	return goTo(Queued, CauseApproved, func(t *Transition) { t.Approved = fmt.Sprintf("approved-%d", t.Issue) })(l)
}

// resolvedBack is a resolution that pushed: back to the queue for a lane the
// queue sent, to review for one before it (ResolveStep).
func resolvedBack(l Lane) Transition {
	if l.Approved != "" {
		return goTo(Queued, CauseResolved)(l)
	}
	return goTo(Reviewing, CauseResolved)(l)
}

// gated is a step that says it started, and waits to be let go.
func (p *parallelFixture) gated(kind string) Step {
	kinds := []string{kind, "all"}
	if kind == "merge" {
		kinds = []string{kind}
	}
	return func(ctx context.Context, l Lane) (Transition, error) {
		name := fmt.Sprintf("%s #%d", kind, l.Issue)
		p.mu.Lock()
		gate := p.gate(name)
		held := kind != "merge" || p.holdMerges
		for _, k := range kinds {
			p.at[k]++
			p.most[k] = max(p.most[k], p.at[k])
		}
		p.mu.Unlock()
		var err error
		if held {
			p.started <- name
			select {
			case <-gate:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		p.mu.Lock()
		for _, k := range kinds {
			p.at[k]--
		}
		// The answer as the step ends: a test may change it while it runs.
		answer := p.answers[kind]
		p.mu.Unlock()
		if err != nil {
			return Transition{}, err
		}
		return answer(l), nil
	}
}

func (p *parallelFixture) gate(name string) chan struct{} {
	g, ok := p.gates[name]
	if !ok {
		g = make(chan struct{})
		p.gates[name] = g
	}
	return g
}

// answer sets where a kind of step takes its lane from now on.
func (p *parallelFixture) answer(kind string, a func(Lane) Transition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answers[kind] = a
}

// let lets steps end.
func (p *parallelFixture) let(names ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, name := range names {
		close(p.gate(name))
	}
}

// expect waits for these steps to start, in any order.
func (p *parallelFixture) expect(names ...string) {
	p.t.Helper()
	want := slices.Sorted(slices.Values(names))
	var got []string
	for len(got) < len(want) {
		select {
		case n := <-p.started:
			got = append(got, n)
		case <-time.After(10 * time.Second):
			p.t.Fatalf("started %v, want %v", got, want)
		}
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		p.t.Fatalf("started %v, want %v", got, want)
	}
}

// quiet says that no step starts for a while.
func (p *parallelFixture) quiet() {
	p.t.Helper()
	select {
	case n := <-p.started:
		p.t.Fatalf("%s started", n)
	case <-time.After(150 * time.Millisecond):
	}
}

type runEnd struct {
	end string
	err error
}

func (p *parallelFixture) runAsync(ctx context.Context) chan runEnd {
	done := make(chan runEnd, 1)
	go func() {
		end, err := p.e.Run(ctx)
		done <- runEnd{end, err}
	}()
	return done
}

func (p *parallelFixture) wait(done chan runEnd) runEnd {
	p.t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(10 * time.Second):
		p.t.Fatal("the run did not end")
	}
	return runEnd{}
}

func (p *parallelFixture) mostOf(kind string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.most[kind]
}

// apart are tickets whose paths are apart from each other's.
func apart(numbers ...int) []Ticket {
	var tickets []Ticket
	for _, n := range numbers {
		tickets = append(tickets, withBody(task(n), body(fmt.Sprintf("part%d/**", n))))
	}
	return tickets
}

// Tickets whose paths are apart are written side by side, and their pull
// requests reviewed side by side: the review approves, and the merge queue
// merges. A ticket whose step runs is not picked again, though GitHub shows
// it as one to start until its Worker labels it.
func TestEngine_LanesSideBySide(t *testing.T) {
	p := newParallelFixture(t, 3, apart(1, 2, 3)...)
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2", "implement #3")
	p.let("implement #2")
	p.expect("review #2")
	p.let("implement #1", "implement #3")
	p.expect("review #1", "review #3")
	p.let("review #2", "review #1", "review #3")

	r := p.wait(done)
	if r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	for _, issue := range []int{1, 2, 3} {
		if l := p.lane(issue); l.State != Merged || l.PR != 100+issue {
			t.Errorf("#%d: %+v", issue, l)
		}
	}
	if p.mostOf("implement") != 3 || p.mostOf("review") != 3 || p.mostOf("merge") != 1 {
		t.Errorf("at once: %v", p.most)
	}
	for _, max := range p.an.maxAsked {
		if max < 1 || max > 3 {
			t.Errorf("%d tickets asked for: %v", max, p.an.maxAsked)
		}
	}
	if p.an.maxAsked[0] != 3 {
		t.Errorf("the first round asked for %d tickets, want 3", p.an.maxAsked[0])
	}
}

// No more steps run at once than engine.max_parallel, and a lane in review
// takes the slot of the ticket it was: a new ticket starts once it is over.
func TestEngine_MaxParallel(t *testing.T) {
	p := newParallelFixture(t, 2, apart(1, 2, 3, 4)...)
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2")
	p.quiet()
	p.let("implement #1")
	p.expect("review #1")
	p.quiet()
	p.let("review #1")
	p.expect("implement #3")
	p.let("implement #2")
	p.expect("review #2")
	p.let("implement #3")
	p.expect("review #3")
	p.quiet()
	p.let("review #2")
	p.expect("implement #4")
	p.let("review #3", "implement #4")
	p.expect("review #4")
	p.let("review #4")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if p.mostOf("all") != 2 {
		t.Errorf("at once: %v", p.most)
	}
}

// A backend runs no more steps at once than engine.max_per_backend gives it;
// a backend it does not name only max_parallel bounds.
func TestEngine_MaxPerBackend(t *testing.T) {
	p := newParallelFixture(t, 3, apart(1, 2, 3)...)
	p.e.Backends = Backends{Implementer: "codex", Reviewer: "claude-code", Resolver: "codex"}
	p.e.MaxPerBackend = map[string]int{"codex": 1}
	done := p.runAsync(t.Context())

	p.expect("implement #1")
	p.quiet()
	p.let("implement #1")
	// The review is claude-code's, which has no limit of its own.
	p.expect("review #1", "implement #2")
	p.let("review #1")
	// #3 waits for codex's one slot, which #2 takes.
	p.quiet()
	p.let("implement #2")
	p.expect("review #2", "implement #3")
	p.let("review #2")
	p.quiet()
	p.let("implement #3")
	p.expect("review #3")
	p.let("review #3")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if p.mostOf("implement") != 1 || p.mostOf("all") != 2 {
		t.Errorf("at once: %v", p.most)
	}
}

// A lane in hand waits for its backend: the Worker and the reviewer share
// one, which runs one step at a time. The resolver's backend is another.
func TestEngine_ALaneInHandWaitsForItsBackend(t *testing.T) {
	p := newParallelFixture(t, 3, withBody(task(1), body("part1/**")), withBody(task(2, "pr-ready"), body("part2/**")), withBody(task(3, "pr-ready"), body("part3/**")))
	p.e.Shadow = nil
	p.e.Backends = Backends{Implementer: "claude-code", Reviewer: "claude-code", Resolver: "codex"}
	p.e.MaxPerBackend = map[string]int{"claude-code": 1}
	p.an.prs = map[int]int{2: 102, 3: 103}
	p.gh.mergeable[102] = "CONFLICTING"
	done := p.runAsync(t.Context())

	// #2's conflict is resolved on codex, and #3 is reviewed on claude-code;
	// #1 waits for claude-code's one slot.
	p.expect("resolve #2", "review #3")
	p.gh.mu.Lock()
	p.gh.mergeable[102] = ""
	p.gh.mu.Unlock()
	p.let("resolve #2")
	// #2's review and #1 could start, and claude-code is at its limit.
	p.quiet()
	p.let("review #3")
	// The lane in hand first.
	p.expect("review #2")
	p.let("review #2")
	p.expect("implement #1")
	p.let("implement #1")
	p.expect("review #1")
	p.let("review #1")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if p.mostOf("all") != 2 {
		t.Errorf("at once: %v", p.most)
	}
}

// A review and a resolution run side by side: neither merges (task 2.3).
func TestEngine_ReviewsAndResolutionsSideBySide(t *testing.T) {
	p := newParallelFixture(t, 3, task(1, "pr-ready"), task(2, "pr-ready"))
	// Not compared: the shadow takes a conflict on #1 while the Principal's
	// analyzer reviews #2, first in GitHub's order, for a difference it does
	// not know (task 1.7a's known differences ask for the same action).
	p.e.Shadow = nil
	p.an.prs = map[int]int{1: 101, 2: 102}
	p.gh.mergeable[101] = "CONFLICTING"
	done := p.runAsync(t.Context())

	p.expect("resolve #1", "review #2")
	p.gh.mu.Lock()
	p.gh.mergeable[101] = ""
	p.gh.mu.Unlock()
	p.let("resolve #1")
	p.expect("review #1")
	p.let("review #1", "review #2")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if p.mostOf("all") != 2 || p.mostOf("review") != 2 {
		t.Errorf("at once: %v", p.most)
	}
}

// A ticket whose paths meet those of a lane in hand waits for it, while one
// whose paths are apart is written beside it.
func TestEngine_PathsWhileLanesRun(t *testing.T) {
	p := newParallelFixture(t, 3, withBody(task(1), body("src/**")), withBody(task(2), body("src/a.go")), withBody(task(3), body("docs/**")))
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #3")
	p.quiet()
	p.let("implement #1")
	p.expect("review #1")
	p.quiet()
	p.let("review #1")
	p.expect("implement #2")
	p.let("implement #3")
	p.expect("review #3")
	p.let("implement #2")
	p.expect("review #2")
	p.let("review #3", "review #2")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if !strings.Contains(p.log.String(), "#2 can start, and waits: #1 holds paths it may change") {
		t.Errorf("the wait is not said:\n%s", p.log.String())
	}
}

// Reconciliation leaves a lane whose step runs to its step: its Worker
// labels its ticket as it goes. The step's own move is taken.
func TestEngine_ReconcileLeavesALaneThatRuns(t *testing.T) {
	p := newParallelFixture(t, 2, apart(1, 2)...)
	p.answers["implement"] = func(l Lane) Transition {
		if l.Issue == 1 {
			return goTo(NeedsYou, CauseEscalated, withWhy(WhyGuardBlocked))(l)
		}
		return goTo(Reviewing, CauseImplemented, withPR(102))(l)
	}
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2")
	// #1's Worker was stopped by a guard: it labels the ticket, and its
	// step has not said so yet.
	p.gh.mu.Lock()
	p.gh.tickets[1].Labels = append(p.gh.tickets[1].Labels, "needs-human-review")
	p.gh.mu.Unlock()
	p.let("implement #2")
	p.expect("review #2")
	p.let("implement #1", "review #2")

	r := p.wait(done)
	if r.end != analyzer.ReasonNeedsHumanReview || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(1); l.State != NeedsYou || l.Why != WhyGuardBlocked {
		t.Errorf("#1: %+v (reconciliation took the step's label for a person's)", l)
	}
	if l := p.lane(2); l.State != Merged {
		t.Errorf("#2: %+v", l)
	}
}

// A lane that needs a person stops alone (task 2.4): the lanes in hand go
// on, and new tickets start in its place; once nothing else is left, the
// run ends for the person.
func TestEngine_ALaneThatNeedsAPersonStopsAlone(t *testing.T) {
	p := newParallelFixture(t, 2, apart(1, 2, 3)...)
	p.answers["implement"] = func(l Lane) Transition {
		if l.Issue == 1 {
			return goTo(NeedsYou, CauseEscalated, withWhy(WhyWorkerFailed))(l)
		}
		return goTo(Reviewing, CauseImplemented, func(t *Transition) { t.PR = 100 + t.Issue })(l)
	}
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2")
	p.let("implement #1")
	p.expect("implement #3")
	p.let("implement #2", "implement #3")
	p.expect("review #2", "review #3")
	p.let("review #2", "review #3")

	r := p.wait(done)
	if r.end != analyzer.ReasonWorkerFailed || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(1); l.State != NeedsYou || l.Why != WhyWorkerFailed {
		t.Errorf("#1: %+v", l)
	}
	for _, issue := range []int{2, 3} {
		if l := p.lane(issue); l.State != Merged {
			t.Errorf("#%d: %+v", issue, l)
		}
	}
}

// A stop starts nothing new: the steps that run end, and so does the run.
func TestEngine_AStopLetsTheStepsThatRunEnd(t *testing.T) {
	p := newParallelFixture(t, 3, apart(1, 2, 3)...)
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2", "implement #3")
	p.an.precheck = &analyzer.Decision{NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonStopRequested}
	p.let("implement #1")
	p.quiet()
	p.let("implement #2", "implement #3")

	r := p.wait(done)
	if r.end != analyzer.ReasonStopRequested || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	for _, issue := range []int{1, 2, 3} {
		if l := p.lane(issue); l.State != Reviewing {
			t.Errorf("#%d: %+v", issue, l)
		}
	}
	if !strings.Contains(p.log.String(), "user_stopped: nothing new starts, and the 2 step(s) that run go on") {
		t.Errorf("the wait is not said:\n%s", p.log.String())
	}
}

// A step the run cannot go on from ends it, once the steps that run with it
// end: what they did is written.
func TestEngine_AStepsErrorEndsTheRunAfterTheOthers(t *testing.T) {
	p := newParallelFixture(t, 2, apart(1, 2, 3)...)
	p.answers["implement"] = func(l Lane) Transition {
		if l.Issue == 1 {
			// A move from where the lane is not: the event log refuses it.
			return Transition{Issue: 1, From: Ready, To: Reviewing, Cause: CauseImplemented}
		}
		return goTo(Reviewing, CauseImplemented, withPR(102))(l)
	}
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2")
	p.let("implement #1")
	p.quiet()
	p.let("implement #2")

	r := p.wait(done)
	if r.end != "error_exit" || r.err == nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(2); l.State != Reviewing {
		t.Errorf("#2: %+v", l)
	}
}

// A stop at once ends every step that runs, and the run with them; the
// lanes stay where they were, to be taken up.
func TestEngine_AStopAtOnceEndsEveryStep(t *testing.T) {
	p := newParallelFixture(t, 3, apart(1, 2, 3)...)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := p.runAsync(ctx)

	p.expect("implement #1", "implement #2", "implement #3")
	cancel()

	if r := p.wait(done); r.end != EndInterrupted || r.err != nil {
		t.Fatalf("ended %q, %v", r.end, r.err)
	}
	for _, issue := range []int{1, 2, 3} {
		if l := p.lane(issue); l.State != Implementing {
			t.Errorf("#%d: %+v", issue, l)
		}
	}
}

// The steps that run are told whenever they change, the oldest first; the
// run file has them (Serve).
func TestEngine_TheStepsThatRun(t *testing.T) {
	p := newParallelFixture(t, 3, apart(1, 2, 3)...)
	var mu sync.Mutex
	var seen [][]StepInfo
	p.e.OnSteps = func(steps []StepInfo) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, steps)
	}
	tick := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	p.e.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		tick = tick.Add(time.Second)
		return tick
	}
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2", "implement #3")
	mu.Lock()
	last := seen[len(seen)-1]
	mu.Unlock()
	var issues []int
	for i, s := range last {
		issues = append(issues, s.Issue)
		if s.Action != analyzer.ActionDispatchWorker || (i > 0 && !last[i-1].Since.Before(s.Since)) {
			t.Errorf("steps: %+v", last)
		}
	}
	if fmt.Sprint(issues) != "[1 2 3]" {
		t.Errorf("steps: %v", issues)
	}
	for _, issue := range []string{"#1", "#2", "#3"} {
		p.let("implement " + issue)
		p.expect("review " + issue)
		p.let("review " + issue)
	}
	if r := p.wait(done); r.end != EndComplete {
		t.Fatalf("ended %q, %v", r.end, r.err)
	}
	mu.Lock()
	defer mu.Unlock()
	if final := seen[len(seen)-1]; len(final) != 0 {
		t.Errorf("steps at the end: %+v", final)
	}
}

func TestPrefixWriter(t *testing.T) {
	var out bytes.Buffer
	w := newPrefixWriter(&lockedWriter{w: &out}, "[#5] ")
	for _, part := range []string{"one\ntw", "o\n", "", "three", " and more\nfour"} {
		if n, err := w.Write([]byte(part)); n != len(part) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", part, n, err)
		}
	}
	if got := out.String(); got != "[#5] one\n[#5] two\n[#5] three and more\n" {
		t.Errorf("before the flush: %q", got)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[#5] one\n[#5] two\n[#5] three and more\n[#5] four\n" {
		t.Errorf("after the flush: %q", got)
	}

	// Writers side by side write whole lines.
	out.Reset()
	shared := &lockedWriter{w: &out}
	var wg sync.WaitGroup
	for _, name := range []string{"a", "b", "c"} {
		wg.Go(func() {
			w := newPrefixWriter(shared, "["+name+"] ")
			for range 50 {
				_, _ = w.Write([]byte(name + name + name))
				_, _ = w.Write([]byte(name + "\n"))
			}
		})
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 150 {
		t.Fatalf("%d lines", len(lines))
	}
	for _, line := range lines {
		name := line[1:2]
		if line != "["+name+"] "+strings.Repeat(name, 4) {
			t.Fatalf("a line run into another: %q", line)
		}
	}

	failing := newPrefixWriter(failWriter{}, "> ")
	if _, err := failing.Write([]byte("x\n")); err == nil {
		t.Error("a writer that fails: no error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("no") }

// Workers run side by side from one WorkerSteps: its defaults are set once,
// and what each Worker prints reaches the log a whole line at a time, with
// its ticket's number.
func TestWorkerSteps_SideBySide(t *testing.T) {
	var log bytes.Buffer
	w := &WorkerSteps{
		Root: t.TempDir(), Log: &lockedWriter{w: &log},
		dispatch: func(ctx context.Context, o worker.DispatchOptions) (*worker.DispatchOutput, error) {
			for i := range 20 {
				fmt.Fprintf(o.Output, "line %d of #%d", i, o.IssueNumber)
				fmt.Fprintln(o.Output)
			}
			fmt.Fprintf(o.Output, "no end of line")
			return &worker.DispatchOutput{Status: "success", PRURL: fmt.Sprintf("https://github.com/o/r/pull/%d", 100+o.IssueNumber)}, nil
		},
		learn: func(context.Context, string, time.Duration, io.Writer) {},
	}
	var wg sync.WaitGroup
	for _, issue := range []int{1, 2, 3, 4} {
		wg.Go(func() {
			tr, err := w.Implement(t.Context(), Lane{Issue: issue, State: Implementing})
			if err != nil || tr.To != Reviewing || tr.PR != 100+issue {
				t.Errorf("#%d: %+v, %v", issue, tr, err)
			}
		})
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSuffix(log.String(), "\n"), "\n")
	if len(lines) != 4*21 {
		t.Fatalf("%d lines:\n%s", len(lines), log.String())
	}
	for _, line := range lines {
		var a, b int
		if _, err := fmt.Sscanf(line, "[#%d] ", &a); err != nil {
			t.Fatalf("a line without its ticket: %q", line)
		}
		if line == fmt.Sprintf("[#%d] no end of line", a) {
			continue
		}
		var i int
		if _, err := fmt.Sscanf(line, "[#%d] line %d of #%d", &a, &i, &b); err != nil || a != b {
			t.Fatalf("a line run into another: %q", line)
		}
	}
	if w.MaxRetries != worker.DefaultMaxRetries || w.WorkerTimeout != worker.DefaultWorkerTimeout {
		t.Errorf("defaults: %+v", w)
	}
}

// A lane in the engine's hands is not a ticket to start, whatever GitHub
// shows of it: here #1 waits for its review while #2's resolution runs, and
// its labels were lost. The reviewer and the resolver share a backend that
// runs one step at a time.
func TestEngine_ALaneInHandIsNotPickedWhateverGitHubShows(t *testing.T) {
	p := newParallelFixture(t, 3, withBody(task(1, "pr-ready"), body("part1/**")), withBody(task(2, "pr-ready"), body("part2/**")))
	p.e.Shadow = nil // the labels are taken off by hand
	p.e.Backends = Backends{Implementer: "claude-code", Reviewer: "codex", Resolver: "codex"}
	p.e.MaxPerBackend = map[string]int{"codex": 1}
	p.an.prs = map[int]int{1: 101, 2: 102}
	p.gh.mergeable[102] = "CONFLICTING"
	done := p.runAsync(t.Context())

	// #1's review could not be completed: it waits to be tried again, while
	// #2's conflict is resolved.
	p.expect("review #1")
	p.answer("review", goTo(ReviewRetry, CauseReviewFailed))
	p.let("review #1")
	p.expect("resolve #2")
	// #1's labels are gone: GitHub shows it as a ticket to start.
	p.gh.mu.Lock()
	p.gh.tickets[1].Labels = []string{"ai-task"}
	p.gh.mergeable[102] = ""
	p.gh.mu.Unlock()
	p.answer("review", approve)
	p.let("resolve #2")
	p.expect("review #2")
	// #1 waits for the review, and is not written.
	p.quiet()
	if l := p.lane(1); l.State != ReviewRetry {
		t.Errorf("#1: %+v", l)
	}
	p.let("review #2")
	// Its gate is open: the review tried again ends at once.
	p.expect("review #1")
	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(1); l.State != Merged || l.Attempts.Reviews != 1 {
		t.Errorf("#1: %+v", l)
	}
}

// The queue merges one pull request at a time, the one that came into it
// first.
func TestEngine_OneMergeAtATime(t *testing.T) {
	p := newParallelFixture(t, 3, apart(1, 2, 3)...)
	p.holdMerges = true
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2", "implement #3")
	p.let("implement #1", "implement #2", "implement #3")
	p.expect("review #1", "review #2", "review #3")
	// #3 is approved first, then #1, then #2: the queue is in that order.
	p.let("review #3")
	p.expect("merge #3")
	p.let("review #1")
	p.quiet()
	p.let("review #2")
	p.quiet()
	p.let("merge #3")
	p.expect("merge #1")
	p.let("merge #1")
	p.expect("merge #2")
	p.let("merge #2")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if p.mostOf("merge") != 1 {
		t.Errorf("at once: %v", p.most)
	}
	var order []string
	for _, line := range strings.Split(p.log.String(), "\n") {
		if i := strings.Index(line, "next: merge_pr #"); i >= 0 {
			order = append(order, strings.Fields(line[i+len("next: merge_pr "):])[0])
		}
	}
	if got := strings.Join(order, " "); got != "#3 #1 #2" {
		t.Errorf("merged in the order %s\n%s", got, p.log.String())
	}
}

// One merge at a time, whichever lane asks: the queue only gives its head,
// and a merge that runs keeps any other from starting.
func TestEngine_CanStartOneMerge(t *testing.T) {
	e := &Engine{MaxParallel: 1}
	merge := func(issue int) *analyzer.Decision {
		return &analyzer.Decision{NextAction: ActionMerge, IssueNumber: issue, PRNumber: 100 + issue}
	}
	if !e.canStart(merge(2)) {
		t.Fatal("no merge runs, and one cannot start")
	}
	e.running = map[int]step{1: {decision: *merge(1)}}
	if e.canStart(merge(2)) {
		t.Error("a second merge can start while one runs")
	}
	if e.taken() != 0 || !e.canStart(&analyzer.Decision{NextAction: analyzer.ActionDispatchWorker, IssueNumber: 3}) {
		t.Error("the merge takes the slot")
	}
}

// The merge takes no slot: with one, a ticket is written, and reviewed,
// while the queue merges another; and the next merge starts when the one
// before ends, though the slot is taken.
func TestEngine_TheMergeTakesNoSlot(t *testing.T) {
	p := newParallelFixture(t, 1, apart(1, 2, 3)...)
	p.holdMerges = true
	done := p.runAsync(t.Context())

	p.expect("implement #1")
	p.let("implement #1")
	p.expect("review #1")
	p.let("review #1")
	p.expect("merge #1", "implement #2")
	p.let("implement #2")
	p.expect("review #2")
	// #2 waits for #1's merge; #3 takes the slot.
	p.let("review #2")
	p.expect("implement #3")
	p.let("merge #1")
	p.expect("merge #2")
	p.let("implement #3")
	p.expect("review #3")
	p.let("merge #2", "review #3")
	p.expect("merge #3")
	p.let("merge #3")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if p.mostOf("all") != 1 || p.mostOf("merge") != 1 {
		t.Errorf("at once: %v", p.most)
	}
}

// A stop lets the merge that runs end, and starts no other.
func TestEngine_AStopLetsTheMergeEnd(t *testing.T) {
	p := newParallelFixture(t, 2, apart(1, 2)...)
	p.holdMerges = true
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2")
	p.let("implement #1", "implement #2")
	p.expect("review #1", "review #2")
	p.let("review #1")
	p.expect("merge #1")
	// The run waits for its steps: the stop is read in the round the next
	// step's end starts.
	p.an.precheck = &analyzer.Decision{NextAction: analyzer.ActionNone, ExitReason: analyzer.ReasonStopRequested}
	p.let("review #2")
	p.quiet()
	p.let("merge #1")

	if r := p.wait(done); r.end != analyzer.ReasonStopRequested || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(1); l.State != Merged {
		t.Errorf("#1: %+v", l)
	}
	if l := p.lane(2); l.State != Queued || l.Approved != "approved-2" {
		t.Errorf("#2: %+v", l)
	}
}

// A ticket closed while its step runs is left to the step: the round another
// step's end starts does not move its lane.
func TestEngine_ATicketClosedWhileItsStepRuns(t *testing.T) {
	p := newParallelFixture(t, 2, apart(1, 2)...)
	p.e.Shadow = nil
	p.answers["implement"] = func(l Lane) Transition {
		if l.Issue == 1 {
			return goTo(Completed, CauseNoChanges)(l)
		}
		return goTo(Reviewing, CauseImplemented, withPR(102))(l)
	}
	done := p.runAsync(t.Context())

	p.expect("implement #1", "implement #2")
	p.gh.mu.Lock()
	p.gh.tickets[1].Closed = true
	p.gh.mu.Unlock()
	p.let("implement #2")
	p.expect("review #2")
	if l := p.lane(1); l.State != Implementing {
		t.Errorf("#1 was moved while its step ran: %+v", l)
	}
	p.let("implement #1")
	p.let("review #2")

	if r := p.wait(done); r.end != EndComplete || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(1); l.State != Completed {
		t.Errorf("#1: %+v", l)
	}
}

// Tickets that wait for each other in a circle through a lane whose step
// runs: the others are handed to a person, and the lane is left to its step.
func TestEngine_ACircleThroughALaneThatRuns(t *testing.T) {
	deps := func(n int, on string) Ticket {
		return withBody(task(n), "## Summary\nx\n\n```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [part"+fmt.Sprint(n)+"/**]\nrisk: low\ndepends_on: ["+on+"]\n```\n")
	}
	p := newParallelFixture(t, 2, deps(1, ""), deps(2, "1"))
	p.e.Shadow = nil
	p.an.pending = nil
	p.an.next = []*analyzer.Decision{dispatch(1), {NextAction: analyzer.ActionNone, IssueNumber: 2, ExitReason: analyzer.ReasonBlockedByDependencies}}
	done := p.runAsync(t.Context())

	p.expect("implement #1")
	// A person makes #1 wait for #2, while #1 is written.
	p.gh.mu.Lock()
	p.gh.tickets[1].Body = deps(1, "2").Body
	p.gh.mu.Unlock()
	p.let("implement #1")
	p.expect("review #1")
	p.let("review #1")

	r := p.wait(done)
	if r.end != analyzer.ReasonNeedsHumanReview || r.err != nil {
		t.Fatalf("ended %q, %v\n%s", r.end, r.err, p.log.String())
	}
	if l := p.lane(1); l.State != Merged {
		t.Errorf("#1 was handed to a person while its step ran: %+v", l)
	}
	if l := p.lane(2); l.State != NeedsYou || l.Why != WhyDependencyCycle {
		t.Errorf("#2: %+v", l)
	}
}
