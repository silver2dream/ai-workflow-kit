package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/task"
)

// SubmitReviewOptions configures the submit review operation
type SubmitReviewOptions struct {
	PRNumber     int
	IssueNumber  int
	Score        int    // 1-10
	CIStatus     string // "passed" | "failed"
	ReviewBody   string
	StateRoot    string
	WorktreePath string        // Path to worktree for test execution
	TestCommand  string        // Command to run tests
	Ticket       string        // Issue body with acceptance criteria
	GHTimeout    time.Duration
	TestTimeout  time.Duration
}

// SubmitReviewResult holds the result of submitting a review
type SubmitReviewResult struct {
	Result string // merged, approved_ci_failed, changes_requested, review_blocked, merge_failed
	Reason string
}

// SubmitReview submits a PR review and handles the result
func SubmitReview(ctx context.Context, opts SubmitReviewOptions) (*SubmitReviewResult, error) {
	if opts.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}
	if opts.TestTimeout == 0 {
		opts.TestTimeout = 5 * time.Minute
	}

	// Get session ID
	sessionMgr := session.NewManager(opts.StateRoot)
	sessionID := sessionMgr.GetCurrentSessionID()
	if sessionID == "" {
		sessionID = "unknown"
	}

	// Prepare review directory
	reviewDir := filepath.Join(opts.StateRoot, ".ai", "state", "reviews", fmt.Sprintf("pr-%d", opts.PRNumber))
	if err := os.MkdirAll(reviewDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create review directory: %w", err)
	}

	// Save review body
	reviewMDPath := filepath.Join(reviewDir, "review.md")
	_ = os.WriteFile(reviewMDPath, []byte(opts.ReviewBody), 0644)

	// Fetch ticket if not provided
	if opts.Ticket == "" {
		ticket, err := fetchIssueBody(ctx, opts.IssueNumber, opts.GHTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch ticket: %w", err)
		}
		opts.Ticket = ticket
	}

	// Get worktree path if not provided
	if opts.WorktreePath == "" {
		opts.WorktreePath = filepath.Join(opts.StateRoot, ".worktrees", fmt.Sprintf("issue-%d", opts.IssueNumber))
	}

	// Get test command if not provided
	if opts.TestCommand == "" {
		opts.TestCommand = getTestCommand(opts.StateRoot, opts.IssueNumber)
	}

	// Verify evidence using new test-based verification
	fmt.Printf("[REVIEW] Starting verification...\n")
	fmt.Printf("[REVIEW] Worktree: %s\n", opts.WorktreePath)
	fmt.Printf("[REVIEW] Test Command: %s\n", opts.TestCommand)

	verifyErr := VerifyTestEvidence(ctx, VerifyOptions{
		Ticket:       opts.Ticket,
		ReviewBody:   opts.ReviewBody,
		WorktreePath: opts.WorktreePath,
		TestCommand:  opts.TestCommand,
		TestTimeout:  opts.TestTimeout,
	})

	if verifyErr != nil {
		fmt.Printf("[REVIEW] ❌ Verification failed: %s\n", verifyErr.Message)
		if verifyErr.Details != nil {
			for _, d := range verifyErr.Details {
				fmt.Printf("[REVIEW]   - %s\n", d)
			}
		}
		return handleVerificationFailure(ctx, opts, sessionID, verifyErr)
	}

	fmt.Printf("[REVIEW] ✅ All verifications passed\n")

	// Count criteria for reporting
	criteria := ParseAcceptanceCriteria(opts.Ticket)
	criteriaCount := len(criteria)

	// Post AWK Review Comment
	timestamp := time.Now().UTC().Format(time.RFC3339)
	commentBody := fmt.Sprintf(`<!-- AWK:session:%s -->
🤖 **AWK Review**

| Field | Value |
|-------|-------|
| Reviewer Session | `+"`%s`"+` |
| Review Timestamp | %s |
| CI Status | %s |
| Criteria Verified | %d |
| Tests Executed | ✅ Passed |
| Assertions Verified | ✅ Found |
| Score | %d/10 |

%s`, sessionID, sessionID, timestamp, opts.CIStatus, criteriaCount, opts.Score, opts.ReviewBody)

	postPRComment(ctx, opts.PRNumber, commentBody, opts.GHTimeout)

	// Submit GitHub Review
	if opts.Score >= 7 {
		// Approve
		approvePR(ctx, opts.PRNumber, opts.Score, opts.GHTimeout)

		if opts.CIStatus == "passed" {
			// Try to merge
			if err := mergePR(ctx, opts.PRNumber, opts.GHTimeout); err != nil {
				return handleMergeFailure(ctx, opts, sessionID, err)
			}

			// Merge successful
			closeIssue(ctx, opts.IssueNumber, opts.GHTimeout)
			removeLabel(ctx, opts.IssueNumber, "pr-ready", opts.GHTimeout)
			updateTasksMd(ctx, opts.StateRoot, opts.IssueNumber)
			cleanupWorktree(opts.StateRoot, opts.IssueNumber)

			return &SubmitReviewResult{Result: "merged"}, nil
		}

		// CI failed
		postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review 通過，但 CI 失敗

審查評分: %d/10 ✅

%s

---
**CI 狀態**: ❌ 失敗

請檢查 CI 日誌並修復問題後重新提交。
PR: #%d`, opts.Score, opts.ReviewBody, opts.PRNumber), opts.GHTimeout)

		editIssueLabels(ctx, opts.IssueNumber, []string{"ai-task"}, []string{"pr-ready"}, opts.GHTimeout)

		return &SubmitReviewResult{Result: "approved_ci_failed"}, nil
	}

	// Request changes
	requestChangesPR(ctx, opts.PRNumber, opts.Score, opts.GHTimeout)
	editIssueLabels(ctx, opts.IssueNumber, []string{"ai-task"}, []string{"pr-ready"}, opts.GHTimeout)

	postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review 不通過 (score: %d/10)

