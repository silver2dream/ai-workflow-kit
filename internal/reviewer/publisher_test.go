package reviewer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// recordingPublisher does nothing outside the test, and keeps what the
// review asked of it.
type recordingPublisher struct {
	calls      []string
	mergeErr   error
	mergeState string
	commentErr error
}

func (p *recordingPublisher) did(format string, args ...any) {
	p.calls = append(p.calls, fmt.Sprintf(format, args...))
}
func (p *recordingPublisher) Ticket(ctx context.Context, issue int) (string, error) {
	p.did("ticket %d", issue)
	return "", errors.New("the test gives the ticket")
}
func (p *recordingPublisher) CommentPR(ctx context.Context, pr int, body string) error {
	p.did("comment-pr %d", pr)
	return p.commentErr
}
func (p *recordingPublisher) CommentIssue(ctx context.Context, issue int, body string) error {
	first := strings.SplitN(body, "\n", 2)[0]
	p.did("comment-issue %d %s", issue, first)
	return nil
}
func (p *recordingPublisher) Approve(ctx context.Context, pr, score int) error {
	p.did("approve %d %d", pr, score)
	return nil
}
func (p *recordingPublisher) RequestChanges(ctx context.Context, pr, score int) error {
	p.did("request-changes %d %d", pr, score)
	return nil
}
func (p *recordingPublisher) Merge(ctx context.Context, pr int, strategy string) error {
	p.did("merge %d %s", pr, strategy)
	return p.mergeErr
}
func (p *recordingPublisher) MergeState(ctx context.Context, pr int) string {
	p.did("merge-state %d", pr)
	return p.mergeState
}
func (p *recordingPublisher) CloseIssue(ctx context.Context, issue int) error {
	p.did("close %d", issue)
	return nil
}
func (p *recordingPublisher) Labels(ctx context.Context, issue int, add, remove []string) error {
	p.did("labels %d +%s -%s", issue, strings.Join(add, ","), strings.Join(remove, ","))
	return nil
}
func (p *recordingPublisher) Landed(stateRoot string, issue int) { p.did("landed %d", issue) }
func (p *recordingPublisher) Cleanup(stateRoot string, issue int) error {
	p.did("cleanup %d", issue)
	return nil
}

const publisherConfig = `version: "1.0"
project:
  name: rally
  type: single-repo
repos:
  - name: root
    path: ./
    type: root
    language: go
git:
  integration_branch: develop
  release_branch: main
review:
  human_feedback: %v
lessons:
  enabled: false
`

const scoreAssertion = `if got := Score(1, 2); got != "1-2" {`

// reviewCase sets up a project and a worktree a review can judge: a ticket
// with one criterion, a test that covers it, and a run of the tests that
// passed. GitHub's reads are stood in for.
func reviewCase(t *testing.T, humanFeedback bool, pub *recordingPublisher) SubmitReviewOptions {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ai", "config.yaml"), []byte(fmt.Sprintf(publisherConfig, humanFeedback)), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, "score_test.go"), []byte("package main\n\nfunc TestScore(t *testing.T) {\n\t"+scoreAssertion+"\n\t\tt.Fail()\n\t}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origDiff, origHead, origState := fetchPRDiffFunc, worktreeHeadFunc, fetchPRStateFunc
	fetchPRDiffFunc = func(ctx context.Context, pr int, timeout time.Duration) (string, error) {
		return "diff --git a/score.go b/score.go\n+func Score(a, b int) string\n", nil
	}
	worktreeHeadFunc = func(string) string { return "abc123" }
	fetchPRStateFunc = func(ctx context.Context, pr int, timeout time.Duration) (*prState, error) {
		return &prState{HeadSHA: "abc123"}, nil
	}
	t.Cleanup(func() { fetchPRDiffFunc, worktreeHeadFunc, fetchPRStateFunc = origDiff, origHead, origState })

	return SubmitReviewOptions{
		PRNumber: 30, IssueNumber: 12, Score: 9, CIStatus: "passed",
		StateRoot: root, WorktreePath: wt, TestCommand: "go test ./...", Language: "go",
		Ticket:         "# Add Score\n\n## Acceptance Criteria\n- [ ] Score returns a-b\n",
		ScoreThreshold: 7, MergeStrategy: "squash",
		Structured: &StructuredReview{
			Score: 9, Summary: "Score is done.",
			Criteria: []StructuredCriterion{{Criterion: "Score returns a-b", Implementation: "score.go:3 Score joins a and b with a dash", TestName: "TestScore", Assertion: scoreAssertion}},
		},
		ReviewBody: "Score is done.",
		Publisher:  pub,
		TestRun:    &TestRun{Command: "go test -v ./...", Output: "=== RUN   TestScore\n--- PASS: TestScore (0.00s)\nPASS\nok  \texample.com/rally\t0.01s\n"},
	}
}

