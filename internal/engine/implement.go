package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/hooks"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// The implementer of P1 is the Worker the Principal's workflow dispatches
// (internal/worker): the engine runs it as `awkit dispatch-worker` does, and
// reads where the lane goes from its result. The Worker keeps its own labels
// as it always has, and they are the lane's projection (design.md §6), so
// projecting after it changes nothing. Projecting before it would: the
// Worker takes a ticket already labeled in-progress for one another Worker
// is at work on. Moving the implementer onto the agent runtime goes with the
// containers (P3).

// lostAfter is how much longer than a Worker may take a lane waits for the
// result of one its ticket says is at work.
const lostAfter = 15 * time.Minute

// WorkerSteps run the Worker for a lane, and read its result.
type WorkerSteps struct {
	Root string
	// GitHub is where the check reads whether a Worker is at work: the
	// Worker labels its ticket InProgress while it is.
	GitHub        GitHub
	InProgress    string
	GHTimeout     time.Duration
	WorkerTimeout time.Duration
	// MaxRetries is how often the Worker of a ticket may fail before the
	// ticket needs a person; 0 is the Worker's own default, as
	// `awkit dispatch-worker` has it.
	MaxRetries int
	// Log is where what the Worker's dispatch reports goes; nil is stderr.
	Log io.Writer

	// Replaced in tests.
	now       func() time.Time
	dispatch  func(context.Context, worker.DispatchOptions) (*worker.DispatchOutput, error)
	check     func(context.Context, worker.CheckResultOptions) (*worker.CheckResultOutput, error)
	learn     func(ctx context.Context, root string, timeout time.Duration, log io.Writer)
	failCount func(root string, issue int) int
	stage     func(root string, issue int) string
}

func (w *WorkerSteps) defaults() {
	if w.GHTimeout == 0 {
		w.GHTimeout = 30 * time.Second
	}
	if w.WorkerTimeout == 0 {
		w.WorkerTimeout = worker.DefaultWorkerTimeout
	}
	if w.MaxRetries == 0 {
		w.MaxRetries = 3
	}
	if w.InProgress == "" {
		w.InProgress = config.DefaultLabels().InProgress
	}
	if w.Log == nil {
		w.Log = os.Stderr
	}
	if w.now == nil {
		w.now = time.Now
	}
	if w.dispatch == nil {
		w.dispatch = worker.DispatchWorker
	}
	if w.check == nil {
		w.check = worker.CheckResult
	}
	if w.learn == nil {
		w.learn = reviewer.LearnBeforeDispatch
	}
	if w.failCount == nil {
		w.failCount = worker.ReadFailCount
	}
	if w.stage == nil {
		w.stage = func(root string, issue int) string {
			if r, err := worker.LoadResult(root, issue); err == nil {
				return r.FailureStage
			}
			return ""
		}
	}
}

// Implement runs the Worker for a lane an implementer was dispatched to
// write (Implementing, with nothing to resolve). A zero transition is no
// move: the ticket's Worker is already at work elsewhere.
func (w *WorkerSteps) Implement(ctx context.Context, lane Lane) (Transition, error) {
	if lane.State != Implementing || lane.Resolve != "" {
		return Transition{}, fmt.Errorf("lane #%d is %s (resolve %q): the Worker writes a lane dispatched to implement it", lane.Issue, show(lane.State), lane.Resolve)
	}
	w.defaults()
	// Maintainers review on their own schedule: what they wrote since the
	// last look is taken in before the Worker's prompt is built.
	w.learn(ctx, w.Root, w.GHTimeout, w.Log)
	opts := worker.DispatchOptions{
		IssueNumber: lane.Issue, StateRoot: w.Root,
		GHTimeout: w.GHTimeout, WorkerTimeout: w.WorkerTimeout, MaxRetries: w.MaxRetries,
	}
	if cfg, err := analyzer.LoadConfig(config.Path(w.Root)); err == nil {
		opts.HookRunner = hooks.NewHookRunner(cfg.Hooks, w.Root, w.Log)
	}
	out, err := w.dispatch(ctx, opts)
	if ctx.Err() != nil {
		return Transition{}, ctx.Err()
	}
	if err != nil {
		return w.failed(lane, "the Worker could not be dispatched: "+err.Error()), nil
	}
	switch out.Status {
	case "success":
		t := w.move(lane, Reviewing, CauseImplemented, "")
		t.PR = prFromURL(out.PRURL)
		return t, nil
	case "success_no_changes":
		return w.move(lane, Completed, CauseNoChanges, "the Worker found nothing to change"), nil
	case "blocked":
		return w.blocked(lane, out.Error), nil
	case "in_progress":
		return Transition{}, nil
	default:
		return w.failed(lane, fmt.Sprintf("the Worker ended %s: %s", out.Status, out.Error)), nil
	}
}

