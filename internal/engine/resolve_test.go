package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/agent"
)

// conflictFixture is a project with a remote of its own (a bare repository
// on disk), a pull request's branch for ticket #12 in its worktree, and a
// base that has moved on since the branch left it.
type conflictFixture struct {
	t                *testing.T
	origin, root, wt string
	step             *ResolveStep
	agents           *fakeAgents
	lane             Lane
}

const (
	scoreBase     = "one\ntwo\nthree\n"
	scorePR       = "one\ntwo (the pull request)\nthree\n"
	scoreMoved    = "one\ntwo (the base)\nthree\n"
	scoreResolved = "one\ntwo (the base) (the pull request)\nthree\n"
)

// newConflictFixture makes the project. With conflict, the base changed the
// line the pull request changed; without, it changed another file.
func newConflictFixture(t *testing.T, conflict bool) *conflictFixture {
	t.Helper()
	moved := map[string]string{"notes.txt": "the base moved on\n"}
	if conflict {
		moved = map[string]string{"score.txt": scoreMoved}
	}
	return newRepoFixture(t,
		map[string]string{"score.txt": scoreBase, "other.txt": "untouched\n"},
		map[string]string{"score.txt": scorePR}, moved)
}

// newRepoFixture makes a project whose files are start, a pull request's
// branch that writes pr over them, and a base that has since written moved.
func newRepoFixture(t *testing.T, start, pr, moved map[string]string) *conflictFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the PATH")
	}
	f := &conflictFixture{t: t, origin: filepath.Join(t.TempDir(), "origin.git"), root: t.TempDir()}
	// Without init -b, which git has only since 2.28.
	gitIn(t, filepath.Dir(f.origin), "init", "-q", "--bare", f.origin)
	gitIn(t, f.origin, "symbolic-ref", "HEAD", "refs/heads/develop")
	gitIn(t, f.root, "init", "-q")
	gitIn(t, f.root, "symbolic-ref", "HEAD", "refs/heads/develop")
	for _, kv := range [][2]string{{"user.email", "t@example.invalid"}, {"user.name", "t"}, {"core.autocrlf", "false"}} {
		gitIn(t, f.root, "config", kv[0], kv[1])
	}
	gitIn(t, f.root, "remote", "add", "origin", f.origin)
	writeFile(t, filepath.Join(f.root, ".gitignore"), ".worktrees/\n.ai/state/\n")
	writeFile(t, filepath.Join(f.root, ".ai", "config.yaml"), reviewConfig)
	writeFiles := func(files map[string]string) {
		for name, text := range files {
			writeFile(t, filepath.Join(f.root, filepath.FromSlash(name)), text)
		}
		gitIn(t, f.root, "add", "-A")
	}
	writeFiles(start)
	gitIn(t, f.root, "commit", "-qm", "[chore] start")
	gitIn(t, f.root, "push", "-q", "origin", "develop")

	gitIn(t, f.root, "checkout", "-q", "-b", "feat/ai-issue-12")
	writeFiles(pr)
	gitIn(t, f.root, "commit", "-qm", "[feat] name the pull request's line")
	gitIn(t, f.root, "push", "-q", "origin", "feat/ai-issue-12")

	gitIn(t, f.root, "checkout", "-q", "develop")
	writeFiles(moved)
	gitIn(t, f.root, "commit", "-qm", "[feat] the base moves on")
	gitIn(t, f.root, "push", "-q", "origin", "develop")

	f.wt = filepath.Join(f.root, ".worktrees", "issue-12")
	gitIn(t, f.root, "worktree", "add", "-q", f.wt, "feat/ai-issue-12")

	f.agents = &fakeAgents{}
	f.lane = Lane{Issue: 12, State: Implementing, PR: 30, Resolve: ResolveConflict, Seq: 50}
	f.step = &ResolveStep{
		Root: f.root, Agents: f.agents,
		Profile: agent.Profile{Role: agent.Resolver, Backend: agent.ClaudeCode, Tools: agent.ShellTools, Minutes: 5, SchemaRetries: 1},
		now:     func() time.Time { return time.Date(2026, 11, 23, 11, 0, 0, 0, time.UTC) },
		base:    func(context.Context, int) (string, error) { return "develop", nil },
		ticket: func(context.Context, int) (string, error) {
			return "# Name the line\n\nThe second line says whose it is.", nil
		},
	}
	return f
}

