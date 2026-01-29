package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// DispatchOptions contains options for DispatchWorker
type DispatchOptions struct {
	IssueNumber        int
	PRNumber           int           // PR number (used when MergeIssue is set to get base branch)
	PrincipalSessionID string
	StateRoot          string        // defaults to git root
	GHTimeout          time.Duration // GitHub API timeout (default: 30s)
	WorkerTimeout      time.Duration // Worker execution timeout (default: 60m)
	MaxRetries         int           // Max retry count (default: 3)
	MergeIssue         string        // "conflict" or "rebase" - indicates Worker needs to fix merge issues
}

// DispatchOutput is the output for bash eval compatibility
type DispatchOutput struct {
	Status       string // success, failed, in_progress, needs_conflict_resolution
	PRURL        string
	Error        string
	WorktreePath string // For conflict resolution
	IssueNumber  int    // For subagent
	PRNumber     int    // For subagent
}

// DispatchWorker dispatches an issue to a worker for execution
// This is the Go implementation of dispatch_worker.sh
func DispatchWorker(ctx context.Context, opts DispatchOptions) (output *DispatchOutput, err error) {
	// Set defaults
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 30 * time.Second
	}
	if opts.WorkerTimeout == 0 {
		opts.WorkerTimeout = 60 * time.Minute
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}

	startTime := time.Now()

	// Write dispatch_start event
	trace.WriteEvent(trace.ComponentPrincipal, trace.TypeDispatchStart, trace.LevelInfo,
		trace.WithIssue(opts.IssueNumber),
		trace.WithData(map[string]any{
			"session":        opts.PrincipalSessionID,
			"worker_timeout": opts.WorkerTimeout.String(),
			"max_retries":    opts.MaxRetries,
			"merge_issue":    opts.MergeIssue,
		}))

	// Write dispatch_end event on function return
	defer func() {
		level := trace.LevelInfo
		if output != nil && output.Status == "failed" {
			level = trace.LevelError
		}
		trace.WriteEvent(trace.ComponentPrincipal, trace.TypeDispatchEnd, level,
			trace.WithIssue(opts.IssueNumber),
			trace.WithData(map[string]any{
				"status":   output.Status,
				"pr_url":   output.PRURL,
				"error":    output.Error,
				"duration": time.Since(startTime).String(),
			}))
	}()

	ghClient := NewGitHubClient(opts.GHTimeout)
	logger := NewDispatchLogger(opts.StateRoot, opts.IssueNumber)
	defer func() {
		_ = logger.Close() // best-effort close, file handle leak is logged but not fatal
	}()

	logger.Log("派工 Issue #%d", opts.IssueNumber)

	// Set up cleanup manager for S2 fix
	cleanup := NewDispatchCleanup(opts.IssueNumber, opts.StateRoot, ghClient)
	defer cleanup.Run()

	// Step 1: Get session ID if not provided
	if opts.PrincipalSessionID == "" {
		opts.PrincipalSessionID = getSessionID(opts.StateRoot)
		if opts.PrincipalSessionID == "" {
			return &DispatchOutput{
				Status: "failed",
				Error:  "無法獲取 Principal Session ID",
			}, nil
		}
	}
	logger.Log("Session ID: %s", opts.PrincipalSessionID)

	// Step 2: Fetch and validate issue
	logger.Log("獲取 Issue 信息...")
	issue, err := ghClient.GetIssue(ctx, opts.IssueNumber)
	if err != nil {
		logger.Log("✗ 無法獲取 Issue 信息: %v", err)
		return &DispatchOutput{
			Status: "failed",
			Error:  fmt.Sprintf("無法獲取 Issue 信息: %v", err),
		}, nil
	}

	// Validate issue state
	if !issue.IsOpen() {
		logger.Log("✗ Issue 不是 open 狀態: %s", issue.State)
		return &DispatchOutput{
			Status: "failed",
			Error:  fmt.Sprintf("Issue 不是 open 狀態: %s", issue.State),
		}, nil
	}

	if !issue.HasLabel("ai-task") {
		logger.Log("✗ Issue 沒有 ai-task 標籤")
		return &DispatchOutput{
			Status: "failed",
			Error:  "Issue 沒有 ai-task 標籤",
		}, nil
	}

	if issue.HasLabel("in-progress") {
		logger.Log("⚠ Issue 已經在執行中")
		return &DispatchOutput{
			Status: "in_progress",
		}, nil
	}

	if issue.HasLabel("worker-failed") {
		logger.Log("✗ Issue 已標記為 worker-failed，需要人工介入")
		return &DispatchOutput{
			Status: "failed",
			Error:  "Issue 已標記為 worker-failed，需要人工介入",
		}, nil
	}

	logger.Log("✓ Issue 驗證通過")

	// Step 2.5: Auto-detect merge issue if not provided
	// This handles the case where Principal doesn't pass --merge-issue
	if opts.MergeIssue == "" {
		logger.Log("正在嘗試自動偵測 merge issue...")

		// First, check the result file for the previous PRURL
		if prevResult, err := LoadResult(opts.StateRoot, opts.IssueNumber); err == nil && prevResult.PRURL != "" {
			logger.Log("從 result file 找到 PR URL: %s", prevResult.PRURL)
			if prNum := ExtractPRNumber(prevResult.PRURL); prNum > 0 {
				// First check if PR is still open (not closed or merged)
				if isOpen, err := ghClient.IsPROpen(ctx, prNum); err == nil && isOpen {
					opts.PRNumber = prNum
					// Check PR merge state (only meaningful for OPEN PRs)
					if mergeState, err := ghClient.GetPRMergeState(ctx, prNum); err == nil {
						logger.Log("PR #%d merge state: %s", prNum, mergeState)
						switch mergeState {
						case "DIRTY":
							opts.MergeIssue = "conflict"
							logger.Log("⚠ 自動檢測到 PR #%d 有 merge conflict", prNum)
						case "BEHIND":
							opts.MergeIssue = "rebase"
							logger.Log("⚠ 自動檢測到 PR #%d 需要 rebase", prNum)
						}
					} else {
						logger.Log("⚠ 無法獲取 PR #%d merge state: %v", prNum, err)
					}
				} else if err != nil {
					logger.Log("⚠ 無法獲取 PR #%d 狀態: %v", prNum, err)
				} else {
					logger.Log("PR #%d 已關閉或已合併，略過 merge 狀態檢查", prNum)
				}
			}
		}

		// Fallback: try to find PR by branch name
		// Note: GetPRByBranch only returns OPEN PRs (gh pr list default behavior)
		if opts.MergeIssue == "" && opts.PRNumber == 0 {
			branch := fmt.Sprintf("feat/ai-issue-%d", opts.IssueNumber)
			logger.Log("嘗試通過 branch name 查找 PR: %s", branch)
			if prInfo, err := ghClient.GetPRByBranch(ctx, branch); err == nil && prInfo != nil {
				opts.PRNumber = prInfo.Number
				logger.Log("找到 PR #%d (branch: %s)", prInfo.Number, branch)
				// Check PR merge state
				if mergeState, err := ghClient.GetPRMergeState(ctx, prInfo.Number); err == nil {
					logger.Log("PR #%d merge state: %s", prInfo.Number, mergeState)
					switch mergeState {
					case "DIRTY":
						opts.MergeIssue = "conflict"
						logger.Log("⚠ 自動檢測到 PR #%d 有 merge conflict", prInfo.Number)
					case "BEHIND":
						opts.MergeIssue = "rebase"
						logger.Log("⚠ 自動檢測到 PR #%d 需要 rebase", prInfo.Number)
					}
				} else {
					logger.Log("⚠ 無法獲取 PR #%d merge state: %v", prInfo.Number, err)
				}
			} else if err != nil {
				logger.Log("未找到 branch %s 的 PR: %v", branch, err)
			}
		}

		// Log the result of auto-detection
		if opts.MergeIssue == "" {
			logger.Log("✓ 無需處理 merge issue（自動偵測完成）")
		}
	} else {
		logger.Log("使用 Principal 傳遞的 merge issue: %s", opts.MergeIssue)
	}

	// Step 3: Prepare ticket file
	logger.Log("準備 ticket 文件...")
	ticketBody := issue.Body

	// Check for previous review blocked reason and append to ticket
	if reviewReason, err := ghClient.GetLatestReviewBlockedReason(ctx, opts.IssueNumber); err == nil && reviewReason != "" {
		ticketBody += fmt.Sprintf(`

---

## ⚠️ 上次 Review 被阻擋 - 請修復以下問題

%s

**請根據以上錯誤修復代碼，確保所有測試通過後再提交。**
`, reviewReason)
		logger.Log("附加 review blocked 原因到 ticket")
	}

	// Append merge issue instructions if needed
	if opts.MergeIssue != "" {
		// Reset fail count - merge issue dispatch is a fresh start, not a retry
		if err := ResetFailCount(opts.StateRoot, opts.IssueNumber); err != nil {
			logger.Log("⚠ 無法重置失敗計數: %v", err)
		} else {
			logger.Log("✓ 已重置失敗計數 (merge issue dispatch)")
		}

		// Get the base branch from PR
		baseBranch := "main" // default fallback
		if opts.PRNumber > 0 {
			if prBaseBranch, err := ghClient.GetPRBaseBranch(ctx, opts.PRNumber); err == nil && prBaseBranch != "" {
				baseBranch = prBaseBranch
				logger.Log("PR #%d base branch: %s", opts.PRNumber, baseBranch)
			}
		}

		var instruction string
		switch opts.MergeIssue {
		case "conflict":
			instruction = fmt.Sprintf(`

---

## ⚠️ MERGE CONFLICT - 請先解決

PR 有 merge conflict，請執行以下步驟：

1. 在 worktree 中執行 `+"`git fetch origin && git rebase origin/%s`"+`
2. 解決衝突後 `+"`git add . && git rebase --continue`"+`
3. 推送更新 `+"`git push --force-with-lease`"+`
4. 確認 CI 通過

**這是最優先的任務，必須先完成才能繼續其他工作。**`, baseBranch)
		case "rebase":
			instruction = fmt.Sprintf(`

---

## ⚠️ BRANCH BEHIND - 請先 Rebase

PR 分支落後 base branch，請執行以下步驟：

1. 在 worktree 中執行 `+"`git fetch origin && git rebase origin/%s`"+`
2. 推送更新 `+"`git push --force-with-lease`"+`
3. 確認 CI 通過

**這是最優先的任務，必須先完成才能繼續其他工作。**`, baseBranch)
		}
		if instruction != "" {
			ticketBody += instruction
			logger.Log("附加 merge issue 指示: %s", opts.MergeIssue)
		}
	}

	ticketPath, err := SaveTicketFile(opts.StateRoot, opts.IssueNumber, ticketBody)
	if err != nil {
		logger.Log("✗ 無法保存 ticket 文件: %v", err)
		return &DispatchOutput{
			Status: "failed",
			Error:  fmt.Sprintf("無法保存 ticket 文件: %v", err),
		}, nil
	}
	logger.Log("✓ Ticket 文件已保存: %s", ticketPath)

	// Parse ticket metadata
	meta := ParseTicketMetadata(issue.Body)
	logger.Log("Repo: %s", meta.Repo)
	if meta.SpecName != "" && meta.TaskLine > 0 {
		logger.Log("Task mapping: %s (line %d)", meta.SpecName, meta.TaskLine)
		os.Setenv("AI_SPEC_NAME", meta.SpecName)
		os.Setenv("AI_TASK_LINE", fmt.Sprintf("%d", meta.TaskLine))
	}

	// Step 4: Add in-progress label and remove merge labels
	logger.Log("標記 Issue 為 in-progress...")
	if err := ghClient.AddLabel(ctx, opts.IssueNumber, "in-progress"); err != nil {
		logger.Log("⚠ 無法添加 in-progress 標籤: %v", err)
	} else {
		logger.Log("✓ Issue 已標記為 in-progress")
	}

	// Remove merge-related labels now that we're processing
	// This is done after adding in-progress to ensure the issue is marked as being worked on
	if opts.MergeIssue != "" {
		_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "merge-conflict")
		_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "needs-rebase")
	}

	// Step 5: Record worker_dispatched
	logger.Log("記錄 worker_dispatched...")
	recordSessionAction(opts.StateRoot, opts.PrincipalSessionID, "worker_dispatched",
		fmt.Sprintf(`{"issue_id":"%d","repo":"%s"}`, opts.IssueNumber, meta.Repo))
	logger.Log("✓ 已記錄 worker_dispatched")

	// Step 6: Execute worker with timeout (S4 fix)
	logger.Log("執行 Worker...")
	workerCtx, workerCancel := context.WithTimeout(ctx, opts.WorkerTimeout)
	defer workerCancel()

	workerExitCode := runWorkerScript(workerCtx, opts.StateRoot, opts.IssueNumber, ticketPath, meta.Repo, opts.MergeIssue, opts.PRNumber)
	logger.Log("Worker 執行完成 (exit code: %d)", workerExitCode)

	// Check for timeout
	if workerCtx.Err() == context.DeadlineExceeded {
		logger.Log("✗ Worker 超時 (超過 %v)", opts.WorkerTimeout)
		// Write timeout result
		timeoutResult := &IssueResult{
			IssueID:      fmt.Sprintf("%d", opts.IssueNumber),
			Status:       "timeout",
			FailureStage: "execution",
			ErrorMessage: fmt.Sprintf("Worker 超過超時時間 %v", opts.WorkerTimeout),
			Recoverable:  true,
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		}
		_ = WriteResultAtomic(opts.StateRoot, opts.IssueNumber, timeoutResult)
	}

	// Step 7: Check result
	result, err := LoadResult(opts.StateRoot, opts.IssueNumber)
	if err != nil {
		logger.Log("⚠ Result 文件不存在")
		return handleWorkerFailure(ctx, opts, logger, ghClient, meta, "no_result")
	}

	logger.Log("Worker status: %s", result.Status)
	if result.PRURL != "" {
		logger.Log("PR URL: %s", result.PRURL)
	}

	// Update result with principal session
	updateResultWithSession(opts.StateRoot, opts.IssueNumber, opts.PrincipalSessionID)

	// Step 8: Handle result

	// Check for needs_conflict_resolution first (before success/failure checks)
	if result.Status == "needs_conflict_resolution" {
		logger.Log("需要 conflict-resolver 處理 merge conflict")
		// Return special status for Principal to call subagent
		// Keep in-progress label - Principal will handle label changes based on subagent result
		return &DispatchOutput{
			Status:       "needs_conflict_resolution",
			WorktreePath: result.WorktreePath,
			IssueNumber:  opts.IssueNumber,
			PRNumber:     opts.PRNumber,
		}, nil
	}

	if result.Status == "success" && result.PRURL != "" {
		return handleWorkerSuccess(ctx, opts, logger, ghClient, result)
	}

	if result.Status == "success_no_changes" {
		return handleWorkerSuccessNoChanges(ctx, opts, logger, ghClient, result)
	}

	return handleWorkerFailure(ctx, opts, logger, ghClient, meta, result.Status)
}

