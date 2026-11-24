package engine

import (
	"context"
	"fmt"
	"io"
	"os"
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
// projecting after it changes nothing. Moving the implementer onto the agent
// runtime goes with the containers (P3).

// WorkerSteps run the Worker for a lane, and read its result.
type WorkerSteps struct {
	Root          string
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
// end: one an engine before it dispatched. A zero transition is no move:
// the Worker is still at work, or has left no result yet.
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
		return Transition{}, nil
	default: // failed, failed_will_retry, crashed, timeout
		return w.failed(lane, fmt.Sprintf("the Worker ended %s: %s", out.Status, out.Error)), nil
	}
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
