package main

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

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/landing"
)

func mergeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mergeConfigure(t *testing.T, dir string) {
	for _, kv := range [][2]string{{"user.name", "test"}, {"user.email", "test@example.invalid"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}} {
		mergeGit(t, dir, "config", kv[0], kv[1])
	}
}

// originGitHub merges a pull request on a bare origin, as GitHub would.
type originGitHub struct {
	t      *testing.T
	origin string
	open   *landing.PR
	checks []landing.Check
	merged int
}

func (g *originGitHub) OpenPR(ctx context.Context, root, branch string) (*landing.PR, error) {
	return g.open, nil
}

func (g *originGitHub) CreatePR(ctx context.Context, root, base, branch, title, body string) (*landing.PR, error) {
	g.open = &landing.PR{Number: 41, URL: "https://github.com/acme/game/pull/41"}
	return g.open, nil
}

func (g *originGitHub) Checks(ctx context.Context, root string, pr int) ([]landing.Check, error) {
	return g.checks, nil
}

func (g *originGitHub) Merge(ctx context.Context, root string, pr int, strategy string) error {
	clone := filepath.Join(g.t.TempDir(), "merge")
	mergeGit(g.t, filepath.Dir(clone), "clone", "--quiet", "-c", "core.autocrlf=false", "--branch", "develop", g.origin, clone)
	mergeConfigure(g.t, clone)
	branch := mergeGit(g.t, clone, "ls-remote", "--heads", "origin")
	name := branch[strings.LastIndex(branch, "refs/heads/feat/")+len("refs/heads/"):]
	mergeGit(g.t, clone, "fetch", "--quiet", "origin", name)
	mergeGit(g.t, clone, "merge", "--quiet", "--squash", "FETCH_HEAD")
	mergeGit(g.t, clone, "commit", "--quiet", "-m", "[docs] squashed")
	mergeGit(g.t, clone, "push", "--quiet", "origin", "develop", ":"+name)
	g.merged++
	g.open = nil
	return nil
}

// mergeProject is a project cloned from a bare origin whose integration
// branch is develop, with a plan held: its documents in the working tree.
func mergeProject(t *testing.T) (root, origin, id string, gh *originGitHub) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	root = filepath.Join(base, "game")
	mergeGit(t, base, "init", "--quiet", "--bare", "--initial-branch=develop", origin)
	mergeGit(t, base, "clone", "--quiet", "-c", "core.autocrlf=false", origin, root)
	mergeConfigure(t, root)
	mergeGit(t, root, "checkout", "--quiet", "-B", "develop")
	writePlanConfig(t, root, "repos:\n  - {name: backend, path: backend/, language: go}\n")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".ai/state/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mergeGit(t, root, "add", "-A")
	mergeGit(t, root, "commit", "--quiet", "-m", "start")
	mergeGit(t, root, "push", "--quiet", "-u", "origin", "develop")

	origModel := planModel
	t.Cleanup(func() { planModel = origModel })
	planModel = &scriptedCouncil{}
	_ = captureStdout(t, func() {
		if code := cmdPlan([]string{"Add a health endpoint.", "--state-root", root}); code != 0 {
			t.Fatalf("cmdPlan = %d", code)
		}
	})
	plans, _ := filepath.Glob(filepath.Join(root, "docs", "plans", "*"))
	if len(plans) != 1 {
		t.Fatalf("%d plans, want one", len(plans))
	}
	gh = &originGitHub{t: t, origin: origin, checks: []landing.Check{{Name: "build", Bucket: "pass"}}}
	return root, origin, filepath.Base(plans[0]), gh
}

// stubMerging puts stand-ins for GitHub in place; it returns the issues
// created and the issues labelled.
func stubMerging(t *testing.T, gh *originGitHub, running bool) (created *[]string, labelled *[]int) {
	t.Helper()
	origCreate, origLabel, origGitHub, origTimings, origRunning := planCreateUnlabeled, planLabel, planGitHub, planLandTimings, planRunGoing
	t.Cleanup(func() {
		planCreateUnlabeled, planLabel, planGitHub, planLandTimings, planRunGoing = origCreate, origLabel, origGitHub, origTimings, origRunning
	})
	created, labelled = &[]string{}, &[]int{}
	planCreateUnlabeled = func(ctx context.Context, stateRoot, title, bodyFile string) (int, error) {
		*created = append(*created, title)
		return 100 + len(*created), nil
	}
	planLabel = func(ctx context.Context, stateRoot string, issues []int) error {
		*labelled = append(*labelled, issues...)
		return nil
	}
	planGitHub = gh
	planRunGoing = func(string) bool { return running }
	planLandTimings = func(o *landing.Options) {
		clock := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
		o.Poll, o.Grace, o.Absent, o.Wait = time.Second, 3*time.Second, 10*time.Second, time.Minute
		o.Sleep = func(d time.Duration) { clock = clock.Add(d) }
		o.Now = func() time.Time { return clock }
	}
	return created, labelled
}

func approveMerging(t *testing.T, root, id string) (int, string) {
	t.Helper()
	var code int
	out := captureStdout(t, func() {
		code = cmdPlan([]string{"approve", id, "--merge-docs", "--state-root", root})
	})
	return code, out
}