// Check reads the result of a lane's Worker that the engine did not see
// end: one an engine before it dispatched, or one whose dispatch found
// another at work. A zero transition is no move: the Worker is still at
// work.
func (w *WorkerSteps) Check(ctx context.Context, lane Lane) (Transition, error) {
	if lane.State != Implementing || lane.Resolve != "" {
		return Transition{}, fmt.Errorf("lane #%d is %s (resolve %q): only a Worker's lane is checked", lane.Issue, show(lane.State), lane.Resolve)
	}
	w.defaults()
	out, err := w.check(ctx, worker.CheckResultOptions{
		IssueNumber: lane.Issue, StateRoot: w.Root, MaxRetries: w.MaxRetries,
		GHTimeout: w.GHTimeout, WorkerTimeout: w.WorkerTimeout,
	})
	if ctx.Err() != nil {
		return Transition{}, ctx.Err()
	}
	if err != nil {
		return Transition{}, fmt.Errorf("the result of lane #%d's Worker could not be read: %w", lane.Issue, err)
	}
	switch out.Status {
	case "success":
		t := w.move(lane, Reviewing, CauseImplemented, "")
		t.PR, _ = strconv.Atoi(strings.TrimSpace(out.PRNumber))
		return t, nil
	case "success_no_changes":
		return w.move(lane, Completed, CauseNoChanges, "the Worker found nothing to change"), nil
	case "blocked":
		return w.blocked(lane, out.Error), nil
	case "failed_max_retries":
		return w.workerFailed(lane, out.Error), nil
	case "not_found", "error":
		return w.noResult(ctx, lane), nil
	default: // failed, failed_will_retry, crashed, timeout
		return w.failed(lane, fmt.Sprintf("the Worker ended %s: %s", out.Status, out.Error)), nil
	}
}

// noResult is where a lane goes whose Worker has left no result. Its ticket
// says whether one is at work: the Worker labels it in-progress, and takes
// the label off when it ends without a result. A lane no Worker is at work
// on is dispatched again, as the analyzer does; one whose ticket says a
// Worker is at work longer than a Worker may take is handed to a person:
// none of this engine's is, and only a person knows whose is.
func (w *WorkerSteps) noResult(ctx context.Context, lane Lane) Transition {
	if w.GitHub == nil {
		return Transition{}
	}
	t, err := w.GitHub.Ticket(ctx, lane.Issue)
	if err != nil {
		fmt.Fprintf(w.Log, "#%d: whether a Worker is at work on it could not be read; it is looked at again: %v\n", lane.Issue, err)
		return Transition{}
	}
	if !slices.Contains(t.Labels, w.InProgress) {
		return w.move(lane, Rework, CauseImplementFail, "no Worker is at work on it, and none left a result: it is dispatched again")
	}
	if waited := w.now().Sub(lane.Since); !lane.Since.IsZero() && waited > w.WorkerTimeout+lostAfter {
		tr := w.move(lane, NeedsYou, CauseEscalated, fmt.Sprintf("its ticket has said %s for %s, and no Worker of this engine is at work on it", w.InProgress, waited.Round(time.Minute)))
		tr.Why = WhyWorkerLost
		return tr
	}
	return Transition{}
}

func (w *WorkerSteps) move(lane Lane, to State, cause Cause, note string) Transition {
	return Transition{Issue: lane.Issue, From: lane.State, To: to, Cause: cause, Note: note, At: w.now().UTC()}
}

// failed is a Worker that failed: done again, or, when it failed as often
// as it may, handed to a person. The Worker counts its failures.
func (w *WorkerSteps) failed(lane Lane, note string) Transition {
	if w.failCount(w.Root, lane.Issue) >= w.MaxRetries {
		return w.workerFailed(lane, note)
	}
	return w.move(lane, Rework, CauseImplementFail, note)
}

func (w *WorkerSteps) workerFailed(lane Lane, note string) Transition {
	t := w.move(lane, NeedsYou, CauseEscalated, fmt.Sprintf("the Worker failed %d times: %s", w.MaxRetries, note))
	t.Why = WhyWorkerFailed
	return t
}

// blocked is a ticket the guard stopped: what the Worker was to read, or
// the pull request it was to open.
func (w *WorkerSteps) blocked(lane Lane, note string) Transition {
	t := w.move(lane, NeedsYou, CauseEscalated, note)
	t.Why = WhyGuardBlocked
	if w.stage(w.Root, lane.Issue) == worker.FailureStageGuardOutput {
		t.Why = WhyGuardOutput
	}
	return t
}

// prFromURL is the number of a pull request from its URL; 0 when it has
// none.
func prFromURL(url string) int {
	i := strings.LastIndex(url, "/pull/")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimRight(url[i+len("/pull/"):], "/"))
	return n
}