func joined(p *recordingPublisher) string { return strings.Join(p.calls, "\n") }

// A review that approves, with the checks passed: the work is merged and the
// ticket closed, by whatever publisher acts on it, and the tests are not run
// again where they were run before.
func TestSubmitReview_Merges(t *testing.T) {
	pub := &recordingPublisher{}
	res, err := SubmitReview(context.Background(), reviewCase(t, false, pub))
	if err != nil || res.Result != "merged" || res.Next != NextMerged {
		t.Fatalf("result: %+v, %v\ncalls:\n%s", res, err, joined(pub))
	}
	want := []string{"comment-pr 30", "approve 30 9", "merge 30 squash", "landed 12", "close 12", "labels 12 + -pr-ready", "cleanup 12"}
	if strings.Join(pub.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", joined(pub), strings.Join(want, "\n"))
	}
}

func TestSubmitReview_TheWaysItDoesNotMerge(t *testing.T) {
	for name, c := range map[string]struct {
		change   func(*SubmitReviewOptions, *recordingPublisher)
		result   string
		next     string
		needsYou string
		calls    []string
	}{
		"the checks failed": {
			func(o *SubmitReviewOptions, p *recordingPublisher) { o.CIStatus = "failed" },
			"changes_requested", NextReviewRetry, "",
			[]string{"approve 30 9", "comment-issue 12 ## AWK Review 通過，但 CI 未通過", "labels 12 +review-failed -pr-ready"},
		},
		"the score is too low": {
			func(o *SubmitReviewOptions, p *recordingPublisher) {
				o.Score, o.Structured.Score = 4, 4
				o.Structured.Improvements = []StructuredImprovement{{Severity: "critical", Location: "score.go:3", Text: "Score ignores b"}}
			},
			"changes_requested", NextRework, "",
			[]string{"request-changes 30 4", "labels 12 +ai-task -pr-ready", "comment-issue 12 ## AWK Review 不通過 (score: 4/10)"},
		},
		"the evidence does not hold": {
			func(o *SubmitReviewOptions, p *recordingPublisher) { o.Structured.Criteria[0].TestName = "TestNothing" },
			"review_blocked", NextReviewRetry, "",
			[]string{"labels 12 +review-failed -pr-ready", "comment-issue 12 ## AWK Review blocked"},
		},
		"the pull request conflicts": {
			func(o *SubmitReviewOptions, p *recordingPublisher) {
				p.mergeErr, p.mergeState = errors.New("not mergeable"), "DIRTY"
			},
			"merge_failed", NextResolveConflict, "",
			[]string{"merge 30 squash", "merge-state 30", "labels 12 +merge-conflict -pr-ready", "comment-issue 12 ## AWK Review: Merge Conflict"},
		},
		"the pull request is behind": {
			func(o *SubmitReviewOptions, p *recordingPublisher) {
				p.mergeErr, p.mergeState = errors.New("behind"), "BEHIND"
			},
			"merge_failed", NextResolveRebase, "",
			[]string{"merge 30 squash", "merge-state 30", "labels 12 +needs-rebase -pr-ready", "comment-issue 12 ## AWK Review: Branch Behind"},
		},
		"GitHub refuses the merge": {
			func(o *SubmitReviewOptions, p *recordingPublisher) {
				p.mergeErr, p.mergeState = errors.New("protected"), "BLOCKED"
			},
			"merge_failed", NextNeedsYou, NeedsYouMergeBlocked,
			[]string{"merge 30 squash", "merge-state 30", "labels 12 +needs-human-review -pr-ready", "comment-issue 12 ## AWK Review: 合併失敗（需要人工介入）"},
		},
		// The review's comment would have put a credential on GitHub: the
		// guard kept it back, and a review with no record is not acted on.
		"the guard keeps the comment back": {
			func(o *SubmitReviewOptions, p *recordingPublisher) {
				p.commentErr = fmt.Errorf("%w: what is about to be written to GitHub holds what looks like a credential (github-token); it was not sent (outbound/github-token)", guard.ErrBlocked)
			},
			"review_blocked", NextNeedsYou, NeedsYouGuardOutput,
			[]string{"comment-pr 30", "labels 12 +needs-human-review -pr-ready", "comment-issue 12 ## AWK Review: 審查留言被擋下（需要人工介入）"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			pub := &recordingPublisher{}
			opts := reviewCase(t, false, pub)
			c.change(&opts, pub)
			res, err := SubmitReview(context.Background(), opts)
			if err != nil || res.Result != c.result || res.Next != c.next || res.NeedsYou != c.needsYou {
				t.Fatalf("result: %+v, %v\ncalls:\n%s", res, err, joined(pub))
			}
			got := joined(pub)
			at := 0
			for _, want := range c.calls {
				i := strings.Index(got[at:], want)
				if i < 0 {
					t.Fatalf("calls lack %q (in order):\n%s", want, got)
				}
				at += i
			}
			if strings.Contains(got, "close 12") || strings.Contains(got, "landed 12") {
				t.Errorf("a review that did not merge closed the ticket:\n%s", got)
			}
			if pub.commentErr != nil && (strings.Contains(got, "approve") || strings.Contains(got, "merge 30")) {
				t.Errorf("a review the guard held back was acted on:\n%s", got)
			}
		})
	}
}

