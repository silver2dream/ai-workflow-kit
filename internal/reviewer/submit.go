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
	PRNumber    int
	IssueNumber int
	Score       int    // 1-10
	CIStatus    string // "passed" | "failed"
	ReviewBody  string
	StateRoot   string
	GHTimeout   time.Duration
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

	// Fetch diff
	diffPath := filepath.Join(reviewDir, "diff.patch")
	diff, err := fetchPRDiff(ctx, opts.PRNumber, opts.GHTimeout)
	if err != nil {
		diff = ""
	} else {
		_ = os.WriteFile(diffPath, []byte(diff), 0644)
	}

	diffHash := "unavailable"
	diffBytes := "0"
	if diff != "" {
		diffHash = sha256_16(diff)
		diffBytes = strconv.Itoa(len(diff))
	}

	// Save review body
	reviewMDPath := filepath.Join(reviewDir, "review.md")
	_ = os.WriteFile(reviewMDPath, []byte(opts.ReviewBody), 0644)

	// Verify evidence
	evidence := ParseEvidence(opts.ReviewBody)
	evidenceCount := len(evidence)
	minEvidence := GetMinEvidence()

	if err := VerifyEvidence(diff, evidence, minEvidence); err != nil {
		return handleEvidenceFailure(ctx, opts, sessionID, diffHash, err)
	}

	// Post AWK Review Comment
	timestamp := time.Now().UTC().Format(time.RFC3339)
	commentBody := fmt.Sprintf(`<!-- AWK:session:%s -->
🤖 **AWK Review**

| Field | Value |
|-------|-------|
| Reviewer Session | `+"`%s`"+` |
| Review Timestamp | %s |
| CI Status | %s |
| Diff Hash | `+"`%s`"+` |
| Diff Bytes | `+"`%s`"+` |
| Evidence Lines | `+"`%d`"+` |
| Score | %d/10 |

%s`, sessionID, sessionID, timestamp, opts.CIStatus, diffHash, diffBytes, evidenceCount, opts.Score, opts.ReviewBody)

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

func handleEvidenceFailure(ctx context.Context, opts SubmitReviewOptions, sessionID, diffHash string, err *EvidenceError) (*SubmitReviewResult, error) {
	editIssueLabels(ctx, opts.IssueNumber, []string{"needs-human-review"}, []string{"pr-ready"}, opts.GHTimeout)

	var missingDetails string
	if err.Missing != nil {
		missingDetails = "\n```\n"
		for _, m := range err.Missing {
			missingDetails += "- " + m + "\n"
		}
		missingDetails += "```\n"
	}

	postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review blocked: evidence verification failed

審查內容缺少可核對的 `+"`EVIDENCE:`"+` 行，或 evidence 與 PR diff 不一致。

PR: #%d
Diff Hash: `+"`%s`"+`

錯誤: %s
%s
Next step: 重新產生審查內容，並加入 `+"`EVIDENCE:`"+` 行（必須是 diff 中可直接搜尋到的字串）。`, opts.PRNumber, diffHash, err.Message, missingDetails), opts.GHTimeout)

	return &SubmitReviewResult{Result: "review_blocked", Reason: err.Message}, nil
}

func handleMergeFailure(ctx context.Context, opts SubmitReviewOptions, sessionID string, mergeErr error) (*SubmitReviewResult, error) {
	// Get merge state status
	mergeState := getMergeStateStatus(ctx, opts.PRNumber, opts.GHTimeout)

	nextStep := "請到 PR 頁面查看 merge 錯誤原因。"
	switch mergeState {
	case "DIRTY":
		nextStep = "PR 有 merge conflict，請解決衝突後 push 重新嘗試合併。"
	case "BEHIND":
		nextStep = "PR 分支落後 base branch，請 rebase/merge base branch 後 push 重新嘗試合併。"
	case "BLOCKED":
		nextStep = "PR 被保護規則擋住（checks/reviews），請確認 required checks/reviews 後再合併。"
	}

	editIssueLabels(ctx, opts.IssueNumber, []string{"needs-human-review"}, []string{"pr-ready"}, opts.GHTimeout)

	postIssueComment(ctx, opts.IssueNumber, fmt.Sprintf(`## AWK Review: 合併失敗（需要人工介入）

PR: #%d
mergeStateStatus: `+"`%s`"+`

下一步建議：%s`, opts.PRNumber, mergeState, nextStep), opts.GHTimeout)

	return &SubmitReviewResult{Result: "merge_failed", Reason: mergeState}, nil
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