%s

---
**Worker 請根據以上意見修改後重新提交。**
PR: #%d`, opts.Score, opts.ReviewBody, opts.PRNumber), opts.GHTimeout)

	return &SubmitReviewResult{Result: "changes_requested"}, nil
}

func handleVerificationFailure(ctx context.Context, opts SubmitReviewOptions, sessionID string, err *EvidenceError) (*SubmitReviewResult, error) {
	editIssueLabels(ctx, opts.IssueNumber, []string{"review-failed"}, []string{"pr-ready"}, opts.GHTimeout)

	var details string
	if err.Details != nil {
		details = "\n```\n"
		for _, d := range err.Details {
			details += "- " + d + "\n"
		}
		details += "```\n"
	}

	failureType := "verification"
	switch err.Code {
	case 1:
		failureType = "criteria/mapping"
	case 2:
		failureType = "test execution"
	case 3:
		failureType = "assertion"
	}

	postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review blocked

審查驗證失敗（%s）。

PR: #%d

錯誤: %s
%s
已標記 review-failed。下一個 session 的 subagent 將重新審查。
**當前 session 不應重試。**`, failureType, opts.PRNumber, err.Message, details), opts.GHTimeout)

	return &SubmitReviewResult{Result: "review_blocked", Reason: err.Message}, nil
}

func handleMergeFailure(ctx context.Context, opts SubmitReviewOptions, sessionID string, mergeErr error) (*SubmitReviewResult, error) {
	// Get merge state status
	mergeState := getMergeStateStatus(ctx, opts.PRNumber, opts.GHTimeout)

	var label, result, message string

	switch mergeState {
	case "DIRTY":
		label = "merge-conflict"
		result = "conflict_needs_fix"
		message = fmt.Sprintf(`## AWK Review: Merge Conflict

PR: #%d
mergeStateStatus: `+"`DIRTY`"+`

PR 有 merge conflict。Worker 將自動解決衝突後重新提交。`, opts.PRNumber)

	case "BEHIND":
		label = "needs-rebase"
		result = "behind_needs_rebase"
		message = fmt.Sprintf(`## AWK Review: Branch Behind

PR: #%d
mergeStateStatus: `+"`BEHIND`"+`

PR 分支落後 base branch。Worker 將自動 rebase 後重新提交。`, opts.PRNumber)

	default: // BLOCKED or other
		label = "needs-human-review"
		result = "merge_blocked"
		message = fmt.Sprintf(`## AWK Review: 合併失敗（需要人工介入）

PR: #%d
mergeStateStatus: `+"`%s`"+`

PR 被保護規則擋住或有其他問題，需要人工處理。`, opts.PRNumber, mergeState)
	}

	editIssueLabels(ctx, opts.IssueNumber, []string{label}, []string{"pr-ready"}, opts.GHTimeout)
	postIssueComment(ctx, opts.IssueNumber, message, opts.GHTimeout)

	return &SubmitReviewResult{Result: result, Reason: mergeState}, nil
}

// fetchIssueBody fetches the issue body from GitHub
func fetchIssueBody(ctx context.Context, issueNumber int, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "view", strconv.Itoa(issueNumber), "--json", "body", "--jq", ".body")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getTestCommand determines the test command based on config and repo
func getTestCommand(stateRoot string, issueNumber int) string {
	// Try to get from result file (has repo info)
	resultFile := filepath.Join(stateRoot, ".ai", "results", fmt.Sprintf("issue-%d.json", issueNumber))
	if data, err := os.ReadFile(resultFile); err == nil {
		var result struct {
			Repo string `json:"repo"`
		}
		if json.Unmarshal(data, &result) == nil && result.Repo != "" {
			// Try to get test command from workflow.yaml
			configFile := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
			if configData, err := os.ReadFile(configFile); err == nil {
				// Simple parsing - look for test_command under the repo
				lines := strings.Split(string(configData), "\n")
				inRepo := false
				for _, line := range lines {
					if strings.Contains(line, "name:") && strings.Contains(line, result.Repo) {
						inRepo = true
					} else if inRepo && strings.Contains(line, "test_command:") {
						parts := strings.SplitN(line, ":", 2)
						if len(parts) == 2 {
							cmd := strings.TrimSpace(parts[1])
							cmd = strings.Trim(cmd, `"'`)
							if cmd != "" {
								return cmd
							}
						}
					} else if inRepo && strings.HasPrefix(strings.TrimSpace(line), "- name:") {
						// Moved to next repo
						break
					}
				}
			}
		}
	}

	// Default to Go test command
	return "go test -v ./..."
}

