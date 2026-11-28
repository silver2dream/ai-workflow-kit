package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
)

// mergeConfig is reviewConfig with a test command, which the queue runs on
// a commit the review did not approve.
const mergeConfig = `version: "1.0"
project:
  name: rally
  type: single-repo
repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      test: "go test ./..."
git:
  integration_branch: develop
  release_branch: main
review:
  human_feedback: false
lessons:
  enabled: false
`

// mergeFixture is a project whose pull request #30, of ticket #12, the
// review approved and the merge queue merges: GitHub is played by the
// fixture over the bare remote, and time by its clock.
type mergeFixture struct {
	*conflictFixture
	merge *MergeStep
	lane  Lane
	pub   *recordingPublisher
	calls []string
	clock time.Time

	// What GitHub says: the pull request's state ("" is OPEN), its checks
	// by head (the latest when a head has none of its own), whether the
	// project has a workflow for pull requests.
	state   string
	checks  func(head string, polls int) []Check
	polls   int
	expects bool
	// mergeErr is what a merge answers, once; mergeState GitHub's word on
	// it then.
	mergeErr   error
	mergeState string
	// testRun is what the tests say; onTest runs while they do.
	testRun reviewer.TestRun
	onTest  func()
	// gateSays is the maintainers' gate.
	gateSays *reviewer.SubmitReviewResult
	ticket   string
}

// newMergeFixture: with moved, the base moved on since the branch left it
// (writing those files); without, the branch is on top of the base.
func newMergeFixture(t *testing.T, moved map[string]string, pr map[string]string) *mergeFixture {
	t.Helper()
	if pr == nil {
		pr = map[string]string{"score.txt": scorePR}
	}
	onTop := moved == nil
	if onTop {
		moved = map[string]string{"notes.txt": "the base moved on\n"}
	}
	cf := newRepoFixture(t, map[string]string{"score.txt": scoreBase, "other.txt": "untouched\n"}, pr, moved)
	writeFile(t, filepath.Join(cf.root, ".ai", "config.yaml"), mergeConfig)
	if onTop {
		// The branch was rebased onto the base already, and pushed.
		gitIn(t, cf.wt, "rebase", "-q", "develop")
		gitIn(t, cf.wt, "push", "-q", "-f", "origin", "feat/ai-issue-12")
	}
	f := &mergeFixture{conflictFixture: cf, pub: &recordingPublisher{}, clock: time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC),
		ticket: "# Name the line\n\nThe second line says whose it is."}
	f.checks = func(string, int) []Check { return []Check{{Name: "test", Bucket: "pass"}} }
	f.lane = Lane{Issue: 12, State: Queued, PR: 30, Approved: f.remoteHead(), Seq: 60}
	f.merge = &MergeStep{
		Root: cf.root, Publisher: EnginePublisher{f.pub}, MaxConflicts: 3, MaxChecks: 3,
		now:   func() time.Time { return f.clock },
		sleep: func(_ context.Context, d time.Duration) error { f.clock = f.clock.Add(d); return nil },
		pull: func(_ context.Context, pr int) (PullRequest, error) {
			state := f.state
			if state == "" {
				state = "OPEN"
			}
			head := f.remoteHead()
			f.polls++
			return PullRequest{State: state, Head: head, Base: "develop", Checks: f.checks(head, f.polls)}, nil
		},
		merge: func(_ context.Context, pr int, strategy, head string) error {
			f.calls = append(f.calls, fmt.Sprintf("merge %d %s %s", pr, strategy, short(head)))
			if err := f.mergeErr; err != nil {
				f.mergeErr = nil
				return err
			}
			if head != f.remoteHead() {
				return errors.New("the head moved")
			}
			// GitHub's rebase merge, as the bare remote can have it.
			gitIn(t, f.origin, "update-ref", "refs/heads/develop", head)
			f.state = "MERGED"
			return nil
		},
		state:  func(context.Context, int) string { return f.mergeState },
		ticket: func(context.Context, int) (string, error) { return f.ticket, nil },
		test: func(_ context.Context, dir, command, _ string, _ time.Duration) reviewer.TestRun {
			f.calls = append(f.calls, "test "+short(strings.TrimSpace(gitIn(t, dir, "rev-parse", "HEAD"))))
			if f.onTest != nil {
				f.onTest()
			}
			run := f.testRun
			run.Command = command
			return run
		},
		gate: func(context.Context, reviewer.MergedOptions) *reviewer.SubmitReviewResult {
			f.calls = append(f.calls, "gate")
			return f.gateSays
		},
		landed: func(_ context.Context, o reviewer.MergedOptions) {
			f.calls = append(f.calls, fmt.Sprintf("landed #%d PR %d approved %s", o.IssueNumber, o.PRNumber, short(o.Head)))
		},
		expects: func(string, string) bool { return f.expects },
	}
	return f
}

