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

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/util"
	"gopkg.in/yaml.v3"
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

type workflowConfig struct {
	Repos      []workflowRepo     `yaml:"repos"`
	Git        workflowGit        `yaml:"git"`
	Escalation workflowEscalation `yaml:"escalation"`
	Timeouts   workflowTimeouts   `yaml:"timeouts"`
	Feedback   workflowFeedback   `yaml:"feedback"`
	Worker     workflowWorker     `yaml:"worker"`
}

type workflowWorker struct {
	Backend    string                 `yaml:"backend"`
	ClaudeCode workflowClaudeCode     `yaml:"claude_code"`
}

type workflowClaudeCode struct {
	Model                      string `yaml:"model"`
	MaxTurns                   int    `yaml:"max_turns"`
	DangerouslySkipPermissions bool   `yaml:"dangerously_skip_permissions"`
}

type workflowFeedback struct {
	Enabled            *bool `yaml:"enabled"`
	MaxHistoryInPrompt int   `yaml:"max_history_in_prompt"`
}

func (f *workflowFeedback) isEnabled() bool {
	if f.Enabled == nil {
		return true
	}
	return *f.Enabled
}

func (f *workflowFeedback) maxHistory() int {
	if f.MaxHistoryInPrompt <= 0 {
		return 10
	}
	return f.MaxHistoryInPrompt
}

type workflowRepo struct {
	Name           string             `yaml:"name"`
	Path           string             `yaml:"path"`
	Type           string             `yaml:"type"`
	Language       string             `yaml:"language"`
	PackageManager string             `yaml:"package_manager"`
	Verify         workflowRepoVerify `yaml:"verify"`
}

type workflowRepoVerify struct {
	Setup string `yaml:"setup"`
	Build string `yaml:"build"`
	Test  string `yaml:"test"`
}

type workflowGit struct {
	IntegrationBranch string `yaml:"integration_branch"`
	ReleaseBranch     string `yaml:"release_branch"`
}

type workflowEscalation struct {
	RetryCount        int `yaml:"retry_count"`
	RetryDelaySeconds int `yaml:"retry_delay_seconds"`
}