// resolver answers as a resolver that does what do does in the worktree.
func (f *conflictFixture) resolver(result string, do func(wt string)) {
	f.agents.answer = func(env agent.Envelope) agent.Result {
		if do != nil {
			do(env.Dir)
		}
		out, _ := json.Marshal(map[string]any{"result": result, "files": []string{"score.txt"}, "reason": "kept both sides", "suspicious": []string{}})
		return agent.Result{Status: agent.StatusOK, Output: out}
	}
}

func (f *conflictFixture) resolve() Transition {
	f.t.Helper()
	tr, err := f.step.Resolve(context.Background(), f.lane)
	if err != nil {
		f.t.Fatalf("Resolve: %v", err)
	}
	next, err := Apply(&f.lane, tr)
	if err != nil {
		f.t.Fatalf("the lane cannot move as the resolution says: %v", err)
	}
	f.lane = *next
	// Never left in a rebase, and on its branch.
	if r := (&laneTree{wt: f.wt}); r.rebasing() {
		f.t.Error("the worktree was left in a rebase")
	}
	if on := gitIn(f.t, f.wt, "rev-parse", "--abbrev-ref", "HEAD"); strings.TrimSpace(on) != "feat/ai-issue-12" {
		f.t.Errorf("the worktree is on %s", on)
	}
	return tr
}

// remote is a file as the pull request's branch on the remote has it.
func (f *conflictFixture) remote(file string) string {
	return gitIn(f.t, f.origin, "show", "feat/ai-issue-12:"+file)
}

func (f *conflictFixture) remoteLog() string {
	return gitIn(f.t, f.origin, "log", "--format=%s", "feat/ai-issue-12")
}

func write(path, text string) { _ = os.WriteFile(path, []byte(text), 0o644) }

// A branch that is only behind is rebased and pushed by the engine alone.
func TestResolve_ABranchBehind(t *testing.T) {
	f := newConflictFixture(t, false)
	f.lane.Resolve = ResolveRebase
	tr := f.resolve()
	if tr.To != Reviewing || tr.Cause != CauseResolved || len(f.agents.asked) != 0 {
		t.Fatalf("%+v; asked %d", tr, len(f.agents.asked))
	}
	if log := f.remoteLog(); !strings.Contains(log, "[feat] the base moves on") || !strings.HasPrefix(log, "[feat] name the pull request's line") {
		t.Errorf("the remote branch is not the pull request on the base:\n%s", log)
	}
}