// remoteHead is the pull request's branch on the remote.
func (f *mergeFixture) remoteHead() string {
	return strings.TrimSpace(gitIn(f.t, f.origin, "rev-parse", "feat/ai-issue-12"))
}

func (f *mergeFixture) run() Transition {
	f.t.Helper()
	tr, err := f.merge.Merge(context.Background(), f.lane)
	if err != nil {
		f.t.Fatalf("Merge: %v", err)
	}
	next, err := Apply(&f.lane, tr)
	if err != nil {
		f.t.Fatalf("the lane cannot move as the merge says: %v", err)
	}
	f.lane = *next
	if (&laneTree{wt: f.wt}).rebasing() {
		f.t.Error("the worktree was left in a rebase")
	}
	return tr
}

func (f *mergeFixture) said() string { return strings.Join(f.calls, "\n") }

// The approved commit on top of the base: nothing is tested again or
// pushed; the checks passed, so it is merged as it is.
func TestMerge_TheApprovedCommitAsItIs(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	head := f.remoteHead()

	tr := f.run()
	if tr.To != Merged || tr.Cause != CauseMerged {
		t.Fatalf("went %s (%s): %s", tr.To, tr.Cause, tr.Note)
	}
	want := fmt.Sprintf("gate\nmerge 30 squash %s\nlanded #12 PR 30 approved %s", short(head), short(head))
	if got := f.said(); got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}
	if f.remoteHead() != head {
		t.Error("the branch was pushed")
	}
}

// The base moved on: the branch is rebased onto it, tested, pushed, and
// merged once the checks of the new commit passed.
func TestMerge_RebasedTestedPushedMerged(t *testing.T) {
	f := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, nil)
	approved := f.lane.Approved
	// The checks of the new commit run for two looks, then pass.
	f.checks = func(head string, polls int) []Check {
		if head == approved || polls < 3 {
			return []Check{{Name: "test", Bucket: "pending"}}
		}
		return []Check{{Name: "test", Bucket: "pass"}, {Name: "lint", Bucket: "skipping"}}
	}

	tr := f.run()
	if tr.To != Merged {
		t.Fatalf("went %s (%s): %s\n%s", tr.To, tr.Cause, tr.Note, f.said())
	}
	head := f.remoteHead()
	if head == approved {
		t.Fatal("the rebased branch was not pushed")
	}
	if got := gitIn(t, f.origin, "show", "develop:notes.txt"); got != "the base moved on\n" {
		t.Errorf("the merge lost the base's change: %q", got)
	}
	if got := gitIn(t, f.origin, "show", "develop:score.txt"); got != scorePR {
		t.Errorf("the merge lost the pull request's change: %q", got)
	}
	want := fmt.Sprintf("test %s\ngate\nmerge 30 squash %s\nlanded #12 PR 30 approved %s", short(head), short(head), short(approved))
	if got := f.said(); got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}
	if !f.clock.After(time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)) {
		t.Error("the pending checks were not waited for")
	}
}

