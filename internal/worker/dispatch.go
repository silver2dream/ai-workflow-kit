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
	Status   string // success, failed, in_progress
	PRURL    string
	Error    string
}

// DispatchWorker dispatches an issue to a worker for execution
// This is the Go implementation of dispatch_worker.sh
func DispatchWorker(ctx context.Context, opts DispatchOptions) (*DispatchOutput, error) {
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

	ghClient := NewGitHubClient(opts.GHTimeout)
	logger := NewDispatchLogger(opts.StateRoot, opts.IssueNumber)
	defer logger.Close()

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
		// Check if there's an existing PR for this issue
		branch := fmt.Sprintf("feat/ai-issue-%d", opts.IssueNumber)
		if prInfo, err := ghClient.GetPRByBranch(ctx, branch); err == nil && prInfo != nil {
			opts.PRNumber = prInfo.Number
			// Check PR merge state
			if mergeState, err := ghClient.GetPRMergeState(ctx, prInfo.Number); err == nil {
				switch mergeState {
				case "DIRTY":
					opts.MergeIssue = "conflict"
					logger.Log("⚠ 自動檢測到 PR #%d 有 merge conflict", prInfo.Number)
				case "BEHIND":
					opts.MergeIssue = "rebase"
					logger.Log("⚠ 自動檢測到 PR #%d 需要 rebase", prInfo.Number)
				}
			}
		}
	}

	// Step 3: Prepare ticket file
	logger.Log("準備 ticket 文件...")
	ticketBody := issue.Body

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

	// Step 4: Add in-progress label
	logger.Log("標記 Issue 為 in-progress...")
	if err := ghClient.AddLabel(ctx, opts.IssueNumber, "in-progress"); err != nil {
		logger.Log("⚠ 無法添加 in-progress 標籤: %v", err)
	} else {
		logger.Log("✓ Issue 已標記為 in-progress")
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

	workerExitCode := runWorkerScript(workerCtx, opts.StateRoot, opts.IssueNumber, ticketPath, meta.Repo)
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
	if result.Status == "success" && result.PRURL != "" {
		return handleWorkerSuccess(ctx, opts, logger, ghClient, result)
	}

	return handleWorkerFailure(ctx, opts, logger, ghClient, meta, result.Status)
}

// handleWorkerSuccess handles successful worker execution
func handleWorkerSuccess(ctx context.Context, opts DispatchOptions, logger *DispatchLogger, ghClient *GitHubClient, result *IssueResult) (*DispatchOutput, error) {
	logger.Log("✓ Worker 成功")

	// Update labels: remove in-progress, add pr-ready
	if err := ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"pr-ready"}, []string{"in-progress"}); err != nil {
		logger.Log("⚠ 無法更新 Issue 標籤: %v", err)
	} else {
		logger.Log("✓ Issue 標籤已更新 (in-progress → pr-ready)")
	}

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

