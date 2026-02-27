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

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/task"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"

	"gopkg.in/yaml.v3"
)

// HookFirer is the interface for firing lifecycle hooks.
type HookFirer interface {
	Fire(ctx context.Context, event string, envVars map[string]string) error
}

// SubmitReviewOptions configures the submit review operation
type SubmitReviewOptions struct {
	PRNumber       int
	IssueNumber    int
	Score          int    // 1-10
	CIStatus       string // "passed" | "failed"
	ReviewBody     string
	StateRoot      string
	WorktreePath   string        // Path to worktree for test execution
	TestCommand    string        // Command to run tests
	Language       string        // Programming language for test name validation
	Ticket         string        // Issue body with acceptance criteria
	ScoreThreshold int           // Minimum score to approve (default: 7, from config)
	MergeStrategy  string        // Merge strategy: "squash" | "merge" | "rebase" (default: "squash", from config)
	GHTimeout      time.Duration
	TestTimeout    time.Duration
	HookRunner     HookFirer     // nil = no hooks
}

// SubmitReviewResult holds the result of submitting a review
type SubmitReviewResult struct {
	Result string // merged, changes_requested, review_blocked, merge_failed
	Reason string
}

// SubmitReview submits a PR review and handles the result
func SubmitReview(ctx context.Context, opts SubmitReviewOptions) (result *SubmitReviewResult, err error) {
	if opts.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}
	if opts.TestTimeout == 0 {
		opts.TestTimeout = 5 * time.Minute
	}
	if opts.ScoreThreshold <= 0 {
		opts.ScoreThreshold = 7
	}
	if opts.MergeStrategy == "" {
		opts.MergeStrategy = "squash"
	}

	// Write review_start event
	trace.WriteEvent(trace.ComponentReviewer, trace.TypeReviewStart, trace.LevelInfo,
		trace.WithPR(opts.PRNumber),
		trace.WithIssue(opts.IssueNumber),
		trace.WithData(map[string]any{
			"score":     opts.Score,
			"ci_status": opts.CIStatus,
		}))

	// Write review_decision event on function return
	defer func() {
		decision := "unknown"
		reason := ""
		if result != nil {
			decision = result.Result
			reason = result.Reason
		}
		level := trace.LevelInfo
		if decision == "changes_requested" || decision == "review_blocked" {
			level = trace.LevelWarn
		}
		if err != nil {
			level = trace.LevelError
		}

		// Build conditions map with optional reason
		conditions := map[string]any{
			"score":     opts.Score,
			"ci_status": opts.CIStatus,
			"pr_number": opts.PRNumber,
		}
		if reason != "" {
			conditions["reason"] = reason
		}

		trace.WriteDecisionEvent(trace.ComponentReviewer, trace.TypeReviewDecision, trace.Decision{
			Rule:       "review score and CI status determines merge decision",
			Conditions: conditions,
			Result:     decision,
		}, trace.WithPR(opts.PRNumber), trace.WithIssue(opts.IssueNumber))

		// Also write review_end event with reason if present
		endData := map[string]any{"result": decision}
		if reason != "" {
			endData["reason"] = reason
		}
		trace.WriteEvent(trace.ComponentReviewer, trace.TypeReviewEnd, level,
			trace.WithPR(opts.PRNumber),
			trace.WithIssue(opts.IssueNumber),
			trace.WithData(endData))

		// Fire post_review hooks (non-blocking, best-effort)
		if opts.HookRunner != nil {
			hookEnv := map[string]string{
				"AWK_PR":       fmt.Sprintf("%d", opts.PRNumber),
				"AWK_ISSUE":    fmt.Sprintf("%d", opts.IssueNumber),
				"AWK_SCORE":    fmt.Sprintf("%d", opts.Score),
				"AWK_DECISION": decision,
			}
			_ = opts.HookRunner.Fire(ctx, "post_review", hookEnv)
		}
	}()

	// Fire pre_review hooks
	if opts.HookRunner != nil {
		hookEnv := map[string]string{
			"AWK_PR":    fmt.Sprintf("%d", opts.PRNumber),
			"AWK_ISSUE": fmt.Sprintf("%d", opts.IssueNumber),
		}
		if err := opts.HookRunner.Fire(ctx, "pre_review", hookEnv); err != nil {
			return nil, fmt.Errorf("pre_review hook aborted: %w", err)
		}
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

	// Get test command and language if not provided
	if opts.TestCommand == "" || opts.Language == "" {
		settings := getRepoSettingsFromConfig(opts.StateRoot, opts.WorktreePath)
		if opts.TestCommand == "" {
			opts.TestCommand = settings.TestCommand
		}
		if opts.Language == "" {
			opts.Language = settings.Language
		}
	}

	// Verify evidence using new test-based verification
	fmt.Printf("[REVIEW] Starting verification...\n")
	fmt.Printf("[REVIEW] Worktree: %s\n", opts.WorktreePath)
	fmt.Printf("[REVIEW] Test Command: %s\n", opts.TestCommand)
	fmt.Printf("[REVIEW] Language: %s\n", opts.Language)

	verifyErr := VerifyTestEvidence(ctx, VerifyOptions{
		Ticket:       opts.Ticket,
		ReviewBody:   opts.ReviewBody,
		WorktreePath: opts.WorktreePath,
		TestCommand:  opts.TestCommand,
		TestTimeout:  opts.TestTimeout,
		Language:     opts.Language,
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

	// Post PR comment (non-critical, log warning if failed)
	if err := postPRComment(ctx, opts.PRNumber, commentBody, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post PR comment: %v\n", err)
	}

	// Submit GitHub Review
	if opts.Score >= opts.ScoreThreshold {
		// Approve (critical operation - must succeed)
		if err := approvePR(ctx, opts.PRNumber, opts.Score, opts.GHTimeout); err != nil {
			return nil, fmt.Errorf("failed to approve PR #%d: %w", opts.PRNumber, err)
		}

		if opts.CIStatus == "passed" {
			// Try to merge
			if err := mergePR(ctx, opts.PRNumber, opts.GHTimeout, opts.MergeStrategy); err != nil {
				return handleMergeFailure(ctx, opts, sessionID, err)
			}

			// Fire on_merge hooks (non-blocking)
			if opts.HookRunner != nil {
				hookEnv := map[string]string{
					"AWK_PR":    fmt.Sprintf("%d", opts.PRNumber),
					"AWK_ISSUE": fmt.Sprintf("%d", opts.IssueNumber),
				}
				_ = opts.HookRunner.Fire(ctx, "on_merge", hookEnv)
			}

			// Merge successful - cleanup operations (non-critical, log warnings)
			if err := closeIssue(ctx, opts.IssueNumber, opts.GHTimeout); err != nil {
				fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to close issue #%d: %v\n", opts.IssueNumber, err)
			}
			if err := removeLabel(ctx, opts.IssueNumber, "pr-ready", opts.GHTimeout); err != nil {
				fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to remove pr-ready label from issue #%d: %v\n", opts.IssueNumber, err)
			}
			if !isEpicMode(opts.StateRoot) {
				updateTasksMd(ctx, opts.StateRoot, opts.IssueNumber)
			} else {
				fmt.Fprintf(os.Stderr, "[REVIEW] epic mode: skipping tasks.md update (GitHub handles checkbox state)\n")
			}
			if err := cleanupWorktree(opts.StateRoot, opts.IssueNumber); err != nil {
				fmt.Fprintf(os.Stderr, "[REVIEW] warning: %v\n", err)
			}

			return &SubmitReviewResult{Result: "merged"}, nil
		}

		// CI not passed - treat as changes_requested so Worker fixes CI issues
		// Use review-failed label (not ai-task) to avoid infinite dispatch loop
		ciReason := "CI failed"
		ciStatusDisplay := "❌ 失敗"
		if opts.CIStatus == "unknown" {
			ciReason = "CI status unknown"
			ciStatusDisplay = "⚠️ 無法確認（未自動合併以確保安全）"
		}

		if err := postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review 通過，但 CI 未通過

審查評分: %d/10 ✅

%s

---
**CI 狀態**: %s

請檢查 CI 日誌並修復問題後重新提交。
PR: #%d`, opts.Score, opts.ReviewBody, ciStatusDisplay, opts.PRNumber), opts.GHTimeout); err != nil {
			fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post issue comment: %v\n", err)
		}

		if err := editIssueLabels(ctx, opts.IssueNumber, []string{"review-failed"}, []string{"pr-ready"}, opts.GHTimeout); err != nil {
			fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to edit issue labels: %v\n", err)
		}

		// Record feedback for CI failure rejection (fire-and-forget)
		_ = RecordFeedback(opts.StateRoot, FeedbackEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			IssueID:    opts.IssueNumber,
			PRNumber:   opts.PRNumber,
			Score:      opts.Score,
			Categories: []string{"ci-failure"},
			Summary:    truncateSummary(ciReason, 500),
		})

		return &SubmitReviewResult{Result: "changes_requested", Reason: ciReason}, nil
	}

	// Request changes (critical operation - must succeed)
	if err := requestChangesPR(ctx, opts.PRNumber, opts.Score, opts.GHTimeout); err != nil {
		return nil, fmt.Errorf("failed to request changes on PR #%d: %w", opts.PRNumber, err)
	}
	if err := editIssueLabels(ctx, opts.IssueNumber, []string{"ai-task"}, []string{"pr-ready"}, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to edit issue labels: %v\n", err)
	}

	if err := postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review 不通過 (score: %d/10)

%s

---
**Worker 請根據以上意見修改後重新提交。**
PR: #%d`, opts.Score, opts.ReviewBody, opts.PRNumber), opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post issue comment: %v\n", err)
	}

	// Record feedback for score-based rejection (fire-and-forget)
	_ = RecordFeedback(opts.StateRoot, BuildFeedbackEntry(opts.IssueNumber, opts.PRNumber, opts.Score, opts.ReviewBody))

	return &SubmitReviewResult{Result: "changes_requested"}, nil
}

