package engine

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// What a run does at once (design.md §5.3, task 2.2b). A run goes by rounds:
// when it starts, and whenever a step ends, the engine takes in what a person
// did on GitHub (Reconcile), asks whether it may go on (Precheck), and starts
// every step that can start now:
//
//   - no more than engine.max_parallel at once, and no more of an agent
//     backend than engine.max_per_backend gives it;
//   - one merge at a time, of the pull request at the head of the merge
//     queue (merge.go, task 2.3). The merge takes no slot and no backend: it
//     runs no agent, and mostly waits for CI. A review approves, and the
//     queue merges, so reviews and resolutions run side by side;
//   - new tickets while the lanes that take a slot leave one (2.2a), as many
//     as are left, past the tickets whose lanes are in the engine's hands.
//
// Each step runs on its own, and says when it ends. The move of its lane to
// the work (dispatched, reviewed again, sent to be resolved) is written
// before it starts, so that the next decision sees the slot it takes. A
// decision that hands one lane to a person moves that lane, and the round
// goes on (task 2.4). Another decision that is not a step ends the run when
// no step runs, as it did one lane at a time; while steps run, the lane it
// names is moved, nothing new starts, and the run waits for them. With one step at a time a round is a
// step, and the run is what it was.

// Backends are the agent backends of the roles that do the steps.
type Backends struct {
	Implementer, Reviewer, Resolver string
}

// step is a step that runs: what it does, on which backend, since when.
type step struct {
	decision analyzer.Decision
	backend  string
	since    time.Time
}

// merges says whether a step is the merge queue's: one at a time, in no
// slot.
func (s step) merges() bool { return s.decision.NextAction == ActionMerge }

// stepEnd is a step that ended, and the error it ended with.
type stepEnd struct {
	issue int
	err   error
}

// roundState is what a round has decided so far: the answers to Mergeable it
// asked, and the tickets picked to start in turn. Once tickets are picked,
// nothing else starts in the round: the lanes in hand were looked at before,
// and the tickets took what room was left.
type roundState struct {
	mergeable Mergeability
	more      []*analyzer.Decision
	picked    bool
}

func (e *Engine) maxParallel() int {
	if e.MaxParallel < 1 {
		return 1
	}
	return e.MaxParallel
}

// backendOf is the backend of the agent that does a decision's step; "" for
// one that runs none (a Worker's result read, a merge).
func (e *Engine) backendOf(d *analyzer.Decision) string {
	switch {
	case d.NextAction == analyzer.ActionReviewPR:
		return e.Backends.Reviewer
	case d.NextAction == analyzer.ActionDispatchWorker && d.MergeIssue != "":
		return e.Backends.Resolver
	case d.NextAction == analyzer.ActionDispatchWorker:
		return e.Backends.Implementer
	}
	return ""
}

// busy counts the steps that run; runs says whether a lane's does. Only the
// run's loop reads and changes them.
func (e *Engine) busy() int { return len(e.running) }

func (e *Engine) runs(issue int) bool {
	_, ok := e.running[issue]
	return ok
}

// taken counts the steps that run in a slot of max_parallel: every one but
// the merge.
func (e *Engine) taken() int {
	n := 0
	for _, s := range e.running {
		if !s.merges() {
			n++
		}
	}
	return n
}

// merging says whether the merge queue's step runs.
func (e *Engine) merging() bool {
	for _, s := range e.running {
		if s.merges() {
			return true
		}
	}
	return false
}

// room is how many more steps of a backend may start now: what
// max_parallel leaves, and what max_per_backend leaves of the backend.
func (e *Engine) room(backend string) int {
	n := e.maxParallel() - e.taken()
	if limit, ok := e.MaxPerBackend[backend]; ok && backend != "" {
		on := 0
		for _, s := range e.running {
			if s.backend == backend {
				on++
			}
		}
		n = min(n, limit-on)
	}
	return n
}

// canStart says whether a decision's step can start now. A decision that is
// not a step always can: it moves lanes, or ends the run.
func (e *Engine) canStart(d *analyzer.Decision) bool {
	if !working(d.NextAction) {
		return true
	}
	if d.NextAction == ActionMerge {
		return !e.merging()
	}
	return e.room(e.backendOf(d)) > 0
}

