package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/agent"
	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// The engine runs the workflow in the Principal's place (design.md §5.1,
// task 1.6). It decides what to do next as the Principal's analyzer
// decides, in the same order, from its lanes where the analyzer reads
// labels: a lane at work, in review, to be reviewed again, to be resolved,
// waiting for a person, and only then the next ticket. What is decided
// before any lane is looked at (a stop, too many rounds, too many failures)
// and which tickets can start (their dependencies, a ticket that cannot be
// used, a run that is complete) it asks the analyzer itself; which of those
// a lane's paths keep off, and which wait for each other in a circle, it
// decides (schedule.go). Its decisions are the analyzer's (analyzer.Decision),
// so that the two can be compared (task 1.7). Lanes run side by side, as
// many as engine.max_parallel lets (parallel.go).

// Analyzer is what the engine asks the Principal's analyzer
// (*analyzer.Analyzer).
type Analyzer interface {
	Precheck() *analyzer.Decision
	Candidates(ctx context.Context, max int, skip analyzer.Skip) (analyzer.Picks, error)
	PRNumber(issue int, body string) int
}

// Step does a lane's work and says where the lane goes. A zero transition
// is no move.
type Step func(ctx context.Context, lane Lane) (Transition, error)

// How a run ends, beside the analyzer's reasons.
const (
	EndComplete    = "all_tasks_complete"
	EndInterrupted = "interrupted"
)

// Engine runs the workflow of a project.
type Engine struct {
	Root     string
	Journal  *Journal
	GitHub   GitHub
	Analyzer Analyzer
	Names    config.Labels
	// MaxReviews bounds how often a pull request is reviewed again, and how
	// often its conflict is sent to be resolved (LimitsOf).
	MaxReviews int
	// Only limits the run to one ticket; 0 is every ticket.
	Only int
	// The steps: the Worker writes and checks (WorkerSteps), the reviewer
	// reviews (ReviewStep), the resolver resolves (ResolveStep), the merge
	// queue merges (MergeStep).
	Implement, Check, Review, Resolve, Merge Step
	// Log is where the engine says what it does; nil is stderr.
	Log io.Writer
	// MaxParallel bounds the steps that run at once; 0 is 1 (one lane at a
	// time). MaxPerBackend bounds those of an agent backend; a backend it
	// does not name is bounded by MaxParallel alone. Backends are the
	// backends of the roles, which the steps are counted by.
	MaxParallel   int
	MaxPerBackend map[string]int
	Backends      Backends
	// OnSteps, when set, is told the steps that run whenever they change
	// (Serve keeps the run file with it).
	OnSteps func([]StepInfo)
	// Shadow, when set, is asked before each step what the Principal's
	// analyzer would do with the tickets in hand, and the engine's decision
	// is compared with it (shadow.go, task 1.7).
	Shadow Shadow

	now func() time.Time
	// logMu keeps what the run and its inbox say from running into each
	// other (Serve takes commands while a step runs).
	logMu sync.Mutex
	// open are the open tickets, as the last reconciliation read them: their
	// blocks say what each holds and waits for.
	open []Ticket
	// running are the steps that run, by lane: only the run's loop reads
	// and changes them (parallel.go).
	running map[int]step
	// wake has the run do a round while it waits for its steps: a lane was
	// moved outside them (an approval).
	wake chan struct{}
}

// Options are what `awkit run` gives the engine.
type Options struct {
	Root string
	Only int
	Log  io.Writer
	// Shadow compares each decision with the Principal's analyzer's
	// (AWKIT_ENGINE=shadow).
	Shadow bool
}