// A person's push since the approval is taken in, and the commit, not the
// one approved, is tested before it is merged.
func TestMerge_APersonsPush(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	other := t.TempDir()
	gitIn(t, other, "clone", "-q", "-b", "feat/ai-issue-12", f.origin, ".")
	for _, kv := range [][2]string{{"user.email", "p@example.invalid"}, {"user.name", "p"}, {"core.autocrlf", "false"}} {
		gitIn(t, other, "config", kv[0], kv[1])
	}
	writeFile(t, filepath.Join(other, "other.txt"), "a person's touch\n")
	gitIn(t, other, "commit", "-qam", "[fix] a person's touch")
	gitIn(t, other, "push", "-q", "origin", "feat/ai-issue-12")
	pushed := f.remoteHead()

	tr := f.run()
	if tr.To != Merged {
		t.Fatalf("went %s (%s): %s\n%s", tr.To, tr.Cause, tr.Note, f.said())
	}
	if !strings.HasPrefix(f.said(), "test "+short(pushed)+"\n") || !strings.Contains(f.said(), "merge 30 squash "+short(pushed)) {
		t.Errorf("calls:\n%s", f.said())
	}
}

// A conflict with the base: the rebase is undone, nothing is pushed, and
// the pull request is sent to be resolved, as often as it may be.
func TestMerge_AConflict(t *testing.T) {
	f := newMergeFixture(t, map[string]string{"score.txt": scoreMoved}, nil)
	before := f.remoteHead()

	tr := f.run()
	if tr.To != Resolving || tr.Resolve != ResolveConflict || f.lane.Attempts.Conflicts != 1 || f.lane.Approved == "" {
		t.Fatalf("went %s (%s, %s): %s; lane %+v", tr.To, tr.Cause, tr.Resolve, tr.Note, f.lane)
	}
	if !strings.Contains(tr.Note, "score.txt") || f.remoteHead() != before || f.said() != "" {
		t.Errorf("note %q; calls:\n%s", tr.Note, f.said())
	}

	f2 := newMergeFixture(t, map[string]string{"score.txt": scoreMoved}, nil)
	f2.lane.Attempts.Conflicts = 3
	if tr := f2.run(); tr.To != NeedsYou || tr.Why != WhyConflictRetries {
		t.Errorf("at the limit: went %s (%s): %s", tr.To, tr.Why, tr.Note)
	}
}

// Tests that fail on the rebased commit send it back with what they
// printed, before anything is pushed; as often as they may.
func TestMerge_TestsThatFail(t *testing.T) {
	f := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, nil)
	before := f.remoteHead()
	f.testRun = reviewer.TestRun{Output: "--- FAIL: TestScore\n    score_test.go:9: got 2", Err: errors.New("exit status 1")}

	tr := f.run()
	if tr.To != Rework || tr.Cause != CauseCheckFailed || f.lane.Attempts.Checks != 1 {
		t.Fatalf("went %s (%s): %s; lane %+v", tr.To, tr.Cause, tr.Note, f.lane)
	}
	if f.remoteHead() != before {
		t.Error("a commit whose tests failed was pushed")
	}
	if got := strings.Join(f.pub.calls, "\n"); got != "comment-issue 12 ## AWK Review：合併前的測試沒過" {
		t.Errorf("published:\n%s", got)
	}

	f2 := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, nil)
	f2.testRun = f.testRun
	f2.lane.Attempts.Checks = 3
	if tr := f2.run(); tr.To != NeedsYou || tr.Why != WhyCheckRetries || len(f2.pub.calls) != 0 {
		t.Errorf("at the limit: went %s (%s): %s; published %v", tr.To, tr.Why, tr.Note, f2.pub.calls)
	}
}

// A check that fails on GitHub sends it back, naming the check.
func TestMerge_ChecksThatFail(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	f.checks = func(string, int) []Check {
		return []Check{{Name: "build", Bucket: "pass"}, {Name: "e2e", Bucket: "fail"}, {Name: "lint", Bucket: "cancel"}}
	}
	tr := f.run()
	if tr.To != Rework || tr.Cause != CauseCheckFailed || !strings.Contains(tr.Note, "e2e, lint") {
		t.Fatalf("went %s (%s): %s", tr.To, tr.Cause, tr.Note)
	}
	if strings.Contains(f.said(), "merge") {
		t.Errorf("calls:\n%s", f.said())
	}
	if got := strings.Join(f.pub.calls, "\n"); got != "comment-issue 12 ## AWK Review：合併前的CI沒過" {
		t.Errorf("published:\n%s", got)
	}
}

