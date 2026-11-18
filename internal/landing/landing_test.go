package landing

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// project is a clone of a bare origin with an integration branch, develop,
// as a project of AWK is: the documents of a meeting are in its working tree.
type project struct {
	t      *testing.T
	origin string
	root   string
	gh     *fakeGitHub
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func configure(t *testing.T, dir string) {
	for _, kv := range [][2]string{{"user.name", "test"}, {"user.email", "test@example.invalid"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}} {
		git(t, dir, "config", kv[0], kv[1])
	}
}

func write(t *testing.T, dir, path, text string) {
	t.Helper()
	file := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newProject(t *testing.T) *project {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	root := filepath.Join(base, "project")
	git(t, base, "init", "--quiet", "--bare", "--initial-branch=develop", origin)
	git(t, base, "clone", "--quiet", origin, root)
	configure(t, root)
	git(t, root, "checkout", "--quiet", "-B", "develop")
	write(t, root, ".gitignore", ".ai/state/\n")
	write(t, root, "docs/decisions.md", "# Decisions\n")
	write(t, root, "docs/old.md", "to be removed\n")
	write(t, root, "src/main.go", "package main\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "--quiet", "-m", "start")
	git(t, root, "push", "--quiet", "-u", "origin", "develop")
	p := &project{t: t, origin: origin, root: root}
	p.gh = &fakeGitHub{p: p}
	return p
}

// meeting writes what a meeting leaves: a new document, a changed one, one
// removed.
func (p *project) meeting() {
	write(p.t, p.root, "docs/plans/p1/tickets.yaml", "tickets:\n  - key: t1\n    issue: 12\n")
	write(p.t, p.root, "docs/decisions.md", "# Decisions\n\n### D1\n")
	if err := os.Remove(filepath.Join(p.root, "docs", "old.md")); err != nil {
		p.t.Fatal(err)
	}
}

func (p *project) pending() []Change {
	p.t.Helper()
	changes, err := Pending(context.Background(), nil, p.root, []string{"docs"})
	if err != nil {
		p.t.Fatal(err)
	}
	return changes
}

// onOrigin is a file as the integration branch on origin has it.
func (p *project) onOrigin(path string) (string, bool) {
	cmd := exec.Command("git", "--git-dir", p.origin, "show", "develop:"+path)
	out, err := cmd.Output()
	return string(out), err == nil
}

// fakeGitHub merges on the bare origin as GitHub would.
type fakeGitHub struct {
	p       *project
	open    *PR
	created int
	checks  [][]Check // what each look at the checks finds; the last stays
	looks   int
	refuse  error
	merged  []string
}

func (f *fakeGitHub) OpenPR(ctx context.Context, root, branch string) (*PR, error) {
	return f.open, nil
}

func (f *fakeGitHub) CreatePR(ctx context.Context, root, base, branch, title, body string) (*PR, error) {
	f.created++
	if base != "develop" || branch != "feat/plan-p1" || !strings.HasPrefix(title, "[docs]") {
		return nil, errors.New("unexpected pull request: " + base + " " + branch + " " + title)
	}
	f.open = &PR{Number: 40 + f.created, URL: "https://github.com/acme/game/pull/41"}
	return f.open, nil
}

func (f *fakeGitHub) Checks(ctx context.Context, root string, pr int) ([]Check, error) {
	f.looks++
	if len(f.checks) == 0 {
		return nil, nil
	}
	at := f.looks - 1
	if at >= len(f.checks) {
		at = len(f.checks) - 1
	}
	return f.checks[at], nil
}

func (f *fakeGitHub) Merge(ctx context.Context, root string, pr int, strategy string) error {
	if f.refuse != nil {
		return f.refuse
	}
	// A squash of the branch onto develop, made in a clone of origin.
	t := f.p.t
	clone := filepath.Join(t.TempDir(), "merge")
	git(t, filepath.Dir(clone), "clone", "--quiet", "-c", "core.autocrlf=false", "--branch", "develop", f.p.origin, clone)
	configure(t, clone)
	git(t, clone, "fetch", "--quiet", "origin", "feat/plan-p1")
	git(t, clone, "merge", "--quiet", "--squash", "FETCH_HEAD")
	git(t, clone, "commit", "--quiet", "-m", "[docs] squashed")
	git(t, clone, "push", "--quiet", "origin", "develop")
	git(t, clone, "push", "--quiet", "origin", "--delete", "feat/plan-p1")
	f.merged = append(f.merged, strategy)
	f.open = nil
	return nil
}

func (p *project) options(changes []Change) Options {
	clock := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	return Options{
		Root: p.root, Integration: "develop", Branch: "feat/plan-p1",
		Title: "[docs] plan p1: what the council wrote", Body: "The documents of plan p1.",
		Changes: changes, GitHub: p.gh,
		Poll: time.Minute, Grace: 2 * time.Minute, Absent: 5 * time.Minute, Wait: 10 * time.Minute,
		Sleep: func(d time.Duration) { clock = clock.Add(d) },
		Now:   func() time.Time { return clock },
	}
}

func TestPending_TellsAddedChangedAndRemoved(t *testing.T) {
	p := newProject(t)
	p.meeting()
	write(t, p.root, "src/main.go", "package main // not a document\n")
	got := p.pending()
	want := []Change{
		{Path: "docs/decisions.md"},
		{Path: "docs/old.md", Deleted: true},
		{Path: "docs/plans/p1/tickets.yaml", Untracked: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pending = %+v\nwant %+v", got, want)
	}
	// A rename is a file removed and one added.
	if got := parseStatus("R  docs/new.md\x00docs/was.md\x00"); !reflect.DeepEqual(got, []Change{{Path: "docs/new.md"}, {Path: "docs/was.md", Deleted: true}}) {
		t.Errorf("rename = %+v", got)
	}
}

// The documents reach develop through a pull request merged once its checks
// passed; the working tree is not touched until Sync, which brings it up to
// develop and leaves it clean.
func TestLand_MergesOnceTheChecksPassThenSyncCleansTheTree(t *testing.T) {
	p := newProject(t)
	p.meeting()
	changes := p.pending()
	p.gh.checks = [][]Check{nil, {{Name: "build", Bucket: "pending"}}, {{Name: "build", Bucket: "pass"}, {Name: "lint", Bucket: "skipping"}}}
	var said []string
	opts := p.options(changes)
	opts.Progress = func(line string) { said = append(said, line) }

	landed, err := Land(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if landed.PR == nil || landed.PR.Number != 41 || p.gh.created != 1 || !reflect.DeepEqual(p.gh.merged, []string{"squash"}) {
		t.Fatalf("landed %+v, created %d, merged %v", landed.PR, p.gh.created, p.gh.merged)
	}
	if text, ok := p.onOrigin("docs/plans/p1/tickets.yaml"); !ok || !strings.Contains(text, "issue: 12") {
		t.Errorf("the work list is not on develop: %q", text)
	}
	if _, ok := p.onOrigin("docs/old.md"); ok {
		t.Error("the removed document is still on develop")
	}
	if text, _ := p.onOrigin("src/main.go"); text != "package main\n" {
		t.Errorf("a file that is not a document was landed: %q", text)
	}
	if !strings.Contains(strings.Join(said, "\n"), "The checks of pull request #41 passed") {
		t.Errorf("progress said:\n%s", strings.Join(said, "\n"))
	}
	// Untouched so far: the meeting's files are as it left them.
	if len(p.pending()) != 3 {
		t.Fatalf("Land changed the working tree: %+v", p.pending())
	}
	if worktrees := git(t, p.root, "worktree", "list"); strings.Count(worktrees, "\n") != 0 {
		t.Errorf("the worktree of the branch was left:\n%s", worktrees)
	}

	synced, err := Sync(context.Background(), nil, p.root, "develop", changes)
	if err != nil || !synced.Updated {
		t.Fatalf("sync = %+v, %v", synced, err)
	}
	if status := git(t, p.root, "status", "--porcelain"); status != "" {
		t.Errorf("the working tree is not clean:\n%s", status)
	}
	if git(t, p.root, "rev-parse", "HEAD") != git(t, p.root, "rev-parse", "origin/develop") {
		t.Error("develop here is not where origin's is")
	}
	if data, _ := os.ReadFile(filepath.Join(p.root, "docs", "decisions.md")); string(data) != "# Decisions\n\n### D1\n" {
		t.Errorf("decisions.md = %q", data)
	}
}

func TestLand_NothingWhenDevelopHasTheDocumentsAlready(t *testing.T) {
	p := newProject(t)
	write(t, p.root, "docs/decisions.md", "# Decisions\n")
	if landed, err := Land(context.Background(), p.options([]Change{{Path: "docs/decisions.md"}})); err != nil || landed.PR != nil || p.gh.created != 0 {
		t.Errorf("landed %+v, %v, %d pull requests", landed, err, p.gh.created)
	}
	if landed, err := Land(context.Background(), p.options(nil)); err != nil || landed.PR != nil {
		t.Errorf("nothing to land: %+v, %v", landed, err)
	}
}

func TestLand_StopsWhenTheChecksFail(t *testing.T) {
	p := newProject(t)
	p.meeting()
	p.gh.checks = [][]Check{{{Name: "build", Bucket: "pending"}}, {{Name: "build", Bucket: "fail"}, {Name: "test", Bucket: "pass"}}}
	landed, err := Land(context.Background(), p.options(p.pending()))
	var failed *ChecksFailed
	if !errors.As(err, &failed) || !reflect.DeepEqual(failed.Failed, []string{"build"}) || landed.PR.Number != 41 {
		t.Fatalf("err = %v, landed %+v", err, landed)
	}
	if len(p.gh.merged) != 0 {
		t.Error("merged though the checks failed")
	}
	if _, ok := p.onOrigin("docs/plans/p1/tickets.yaml"); ok {
		t.Error("the documents are on develop")
	}
	// Approved again once the checks are fixed: the open pull request is
	// taken, with the documents pushed to it again.
	p.gh.checks, p.gh.looks = [][]Check{{{Name: "build", Bucket: "pass"}}}, 0
	if _, err := Land(context.Background(), p.options(p.pending())); err != nil {
		t.Fatal(err)
	}
	if p.gh.created != 1 || len(p.gh.merged) != 1 {
		t.Errorf("created %d pull requests, merged %d", p.gh.created, len(p.gh.merged))
	}
}

func TestLand_NoChecksMeansNoCI(t *testing.T) {
	p := newProject(t)
	p.meeting()
	var said []string
	opts := p.options(p.pending())
	opts.Progress = func(line string) { said = append(said, line) }
	if _, err := Land(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if p.gh.looks < 3 || len(p.gh.merged) != 1 || !strings.Contains(strings.Join(said, "\n"), "has no checks") {
		t.Errorf("looked %d times, merged %v, said %v", p.gh.looks, p.gh.merged, said)
	}
}

func TestLand_ChecksThatTakeTooLong(t *testing.T) {
	p := newProject(t)
	p.meeting()
	p.gh.checks = [][]Check{{{Name: "build", Bucket: "pending"}}}
	_, err := Land(context.Background(), p.options(p.pending()))
	var long *ChecksTooLong
	if !errors.As(err, &long) || len(p.gh.merged) != 0 {
		t.Errorf("err = %v, merged %v", err, p.gh.merged)
	}
}

func TestLand_AMergeGitHubRefusesIsNotForced(t *testing.T) {
	p := newProject(t)
	p.meeting()
	p.gh.checks = [][]Check{{{Name: "build", Bucket: "pass"}}}
	p.gh.refuse = errors.New("the base branch policy prohibits the merge")
	_, err := Land(context.Background(), p.options(p.pending()))
	var refused *MergeRefused
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), "merge it on GitHub") {
		t.Errorf("err = %v", err)
	}
}

// Sync touches nothing it cannot be sure of.
func TestSync_LeavesWhatItCannotBeSureOf(t *testing.T) {
	land := func(t *testing.T) (*project, []Change) {
		p := newProject(t)
		p.meeting()
		changes := p.pending()
		if _, err := Land(context.Background(), p.options(changes)); err != nil {
			t.Fatal(err)
		}
		return p, changes
	}
	untouched := func(t *testing.T, p *project, why string, synced Synced, err error) {
		t.Helper()
		if err != nil || synced.Updated || !strings.Contains(synced.Why, why) {
			t.Errorf("sync = %+v, %v; want why %q", synced, err, why)
		}
		if len(p.pending()) != 3 {
			t.Errorf("the working tree was touched: %+v", p.pending())
		}
	}

	t.Run("another branch", func(t *testing.T) {
		p, changes := land(t)
		git(t, p.root, "checkout", "--quiet", "-b", "feat/mine")
		synced, err := Sync(context.Background(), nil, p.root, "develop", changes)
		untouched(t, p, `on branch "feat/mine"`, synced, err)
	})
	t.Run("a commit of its own", func(t *testing.T) {
		p, changes := land(t)
		write(t, p.root, "src/extra.go", "package main\n")
		git(t, p.root, "add", "src/extra.go")
		git(t, p.root, "commit", "--quiet", "-m", "mine")
		synced, err := Sync(context.Background(), nil, p.root, "develop", changes)
		untouched(t, p, "commits that are not on origin", synced, err)
	})
	t.Run("a document changed since", func(t *testing.T) {
		p, changes := land(t)
		write(t, p.root, "docs/decisions.md", "# Decisions\n\n### D1\n\nmore\n")
		synced, err := Sync(context.Background(), nil, p.root, "develop", changes)
		untouched(t, p, "docs/decisions.md here is not what was merged", synced, err)
	})
	t.Run("line ends a checkout made", func(t *testing.T) {
		p, changes := land(t)
		write(t, p.root, "docs/decisions.md", "# Decisions\r\n\r\n### D1\r\n")
		if synced, err := Sync(context.Background(), nil, p.root, "develop", changes); err != nil || !synced.Updated {
			t.Errorf("sync = %+v, %v", synced, err)
		}
	})
}
