package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/jittest"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// A project whose configuration names no repo is one repo, its root. The
// command used to take the language of the first repo without looking
// whether there is one.
func TestCmdJiTTest_AProjectWithoutRepos(t *testing.T) {
	dir := t.TempDir()
	config := layout.Config(dir)
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("git: {integration_branch: develop}\nreview: {jittest: {enabled: true}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := -1
	out := captureStdout(t, func() {
		code = cmdJiTTest([]string{"--pr", "1", "--issue", "2", "--state-root", dir})
	})

	// No language to generate tests for: the run says so, and is not a crash.
	if code != 0 {
		t.Errorf("jittest = %d, want 0: with the default policy a run that could not test is a warning", code)
	}
	if !strings.Contains(out, `JiT tests are not available for language \"\"`) {
		t.Errorf("the result should say why nothing was tested:\n%s", out)
	}

	// The dry run shows what the configuration gives, defaults included.
	out = captureStdout(t, func() {
		code = cmdJiTTest([]string{"--pr", "1", "--issue", "2", "--state-root", dir, "--dry-run"})
	})
	for _, want := range []string{"Max tests: 5", "Timeout: 120s", "Failure policy: warn", "Model: sonnet"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("the dry run (%d) should show %q:\n%s", code, want, out)
		}
	}

	// Turned off, it says so and does nothing.
	if err := os.WriteFile(config, []byte("git: {integration_branch: develop}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr string
	_ = captureStdout(t, func() {
		stderr = captureStderr(func() { code = cmdJiTTest([]string{"--pr", "1", "--issue", "2", "--state-root", dir}) })
	})
	if code != 1 || !strings.Contains(stderr, "jittest is not enabled") {
		t.Errorf("with JiT tests off: %d\n%s", code, stderr)
	}
}

// The command hands the test generator what the configuration says: the diff
// is taken against the integration branch, for the language of the first
// repo. It used to pass neither.
func TestCmdJiTTest_GivesTheGeneratorTheBranchAndTheLanguage(t *testing.T) {
	orig := runJiTTests
	t.Cleanup(func() { runJiTTests = orig })
	var inputs []jittest.Input
	var configs []analyzer.JiTTestConfig
	runJiTTests = func(ctx context.Context, input jittest.Input, cfg analyzer.JiTTestConfig) (*jittest.Result, error) {
		inputs = append(inputs, input)
		configs = append(configs, cfg)
		return &jittest.Result{Generated: 2, Passed: 2}, nil
	}

	dir := t.TempDir()
	file := layout.Config(dir)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "git: {integration_branch: trunk}\n" +
		"repos: [{name: api, path: ./, language: go}, {name: ui, path: ui/, language: godot}]\n" +
		"review: {jittest: {enabled: true, max_tests: 3}}\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := -1
	out := captureStdout(t, func() {
		code = cmdJiTTest([]string{"--pr", "7", "--issue", "3", "--state-root", dir})
	})

	if code != 0 || len(inputs) != 1 {
		t.Fatalf("jittest = %d after %d run(s) of the tests:\n%s", code, len(inputs), out)
	}
	// No remote here, so the base is the branch as the project has it.
	want := jittest.Input{PRNumber: 7, IssueNumber: 3, WorkDir: dir, BaseBranch: "trunk", HeadBranch: "HEAD", Language: "go"}
	if inputs[0] != want {
		t.Errorf("the generator was given\n%+v\nwant\n%+v", inputs[0], want)
	}
	if configs[0].MaxTests != 3 || configs[0].TimeoutSeconds != 120 || configs[0].FailurePolicy != "warn" {
		t.Errorf("the tests were configured with %+v, want what the file says and the defaults", configs[0])
	}
	if !strings.Contains(out, `"generated": 2`) || !strings.Contains(out, `"passed": 2`) {
		t.Errorf("the result is not printed:\n%s", out)
	}
	// The run is counted under the language it was for.
	stats, err := jittest.LoadStats(dir)
	if err != nil || stats.TotalRuns != 1 || stats.ByLanguage["go"] == nil || stats.ByLanguage["go"].Runs != 1 {
		t.Errorf("the run is not recorded as one run of go tests: %+v (err %v)", stats, err)
	}

	// Where the remote has the branch, the diff is taken against that one: a
	// local branch of the same name may be behind.
	if _, err := exec.LookPath("git"); err != nil {
		t.Log("git is not installed: the base as the remote has it is not checked")
		return
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-q", "-b", "trunk")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "a.txt")
	git("commit", "-q", "-m", "first")
	git("update-ref", "refs/remotes/origin/trunk", git("rev-parse", "HEAD"))
	_ = captureStdout(t, func() {
		code = cmdJiTTest([]string{"--pr", "7", "--issue", "3", "--state-root", dir})
	})
	if got := inputs[len(inputs)-1].BaseBranch; code != 0 || len(inputs) != 2 || got != "origin/trunk" {
		t.Errorf("jittest = %d after %d run(s), compared with %q; want the integration branch as the remote has it", code, len(inputs), got)
	}
}
