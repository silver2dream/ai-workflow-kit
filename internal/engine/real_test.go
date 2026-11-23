package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/agent"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
)

// withAwkit runs agents with the command-policy hook of a given awkit: in a
// test, this program is not awkit.
type withAwkit struct {
	runner *agent.Runner
	awkit  string
}

func (w withAwkit) Run(ctx context.Context, env agent.Envelope) agent.Result {
	env.Awkit = w.awkit
	return w.runner.Run(ctx, env)
}

// TestRealReview has the real reviewer (the claude CLI, with the reviewer's
// default settings) review a small change in a project of its own, and the
// review's own decision act on it, through a publisher that only records.
// No GitHub is reached: the review's reads of it are stood in for, and the
// package's stand-in gh is first on the PATH. It is skipped unless
// AWKIT_REAL_AGENTS names an awkit built from this tree: it calls a model,
// and CI has no login.
func TestRealReview(t *testing.T) {
	awkit := os.Getenv("AWKIT_REAL_AGENTS")
	if awkit == "" {
		t.Skip("set AWKIT_REAL_AGENTS to an awkit built from this tree to run the real reviewer")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai", "config.yaml"), reviewConfig)
	writeFile(t, filepath.Join(root, ".claude", "rules", "go-style.md"), "---\npaths: [\"*.go\"]\n---\n# Go style\n\nEvery exported function has a doc comment.\n")

	wt := filepath.Join(root, ".worktrees", "issue-12")
	writeFile(t, filepath.Join(wt, "go.mod"), "module example.com/rally\n\ngo 1.22\n")
	gitIn(t, wt, "init", "-q")
	gitIn(t, wt, "add", "-A")
	gitIn(t, wt, "commit", "-qm", "[chore] start the module")
	writeFile(t, filepath.Join(wt, "score.go"), "package rally\n\nimport \"fmt\"\n\n// Score shows a rally's score: the two players' points, joined by a dash.\nfunc Score(a, b int) string {\n\treturn fmt.Sprintf(\"%d-%d\", a, b)\n}\n")
	writeFile(t, filepath.Join(wt, "score_test.go"), "package rally\n\nimport \"testing\"\n\nfunc TestScore(t *testing.T) {\n\tif got := Score(1, 2); got != \"1-2\" {\n\t\tt.Errorf(\"Score(1, 2) = %q, want 1-2\", got)\n\t}\n\tif got := Score(10, 0); got != \"10-0\" {\n\t\tt.Errorf(\"Score(10, 0) = %q, want 10-0\", got)\n\t}\n}\n")
	gitIn(t, wt, "add", "-A")
	gitIn(t, wt, "commit", "-qm", "[feat] show a rally's score")
	diff := gitIn(t, wt, "diff", "HEAD~1", "HEAD")

	rec := &recordingPublisher{}
	runner := agent.NewRunner()
	step := &ReviewStep{
		Root: root, Agents: withAwkit{runner, awkit}, Publisher: EnginePublisher{rec},
		prepare: func(context.Context, reviewer.PrepareReviewOptions) (*reviewer.ReviewContext, error) {
			return &reviewer.ReviewContext{
				PRNumber: 30, IssueNumber: 12, CIStatus: "passed", WorktreePath: wt,
				TestCommand: "go test ./...", Language: "go",
				Ticket:      "# Show a rally's score\n\n## Acceptance Criteria\n- [ ] Score(a, b) returns the two points joined by a dash: \"1-2\" for 1 and 2\n- [ ] All tests pass\n",
				CommitsJSON: "- " + strings.TrimSpace(gitIn(t, wt, "log", "-1", "--format=%h %s")),
			}, nil
		},
		diff: func(context.Context, int, time.Duration) (string, error) { return diff, nil },
	}
	step.defaults()
	step.Profile.USD = 2

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	lane := Lane{Issue: 12, State: Reviewing, PR: 30, Seq: 3}
	tr, err := step.Review(ctx, lane)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("the reviewer (%s %s) moved the lane to %s (%s): %s", step.Profile.Backend, step.Profile.Model, tr.To, tr.Cause, tr.Note)
	t.Logf("what the review did:\n%s", strings.Join(rec.calls, "\n"))
	answer, _ := os.ReadFile(filepath.Join(agent.RunDir(root, 12, 3), "reviewer-1", "try-1", "answer.json"))
	t.Logf("the reviewer's answer:\n%s", answer)
	events := eventsWith(t, root, `"role":"reviewer"`)
	t.Logf("its run, as the event log has it:\n%s", strings.Join(events, "\n"))
	if _, err := Apply(&lane, tr); err != nil {
		t.Fatal(err)
	}
	// The change does what the ticket asks, and its test covers it: a
	// review that reads it approves, and its evidence holds.
	if tr.To != Merged || !strings.Contains(strings.Join(rec.calls, "\n"), "merge 30 squash") {
		t.Fatalf("the change was not merged: %+v", tr)
	}
	if len(events) == 0 {
		t.Error("the reviewer's run is not in the event log")
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.invalid", "-c", "user.name=t", "-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