// New is the engine of a project, with its steps.
func New(opts Options) (*Engine, error) {
	cfg, err := config.Load(opts.Root)
	if err != nil {
		return nil, err
	}
	// The agents' settings: the roles' defaults, the project's on them.
	agents, err := agent.FromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", config.Path(""), err)
	}
	worker, reviewer, resolver := agents.Implementer, agents.Reviewer, agents.Resolver
	journal, err := OpenJournal(opts.Root)
	if err != nil {
		return nil, err
	}
	a := analyzer.New(opts.Root, cfg)
	if opts.Only > 0 {
		a.LimitTo(opts.Only)
	}
	limits := LimitsOf(cfg)
	gh := NewGitHub(cfg.GitHub.Labels.Task, 30*time.Second)
	// The run and its steps write to one log, one write at a time.
	var log io.Writer = os.Stderr
	if opts.Log != nil {
		log = opts.Log
	}
	log = &lockedWriter{w: log}
	work := &WorkerSteps{Root: opts.Root, Log: log, GitHub: gh, InProgress: cfg.GitHub.Labels.InProgress}
	review := &ReviewStep{Root: opts.Root, Agents: agent.NewRunner(), Profile: reviewer}
	resolve := &ResolveStep{Root: opts.Root, Agents: agent.NewRunner(), Profile: resolver}
	merge := &MergeStep{Root: opts.Root, Log: log}
	e := &Engine{
		Root: opts.Root, Journal: journal,
		GitHub: gh, Analyzer: a,
		Names: cfg.GitHub.Labels, MaxReviews: limits.Reviews, Only: opts.Only,
		Implement: work.Implement, Check: work.Check, Review: review.Review, Resolve: resolve.Resolve, Merge: merge.Merge,
		Log:         log,
		MaxParallel: cfg.Engine.MaxParallel, MaxPerBackend: cfg.Engine.MaxPerBackend,
		Backends: Backends{Implementer: worker.Backend, Reviewer: reviewer.Backend, Resolver: resolver.Backend},
	}
	if opts.Shadow {
		e.Shadow = NewShadow(opts.Root, cfg, opts.Only, analyzer.NewGitHubClient(30*time.Second))
	}
	// A review is independent when another model makes it (design.md §9.4).
	if reviewer.Backend == worker.Backend && reviewer.Model != "" && reviewer.Model == worker.Model {
		e.logf("the reviewer is the same model as the Worker (%s %s): its review is less independent; agents.reviewer sets another", reviewer.Backend, reviewer.Model)
	}
	return e, nil
}

func (e *Engine) logf(format string, args ...any) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	w := e.Log
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "[engine] "+format+"\n", args...)
}

func (e *Engine) clock() time.Time {
	if e.now != nil {
		return e.now().UTC()
	}
	return time.Now().UTC()
}

func describe(d *analyzer.Decision) string {
	s := ""
	if d.IssueNumber > 0 {
		s += fmt.Sprintf(" #%d", d.IssueNumber)
	}
	if d.PRNumber > 0 {
		s += fmt.Sprintf(" (PR #%d)", d.PRNumber)
	}
	if d.MergeIssue != "" {
		s += " to resolve its " + d.MergeIssue
	}
	if d.ExitReason != "" {
		s += ": " + d.ExitReason
	}
	return s
}

// Reconcile brings the lanes to what GitHub says (design.md §5.2). GitHub
// shows the lanes; what a person changes there is an instruction, not an
// error to put back:
//
//   - an open ticket without a lane is taken up where its labels left it (a
//     project the Principal's workflow ran, a ticket made since);
//   - a ticket that ended and is open again, with the task label, is done
//     again;
//   - a lane that waits for a person whose label the person took off goes
//     on, where the labels left it;
//   - needs-human-review put on a lane hands it to the person (P1: the run
//     stops, as the Principal's does; P2 pauses that lane only);
//   - a ticket closed by a person ends its lane: merged when its work is
//     known to have landed, cancelled otherwise; one that is no longer a
//     ticket (its task label taken off) is cancelled.
//
// Any other change to the labels is left as the person left it. A lane
// whose step runs is left alone: its own step changes its ticket's labels
// (the Worker labels it in-progress, then pr-ready), and the round after the
// step ends looks at it.
func (e *Engine) Reconcile(ctx context.Context) error {
	tickets, err := e.GitHub.Tickets(ctx)
	if err != nil {
		return err
	}
	sort.Slice(tickets, func(i, j int) bool { return tickets[i].Number < tickets[j].Number })
	e.open = tickets
	open := map[int]bool{}
	for _, t := range tickets {
		if e.Only != 0 && t.Number != e.Only {
			continue
		}
		open[t.Number] = true
		if e.runs(t.Number) {
			continue
		}
		l := e.Journal.Lane(t.Number)
		switch {
		case l == nil:
			in, ok := Infer(t.Labels, t.Closed, false, e.Names)
			if !ok {
				continue
			}
			tr := Transition{
				Issue: t.Number, To: in.State, Cause: CauseImported, Resolve: in.Resolve, Why: in.Why,
				PR: e.Analyzer.PRNumber(t.Number, t.Body), Note: "taken up where its labels left it", At: e.clock(),
			}
			if _, err := e.Journal.Record(tr); err != nil {
				return err
			}
		case l.State.Terminal():
			if _, err := e.record(ctx, Transition{Issue: l.Issue, From: l.State, To: Ready, Cause: CauseReopened, Note: "open again on GitHub, as a ticket", At: e.clock()}, true); err != nil {
				return err
			}
		default:
			if err := e.byPerson(ctx, *l, t); err != nil {
				return err
			}
		}
	}

	// The lanes whose ticket is no longer an open ticket.
	for _, l := range e.live() {
		if open[l.Issue] || e.runs(l.Issue) {
			continue
		}
		t, err := e.GitHub.Ticket(ctx, l.Issue)
		if err != nil {
			e.logf("#%d: it is not in the list of open tickets, and could not be read: %v", l.Issue, err)
			continue
		}
		tr := Transition{Issue: l.Issue, From: l.State, At: e.clock()}
		switch {
		case t.Closed && analyzer.KnownLanded(e.Root, l.Issue):
			tr.To, tr.Cause, tr.Note = Merged, CauseLanded, "closed on GitHub, its work landed"
		case t.Closed:
			tr.To, tr.Cause, tr.Note = Cancelled, CauseCancelled, "closed on GitHub"
		default:
			tr.To, tr.Cause, tr.Note = Cancelled, CauseCancelled, "no longer a ticket: its task label was taken off on GitHub"
		}
		if _, err := e.record(ctx, tr, true); err != nil {
			return err
		}
	}
	return nil
}