// A maintainer's open feedback outranks an approving review: the
// implementer is sent back with it, or, when it pushed nothing for it, a
// person is asked.
func TestSubmitReview_TheMaintainersWord(t *testing.T) {
	open := humanFeedback{Ref: "review:7", Author: "owner", Commit: "abc123", Body: "Rename Score to MatchScore.", At: "2026-11-23T09:00:00Z"}

	pub := &recordingPublisher{}
	opts := reviewCase(t, true, pub)
	fetchPRStateFunc = func(ctx context.Context, pr int, timeout time.Duration) (*prState, error) {
		return &prState{HeadSHA: "abc123", Feedback: []humanFeedback{open}}, nil
	}
	res, err := SubmitReview(context.Background(), opts)
	if err != nil || res.Next != NextRework || strings.Contains(joined(pub), "merge 30") {
		t.Fatalf("result: %+v, %v\ncalls:\n%s", res, err, joined(pub))
	}
	if !strings.Contains(joined(pub), "labels 12 +ai-task -pr-ready") {
		t.Errorf("calls:\n%s", joined(pub))
	}

	// The same feedback, already handed over once: a person decides.
	pub2 := &recordingPublisher{}
	opts2 := opts
	opts2.Publisher = pub2
	res, err = SubmitReview(context.Background(), opts2)
	if err != nil || res.Next != NextNeedsYou || res.NeedsYou != NeedsYouMaintainer || !strings.Contains(joined(pub2), "labels 12 +needs-human-review -pr-ready") {
		t.Fatalf("result: %+v, %v\ncalls:\n%s", res, err, joined(pub2))
	}
}

// The tests are run when no run is given, as before.
func TestVerifyEvidence_RunsTheTestsWhenNotGiven(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on the PATH")
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, "score_test.go"), []byte(scoreAssertion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := VerifyOptions{
		Ticket: "- [ ] Score returns a-b\n", WorktreePath: wt, Language: "go",
		TestCommand:   `echo "--- PASS: TestScore (0.00s)"`,
		Verifications: []CriteriaVerification{{Criteria: "Score returns a-b", Implementation: "score.go:3 Score joins a and b with a dash", TestName: "TestScore", Assertion: scoreAssertion}},
	}
	if _, err := VerifyTestEvidenceWithReport(context.Background(), opts); err != nil {
		t.Errorf("run here: %+v", err)
	}
	opts.TestCommand = "exit 1"
	opts.TestRun = &TestRun{Command: "given", Output: "--- PASS: TestScore (0.00s)\n"}
	if _, err := VerifyTestEvidenceWithReport(context.Background(), opts); err != nil {
		t.Errorf("a given run is used, not the command: %+v", err)
	}
}