func handleVerificationFailure(ctx context.Context, opts SubmitReviewOptions, sessionID string, err *EvidenceError) (*SubmitReviewResult, error) {
	if labelErr := editIssueLabels(ctx, opts.IssueNumber, []string{"review-failed"}, []string{"pr-ready"}, opts.GHTimeout); labelErr != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to edit issue labels: %v\n", labelErr)
	}

	// Record feedback for verification failure (fire-and-forget)
	verifyCategory := "verification"
	switch err.Code {
	case 1:
		verifyCategory = "criteria-mapping"
	case 2:
		verifyCategory = "test-execution"
	case 3:
		verifyCategory = "assertion"
	}
	_ = RecordFeedback(opts.StateRoot, FeedbackEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		IssueID:    opts.IssueNumber,
		PRNumber:   opts.PRNumber,
		Score:      opts.Score,
		Categories: []string{verifyCategory},
		Summary:    truncateSummary(err.Message, 500),
	})

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

	if commentErr := postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review blocked

審查驗證失敗（%s）。

PR: #%d

錯誤: %s
%s
已標記 review-failed。下一個 session 的 subagent 將重新審查。
**當前 session 不應重試。**`, failureType, opts.PRNumber, err.Message, details), opts.GHTimeout); commentErr != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post issue comment: %v\n", commentErr)
	}

	// Build detailed reason including failed tests
	reason := err.Message
	if len(err.Details) > 0 {
		reason += ": " + strings.Join(err.Details, ", ")
	}

	return &SubmitReviewResult{Result: "review_blocked", Reason: reason}, nil
}

func handleMergeFailure(ctx context.Context, opts SubmitReviewOptions, sessionID string, mergeErr error) (*SubmitReviewResult, error) {
	// Get merge state status
	mergeState := getMergeStateStatus(ctx, opts.PRNumber, opts.GHTimeout)

	// Log the original merge error for debugging
	fmt.Fprintf(os.Stderr, "[REVIEW] merge failed: err=%v, mergeStateStatus=%s, PR=#%d\n", mergeErr, mergeState, opts.PRNumber)

	var label, result, message string

	switch mergeState {
	case "DIRTY":
		label = "merge-conflict"
		result = "merge_failed"
		message = fmt.Sprintf(`## AWK Review: Merge Conflict

