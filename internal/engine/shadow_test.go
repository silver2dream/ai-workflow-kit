package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// The exhaustive comparison (design.md §5.1, task 1.7): in every state the
// tickets in the workflow's hands can be in, the engine's decision is the
// one the Principal's analyzer makes from the labels, or a known
// difference. A real run only goes where its plan happens to go; the
// states it does not reach are compared here.

// ghOrder is GitHub as gh lists the tickets: the newest first.
type ghOrder struct{ *fakeGitHub }

func (g ghOrder) Tickets(ctx context.Context) ([]Ticket, error) {
	tickets, err := g.fakeGitHub.Tickets(ctx)
	sort.Slice(tickets, func(i, j int) bool { return tickets[i].Number > tickets[j].Number })
	return tickets, err
}

// prLookup is the rest of GitHub the shadow's analyzer asks: the pull
// request of a ticket, by its branch. A question it is not expected to ask
// panics (the embedded interface is nil).
type prLookup struct {
	analyzer.GitHubClientInterface
	pr func(issue int) int
}

func (p prLookup) FindPRByBranch(_ context.Context, branch string) (int, error) {
	var issue int
	if _, err := fmt.Sscanf(branch, "feat/ai-issue-%d", &issue); err != nil {
		return 0, err
	}
	return p.pr(issue), nil
}

// shadowCase is a state of the tickets in hand: the lanes, the labels their
// tickets have (the lanes' projection, unless given), and GitHub's word on
// each pull request ("" is MERGEABLE, "error" is no answer).
type shadowCase struct {
	lanes     []Lane
	labels    map[int][]string
	mergeable map[int]string
}