// handleWorkerSuccess handles successful worker execution
func handleWorkerSuccess(ctx context.Context, opts DispatchOptions, logger *DispatchLogger, ghClient *GitHubClient, result *IssueResult) (*DispatchOutput, error) {
	logger.Log("✓ Worker 成功")

	// If this was a merge-issue dispatch, verify the PR is actually mergeable now
	// Try to get PR number from result.PRURL if not provided
	prNumber := opts.PRNumber
	if prNumber == 0 && result.PRURL != "" {
		prNumber = ExtractPRNumber(result.PRURL)
	}
	if opts.MergeIssue != "" && prNumber > 0 {
		mergeState, err := ghClient.GetPRMergeState(ctx, prNumber)
		if err != nil {
			// Cannot verify merge state - fail conservatively to allow retry
			// This prevents marking as pr-ready when merge conflict may still exist
			logger.Log("✗ 無法檢查 PR merge 狀態: %v", err)
			logger.Log("無法確認 merge issue 是否已解決，保守處理為失敗")

			// Keep the original merge label so next dispatch can retry
			if opts.MergeIssue == "conflict" {
				_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"merge-conflict"}, []string{"in-progress"})
			} else if opts.MergeIssue == "rebase" {
				_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"needs-rebase"}, []string{"in-progress"})
			}

			return &DispatchOutput{
				Status: "failed",
				PRURL:  result.PRURL,
				Error:  fmt.Sprintf("無法驗證 merge 狀態: %v", err),
			}, nil
		}

		if mergeState == "DIRTY" {
			// Worker claimed success but conflict still exists
			logger.Log("✗ Worker 回報成功但 PR #%d 仍有 merge conflict", prNumber)
			logger.Log("Worker 沒有正確執行 rebase 指示")

			// Don't mark as pr-ready, keep merge-conflict label
			_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"merge-conflict"}, []string{"in-progress"})
			_ = ghClient.CommentOnIssue(ctx, opts.IssueNumber,
				fmt.Sprintf("⚠️ Worker 回報成功，但 PR #%d 仍有 merge conflict。\n\n請確認 Worker 有執行 rebase 指示。", prNumber))

			return &DispatchOutput{
				Status: "failed",
				PRURL:  result.PRURL,
				Error:  "Worker 沒有解決 merge conflict",
			}, nil
		} else if mergeState == "BEHIND" {
			// Worker claimed success but branch is still behind
			logger.Log("✗ Worker 回報成功但 PR #%d 仍落後 base branch", prNumber)

			_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"needs-rebase"}, []string{"in-progress"})
			_ = ghClient.CommentOnIssue(ctx, opts.IssueNumber,
				fmt.Sprintf("⚠️ Worker 回報成功，但 PR #%d 仍落後 base branch。\n\n請確認 Worker 有執行 rebase 指示。", prNumber))

			return &DispatchOutput{
				Status: "failed",
				PRURL:  result.PRURL,
				Error:  "Worker 沒有完成 rebase",
			}, nil
		} else {
			logger.Log("✓ PR #%d merge 狀態: %s", prNumber, mergeState)
		}
	}

	// Update labels: remove in-progress, add pr-ready
	if err := ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"pr-ready"}, []string{"in-progress"}); err != nil {
		logger.Log("⚠ 無法更新 Issue 標籤: %v", err)
	} else {
		logger.Log("✓ Issue 標籤已更新 (in-progress → pr-ready)")
	}

	// Safety: also remove merge-related labels in case they weren't removed earlier
	// This prevents a loop where merge-conflict persists after Worker success
	_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "merge-conflict")
	_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "needs-rebase")

	// Record worker_completed
	recordSessionAction(opts.StateRoot, opts.PrincipalSessionID, "worker_completed",
		fmt.Sprintf(`{"issue_id":"%d","status":"success","pr_url":"%s"}`, opts.IssueNumber, result.PRURL))

	// Reset consecutive failures
	_ = ResetConsecutiveFailures(opts.StateRoot)

	return &DispatchOutput{
		Status: "success",
		PRURL:  result.PRURL,
	}, nil
}

