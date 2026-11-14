package jittest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The file names of generated tests come from a model that read the diff. A
// name is taken only if the file lands where a generated test belongs.
func TestJitTestPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/existing_jittest_test.go", "package pkg\n")
	outside := t.TempDir()

	accepted := map[string]string{
		"pkg/foo_jittest_test.go":                    "pkg/foo_jittest_test.go",
		"foo_jittest_test.go":                        "foo_jittest_test.go",
		"new/dir/foo.jittest.test.ts":                "new/dir/foo.jittest.test.ts",
		"tests/Game.Core.Tests/Counter.jittest.cs":   "tests/Game.Core.Tests/Counter.jittest.cs",
		"  pkg/padded_jittest_test.go  ":             "pkg/padded_jittest_test.go",
		"pkg/../pkg/roundabout_jittest_test.go":      "pkg/roundabout_jittest_test.go",
		"pkg/with space/foo_jittest_test.go":         "pkg/with space/foo_jittest_test.go",
		"pkg/..hidden-looking/foo_jittest_test.go":   "",
		"pkg/sub.dir/foo_jittest_test.go":            "pkg/sub.dir/foo_jittest_test.go",
		"tests/test_handler_jittest.py":              "tests/test_handler_jittest.py",
		"pkg/jittest_only_in_name.go":                "pkg/jittest_only_in_name.go",
		"deep/a/b/c/d/e/f/g/h/i/foo_jittest_test.go": "deep/a/b/c/d/e/f/g/h/i/foo_jittest_test.go",
		"pkg/UPPER_jittest_Test.go":                  "pkg/UPPER_jittest_Test.go",
		"pkg/unicode_測試_jittest_test.go":             "pkg/unicode_測試_jittest_test.go",
		"pkg/dots...jittest.go":                      "pkg/dots...jittest.go",
		"pkg/trailing.jittest":                       "pkg/trailing.jittest",
		"jittest":                                    "jittest",
		"pkg/a_jittest_test.go/../b_jittest_test.go": "pkg/b_jittest_test.go",
		"pkg//double//slash_jittest_test.go":         "pkg/double/slash_jittest_test.go",
		"./pkg/dot_slash_jittest_test.go":            "pkg/dot_slash_jittest_test.go",
	}
	for name, want := range accepted {
		got, err := jitTestPath(dir, name)
		if want == "" {
			if err == nil {
				t.Errorf("jitTestPath(%q) = %q, want it refused", name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("jitTestPath(%q) refused: %v", name, err)
			continue
		}
		if want := filepath.Join(dir, filepath.FromSlash(want)); got != want {
			t.Errorf("jitTestPath(%q) = %q, want %q", name, got, want)
		}
	}

	refused := map[string]string{
		"":                                 "inside the worktree",
		"   ":                              "inside the worktree",
		"../escape_jittest_test.go":        "leaves the worktree",
		"pkg/../../escape_jittest_test.go": "leaves the worktree",
		"..":                               "leaves the worktree",
		filepath.Join(outside, "abs_jittest_test.go"): "inside the worktree",
		"/etc/cron.d/jittest":                         "inside the worktree",
		// Not where tests go.
		".git/hooks/jittest":              "tests do not go into .git",
		".github/workflows/jittest.yml":   "tests do not go into .github",
		".ai/state/jittest.json":          "tests do not go into .ai",
		"pkg/.hidden/foo_jittest_test.go": "tests do not go into .hidden",
		// The cleanup finds generated files by their name.
		"pkg/foo_test.go":     `must contain "jittest"`,
		"jittest/foo_test.go": `must contain "jittest"`,
		// The cleanup deletes what was written, so nothing existing is replaced.
		"pkg/existing_jittest_test.go": "already exists",
	}
	for name, wantReason := range refused {
		got, err := jitTestPath(dir, name)
		if err == nil {
			t.Errorf("jitTestPath(%q) = %q, want it refused (%s)", name, got, wantReason)
			continue
		}
		if !strings.Contains(err.Error(), wantReason) {
			t.Errorf("jitTestPath(%q) refused with %q, want a reason like %q", name, err, wantReason)
		}
	}

	if runtime.GOOS == "windows" {
		for _, name := range []string{`C:\Windows\Temp\foo_jittest_test.go`, `\\server\share\foo_jittest_test.go`, `C:foo_jittest_test.go`} {
			if got, err := jitTestPath(dir, name); err == nil {
				t.Errorf("jitTestPath(%q) = %q, want it refused", name, got)
			}
		}
	}
}

// A directory of the worktree that is a link to somewhere else does not let a
// generated file out.
func TestJitTestPath_LinkOutOfTheWorktree(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	// A link that stays inside the worktree is fine.
	if err := os.MkdirAll(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "alias")); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	for _, name := range []string{"linked/foo_jittest_test.go", "linked/new/dir/foo_jittest_test.go"} {
		if got, err := jitTestPath(dir, name); err == nil {
			t.Errorf("jitTestPath(%q) = %q, which is outside the worktree", name, got)
		} else if !strings.Contains(err.Error(), "link out of the worktree") {
			t.Errorf("jitTestPath(%q) refused with %q", name, err)
		}
	}
	if _, err := jitTestPath(dir, "alias/foo_jittest_test.go"); err != nil {
		t.Errorf("a link that stays inside the worktree was refused: %v", err)
	}
}

