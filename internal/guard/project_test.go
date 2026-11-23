package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIssueOfDir(t *testing.T) {
	for dir, want := range map[string]int{
		`C:\games\rally\.worktrees\issue-7`:       7,
		"/games/rally/.worktrees/issue-31/server": 31,
		"/games/rally": 0,
		"":             0,
	} {
		if got := IssueOfDir(dir); got != want {
			t.Errorf("%q: %d, want %d", dir, got, want)
		}
	}
}

func TestProjectRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	sub := filepath.Join(root, "server", "game")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	for _, dir := range []string{root, sub} {
		got, _ := filepath.EvalSymlinks(ProjectRoot(dir))
		if !sameDir(got, want) {
			t.Errorf("ProjectRoot(%s) = %q, want %q", dir, got, want)
		}
	}
	if got := ProjectRoot(t.TempDir()); got != "" {
		t.Errorf("outside any repository: %q, want nothing", got)
	}
	if got := ProjectRoot(""); got != "" {
		t.Errorf("no directory: %q", got)
	}
}

func sameDir(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