// byPerson moves a lane for what a person changed of its labels.
func (e *Engine) byPerson(ctx context.Context, l Lane, t Ticket) error {
	has := map[string]bool{}
	for _, label := range t.Labels {
		has[label] = true
	}
	stops := has[e.Names.NeedsHumanReview] || has[e.Names.WorkerFailed]
	switch {
	case l.State == NeedsYou && !stops:
		// The person took the label off: the lane goes on from where the
		// labels say.
		in, ok := Infer(t.Labels, false, false, e.Names)
		if !ok {
			return nil
		}
		to := in.State
		if to == Ready && l.PR != 0 {
			to = Rework
		}
		if !Allowed(NeedsYou, to, CauseReleased) {
			e.logf("#%d: its person's label is off, and its labels say %s, which a lane that waits for a person does not go to; it waits on", l.Issue, to)
			return nil
		}
		_, err := e.record(ctx, Transition{Issue: l.Issue, From: NeedsYou, To: to, Cause: CauseReleased, Resolve: in.Resolve, Note: "a person took its label off on GitHub", At: e.clock()}, true)
		return err
	case has[e.Names.NeedsHumanReview] && l.State != NeedsYou && l.State != AwaitingApproval && l.State != Paused:
		t := Transition{Issue: l.Issue, From: l.State, To: NeedsYou, Cause: CauseEscalated, Why: WhyPerson, Note: "a person marked it on GitHub", At: e.clock()}
		_, err := e.record(ctx, t, true)
		return err
	}
	return nil
}

// live are the lanes in the run that are not over, by issue number.
func (e *Engine) live() []Lane {
	var lanes []Lane
	for _, l := range e.Journal.Lanes() {
		if !l.State.Terminal() && (e.Only == 0 || l.Issue == e.Only) {
			lanes = append(lanes, l)
		}
	}
	sort.Slice(lanes, func(i, j int) bool { return lanes[i].Issue < lanes[j].Issue })
	return lanes
}

// resolving says whether a lane waits for, or is at, the resolution of its
// pull request, and of which kind.
func resolving(l Lane, kind string) bool {
	return (l.State == Resolving || (l.State == Implementing && l.Resolve != "")) && l.Resolve == kind
}

// Next is what to do next, as the analyzer would decide it (Decide), read
// from the lanes: the first thing a round does (parallel.go). With a shadow,
// the decision on the lanes is compared with the analyzer's on the labels
// (shadow.go).
func (e *Engine) Next(ctx context.Context) (*analyzer.Decision, error) {
	if d := e.Analyzer.Precheck(); d != nil {
		return d, nil
	}
	return e.decide(ctx, &roundState{mergeable: e.mergeability()})
}