// handleWorkerSuccessNoChanges handles successful worker execution that didn't require code changes
func handleWorkerSuccessNoChanges(ctx context.Context, opts DispatchOptions, logger *DispatchLogger, ghClient *GitHubClient, result *IssueResult) (*DispatchOutput, error) {
	logger.Log("✓ Worker 成功完成 (無需程式碼變更)")

	// Update labels: remove in-progress, add completed
	if err := ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"completed"}, []string{"in-progress"}); err != nil {
		logger.Log("⚠ 無法更新 Issue 標籤: %v", err)
	} else {
		logger.Log("✓ Issue 標籤已更新 (in-progress → completed)")
	}

	// Post comment explaining the result
	comment := "Worker 已成功完成任務，但無需修改程式碼。\n\n" +
		"可能的原因：\n" +
		"- 任務已經完成\n" +
		"- 程式碼已符合要求\n" +
		"- 任務屬於調查/分析類型，無需修改程式碼"
	if err := ghClient.CommentOnIssue(ctx, opts.IssueNumber, comment); err != nil {
		logger.Log("⚠ 無法發送評論: %v", err)
	}

	// Record worker_completed
	recordSessionAction(opts.StateRoot, opts.PrincipalSessionID, "worker_completed",
		fmt.Sprintf(`{"issue_id":"%d","status":"success_no_changes"}`, opts.IssueNumber))

	// Reset consecutive failures
	_ = ResetConsecutiveFailures(opts.StateRoot)

	return &DispatchOutput{
		Status: "success_no_changes",
	}, nil
}