// Steps are the steps that run, the oldest first.
func (e *Engine) Steps() []StepInfo {
	steps := make([]StepInfo, 0, len(e.running))
	for _, s := range e.running {
		d := s.decision
		steps = append(steps, StepInfo{Action: d.NextAction, Issue: d.IssueNumber, PR: d.PRNumber, Merge: d.MergeIssue, Since: s.since})
	}
	sort.Slice(steps, func(i, j int) bool {
		if !steps[i].Since.Equal(steps[j].Since) {
			return steps[i].Since.Before(steps[j].Since)
		}
		return steps[i].Issue < steps[j].Issue
	})
	return steps
}

func (e *Engine) stepsChanged() {
	if e.OnSteps != nil {
		e.OnSteps(e.Steps())
	}
}

// Run runs the workflow until it ends, and says why it ended: one of the
// analyzer's reasons, EndComplete, or EndInterrupted when ctx is done. An
// error is one the run cannot go on from (the event log refused a move); the
// reason then says how it is reported. A run that is to end lets the steps
// that run end first: what they did is written.
func (e *Engine) Run(ctx context.Context) (string, error) {
	ended := make(chan stepEnd)
	e.running = map[int]step{}
	defer func() { e.running = nil }()
	var end string
	var failure error
	stop := func(reason string, err error) {
		if end != "" {
			return
		}
		if ctx.Err() != nil {
			reason, err = EndInterrupted, nil
		}
		end, failure = reason, err
		if n := e.busy(); n > 0 {
			e.logf("the run ends (%s) once the %d step(s) that run end%s", reason, n, errorOf(err))
		}
	}
	for {
		if end == "" {
			if ctx.Err() != nil {
				stop(EndInterrupted, nil)
			} else if reason, err := e.round(ctx, ended); reason != "" || err != nil {
				stop(reason, err)
			}
		}
		if e.busy() == 0 {
			if end == "" {
				return "error_exit", fmt.Errorf("the round started nothing, and no step runs")
			}
			return end, failure
		}
		r := <-ended
		delete(e.running, r.issue)
		e.stepsChanged()
		if r.err != nil {
			stop("error_exit", r.err)
		}
	}
}

func errorOf(err error) string {
	if err == nil {
		return ""
	}
	return ": " + err.Error()
}

// round reconciles, and starts every step that can start. It says the run's
// end when a decision ends it; "" when the run waits for the steps that run.
func (e *Engine) round(ctx context.Context, ended chan<- stepEnd) (string, error) {
	// What a person did on GitHub since the last step is taken in first.
	if err := e.Reconcile(ctx); err != nil {
		return analyzer.ReasonGitHubAPIError, err
	}
	if d := e.Analyzer.Precheck(); d != nil {
		e.logf("next: %s%s", d.NextAction, describe(d))
		return e.decided(ctx, d)
	}
	r := &roundState{mergeable: e.mergeability()}
	for {
		d, err := e.decide(ctx, r)
		if err != nil {
			return analyzer.ReasonGitHubAPIError, err
		}
		if d == nil {
			if e.busy() == 0 {
				return "error_exit", fmt.Errorf("no step can start, and none runs")
			}
			return "", nil
		}
		e.logf("next: %s%s", d.NextAction, describe(d))
		if !working(d.NextAction) {
			// A decision that hands one lane to a person stops that lane
			// alone: the round goes on (design.md §6, task 2.4).
			if d.NextAction == analyzer.ActionNone {
				moved, err := e.settle(ctx, d)
				if err != nil {
					return "error_exit", err
				}
				if moved {
					r.picked = false
					continue
				}
			}
			return e.decided(ctx, d)
		}
		if err := e.start(ctx, d, ended); err != nil {
			return "error_exit", err
		}
	}
}