// A conflict is resolved by the resolver, checked, continued and pushed by
// the engine.
func TestResolve_AConflictResolved(t *testing.T) {
	f := newConflictFixture(t, true)
	f.resolver(ResolverResolved, func(wt string) { write(filepath.Join(wt, "score.txt"), scoreResolved) })
	tr := f.resolve()
	if tr.To != Reviewing || tr.Cause != CauseResolved || !strings.Contains(tr.Note, "score.txt") {
		t.Fatalf("%+v", tr)
	}
	if got := f.remote("score.txt"); got != scoreResolved {
		t.Errorf("the remote has:\n%s", got)
	}
	if len(f.agents.asked) != 1 {
		t.Fatalf("asked %d times", len(f.agents.asked))
	}
	env := f.agents.asked[0]
	if env.Dir != f.wt || env.Profile.Role != agent.Resolver || strings.Join(env.Protected, ",") != "develop,main" {
		t.Errorf("envelope: %+v", env)
	}
	if err := agent.Validate(env.Schema, json.RawMessage(`{"result":"RESOLVED","files":["score.txt"],"reason":"both","suspicious":[]}`)); err != nil {
		t.Errorf("the schema: %v", err)
	}
	prompt := env.Prompt
	for _, want := range []string{"rebasing the pull request's branch onto `origin/develop`", "The second line says whose it is."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	data := prompt[strings.Index(prompt, "<data"):]
	for _, want := range []string{"<<<<<<< ", "two (the base)", "two (the pull request)", "[feat] the base moves on", "[feat] name the pull request's line"} {
		if !strings.Contains(data, want) {
			t.Errorf("%q is not in a data block", want)
		}
	}
	if _, err := os.Stat(filepath.Join(agent.RunDir(f.root, 12, 50), "round-1", "envelope.json")); err != nil {
		t.Errorf("what the resolver was given is not kept: %v", err)
	}
}

// What a resolver does wrong, or cannot do, does not reach the remote.
func TestResolve_WhatIsNotPushed(t *testing.T) {
	for _, c := range []struct {
		name   string
		result string
		do     func(wt string)
		status agent.Status
		to     State
		why    string
		note   string
	}{
		{"markers left", ResolverResolved, func(wt string) {
			text, _ := os.ReadFile(filepath.Join(wt, "score.txt"))
			write(filepath.Join(wt, "score.txt"), strings.Replace(string(text), "two (the base)", "two (the base, kept)", 1))
		}, agent.StatusOK, Resolving, "", "conflict markers are left in score.txt"},
		{"another file edited", ResolverResolved, func(wt string) {
			write(filepath.Join(wt, "score.txt"), scoreResolved)
			write(filepath.Join(wt, "other.txt"), "touched\n")
		}, agent.StatusOK, NeedsYou, WhyGuardOutput, "other.txt"},
		{"another file staged", ResolverResolved, func(wt string) {
			write(filepath.Join(wt, "score.txt"), scoreResolved)
			write(filepath.Join(wt, "other.txt"), "touched\n")
			cmd := exec.Command("git", "add", "other.txt")
			cmd.Dir = wt
			_ = cmd.Run()
		}, agent.StatusOK, NeedsYou, WhyGuardOutput, "other.txt"},
		{"a commit of its own", ResolverResolved, func(wt string) {
			write(filepath.Join(wt, "score.txt"), scoreResolved)
			cmd := exec.Command("git", "-c", "core.editor=true", "commit", "-qam", "resolved")
			cmd.Dir = wt
			_ = cmd.Run()
		}, agent.StatusOK, Resolving, "", "moved the branch"},
		{"too complex", ResolverTooComplex, nil, agent.StatusOK, NeedsYou, WhyNeedsHumanReview, "needs a person: kept both sides"},
		{"failed", ResolverFailed, nil, agent.StatusOK, Resolving, "", "did not resolve score.txt"},
		{"the resolver timed out", "", nil, agent.StatusTimeout, Resolving, "", "timeout"},
		{"the backend refused", "", nil, agent.StatusRefused, NeedsYou, WhyAgentRefused, "was not run"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newConflictFixture(t, true)
			if c.status == agent.StatusOK {
				f.resolver(c.result, c.do)
			} else {
				f.agents.replies = []agent.Result{{Status: c.status, Error: "it went wrong"}}
			}
			before := f.remoteLog()
			tr := f.resolve()
			if tr.To != c.to || tr.Why != c.why || !strings.Contains(tr.Note, c.note) {
				t.Fatalf("%+v", tr)
			}
			if f.remoteLog() != before || f.remote("score.txt") != scorePR {
				t.Errorf("the remote branch changed:\n%s", f.remoteLog())
			}
			if c.why == WhyGuardOutput && len(eventsWith(t, f.root, "resolver-scope")) == 0 {
				t.Error("the block was not recorded")
			}
		})
	}
}

// What the resolver's own checks leave in the worktree is neither pushed
// nor left for the review.
func TestResolve_WhatItsChecksLeave(t *testing.T) {
	f := newConflictFixture(t, true)
	f.resolver(ResolverResolved, func(wt string) {
		write(filepath.Join(wt, "score.txt"), scoreResolved)
		write(filepath.Join(wt, "rally.exe"), "a build")
	})
	if tr := f.resolve(); tr.To != Reviewing {
		t.Fatalf("%+v", tr)
	}
	if _, err := os.Stat(filepath.Join(f.wt, "rally.exe")); err == nil {
		t.Error("the build was left in the worktree")
	}
	if strings.Contains(gitIn(t, f.origin, "ls-tree", "-r", "--name-only", "feat/ai-issue-12"), "rally.exe") {
		t.Error("the build was pushed")
	}
}

// A pull request whose commits conflict one after another is resolved a
// commit at a time.
func TestResolve_CommitAfterCommit(t *testing.T) {
	f := newConflictFixture(t, true)
	write(filepath.Join(f.wt, "score.txt"), "one\ntwo (the pull request, again)\nthree\n")
	gitIn(t, f.wt, "commit", "-qam", "[feat] name it again")
	gitIn(t, f.wt, "push", "-q", "origin", "feat/ai-issue-12")

	rounds := 0
	f.agents.answer = func(env agent.Envelope) agent.Result {
		rounds++
		text := scoreResolved
		if rounds == 2 {
			text = "one\ntwo (the base) (the pull request, again)\nthree\n"
		}
		write(filepath.Join(env.Dir, "score.txt"), text)
		return agent.Result{Status: agent.StatusOK, Output: json.RawMessage(`{"result":"RESOLVED","files":["score.txt"],"reason":"both","suspicious":[]}`)}
	}
	if tr := f.resolve(); tr.To != Reviewing || rounds != 2 {
		t.Fatalf("%+v after %d rounds", tr, rounds)
	}
	if got := f.remote("score.txt"); got != "one\ntwo (the base) (the pull request, again)\nthree\n" {
		t.Errorf("the remote has:\n%s", got)
	}
	if !strings.HasSuffix(f.agents.asked[1].RunDir, filepath.Join("round-2", "resolver")) {
		t.Errorf("the second round's files: %s", f.agents.asked[1].RunDir)
	}
}

