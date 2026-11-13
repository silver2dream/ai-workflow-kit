package worker

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	repocfg "github.com/silver2dream/ai-workflow-kit/internal/repo"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
	"github.com/silver2dream/ai-workflow-kit/internal/util"
)

// RunIssueOptions controls worker execution for a single issue.
type RunIssueOptions struct {
	IssueID      int
	TicketFile   string
	RepoOverride string
	StateRoot    string
	GHTimeout    time.Duration
	GitTimeout   time.Duration
	CodexTimeout time.Duration
	MergeIssue   string // "conflict" or "rebase" - indicates merge issue to resolve
	PRNumber     int    // PR number for merge issue resolution
}

// RunIssueResult reports worker execution status.
type RunIssueResult struct {
	Status   string
	PRURL    string
	Error    string
	ExitCode int
}

// The configuration is read by package config, for everyone. These are the
// names the Worker has for it.
type (
	workflowConfig     = config.Config
	workflowWorker     = config.Worker
	workflowClaudeCode = config.ClaudeCode
	workflowFeedback   = config.Feedback
	workflowRepo       = config.Repo
	workflowRepoVerify = config.Verify
	workflowGit        = config.Git
	workflowEscalation = config.Escalation
	workflowTimeouts   = config.Timeouts
	workflowLessons    = config.Lessons
)

type attemptInfo struct {
	AttemptNumber         int
	PreviousSessionIDs    []string
	PreviousFailureReason string
}

type issueResultContext struct {
	IssueID               int
	Status                string
	PRURL                 string
	RepoName              string
	RepoType              string
	RepoPath              string
	WorktreeDir           string
	WorkDir               string
	Branch                string
	BaseBranch            string
	SummaryFile           string
	ErrorMessage          string
	FailureStage          string
	WorkerSessionID       string
	AttemptNumber         int
	PreviousSessionIDs    []string
	PreviousFailureReason string
	WorkerPID             int
	WorkerStartTime       int64
	Duration              time.Duration
	RetryCount            int
	SubmoduleSHA          string
	ConsistencyStatus     string
	Recoverable           bool
}

type workerLogger struct {
	file    *os.File
	summary *os.File
}