// The documents of a plan reach develop before any of its tickets can be
// taken up: the issues are labelled once they are merged, and the working
// tree is left clean for a run.
func TestCmdPlanApprove_MergeDocs(t *testing.T) {
	root, origin, id, gh := mergeProject(t)
	created, labelled := stubMerging(t, gh, false)

	code, out := approveMerging(t, root, id)
	if code != 0 {
		t.Fatalf("approve = %d\n%s", code, out)
	}
	if len(*created) != 2 || !reflect.DeepEqual(*labelled, []int{101, 102}) || gh.merged != 1 {
		t.Fatalf("created %v, labelled %v, merged %d\n%s", *created, *labelled, gh.merged, out)
	}
	tickets, err := exec.Command("git", "--git-dir", origin, "show", "develop:docs/plans/"+id+"/tickets.yaml").Output()
	if err != nil || !strings.Contains(string(tickets), "issue: 101") {
		t.Errorf("the work list with its issues is not on develop: %v\n%s", err, tickets)
	}
	if _, err := exec.Command("git", "--git-dir", origin, "show", "develop:docs/decisions.md").Output(); err != nil {
		t.Error("the decision log is not on develop")
	}
	if status := mergeGit(t, root, "status", "--porcelain"); status != "" {
		t.Errorf("the working tree is not clean for a run:\n%s", status)
	}
	record, err := council.ReadApproval(root, id)
	if err != nil || record.State != council.ApprovalDone || record.Step != "done" || record.PR != 41 || record.Created != 2 || !reflect.DeepEqual(record.Issues, []int{101, 102}) || record.Left != "" {
		t.Errorf("record = %+v, %v", record, err)
	}

	// Approved again: nothing new is created or merged; the labels are put on again.
	code, out = approveMerging(t, root, id)
	if code != 0 || len(*created) != 2 || gh.merged != 1 || len(*labelled) != 4 {
		t.Errorf("again: %d, created %v, merged %d, labelled %v\n%s", code, *created, gh.merged, *labelled, out)
	}
	if !strings.Contains(out, "0 issue(s) created, 2 existed already") {
		t.Errorf("again said:\n%s", out)
	}
}

// Checks that fail stop the approval before a ticket gets its label; once
// they pass, approving again goes on with the same issues.
func TestCmdPlanApprove_MergeDocsStopsWhenTheChecksFail(t *testing.T) {
	root, _, id, gh := mergeProject(t)
	created, labelled := stubMerging(t, gh, false)
	gh.checks = []landing.Check{{Name: "build", Bucket: "fail"}}

	if code, out := approveMerging(t, root, id); code != 1 {
		t.Fatalf("approve = %d\n%s", code, out)
	}
	record, _ := council.ReadApproval(root, id)
	if record == nil || record.State != council.ApprovalFailed || record.Why != "checks" || record.Step != "checks" || record.PR != 41 {
		t.Errorf("record = %+v", record)
	}
	if len(*labelled) != 0 || gh.merged != 0 || len(*created) != 2 {
		t.Errorf("labelled %v, merged %d, created %v", *labelled, gh.merged, *created)
	}

	gh.checks = []landing.Check{{Name: "build", Bucket: "pass"}}
	if code, out := approveMerging(t, root, id); code != 0 {
		t.Fatalf("again = %d\n%s", code, out)
	}
	if len(*created) != 2 || gh.merged != 1 || !reflect.DeepEqual(*labelled, []int{101, 102}) {
		t.Errorf("again: created %v, merged %d, labelled %v", *created, gh.merged, *labelled)
	}
}

// Nothing is created while a run is going, nor when origin lacks the branch.
func TestCmdPlanApprove_MergeDocsRefusesBeforeCreatingAnything(t *testing.T) {
	root, _, id, gh := mergeProject(t)
	created, _ := stubMerging(t, gh, true)
	if code, _ := approveMerging(t, root, id); code != 1 || len(*created) != 0 {
		t.Errorf("with a run going: %d, created %v", code, *created)
	}
	if record, _ := council.ReadApproval(root, id); record == nil || record.Why != "run" {
		t.Errorf("record = %+v", record)
	}

	planRunGoing = func(string) bool { return false }
	mergeGit(t, root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	if code, _ := approveMerging(t, root, id); code != 1 || len(*created) != 0 {
		t.Errorf("without the branch on origin: %d, created %v", code, *created)
	}
	if record, _ := council.ReadApproval(root, id); record == nil || record.Why != "branch" {
		t.Errorf("record = %+v", record)
	}
}

// A dry run says what would happen, and does none of it.
func TestCmdPlanApprove_MergeDocsDryRun(t *testing.T) {
	root, _, id, gh := mergeProject(t)
	created, labelled := stubMerging(t, gh, false)
	var code int
	out := captureStdout(t, func() {
		code = cmdPlan([]string{"approve", id, "--merge-docs", "--dry-run", "--state-root", root})
	})
	if code != 0 || len(*created) != 0 || len(*labelled) != 0 || gh.merged != 0 {
		t.Fatalf("dry run = %d, created %v, labelled %v, merged %d\n%s", code, *created, *labelled, gh.merged, out)
	}
	for _, want := range []string{"2 issue(s) would be created without the task label", "docs/decisions.md", "docs/plans/" + id + "/tickets.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run does not say %q:\n%s", want, out)
		}
	}
	if _, err := council.ReadApproval(root, id); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run wrote an approval record: %v", err)
	}
}