type workflowTimeouts struct {
	GitSeconds      int `yaml:"git_seconds"`       // Git operations timeout (default: 120)
	GHSeconds       int `yaml:"gh_seconds"`        // GitHub CLI operations timeout (default: 60)
	CodexMinutes    int `yaml:"codex_minutes"`     // Codex execution timeout (default: 30)
	GHRetryCount    int `yaml:"gh_retry_count"`    // Max retry attempts for gh CLI calls (default: 3)
	GHRetryBaseDelay int `yaml:"gh_retry_base_delay"` // Base delay in seconds for retry backoff (default: 2)
}

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
	SpecName              string
	TaskLine              string
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

	meta := ParseTicketMetadata(string(ticketBody))
	repoName := strings.TrimSpace(opts.RepoOverride)
	if repoName == "" {
		repoName = meta.Repo
	}
	repoName = strings.ToLower(strings.TrimSpace(repoName))
	if repoName == "" {
		repoName = "root"
	}

	configPath := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	cfg, cfgErr := loadWorkflowConfig(configPath)
	if cfgErr != nil {
		cfg = &workflowConfig{}
	}

	// Apply configured timeouts (config overrides defaults, opts overrides config)
	if opts.GHTimeout == 0 {
		if cfg.Timeouts.GHSeconds > 0 {
			opts.GHTimeout = time.Duration(cfg.Timeouts.GHSeconds) * time.Second
		} else {
			opts.GHTimeout = 60 * time.Second
		}
	}
	if opts.GitTimeout == 0 {
		if cfg.Timeouts.GitSeconds > 0 {
			opts.GitTimeout = time.Duration(cfg.Timeouts.GitSeconds) * time.Second
		} else {
			opts.GitTimeout = 120 * time.Second
		}
	}
	if opts.CodexTimeout == 0 {
		if cfg.Timeouts.CodexMinutes > 0 {
			opts.CodexTimeout = time.Duration(cfg.Timeouts.CodexMinutes) * time.Minute
		} else {
			opts.CodexTimeout = 30 * time.Minute
		}
	}

	// Build retry config from workflow.yaml
	ghRetryConfig := buildRetryConfig(cfg.Timeouts)

	integrationBranch := cfg.Git.IntegrationBranch
	if integrationBranch == "" {
		integrationBranch = "develop"
	}
	releaseBranch := cfg.Git.ReleaseBranch
	if releaseBranch == "" {
		releaseBranch = "main"
	}

	retryCount := cfg.Escalation.RetryCount
	if retryCount == 0 {
		retryCount = 2
	}
	retryDelay := time.Duration(cfg.Escalation.RetryDelaySeconds) * time.Second
	if retryDelay == 0 {
		retryDelay = 5 * time.Second
	}

	validRepos := buildValidRepos(cfg)
	if !containsString(validRepos, repoName) {
		return nil, fmt.Errorf("repo must be one of: %s (got %q)", strings.Join(validRepos, ", "), repoName)
	}

	repoType, repoPath := resolveRepoConfig(cfg, repoName)
	if repoName == "root" {
		repoType = "root"
		repoPath = "./"
	}

	releaseFlag := meta.Release
	if !releaseFlag {
		if value := extractTicketValue(string(ticketBody), "Release"); value != "" {
			releaseFlag = parseBool(value)
		}
	}
	if releaseFlag && repoName != "root" {
		return nil, fmt.Errorf("release tickets are allowed only for root repo")
	}

	prBase := integrationBranch
	if releaseFlag {
		prBase = releaseBranch
	}

	branch := fmt.Sprintf("feat/ai-issue-%d", opts.IssueID)
	logDir := filepath.Join(stateRoot, ".ai", "exe-logs")
	runDir := filepath.Join(stateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", opts.IssueID))
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, ".ai", "results"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, ".ai", "state"), 0755); err != nil {
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
	logger.Log("worker_session_id=%s", workerSessionID)
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
	_ = WritePIDFile(stateRoot, opts.IssueID, pidInfo)

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
					SpecName:              resolveSpecName(meta),
					TaskLine:              resolveTaskLine(meta),
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

	titleLine := extractTitleLine(string(ticketBody))
	if titleLine == "" {
		titleLine = fmt.Sprintf("issue-%d", opts.IssueID)
	}
	commitMsg := BuildCommitMessage(titleLine)

	workDirInstruction := buildWorkDirInstruction(repoType, repoPath, workDir, repoName)
	promptFile := filepath.Join(runDir, "prompt.txt")
	if err := writePromptFile(promptFile, workDirInstruction, string(ticketBody), stateRoot, opts.IssueID, opts.GHTimeout, &cfg.Feedback); err != nil {
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

	appendGitStatusDiff(ctx, wtDir, summaryFile)

	if codex.ExitCode != 0 {
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
			SpecName:              resolveSpecName(meta),
			TaskLine:              resolveTaskLine(meta),
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

	allowScriptChanges := meta.AllowScriptChanges
	if value := extractTicketValue(string(ticketBody), "allow_script_changes"); value != "" {
		allowScriptChanges = parseBool(value)
	}
	scriptWhitelist := extractTicketValue(string(ticketBody), "script_whitelist")

	scriptViolations, err := CheckScriptModifications(ctx, wtDir, allowScriptChanges, scriptWhitelist)
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

	allowSecrets := meta.AllowSecrets
	if value := extractTicketValue(string(ticketBody), "allow_secrets"); value != "" {
		allowSecrets = parseBool(value)
	}
	customPatterns := strings.Fields(extractTicketValue(string(ticketBody), "secret_patterns"))

	secretMatches, err := CheckSensitiveInfo(ctx, wtDir, allowSecrets, customPatterns)
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

	// Run setup command (dependency install) before verification
	setupCmd := getSetupCommand(cfg, repoName)
	if setupCmd != "" {
		logger.Log("執行驗證前置設定...")
		logger.Log("  運行: %s", setupCmd)
		if setupErr := runVerificationCommand(ctx, workDir, setupCmd, opts.GitTimeout); setupErr != nil {
			runErr = fmt.Errorf("verification setup failed: %s: %w", setupCmd, setupErr)
			result.Error = runErr.Error()
			result.Status = "failed"
			if trace != nil {
				_ = trace.StepEnd("failed", fmt.Sprintf("setup failed: %s: %v", setupCmd, setupErr), nil)
			}
			logger.Log("  ✗ 設定失敗: %v", setupErr)
			return result, runErr
		}
		logger.Log("  ✓ 設定完成")
	}

	// Try to get verification commands from ticket first, fallback to config
	verifyCommands := GetVerificationCommandsForRepo(string(ticketBody), repoName)
	if len(verifyCommands) == 0 {
		// Fallback to config's verify.build and verify.test
		verifyCommands = getConfigVerifyCommands(cfg, repoName)
		if len(verifyCommands) > 0 {
			logger.Log("使用 workflow.yaml 預設驗證命令")
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
	allowParentChanges := meta.AllowParentChanges
	if value := extractTicketValue(string(ticketBody), "allow_parent_changes"); value != "" {
		allowParentChanges = parseBool(value)
	}

	if repoType == "submodule" {
		outside, err := CheckSubmoduleBoundary(ctx, wtDir, repoPath, allowParentChanges)
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
				SpecName:              resolveSpecName(meta),
				TaskLine:              resolveTaskLine(meta),
			})
			resultWritten = true
			return result, err
		}
		consistencyStatus = submoduleState.ConsistencyStatus
	} else {
		if err := runGit(ctx, wtDir, opts.GitTimeout, "diff", "--cached", "--quiet"); err == nil {
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
				SpecName:              resolveSpecName(meta),
				TaskLine:              resolveTaskLine(meta),
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
				SpecName:              resolveSpecName(meta),
				TaskLine:              resolveTaskLine(meta),
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
	prURL, err := createOrFindPR(ctx, branch, prBase, commitMsg, opts.IssueID, opts.GHTimeout, ghRetryConfig)
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
			SpecName:              resolveSpecName(meta),
			TaskLine:              resolveTaskLine(meta),
		})
		resultWritten = true
		return result, err
	}
	if trace != nil {
		_ = trace.StepEnd("success", "", nil)
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
		SpecName:              resolveSpecName(meta),
		TaskLine:              resolveTaskLine(meta),
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

func loadWorkflowConfig(path string) (*workflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg workflowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func buildValidRepos(cfg *workflowConfig) []string {
	valid := []string{"root"}
	if cfg == nil {
		return append(valid, "backend", "frontend")
	}
	for _, repo := range cfg.Repos {
		if repo.Name != "" && !containsString(valid, repo.Name) {
			valid = append(valid, repo.Name)
		}
	}
	return valid
}

func resolveRepoConfig(cfg *workflowConfig, repoName string) (string, string) {
	if cfg == nil {
		return "directory", "./"
	}
	for _, repo := range cfg.Repos {
		if repo.Name == repoName {
			repoType := repo.Type
			if repoType == "" {
				repoType = "directory"
			}
			repoPath := repo.Path
			if repoPath == "" {
				repoPath = "./"
			}
			return repoType, repoPath
		}
	}
	return "directory", "./"
}

// getConfigVerifyCommands returns verification commands from workflow.yaml config
func getConfigVerifyCommands(cfg *workflowConfig, repoName string) []string {
	if cfg == nil {
		return nil
	}
	for _, repo := range cfg.Repos {
		if repo.Name == repoName {
			var commands []string
			if repo.Verify.Build != "" {
				commands = append(commands, repo.Verify.Build)
			}
			if repo.Verify.Test != "" {
				commands = append(commands, repo.Verify.Test)
			}
			return commands
		}
	}
	return nil
}

// getSetupCommand returns the setup command for a repo.
// Explicit verify.setup takes priority; otherwise auto-detect from language/package_manager.
func getSetupCommand(cfg *workflowConfig, repoName string) string {
	if cfg == nil {
		return ""
	}
	for _, repo := range cfg.Repos {
		if repo.Name == repoName {
			if repo.Verify.Setup != "" {
				return repo.Verify.Setup
			}
			return inferSetupCommand(repo.Language, repo.PackageManager)
		}
	}
	return ""
}

// inferSetupCommand returns a dependency install command based on language and package manager.
func inferSetupCommand(language, packageManager string) string {
	switch strings.ToLower(language) {
	case "node", "nodejs", "typescript", "javascript",
		"react", "vue", "angular", "nextjs", "nuxt", "svelte",
		"express", "nestjs":
		switch strings.ToLower(packageManager) {
		case "yarn":
			return "yarn install --frozen-lockfile 2>/dev/null || yarn install"
		case "pnpm":
			return "pnpm install --frozen-lockfile 2>/dev/null || pnpm install"
		default:
			return "npm ci 2>/dev/null || npm install"
		}
	case "python", "django", "flask", "fastapi":
		return "pip install -r requirements.txt 2>/dev/null || true"
	case "dotnet", "csharp", "aspnet", "blazor":
		return "dotnet restore"
	default:
		// go, rust, unity, unreal, godot, generic — no setup needed
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

func writePromptFile(path, workDirInstruction, ticket, stateRoot string, issueID int, ghTimeout time.Duration, feedbackCfg *workflowFeedback) error {
	reviewComments := fetchReviewComments(issueID, ghTimeout)

	builder := strings.Builder{}
	builder.WriteString("You are an automated coding agent running inside a git worktree.\n\n")
	builder.WriteString("Repo rules:\n")
	builder.WriteString("- Read and follow AGENTS.md.\n")
	builder.WriteString("- Keep changes minimal and strictly within ticket scope.\n")
	if workDirInstruction != "" {
		builder.WriteString(workDirInstruction)
		builder.WriteString("\n")
	}
	builder.WriteString("IMPORTANT: Do NOT run any git commands (commit, push, etc.) or create PRs.\n")
	builder.WriteString("The runner script will handle git operations after you complete the code changes.\n")
	builder.WriteString("Your job is ONLY to:\n")
	builder.WriteString("1. Write/modify code files\n")
	builder.WriteString("2. Run verification commands\n")
	builder.WriteString("3. Report the results\n\n")
	builder.WriteString("============================================================\n")
	builder.WriteString("FORBIDDEN OPERATIONS (Req 3.4, 3.5)\n")
	builder.WriteString("============================================================\n")
	builder.WriteString("You MUST NOT:\n")
	builder.WriteString("- Read, write, or access any files in .ai/state/principal/\n")
	builder.WriteString("- Read, write, or access session.json or any session log files\n")
	builder.WriteString("- Modify any files in .ai/scripts/ or .ai/commands/\n")
	builder.WriteString("- Access or modify Principal session data\n")
	builder.WriteString("- Forge or manipulate session IDs\n")
	builder.WriteString("- Access audit logs or review data\n\n")
	builder.WriteString("These paths are reserved for the Principal agent and are protected.\n")
	builder.WriteString("Attempting to access them will result in task failure.\n")
	builder.WriteString("============================================================\n\n")
	builder.WriteString("Ticket:\n")
	builder.WriteString(ticket)
	builder.WriteString("\n")

	if reviewComments != "" {
		builder.WriteString("\n============================================================\n")
		builder.WriteString("PREVIOUS REVIEW FEEDBACK (IMPORTANT - Address these issues!)\n")
		builder.WriteString("============================================================\n")
		builder.WriteString(reviewComments)
		builder.WriteString("\n============================================================\n")
	}

	// Inject historical feedback patterns from past rejections
	maxEntries := 10
	feedbackEnabled := true
	if feedbackCfg != nil {
		feedbackEnabled = feedbackCfg.isEnabled()
		maxEntries = feedbackCfg.maxHistory()
	}
	if historicalFeedback := loadHistoricalFeedback(stateRoot, maxEntries); feedbackEnabled && historicalFeedback != "" {
		builder.WriteString("\n============================================================\n")
		builder.WriteString("HISTORICAL REVIEW PATTERNS (Learn from past rejections)\n")
		builder.WriteString("============================================================\n")
		builder.WriteString(historicalFeedback)
		builder.WriteString("============================================================\n")
	}

	builder.WriteString("\nAfter making changes:\n")
	builder.WriteString("- Print: git status --porcelain\n")
	builder.WriteString("- Print: git diff\n")
	builder.WriteString("- Run verification commands from the ticket.\n")
	builder.WriteString("- Do NOT commit or push - the runner will handle that.\n")

	return os.WriteFile(path, []byte(builder.String()), 0644)
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
	if filepath.IsAbs(repoPath) {
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
		Comments []struct {
			Body      string `json:"body"`
			CreatedAt string `json:"createdAt"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return ""
	}

	builder := strings.Builder{}
	for _, comment := range payload.Comments {
		if strings.Contains(comment.Body, "AWK Review") {
			builder.WriteString("---\n")
			builder.WriteString(comment.CreatedAt)
			builder.WriteString(":\n")
			builder.WriteString(comment.Body)
			builder.WriteString("\n")
		}
	}

	result := builder.String()
	if len(result) > 4000 {
		result = result[len(result)-4000:]
	}
	return result
}

func loadHistoricalFeedback(stateRoot string, maxEntries int) string {
	if maxEntries <= 0 {
		maxEntries = 10
	}
	entries, err := reviewer.LoadRecentFeedback(stateRoot, maxEntries)
	if err != nil || len(entries) == 0 {
		return ""
	}
	return reviewer.FormatFeedbackForPrompt(entries, 2000)
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
		_ = runGit(ctx, wtDir, timeout, "reset", "-q", ".ai", ".worktrees")
	}
	return nil
}

func createOrFindPR(ctx context.Context, branch, base, title string, issueID int, timeout time.Duration, retryCfg ghutil.RetryConfig) (string, error) {
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

	body := fmt.Sprintf("Closes #%d\n\n%s", issueID, title)
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

	_, _ = ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "issue", "comment", fmt.Sprintf("%d", issueID), "--body", body.String())
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
		SpecName:          info.SpecName,
		TaskLine:          info.TaskLine,
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

func resolveSpecName(meta *TicketMetadata) string {
	specName := strings.TrimSpace(os.Getenv("AI_SPEC_NAME"))
	if specName != "" {
		return specName
	}
	return meta.SpecName
}

func resolveTaskLine(meta *TicketMetadata) string {
	taskLine := strings.TrimSpace(os.Getenv("AI_TASK_LINE"))
	if taskLine != "" {
		return taskLine
	}
	if meta.TaskLine > 0 {
		return strconv.Itoa(meta.TaskLine)
	}
	return ""
}

func generateSessionID(role string) string {
	now := time.Now().UTC().Format("20060102-150405")
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		random = []byte{0, 0, 0, 0}
	}
	return fmt.Sprintf("%s-%s-%x", role, now, random)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