// RunIssue executes the worker flow for an issue.
func RunIssue(ctx context.Context, opts RunIssueOptions) (*RunIssueResult, error) {
	if opts.IssueID == 0 {
		return nil, fmt.Errorf("issue id is required")
	}
	if strings.TrimSpace(opts.TicketFile) == "" {
		return nil, fmt.Errorf("ticket file is required")
	}

	stateRoot := opts.StateRoot
	if stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			return nil, err
		}
		stateRoot = root
	}

	ticketBody, err := os.ReadFile(opts.TicketFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read ticket file: %w", err)
	}

	// What the ticket says about itself is read from the ticket, never from
	// what the dispatch added to the file.
	ticketText := ticketOf(string(ticketBody))
	meta, err := ReadTicketMetadata(ticketText)
	if err != nil {
		return nil, fmt.Errorf("ticket #%d cannot be used: %w", opts.IssueID, err)
	}
	repoName := strings.TrimSpace(opts.RepoOverride)
	if repoName == "" {
		repoName = meta.Repo
	}
	repoName = strings.ToLower(strings.TrimSpace(repoName))
	if repoName == "" {
		repoName = "root"
	}

	configPath := config.Path(stateRoot)
	cfg, cfgErr := loadWorkflowConfig(configPath)
	if cfgErr != nil {
		// A project that has not been upgraded has a configuration, at its
		// earlier place: working from the defaults would ignore it.
		var old *config.OldLayoutError
		if errors.As(cfgErr, &old) {
			return nil, cfgErr
		}
		cfg = config.Default()
	}

	opts.fillTimeouts(cfg.Timeouts)

	// Build retry config from workflow.yaml
	ghRetryConfig := buildRetryConfig(cfg.Timeouts)

	integrationBranch := cfg.BaseBranch()
	releaseBranch := cfg.Git.ReleaseBranch
	if releaseBranch == "" {
		releaseBranch = "main"
	}

	retryCount := cfg.Escalation.RetryCount
	retryDelay := time.Duration(cfg.Escalation.RetryDelaySeconds) * time.Second
	if retryDelay <= 0 {
		retryDelay = 5 * time.Second
	}

	// The ticket's repo is lowered above; the configuration may write the
	// name with capitals, and finds it all the same.
	if !cfg.HasTicketRepo(repoName) {
		return nil, fmt.Errorf("repo must be one of: %s (got %q)", strings.Join(cfg.TicketRepos(), ", "), repoName)
	}

	repoType, repoPath := resolveRepoConfig(cfg, repoName)
	if repoName == "root" {
		repoType = "root"
		repoPath = "./"
	}

	releaseFlag := meta.Release
	if releaseFlag && repoName != "root" {
		return nil, fmt.Errorf("release tickets are allowed only for root repo")
	}

	prBase := integrationBranch
	if releaseFlag {
		prBase = releaseBranch
	}

	branch := fmt.Sprintf("feat/ai-issue-%d", opts.IssueID)
	logDir := layout.Logs(stateRoot)
	runDir := layout.Run(stateRoot, opts.IssueID)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(layout.Results(stateRoot), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, ".ai", "state"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, ".worktrees"), 0755); err != nil {
		return nil, err
	}

	summaryFile := filepath.Join(runDir, "summary.txt")
	_ = os.WriteFile(summaryFile, []byte(""), 0644)

	workerLogFile := filepath.Join(logDir, fmt.Sprintf("issue-%d.worker.log", opts.IssueID))
	codexLogBase := filepath.Join(logDir, fmt.Sprintf("issue-%d.%s.codex", opts.IssueID, repoName))
	earlyFailureLog := codexLogBase + ".early-failure.log"

	logger := newWorkerLogger(workerLogFile, summaryFile)
	defer func() {
		if err := logger.Close(); err != nil {
			// Log to stderr as the logger itself is closing
			fmt.Fprintf(os.Stderr, "WARNING: failed to close worker logger: %v\n", err)
		}
	}()

	workerSessionID := generateSessionID("worker")
	logger.Log("worker_session_id=%s issue=#%d repo=%s attempt=%d", workerSessionID, opts.IssueID, repoName, loadAttemptInfo(stateRoot, opts.IssueID).AttemptNumber)
	logger.Log("config=%s repos=%d", configPath, len(cfg.Repos))
	if cfgErr != nil {
		logger.Log("[ERROR] config_load_failed: %v (using defaults)", cfgErr)
	}
	_ = os.Setenv("WORKER_SESSION_ID", workerSessionID)
	_ = os.Setenv("WORKER_LOG_FILE", workerLogFile)

	attemptInfo := loadAttemptInfo(stateRoot, opts.IssueID)

	startTime := time.Now()
	pidInfo := &PIDFile{
		PID:         os.Getpid(),
		StartTime:   startTime.Unix(),
		IssueNumber: opts.IssueID,
		SessionID:   workerSessionID,
		StartedAt:   startTime,
	}
	if err := WritePIDFile(stateRoot, opts.IssueID, pidInfo); err != nil {
		logger.Log("[ERROR] WritePIDFile: %v", err)
	}

	trace, _ := NewTraceRecorder(stateRoot, opts.IssueID, repoName, branch, prBase, pidInfo.PID, startTime)

	var runErr error
	result := &RunIssueResult{Status: "failed", ExitCode: 1}
	resultWritten := false

	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = fmt.Errorf("panic: %v", recovered)
			if !resultWritten {
				_ = writeIssueResult(ctx, stateRoot, issueResultContext{
					IssueID:               opts.IssueID,
					Status:                "crashed",
					RepoName:              repoName,
					RepoType:              repoType,
					RepoPath:              repoPath,
					Branch:                branch,
					BaseBranch:            prBase,
					SummaryFile:           summaryFile,
					ErrorMessage:          fmt.Sprintf("panic: %v", recovered),
					FailureStage:          "panic",
					WorkerSessionID:       workerSessionID,
					AttemptNumber:         attemptInfo.AttemptNumber,
					PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
					PreviousFailureReason: attemptInfo.PreviousFailureReason,
					WorkerPID:             pidInfo.PID,
					WorkerStartTime:       pidInfo.StartTime,
					Duration:              time.Since(startTime),
					RetryCount:            0,
					Recoverable:           true,
				})
			}
		}

		if trace != nil {
			_ = trace.Finalize(runErr)
		}
		_ = CleanupPIDFile(stateRoot, opts.IssueID)
	}()

	if trace != nil {
		trace.StepStart("attempt_guard")
	}
	if err := runAttemptGuard(ctx, stateRoot, opts.IssueID, workerLogFile, opts); err != nil {
		runErr = err
		logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "attempt_guard", err.Error())
		if trace != nil {
			_ = trace.StepEnd("failed", "attempt_guard failed", nil)
		}
		result.Error = err.Error()
		return result, err
	}
	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	if trace != nil {
		trace.StepStart("preflight")
	}

	// Auto-clean root repository before checking dirty state (Issue #122 fix)
	// This handles leftover changes from previous failed Worker executions
	if err := autoCleanRootRepository(ctx, stateRoot, opts.GitTimeout, logger.Log); err != nil {
		logger.Log("WARNING: auto-clean failed: %v (proceeding with dirty check)", err)
	}

	if dirty, details := isWorkingTreeDirty(ctx, stateRoot); dirty {
		runErr = fmt.Errorf("working tree not clean")
		logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "preflight", runErr.Error())
		if details != "" {
			_, _ = fmt.Fprintln(os.Stderr, details)
		}
		if trace != nil {
			_ = trace.StepEnd("failed", "working tree not clean", nil)
		}
		result.Error = runErr.Error()
		return result, runErr
	}
	if err := runPreflight(ctx, stateRoot, repoType, repoPath, workerLogFile, opts); err != nil {
		runErr = err
		logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "preflight", err.Error())
		if trace != nil {
			_ = trace.StepEnd("failed", "preflight failed", nil)
		}
		result.Error = err.Error()
		return result, err
	}
	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	if trace != nil {
		trace.StepStart("worktree")
	}
	worktreeInfo, err := SetupWorktree(ctx, WorktreeOptions{
		StateRoot:         stateRoot,
		IssueID:           opts.IssueID,
		Branch:            branch,
		RepoType:          repoType,
		RepoPath:          repoPath,
		IntegrationBranch: integrationBranch,
		GitTimeout:        opts.GitTimeout,
	})
	if err != nil {
		runErr = err
		logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "worktree", err.Error())
		if trace != nil {
			_ = trace.StepEnd("failed", "worktree setup failed", nil)
		}
		result.Error = err.Error()
		return result, err
	}

	if _, err := os.Stat(worktreeInfo.WorkDir); err != nil {
		runErr = fmt.Errorf("work directory not found: %s", worktreeInfo.WorkDir)
		logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "worktree", runErr.Error())
		if trace != nil {
			_ = trace.StepEnd("failed", "work directory not found", nil)
		}
		result.Error = runErr.Error()
		return result, runErr
	}

	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	workDir := worktreeInfo.WorkDir
	wtDir := worktreeInfo.WorktreeDir
	logger.Log("worktree=%s work_dir=%s", wtDir, workDir)

	_ = runGit(ctx, wtDir, opts.GitTimeout, "fetch", "origin", "--prune")
	_ = runGit(ctx, wtDir, opts.GitTimeout, "checkout", "-q", branch)

	if strings.EqualFold(strings.TrimSpace(os.Getenv("AI_BRANCH_MODE")), "reset") {
		baseRef := strings.TrimSpace(os.Getenv("AI_RESET_BASE"))
		if baseRef == "" {
			baseRef = "origin/" + integrationBranch
		}
		// Validate baseRef to prevent injection of invalid refs
		if !isValidGitRef(baseRef) {
			logger.Log("WARNING: invalid AI_RESET_BASE value, using default")
			baseRef = "origin/" + integrationBranch
		}
		logger.Log("reset branch to %s", baseRef)
		_ = runGit(ctx, wtDir, opts.GitTimeout, "fetch", "origin", "--prune")
		_ = runGit(ctx, wtDir, opts.GitTimeout, "reset", "--hard", baseRef)
	}

	// Handle merge issue if present (conflict resolution)
	if opts.MergeIssue != "" && opts.PRNumber > 0 {
		logger.Log("嘗試解決 merge issue: %s (PR #%d)", opts.MergeIssue, opts.PRNumber)

		// Create ghClient for GetPRBaseBranch
		ghClient := NewGitHubClient(opts.GHTimeout)
		ghClient.Retry = ghRetryConfig

		// Get base branch from PR
		baseBranch, err := ghClient.GetPRBaseBranch(ctx, opts.PRNumber)
		if err != nil || baseBranch == "" {
			baseBranch = integrationBranch // fallback
			logger.Log("⚠ 無法獲取 PR base branch，使用 fallback: %s", baseBranch)
		} else {
			logger.Log("PR base branch: %s", baseBranch)
		}

		// Get repo name for PR URL
		repoFullName := getRepoName(ctx)

		// Check for in-progress rebase and abort if needed
		rebaseDir := filepath.Join(wtDir, ".git", "rebase-merge")
		if _, err := os.Stat(rebaseDir); err == nil {
			logger.Log("⚠ 檢測到進行中的 rebase，先 abort")
			_ = runGit(ctx, wtDir, opts.GitTimeout, "rebase", "--abort")
		}

		// Clean worktree before rebase
		_ = runGit(ctx, wtDir, opts.GitTimeout, "reset", "--hard", "HEAD")
		_ = runGit(ctx, wtDir, opts.GitTimeout, "clean", "-fd")

		// Attempt rebase
		rebaseErr := RebaseOntoBase(ctx, wtDir, baseBranch, opts.GitTimeout)

		if rebaseErr == nil {
			// Rebase succeeded, push and return
			if pushErr := ForcePushBranch(ctx, wtDir, branch, opts.GitTimeout); pushErr != nil {
				logger.Log("✗ Push 失敗: %v", pushErr)
				_ = writeIssueResult(ctx, stateRoot, issueResultContext{
					IssueID:         opts.IssueID,
					Status:          "failed",
					RepoName:        repoName,
					RepoType:        repoType,
					RepoPath:        repoPath,
					WorktreeDir:     wtDir,
					WorkDir:         workDir,
					Branch:          branch,
					BaseBranch:      prBase,
					ErrorMessage:    pushErr.Error(),
					FailureStage:    "git_push",
					WorkerSessionID: workerSessionID,
				})
				result.Status = "failed"
				result.Error = pushErr.Error()
				return result, pushErr
			}

			// Success! Write result and return
			prURL := fmt.Sprintf("https://github.com/%s/pull/%d", repoFullName, opts.PRNumber)
			logger.Log("✓ Rebase + push 成功，跳過 Worker 執行")
			_ = writeIssueResult(ctx, stateRoot, issueResultContext{
				IssueID:         opts.IssueID,
				Status:          "success",
				PRURL:           prURL,
				RepoName:        repoName,
				RepoType:        repoType,
				RepoPath:        repoPath,
				WorktreeDir:     wtDir,
				WorkDir:         workDir,
				Branch:          branch,
				BaseBranch:      prBase,
				WorkerSessionID: workerSessionID,
			})
			result.Status = "success"
			result.PRURL = prURL
			result.ExitCode = 0
			return result, nil
		}

		if errors.Is(rebaseErr, ErrRebaseConflict) {
			// Has actual conflicts, need AI intervention
			logger.Log("✗ 有 merge conflict，需要 conflict-resolver")
			_ = writeIssueResult(ctx, stateRoot, issueResultContext{
				IssueID:         opts.IssueID,
				Status:          "needs_conflict_resolution",
				RepoName:        repoName,
				RepoType:        repoType,
				RepoPath:        repoPath,
				WorktreeDir:     wtDir,
				WorkDir:         workDir,
				Branch:          branch,
				BaseBranch:      prBase,
				ErrorMessage:    "rebase 有衝突需要 AI 解決",
				WorkerSessionID: workerSessionID,
			})
			result.Status = "needs_conflict_resolution"
			return result, nil
		}

		// Other rebase error, abort and continue with Worker
		logger.Log("⚠ Rebase 失敗 (%v)，繼續執行 Worker", rebaseErr)
		_ = runGit(ctx, wtDir, opts.GitTimeout, "rebase", "--abort")
	}

	// Run setup command (dependency install) before AI worker starts.
	// This must happen before Codex because the AI worker will run
	// verification commands from the ticket, which require dependencies.
	setupCmd := getSetupCommand(cfg, repoName)
	if setupCmd != "" {
		logger.Log("verify.setup: %s", setupCmd)
		if setupErr := runVerificationCommand(ctx, workDir, setupCmd, opts.GitTimeout); setupErr != nil {
			runErr = fmt.Errorf("verify.setup failed: %s: %w", setupCmd, setupErr)
			logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "verify_setup", runErr.Error())
			result.Error = runErr.Error()
			result.Status = "failed"
			return result, runErr
		}
		logger.Log("  ✓ 依賴安裝完成")
	}

	titleLine := extractTitleLine(string(ticketBody))
	if titleLine == "" {
		titleLine = fmt.Sprintf("issue-%d", opts.IssueID)
	}
	commitMsg := BuildCommitMessage(titleLine)

	workDirInstruction := buildWorkDirInstruction(repoType, repoPath, workDir, repoName)

	// Detect existing PR for this branch (retry case) to fetch file-level review comments
	existingPRNumber := 0
	if attemptInfo.AttemptNumber > 1 {
		ghClient := NewGitHubClient(opts.GHTimeout)
		if prInfo, err := ghClient.GetPRByBranch(ctx, branch); err == nil && prInfo != nil && prInfo.Number > 0 {
			existingPRNumber = prInfo.Number
			logger.Log("found existing PR #%d for branch %s (retry attempt %d)", existingPRNumber, branch, attemptInfo.AttemptNumber)
		}
	}

	promptFile := filepath.Join(runDir, "prompt.txt")
	if err := writePromptFile(promptFile, workDirInstruction, string(ticketBody), stateRoot, opts.IssueID, existingPRNumber, opts.GHTimeout, &cfg.Feedback, &cfg.Worker, &cfg.Lessons); err != nil {
		runErr = err
		logEarlyFailure(earlyFailureLog, repoName, repoType, repoPath, branch, prBase, "prompt", err.Error())
		result.Error = err.Error()
		return result, err
	}

	if trace != nil {
		trace.StepStart("worker_start_comment")
	}
	postIssueComment(ctx, opts.IssueID, workerSessionID, "worker_start", "", buildWorkerStartExtra(attemptInfo.AttemptNumber), opts.GHTimeout)
	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	// Select and run AI worker backend
	backendName := cfg.Worker.Backend
	if backendName == "" {
		backendName = "codex"
	}
	registry := DefaultRegistry(
		cfg.Worker.ClaudeCode.Model,
		cfg.Worker.ClaudeCode.MaxTurns,
		cfg.Worker.ClaudeCode.DangerouslySkipPermissions,
	)
	backend, backendErr := registry.Get(backendName)
	if backendErr != nil {
		// Fallback to codex if unknown backend
		fmt.Fprintf(os.Stderr, "[worker] warning: %v, falling back to codex\n", backendErr)
		backend = NewCodexBackend()
	}
	logger.Log("AI worker 開始 (backend=%s, timeout=%s)", backendName, resolveTimeout("AI_CODEX_TIMEOUT", opts.CodexTimeout))
	codexStart := time.Now()
	codex := backend.Execute(ctx, BackendOptions{
		WorkDir:     wtDir,
		PromptFile:  promptFile,
		SummaryFile: summaryFile,
		LogBase:     codexLogBase,
		MaxAttempts: retryCount + 1,
		RetryDelay:  retryDelay,
		Timeout:     resolveTimeout("AI_CODEX_TIMEOUT", opts.CodexTimeout),
		Trace:       trace,
	})
	codexDuration := time.Since(codexStart)
	logger.Log("AI worker 結束 (exit=%d, duration=%s, retries=%d)", codex.ExitCode, codexDuration.Truncate(time.Second), codex.RetryCount)

	appendGitStatusDiff(ctx, wtDir, summaryFile)

	if codex.ExitCode != 0 {
		logger.Log("✗ AI worker 失敗: %s (stage=%s)", codex.FailureReason, codex.FailureStage)
		runErr = fmt.Errorf("%s", codex.FailureReason)
		result.ExitCode = codex.ExitCode
		result.Error = codex.FailureReason
		result.Status = "failed"

		execDuration := time.Since(startTime)
		_ = writeIssueResult(ctx, stateRoot, issueResultContext{
			IssueID:               opts.IssueID,
			Status:                "failed",
			RepoName:              repoName,
			RepoType:              repoType,
			RepoPath:              repoPath,
			WorktreeDir:           wtDir,
			WorkDir:               workDir,
			Branch:                branch,
			BaseBranch:            prBase,
			SummaryFile:           summaryFile,
			ErrorMessage:          codex.FailureReason,
			FailureStage:          codex.FailureStage,
			WorkerSessionID:       workerSessionID,
			AttemptNumber:         attemptInfo.AttemptNumber,
			PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
			PreviousFailureReason: attemptInfo.PreviousFailureReason,
			WorkerPID:             pidInfo.PID,
			WorkerStartTime:       pidInfo.StartTime,
			Duration:              execDuration,
			RetryCount:            codex.RetryCount,
		})
		resultWritten = true

		if trace != nil {
			trace.StepStart("worker_complete_comment")
		}
		postIssueComment(ctx, opts.IssueID, workerSessionID, "worker_complete", "",
			buildWorkerCompleteExtra("", execDuration, "failed", codex.ExitCode), opts.GHTimeout)
		if trace != nil {
			_ = trace.StepEnd("success", "", nil)
		}

		return result, runErr
	}

	if trace != nil {
		trace.StepStart("security_check")
	}

	if err := stageChanges(ctx, wtDir, repoName, opts.GitTimeout); err != nil {
		runErr = err
		result.Error = err.Error()
		if trace != nil {
			_ = trace.StepEnd("failed", "git add failed", nil)
		}
		return result, err
	}

	scriptViolations, err := CheckScriptModifications(ctx, wtDir, meta.AllowScriptChanges, meta.ScriptWhitelist)
	if err != nil {
		runErr = err
		result.Error = err.Error()
		if trace != nil {
			_ = trace.StepEnd("failed", "script modification not allowed", nil)
		}
		return result, err
	}
	if len(scriptViolations) > 0 {
		logger.Log("script modifications allowed: %s", strings.Join(scriptViolations, ", "))
	}

	secretMatches, err := CheckSensitiveInfo(ctx, wtDir, meta.AllowSecrets, meta.SecretPatterns)
	if err != nil {
		runErr = err
		result.Error = err.Error()
		if trace != nil {
			_ = trace.StepEnd("failed", "sensitive information detected", nil)
		}
		return result, err
	}
	if len(secretMatches) > 0 {
		logger.Log("sensitive patterns allowed: %s", strings.Join(secretMatches, ", "))
	}

	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	// Run verification tests before commit
	if trace != nil {
		trace.StepStart("run_tests")
	}

	// Try to get verification commands from ticket first, fallback to config.
	// Only the ticket itself names commands to run: a "## Verification"
	// heading in what the dispatch added (a review's findings) is not one.
	verifyCommands := GetVerificationCommandsForRepo(ticketText, repoName)
	if len(verifyCommands) == 0 {
		// Fallback to config's verify.build and verify.test
		verifyCommands = getConfigVerifyCommands(cfg, repoName)
		if len(verifyCommands) > 0 {
			logger.Log("使用設定檔的預設驗證命令")
		}
	}

	if len(verifyCommands) > 0 {
		logger.Log("執行驗證測試 (目錄: %s)...", workDir)
		for _, cmd := range verifyCommands {
			logger.Log("  運行: %s", cmd)
			if testErr := runVerificationCommand(ctx, workDir, cmd, opts.GitTimeout); testErr != nil {
				runErr = fmt.Errorf("verification failed: %s: %w", cmd, testErr)
				result.Error = runErr.Error()
				result.Status = "failed"
				if trace != nil {
					_ = trace.StepEnd("failed", fmt.Sprintf("test failed: %s: %v", cmd, testErr), nil)
				}
				logger.Log("  ✗ 測試失敗: %v", testErr)
				// Return error to trigger Codex retry
				return result, runErr
			}
			logger.Log("  ✓ 通過")
		}
		logger.Log("✓ 所有驗證測試通過")
	} else {
		logger.Log("未找到驗證命令，跳過測試")
	}

	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	if trace != nil {
		trace.StepStart("git_commit")
	}

	cleanupIndexLocks(wtDir, logger.Log)

	consistencyStatus := "consistent"
	submoduleState := &SubmoduleState{ConsistencyStatus: consistencyStatus}

	if repoType == "submodule" {
		outside, err := CheckSubmoduleBoundary(ctx, wtDir, repoPath, meta.AllowParentChanges)
		if err != nil {
			runErr = err
			result.Error = err.Error()
			if trace != nil {
				_ = trace.StepEnd("failed", "changes outside submodule boundary", nil)
			}
			return result, err
		}
		if len(outside) > 0 {
			logger.Log("changes outside submodule boundary allowed: %s", strings.Join(outside, ", "))
		}

		submoduleState, err = CommitSubmodule(ctx, wtDir, repoPath, commitMsg, opts.GitTimeout)
		if err != nil {
			runErr = err
			if errors.Is(err, ErrNoSubmoduleChanges) {
				result.Error = "no changes in submodule"
			} else {
				result.Error = err.Error()
			}
			consistencyStatus = submoduleState.ConsistencyStatus
			if trace != nil {
				_ = trace.StepEnd("failed", "submodule commit failed", nil)
			}

			_ = writeIssueResult(ctx, stateRoot, issueResultContext{
				IssueID:               opts.IssueID,
				Status:                "failed",
				RepoName:              repoName,
				RepoType:              repoType,
				RepoPath:              repoPath,
				WorktreeDir:           wtDir,
				WorkDir:               workDir,
				Branch:                branch,
				BaseBranch:            prBase,
				SummaryFile:           summaryFile,
				ErrorMessage:          result.Error,
				FailureStage:          "git_commit",
				WorkerSessionID:       workerSessionID,
				AttemptNumber:         attemptInfo.AttemptNumber,
				PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
				PreviousFailureReason: attemptInfo.PreviousFailureReason,
				WorkerPID:             pidInfo.PID,
				WorkerStartTime:       pidInfo.StartTime,
				Duration:              time.Since(startTime),
				RetryCount:            codex.RetryCount,
				SubmoduleSHA:          submoduleState.SubmoduleSHA,
				ConsistencyStatus:     consistencyStatus,
			})
			resultWritten = true
			return result, err
		}
		consistencyStatus = submoduleState.ConsistencyStatus
	} else {
		if err := runGit(ctx, wtDir, opts.GitTimeout, "diff", "--cached", "--quiet"); err == nil {
			// No staged changes. This is a legitimate "nothing to do" ONLY if the
			// worker actually ran to completion. If its output shows it was blocked
			// from writing (read-only sandbox / denied approval), a zero diff is a
			// FALSE success — fail so the issue retries and surfaces the real cause
			// instead of being silently closed with nothing implemented.
			if reason := workerWriteBlockedReason(summaryFile); reason != "" {
				result.Status = "failed"
				result.Error = "worker made no changes: blocked from writing (" + reason + ")"
				if trace != nil {
					_ = trace.StepEnd("failed", "worker blocked from writing", nil)
				}
				_ = writeIssueResult(ctx, stateRoot, issueResultContext{
					IssueID:               opts.IssueID,
					Status:                "failed",
					RepoName:              repoName,
					RepoType:              repoType,
					RepoPath:              repoPath,
					WorktreeDir:           wtDir,
					WorkDir:               workDir,
					Branch:                branch,
					BaseBranch:            prBase,
					SummaryFile:           summaryFile,
					ErrorMessage:          result.Error,
					FailureStage:          "worker_write_blocked",
					WorkerSessionID:       workerSessionID,
					AttemptNumber:         attemptInfo.AttemptNumber,
					PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
					PreviousFailureReason: attemptInfo.PreviousFailureReason,
					WorkerPID:             pidInfo.PID,
					WorkerStartTime:       pidInfo.StartTime,
					Duration:              time.Since(startTime),
					RetryCount:            codex.RetryCount,
				})
				resultWritten = true
				return result, fmt.Errorf("%s", result.Error)
			}

			// No staged changes - but if codex succeeded and tests passed, this is success_no_changes
			// (task was completed without requiring code changes)
			if trace != nil {
				_ = trace.StepEnd("success", "no changes needed", nil)
			}
			result.Status = "success_no_changes"
			_ = writeIssueResult(ctx, stateRoot, issueResultContext{
				IssueID:               opts.IssueID,
				Status:                "success_no_changes",
				RepoName:              repoName,
				RepoType:              repoType,
				RepoPath:              repoPath,
				WorktreeDir:           wtDir,
				WorkDir:               workDir,
				Branch:                branch,
				BaseBranch:            prBase,
				SummaryFile:           summaryFile,
				WorkerSessionID:       workerSessionID,
				AttemptNumber:         attemptInfo.AttemptNumber,
				PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
				PreviousFailureReason: attemptInfo.PreviousFailureReason,
				WorkerPID:             pidInfo.PID,
				WorkerStartTime:       pidInfo.StartTime,
				Duration:              time.Since(startTime),
				RetryCount:            codex.RetryCount,
			})
			resultWritten = true
			// Return success (no error) for success_no_changes
			return result, nil
		}

		if err := runGit(ctx, wtDir, opts.GitTimeout, "commit", "-m", commitMsg); err != nil {
			runErr = err
			result.Error = err.Error()
			if trace != nil {
				_ = trace.StepEnd("failed", "git commit failed", nil)
			}
			return result, err
		}
	}

	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	if trace != nil {
		trace.StepStart("git_push")
	}

	if repoType == "submodule" {
		pushState, err := PushSubmodule(ctx, wtDir, repoPath, branch, opts.GitTimeout)
		if err != nil {
			runErr = err
			result.Error = err.Error()
			consistencyStatus = pushState.ConsistencyStatus
			if trace != nil {
				_ = trace.StepEnd("failed", "submodule push failed", nil)
			}
			_ = writeIssueResult(ctx, stateRoot, issueResultContext{
				IssueID:               opts.IssueID,
				Status:                "failed",
				RepoName:              repoName,
				RepoType:              repoType,
				RepoPath:              repoPath,
				WorktreeDir:           wtDir,
				WorkDir:               workDir,
				Branch:                branch,
				BaseBranch:            prBase,
				SummaryFile:           summaryFile,
				ErrorMessage:          result.Error,
				FailureStage:          "git_push",
				WorkerSessionID:       workerSessionID,
				AttemptNumber:         attemptInfo.AttemptNumber,
				PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
				PreviousFailureReason: attemptInfo.PreviousFailureReason,
				WorkerPID:             pidInfo.PID,
				WorkerStartTime:       pidInfo.StartTime,
				Duration:              time.Since(startTime),
				RetryCount:            codex.RetryCount,
				SubmoduleSHA:          submoduleState.SubmoduleSHA,
				ConsistencyStatus:     consistencyStatus,
			})
			resultWritten = true
			return result, err
		}
		consistencyStatus = pushState.ConsistencyStatus
	} else {
		if err := GitPush(ctx, wtDir, branch, opts.GitTimeout); err != nil {
			runErr = err
			result.Error = err.Error()
			if trace != nil {
				_ = trace.StepEnd("failed", "git push failed", nil)
			}
			return result, err
		}
	}

	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	if trace != nil {
		trace.StepStart("create_pr")
	}
	planContent := resolvePlanContent(workDir, runDir, opts.IssueID)
	prURL, err := createOrFindPR(ctx, branch, prBase, commitMsg, opts.IssueID, opts.GHTimeout, ghRetryConfig, planContent)
	if err != nil {
		runErr = err
		result.Error = err.Error()
		if trace != nil {
			_ = trace.StepEnd("failed", "PR not created", nil)
		}
		_ = writeIssueResult(ctx, stateRoot, issueResultContext{
			IssueID:               opts.IssueID,
			Status:                "failed",
			RepoName:              repoName,
			RepoType:              repoType,
			RepoPath:              repoPath,
			WorktreeDir:           wtDir,
			WorkDir:               workDir,
			Branch:                branch,
			BaseBranch:            prBase,
			SummaryFile:           summaryFile,
			ErrorMessage:          result.Error,
			FailureStage:          "create_pr",
			WorkerSessionID:       workerSessionID,
			AttemptNumber:         attemptInfo.AttemptNumber,
			PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
			PreviousFailureReason: attemptInfo.PreviousFailureReason,
			WorkerPID:             pidInfo.PID,
			WorkerStartTime:       pidInfo.StartTime,
			Duration:              time.Since(startTime),
			RetryCount:            codex.RetryCount,
			SubmoduleSHA:          submoduleState.SubmoduleSHA,
			ConsistencyStatus:     consistencyStatus,
		})
		resultWritten = true
		return result, err
	}
	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	// On retry attempts, reply to previous review comments to signal fixes
	if attemptInfo.AttemptNumber > 1 {
		replyToReviewComments(ctx, prURL, opts.GHTimeout)
	}

	if err := writeIssueResult(ctx, stateRoot, issueResultContext{
		IssueID:               opts.IssueID,
		Status:                "success",
		PRURL:                 prURL,
		RepoName:              repoName,
		RepoType:              repoType,
		RepoPath:              repoPath,
		WorktreeDir:           wtDir,
		WorkDir:               workDir,
		Branch:                branch,
		BaseBranch:            prBase,
		SummaryFile:           summaryFile,
		WorkerSessionID:       workerSessionID,
		AttemptNumber:         attemptInfo.AttemptNumber,
		PreviousSessionIDs:    attemptInfo.PreviousSessionIDs,
		PreviousFailureReason: attemptInfo.PreviousFailureReason,
		WorkerPID:             pidInfo.PID,
		WorkerStartTime:       pidInfo.StartTime,
		Duration:              time.Since(startTime),
		RetryCount:            codex.RetryCount,
		SubmoduleSHA:          submoduleState.SubmoduleSHA,
		ConsistencyStatus:     consistencyStatus,
	}); err != nil {
		runErr = err
		result.Error = err.Error()
		return result, err
	}
	resultWritten = true

	if trace != nil {
		trace.StepStart("worker_complete_comment")
	}
	postIssueComment(ctx, opts.IssueID, workerSessionID, "worker_complete", prURL,
		buildWorkerCompleteExtra(prURL, time.Since(startTime), "", 0), opts.GHTimeout)
	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
	}

	resetFailCount(runDir, logger.Log)

	result.Status = "success"
	result.PRURL = prURL
	result.ExitCode = 0
	return result, nil
}

