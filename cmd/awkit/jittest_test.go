package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A project whose configuration names no repo is one repo, its root. The
// command used to take the language of the first repo without looking
// whether there is one.
func TestCmdJiTTest_AProjectWithoutRepos(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".ai", "config", "workflow.yaml")
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
	for _, want := range []string{"Max tests: 5", "Timeout: 120s", "Failure policy: warn", "Model: claude-sonnet-4-6"} {
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
