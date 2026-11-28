package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// What the engine's merge queue (design.md §5.4, task 2.3) takes from the
// review. The engine's review approves and does not merge
// (SubmitReviewOptions.Queue); the queue merges the pull request once it is
// checked again on the integration branch as it is then, and does what
// follows a merge as SubmitReview does after its own.

// Approval is what a review that approved said of the pull request, kept
// for the merge that follows it.
type Approval struct {
	PR    int `json:"pr"`
	Issue int `json:"issue"`
	// Score is the review's score, the one the decision was made on.
	Score int `json:"score"`
	// Head is the commit approved: the one the review's tests ran on.
	Head string    `json:"head,omitempty"`
	At   time.Time `json:"at"`
}

func approvalPath(stateRoot string, pr int) string {
	return filepath.Join(stateRoot, ".ai", "state", "reviews", fmt.Sprintf("pr-%d", pr), "approval.json")
}

// SaveApproval keeps a review's approval of a pull request, in place of
// the one before.
func SaveApproval(stateRoot string, a Approval) error {
	path := approvalPath(stateRoot, a.PR)
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadApproval is the last approval of a pull request; ok is false when it
// has none that can be read.
func LoadApproval(stateRoot string, pr int) (a Approval, ok bool) {
	data, err := os.ReadFile(approvalPath(stateRoot, pr))
	if err != nil || json.Unmarshal(data, &a) != nil || a.PR != pr {
		return Approval{}, false
	}
	return a, true
}

// MergedOptions is a pull request the merge queue merged, and what its
// review said of it.
type MergedOptions struct {
	StateRoot   string
	PRNumber    int
	IssueNumber int
	// Score and Head are the approval's: the review's score, and the commit
	// it approved.
	Score int
	Head  string
	// Publisher does what follows on GitHub and in the project; nil is the
	// Principal's (as SubmitReviewOptions has it).
	Publisher  Publisher
	HookRunner HookFirer
	GHTimeout  time.Duration
}

func (o MergedOptions) submit() SubmitReviewOptions {
	opts := SubmitReviewOptions{
		StateRoot: o.StateRoot, PRNumber: o.PRNumber, IssueNumber: o.IssueNumber,
		Score: o.Score, Publisher: o.Publisher, HookRunner: o.HookRunner, GHTimeout: o.GHTimeout,
		headSHA: o.Head,
	}
	opts.fillDefaults()
	return opts
}

// AfterMerge does what follows the merge of a pull request, as SubmitReview
// does after the merges it makes: the on_merge hooks, the work recorded as
// landed, the issue closed, the worktree removed, and the learning loop
// closed on the approval.
func AfterMerge(ctx context.Context, o MergedOptions) {
	opts := o.submit()
	ctx = guard.WithScope(ctx, guard.Scope{StateRoot: opts.StateRoot, Issue: opts.IssueNumber})
	followMerge(ctx, opts)
}

// followMerge is what follows a merge. Nothing of it can undo the merge: what
// fails is said, and the rest is done.
func followMerge(ctx context.Context, opts SubmitReviewOptions) {
	// Fire on_merge hooks (non-blocking)
	if opts.HookRunner != nil {
		hookEnv := map[string]string{
			"AWK_PR":    fmt.Sprintf("%d", opts.PRNumber),
			"AWK_ISSUE": fmt.Sprintf("%d", opts.IssueNumber),
		}
		_ = opts.HookRunner.Fire(ctx, "on_merge", hookEnv)
	}

	// The work landed: the decision engine reads this instead of asking
	// GitHub whether the closed issue has a merged pull request.
	opts.publisher().Landed(opts.StateRoot, opts.IssueNumber)

	// Merge successful - cleanup operations (non-critical, log warnings)
	if err := opts.publisher().CloseIssue(ctx, opts.IssueNumber); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to close issue #%d: %v\n", opts.IssueNumber, err)
	}
	if err := opts.publisher().Labels(ctx, opts.IssueNumber, nil, []string{"pr-ready"}); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to remove pr-ready label from issue #%d: %v\n", opts.IssueNumber, err)
	}
	if err := opts.publisher().Cleanup(opts.StateRoot, opts.IssueNumber); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: %v\n", err)
	}

	// Learning loop: record the approval, settle injected lessons,
	// and distill the rejections this merge resolved.
	closeLearningLoop(ctx, opts, learningOutcome{Outcome: lessons.OutcomeMerged})
}

// HumanGate holds an approved pull request on the maintainers' review, as
// SubmitReview holds one before it merges: nil lets it merge. Otherwise the
// result says where the ticket goes (NextRework with the feedback, or
// NextNeedsYou), and the gate has said so on the issue. The queue asks it
// again just before it merges: a maintainer may have written since the
// review.
func HumanGate(ctx context.Context, o MergedOptions) *SubmitReviewResult {
	opts := o.submit()
	ctx = guard.WithScope(ctx, guard.Scope{StateRoot: opts.StateRoot, Issue: opts.IssueNumber})
	// An approval is past the threshold whatever its score.
	opts.ScoreThreshold = 1
	if opts.Score < 1 {
		opts.Score = 1
	}
	_, held := applyHumanGate(ctx, opts)
	return held
}

// RepoTestCommand is the test command the review runs in a worktree, and
// the language of the repo it is of: "" when the configuration has none.
func RepoTestCommand(stateRoot, worktreePath string) (command, language string) {
	s := getRepoSettingsFromConfig(stateRoot, worktreePath)
	return s.TestCommand, s.Language
}

// MergeAt merges a pull request in the project's way ("squash", "merge",
// "rebase"), and deletes its branch, only while head is its head: a commit
// pushed since is not merged unchecked.
func MergeAt(ctx context.Context, pr int, strategy, head string, timeout time.Duration) error {
	if head == "" {
		return fmt.Errorf("pull request #%d: the commit to merge is not known", pr)
	}
	return mergePRAt(ctx, pr, timeout, strategy, head)
}

// MergeState is why GitHub would not merge a pull request: DIRTY, BEHIND,
// BLOCKED…; "unknown" when it cannot be asked.
func MergeState(ctx context.Context, pr int, timeout time.Duration) string {
	return getMergeStateStatus(ctx, pr, timeout)
}
