package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// run-issue passes no timeout of its own, so that the Worker takes them from
// the configuration; a timeout given on the command line is passed on. It
// used to pass 60s, 120s and 30m whatever the configuration said.
func TestCmdRunIssue_TimeoutsComeFromTheConfiguration(t *testing.T) {
	orig := runIssue
	t.Cleanup(func() { runIssue = orig })
	var got worker.RunIssueOptions
	runIssue = func(ctx context.Context, opts worker.RunIssueOptions) (*worker.RunIssueResult, error) {
		got = opts
		return &worker.RunIssueResult{Status: "success", PRURL: "https://example.invalid/pr/1"}, nil
	}
	dir := t.TempDir()

	out := captureStdout(t, func() {
		if code := cmdRunIssue([]string{"--issue", "5", "--ticket", "ticket.md", "--state-root", dir}); code != 0 {
			t.Errorf("run-issue = %d", code)
		}
	})

	if got.IssueID != 5 || got.TicketFile != "ticket.md" || got.StateRoot != dir {
		t.Errorf("the Worker was given %+v", got)
	}
	if got.GHTimeout != 0 || got.GitTimeout != 0 || got.CodexTimeout != 0 {
		t.Errorf("run-issue passes timeouts of its own (%v, %v, %v): the configuration's are never used", got.GHTimeout, got.GitTimeout, got.CodexTimeout)
	}
	if !strings.Contains(out, "STATUS=success\n") || !strings.Contains(out, "PR_URL=https://example.invalid/pr/1\n") {
		t.Errorf("the result is not printed:\n%s", out)
	}

	_ = captureStdout(t, func() {
		args := []string{"--issue", "5", "--ticket", "ticket.md", "--state-root", dir, "--gh-timeout", "5s", "--git-timeout", "6s", "--codex-timeout", "7m"}
		if code := cmdRunIssue(args); code != 0 {
			t.Errorf("run-issue with timeouts = %d", code)
		}
	})
	if got.GHTimeout != 5*time.Second || got.GitTimeout != 6*time.Second || got.CodexTimeout != 7*time.Minute {
		t.Errorf("the timeouts of the command line are not passed on: %v, %v, %v", got.GHTimeout, got.GitTimeout, got.CodexTimeout)
	}

	// The help says where a timeout comes from when no flag gives it.
	if help := captureStderr(usageRunIssue); !strings.Contains(help, "timeouts.gh_seconds of "+layout.ConfigFile) || !strings.Contains(help, "timeouts.codex_minutes") {
		t.Errorf("the help does not name the configuration:\n%s", help)
	}
}