func (c shadowCase) String() string {
	var parts []string
	for _, l := range c.lanes {
		s := fmt.Sprintf("#%d %s", l.Issue, l.State)
		if l.Resolve != "" {
			s += "/" + l.Resolve
		}
		if l.Why != "" {
			s += "/" + l.Why
		}
		if l.PR != 0 {
			s += fmt.Sprintf(" PR #%d", l.PR)
			if m := c.mergeable[l.PR]; m != "" {
				s += " " + m
			}
		}
		if l.Attempts.Reviews != 0 || l.Attempts.Conflicts != 0 {
			s += fmt.Sprintf(" reviews=%d conflicts=%d", l.Attempts.Reviews, l.Attempts.Conflicts)
		}
		if labels, ok := c.labels[l.Issue]; ok {
			s += " labels=" + strings.Join(labels, ",")
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// labelsOf are the labels a lane's ticket has in the case.
func (c shadowCase) labelsOf(l Lane, names config.Labels) []string {
	if labels, ok := c.labels[l.Issue]; ok {
		return labels
	}
	return Project(l, names).Labels
}

// errMergeable is GitHub not answering whether a pull request merges.
var errMergeable = errors.New("GitHub did not answer")

type mergeableGitHub struct {
	ghOrder
	mergeable map[int]string
}

func (g mergeableGitHub) Mergeable(_ context.Context, pr int) (string, error) {
	switch m := g.mergeable[pr]; m {
	case "":
		return "MERGEABLE", nil
	case "error":
		return "", errMergeable
	default:
		return m, nil
	}
}

// shadowBench compares the two decisions on one case after another, in one
// project.
type shadowBench struct {
	t       *testing.T
	root    string
	journal *Journal
	cfg     *config.Config
}

func newShadowBench(t *testing.T) *shadowBench {
	t.Helper()
	root := t.TempDir()
	j, err := OpenJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	cfg := &config.Config{}
	cfg.GitHub.Labels = config.DefaultLabels()
	cfg.Escalation.MaxReviewAttempts = 3
	return &shadowBench{t: t, root: root, journal: j, cfg: cfg}
}

// compare sets the case's lanes and labels and compares the decisions.
func (b *shadowBench) compare(c shadowCase) Comparison {
	b.t.Helper()
	names := config.DefaultLabels()
	b.journal.mu.Lock()
	b.journal.lanes = map[int]*Lane{}
	for i := range c.lanes {
		l := c.lanes[i]
		b.journal.lanes[l.Issue] = &l
	}
	b.journal.mu.Unlock()

	var tickets []Ticket
	prs := map[int]int{}
	for _, l := range c.lanes {
		tickets = append(tickets, Ticket{Number: l.Issue, Labels: c.labelsOf(l, names)})
		prs[l.Issue] = l.PR
	}
	gh := mergeableGitHub{ghOrder{newFakeGitHub(tickets...)}, c.mergeable}
	e := &Engine{
		Root: b.root, Journal: b.journal, GitHub: gh, Analyzer: &fakeAnalyzer{prs: prs},
		Names: names, MaxReviews: 3, Log: &strings.Builder{},
		Shadow: NewShadow(b.root, b.cfg, 0, prLookup{pr: func(issue int) int { return prs[issue] }}),
	}
	ctx := context.Background()
	mergeable := e.mergeability()
	return e.compare(ctx, e.inHand(ctx, mergeable), mergeable)
}

// expect checks a comparison: the same decision, or the known difference
// given ("" is none).
func (b *shadowBench) expect(c shadowCase, known string) {
	b.t.Helper()
	got := b.compare(c)
	switch {
	case got.Error != "":
		b.t.Errorf("%s: the analyzer could not decide: %s", c, got.Error)
	case known == "" && !got.Same:
		b.t.Errorf("%s: the engine decides %s; the analyzer %s", c, decided(got.Engine), decided(got.Analyzer))
	case known != "" && got.Known != known:
		b.t.Errorf("%s: want the known difference %q, got same=%v known=%q (engine %s; analyzer %s)", c, known, got.Same, got.Known, decided(got.Engine), decided(got.Analyzer))
	}
}

// p1States are the states a lane is in, in a run of P1, that the workflow
// has in hand or waits on. A lane of P1 is never checking (the checks are
// the review's), awaiting approval (P2's gate), queued (P2's merge queue) or
// paused (P2's command).
var p1States = []State{Blocked, Ready, Implementing, Reviewing, ReviewRetry, Rework, Resolving, NeedsYou}

var allWhys = []string{WhyWorkerFailed, WhyNeedsHumanReview, WhyReviewMaxRetries, WhyConflictRetries, WhyNoPR, WhyMergeBlocked, WhyGuardBlocked, WhyGuardOutput, WhyInvalidTicket, WhyDependencyCycle, WhyPerson, WhyAgentRefused, WhyWorkerLost}

// variants are a state's lanes: every resolution of a lane that resolves,
// every reason of one that needs a person.
func variants(issue int, state State) []Lane {
	switch state {
	case Implementing:
		return []Lane{{Issue: issue, State: state}, {Issue: issue, State: state, Resolve: ResolveConflict}, {Issue: issue, State: state, Resolve: ResolveRebase}}
	case Resolving:
		return []Lane{{Issue: issue, State: state, Resolve: ResolveConflict}, {Issue: issue, State: state, Resolve: ResolveRebase}}
	case NeedsYou:
		var lanes []Lane
		for _, why := range allWhys {
			lanes = append(lanes, Lane{Issue: issue, State: state, Why: why})
		}
		return lanes
	}
	return []Lane{{Issue: issue, State: state}}
}

// dispatchedLabels are the labels of a lane the engine dispatched: the
// dispatch does not show on GitHub (design.md §5.1), so the ticket has the
// labels of where the lane was, or the Worker's in-progress.
func dispatchedLabels(l Lane, names config.Labels) [][]string {
	if l.State != Implementing {
		return nil
	}
	if l.Resolve != "" {
		return [][]string{Project(Lane{State: Resolving, Resolve: l.Resolve}, names).Labels}
	}
	return [][]string{{names.Task, names.InProgress}, {names.Task}}
}

// Every lane of P1, alone: its state, resolution and reason, with and
// without a pull request, its attempts under, at and over the limit, and
// every word GitHub has on its pull request.
func TestShadow_EveryLane(t *testing.T) {
	b := newShadowBench(t)
	names := config.DefaultLabels()
	counts := []int{0, 2, 3, 4}
	cases := 0
	for _, state := range p1States {
		for _, base := range variants(7, state) {
			for _, pr := range []int{0, 30} {
				for _, reviews := range counts {
					for _, conflicts := range counts {
						for _, m := range []string{"", "CONFLICTING", "UNKNOWN", "error"} {
							l := base
							l.PR = pr
							l.Attempts = Attempts{Reviews: reviews, Conflicts: conflicts}
							c := shadowCase{lanes: []Lane{l}, mergeable: map[int]string{30: m}}
							labelings := dispatchedLabels(l, names)
							if labelings == nil {
								b.expect(c, "")
								cases++
								continue
							}
							for _, labels := range labelings {
								c.labels = map[int][]string{7: labels}
								known := ""
								if l.Resolve == "" && !slices.Contains(labels, names.InProgress) {
									known = KnownCheckFirst
								}
								b.expect(c, known)
								cases++
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d cases", cases)
}

// Every set of the workflow's labels on a ticket, as a person or the
// Principal's workflow may leave it, taken up where its labels say (Infer,
// as Reconcile takes a ticket up), with and without a pull request.
func TestShadow_EveryLabelSet(t *testing.T) {
	b := newShadowBench(t)
	names := config.DefaultLabels()
	flags := []string{names.InProgress, names.PRReady, names.ReviewFailed, names.MergeConflict, names.NeedsRebase, names.WorkerFailed, names.NeedsHumanReview}
	cases := 0
	for set := 0; set < 1<<len(flags); set++ {
		labels := []string{names.Task}
		for i, flag := range flags {
			if set&(1<<i) != 0 {
				labels = append(labels, flag)
			}
		}
		in, ok := Infer(labels, false, false, names)
		if !ok {
			t.Fatalf("%v is no ticket", labels)
		}
		for _, pr := range []int{0, 30} {
			for _, m := range []string{"", "CONFLICTING"} {
				l := Lane{Issue: 7, State: in.State, Resolve: in.Resolve, Why: in.Why, PR: pr}
				b.expect(shadowCase{lanes: []Lane{l}, labels: map[int][]string{7: labels}, mergeable: map[int]string{30: m}}, "")
				cases++
			}
		}
	}
	t.Logf("%d cases", cases)
}

// Every two lanes of P1 at once, either way round: which is taken first.
// Two lanes in the same state are the known difference of the order.
func TestShadow_EveryTwoLanes(t *testing.T) {
	b := newShadowBench(t)
	names := config.DefaultLabels()
	var kinds []Lane
	for _, state := range p1States {
		kinds = append(kinds, variants(0, state)...)
	}
	cases := 0
	for _, first := range kinds {
		for _, second := range kinds {
			for _, m := range []string{"", "CONFLICTING"} {
				x, y := first, second
				x.Issue, x.PR = 3, 30
				y.Issue, y.PR = 5, 50
				c := shadowCase{lanes: []Lane{x, y}, mergeable: map[int]string{30: m, 50: m}, labels: map[int][]string{}}
				// A dispatched lane's ticket has the Worker's label.
				for _, l := range c.lanes {
					if l.State == Implementing {
						c.labels[l.Issue] = dispatchedLabels(l, names)[0]
					}
				}
				// The analyzer cannot tell two tickets with the same labels apart.
				same := strings.Join(c.labelsOf(x, names), ",") == strings.Join(c.labelsOf(y, names), ",")
				got := b.compare(c)
				switch {
				case got.Error != "":
					t.Errorf("%s: the analyzer could not decide: %s", c, got.Error)
				case got.Same:
				case got.Known == KnownOrder && same:
				default:
					t.Errorf("%s: the engine decides %s; the analyzer %s (known %q)", c, decided(got.Engine), decided(got.Analyzer), got.Known)
				}
				cases++
			}
		}
	}
	t.Logf("%d cases", cases)
}

// shadowed has a fixture's engine compare each of its decisions with the
// analyzer's, and checks, when the test ends, that each one was the same or
// a known difference: every flow of the engine's tests runs in the shadow.
func shadowed(f *engineFixture) {
	f.e.Shadow = func(ctx context.Context, tickets []Ticket, mergeable Mergeability, attempts func(string, int) (int, bool)) (*analyzer.Decision, error) {
		cfg := &config.Config{}
		cfg.GitHub.Labels = f.e.Names
		cfg.Escalation.MaxReviewAttempts = f.e.MaxReviews
		pr := func(issue int) int {
			if l := f.e.Journal.Lane(issue); l != nil {
				return l.PR
			}
			return f.an.prs[issue]
		}
		return NewShadow(f.e.Root, cfg, f.e.Only, prLookup{pr: pr})(ctx, tickets, mergeable, attempts)
	}
	f.t.Cleanup(func() {
		run, _, err := LastShadow(f.e.Root)
		if err != nil {
			f.t.Errorf("the shadow's comparisons cannot be read: %v", err)
		}
		if run.Failed > 0 {
			f.t.Errorf("%d of %d decisions could not be compared", run.Failed, run.Steps)
		}
		for _, d := range run.Differences {
			f.t.Errorf("the shadow differs: the engine %s; the analyzer %s", decided(d.Comparison.Engine), decided(d.Comparison.Analyzer))
		}
	})
}

// A run in the shadow: each decision is compared and written, and
// LastShadow reads how the last run compared: its differences, the
// comparisons that failed, and none when the last run had no shadow.
func TestShadow_ARunAndHowItCompared(t *testing.T) {
	// A project the engine has not run: nothing to show, and no error.
	if _, ok, err := LastShadow(t.TempDir()); ok || err != nil {
		t.Errorf("a project the engine has not run: %v, %v", ok, err)
	}

	f := newEngineFixture(t, task(12))
	f.an.next = []*analyzer.Decision{dispatch(12), {NextAction: analyzer.ActionAllComplete}}
	f.implement = []func(Lane) Transition{goTo(Reviewing, CauseImplemented, withPR(30))}
	f.review = []func(Lane) Transition{goTo(Merged, CauseMerged)}
	started := func() {
		if err := f.e.Journal.Note(TypeRun, "info", 0, map[string]any{"event": "started"}); err != nil {
			t.Fatal(err)
		}
	}
	started()
	if end := f.run(); end != EndComplete {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	run, ok, err := LastShadow(f.e.Root)
	// The ticket picked, its review, the run complete.
	if err != nil || !ok || run.Steps != 3 || run.Same != 3 || len(run.Differences) != 0 || run.Started.IsZero() {
		t.Fatalf("the run compared: %+v, %v, %v", run, ok, err)
	}

	// A decision the analyzer does not make, and one it cannot make.
	shadow := f.e.Shadow
	ctx := context.Background()
	mine := &analyzer.Decision{NextAction: analyzer.ActionReviewPR, IssueNumber: 12, PRNumber: 30}
	if c := f.e.compare(ctx, mine, f.e.mergeability()); !c.Differs() || c.Analyzer != nil {
		t.Errorf("a decision the analyzer does not make: %+v", c)
	}
	f.e.Shadow = func(context.Context, []Ticket, Mergeability, func(string, int) (int, bool)) (*analyzer.Decision, error) {
		return nil, errors.New("GitHub did not answer")
	}
	if c := f.e.compare(ctx, mine, f.e.mergeability()); c.Differs() || c.Error == "" {
		t.Errorf("a decision the analyzer cannot make: %+v", c)
	}
	f.e.Shadow = shadow
	run, _, _ = LastShadow(f.e.Root)
	if run.Steps != 5 || run.Failed != 1 || len(run.Differences) != 1 || run.Differences[0].Comparison.Engine.NextAction != analyzer.ActionReviewPR {
		t.Errorf("the differences: %+v", run)
	}
	if !strings.Contains(f.log.String(), "shadow: the engine decided review_pr #12 (PR #30); the Principal's workflow would have decided nothing of the tickets in hand") {
		t.Errorf("the log does not say what differed:\n%s", f.log.String())
	}

	// The next run has no shadow: there is nothing to show of it.
	started()
	if _, ok, err := LastShadow(f.e.Root); ok || err != nil {
		t.Errorf("a run with no shadow: %v, %v", ok, err)
	}
}