// pick are the next tickets to start, as many as there is room for, as the
// analyzer finds the tickets that can start, past the ones the paths of a
// lane keep off (design.md §5.3); then what the analyzer decided instead,
// when it did. The tickets that wait for each other in a circle are handed
// to a person first, at every pick (task 2.4). With every ticket kept off and
// nothing in hand, the work waits for the lanes that hold the paths: the run
// ends for a person. None while steps run that leave no room, or that the
// tickets wait for.
func (e *Engine) pick(ctx context.Context) ([]*analyzer.Decision, error) {
	lanes := e.live()
	free := min(e.maxParallel()-taken(lanes), e.room(e.Backends.Implementer))
	if free < 1 {
		if e.busy() > 0 {
			// A slot comes free when a step ends.
			return nil, nil
		}
		// Every lane that takes a slot is in hand, and decided first.
		return nil, fmt.Errorf("no slot is free for a ticket to start, and no lane in hand was decided (%d take a slot)", taken(lanes))
	}
	// Tickets that wait for each other in a circle can never start: they
	// are handed to a person before any ticket is picked.
	if err := e.circles(ctx); err != nil {
		return nil, err
	}
	paths := map[int][]string{}
	known := map[int]bool{}
	for _, t := range e.open {
		paths[t.Number], known[t.Number] = pathsOfBody(t.Body), true
	}
	var holds []hold
	for _, l := range lanes {
		if holdsPaths(l) {
			// A lane whose ticket is not open is taken for one that holds
			// every file: reconciliation ends it before it is let go.
			holds = append(holds, hold{Issue: l.Issue, Paths: paths[l.Issue]})
		}
	}
	keptBy := map[int]int{}
	picks, err := e.Analyzer.Candidates(ctx, free, func(issue int, meta ticket.Meta, found bool) bool {
		// A lane in the engine's hands is not a ticket to start, whatever
		// GitHub shows: one whose step runs (its Worker labels it in-progress
		// only once it is at work), or one that waits for its turn.
		if !startable(e.Journal.Lane(issue)) {
			return true
		}
		paths := pathsOf(meta, found)
		if by := heldBy(holds, issue, paths); by != 0 {
			keptBy[issue] = by
			return true
		}
		// A ticket not passed over is picked: the ones picked with it keep
		// off what it may change.
		holds = append(holds, hold{Issue: issue, Paths: paths})
		return false
	})
	if err != nil {
		return nil, err
	}
	for _, issue := range picks.Skipped {
		if by := keptBy[issue]; by != 0 {
			e.logf("#%d can start, and waits: #%d holds paths it may change", issue, by)
		}
	}
	if picks.Then == nil {
		if len(picks.Tickets) > 0 {
			return picks.Tickets, nil
		}
		if len(picks.Skipped) == 0 {
			return nil, fmt.Errorf("the analyzer found no ticket to start, and said nothing else")
		}
		if e.busy() > 0 {
			// They wait for the steps that run.
			return nil, nil
		}
		first := picks.Skipped[0]
		holder := keptBy[first]
		if holder == 0 {
			holder = first
		}
		return []*analyzer.Decision{{NextAction: analyzer.ActionNone, IssueNumber: holder, ExitReason: analyzer.ReasonNeedsHumanReview}}, nil
	}
	return append(picks.Tickets, picks.Then), nil
}

// startable says whether a ticket's lane lets it start: it has none, it
// waits to start (ready, to be done again, waiting for its dependencies), or
// it ended and the ticket is open again.
func startable(l *Lane) bool {
	return l == nil || l.State == Ready || l.State == Rework || l.State == Blocked || l.State.Terminal()
}

