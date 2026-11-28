package reviewer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

// For the merge queue: a review that approves approves on GitHub, keeps the
// approval, and merges nothing; what follows a merge is left to the queue.
func TestSubmitReview_ForTheMergeQueue(t *testing.T) {
	pub := &recordingPublisher{}
	opts := reviewCase(t, false, pub)
	opts.Queue = true
	res, err := SubmitReview(context.Background(), opts)
	if err != nil || res.Result != "approved" || res.Next != NextQueued || res.Head != "abc123" {
		t.Fatalf("result: %+v, %v\ncalls:\n%s", res, err, joined(pub))
	}
	if got := joined(pub); got != "comment-pr 30\napprove 30 9" {
		t.Errorf("calls:\n%s", got)
	}
	a, ok := LoadApproval(opts.StateRoot, 30)
	if !ok || a.PR != 30 || a.Issue != 12 || a.Score != 9 || a.Head != "abc123" || a.At.IsZero() {
		t.Errorf("approval: %+v, %v", a, ok)
	}
	if entries, _ := LoadFeedback(opts.StateRoot); len(entries) != 0 {
		t.Errorf("the learning loop closed on an approval that did not merge: %+v", entries)
	}

	// What does not pass is as before: nothing is kept.
	pub2 := &recordingPublisher{}
	opts2 := reviewCase(t, false, pub2)
	opts2.Queue, opts2.CIStatus = true, "failed"
	res, err = SubmitReview(context.Background(), opts2)
	if err != nil || res.Next != NextReviewRetry {
		t.Fatalf("checks failed: %+v, %v", res, err)
	}
	if _, ok := LoadApproval(opts2.StateRoot, 30); ok {
		t.Error("an approval was kept for checks that failed")
	}
}

// An approval is read back for its pull request only.
func TestLoadApproval(t *testing.T) {
	root := t.TempDir()
	if _, ok := LoadApproval(root, 30); ok {
		t.Error("an approval that is not there was read")
	}
	if err := SaveApproval(root, Approval{PR: 30, Issue: 12, Score: 8, Head: "abc", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if a, ok := LoadApproval(root, 30); !ok || a.Score != 8 || a.Head != "abc" {
		t.Errorf("approval: %+v, %v", a, ok)
	}
	if err := SaveApproval(root, Approval{PR: 30, Issue: 12, Score: 9, Head: "def"}); err != nil {
		t.Fatal(err)
	}
	if a, _ := LoadApproval(root, 30); a.Score != 9 || a.Head != "def" {
		t.Errorf("the later approval: %+v", a)
	}
	if err := os.MkdirAll(filepath.Dir(approvalPath(root, 31)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(approvalPath(root, 31), []byte(`{"pr": 30}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadApproval(root, 31); ok {
		t.Error("another pull request's approval was read")
	}
}

// What follows the queue's merge is what follows the review's own: the
// work landed, the ticket closed, the worktree removed, and the learning
// loop closed on the commit approved, with the review's score.
func TestAfterMerge(t *testing.T) {
	pub := &recordingPublisher{}
	opts := reviewCase(t, false, pub)
	hooks := &firedHooks{}
	AfterMerge(context.Background(), MergedOptions{
		StateRoot: opts.StateRoot, PRNumber: 30, IssueNumber: 12, Score: 9, Head: "def456",
		Publisher: pub, HookRunner: hooks,
	})
	if got := joined(pub); got != "landed 12\nclose 12\nlabels 12 + -pr-ready\ncleanup 12" {
		t.Errorf("calls:\n%s", got)
	}
	if strings.Join(hooks.events, ",") != "on_merge" {
		t.Errorf("hooks: %v", hooks.events)
	}
	entries, err := LoadFeedback(opts.StateRoot)
	if err != nil || len(entries) != 1 || entries[0].Outcome != outcomeApproved || entries[0].Score != 9 || entries[0].HeadSHA != "def456" || entries[0].PRNumber != 30 {
		t.Errorf("feedback: %+v, %v", entries, err)
	}
}

type firedHooks struct{ events []string }

func (h *firedHooks) Fire(_ context.Context, event string, _ map[string]string) error {
	h.events = append(h.events, event)
	return nil
}

// The maintainers' gate, asked by the queue: open feedback holds the pull
// request as the review would; none lets it merge.
func TestHumanGate(t *testing.T) {
	open := humanFeedback{Ref: "review:7", Author: "owner", Commit: "abc123", Body: "Rename Score to MatchScore.", At: "2026-11-23T09:00:00Z"}
	pub := &recordingPublisher{}
	opts := reviewCase(t, true, pub)
	o := MergedOptions{StateRoot: opts.StateRoot, PRNumber: 30, IssueNumber: 12, Publisher: pub}

	if held := HumanGate(context.Background(), o); held != nil {
		t.Fatalf("held without feedback: %+v", held)
	}
	fetchPRStateFunc = func(ctx context.Context, pr int, timeout time.Duration) (*prState, error) {
		return &prState{HeadSHA: "abc123", Feedback: []humanFeedback{open}}, nil
	}
	// A score the approval did not keep (0) is an approval all the same.
	held := HumanGate(context.Background(), o)
	if held == nil || held.Next != NextRework || !strings.Contains(joined(pub), "comment-issue 12 ## AWK Review: 維護者的審查意見待處理") {
		t.Fatalf("held: %+v\ncalls:\n%s", held, joined(pub))
	}
	// Handed over once already, with nothing pushed: a person.
	if held := HumanGate(context.Background(), o); held == nil || held.Next != NextNeedsYou || held.NeedsYou != NeedsYouMaintainer {
		t.Errorf("held again: %+v", held)
	}
}

// The queue merges the commit it checked, and only it.
func TestMergeAt(t *testing.T) {
	orig := runGHMerge
	t.Cleanup(func() { runGHMerge = orig })
	var said []string
	runGHMerge = func(ctx context.Context, cfg ghutil.RetryConfig, name string, args ...string) ([]byte, error) {
		said = append(said, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	if err := MergeAt(context.Background(), 30, "rebase", "abc123", time.Second); err != nil {
		t.Fatal(err)
	}
	if err := MergeAt(context.Background(), 31, "", "def456", time.Second); err != nil {
		t.Fatal(err)
	}
	want := "gh pr merge 30 --rebase --delete-branch --match-head-commit abc123\ngh pr merge 31 --squash --delete-branch --match-head-commit def456"
	if got := strings.Join(said, "\n"); got != want {
		t.Errorf("gh:\n%s\nwant:\n%s", got, want)
	}
	if err := MergeAt(context.Background(), 32, "squash", "", time.Second); err == nil || len(said) != 2 {
		t.Errorf("a merge of no commit: %v, %v", err, said)
	}

	// The review's own merge names no commit, as before.
	said = nil
	if err := mergePR(context.Background(), 33, time.Second, "merge"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(said, "") != "gh pr merge 33 --merge --delete-branch" {
		t.Errorf("gh: %v", said)
	}
	runGHMerge = func(context.Context, ghutil.RetryConfig, string, ...string) ([]byte, error) {
		return nil, errors.New("refused")
	}
	if err := MergeAt(context.Background(), 34, "squash", "abc", time.Second); err == nil {
		t.Error("a refusal was not said")
	}
}
