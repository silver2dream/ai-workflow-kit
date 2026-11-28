package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/hooks"
	"github.com/silver2dream/ai-workflow-kit/internal/landing"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// The merge queue (design.md §5.4, task 2.3). A pull request the review
// approved waits in the queue (Queued), and the queue merges one at a time,
// the one that came first: it rebases the branch onto the integration branch
// as it is now, checks again a commit the review did not approve (the diff
// policy, the tests), pushes it, waits for CI, asks the maintainers' review
// once more, and merges exactly that commit. What fails sends the lane where
// it can be dealt with (resolving, rework, a person), and the queue goes on
// with the next. The step says where the lane goes; the engine's loop
// records it.

// ActionMerge is the engine's decision to merge the pull request at the head
// of its merge queue: a step of the engine's own, which the analyzer, with
// no queue, never decides.
const ActionMerge = "merge_pr"

// maxMergeTries bounds how often one merge starts over because the branch
// or the integration branch moved under it: the step ends, for a person,
// rather than chase them.
const maxMergeTries = 3

// maxCheckOutput is how much of what failed tests printed the comment that
// sends the lane back carries.
const maxCheckOutput = 4000

// MergeStep merges the pull request at the head of the merge queue.
type MergeStep struct {
	// Root is the project.
	Root string
	// Log is where the step says what it does and waits for; nil says
	// nothing.
	Log io.Writer
	// MaxConflicts bounds how often a lane's conflict is sent to be
	// resolved, MaxChecks how often its checks may fail in the queue; 0 is
	// what the project's configuration gives (LimitsOf).
	MaxConflicts, MaxChecks int
	// Publisher does what follows the merge, and comments; nil is the
	// engine's.
	Publisher reviewer.Publisher
	// GHTimeout bounds each call to GitHub, GitTimeout each git command; 0
	// is a minute and five minutes.
	GHTimeout, GitTimeout time.Duration
	// How the checks are waited for, as the landing of approved documents
	// waits (internal/landing): looked at every Poll (15s), for at most Wait
	// (40 minutes); none for Grace (90s) is a project with no CI, unless it
	// has a workflow for pull requests, whose checks may take Absent (10
	// minutes) to start.
	Poll, Wait, Grace, Absent time.Duration

	once sync.Once
	// Replaced in tests.
	now     func() time.Time
	sleep   func(ctx context.Context, d time.Duration) error
	pull    func(ctx context.Context, pr int) (PullRequest, error)
	merge   func(ctx context.Context, pr int, strategy, head string) error
	state   func(ctx context.Context, pr int) string
	ticket  func(ctx context.Context, issue int) (string, error)
	test    func(ctx context.Context, dir, command, language string, timeout time.Duration) reviewer.TestRun
	gate    func(ctx context.Context, o reviewer.MergedOptions) *reviewer.SubmitReviewResult
	landed  func(ctx context.Context, o reviewer.MergedOptions)
	expects func(dir, base string) bool
}

// PullRequest is what the queue reads of a pull request.
type PullRequest struct {
	// State is OPEN, CLOSED or MERGED.
	State string
	// Head is the commit at its head; Base the branch it goes into.
	Head, Base string
	// Checks are the checks of its head.
	Checks []Check
}

// Check is one check of a pull request's head: its name, and where it is
// (pass, fail, pending, skipping, cancel), as gh's buckets say it.
type Check struct {
	Name, Bucket string
}

func (s *MergeStep) defaults() { s.once.Do(s.setDefaults) }