// circles hands the tickets that wait for each other in a circle to a
// person, and says so on each. What the open tickets wait for is read from
// their blocks: a closed ticket waits for nothing (it is done, or done again
// first), so no circle goes through it. Only a ticket that has not started
// is handed over: what it depends on decides when it starts, and one that
// started goes on to its merge, which breaks the circle.
func (e *Engine) circles(ctx context.Context) error {
	waits := map[int][]int{}
	for _, t := range e.open {
		meta, found, err := ticket.Parse(t.Body)
		if err != nil || !found {
			continue
		}
		waits[t.Number] = meta.DependsOn
	}
	for _, c := range Cycles(waits) {
		for _, issue := range c.Members {
			if (e.Only != 0 && issue != e.Only) || e.runs(issue) {
				continue
			}
			l := e.Journal.Lane(issue)
			if l != nil && (!startable(l) || l.State.Terminal()) {
				continue
			}
			var err error
			if l == nil {
				l, err = e.record(ctx, Transition{Issue: issue, To: Blocked, Cause: CauseOpened, Note: "it waits for tickets that are not done", At: e.clock()}, true)
				if err != nil {
					return err
				}
			}
			t := Transition{Issue: issue, From: l.State, To: NeedsYou, Cause: CauseEscalated, Why: WhyDependencyCycle, Note: c.Note(), At: e.clock()}
			if _, err := e.record(ctx, t, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// inHand is what to do with the lanes in the workflow's hands: the head of
// the merge queue, then a lane at work, in review, to be reviewed again, to
// be resolved, in that order (the analyzer's LabelDecision, which has no
// queue). A lane whose step runs is passed over, and so is one whose step
// cannot start yet (a merge while another runs, a backend at its limit):
// with no step running, none is. A lane that waits for a person is no lane
// in hand: it stops alone (design.md §6, task 2.4), where the analyzer stops
// the run. nil is none in hand: the next ticket is picked.
func (e *Engine) inHand(ctx context.Context, mergeable Mergeability) *analyzer.Decision {
	if d := e.queued(); d != nil && e.canStart(d) {
		return d
	}
	lanes := e.live()
	first := func(match func(Lane) bool, decide func(*Lane) *analyzer.Decision) *analyzer.Decision {
		for i := range lanes {
			if e.runs(lanes[i].Issue) || !match(lanes[i]) {
				continue
			}
			if d := decide(&lanes[i]); e.canStart(d) {
				return d
			}
		}
		return nil
	}
	none := func(l *Lane, reason string) *analyzer.Decision {
		return &analyzer.Decision{NextAction: analyzer.ActionNone, IssueNumber: l.Issue, PRNumber: l.PR, ExitReason: reason}
	}

	// A Worker at work, or one an engine before this one left.
	if d := first(func(l Lane) bool { return l.State == Implementing && l.Resolve == "" }, func(l *Lane) *analyzer.Decision {
		return &analyzer.Decision{NextAction: analyzer.ActionCheckResult, IssueNumber: l.Issue}
	}); d != nil {
		return d
	}
	// A pull request to review. One that conflicts with its base is
	// resolved first: reviewing it would end in a merge that fails.
	if d := first(func(l Lane) bool { return l.State == Reviewing }, func(l *Lane) *analyzer.Decision {
		if l.PR == 0 {
			return none(l, analyzer.ReasonNeedsHumanReview)
		}
		if state, err := mergeable(ctx, l.PR); err == nil && state == "CONFLICTING" {
			if l.Attempts.Conflicts >= e.MaxReviews {
				return none(l, analyzer.ReasonNeedsHumanReview)
			}
			return &analyzer.Decision{NextAction: analyzer.ActionDispatchWorker, IssueNumber: l.Issue, PRNumber: l.PR, MergeIssue: analyzer.MergeIssueConflict}
		}
		return &analyzer.Decision{NextAction: analyzer.ActionReviewPR, IssueNumber: l.Issue, PRNumber: l.PR}
	}); d != nil {
		return d
	}
	// A review that could not be completed is tried again, as often as it
	// may be.
	if d := first(func(l Lane) bool { return l.State == ReviewRetry }, func(l *Lane) *analyzer.Decision {
		if l.PR == 0 {
			return none(l, analyzer.ReasonNeedsHumanReview)
		}
		if l.Attempts.Reviews >= e.MaxReviews {
			return none(l, analyzer.ReasonReviewMaxRetries)
		}
		return &analyzer.Decision{NextAction: analyzer.ActionReviewPR, IssueNumber: l.Issue, PRNumber: l.PR}
	}); d != nil {
		return d
	}
	// A pull request to resolve: a conflict, then one that is behind.
	for _, kind := range []string{ResolveConflict, ResolveRebase} {
		if d := first(func(l Lane) bool { return resolving(l, kind) }, func(l *Lane) *analyzer.Decision {
			if l.PR == 0 {
				return none(l, analyzer.ReasonNeedsHumanReview)
			}
			return &analyzer.Decision{NextAction: analyzer.ActionDispatchWorker, IssueNumber: l.Issue, PRNumber: l.PR, MergeIssue: kind}
		}); d != nil {
			return d
		}
	}
	return nil
}

// queued is what to do with the head of the merge queue: merge it. nil when
// the queue is empty, or its head's step runs.
func (e *Engine) queued() *analyzer.Decision {
	q := queue(e.live())
	if len(q) == 0 || e.runs(q[0].Issue) {
		return nil
	}
	l := q[0]
	if l.PR == 0 {
		return &analyzer.Decision{NextAction: analyzer.ActionNone, IssueNumber: l.Issue, ExitReason: analyzer.ReasonNeedsHumanReview}
	}
	return &analyzer.Decision{NextAction: ActionMerge, IssueNumber: l.Issue, PRNumber: l.PR}
}

// Step does what a decision says. It says the run's end when the decision
// is one (a reason), and "" to go on.
func (e *Engine) Step(ctx context.Context, d *analyzer.Decision) (string, error) {
	switch d.NextAction {
	case analyzer.ActionAllComplete:
		return EndComplete, nil
	case analyzer.ActionNone:
		if _, err := e.settle(ctx, d); err != nil {
			return "", err
		}
		if d.ExitReason == "" {
			return analyzer.ActionNone, nil
		}
		return d.ExitReason, nil
	}
	lane, work, err := e.begin(ctx, d)
	if err != nil {
		return "", err
	}
	return "", work(ctx, *lane)
}

// checked reads the result of a lane's Worker.
func (e *Engine) checked(ctx context.Context, lane Lane) error {
	t, err := e.Check(ctx, lane)
	if err != nil {
		return err
	}
	// The Worker's result tells a person what it hands them; a Worker that
	// left none does not.
	return e.apply(ctx, t, t.Why != WhyWorkerLost)
}

func (e *Engine) laneOf(issue int) (*Lane, error) {
	l := e.Journal.Lane(issue)
	if l == nil {
		return nil, fmt.Errorf("#%d has no lane", issue)
	}
	return l, nil
}

// move records a transition of a lane from where it is.
func (e *Engine) move(ctx context.Context, l *Lane, to State, cause Cause, note string) (*Lane, error) {
	t := Transition{Issue: l.Issue, From: l.State, To: to, Cause: cause, Note: note, At: e.clock()}
	return e.record(ctx, t, true)
}

// apply records what a step said, when it said to move.
func (e *Engine) apply(ctx context.Context, t Transition, told bool) error {
	if t.To == "" {
		return nil
	}
	_, err := e.record(ctx, t, told)
	return err
}

// record writes a transition to the event log, shows the lane on GitHub,
// and tells a person when it needs one and its step did not already.
func (e *Engine) record(ctx context.Context, t Transition, told bool) (*Lane, error) {
	lane, err := e.write(t)
	if err != nil {
		return nil, err
	}
	e.project(ctx, *lane)
	if lane.State == NeedsYou && !told {
		if err := e.GitHub.Comment(ctx, lane.Issue, needsYouComment(*lane, e.Names)); err != nil {
			e.logf("#%d: the comment that says why it needs a person could not be posted: %v", lane.Issue, err)
		}
	}
	if lane.State == AwaitingApproval && !told {
		if err := e.GitHub.Comment(ctx, lane.Issue, approvalComment(*lane, e.Names)); err != nil {
			e.logf("#%d: the comment that asks for its owner's approval could not be posted: %v", lane.Issue, err)
		}
	}
	return lane, nil
}

// dispatched records the Worker taking a lane, and leaves its ticket as it
// is: the Worker labels the ticket in-progress itself, and a ticket that is
// labeled so when it is dispatched it takes for one another Worker is at
// work on, and does nothing.
func (e *Engine) dispatched(l *Lane) (*Lane, error) {
	return e.write(Transition{Issue: l.Issue, From: l.State, To: Implementing, Cause: CauseDispatched, At: e.clock()})
}

// write writes a transition to the event log, and says it.
func (e *Engine) write(t Transition) (*Lane, error) {
	lane, err := e.Journal.Record(t)
	if err != nil {
		return nil, err
	}
	e.logf("#%d: %s → %s (%s)%s", t.Issue, show(t.From), t.To, t.Cause, noteOf(t.Note))
	return lane, nil
}

func noteOf(note string) string {
	if note == "" {
		return ""
	}
	return ": " + note
}

// project brings a lane's ticket on GitHub to what the lane is. A failure is
// said and left: the next move projects again, and reconciliation looks.
func (e *Engine) project(ctx context.Context, lane Lane) {
	t, err := e.GitHub.Ticket(ctx, lane.Issue)
	if err != nil {
		e.logf("#%d: its labels could not be read: %v", lane.Issue, err)
		return
	}
	ops := Diff(lane, t.Labels, t.Closed, e.Names)
	if ops.Empty() {
		return
	}
	if err := e.GitHub.Edit(ctx, lane.Issue, ops); err != nil {
		e.logf("#%d: its labels could not be changed: %v", lane.Issue, err)
	}
}

// whyText says, for a person, why a lane needs them.
var whyText = map[string]string{
	WhyWorkerFailed:     "the Worker failed as often as it may",
	WhyNeedsHumanReview: "the workflow handed it to a person",
	WhyReviewMaxRetries: "the review could not be completed as often as it may be tried",
	WhyConflictRetries:  "the conflict of its pull request could not be resolved as often as it may be tried",
	WhyNoPR:             "its pull request cannot be found",
	WhyMergeBlocked:     "GitHub refused the merge",
	WhyGuardBlocked:     "a guardrail stopped what an agent was to read",
	WhyGuardOutput:      "a guardrail stopped what was to be written to GitHub",
	WhyInvalidTicket:    "its awk-ticket block cannot be used",
	WhyAgentRefused:     "an agent's backend cannot be held to its role's settings",
	WhyWorkerLost:       "its ticket says a Worker is at work on it, but none has left a result in the time a Worker may take",
	WhyDependencyCycle:  "the tickets it depends on go round in a circle",
	WhyPerson:           "a person marked it on GitHub",
	WhyDiffPolicy:       "its change, rebased in the merge queue, breaks the diff policy",
	WhyChecksStuck:      "the checks of its pull request did not start, or did not end, in the time they may take",
	WhyCheckRetries:     "the merge queue's checks failed on its change as often as they may",
	WhyMaintainer:       "a maintainer's review is open on its pull request, and nothing was pushed for it since",
}

// WhyText says, for a person, why a lane needs them: what the engine's
// comment on the issue says.
func WhyText(why string) string {
	if text := whyText[why]; text != "" {
		return text
	}
	return why
}

func needsYouComment(l Lane, names config.Labels) string {
	why := whyText[l.Why]
	if why == "" {
		why = l.Why
	}
	label := names.NeedsHumanReview
	if l.Why == WhyWorkerFailed {
		label = names.WorkerFailed
	}
	return fmt.Sprintf("## AWK：這張工單需要你處理\n\n%s。\n\n%s\n\n處理好之後移除 `%s` 標籤，下一輪會接著做。細節在這台電腦的 `awkit events --issue %d`。",
		why, l.Note, label, l.Issue)
}

// settle moves the lane a decision to stop gave up on, when it gave up on
// one that is not already waiting for a person: a limit reached, a pull
// request that cannot be found, a ticket that cannot be used, one that
// waits for its dependencies. moved says whether it handed one to a person:
// then that lane alone stops, and the round goes on (design.md §6, task
// 2.4); a decision that hands none over is one the run stops on.
func (e *Engine) settle(ctx context.Context, d *analyzer.Decision) (moved bool, err error) {
	l := e.Journal.Lane(d.IssueNumber)
	if l == nil || l.State.Terminal() || l.State == NeedsYou || l.State == Paused {
		return false, nil
	}
	needsYou := func(why, note string) (bool, error) {
		t := Transition{Issue: l.Issue, From: l.State, To: NeedsYou, Cause: CauseEscalated, Why: why, Note: note, At: e.clock()}
		_, err := e.record(ctx, t, false)
		return err == nil, err
	}
	switch {
	case d.ExitReason == analyzer.ReasonReviewMaxRetries && l.State == ReviewRetry:
		return needsYou(WhyReviewMaxRetries, fmt.Sprintf("the review of PR #%d was tried %d times", l.PR, l.Attempts.Reviews))
	case d.ExitReason == analyzer.ReasonNeedsHumanReview && l.PR == 0 && slices.Contains([]State{Reviewing, ReviewRetry, Resolving, Queued}, l.State):
		return needsYou(WhyNoPR, "the lane's pull request is not known")
	case d.ExitReason == analyzer.ReasonNeedsHumanReview && l.State == Reviewing:
		return needsYou(WhyConflictRetries, fmt.Sprintf("PR #%d still conflicts with its base after %d resolutions", l.PR, l.Attempts.Conflicts))
	case d.ExitReason == analyzer.ReasonInvalidTicket:
		return needsYou(WhyInvalidTicket, "the analyzer says the ticket cannot be used; its comment on the issue says why")
	case d.ExitReason == analyzer.ReasonNeedsHumanReview && (l.State == Ready || l.State == Rework):
		// The analyzer handed the ticket to a person (and told them).
		t := Transition{Issue: l.Issue, From: l.State, To: NeedsYou, Cause: CauseEscalated, Why: WhyNeedsHumanReview, At: e.clock()}
		_, err := e.record(ctx, t, true)
		return err == nil, err
	case d.ExitReason == analyzer.ReasonBlockedByDependencies && l.State == Ready:
		// Every ticket waits for another: no lane is handed to a person,
		// and asking again would find the same.
		_, err := e.move(ctx, l, Blocked, CauseBlocked, "it waits for a ticket it depends on")
		return false, err
	}
	return false, nil
}

// forAPerson is how a run ends that has nothing more to do, or stops on a
// lane that waits already: for a person, when lanes wait for one
// (needs_human_review; worker_failed when each of them is a Worker that
// failed as often as it may). A lane that waits for a person needs them,
// paused or waiting for their approval as well. The run's own stops (a stop,
// its limits, GitHub) are as they are.
func (e *Engine) forAPerson(end string) string {
	switch end {
	case EndComplete, analyzer.ReasonNoActionableTasks, analyzer.ReasonBlockedByDependencies,
		analyzer.ReasonNeedsHumanReview, analyzer.ReasonWorkerFailed, analyzer.ReasonReviewMaxRetries, analyzer.ReasonInvalidTicket:
	default:
		return end
	}
	reason := ""
	for _, l := range e.live() {
		switch {
		case l.State == NeedsYou && l.Why == WhyWorkerFailed:
			reason = analyzer.ReasonWorkerFailed
		case l.State == NeedsYou, l.State == Paused, l.State == AwaitingApproval:
			return analyzer.ReasonNeedsHumanReview
		}
	}
	if reason != "" {
		return reason
	}
	return end
}

// toImplement dispatches a ticket the analyzer picked to the Worker.
func (e *Engine) toImplement(ctx context.Context, d *analyzer.Decision) (*Lane, error) {
	l := e.Journal.Lane(d.IssueNumber)
	var err error
	switch {
	case l == nil:
		l, err = e.record(ctx, Transition{Issue: d.IssueNumber, To: Ready, Cause: CauseOpened, At: e.clock()}, true)
	case l.State.Terminal():
		// Closed without its work landing: the analyzer opened it again.
		l, err = e.move(ctx, l, Ready, CauseReopened, "closed without its work landing")
	case l.State == Blocked:
		l, err = e.move(ctx, l, Ready, CauseUnblocked, "")
	}
	if err != nil {
		return nil, err
	}
	if l.State != Ready && l.State != Rework {
		return nil, fmt.Errorf("lane #%d is %s: the analyzer picked it as a ticket to write", l.Issue, l.State)
	}
	return e.dispatched(l)
}

// implemented has the Worker write a ticket it was dispatched.
func (e *Engine) implemented(ctx context.Context, lane Lane) error {
	t, err := e.Implement(ctx, lane)
	if err != nil {
		return err
	}
	return e.apply(ctx, t, true)
}

// toReview brings a lane to its pull request's review.
func (e *Engine) toReview(ctx context.Context, d *analyzer.Decision) (*Lane, error) {
	l, err := e.laneOf(d.IssueNumber)
	if err != nil {
		return nil, err
	}
	if l.State == ReviewRetry {
		return e.move(ctx, l, Reviewing, CauseReviewRetried, "")
	}
	return l, nil
}

// reviewed has a lane's pull request reviewed.
func (e *Engine) reviewed(ctx context.Context, lane Lane) error {
	t, err := e.Review(ctx, lane)
	if err != nil {
		return err
	}
	// The review's own decision (reviewer.SubmitReview) comments when it
	// hands a ticket to a person; the review step does not, nor does it say
	// why a change waits for its owner.
	told := t.Why != WhyGuardBlocked && t.Why != WhyAgentRefused && t.To != AwaitingApproval
	return e.apply(ctx, t, told)
}

// toResolve sends a lane's pull request to be resolved.
func (e *Engine) toResolve(ctx context.Context, d *analyzer.Decision) (*Lane, error) {
	l, err := e.laneOf(d.IssueNumber)
	if err != nil {
		return nil, err
	}
	if l.State == Reviewing {
		t := Transition{Issue: l.Issue, From: Reviewing, To: Resolving, Cause: CauseConflicting, Resolve: d.MergeIssue, Note: "the pull request conflicts with its base", At: e.clock()}
		if l, err = e.record(ctx, t, true); err != nil {
			return nil, err
		}
	}
	if l.State == Resolving {
		return e.move(ctx, l, Implementing, CauseDispatched, "")
	}
	return l, nil
}

// resolved has a lane's pull request rebased, and its conflict resolved.
func (e *Engine) resolved(ctx context.Context, lane Lane) error {
	t, err := e.Resolve(ctx, lane)
	if err != nil {
		return err
	}
	return e.apply(ctx, t, false)
}

// merged has the pull request at the head of the merge queue merged.
func (e *Engine) merged(ctx context.Context, lane Lane) error {
	if lane.State != Queued {
		return fmt.Errorf("lane #%d is %s: the merge queue merges a lane in it", lane.Issue, lane.State)
	}
	t, err := e.Merge(ctx, lane)
	if err != nil {
		return err
	}
	// The maintainers' gate says on the issue why it holds the pull
	// request; the queue's other stops do not.
	return e.apply(ctx, t, t.Why == WhyMaintainer)
}