PR: #%d
mergeStateStatus: `+"`DIRTY`"+`

PR 有 merge conflict。Worker 將自動解決衝突後重新提交。`, opts.PRNumber)

	case "BEHIND":
		label = "needs-rebase"
		result = "merge_failed"
		message = fmt.Sprintf(`## AWK Review: Branch Behind

PR: #%d
mergeStateStatus: `+"`BEHIND`"+`

PR 分支落後 base branch。Worker 將自動 rebase 後重新提交。`, opts.PRNumber)

	default: // BLOCKED or other
		label = "needs-human-review"
		result = "merge_failed"
		message = fmt.Sprintf(`## AWK Review: 合併失敗（需要人工介入）

PR: #%d
mergeStateStatus: `+"`%s`"+`
mergeError: `+"`%v`"+`

PR 被保護規則擋住或有其他問題，需要人工處理。`, opts.PRNumber, mergeState, mergeErr)
	}

	if err := editIssueLabels(ctx, opts.IssueNumber, []string{label}, []string{"pr-ready"}, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to edit issue labels: %v\n", err)
	}
	if err := postIssueComment(ctx, opts.IssueNumber, message, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[REVIEW] warning: failed to post issue comment: %v\n", err)
	}

	reason := mergeState
	if mergeErr != nil {
		reason = fmt.Sprintf("%s: %v", mergeState, mergeErr)
	}
	return &SubmitReviewResult{Result: result, Reason: reason}, nil
}

// fetchIssueBody fetches the issue body from GitHub
func fetchIssueBody(ctx context.Context, issueNumber int, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "view", strconv.Itoa(issueNumber), "--json", "body", "--jq", ".body")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getTestCommand determines the test command based on config and repo
// Delegates to getTestCommandFromConfig with worktree path derived from issue number
func getTestCommand(stateRoot string, issueNumber int) string {
	worktreePath := filepath.Join(stateRoot, ".worktrees", fmt.Sprintf("issue-%d", issueNumber))
	return getTestCommandFromConfig(stateRoot, worktreePath)
}

// GitHub helper functions - all functions now return errors for proper handling

func postPRComment(ctx context.Context, prNumber int, body string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "comment", strconv.Itoa(prNumber), "--body", body)
	return err
}

func postIssueComment(ctx context.Context, issueNumber int, body string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "comment", strconv.Itoa(issueNumber), "--body", body)
	return err
}

func approvePR(ctx context.Context, prNumber, score int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "review", strconv.Itoa(prNumber),
		"--approve", "--body", fmt.Sprintf("AWK Review: APPROVED (score: %d/10)", score))
	return err
}

func requestChangesPR(ctx context.Context, prNumber, score int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "review", strconv.Itoa(prNumber),
		"--request-changes", "--body", fmt.Sprintf("AWK Review: CHANGES REQUESTED (score: %d/10)", score))
	return err
}

func mergePR(ctx context.Context, prNumber int, timeout time.Duration, mergeStrategy ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	strategy := "squash"
	if len(mergeStrategy) > 0 && mergeStrategy[0] != "" {
		strategy = mergeStrategy[0]
	}

	var strategyFlag string
	switch strategy {
	case "merge":
		strategyFlag = "--merge"
	case "rebase":
		strategyFlag = "--rebase"
	default:
		strategyFlag = "--squash"
	}

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "merge", strconv.Itoa(prNumber),
		strategyFlag, "--delete-branch")
	if err != nil {
		// Write PR merge failure event
		trace.WriteEvent(trace.ComponentGitHub, trace.TypePRMergeFail, trace.LevelError,
			trace.WithPR(prNumber),
			trace.WithError(err))
		return err
	}

	// Write PR merge success event
	trace.WriteEvent(trace.ComponentGitHub, trace.TypePRMerge, trace.LevelInfo,
		trace.WithPR(prNumber))
	return nil
}

func closeIssue(ctx context.Context, issueNumber int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "close", strconv.Itoa(issueNumber))
	return err
}

func removeLabel(ctx context.Context, issueNumber int, label string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "edit", strconv.Itoa(issueNumber),
		"--remove-label", label)
	return err
}

func editIssueLabels(ctx context.Context, issueNumber int, addLabels, removeLabels []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"issue", "edit", strconv.Itoa(issueNumber)}
	for _, l := range addLabels {
		args = append(args, "--add-label", l)
	}
	for _, l := range removeLabels {
		args = append(args, "--remove-label", l)
	}

	_, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", args...)
	return err
}

func getMergeStateStatus(ctx context.Context, prNumber int, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "pr", "view", strconv.Itoa(prNumber),
		"--json", "mergeStateStatus", "--jq", ".mergeStateStatus")
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

	// Find tasks.md path via proper YAML parsing
	configFile := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	specBasePath := ".ai/specs"
	if configData, err := os.ReadFile(configFile); err == nil {
		var cfg struct {
			Specs struct {
				BasePath string `yaml:"base_path"`
			} `yaml:"specs"`
		}
		if err := yaml.Unmarshal(configData, &cfg); err == nil && cfg.Specs.BasePath != "" {
			specBasePath = cfg.Specs.BasePath
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

// isEpicMode checks if the project uses GitHub Epic tracking mode.
// Returns false on any error (safe default to tasks_md behavior).
func isEpicMode(stateRoot string) bool {
	configPath := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil {
		return false
	}
	return cfg.IsEpicMode()
}

func cleanupWorktree(stateRoot string, issueNumber int) error {
	wtDir := filepath.Join(stateRoot, ".worktrees", fmt.Sprintf("issue-%d", issueNumber))
	if info, err := os.Stat(wtDir); err == nil && info.IsDir() {
		cmd := exec.Command("git", "worktree", "remove", wtDir, "--force")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to remove worktree %s: %w", wtDir, err)
		}
	}
	return nil
}