// fillTimeouts gives the timeouts the caller left open the values of the
// configuration. A timeout the caller set stays.
func (opts *RunIssueOptions) fillTimeouts(timeouts workflowTimeouts) {
	defaults := config.Default().Timeouts
	pick := func(configured, fallback int) int {
		if configured > 0 {
			return configured
		}
		return fallback
	}
	if opts.GHTimeout == 0 {
		opts.GHTimeout = time.Duration(pick(timeouts.GHSeconds, defaults.GHSeconds)) * time.Second
	}
	if opts.GitTimeout == 0 {
		opts.GitTimeout = time.Duration(pick(timeouts.GitSeconds, defaults.GitSeconds)) * time.Second
	}
	if opts.CodexTimeout == 0 {
		opts.CodexTimeout = time.Duration(pick(timeouts.CodexMinutes, defaults.CodexMinutes)) * time.Minute
	}
}

func loadWorkflowConfig(path string) (*workflowConfig, error) {
	return config.LoadFile(path)
}

// resolveRepoConfig returns the type and the path of the repo a ticket names.
// A repo the configuration does not know, or says nothing more about, is a
// directory at the root.
func resolveRepoConfig(cfg *workflowConfig, repoName string) (string, string) {
	repoType, repoPath := "directory", "./"
	if cfg == nil {
		return repoType, repoPath
	}
	if repo := cfg.FindRepo(repoName); repo != nil {
		if repo.Type != "" {
			repoType = repo.Type
		}
		if repo.Path != "" {
			repoPath = repo.Path
		}
	}
	return repoType, repoPath
}