// handleWorkerFailure handles failed worker execution
func handleWorkerFailure(ctx context.Context, opts DispatchOptions, logger *DispatchLogger, ghClient *GitHubClient, meta *TicketMetadata, failReason string) (*DispatchOutput, error) {
	logger.Log("✗ Worker 失敗 (reason: %s)", failReason)

	// Read fail count
	failCount := ReadFailCount(opts.StateRoot, opts.IssueNumber)
	logger.Log("失敗次數: %d / %d", failCount, opts.MaxRetries)

	// Update consecutive failures atomically to prevent corruption
	consecutiveFailures := ReadConsecutiveFailures(opts.StateRoot) + 1
	failuresFile := filepath.Join(opts.StateRoot, ".ai", "state", "consecutive_failures")
	if err := WriteFileAtomic(failuresFile, []byte(fmt.Sprintf("%d", consecutiveFailures)), 0644); err != nil {
		logger.Log("⚠ 無法更新連續失敗計數器: %v", err)
		// Continue execution - this is a tracking metric, not critical for workflow
	}

	if failCount >= opts.MaxRetries {
		logger.Log("✗ 達到最大重試次數 (%d)", opts.MaxRetries)

		// Update labels: remove in-progress, add worker-failed
		_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"worker-failed"}, []string{"in-progress"})

		// Post comment
		comment := fmt.Sprintf("Worker 已失敗 %d 次，需要人工介入。\n\n"+
			"請檢查：\n"+
			"1. 任務描述是否清晰\n"+
			"2. 是否有技術難點\n"+
			"3. 是否需要調整任務範圍\n\n"+
			"執行日誌位置：.ai/runs/issue-%d/", failCount, opts.IssueNumber)
		_ = ghClient.CommentOnIssue(ctx, opts.IssueNumber, comment)

		logger.Log("✓ 已標記為 worker-failed")

		// Record worker_failed
		recordSessionAction(opts.StateRoot, opts.PrincipalSessionID, "worker_failed",
			fmt.Sprintf(`{"issue_id":"%d","attempts":%d}`, opts.IssueNumber, failCount))

		return &DispatchOutput{
			Status: "failed",
			Error:  fmt.Sprintf("達到最大重試次數 (%d)", opts.MaxRetries),
		}, nil
	}

	logger.Log("將在下一輪重試 (attempt %d/%d)", failCount, opts.MaxRetries)

	// Remove in-progress label for retry
	_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "in-progress")
	logger.Log("✓ 已移除 in-progress 標籤")

	return &DispatchOutput{
		Status: "failed",
		Error:  fmt.Sprintf("Worker 失敗，將重試 (%d/%d)", failCount, opts.MaxRetries),
	}, nil
}