// The diff policy holds the rebased change to what the ticket allows.
func TestMerge_TheDiffPolicy(t *testing.T) {
	secret := map[string]string{"score.txt": scorePR, "keys.go": "package keys\n\nconst x = 1 // api_key = \"abc123\"\n"}
	f := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, secret)
	tr := f.run()
	if tr.To != NeedsYou || tr.Why != WhyDiffPolicy || !strings.Contains(tr.Note, "allow_secrets") {
		t.Fatalf("went %s (%s): %s", tr.To, tr.Why, tr.Note)
	}
	if strings.Contains(f.said(), "test") {
		t.Errorf("tested after the policy refused it:\n%s", f.said())
	}

	// A ticket that allows it.
	f2 := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, secret)
	f2.ticket = "# Keys\n\n" + body("keys.go, score.txt", "allow_secrets: true")
	if tr := f2.run(); tr.To != Merged {
		t.Errorf("allowed: went %s (%s): %s", tr.To, tr.Why, tr.Note)
	}
}

// No checks: none in time is a project without CI, unless it has a workflow
// for pull requests, whose checks did not start; checks that do not end in
// time are for a person.
func TestMerge_ChecksThatDoNotCome(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	f.checks = func(string, int) []Check { return nil }
	if tr := f.run(); tr.To != Merged || !f.clock.After(time.Date(2026, 11, 28, 9, 1, 29, 0, time.UTC)) {
		t.Errorf("without CI: went %s at %v: %s", tr.To, f.clock, tr.Note)
	}

	f = newMergeFixture(t, nil, nil)
	f.checks = func(string, int) []Check { return nil }
	f.expects = true
	if tr := f.run(); tr.To != NeedsYou || tr.Why != WhyChecksStuck || strings.Contains(f.said(), "merge") {
		t.Errorf("with a workflow: went %s (%s): %s", tr.To, tr.Why, tr.Note)
	}

	f = newMergeFixture(t, nil, nil)
	f.checks = func(string, int) []Check { return []Check{{Name: "e2e", Bucket: "pending"}} }
	f.merge.Poll = 5 * time.Minute
	if tr := f.run(); tr.To != NeedsYou || tr.Why != WhyChecksStuck || !strings.Contains(tr.Note, "e2e") || f.clock.Sub(time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)) < 40*time.Minute {
		t.Errorf("too long: went %s (%s) at %v: %s", tr.To, tr.Why, f.clock, tr.Note)
	}
}

// The maintainers' gate, asked again before the merge: open feedback sends
// it back, or to a person.
func TestMerge_TheMaintainersGate(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	f.gateSays = &reviewer.SubmitReviewResult{Result: "changes_requested", Next: reviewer.NextRework, Reason: "a maintainer asked for changes"}
	if tr := f.run(); tr.To != Rework || tr.Cause != CauseChangesWanted || strings.Contains(f.said(), "merge") {
		t.Errorf("sent back: went %s (%s): %s\n%s", tr.To, tr.Cause, tr.Note, f.said())
	}
	f = newMergeFixture(t, nil, nil)
	f.gateSays = &reviewer.SubmitReviewResult{Result: "review_blocked", Next: reviewer.NextNeedsYou, NeedsYou: reviewer.NeedsYouMaintainer}
	if tr := f.run(); tr.To != NeedsYou || tr.Why != WhyMaintainer {
		t.Errorf("held: went %s (%s): %s", tr.To, tr.Why, tr.Note)
	}
}