// getConfigVerifyCommands returns the build and test commands the
// configuration gives the repo a ticket names.
func getConfigVerifyCommands(cfg *workflowConfig, repoName string) []string {
	if cfg == nil {
		return nil
	}
	return cfg.GetVerifyCommands(repoName)
}

// getSetupCommand returns the setup command for a repo.
// Explicit verify.setup takes priority; otherwise auto-detect from language/package_manager.
func getSetupCommand(cfg *workflowConfig, repoName string) string {
	if cfg == nil {
		return ""
	}
	repo := cfg.FindRepo(repoName)
	if repo == nil {
		return ""
	}
	if repo.Verify.Setup != "" {
		return repo.Verify.Setup
	}
	return inferSetupCommand(repo.Language, repo.PackageManager)
}

// inferSetupCommand returns a dependency install command based on language and package manager.
func inferSetupCommand(language, packageManager string) string {
	switch repocfg.ToolchainOf(language) {
	case repocfg.ToolchainNode:
		switch strings.ToLower(packageManager) {
		case "yarn":
			return "yarn install --frozen-lockfile 2>/dev/null || yarn install"
		case "pnpm":
			return "pnpm install --frozen-lockfile 2>/dev/null || pnpm install"
		default:
			return "npm ci 2>/dev/null || npm install"
		}
	case repocfg.ToolchainPython:
		return "pip install -r requirements.txt 2>/dev/null || true"
	case repocfg.ToolchainDotnet:
		return "dotnet restore"
	default:
		// Go fetches its modules as it builds; any other language says what
		// it needs in verify.setup.
		return ""
	}
}