// runWorkerScript runs the worker using awkit run-issue command
func runWorkerScript(ctx context.Context, stateRoot string, issueNumber int, ticketPath, repo, mergeIssue string, prNumber int) int {
	exitCode, _ := runWorkerCommand(ctx, stateRoot, issueNumber, ticketPath, repo, mergeIssue, prNumber)
	return exitCode
}

// runWorkerCommand runs the worker using awkit run-issue command
func runWorkerCommand(ctx context.Context, stateRoot string, issueNumber int, ticketPath, repo, mergeIssue string, prNumber int) (int, bool) {
	// Check if awkit binary exists
	awkitPath, err := exec.LookPath("awkit")
	if err != nil {
		return 1, false // awkit not found
	}

	args := []string{"run-issue",
		"--issue", fmt.Sprintf("%d", issueNumber),
		"--ticket", ticketPath,
		"--state-root", stateRoot,
	}
	if repo != "" {
		args = append(args, "--repo", strings.TrimSpace(repo))
	}
	// Pass merge issue parameters
	if mergeIssue != "" {
		args = append(args, "--merge-issue", mergeIssue)
		if prNumber > 0 {
			args = append(args, "--pr-number", fmt.Sprintf("%d", prNumber))
		}
	}

	cmd := exec.CommandContext(ctx, awkitPath, args...)
	cmd.Dir = stateRoot
	cmd.Stdout = os.Stderr // Worker output goes to stderr
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), true
		}
		return 1, true
	}
	return 0, true
}

