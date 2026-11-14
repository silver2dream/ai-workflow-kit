package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

// Once a stop was asked for, the decision engine hands out nothing more. This
// is how `awkit stop` reaches a Principal in the middle of its session: the
// run's own loop looks for the marker only between sessions, and a session
// can go on for hours. The reason is the marker's when it is one
// `awkit stop-workflow` takes, and the user's otherwise; no iteration is
// counted and GitHub is not asked.
func TestDecide_AStopWasAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name, marker, want string
	}{
		{"awkit stop", "STOP requested by awkit stop\nReason: user_stopped\n", "user_stopped"},
		{"a marker made by hand", "", "user_stopped"},
		{"the run was already ended for a reason", "STOP requested by stop-workflow\nReason: needs_human_review\n", "needs_human_review"},
		{"a reason nobody knows", "Reason: because\n", "user_stopped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := stopmark.Path(dir)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.marker), 0o644); err != nil {
				t.Fatal(err)
			}

			mock := NewMockGitHubClient()
			config := defaultTestConfig()
			// Work is waiting, and is not handed out.
			mock.PendingIssues = []Issue{{Number: 7, Body: "```yaml awk-ticket\nschema: 1\nrepo: root\n```"}}
			mock.IssuesByLabel[config.GitHub.Labels.InProgress] = []Issue{{Number: 3}}

			decision, err := newTestAnalyzer(dir, config, mock).Decide(context.Background())
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if decision.NextAction != ActionNone || decision.ExitReason != tc.want || decision.IssueNumber != 0 {
				t.Errorf("Decide = %+v, want none with the reason %q", decision, tc.want)
			}
			if len(mock.Calls) != 0 {
				t.Errorf("GitHub was asked: %v", mock.Calls)
			}
			if _, err := os.Stat(filepath.Join(dir, ".ai", "state", "loop_count")); !os.IsNotExist(err) {
				t.Errorf("an iteration was counted (err %v)", err)
			}
			if _, present := stopmark.Read(dir); !present {
				t.Error("the decision engine removed the marker; the run's loop still has to see it")
			}
		})
	}

	// Without the marker the engine decides as ever.
	dir := t.TempDir()
	mock := NewMockGitHubClient()
	config := defaultTestConfig()
	mock.IssuesByLabel[config.GitHub.Labels.InProgress] = []Issue{{Number: 3}}
	decision, err := newTestAnalyzer(dir, config, mock).Decide(context.Background())
	if err != nil || decision.NextAction != ActionCheckResult {
		t.Errorf("without a marker: %+v (err %v), want the in-progress ticket checked", decision, err)
	}
}

func TestReasonStopRequested(t *testing.T) {
	if ReasonStopRequested != "user_stopped" {
		t.Errorf("ReasonStopRequested = %q; `awkit stop-workflow` takes user_stopped", ReasonStopRequested)
	}
}