// decided acts on a decision that is not a step: it moves the lane it names,
// and ends the run as it says when no step runs. While steps run the run
// goes on, and waits for them.
func (e *Engine) decided(ctx context.Context, d *analyzer.Decision) (string, error) {
	end, err := e.Step(ctx, d)
	if err != nil {
		return "error_exit", err
	}
	if n := e.busy(); n > 0 {
		e.logf("%s: nothing new starts, and the %d step(s) that run go on", end, n)
		return "", nil
	}
	return e.forAPerson(end), nil
}

// decide is the next thing to do in a round: a step that can start now, or
// what was decided instead; nil when nothing more can start until a step
// that runs ends.
func (e *Engine) decide(ctx context.Context, r *roundState) (*analyzer.Decision, error) {
	if len(r.more) > 0 {
		d := r.more[0]
		r.more = r.more[1:]
		return d, nil
	}
	if r.picked {
		return nil, nil
	}
	if e.taken() >= e.maxParallel() {
		// The merge takes no slot: it starts when the slots are full.
		if d := e.queued(); d != nil && e.canStart(d) {
			return d, nil
		}
		return nil, nil
	}
	d := e.inHand(ctx, r.mergeable)
	// The Principal's analyzer cannot tell the lanes whose step runs: it is
	// compared while none does, which is every decision one step at a time.
	if e.Shadow != nil && e.busy() == 0 {
		e.compare(ctx, d, r.mergeable)
	}
	if d != nil {
		return d, nil
	}
	r.picked = true
	picks, err := e.pick(ctx)
	if err != nil || len(picks) == 0 {
		return nil, err
	}
	r.more = picks[1:]
	return picks[0], nil
}

// start moves a lane to the work a decision starts, and runs the work on its
// own; ended is told when it ends.
func (e *Engine) start(ctx context.Context, d *analyzer.Decision, ended chan<- stepEnd) error {
	lane, work, err := e.begin(ctx, d)
	if err != nil {
		return err
	}
	if e.running == nil {
		e.running = map[int]step{}
	}
	e.running[lane.Issue] = step{decision: *d, backend: e.backendOf(d), since: e.clock()}
	e.stepsChanged()
	go func() {
		ended <- stepEnd{issue: lane.Issue, err: work(ctx, *lane)}
	}()
	return nil
}

// begin moves a lane to the work a decision starts, and says what does the
// work.
func (e *Engine) begin(ctx context.Context, d *analyzer.Decision) (*Lane, func(context.Context, Lane) error, error) {
	var l *Lane
	var err error
	switch {
	case d.NextAction == analyzer.ActionCheckResult:
		l, err = e.laneOf(d.IssueNumber)
		return l, e.checked, err
	case d.NextAction == ActionMerge:
		l, err = e.laneOf(d.IssueNumber)
		return l, e.merged, err
	case d.NextAction == analyzer.ActionReviewPR:
		l, err = e.toReview(ctx, d)
		return l, e.reviewed, err
	case d.NextAction == analyzer.ActionDispatchWorker && d.MergeIssue != "":
		l, err = e.toResolve(ctx, d)
		return l, e.resolved, err
	case d.NextAction == analyzer.ActionDispatchWorker:
		l, err = e.toImplement(ctx, d)
		return l, e.implemented, err
	}
	return nil, nil, fmt.Errorf("the engine has no step for %q", d.NextAction)
}

// lockedWriter is a writer that whatever runs at once writes to one write at
// a time: the engine's log, which the steps write to as well.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// prefixWriter writes each line it is given to w with prefix before it: the
// output of a Worker, among others that run at once. A line is written whole,
// once its end comes; Flush writes what is left of the last one.
type prefixWriter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
	line   []byte
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		p.line = append(p.line, c)
		if c == '\n' {
			if err := p.emit(); err != nil {
				return 0, err
			}
		}
	}
	return len(b), nil
}

// Flush writes what is left of the last line, with an end of its own.
func (p *prefixWriter) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.line) == 0 {
		return nil
	}
	p.line = append(p.line, '\n')
	return p.emit()
}

func (p *prefixWriter) emit() error {
	out := append([]byte(p.prefix), p.line...)
	p.line = p.line[:0]
	_, err := p.w.Write(out)
	return err
}