func (s *MergeStep) setDefaults() {
	if s.Log == nil {
		s.Log = io.Discard
	}
	if s.MaxConflicts <= 0 || s.MaxChecks <= 0 {
		cfg, err := config.Load(s.Root)
		if err != nil {
			cfg = nil
		}
		limits := LimitsOf(cfg)
		if s.MaxConflicts <= 0 {
			s.MaxConflicts = limits.Conflicts
		}
		if s.MaxChecks <= 0 {
			s.MaxChecks = limits.Checks
		}
	}
	if s.GHTimeout == 0 {
		s.GHTimeout = time.Minute
	}
	if s.GitTimeout == 0 {
		s.GitTimeout = 5 * time.Minute
	}
	if s.Publisher == nil {
		s.Publisher = NewEnginePublisher(s.GHTimeout)
	}
	if s.Poll <= 0 {
		s.Poll = 15 * time.Second
	}
	if s.Wait <= 0 {
		s.Wait = 40 * time.Minute
	}
	if s.Grace <= 0 {
		s.Grace = 90 * time.Second
	}
	if s.Absent <= 0 {
		s.Absent = 10 * time.Minute
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.sleep == nil {
		s.sleep = sleepCtx
	}
	if s.pull == nil {
		s.pull = s.readPR
	}
	if s.merge == nil {
		s.merge = func(ctx context.Context, pr int, strategy, head string) error {
			return reviewer.MergeAt(ctx, pr, strategy, head, s.GHTimeout)
		}
	}
	if s.state == nil {
		s.state = func(ctx context.Context, pr int) string { return reviewer.MergeState(ctx, pr, s.GHTimeout) }
	}
	if s.ticket == nil {
		s.ticket = (&ResolveStep{GHTimeout: s.GHTimeout}).issueText
	}
	if s.test == nil {
		s.test = reviewer.RunTests
	}
	if s.gate == nil {
		s.gate = reviewer.HumanGate
	}
	if s.landed == nil {
		s.landed = reviewer.AfterMerge
	}
	if s.expects == nil {
		s.expects = landing.ExpectsChecks
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Merge merges the pull request of a lane at the head of the merge queue
// (Queued), and says where the lane goes: to Merged, or where what stopped
// the merge can be dealt with. The worktree is never left in a rebase. An
// error is the caller's mistake, the engine stopping (ctx), or GitHub or git
// that could not be asked at all; the lane then stays in the queue, and the
// run ends with the error.
func (s *MergeStep) Merge(ctx context.Context, lane Lane) (Transition, error) {
	if lane.State != Queued {
		return Transition{}, fmt.Errorf("lane #%d is %s: only a lane in the merge queue is merged", lane.Issue, show(lane.State))
	}
	if lane.PR == 0 {
		return Transition{}, fmt.Errorf("lane #%d has no pull request to merge", lane.Issue)
	}
	s.defaults()
	m := &merging{laneTree: newLaneTree(s.Root, lane.Issue, s.GitTimeout), step: s, lane: lane}
	m.approval, _ = reviewer.LoadApproval(s.Root, lane.PR)
	t, err := m.run(ctx)
	if m.rebasing() {
		_, _ = m.git(context.WithoutCancel(ctx), "rebase", "--abort")
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		return Transition{}, err
	}
	return t, nil
}

// merging is one run of the step for a lane.
type merging struct {
	laneTree
	step     *MergeStep
	lane     Lane
	approval reviewer.Approval
	// base is the branch the pull request goes into; head the commit the
	// queue verified and merges.
	base, head string
}

// again is a merge that starts over: the branch or the integration branch
// moved under it.
type again struct{ why string }

func (a again) Error() string { return a.why }

func (m *merging) run(ctx context.Context) (Transition, error) {
	for try := 1; ; try++ {
		t, err := m.once(ctx)
		var moved again
		if !errors.As(err, &moved) {
			return t, err
		}
		if try >= maxMergeTries {
			return m.needsYou(WhyMergeBlocked, fmt.Sprintf("PR #%d was not merged: it was tried %d times, and each time something moved under it; the last time %s", m.lane.PR, try, moved.why)), nil
		}
		m.say("%s: the merge starts over", moved.why)
	}
}

// once is one try: a transition, an error, or again.
func (m *merging) once(ctx context.Context) (Transition, error) {
	pr, err := m.step.pull(ctx, m.lane.PR)
	if err != nil {
		return Transition{}, fmt.Errorf("PR #%d could not be read: %w", m.lane.PR, err)
	}
	if t, over := m.over(ctx, pr); over {
		return t, nil
	}
	m.base = pr.Base
	if m.base == "" {
		m.base = m.integrationBranch()
	}

	if err := m.ready(ctx); err != nil {
		return Transition{}, fmt.Errorf("the worktree of #%d could not be made ready: %w", m.lane.Issue, err)
	}
	if _, err := m.git(ctx, "fetch", "origin", "--prune"); err != nil {
		return Transition{}, fmt.Errorf("git fetch: %w", err)
	}
	// The pull request is what GitHub has: what a person pushed to it is
	// taken in, and a rebase of the queue's that was not pushed is dropped.
	lease, _ := m.git(ctx, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+m.branch)
	if lease == "" {
		return m.needsYou(WhyNoPR, fmt.Sprintf("the branch %s of PR #%d is not on GitHub", m.branch, m.lane.PR)), nil
	}
	if _, err := m.git(ctx, "reset", "--hard", lease); err != nil {
		return Transition{}, fmt.Errorf("the worktree of #%d could not take the branch as GitHub has it: %w", m.lane.Issue, err)
	}

	// Onto the integration branch as it is now.
	tip, err := m.git(ctx, "rev-parse", "origin/"+m.base)
	if err != nil {
		return Transition{}, fmt.Errorf("origin/%s: %w", m.base, err)
	}
	if from, err := m.git(ctx, "merge-base", "HEAD", "origin/"+m.base); err != nil || from != tip {
		if _, err := m.git(ctx, "rebase", "origin/"+m.base); err != nil {
			if ctx.Err() != nil {
				return Transition{}, ctx.Err()
			}
			conflicted := m.unmerged(ctx)
			if len(conflicted) == 0 {
				return m.needsYou(WhyNeedsHumanReview, fmt.Sprintf("the rebase of PR #%d onto origin/%s failed: %v", m.lane.PR, m.base, err)), nil
			}
			return m.conflict("its files " + strings.Join(conflicted, ", ") + " conflict with origin/" + m.base), nil
		}
		m.say("rebased onto origin/%s (%s)", m.base, short(tip))
	}
	if m.head, err = m.git(ctx, "rev-parse", "HEAD"); err != nil {
		return Transition{}, err
	}

	// A commit the review did not approve is checked again.
	if m.head != m.approved() {
		if t, stop, err := m.verify(ctx); stop || err != nil {
			return t, err
		}
	}

	if m.head != lease {
		args := []string{"push", "--force-with-lease=" + m.branch + ":" + lease, "origin", "HEAD:refs/heads/" + m.branch}
		if _, err := m.git(ctx, args...); err != nil {
			if ctx.Err() != nil {
				return Transition{}, ctx.Err()
			}
			return Transition{}, again{fmt.Sprintf("the push of %s was refused (%v)", short(m.head), err)}
		}
		m.say("pushed %s to PR #%d", short(m.head), m.lane.PR)
	}

	if t, stop, err := m.checks(ctx); stop || err != nil {
		return t, err
	}

	// A maintainer may have written since the review.
	if held := m.step.gate(ctx, m.merged()); held != nil {
		return m.held(held), nil
	}

	if err := m.step.merge(ctx, m.lane.PR, reviewer.GetReviewSettings(m.root).MergeStrategy, m.head); err != nil {
		if ctx.Err() != nil {
			return Transition{}, ctx.Err()
		}
		return m.refused(ctx, err)
	}
	m.say("merged PR #%d (%s) into %s", m.lane.PR, short(m.head), m.base)
	return m.done(ctx, fmt.Sprintf("merged %s into %s", short(m.head), m.base)), nil
}

// over says whether the pull request was merged or closed already, and the
// lane's move then.
func (m *merging) over(ctx context.Context, pr PullRequest) (Transition, bool) {
	switch pr.State {
	case "MERGED":
		m.head = pr.Head
		return m.done(ctx, "its pull request was merged already"), true
	case "CLOSED":
		return m.needsYou(WhyNoPR, fmt.Sprintf("PR #%d was closed without being merged", m.lane.PR)), true
	}
	return Transition{}, false
}

// approved is the commit the review approved.
func (m *merging) approved() string {
	if m.lane.Approved != "" {
		return m.lane.Approved
	}
	return m.approval.Head
}

// verify checks the commit again: the diff policy on its change, then the
// tests the review ran. stop is true with the lane's move when it did not
// pass.
func (m *merging) verify(ctx context.Context) (t Transition, stop bool, err error) {
	text, err := m.step.ticket(ctx, m.lane.Issue)
	if err != nil {
		return Transition{}, false, fmt.Errorf("the ticket of #%d could not be read: %w", m.lane.Issue, err)
	}
	meta, err := worker.ReadTicketMetadata(text)
	if err != nil {
		return m.needsYou(WhyInvalidTicket, "the ticket cannot be used: "+err.Error()), true, nil
	}
	if err := worker.CheckCommits(ctx, m.wt, "origin/"+m.base+"...HEAD", meta); err != nil {
		var refused *worker.PolicyError
		if errors.As(err, &refused) {
			return m.needsYou(WhyDiffPolicy, fmt.Sprintf("PR #%d, rebased onto origin/%s: %s", m.lane.PR, m.base, refused.Error())), true, nil
		}
		return Transition{}, false, fmt.Errorf("the change of PR #%d could not be read: %w", m.lane.PR, err)
	}
	command, language := reviewer.RepoTestCommand(m.root, m.wt)
	if command == "" {
		return Transition{}, false, nil
	}
	m.say("testing %s: %s", short(m.head), command)
	run := m.step.test(ctx, m.wt, command, language, reviewer.GetReviewSettings(m.root).TestTimeout)
	if ctx.Err() != nil {
		return Transition{}, false, ctx.Err()
	}
	if run.Err != nil {
		detail := fmt.Sprintf("`%s` 沒過（%v）：\n\n```\n%s\n```", run.Command, run.Err, tail(run.Output, maxCheckOutput))
		t, err := m.checkFailed(ctx, fmt.Sprintf("the tests failed on %s, rebased onto origin/%s", short(m.head), m.base), "測試", detail)
		return t, true, err
	}
	return Transition{}, false, nil
}

// checks waits for the checks of the commit on GitHub. stop is true with the
// lane's move when the merge does not go on.
func (m *merging) checks(ctx context.Context) (t Transition, stop bool, err error) {
	s := m.step
	start := s.now()
	expect := s.expects(m.wt, m.base)
	said := ""
	for {
		pr, err := s.pull(ctx, m.lane.PR)
		if err != nil {
			return Transition{}, false, fmt.Errorf("PR #%d could not be read: %w", m.lane.PR, err)
		}
		if t, over := m.over(ctx, pr); over {
			return t, true, nil
		}
		waited := s.now().Sub(start)
		if pr.Head != m.head {
			if waited >= s.Absent {
				return Transition{}, false, again{fmt.Sprintf("PR #%d shows %s at its head, not %s", m.lane.PR, short(pr.Head), short(m.head))}
			}
		} else {
			var failed, pending []string
			for _, c := range pr.Checks {
				switch c.Bucket {
				case "fail", "cancel":
					failed = append(failed, c.Name)
				case "pending":
					pending = append(pending, c.Name)
				}
			}
			switch {
			case len(failed) > 0:
				detail := "沒過的檢查：" + strings.Join(failed, "、")
				t, err := m.checkFailed(ctx, fmt.Sprintf("the checks %s failed on %s", strings.Join(failed, ", "), short(m.head)), "CI", detail)
				return t, true, err
			case len(pr.Checks) == 0 && !expect && waited >= s.Grace:
				m.say("PR #%d has no checks: the project has no CI for pull requests", m.lane.PR)
				return Transition{}, false, nil
			case len(pr.Checks) == 0 && expect && waited >= s.Absent:
				return m.needsYou(WhyChecksStuck, fmt.Sprintf("no check of PR #%d started in %v, though a workflow of the project runs on pull requests into %s", m.lane.PR, s.Absent, m.base)), true, nil
			case len(pr.Checks) > 0 && len(pending) == 0:
				if said != "" {
					m.say("the checks of PR #%d passed", m.lane.PR)
				}
				return Transition{}, false, nil
			case waited >= s.Wait:
				return m.needsYou(WhyChecksStuck, fmt.Sprintf("the checks %s of PR #%d had not ended after %v", strings.Join(pending, ", "), m.lane.PR, s.Wait)), true, nil
			}
			if now := fmt.Sprintf("waiting for the checks of PR #%d: %d of %d pending", m.lane.PR, len(pending), len(pr.Checks)); len(pr.Checks) > 0 && now != said {
				m.say("%s", now)
				said = now
			}
		}
		if err := s.sleep(ctx, s.Poll); err != nil {
			return Transition{}, false, err
		}
	}
}

// refused is where a merge GitHub refused goes: resolved when it conflicts,
// tried again when the branch or its base moved, a person otherwise.
func (m *merging) refused(ctx context.Context, mergeErr error) (Transition, error) {
	if pr, err := m.step.pull(ctx, m.lane.PR); err == nil {
		if t, over := m.over(ctx, pr); over {
			return t, nil
		}
		if pr.Head != m.head {
			return Transition{}, again{fmt.Sprintf("PR #%d's head moved to %s", m.lane.PR, short(pr.Head))}
		}
	}
	switch state := m.step.state(ctx, m.lane.PR); state {
	case "DIRTY":
		return m.conflict("GitHub says it conflicts with " + m.base), nil
	case "BEHIND":
		return Transition{}, again{fmt.Sprintf("%s moved on", m.base)}
	default:
		return m.needsYou(WhyMergeBlocked, fmt.Sprintf("GitHub refused the merge of PR #%d (%s): %v", m.lane.PR, state, mergeErr)), nil
	}
}

// done does what follows the merge, and moves the lane to Merged.
func (m *merging) done(ctx context.Context, note string) Transition {
	m.step.landed(ctx, m.merged())
	return m.move(Merged, CauseMerged, note)
}

// merged is the merge as the review's learning sees it.
func (m *merging) merged() reviewer.MergedOptions {
	o := reviewer.MergedOptions{
		StateRoot: m.root, PRNumber: m.lane.PR, IssueNumber: m.lane.Issue,
		Score: m.approval.Score, Head: m.approved(),
		Publisher: m.step.Publisher, GHTimeout: m.step.GHTimeout,
	}
	if cfg, err := analyzer.LoadConfig(config.Path(m.root)); err == nil {
		o.HookRunner = hooks.NewHookRunner(cfg.Hooks, m.root, m.step.Log)
	}
	return o
}

// conflict sends the pull request to be resolved, as often as it may be.
func (m *merging) conflict(why string) Transition {
	if m.lane.Attempts.Conflicts >= m.step.MaxConflicts {
		return m.needsYou(WhyConflictRetries, fmt.Sprintf("PR #%d still conflicts after %d resolutions: %s", m.lane.PR, m.lane.Attempts.Conflicts, why))
	}
	t := m.move(Resolving, CauseConflicting, fmt.Sprintf("PR #%d: %s", m.lane.PR, why))
	t.Resolve = ResolveConflict
	return t
}

// checkFailed sends the change back to its implementer with what failed,
// as often as it may be.
func (m *merging) checkFailed(ctx context.Context, note, what, detail string) (Transition, error) {
	if m.lane.Attempts.Checks >= m.step.MaxChecks {
		return m.needsYou(WhyCheckRetries, fmt.Sprintf("%s; the queue's checks failed on its change %d times before", note, m.lane.Attempts.Checks)), nil
	}
	body := fmt.Sprintf("## AWK Review：合併前的%s沒過\n\nPR #%d 審查通過了，但在合併佇列裡接上 `%s` 最新的版本（%s）之後，%s沒過，所以沒有合併。\n\n%s\n\n---\n**Worker 請修好之後重新提交。**\nPR: #%d",
		what, m.lane.PR, m.base, short(m.head), what, detail, m.lane.PR)
	if err := m.step.Publisher.CommentIssue(ctx, m.lane.Issue, body); err != nil {
		if errors.Is(err, guard.ErrBlocked) {
			return m.needsYou(WhyGuardOutput, note+"; the comment that says so was kept from GitHub: "+err.Error()), nil
		}
		m.say("the comment that sends it back could not be posted: %v", err)
	}
	return m.move(Rework, CauseCheckFailed, note), nil
}

// held is where the maintainers' gate sends the lane.
func (m *merging) held(res *reviewer.SubmitReviewResult) Transition {
	if res.Next == reviewer.NextRework {
		note := res.Reason
		if note == "" {
			note = "a maintainer's review is open on the pull request"
		}
		return m.move(Rework, CauseChangesWanted, note)
	}
	note := res.Reason
	if note == "" {
		note = fmt.Sprintf("the maintainers' gate held PR #%d (%s)", m.lane.PR, res.Result)
	}
	return m.needsYou(WhyMaintainer, note)
}

func (m *merging) move(to State, cause Cause, note string) Transition {
	return Transition{Issue: m.lane.Issue, From: Queued, To: to, Cause: cause, Note: note, At: m.step.now().UTC()}
}

func (m *merging) needsYou(why, note string) Transition {
	t := m.move(NeedsYou, CauseEscalated, note)
	t.Why = why
	return t
}

func (m *merging) say(format string, args ...any) {
	fmt.Fprintf(m.step.Log, "[#%d] "+format+"\n", append([]any{m.lane.Issue}, args...)...)
}

func (m *merging) config() *config.Config {
	cfg, err := config.Load(m.root)
	if err != nil {
		cfg = config.Default()
	}
	return cfg
}

func (m *merging) integrationBranch() string { return m.config().BaseBranch() }

// short is a commit's short name.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// tail is the end of what a command printed, at most max characters.
func tail(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	cut := text[len(text)-max:]
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i < len(cut)-1 {
		cut = cut[i+1:]
	}
	return "…\n" + cut
}

// readPR reads a pull request, and the checks of its head, from GitHub.
func (s *MergeStep) readPR(ctx context.Context, pr int) (PullRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, s.GHTimeout)
	defer cancel()
	out, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "view", strconv.Itoa(pr), "--json", "state,headRefOid,baseRefName,statusCheckRollup")
	if err != nil {
		return PullRequest{}, err
	}
	return parsePR(out)
}

// parsePR reads what `gh pr view --json state,headRefOid,baseRefName,
// statusCheckRollup` says. The rollup has a check run (a GitHub Actions job)
// or a commit status for each check; each is put in gh's buckets.
func parsePR(data []byte) (PullRequest, error) {
	var raw struct {
		State       string `json:"state"`
		HeadRefOid  string `json:"headRefOid"`
		BaseRefName string `json:"baseRefName"`
		Rollup      []struct {
			Typename   string `json:"__typename"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Context    string `json:"context"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return PullRequest{}, fmt.Errorf("gh pr view said what cannot be read: %w", err)
	}
	pr := PullRequest{State: raw.State, Head: raw.HeadRefOid, Base: raw.BaseRefName}
	for _, c := range raw.Rollup {
		check := Check{Name: c.Name}
		if c.Typename == "StatusContext" || (c.Context != "" && c.Name == "") {
			check.Name = c.Context
			check.Bucket = statusBucket(c.State)
		} else {
			check.Bucket = runBucket(c.Status, c.Conclusion)
		}
		pr.Checks = append(pr.Checks, check)
	}
	return pr, nil
}

// runBucket is the bucket of a check run, as gh puts it.
func runBucket(status, conclusion string) string {
	if status != "" && status != "COMPLETED" {
		return "pending"
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL":
		return "pass"
	case "SKIPPED":
		return "skipping"
	case "CANCELLED":
		return "cancel"
	case "":
		return "pending"
	}
	// FAILURE, TIMED_OUT, ACTION_REQUIRED, STARTUP_FAILURE, STALE.
	return "fail"
}

// statusBucket is the bucket of a commit status, as gh puts it.
func statusBucket(state string) string {
	switch state {
	case "SUCCESS":
		return "pass"
	case "PENDING", "EXPECTED", "":
		return "pending"
	}
	return "fail"
}

// queue is the merge queue: the lanes in it, the first to be merged first.
// A lane's place is when it came into the queue; at the same time, the
// lower number first.
func queue(lanes []Lane) []Lane {
	var q []Lane
	for _, l := range lanes {
		if l.State == Queued {
			q = append(q, l)
		}
	}
	slices.SortStableFunc(q, func(a, b Lane) int {
		if c := a.Since.Compare(b.Since); c != 0 {
			return c
		}
		return a.Issue - b.Issue
	})
	return q
}