// A merge GitHub refused: a conflict is resolved, a base that moved is
// tried again (three times at most), anything else is for a person.
func TestMerge_Refused(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	f.mergeErr, f.mergeState = errors.New("not mergeable"), "DIRTY"
	if tr := f.run(); tr.To != Resolving || tr.Resolve != ResolveConflict {
		t.Errorf("DIRTY: went %s (%s): %s", tr.To, tr.Cause, tr.Note)
	}

	f = newMergeFixture(t, nil, nil)
	f.mergeErr, f.mergeState = errors.New("protected"), "BLOCKED"
	if tr := f.run(); tr.To != NeedsYou || tr.Why != WhyMergeBlocked || !strings.Contains(tr.Note, "BLOCKED") {
		t.Errorf("BLOCKED: went %s (%s): %s", tr.To, tr.Why, tr.Note)
	}

	// Behind once: the second try merges.
	f = newMergeFixture(t, nil, nil)
	f.mergeErr, f.mergeState = errors.New("behind"), "BEHIND"
	if tr := f.run(); tr.To != Merged || strings.Count(f.said(), "merge 30") != 2 {
		t.Errorf("BEHIND once: went %s: %s\n%s", tr.To, tr.Note, f.said())
	}

	// Behind every time: a person, after three tries.
	f = newMergeFixture(t, nil, nil)
	f.mergeState = "BEHIND"
	f.merge.merge = func(context.Context, int, string, string) error {
		f.calls = append(f.calls, "merge")
		return errors.New("behind")
	}
	if tr := f.run(); tr.To != NeedsYou || tr.Why != WhyMergeBlocked || strings.Count(f.said(), "merge") != maxMergeTries {
		t.Errorf("BEHIND always: went %s (%s): %s\n%s", tr.To, tr.Why, tr.Note, f.said())
	}
}

// A push that was refused because the branch moved meanwhile: the merge
// starts over with what was pushed.
func TestMerge_ABranchThatMovesWhileItIsTested(t *testing.T) {
	f := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, nil)
	other := t.TempDir()
	gitIn(t, other, "clone", "-q", "-b", "feat/ai-issue-12", f.origin, ".")
	for _, kv := range [][2]string{{"user.email", "p@example.invalid"}, {"user.name", "p"}, {"core.autocrlf", "false"}} {
		gitIn(t, other, "config", kv[0], kv[1])
	}
	pushed := false
	f.onTest = func() {
		if pushed {
			return
		}
		pushed = true
		writeFile(t, filepath.Join(other, "other.txt"), "pushed while the queue tested\n")
		gitIn(t, other, "commit", "-qam", "[fix] meanwhile")
		gitIn(t, other, "push", "-q", "origin", "feat/ai-issue-12")
	}

	tr := f.run()
	if tr.To != Merged || strings.Count(f.said(), "test ") != 2 {
		t.Fatalf("went %s (%s): %s\n%s", tr.To, tr.Cause, tr.Note, f.said())
	}
	if got := gitIn(t, f.origin, "show", "develop:other.txt"); got != "pushed while the queue tested\n" {
		t.Errorf("what was pushed meanwhile was lost: %q", got)
	}
}

// A pull request merged already (the engine stopped after its merge) is
// over; one closed is for a person.
func TestMerge_APullRequestThatIsOver(t *testing.T) {
	f := newMergeFixture(t, nil, nil)
	f.state = "MERGED"
	if tr := f.run(); tr.To != Merged || !strings.HasPrefix(f.said(), "landed #12") {
		t.Errorf("merged: went %s: %s\n%s", tr.To, tr.Note, f.said())
	}
	f = newMergeFixture(t, nil, nil)
	f.state = "CLOSED"
	if tr := f.run(); tr.To != NeedsYou || tr.Why != WhyNoPR || f.said() != "" {
		t.Errorf("closed: went %s (%s): %s\n%s", tr.To, tr.Why, tr.Note, f.said())
	}
}

