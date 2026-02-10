// Package analyzer implements workflow decision logic
package analyzer

import (
	"fmt"

	"github.com/silver2dream/ai-workflow-kit/internal/util"
)

// Decision represents the next action to take
type Decision struct {
	NextAction  string `json:"next_action"`  // generate_tasks | create_task | dispatch_worker | check_result | review_pr | all_complete | none
	IssueNumber int    `json:"issue_number"`
	PRNumber    int    `json:"pr_number"`
	SpecName    string `json:"spec_name"`
	TaskLine    int    `json:"task_line"`
	ExitReason  string `json:"exit_reason"`  // worker_failed | needs_human_review | max_loop_reached | max_consecutive_failures | no_actionable_tasks | config_not_found
	MergeIssue  string `json:"merge_issue"`  // conflict | rebase - indicates Worker needs to fix merge issues
	EpicIssue   int    `json:"epic_issue"`   // tracking issue number (epic mode)
	TaskText    string `json:"task_text"`    // task text from epic body (epic mode)
}

// Action constants
const (
	ActionGenerateTasks  = "generate_tasks"
	ActionCreateTask     = "create_task"
	ActionDispatchWorker = "dispatch_worker"
	ActionCheckResult    = "check_result"
	ActionReviewPR       = "review_pr"
	ActionAllComplete    = "all_complete"
	ActionNone           = "none"
)

// Exit reason constants
const (
	ReasonWorkerFailed             = "worker_failed"
	ReasonNeedsHumanReview         = "needs_human_review"
	ReasonReviewMaxRetries         = "review_max_retries"
	ReasonMaxLoopReached           = "max_loop_reached"
	ReasonMaxConsecutiveFailures   = "max_consecutive_failures"
	ReasonNoActionableTasks        = "no_actionable_tasks"
	ReasonConfigNotFound           = "config_not_found"
	ReasonLoopCountError           = "loop_count_error"
	ReasonGitHubAPIError           = "github_api_error"
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
SPEC_NAME=%s
TASK_LINE=%d
EXIT_REASON=%s
MERGE_ISSUE=%s
EPIC_ISSUE=%d
TASK_TEXT=%s
`, d.NextAction, d.IssueNumber, d.PRNumber, util.ShellSafe(d.SpecName), d.TaskLine, d.ExitReason, d.MergeIssue, d.EpicIssue, util.ShellSafe(d.TaskText))
}