// handleWorkerFailure handles failed worker execution
func handleWorkerFailure(ctx context.Context, opts DispatchOptions, logger *DispatchLogger, ghClient *GitHubClient, meta *TicketMetadata, failReason string) (*DispatchOutput, error) {
	logger.Log("✗ Worker 失敗 (reason: %s)", failReason)

	// Read fail count
	failCount := ReadFailCount(opts.StateRoot, opts.IssueNumber)
	logger.Log("失敗次數: %d / %d", failCount, opts.MaxRetries)

	// Update consecutive failures
	consecutiveFailures := ReadConsecutiveFailures(opts.StateRoot) + 1
	_ = os.WriteFile(filepath.Join(opts.StateRoot, ".ai", "state", "consecutive_failures"),
		[]byte(fmt.Sprintf("%d", consecutiveFailures)), 0644)

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

// runWorkerScript runs the worker (Go command or fallback to bash script)
// Uses awkit run-issue by default, falls back to bash script if:
// - AWKIT_USE_SCRIPT=1 is set, or
// - awkit binary is not found in PATH
func runWorkerScript(ctx context.Context, stateRoot string, issueNumber int, ticketPath, repo string) int {
	// Check if we should use the bash script fallback
	useScript := os.Getenv("AWKIT_USE_SCRIPT") == "1"

	if !useScript {
		// Try to use awkit run-issue (Go implementation)
		if exitCode, ok := runWorkerCommand(ctx, stateRoot, issueNumber, ticketPath, repo); ok {
			return exitCode
		}
		// Fall through to bash script if awkit not available
	}

	// Fallback: use bash script
	return runWorkerBashScript(ctx, stateRoot, issueNumber, ticketPath, repo)
}

// runWorkerCommand runs the worker using awkit run-issue command
// Returns (exitCode, true) if successful, (0, false) if awkit not available
func runWorkerCommand(ctx context.Context, stateRoot string, issueNumber int, ticketPath, repo string) (int, bool) {
	// Check if awkit binary exists
	awkitPath, err := exec.LookPath("awkit")
	if err != nil {
		return 0, false // awkit not found, signal to use fallback
	}

	args := []string{"run-issue",
		"--issue", fmt.Sprintf("%d", issueNumber),
		"--ticket", ticketPath,
		"--state-root", stateRoot,
	}
	if repo != "" {
		args = append(args, "--repo", strings.TrimSpace(repo))
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

// runWorkerBashScript runs the worker using bash script (legacy fallback)
func runWorkerBashScript(ctx context.Context, stateRoot string, issueNumber int, ticketPath, repo string) int {
	scriptPath := filepath.Join(stateRoot, ".ai", "scripts", "run_issue_codex.sh")

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return 1
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath,
		fmt.Sprintf("%d", issueNumber), ticketPath, strings.TrimSpace(repo))
	cmd.Dir = stateRoot
	cmd.Stdout = os.Stderr // Worker output goes to stderr
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

// getSessionID gets the current principal session ID
func getSessionID(stateRoot string) string {
	// Try Go implementation first
	mgr := session.NewManager(stateRoot)
	if sessionID := mgr.GetCurrentSessionID(); sessionID != "" {
		return sessionID
	}

	// Fallback to bash script if Go implementation fails
	if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
		scriptPath := filepath.Join(stateRoot, ".ai", "scripts", "session_manager.sh")
		cmd := exec.Command("bash", scriptPath, "get_current_session_id")
		cmd.Dir = stateRoot
		output, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	}

	return ""
}

// recordSessionAction records an action in the session log
func recordSessionAction(stateRoot, sessionID, action, data string) {
	// Try Go implementation first
	mgr := session.NewManager(stateRoot)
	if err := mgr.AppendAction(sessionID, action, data); err == nil {
		return
	}

	// Fallback to bash script
	if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
		scriptPath := filepath.Join(stateRoot, ".ai", "scripts", "session_manager.sh")
		cmd := exec.Command("bash", scriptPath, "append_session_action", sessionID, action, data)
		cmd.Dir = stateRoot
		_ = cmd.Run()
	}
}

// updateResultWithSession updates the result file with principal session ID
func updateResultWithSession(stateRoot string, issueNumber int, sessionID string) {
	// Try Go implementation first
	mgr := session.NewManager(stateRoot)
	issueIDStr := fmt.Sprintf("%d", issueNumber)
	if err := mgr.UpdateResultWithPrincipalSession(issueIDStr, sessionID); err == nil {
		return
	}

	// Fallback to bash script
	if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
		scriptPath := filepath.Join(stateRoot, ".ai", "scripts", "session_manager.sh")
		cmd := exec.Command("bash", scriptPath, "update_result_with_principal_session",
			fmt.Sprintf("%d", issueNumber), sessionID)
		cmd.Dir = stateRoot
		_ = cmd.Run()
	}
}

// FormatBashOutput formats the dispatch output for bash eval
func (o *DispatchOutput) FormatBashOutput() string {
	return fmt.Sprintf("WORKER_STATUS=%s\n", o.Status)
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

// Close closes the logger
func (l *DispatchLogger) Close() {
	if l.file != nil {
		_ = l.file.Close()
	}
}