// A refused file name is skipped and reported; nothing is written for it and
// the rest of the run goes on.
func TestRunTests_RefusedFileNamesAreSkipped(t *testing.T) {
	origExec := executeTests
	defer func() { executeTests = origExec }()
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		return testExecOutcome{exitCode: 0}
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "worktree")
	writeFile(t, dir, "pkg/keep.go", "package pkg\n")
	const original = "package pkg // the project's own file\n"
	writeFile(t, dir, "pkg/real_jittest_test.go", original)

	tests := []GeneratedTest{
		{Filename: "../escaped_jittest_test.go", Content: "package x\n"},
		{Filename: "pkg/real_jittest_test.go", Content: "package overwritten\n"},
		{Filename: "pkg/plain_test.go", Content: "package pkg\n"},
		{Filename: "pkg/good_jittest_test.go", Content: "package pkg\n"},
	}

	result := RunTests(context.Background(), dir, tests, "echo ok")

	if result.Generated != 4 || result.Passed != 1 || result.Skipped != 3 || result.Failed != 0 {
		t.Errorf("generated %d, passed %d, skipped %d, failed %d; want 4, 1, 3, 0", result.Generated, result.Passed, result.Skipped, result.Failed)
	}
	if _, err := os.Stat(filepath.Join(parent, "escaped_jittest_test.go")); !os.IsNotExist(err) {
		t.Error("a file was written outside the worktree")
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg", "plain_test.go")); !os.IsNotExist(err) {
		t.Error("a file the cleanup could not find by name was written")
	}
	refusals := 0
	for _, outcome := range result.Tests {
		if strings.Contains(outcome.Error, "refused to write") {
			refusals++
		}
	}
	if refusals != 3 {
		t.Errorf("%d refusals reported, want 3: %+v", refusals, result.Tests)
	}
}

// When every name is refused there is nothing to run.
func TestRunTests_AllRefused(t *testing.T) {
	origExec := executeTests
	defer func() { executeTests = origExec }()
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		t.Error("the test command ran although no file was written")
		return testExecOutcome{}
	}

	result := RunTests(context.Background(), t.TempDir(), []GeneratedTest{{Filename: "../x_jittest_test.go", Content: "x"}}, "echo ok")

	if result.Skipped != 1 || result.Error == "" {
		t.Errorf("result = %+v, want the one test skipped and an error noted", result)
	}
}