func extractTitleLine(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "#") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// workerPromptInputs is everything the Worker prompt is assembled from.
// writePromptFile gathers it (GitHub, design docs, knowledge graph, lessons);
// buildWorkerPrompt only lays it out, so the prompt text can be tested without
// touching the network or the state directory.
type workerPromptInputs struct {
	WorkDirInstruction string
	Ticket             string
	IssueID            int
	GraphSection       string // ticket-relevant slice of the knowledge graph
	ReviewComments     string // earlier AWK Review feedback on the issue
	PRReviewComments   string // file-level comments from the previous review
	LessonsSection     string // distilled lessons relevant to the ticket
	TopCategorySection string // most common rejection categories
}

func writePromptFile(path, workDirInstruction, ticket, stateRoot string, issueID int, prNumber int, ghTimeout time.Duration, feedbackCfg *workflowFeedback, workerCfg *workflowWorker, lessonsCfg *workflowLessons) error {
	in := workerPromptInputs{
		WorkDirInstruction: workDirInstruction,
		Ticket:             ticket,
		IssueID:            issueID,
	}
	in.ReviewComments = fetchReviewComments(issueID, ghTimeout)
	in.PRReviewComments = fetchPRReviewComments(prNumber, ghTimeout)
	// A ticket-relevant slice of the codebase knowledge graph, when one is
	// present (best-effort grounding; see knowledgegraph.go).
	in.GraphSection = loadKnowledgeGraphContext(stateRoot, issueID, ticket, workerCfg)
	// Distilled lessons relevant to this ticket (learning loop; replaces the
	// old raw replay of recent rejections) plus the cheap top-category
	// one-liners.
	in.LessonsSection = loadLessonsSection(stateRoot, issueID, ticket, lessonsCfg)
	if feedbackCfg == nil || feedbackCfg.IsEnabled() {
		in.TopCategorySection = loadTopCategorySummary(stateRoot)
	}

	return os.WriteFile(path, []byte(buildWorkerPrompt(in)), 0644)
}

// buildWorkerPrompt lays out the Worker prompt from already gathered inputs.
func buildWorkerPrompt(in workerPromptInputs) string {
	builder := strings.Builder{}
	builder.WriteString("You are an automated coding agent running inside a git worktree.\n\n")
	builder.WriteString("Repo rules:\n")
	builder.WriteString("- Read and follow AGENTS.md.\n")
	builder.WriteString("- Keep changes minimal and strictly within ticket scope.\n")
	if in.WorkDirInstruction != "" {
		builder.WriteString(in.WorkDirInstruction)
		builder.WriteString("\n")
	}
	builder.WriteString("IMPORTANT: Do NOT run any git commands (commit, push, etc.) or create PRs.\n")
	builder.WriteString("The runner script will handle git operations after you complete the code changes.\n")
	builder.WriteString("Your job is ONLY to:\n")
	builder.WriteString("1. Write/modify code files\n")
	builder.WriteString("2. Run verification commands\n")
	builder.WriteString("3. Report the results\n\n")
	builder.WriteString("============================================================\n")
	builder.WriteString("FORBIDDEN OPERATIONS\n")
	builder.WriteString("============================================================\n")
	builder.WriteString("You MUST NOT:\n")
	builder.WriteString("- Read, write, or access any files in .ai/state/principal/\n")
	builder.WriteString("- Read, write, or access session.json or any session log files\n")
	builder.WriteString("- Access or modify Principal session data\n")
	builder.WriteString("- Forge or manipulate session IDs\n")
	builder.WriteString("- Access audit logs or review data\n")
	builder.WriteString("- Modify the workflow's own files (" + protectedPathList() + ") unless the ticket's `" + ticket.Info + "` block sets `allow_script_changes: true`\n\n")
	builder.WriteString("These paths are reserved for the Principal agent and are protected.\n")
	builder.WriteString("Attempting to access them will result in task failure.\n")
	builder.WriteString("============================================================\n\n")

	if in.GraphSection != "" {
		builder.WriteString("============================================================\n")
		builder.WriteString("CODEBASE MAP\n")
		builder.WriteString("============================================================\n")
		builder.WriteString(in.GraphSection)
		builder.WriteString("============================================================\n\n")
	}

	builder.WriteString("Ticket:\n")
	builder.WriteString(in.Ticket)
	builder.WriteString("\n")

	if in.ReviewComments != "" {
		builder.WriteString("\n============================================================\n")
		builder.WriteString("PREVIOUS REVIEW FEEDBACK (IMPORTANT - Address these issues!)\n")
		builder.WriteString("============================================================\n")
		builder.WriteString(in.ReviewComments)
		builder.WriteString("\n============================================================\n")
	}

	if in.PRReviewComments != "" {
		builder.WriteString("\n============================================================\n")
		builder.WriteString("FILE-LEVEL REVIEW COMMENTS (from previous review):\n")
		builder.WriteString("============================================================\n")
		builder.WriteString(in.PRReviewComments)
		builder.WriteString("============================================================\n")
	}

	if in.LessonsSection != "" {
		builder.WriteString("\n============================================================\n")
		builder.WriteString(in.LessonsSection)
		builder.WriteString("============================================================\n")
	}
	if in.TopCategorySection != "" {
		builder.WriteString("\n============================================================\n")
		builder.WriteString(in.TopCategorySection)
		builder.WriteString("============================================================\n")
	}

	builder.WriteString("\nBEFORE making code changes, create an implementation plan file at:\n")
	builder.WriteString(fmt.Sprintf(layout.RunsDir+"/issue-%d/plan.md\n\n", in.IssueID))
	builder.WriteString("The plan file MUST contain:\n")
	builder.WriteString("- ## Summary — brief description of the approach (1-3 sentences)\n")
	builder.WriteString("- ## Files to modify — list of files and what changes\n")
	builder.WriteString("- ## Key decisions — important implementation decisions and rationale\n")
	builder.WriteString("Keep the plan concise. This will be included in the PR description for reviewers.\n")

	builder.WriteString("\nAfter making changes:\n")
	builder.WriteString("- Run the verification commands (the ticket's, or the defaults in AGENTS.md).\n")
	builder.WriteString("- Do NOT commit or push - the runner will handle that.\n")

	return builder.String()
}

