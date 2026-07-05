package selfheal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// gitT runs a git command in dir and fails the test on error, returning stdout.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func hasOnBranch(t *testing.T, repo, ref, path string) bool {
	t.Helper()
	out, _ := exec.Command("git", "-C", repo, "ls-tree", ref, "--", path).Output()
	return strings.TrimSpace(string(out)) != ""
}

// TestReconcileIntegrationScaffold_RealPush exercises the full real mechanic —
// isolated worktree, scaffold regeneration, commit, and push — against a local
// bare remote, so it proves the self-heal actually repairs an integration branch
// that lost its scaffold (the tennis-arena "empty main" corruption) without ever
// touching a real remote. It also asserts idempotency: a second run is a healthy
// no-op, which is what makes it safe to run every analyze-next loop.
func TestReconcileIntegrationScaffold_RealPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	// A bare "origin" and a clone that has a main branch WITHOUT any repo
	// scaffold — exactly the shape left behind when the first task
	// false-completes and the scaffold never lands on the integration branch.
	gitT(t, root, "init", "--bare", "-b", "main", bare)
	gitT(t, root, "clone", bare, work)
	gitT(t, work, "config", "user.email", "selfheal-test@example.com")
	gitT(t, work, "config", "user.name", "Selfheal Test")
	gitT(t, work, "config", "commit.gpgsign", "false")

	writeFile(t, filepath.Join(work, "README.md"), "# proj\n")
	gitT(t, work, "add", "-A")
	gitT(t, work, "commit", "-m", "init without scaffold")
	gitT(t, work, "branch", "-M", "main")
	gitT(t, work, "push", "origin", "main")

	repos := []Repo{
		{Path: "backend/", Language: "go"},
		{Path: "frontend/", Language: "node"},
	}

	// Sanity: the corruption is present on origin/main.
	for _, base := range []string{"backend/go.mod", "frontend/package.json"} {
		if hasOnBranch(t, work, "origin/main", base) {
			t.Fatalf("precondition failed: %s already on origin/main", base)
		}
	}

	// First reconcile must actually fix and push.
	res := ReconcileIntegrationScaffold(work, "main", repos, false)
	if res.Err != nil {
		t.Fatalf("reconcile failed: %v (%s)", res.Err, res.Message)
	}
	if !res.Fixed {
		t.Fatalf("expected a fix to be applied, got %+v", res)
	}

	// The scaffold base files must now exist on origin/main. The push happened
	// from a linked worktree, which shares this repo's remote-tracking refs, so
	// origin/main here reflects the pushed state.
	for _, base := range []string{"backend/go.mod", "frontend/package.json"} {
		if !hasOnBranch(t, work, "origin/main", base) {
			t.Errorf("after reconcile, origin/main still missing %s", base)
		}
	}

	// It must NOT have committed dependency trees. SkipDeps means npm install
	// never runs, so no node_modules can exist to be staged; regressing that
	// would dump thousands of files onto the integration branch.
	tree := gitT(t, work, "ls-tree", "-r", "--name-only", "origin/main")
	if strings.Contains(tree, "node_modules/") {
		t.Errorf("reconcile committed node_modules to origin/main:\n%s", tree)
	}
	// It must NOT write its own .gitignore: a .gitignore that differs from the one
	// `awkit init` commits becomes an add/add conflict against an in-flight
	// scaffold PR (the tennis-arena PR #8 stall). The reconcile restores source
	// scaffold only; .gitignore is init's concern.
	if hasOnBranch(t, work, "origin/main", ".gitignore") {
		t.Errorf("reconcile should not author a .gitignore (conflict vector), but one is on origin/main")
	}

	// The main working tree must be untouched — the fix ran on an isolated
	// worktree, so `work` should have no stray files or dirty state.
	if status := gitT(t, work, "status", "--porcelain"); status != "" {
		t.Errorf("main working tree was disturbed by reconcile: %q", status)
	}
	if hasOnBranch(t, work, "HEAD", "backend/go.mod") {
		t.Errorf("reconcile leaked scaffold into the checked-out branch")
	}

	// Second reconcile is a healthy no-op — safe to run every loop.
	res2 := ReconcileIntegrationScaffold(work, "main", repos, false)
	if res2.Err != nil {
		t.Fatalf("second reconcile errored: %v", res2.Err)
	}
	if !res2.Healthy || res2.Fixed {
		t.Errorf("second reconcile should be a healthy no-op, got %+v", res2)
	}
}
