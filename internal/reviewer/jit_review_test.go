package reviewer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/jittest"
)

// The JiT tests of a review are generated from what the pull request adds to
// the branch it goes to: the integration branch of the configuration, not
// "main". Turned off, or without a configuration, nothing is run.
func TestRunJiTTestIfEnabled_ComparesWithTheIntegrationBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	orig := runJiT
	t.Cleanup(func() { runJiT = orig })
	var inputs []jittest.Input
	var configs []analyzer.JiTTestConfig
	runJiT = func(ctx context.Context, input jittest.Input, cfg analyzer.JiTTestConfig) (*jittest.Result, error) {
		inputs = append(inputs, input)
		configs = append(configs, cfg)
		return &jittest.Result{Generated: 1, Passed: 1}, nil
	}

	root := t.TempDir()
	writeConfig := func(content string) {
		t.Helper()
		path := filepath.Join(root, ".ai", "config", "workflow.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	worktree := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", worktree, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "trunk")
	if err := os.WriteFile(filepath.Join(worktree, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "a.txt")
	git("commit", "-q", "-m", "first")
	opts := SubmitReviewOptions{PRNumber: 7, IssueNumber: 3, StateRoot: root, WorktreePath: worktree, Language: "go", TestCommand: "go test ./..."}

	// No configuration, and JiT tests that are off: nothing is run.
	if result := runJiTTestIfEnabled(context.Background(), opts); result != nil || len(inputs) != 0 {
		t.Fatalf("without a configuration JiT tests ran: %+v", inputs)
	}
	writeConfig("git: {integration_branch: trunk}\n")
	if result := runJiTTestIfEnabled(context.Background(), opts); result != nil || len(inputs) != 0 {
		t.Fatalf("turned off, JiT tests ran: %+v", inputs)
	}

	// On, with a branch the remote does not have here: the local branch.
	writeConfig("git: {integration_branch: trunk}\nreview: {jittest: {enabled: true, max_tests: 2}}\n")
	result := runJiTTestIfEnabled(context.Background(), opts)
	if result == nil || len(inputs) != 1 {
		t.Fatalf("turned on, JiT tests did not run: %+v", inputs)
	}
	want := jittest.Input{PRNumber: 7, IssueNumber: 3, WorkDir: worktree, BaseBranch: "trunk", HeadBranch: "HEAD", Language: "go", TestCommand: "go test ./..."}
	if inputs[0] != want {
		t.Errorf("the JiT tests were given\n%+v\nwant\n%+v", inputs[0], want)
	}
	if configs[0].MaxTests != 2 || configs[0].TimeoutSeconds != 120 || configs[0].Model == "" {
		t.Errorf("the JiT tests were configured with %+v, want what the file says and the defaults", configs[0])
	}

	// With the branch as the remote has it: that one.
	git("update-ref", "refs/remotes/origin/trunk", git("rev-parse", "HEAD"))
	runJiTTestIfEnabled(context.Background(), opts)
	if got := inputs[len(inputs)-1].BaseBranch; got != "origin/trunk" {
		t.Errorf("the base is %q, want the integration branch as the remote has it", got)
	}
}