func buildWorkDirInstruction(repoType, repoPath, workDir, repoName string) string {
	// Use IsRootPath for consistent root path detection
	if util.IsRootPath(repoPath) {
		repoPath = repoName
	} else {
		repoPath = util.NormalizePath(repoPath)
	}

	// Validate repoPath to prevent path traversal attacks
	// - Must not contain ".." components
	// - Must not be an absolute path
	// - Must not contain suspicious characters
	if !isValidRepoPath(repoPath) {
		// Fallback to safe default
		repoPath = repoName
	}

	switch repoType {
	case "directory":
		return fmt.Sprintf("IMPORTANT: You are working in a MONOREPO (directory type).\n- Working directory: %s\n- All file paths should be relative to the worktree root\n- Example: %s/internal/foo.go (not internal/foo.go)\n", workDir, repoPath)
	case "submodule":
		return fmt.Sprintf("IMPORTANT: You are working in a SUBMODULE within a monorepo.\n- Submodule path: %s\n- Working directory: %s\n- WARNING: Do NOT modify files outside the submodule directory!\n- All changes must be within: %s/\n- Files outside this boundary will cause the commit to fail.\n", repoPath, workDir, repoPath)
	default:
		return ""
	}
}

// isValidGitRef validates that a git ref name is safe and well-formed.
// It checks for common unsafe patterns that could indicate injection attempts.
func isValidGitRef(ref string) bool {
	if ref == "" {
		return false
	}

	// Git ref names cannot contain these characters
	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\", "\x00"}
	for _, c := range invalidChars {
		if strings.Contains(ref, c) {
			return false
		}
	}

	// Cannot start or end with a dot or slash
	if strings.HasPrefix(ref, ".") || strings.HasSuffix(ref, ".") {
		return false
	}
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") {
		return false
	}

	// Cannot contain consecutive dots (..)
	if strings.Contains(ref, "..") {
		return false
	}

	// Cannot contain consecutive slashes
	if strings.Contains(ref, "//") {
		return false
	}

	// Cannot end with .lock
	if strings.HasSuffix(ref, ".lock") {
		return false
	}

	// Cannot contain @{ sequence
	if strings.Contains(ref, "@{") {
		return false
	}

	return true
}

// isValidRepoPath validates that a repo path is safe and does not attempt path traversal.
func isValidRepoPath(repoPath string) bool {
	if repoPath == "" {
		return false
	}

	// Check for absolute paths (Unix and Windows)
	// filepath.IsAbs is platform-dependent: on Windows "/x" and "\x" are
	// drive-relative (not absolute) yet still rooted, so reject any leading
	// separator explicitly to keep behavior identical across platforms.
	if filepath.IsAbs(repoPath) {
		return false
	}
	if repoPath[0] == '/' || repoPath[0] == '\\' {
		return false
	}
	if len(repoPath) >= 2 && repoPath[1] == ':' {
		return false // Windows drive letter like "C:"
	}

	// Check for path traversal attempts
	// Split by both / and \ to handle cross-platform paths
	parts := strings.FieldsFunc(repoPath, func(r rune) bool {
		return r == '/' || r == '\\'
	})

	for _, part := range parts {
		if part == ".." {
			return false
		}
	}

	// Check for suspicious patterns
	if strings.Contains(repoPath, "..") {
		return false
	}

	return true
}