// What a person pushed to the pull request is kept; a branch that went two
// ways is for a person.
func TestResolve_APersonsPush(t *testing.T) {
	f := newConflictFixture(t, false)
	other := t.TempDir()
	gitIn(t, other, "clone", "-q", "-b", "feat/ai-issue-12", f.origin, ".")
	write(filepath.Join(other, "fix.txt"), "a person's fix\n")
	gitIn(t, other, "add", "fix.txt")
	gitIn(t, other, "commit", "-qm", "[fix] a person's fix")
	gitIn(t, other, "push", "-q", "origin", "feat/ai-issue-12")

	if tr := f.resolve(); tr.To != Reviewing {
		t.Fatalf("%+v", tr)
	}
	if log := f.remoteLog(); !strings.HasPrefix(log, "[fix] a person's fix") || !strings.Contains(log, "[feat] the base moves on") {
		t.Errorf("the person's fix was not kept:\n%s", log)
	}

	// The worktree has a commit the remote does not, and the remote one the
	// worktree does not.
	f.lane = Lane{Issue: 12, State: Implementing, PR: 30, Resolve: ResolveRebase, Seq: 51}
	gitIn(t, other, "pull", "-q", "--rebase=false", "origin", "feat/ai-issue-12")
	write(filepath.Join(other, "fix.txt"), "a second fix\n")
	gitIn(t, other, "commit", "-qam", "[fix] a second fix")
	gitIn(t, other, "push", "-q", "-f", "origin", "HEAD:feat/ai-issue-12")
	write(filepath.Join(f.wt, "local.txt"), "only here\n")
	gitIn(t, f.wt, "add", "local.txt")
	gitIn(t, f.wt, "commit", "-qm", "[feat] only here")
	if tr := f.resolve(); tr.To != NeedsYou || !strings.Contains(tr.Note, "different ways") {
		t.Fatalf("%+v", tr)
	}
	if strings.Contains(f.remoteLog(), "only here") {
		t.Error("the worktree's commit was pushed over the person's")
	}
}

func TestResolve_NotALaneToResolve(t *testing.T) {
	f := newConflictFixture(t, false)
	for _, lane := range []Lane{
		{Issue: 12, State: Resolving, PR: 30, Resolve: ResolveConflict},
		{Issue: 12, State: Implementing, PR: 30},
		{Issue: 12, State: Implementing, Resolve: ResolveConflict},
	} {
		if _, err := f.step.Resolve(context.Background(), lane); err == nil {
			t.Errorf("resolved %+v", lane)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if tr, err := f.step.Resolve(ctx, f.lane); err == nil {
		t.Errorf("a stopped engine moved the lane: %+v", tr)
	}
}

func TestResolve_Companions(t *testing.T) {
	r := &resolution{}
	before := snapshot{index: map[string]string{"go.sum": "a", "web/package-lock.json": "b", "main.go": "c"}}
	after := snapshot{
		index:     map[string]string{"go.sum": "a2", "web/package-lock.json": "b", "main.go": "c"},
		changed:   []string{"web/package-lock.json", "main.go"},
		untracked: []string{"yarn.lock", "web/yarn.lock", "rally.exe"},
	}
	outside, allowed := r.outside(before, after, []string{"go.mod", "web/package.json"})
	if strings.Join(outside, ",") != "main.go" || strings.Join(allowed, ",") != "go.sum,web/package-lock.json,web/yarn.lock" {
		t.Errorf("outside %v, allowed %v", outside, allowed)
	}
}

func TestHasMarker(t *testing.T) {
	for text, want := range map[string]bool{
		"a\n<<<<<<< HEAD\nb\n=======\nc\n>>>>>>> abc (x)\n": true,
		"a\n>>>>>>>\n":                     true,
		"|||||||  base\n":                  true,
		"Title\n=======\n\ntext\n":         false,
		"<<<<<<<< eight\n":                 false,
		"x := 1 // <<<<<<< not at start\n": false,
	} {
		if hasMarker([]byte(text)) != want {
			t.Errorf("hasMarker(%q) = %v", text, !want)
		}
	}
}
