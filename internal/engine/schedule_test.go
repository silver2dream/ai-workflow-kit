package engine

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

func TestTakesSlot(t *testing.T) {
	takes := map[State]bool{Implementing: true, Checking: true, Reviewing: true, ReviewRetry: true, Resolving: true}
	for _, s := range States {
		if got := takesSlot(s); got != takes[s] {
			t.Errorf("%s takes a slot: %v, want %v", s, got, takes[s])
		}
	}
	lanes := []Lane{{State: Implementing}, {State: Rework}, {State: NeedsYou}, {State: Reviewing}, {State: Queued}}
	if n := taken(lanes); n != 2 {
		t.Errorf("taken = %d, want 2", n)
	}
}

func TestHoldsPaths(t *testing.T) {
	holding := map[State]bool{Implementing: true, Checking: true, Reviewing: true, ReviewRetry: true, Rework: true, Resolving: true, AwaitingApproval: true, Queued: true}
	for _, s := range States {
		if s == NeedsYou || s == Paused {
			continue
		}
		if got := holdsPaths(Lane{State: s}); got != holding[s] {
			t.Errorf("%s holds its paths: %v, want %v", s, got, holding[s])
		}
	}
	for _, tt := range []struct {
		name string
		lane Lane
		want bool
	}{
		{"waits for a person, with a pull request", Lane{State: NeedsYou, PR: 30, Why: WhyInvalidTicket}, true},
		{"the Worker failed before a pull request", Lane{State: NeedsYou, Why: WhyWorkerFailed}, true},
		{"a Worker that left nothing", Lane{State: NeedsYou, Why: WhyWorkerLost}, true},
		{"a person marked it", Lane{State: NeedsYou, Why: WhyPerson}, true},
		{"a ticket that cannot be used", Lane{State: NeedsYou, Why: WhyInvalidTicket}, false},
		{"a ticket in a circle", Lane{State: NeedsYou, Why: WhyDependencyCycle}, false},
		{"paused before it started", Lane{State: Paused, Resume: Ready, Why: "a person paused it"}, false},
		{"paused while blocked", Lane{State: Paused, Resume: Blocked}, false},
		{"paused in rework", Lane{State: Paused, Resume: Rework}, true},
		{"paused in review", Lane{State: Paused, Resume: Reviewing}, true},
		{"paused while it waited for a person", Lane{State: Paused, Resume: NeedsYou}, true},
	} {
		if got := holdsPaths(tt.lane); got != tt.want {
			t.Errorf("%s: holds its paths: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPathsOf(t *testing.T) {
	for name, tt := range map[string]struct {
		body string
		want []string
	}{
		"schema 2, lowered": {"```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [Docs/A.md, src/**]\nrisk: low\n```\n", []string{"docs/a.md", "src/**"}},
		"schema 1":          {"```yaml awk-ticket\nschema: 1\nrepo: root\n```\n", nil},
		"no block":          {"## Summary\nx\n", nil},
		"a broken block":    {"```yaml awk-ticket\nschema: 2\nrepo: root\n```\n", nil},
	} {
		if got := pathsOfBody(tt.body); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %v, want %v", name, got, tt.want)
		}
	}
	if got := pathsOf(ticket.Meta{Paths: []string{"A/**"}}, false); got != nil {
		t.Errorf("a ticket without a block declares %v", got)
	}
}

func TestHeldBy(t *testing.T) {
	holds := []hold{{Issue: 4, Paths: []string{"a/**"}}, {Issue: 6, Paths: []string{"b/x.go"}}, {Issue: 8, Paths: []string{"c/*.go", "d/**"}}}
	for _, tt := range []struct {
		name  string
		issue int
		paths []string
		want  int
	}{
		{"apart from every lane", 9, []string{"e/**"}, 0},
		{"in a lane's folder", 9, []string{"a/y/z.go"}, 4},
		{"the same file", 9, []string{"b/x.go"}, 6},
		{"a file next to a lane's", 9, []string{"b/y.go"}, 0},
		{"one of several paths", 9, []string{"e/**", "d/q"}, 8},
		{"declares none", 9, nil, 4},
		{"its own lane does not keep it off", 4, []string{"a/**"}, 0},
		{"its own lane, and another", 4, []string{"a/**", "b/**"}, 6},
	} {
		if got := heldBy(holds, tt.issue, tt.paths); got != tt.want {
			t.Errorf("%s: held by #%d, want #%d", tt.name, got, tt.want)
		}
	}
	// A lane that declares none keeps off every ticket but its own.
	everything := []hold{{Issue: 5, Paths: nil}}
	if got := heldBy(everything, 9, []string{"e/**"}); got != 5 {
		t.Errorf("a lane that declares no paths: held by #%d, want #5", got)
	}
	if got := heldBy(everything, 5, nil); got != 0 {
		t.Errorf("a lane that declares no paths keeps off its own ticket")
	}
}

func TestCycles(t *testing.T) {
	for _, tt := range []struct {
		name  string
		waits map[int][]int
		want  []Cycle
	}{
		{"none", map[int][]int{3: {1}, 5: {3, 1}}, nil},
		{"two", map[int][]int{3: {5}, 5: {3}, 7: {3}}, []Cycle{{Members: []int{3, 5}, Round: []int{3, 5}}}},
		{"three, the shortest way round", map[int][]int{8: {3}, 3: {5}, 5: {8}}, []Cycle{{Members: []int{3, 5, 8}, Round: []int{3, 5, 8}}}},
		{"two apart", map[int][]int{1: {2}, 2: {1}, 10: {11}, 11: {12}, 12: {10}, 4: {1}},
			[]Cycle{{Members: []int{1, 2}, Round: []int{1, 2}}, {Members: []int{10, 11, 12}, Round: []int{10, 11, 12}}}},
		{"two circles through one ticket", map[int][]int{1: {2}, 2: {1, 3}, 3: {2}},
			[]Cycle{{Members: []int{1, 2, 3}, Round: []int{1, 2}}}},
		{"a long way and a short one", map[int][]int{1: {2, 4}, 2: {3}, 3: {1}, 4: {1}},
			[]Cycle{{Members: []int{1, 2, 3, 4}, Round: []int{1, 4}}}},
		{"one that waits for itself", map[int][]int{5: {5}, 6: {5}}, nil},
		{"one that waits for itself, in a circle", map[int][]int{1: {1, 2}, 2: {1}}, []Cycle{{Members: []int{1, 2}, Round: []int{1, 2}}}},
	} {
		if got := Cycles(tt.waits); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %+v, want %+v", tt.name, got, tt.want)
		}
	}
	note := Cycle{Members: []int{3, 5, 8}, Round: []int{3, 5, 8}}.Note()
	if !strings.Contains(note, "#3, #5 and #8 wait for each other in a circle (#3 → #5 → #8 → #3") {
		t.Errorf("note: %s", note)
	}
	if note := (Cycle{Members: []int{1, 2}, Round: []int{1, 2}}).Note(); !strings.HasPrefix(note, "#1 and #2 wait") {
		t.Errorf("note: %s", note)
	}
}

// body is an issue body with a schema 2 block for the root.
func body(paths string, more ...string) string {
	b := "## Summary\nwork\n\n```yaml awk-ticket\nschema: 2\nrepo: root\npaths: [" + paths + "]\nrisk: low\n"
	for _, line := range more {
		b += line + "\n"
	}
	return b + "```\n"
}

func withBody(t Ticket, b string) Ticket {
	t.Body = b
	return t
}

// atRework records a lane that was written and is to be written again.
func atRework(t *testing.T, f *engineFixture, issue int) {
	t.Helper()
	at := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	for _, tr := range []Transition{
		{Issue: issue, To: Ready, Cause: CauseOpened, At: at},
		{Issue: issue, From: Ready, To: Implementing, Cause: CauseDispatched, At: at},
		{Issue: issue, From: Implementing, To: Rework, Cause: CauseImplementFail, At: at},
	} {
		if _, err := f.e.Journal.Record(tr); err != nil {
			t.Fatal(err)
		}
	}
}

// A lane to be written again holds its ticket's paths: a ticket whose paths
// meet them waits, though its number comes first, and the lane is written.
// One whose paths are apart starts first, by its number.
func TestEngine_PathsALaneHolds(t *testing.T) {
	for _, tt := range []struct {
		name     string
		three    string
		five     string
		first    string
		waitSaid bool
	}{
		{"meet", body("src/a.go"), body("src/**"), "implement #5", true},
		{"meet, in another case", body("SRC/A.go"), body("src/**"), "implement #5", true},
		{"apart", body("docs/**"), body("src/**"), "implement #3", false},
		{"the lane declares none", body("docs/**"), "```yaml awk-ticket\nschema: 1\nrepo: root\n```\n", "implement #5", true},
		{"the ticket declares none", "## Summary\nno block\n", body("src/**"), "implement #5", true},
	} {
		f := newEngineFixture(t, withBody(task(3), tt.three), withBody(task(5), tt.five))
		atRework(t, f, 5)
		f.an.pending = f.gh.pendingOf
		f.implement = []func(Lane) Transition{goTo(NeedsYou, CauseEscalated, withWhy(WhyWorkerFailed))}

		if end := f.run(); end != analyzer.ReasonWorkerFailed {
			t.Fatalf("%s: ended %q\n%s", tt.name, end, f.log.String())
		}
		if got := strings.Join(f.steps, ", "); got != tt.first {
			t.Errorf("%s: steps %s, want %s", tt.name, got, tt.first)
		}
		if said := strings.Contains(f.log.String(), "#3 can start, and waits: #5 holds paths it may change"); said != tt.waitSaid {
			t.Errorf("%s: the wait said: %v\n%s", tt.name, said, f.log.String())
		}
		for _, max := range f.an.maxAsked {
			if max != 1 {
				t.Errorf("%s: %d tickets asked for; one lane at a time by default (engine.max_parallel)", tt.name, max)
			}
		}
	}
}

// When every ticket that could start is kept off by a lane nobody is at work
// on (one paused), the run ends for a person, and names that lane.
func TestEngine_EveryTicketKeptOff(t *testing.T) {
	f := newEngineFixture(t, withBody(task(3), body("src/a.go")), withBody(task(5), body("src/**")))
	atRework(t, f, 5)
	if _, err := f.e.Journal.Record(Transition{Issue: 5, From: Rework, To: Paused, Cause: CausePaused, Note: "a person paused it", At: time.Date(2026, 11, 24, 8, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	f.an.pending = func() []Ticket { return f.gh.pendingOf()[:1] } // a paused lane is not picked

	d, err := f.e.Next(t.Context())

	if err != nil {
		t.Fatal(err)
	}
	if d.NextAction != analyzer.ActionNone || d.IssueNumber != 5 || d.ExitReason != analyzer.ReasonNeedsHumanReview {
		t.Errorf("decision = %+v, want none, for a person, naming #5", d)
	}
	if end := f.run(); end != analyzer.ReasonNeedsHumanReview || len(f.steps) != 0 {
		t.Errorf("ended %q with steps %v", end, f.steps)
	}
}

// Tickets that wait for each other in a circle are handed to a person, each
// told which they are and how they go round; one that waits for them is not.
func TestEngine_ACircle(t *testing.T) {
	v1 := func(deps string) string {
		return "## Summary\nx\n\n```yaml awk-ticket\nschema: 1\nrepo: root\ndepends_on: [" + deps + "]\n```\n"
	}
	// #5 waits for itself: a ticket that cannot be used, which the analyzer
	// hands to a person as such, not a circle.
	f := newEngineFixture(t, withBody(task(1), v1("2")), withBody(task(2), v1("1")), withBody(task(3), v1("1")), withBody(task(4), v1("9")), withBody(task(5), v1("5")))
	f.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionNone, IssueNumber: 1, ExitReason: analyzer.ReasonBlockedByDependencies}}

	if end := f.run(); end != analyzer.ReasonNeedsHumanReview {
		t.Fatalf("ended %q\n%s", end, f.log.String())
	}
	for _, issue := range []int{1, 2} {
		l := f.lane(issue)
		if l.State != NeedsYou || l.Why != WhyDependencyCycle || !strings.Contains(l.Note, "#1 and #2 wait for each other in a circle (#1 → #2 → #1") {
			t.Errorf("#%d: %+v", issue, l)
		}
		if got := f.gh.labels(issue); got != "ai-task,needs-human-review" {
			t.Errorf("#%d labels: %s", issue, got)
		}
	}
	// Reconciliation took every open ticket up: the ones outside the circle
	// wait as they were.
	if f.lane(3).State != Ready || f.lane(4).State != Ready || f.lane(5).State != Ready {
		t.Errorf("a ticket outside the circle was moved: #3 %+v, #4 %+v, #5 %+v", f.lane(3), f.lane(4), f.lane(5))
	}
	if len(f.gh.comments) != 2 || !strings.HasPrefix(f.gh.comments[0], "#1 ") || !strings.HasPrefix(f.gh.comments[1], "#2 ") ||
		!strings.Contains(f.gh.comments[0], WhyText(WhyDependencyCycle)) {
		t.Errorf("comments: %q", f.gh.comments)
	}

	// Without a circle, a run blocked by dependencies ends as it did.
	g := newEngineFixture(t, withBody(task(3), v1("9")))
	g.an.next = []*analyzer.Decision{{NextAction: analyzer.ActionNone, IssueNumber: 3, ExitReason: analyzer.ReasonBlockedByDependencies}}
	if end := g.run(); end != analyzer.ReasonBlockedByDependencies {
		t.Errorf("without a circle: ended %q", end)
	}
	if l := g.lane(3); l.State != Blocked {
		t.Errorf("without a circle: #3 is %+v", l)
	}
}