func fetchReviewComments(issueID int, ghTimeout time.Duration) string {
	if _, err := exec.LookPath("gh"); err != nil {
		return ""
	}

	// ghTimeout should be set from workflow.yaml config (timeouts.gh_seconds)
	// or default to 60s in RunIssue. This is a safety fallback.
	if ghTimeout <= 0 {
		ghTimeout = 60 * time.Second
	}

	ctx, cancel := withOptionalTimeout(context.Background(), ghTimeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "view", fmt.Sprintf("%d", issueID), "--json", "comments")
	if err != nil {
		return ""
	}

	var payload struct {
		Comments []issueComment `json:"comments"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return ""
	}

	result, ignored := selectReviewFeedback(payload.Comments)
	if ignored > 0 {
		fmt.Fprintf(os.Stderr, "[WORKER] ignored %d 'AWK Review' comment(s) on issue #%d from accounts without write access\n", ignored, issueID)
	}
	if len(result) > 4000 {
		result = result[len(result)-4000:]
	}
	return result
}

// replyToReviewComments fetches PR review comments and posts a reply to each
// indicating the issues have been addressed. This is called on retry attempts
// (attempt > 1) after the worker pushes fixes.
// Failures are logged but do not block the workflow.
func replyToReviewComments(ctx context.Context, prURL string, ghTimeout time.Duration) {
	if _, err := exec.LookPath("gh"); err != nil {
		return
	}

	prNumber := ExtractPRNumber(prURL)
	if prNumber == 0 {
		return
	}

	if ghTimeout <= 0 {
		ghTimeout = 60 * time.Second
	}

	ctx, cancel := withOptionalTimeout(ctx, ghTimeout)
	defer cancel()

	// Fetch PR review comments
	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(),
		"gh", "api",
		fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", prNumber),
		"--paginate",
		"--jq", ".[].id",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] replyToReviewComments: failed to fetch comments for PR #%d: %v\n", prNumber, err)
		return
	}

	commentIDs := parseCommentIDs(string(output))
	if len(commentIDs) == 0 {
		return
	}

	for _, commentID := range commentIDs {
		if _, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(),
			"gh", "api",
			"--method", "POST",
			fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments/%d/replies", prNumber, commentID),
			"-f", "body="+reviewer.AddressedReplyBody,
		); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] replyToReviewComments: failed to reply to comment %d on PR #%d: %v\n", commentID, prNumber, err)
			// Continue to next comment; don't block on individual failures
		}
	}
}

// parseCommentIDs parses newline-separated comment IDs from gh api output.
func parseCommentIDs(output string) []int {
	var ids []int
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func fetchPRReviewComments(prNumber int, ghTimeout time.Duration) string {
	if prNumber <= 0 {
		return ""
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return ""
	}

	if ghTimeout <= 0 {
		ghTimeout = 60 * time.Second
	}

	ctx, cancel := withOptionalTimeout(context.Background(), ghTimeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "api",
		fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", prNumber),
	)
	if err != nil {
		return ""
	}

	var comments []prReviewComment
	if err := json.Unmarshal(output, &comments); err != nil {
		return ""
	}

	// The viewer's login is only needed to vouch for a comment whose author
	// association alone is not enough (an app token, for instance).
	viewer := ""
	for _, c := range comments {
		if !trustedAuthorAssociation(c.AuthorAssociation) {
			viewer = ghViewerLogin(ctx)
			break
		}
	}

	lines, ignored := trustedPRReviewLines(comments, viewer)
	if ignored > 0 {
		fmt.Fprintf(os.Stderr, "[WORKER] ignored %d review comment(s) on PR #%d from accounts without write access\n", ignored, prNumber)
	}
	return formatPRReviewComments(lines)
}

// formatPRReviewComments parses raw tab-delimited PR review comment lines
// and formats them into structured feedback. Each input line has the format:
// path\tline\tbody
func formatPRReviewComments(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	builder := strings.Builder{}
	totalLen := 0
	const maxTotal = 4000
	const maxBodyLen = 200

	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		path := strings.TrimSpace(parts[0])
		lineNum := strings.TrimSpace(parts[1])
		body := strings.TrimSpace(parts[2])

		if path == "" || body == "" {
			continue
		}

		// Collapse newlines in body to spaces
		body = strings.Join(strings.Fields(body), " ")

		if len(body) > maxBodyLen {
			body = body[:maxBodyLen] + "..."
		}

		entry := fmt.Sprintf("- **%s:%s** → %s\n", path, lineNum, body)

		if totalLen+len(entry) > maxTotal {
			break
		}
		builder.WriteString(entry)
		totalLen += len(entry)
	}

	return builder.String()
}

func loadTopCategorySummary(stateRoot string) string {
	entries, err := reviewer.LoadFeedback(stateRoot)
	if err != nil || len(entries) == 0 {
		return ""
	}
	return reviewer.FormatTopCategoriesForPrompt(entries, 3)
}

func runAttemptGuard(ctx context.Context, stateRoot string, issueID int, logFile string, opts RunIssueOptions) error {
	guard := NewAttemptGuard(stateRoot, issueID)
	result, err := guard.Check()
	if err != nil {
		return fmt.Errorf("attempt_guard: %w", err)
	}
	if !result.CanProceed {
		return fmt.Errorf("attempt_guard: %s (attempt %d)", result.Reason, result.AttemptNumber)
	}
	return nil
}

func runPreflight(ctx context.Context, stateRoot, repoType, repoPath, logFile string, opts RunIssueOptions) error {
	preflight := NewWorkerPreflight(stateRoot, repoType, repoPath)
	preflight.Timeout = opts.GitTimeout
	result, err := preflight.Check(ctx)
	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	if !result.Passed {
		return fmt.Errorf("preflight: %s", result.Message)
	}
	return nil
}

func isWorkingTreeDirty(ctx context.Context, dir string) (bool, string) {
	output, err := gitOutput(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, ""
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return false, ""
	}
	return true, output
}

// workflowManagedPaths are path prefixes that autoCleanRootRepository is
// allowed to reset. Changes outside these paths are left untouched so that
// user modifications (e.g. edited config files) are never silently discarded.
var workflowManagedPaths = []string{
	".ai/",
	".worktrees/",
}

// autoCleanRootRepository cleans workflow-managed paths in the root repository
// before preflight check. Only .ai/ and .worktrees/ are cleaned; user files
// outside these paths are preserved.
func autoCleanRootRepository(ctx context.Context, stateRoot string, timeout time.Duration, logf func(string, ...interface{})) error {
	// Step 1: Remove stale index lock
	lockPath := filepath.Join(stateRoot, ".git", "index.lock")
	if _, err := os.Stat(lockPath); err == nil {
		if err := os.Remove(lockPath); err != nil {
			if logf != nil {
				logf("WARNING: failed to remove index.lock: %v", err)
			}
		} else if logf != nil {
			logf("Removed stale .git/index.lock")
		}
	}

	// Step 2: Check if working tree is dirty at all
	dirty, _ := isWorkingTreeDirty(ctx, stateRoot)
	if !dirty {
		return nil // Nothing to clean
	}

	// Step 3: Restore tracked files and clean untracked files in workflow-managed paths only.
	// This avoids parsing porcelain output and directly targets workflow directories.
	cleaned := false
	for _, prefix := range workflowManagedPaths {
		managedDir := filepath.Join(stateRoot, prefix)
		if _, err := os.Stat(managedDir); err != nil {
			continue // directory doesn't exist, skip
		}

		// Restore tracked changes in this path (best-effort)
		if err := runGit(ctx, stateRoot, timeout, "checkout", "HEAD", "--", prefix); err != nil {
			if logf != nil {
				logf("git checkout %s partial: %v", prefix, err)
			}
		} else {
			cleaned = true
		}

		// Remove untracked files in this path
		if err := runGit(ctx, stateRoot, timeout, "clean", "-fd", prefix); err != nil {
			if logf != nil {
				logf("WARNING: git clean %s failed: %v", prefix, err)
			}
		} else {
			cleaned = true
		}
	}

	if cleaned && logf != nil {
		logf("Successfully cleaned workflow-managed files in root repository")
	}
	return nil
}

func appendGitStatusDiff(ctx context.Context, wtDir, summaryFile string) {
	handle, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() {
		_ = handle.Close() // best-effort close, error intentionally ignored
	}()

	status, _ := gitOutput(ctx, wtDir, "status", "--porcelain")
	diff, _ := gitOutput(ctx, wtDir, "diff")

	fmt.Fprintln(handle, "=== git status ===")
	fmt.Fprintln(handle, strings.TrimSpace(status))
	fmt.Fprintln(handle, "")
	fmt.Fprintln(handle, "=== git diff ===")
	fmt.Fprintln(handle, diff)
}

func stageChanges(ctx context.Context, wtDir, repoName string, timeout time.Duration) error {
	if err := runGit(ctx, wtDir, timeout, "add", "-A"); err != nil {
		return err
	}
	if repoName == "root" {
		// What a run leaves behind is never part of a commit, whatever the
		// project's ignore file says. The project's configuration and roster
		// are staged like any other file: a change to them is then checked
		// like a change to any control file, where it used to be dropped
		// without a word.
		_ = runGit(ctx, wtDir, timeout, "reset", "-q", "--", layout.StateDir, ".worktrees")
	}
	return nil
}

// readPlanFile reads the implementation plan file created by the Worker.
// Returns empty string if the file does not exist or cannot be read.
func readPlanFile(runDir string) string {
	planPath := filepath.Join(runDir, "plan.md")
	data, err := os.ReadFile(planPath)
	if err != nil {
		return ""
	}
	plan := strings.TrimSpace(string(data))
	if plan == "" {
		return ""
	}
	return plan
}

// resolvePlanContent finds the Worker's plan.md. The prompt names a path
// relative to the Worker's working directory, which is inside the worktree,
// so look there first; runDir (under the state root) stays as a fallback.
func resolvePlanContent(workDir, runDir string, issueID int) string {
	if workDir != "" {
		inWorktree := layout.Run(workDir, issueID)
		if plan := readPlanFile(inWorktree); plan != "" {
			return plan
		}
	}
	return readPlanFile(runDir)
}

// buildPRBody constructs the PR body with optional implementation plan.
func buildPRBody(issueID int, title, plan string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Closes #%d\n", issueID))

	if plan != "" {
		b.WriteString("\n## Implementation Plan\n\n")
		b.WriteString(plan)
		b.WriteString("\n")
	}

	b.WriteString("\n## Changes\n\n")
	b.WriteString(title)
	b.WriteString("\n")

	return b.String()
}

func createOrFindPR(ctx context.Context, branch, base, title string, issueID int, timeout time.Duration, retryCfg ghutil.RetryConfig, planContent string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found in PATH")
	}

	client := NewGitHubClient(timeout)
	client.Retry = retryCfg
	prInfo, err := client.GetPRByBranch(ctx, branch)
	if err != nil {
		return "", err
	}
	if prInfo != nil && prInfo.URL != "" {
		return prInfo.URL, nil
	}

	body := buildPRBody(issueID, title, planContent)
	prCtx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(prCtx, retryCfg, "gh", "pr", "create",
		"--base", base,
		"--head", branch,
		"--title", title,
		"--body", body,
	)
	if err != nil {
		return "", fmt.Errorf("gh pr create failed: %w", err)
	}

	re := regexp.MustCompile(`https://github\.com/[^\s]+/pull/\d+`)
	match := re.FindString(string(output))
	if match == "" {
		return "", fmt.Errorf("PR not created or URL not found")
	}
	return match, nil
}

func postIssueComment(ctx context.Context, issueID int, sessionID, commentType, prURL, extraData string, timeout time.Duration) {
	if _, err := exec.LookPath("gh"); err != nil {
		return
	}
	ctx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()

	now := time.Now()
	// Format: "20:20:11 CST (12:20:11 UTC)"
	localTime := now.Format("15:04:05 MST")
	utcTime := now.UTC().Format("15:04:05 UTC")
	timestamp := fmt.Sprintf("%s (%s)", localTime, utcTime)

	body := strings.Builder{}
	// AWK marker format: <!-- AWK:session:SESSION_ID:COMMENT_TYPE[:PRURL] -->
	// This allows IssueMonitor.parseAWKComment to extract comment type and PR URL
	body.WriteString("<!-- AWK:session:")
	body.WriteString(sessionID)
	body.WriteString(":")
	body.WriteString(commentType)
	if prURL != "" {
		body.WriteString(":")
		body.WriteString(prURL)
	}
	body.WriteString(" -->\n")
	body.WriteString("**AWK Tracking**\n\n")
	body.WriteString("| Field | Value |\n")
	body.WriteString("|-------|-------|\n")
	body.WriteString(fmt.Sprintf("| Session | `%s` |\n", sessionID))
	body.WriteString(fmt.Sprintf("| Timestamp | %s |\n", timestamp))
	body.WriteString(fmt.Sprintf("| Type | %s |\n", commentType))
	// Note: prURL is included in AWK marker for IssueMonitor parsing
	// and in extraData table via buildWorkerCompleteExtra, so we don't duplicate it here
	if extraData != "" {
		body.WriteString(extraData)
		if !strings.HasSuffix(extraData, "\n") {
			body.WriteString("\n")
		}
	}

	if _, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "comment", fmt.Sprintf("%d", issueID), "--body", body.String()); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] postIssueComment: issue #%d type=%s: %v\n", issueID, commentType, err)
	}
}

func buildWorkerStartExtra(attempt int) string {
	if attempt > 1 {
		return fmt.Sprintf("| Attempt | %d |\n", attempt)
	}
	return ""
}

func buildWorkerCompleteExtra(prURL string, duration time.Duration, status string, exitCode int) string {
	lines := strings.Builder{}
	if status != "" {
		lines.WriteString(fmt.Sprintf("| Status | %s |\n", status))
	}
	if prURL != "" {
		lines.WriteString(fmt.Sprintf("| PR | %s |\n", prURL))
	}
	if duration > 0 {
		lines.WriteString(fmt.Sprintf("| Duration | %s |\n", formatDuration(int(duration.Seconds()))))
	}
	if exitCode != 0 {
		lines.WriteString(fmt.Sprintf("| Exit Code | %d |\n", exitCode))
	}
	return lines.String()
}

func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		mins := seconds / 60
		secs := seconds % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}

func resetFailCount(runDir string, logf func(string, ...interface{})) {
	failCount := filepath.Join(runDir, "fail_count.txt")
	if _, err := os.Stat(failCount); err == nil {
		_ = os.Remove(failCount)
		if logf != nil {
			logf("resetting fail_count (success)")
		}
	}
}