// GitHub helper functions

func postPRComment(ctx context.Context, prNumber int, body string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "comment", strconv.Itoa(prNumber), "--body", body)
	_ = cmd.Run()
}

func postIssueComment(ctx context.Context, issueNumber int, body string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "comment", strconv.Itoa(issueNumber), "--body", body)
	_ = cmd.Run()
}

func approvePR(ctx context.Context, prNumber, score int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "review", strconv.Itoa(prNumber),
		"--approve", "--body", fmt.Sprintf("AWK Review: APPROVED (score: %d/10)", score))
	_ = cmd.Run()
}

func requestChangesPR(ctx context.Context, prNumber, score int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "review", strconv.Itoa(prNumber),
		"--request-changes", "--body", fmt.Sprintf("AWK Review: CHANGES REQUESTED (score: %d/10)", score))
	_ = cmd.Run()
}

func mergePR(ctx context.Context, prNumber int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "merge", strconv.Itoa(prNumber),
		"--squash", "--delete-branch")
	return cmd.Run()
}

func closeIssue(ctx context.Context, issueNumber int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "close", strconv.Itoa(issueNumber))
	_ = cmd.Run()
}

func removeLabel(ctx context.Context, issueNumber int, label string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "edit", strconv.Itoa(issueNumber),
		"--remove-label", label)
	_ = cmd.Run()
}

func editIssueLabels(ctx context.Context, issueNumber int, addLabels, removeLabels []string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"issue", "edit", strconv.Itoa(issueNumber)}
	for _, l := range addLabels {
		args = append(args, "--add-label", l)
	}
	for _, l := range removeLabels {
		args = append(args, "--remove-label", l)
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	_ = cmd.Run()
}

func getMergeStateStatus(ctx context.Context, prNumber int, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(prNumber),
		"--json", "mergeStateStatus", "--jq", ".mergeStateStatus")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func updateTasksMd(ctx context.Context, stateRoot string, issueNumber int) {
	resultFile := filepath.Join(stateRoot, ".ai", "results", fmt.Sprintf("issue-%d.json", issueNumber))
	data, err := os.ReadFile(resultFile)
	if err != nil {
		return
	}

	var result struct {
		SpecName string `json:"spec_name"`
		TaskLine int    `json:"task_line"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return
	}

	if result.SpecName == "" || result.TaskLine <= 0 {
		return
	}

	// Find tasks.md path
	configFile := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	specBasePath := ".ai/specs"
	if configData, err := os.ReadFile(configFile); err == nil {
		// Simple extraction without full YAML parsing
		for _, line := range strings.Split(string(configData), "\n") {
			if strings.Contains(line, "base_path:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					specBasePath = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	specNameClean := strings.ReplaceAll(result.SpecName, " ", "")
	tasksFile := filepath.Join(stateRoot, specBasePath, specNameClean, "tasks.md")

	content, err := os.ReadFile(tasksFile)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	if result.TaskLine <= len(lines) {
		lines[result.TaskLine-1] = strings.Replace(lines[result.TaskLine-1], "[ ]", "[x]", 1)
		if err := os.WriteFile(tasksFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return
		}

		// Commit tasks.md update (best-effort)
		if err := task.CommitTasksUpdate(tasksFile, issueNumber, "complete"); err != nil {
			// Log warning but continue
			fmt.Fprintf(os.Stderr, "warning: failed to commit tasks.md update: %v\n", err)
		}
	}
}

func cleanupWorktree(stateRoot string, issueNumber int) {
	wtDir := filepath.Join(stateRoot, ".worktrees", fmt.Sprintf("issue-%d", issueNumber))
	if info, err := os.Stat(wtDir); err == nil && info.IsDir() {
		cmd := exec.Command("git", "worktree", "remove", wtDir, "--force")
		_ = cmd.Run()
	}
}