// getSessionID gets the current principal session ID
func getSessionID(stateRoot string) string {
	mgr := session.NewManager(stateRoot)
	return mgr.GetCurrentSessionID()
}

// recordSessionAction records an action in the session log
func recordSessionAction(stateRoot, sessionID, action, data string) {
	mgr := session.NewManager(stateRoot)
	_ = mgr.AppendAction(sessionID, action, data)
}

// updateResultWithSession updates the result file with principal session ID
func updateResultWithSession(stateRoot string, issueNumber int, sessionID string) {
	mgr := session.NewManager(stateRoot)
	issueIDStr := fmt.Sprintf("%d", issueNumber)
	_ = mgr.UpdateResultWithPrincipalSession(issueIDStr, sessionID)
}

// FormatBashOutput formats the dispatch output for bash eval
func (o *DispatchOutput) FormatBashOutput() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("WORKER_STATUS=%s\n", o.Status))
	if o.PRURL != "" {
		sb.WriteString(fmt.Sprintf("PR_URL=%s\n", o.PRURL))
	}
	if o.Error != "" {
		sb.WriteString(fmt.Sprintf("WORKER_ERROR=%s\n", o.Error))
	}
	// needs_conflict_resolution specific fields
	if o.Status == "needs_conflict_resolution" {
		sb.WriteString(fmt.Sprintf("WORKTREE_PATH=%s\n", o.WorktreePath))
		sb.WriteString(fmt.Sprintf("ISSUE_NUMBER=%d\n", o.IssueNumber))
		sb.WriteString(fmt.Sprintf("PR_NUMBER=%d\n", o.PRNumber))
	}
	return sb.String()
}

// DispatchLogger logs dispatch operations
type DispatchLogger struct {
	file *os.File
}

// NewDispatchLogger creates a new dispatch logger
func NewDispatchLogger(stateRoot string, issueNumber int) *DispatchLogger {
	logDir := filepath.Join(stateRoot, ".ai", "exe-logs")
	_ = os.MkdirAll(logDir, 0755)

	logPath := filepath.Join(logDir, "principal.log")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &DispatchLogger{}
	}

	return &DispatchLogger{file: file}
}

// Log logs a message
func (l *DispatchLogger) Log(format string, args ...interface{}) {
	if l.file == nil {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("[PRINCIPAL] %s | %s\n", timestamp, fmt.Sprintf(format, args...))
	_, _ = l.file.WriteString(msg)
}

// Close closes the logger and returns any error encountered.
// On Windows, failing to close file handles can cause file locking issues.
func (l *DispatchLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