func cleanupIndexLocks(wtDir string, logf func(string, ...interface{})) {
	lockPath := filepath.Join(wtDir, ".git", "index.lock")
	if _, err := os.Stat(lockPath); err == nil {
		if logf != nil {
			logf("removing stale index.lock")
		}
		_ = os.Remove(lockPath)
	}

	dotGitPath := filepath.Join(wtDir, ".git")
	info, err := os.Stat(dotGitPath)
	if err != nil || info.IsDir() {
		return
	}

	data, err := os.ReadFile(dotGitPath)
	if err != nil {
		return
	}

	content := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(content, prefix) {
		return
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(content, prefix))
	if gitDir == "" {
		return
	}
	lockPath = filepath.Join(gitDir, "index.lock")
	if _, err := os.Stat(lockPath); err == nil {
		if logf != nil {
			logf("removing stale index.lock from gitdir")
		}
		_ = os.Remove(lockPath)
	}
}

func writeIssueResult(ctx context.Context, stateRoot string, info issueResultContext) error {
	if info.ConsistencyStatus == "" {
		info.ConsistencyStatus = "consistent"
	}

	workDir := info.WorkDir
	if workDir == "" {
		workDir = stateRoot
	}
	worktreeDir := info.WorktreeDir
	if worktreeDir == "" {
		worktreeDir = workDir
	}

	headSHA := ""
	if output, err := gitOutput(ctx, filepath.Clean(worktreeDir), "rev-parse", "HEAD"); err == nil {
		headSHA = strings.TrimSpace(output)
	}

	submoduleStatus := ""
	if _, err := os.Stat(filepath.Join(stateRoot, ".gitmodules")); err == nil {
		if output, err := gitOutput(ctx, stateRoot, "submodule", "status", "--recursive"); err == nil {
			submoduleStatus = strings.TrimSpace(output)
		}
	}

	recoveryCommand := ""
	if info.RepoType == "submodule" && info.ConsistencyStatus != "consistent" {
		switch info.ConsistencyStatus {
		case "submodule_committed_parent_failed":
			recoveryCommand = fmt.Sprintf("cd %s && git reset --hard HEAD~1", filepath.Join(worktreeDir, info.RepoPath))
		case "submodule_push_failed":
			recoveryCommand = fmt.Sprintf("cd %s && git push origin HEAD", filepath.Join(worktreeDir, info.RepoPath))
		case "parent_push_failed_submodule_pushed":
			recoveryCommand = fmt.Sprintf("cd %s && git push origin %s", worktreeDir, info.Branch)
		}
	}

	result := &IssueResult{
		IssueID:           fmt.Sprintf("%d", info.IssueID),
		Status:            info.Status,
		Repo:              info.RepoName,
		RepoType:          info.RepoType,
		WorkDir:           workDir,
		WorktreePath:      worktreeDir, // For conflict resolution
		Branch:            info.Branch,
		BaseBranch:        info.BaseBranch,
		HeadSHA:           headSHA,
		SubmoduleSHA:      info.SubmoduleSHA,
		ConsistencyStatus: info.ConsistencyStatus,
		FailureStage:      info.FailureStage,
		RecoveryCommand:   recoveryCommand,
		TimestampUTC:      time.Now().UTC().Format(time.RFC3339),
		PRURL:             info.PRURL,
		SummaryFile:       info.SummaryFile,
		SubmoduleStatus:   submoduleStatus,
		ErrorMessage:      info.ErrorMessage,
		Recoverable:       info.Recoverable,
		Session: SessionInfo{
			WorkerSessionID:       info.WorkerSessionID,
			AttemptNumber:         info.AttemptNumber,
			PreviousSessionIDs:    info.PreviousSessionIDs,
			PreviousFailureReason: info.PreviousFailureReason,
			WorkerPID:             info.WorkerPID,
			WorkerStartTime:       info.WorkerStartTime,
		},
		Metrics: ResultMetrics{
			DurationSeconds: int(info.Duration.Seconds()),
			RetryCount:      info.RetryCount,
		},
	}

	// Best-effort LLM usage from the worker log (zeros for text-only backends).
	logUsage := ScanUsageFromLog(layout.WorkerLog(stateRoot, info.IssueID))
	result.Metrics.TokensIn = logUsage.TokensIn
	result.Metrics.TokensOut = logUsage.TokensOut
	result.Metrics.CostUSD = logUsage.CostUSD

	return WriteResultAtomic(stateRoot, info.IssueID, result)
}

// maxPreviousSessionIDs limits the number of previous session IDs to retain
// to prevent unbounded memory growth
const maxPreviousSessionIDs = 10

func loadAttemptInfo(stateRoot string, issueID int) attemptInfo {
	// Use fail_count.txt as the authoritative source for attempt number
	// This ensures consistency with AttemptGuard.Check
	failCount := ReadFailCount(stateRoot, issueID)
	attempt := failCount + 1 // fail_count is 0-based, attempt is 1-based

	// Load previous session info from result file
	result, err := LoadResult(stateRoot, issueID)
	if err != nil || result == nil {
		return attemptInfo{AttemptNumber: attempt}
	}

	if result.Session.WorkerSessionID == "" {
		return attemptInfo{AttemptNumber: attempt}
	}

	previous := append([]string{}, result.Session.PreviousSessionIDs...)
	previous = append(previous, result.Session.WorkerSessionID)

	// Limit the number of previous session IDs to prevent unbounded growth
	if len(previous) > maxPreviousSessionIDs {
		previous = previous[len(previous)-maxPreviousSessionIDs:]
	}

	return attemptInfo{
		AttemptNumber:         attempt,
		PreviousSessionIDs:    previous,
		PreviousFailureReason: result.Session.PreviousFailureReason,
	}
}

func generateSessionID(role string) string {
	now := time.Now().UTC().Format("20060102-150405")
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		random = []byte{0, 0, 0, 0}
	}
	return fmt.Sprintf("%s-%s-%x", role, now, random)
}

func extractTicketValue(content, key string) string {
	key = regexp.QuoteMeta(key)
	patterns := []string{
		fmt.Sprintf(`(?im)^\s*[-*]\s*%s\s*:\s*([^\r\n]+)`, key),
		fmt.Sprintf(`(?im)\*\*%s\*\*\s*:\s*([^\r\n]+)`, key),
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(content); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func resolveTimeout(envVar string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return fallback
}

func logEarlyFailure(path, repoName, repoType, repoPath, branch, baseBranch, stage, message string) {
	handle, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() {
		_ = handle.Close() // best-effort close, error intentionally ignored
	}()

	fmt.Fprintln(handle, "============================================================")
	fmt.Fprintf(handle, "EARLY FAILURE LOG - issue-%s\n", filepath.Base(path))
	fmt.Fprintln(handle, "============================================================")
	fmt.Fprintf(handle, "Timestamp: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(handle, "Stage: %s\n", stage)
	fmt.Fprintf(handle, "Repo: %s\n", repoName)
	fmt.Fprintf(handle, "Repo Type: %s\n", repoType)
	fmt.Fprintf(handle, "Repo Path: %s\n", repoPath)
	fmt.Fprintf(handle, "Branch: %s\n", branch)
	fmt.Fprintf(handle, "Base Branch: %s\n", baseBranch)
	fmt.Fprintf(handle, "Error: %s\n", message)
	fmt.Fprintln(handle, "============================================================")
}

func resolveGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func newWorkerLogger(logFile, summaryFile string) *workerLogger {
	logger := &workerLogger{}
	if logFile != "" {
		if file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			logger.file = file
		}
	}
	if summaryFile != "" {
		if file, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			logger.summary = file
		}
	}
	return logger
}

func (l *workerLogger) Log(format string, args ...interface{}) {
	if l.file == nil {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf("[WORKER] %s | %s\n", timestamp, fmt.Sprintf(format, args...))
	_, _ = l.file.WriteString(message)
	if l.summary != nil {
		_, _ = l.summary.WriteString(message)
	}
}

func (l *workerLogger) Close() error {
	var errs []string
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("file: %v", err))
		}
	}
	if l.summary != nil {
		if err := l.summary.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("summary: %v", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to close logger: %s", strings.Join(errs, "; "))
	}
	return nil
}

// runVerificationCommand runs a verification command in the given directory
func runVerificationCommand(ctx context.Context, workDir, command string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use bash to run the command for shell expansion support
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workDir

	// Capture output for error reporting
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout after %v", timeout)
		}
		// Include stderr in error message for debugging
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput != "" {
			return fmt.Errorf("%w: %s", err, errOutput)
		}
		return err
	}

	return nil
}

// buildRetryConfig constructs a ghutil.RetryConfig from workflow.yaml timeout settings.
// Falls back to DefaultRetryConfig values when config fields are zero.
func buildRetryConfig(t workflowTimeouts) ghutil.RetryConfig {
	cfg := ghutil.DefaultRetryConfig()
	if t.GHRetryCount > 0 {
		cfg.MaxAttempts = t.GHRetryCount
	}
	if t.GHRetryBaseDelay > 0 {
		cfg.BaseDelay = time.Duration(t.GHRetryBaseDelay) * time.Second
	}
	return cfg
}