// A stop at once ends the merge where it is: nothing is merged, and the
// worktree is not left in a rebase.
func TestMerge_AStopAtOnce(t *testing.T) {
	f := newMergeFixture(t, map[string]string{"notes.txt": "the base moved on\n"}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	f.checks = func(string, int) []Check { return []Check{{Name: "test", Bucket: "pending"}} }
	f.merge.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}
	if _, err := f.merge.Merge(ctx, f.lane); !errors.Is(err, context.Canceled) {
		t.Fatalf("Merge: %v", err)
	}
	if strings.Contains(f.said(), "merge") {
		t.Errorf("calls:\n%s", f.said())
	}
	if (&laneTree{wt: f.wt}).rebasing() {
		t.Error("the worktree was left in a rebase")
	}
}

// Only a lane in the queue, with a pull request, is merged.
func TestMerge_NotALaneToMerge(t *testing.T) {
	s := &MergeStep{Root: t.TempDir()}
	if _, err := s.Merge(context.Background(), Lane{Issue: 1, State: Reviewing, PR: 3}); err == nil {
		t.Error("a lane in review was merged")
	}
	if _, err := s.Merge(context.Background(), Lane{Issue: 1, State: Queued}); err == nil {
		t.Error("a lane without a pull request was merged")
	}
}

// What gh says of a pull request's checks is put in its buckets.
func TestParsePR(t *testing.T) {
	data := `{"state":"OPEN","headRefOid":"abc","baseRefName":"develop","statusCheckRollup":[
		{"__typename":"CheckRun","name":"go","status":"COMPLETED","conclusion":"SUCCESS"},
		{"__typename":"CheckRun","name":"web","status":"IN_PROGRESS","conclusion":""},
		{"__typename":"CheckRun","name":"e2e","status":"COMPLETED","conclusion":"FAILURE"},
		{"__typename":"CheckRun","name":"docs","status":"COMPLETED","conclusion":"SKIPPED"},
		{"__typename":"CheckRun","name":"old","status":"COMPLETED","conclusion":"CANCELLED"},
		{"__typename":"CheckRun","name":"slow","status":"COMPLETED","conclusion":"TIMED_OUT"},
		{"__typename":"CheckRun","name":"queued","status":"QUEUED","conclusion":""},
		{"__typename":"StatusContext","context":"ci/legacy","state":"PENDING"},
		{"__typename":"StatusContext","context":"ci/ok","state":"SUCCESS"},
		{"__typename":"StatusContext","context":"ci/bad","state":"ERROR"}
	]}`
	pr, err := parsePR([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if pr.State != "OPEN" || pr.Head != "abc" || pr.Base != "develop" {
		t.Errorf("pr: %+v", pr)
	}
	var got []string
	for _, c := range pr.Checks {
		got = append(got, c.Name+"="+c.Bucket)
	}
	want := "go=pass web=pending e2e=fail docs=skipping old=cancel slow=fail queued=pending ci/legacy=pending ci/ok=pass ci/bad=fail"
	if strings.Join(got, " ") != want {
		t.Errorf("checks:\n%s\nwant:\n%s", strings.Join(got, " "), want)
	}
	if _, err := parsePR([]byte("not json")); err == nil {
		t.Error("what cannot be read was read")
	}
}

// The queue: the lanes in it, by when they came into it, then by number.
func TestQueue(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 11, 28, 9, m, 0, 0, time.UTC) }
	lanes := []Lane{
		{Issue: 1, State: Queued, Since: at(5)},
		{Issue: 2, State: Reviewing, Since: at(1)},
		{Issue: 3, State: Queued, Since: at(2)},
		{Issue: 4, State: Queued, Since: at(5)},
		{Issue: 5, State: Resolving, Since: at(0)},
	}
	var got []int
	for _, l := range queue(lanes) {
		got = append(got, l.Issue)
	}
	if fmt.Sprint(got) != "[3 1 4]" {
		t.Errorf("queue: %v", got)
	}
}

// tail keeps the end of what was printed, from a line's start.
func TestTail(t *testing.T) {
	if got := tail("  short  ", 10); got != "short" {
		t.Errorf("short: %q", got)
	}
	long := strings.Repeat("x", 20) + "\nkeep this\nand this"
	if got := tail(long, 20); got != "…\nkeep this\nand this" {
		t.Errorf("long: %q", got)
	}
}
