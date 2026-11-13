// Package analyzer implements workflow decision logic
package analyzer

import "fmt"

// Decision represents the next action to take
type Decision struct {
	NextAction  string `json:"next_action"` // dispatch_worker | check_result | review_pr | all_complete | none
	IssueNumber int    `json:"issue_number"`
	PRNumber    int    `json:"pr_number"`
	ExitReason  string `json:"exit_reason"` // why the action is none; see the Reason constants
	MergeIssue  string `json:"merge_issue"` // conflict | rebase - indicates Worker needs to fix merge issues
}

// Action constants
const (
	ActionDispatchWorker = "dispatch_worker"
	ActionCheckResult    = "check_result"
	ActionReviewPR       = "review_pr"
	ActionAllComplete    = "all_complete"
	ActionNone           = "none"
)

// Exit reason constants
const (
	ReasonWorkerFailed           = "worker_failed"
	ReasonNeedsHumanReview       = "needs_human_review"
	ReasonReviewMaxRetries       = "review_max_retries"
	ReasonMaxLoopReached         = "max_loop_reached"
	ReasonMaxConsecutiveFailures = "max_consecutive_failures"
	ReasonNoActionableTasks      = "no_actionable_tasks"
	ReasonConfigNotFound         = "config_not_found"
	// ReasonConfigInvalid: the configuration is there and cannot be used. What
	// is wrong with it is on standard error, and `awkit validate` says it
	// again.
	ReasonConfigInvalid  = "config_invalid"
	ReasonLoopCountError = "loop_count_error"
	ReasonGitHubAPIError = "github_api_error"
	// ReasonInvalidTicket: a ticket's awk-ticket block cannot be read. The
	// issue carries a comment that says why.
	ReasonInvalidTicket = "invalid_ticket"
	// ReasonBlockedByDependencies: every pending ticket waits for an issue
	// that is still open, or that was closed as not planned.
	ReasonBlockedByDependencies = "blocked_by_dependencies"
)

// Merge issue constants
const (
	MergeIssueConflict = "conflict"
	MergeIssueRebase   = "rebase"
)

// FormatBashOutput formats the decision for bash eval
func (d *Decision) FormatBashOutput() string {
	return fmt.Sprintf(`NEXT_ACTION=%s
ISSUE_NUMBER=%d
PR_NUMBER=%d
EXIT_REASON=%s
MERGE_ISSUE=%s
`, d.NextAction, d.IssueNumber, d.PRNumber, d.ExitReason, d.MergeIssue)
}
