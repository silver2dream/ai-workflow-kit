package reviewer

import (
	"context"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// A review decides, then acts: it comments, approves or asks for changes,
// merges, moves the ticket on. What it does outside itself goes through a
// Publisher, so that the decision is the same whoever acts on it: the
// Principal's workflow, which keeps a ticket's state in its labels
// (ghPublisher), or the engine, which keeps it in its lanes and shows it as
// labels itself (task 1.4).

// Publisher is what a review does to GitHub and to the project.
type Publisher interface {
	// Ticket is the body of the issue under review.
	Ticket(ctx context.Context, issue int) (string, error)
	CommentPR(ctx context.Context, pr int, body string) error
	CommentIssue(ctx context.Context, issue int, body string) error
	Approve(ctx context.Context, pr, score int) error
	RequestChanges(ctx context.Context, pr, score int) error
	// Merge merges the pull request in the project's way ("squash",
	// "merge", "rebase").
	Merge(ctx context.Context, pr int, strategy string) error
	// MergeState is why GitHub would not merge: DIRTY, BEHIND, BLOCKED…
	MergeState(ctx context.Context, pr int) string
	CloseIssue(ctx context.Context, issue int) error
	// Labels moves a ticket on in the Principal's workflow, which keeps its
	// state in labels. The engine keeps it in its lanes: its Publisher
	// changes no label here.
	Labels(ctx context.Context, issue int, add, remove []string) error
	// Landed records that a ticket's work is on the integration branch.
	Landed(stateRoot string, issue int)
	// Cleanup removes the ticket's worktree once its work landed.
	Cleanup(stateRoot string, issue int) error
}

// ghPublisher is the Principal's workflow's: the gh CLI, the labels, the
// worktrees under .worktrees.
type ghPublisher struct{ timeout time.Duration }

func (p ghPublisher) Ticket(ctx context.Context, issue int) (string, error) {
	return fetchIssueBody(ctx, issue, p.timeout)
}
func (p ghPublisher) CommentPR(ctx context.Context, pr int, body string) error {
	return postPRComment(ctx, pr, body, p.timeout)
}
func (p ghPublisher) CommentIssue(ctx context.Context, issue int, body string) error {
	return commentIssueFunc(ctx, issue, body, p.timeout)
}
func (p ghPublisher) Approve(ctx context.Context, pr, score int) error {
	return approvePR(ctx, pr, score, p.timeout)
}
func (p ghPublisher) RequestChanges(ctx context.Context, pr, score int) error {
	return requestChangesPR(ctx, pr, score, p.timeout)
}
func (p ghPublisher) Merge(ctx context.Context, pr int, strategy string) error {
	return mergePR(ctx, pr, p.timeout, strategy)
}
func (p ghPublisher) MergeState(ctx context.Context, pr int) string {
	return getMergeStateStatus(ctx, pr, p.timeout)
}
func (p ghPublisher) CloseIssue(ctx context.Context, issue int) error {
	return closeIssue(ctx, issue, p.timeout)
}
func (p ghPublisher) Labels(ctx context.Context, issue int, add, remove []string) error {
	return labelIssueFunc(ctx, issue, add, remove, p.timeout)
}
func (p ghPublisher) Landed(stateRoot string, issue int) { analyzer.MarkLanded(stateRoot, issue) }
func (p ghPublisher) Cleanup(stateRoot string, issue int) error {
	return cleanupWorktree(stateRoot, issue)
}

// publisher is the review's Publisher: the one given, else the Principal's.
func (opts SubmitReviewOptions) publisher() Publisher {
	if opts.Publisher != nil {
		return opts.Publisher
	}
	return ghPublisher{timeout: opts.GHTimeout}
}

// What a ticket does after its review: what the result says to the one who
// moves the ticket on, in the place of the labels the Principal's workflow
// reads.
const (
	// NextMerged: the work landed.
	NextMerged = "merged"
	// NextRework: the implementer does it again, with the review (or a
	// maintainer's open feedback).
	NextRework = "rework"
	// NextReviewRetry: the review could not be completed (its evidence, the
	// checks, a JiT test); it is tried again.
	NextReviewRetry = "review_retry"
	// NextResolveConflict and NextResolveRebase: the pull request conflicts
	// with its base, or is behind it.
	NextResolveConflict = "resolve_conflict"
	NextResolveRebase   = "resolve_rebase"
	// NextNeedsYou: only a person can say what comes next.
	NextNeedsYou = "needs_you"
)
