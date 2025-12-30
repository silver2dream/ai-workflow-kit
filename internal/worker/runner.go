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
}

type workflowRepo struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
	Type string `yaml:"type"`
}

type workflowGit struct {
	IntegrationBranch string `yaml:"integration_branch"`
	ReleaseBranch     string `yaml:"release_branch"`
}

type workflowEscalation struct {
	RetryCount        int `yaml:"retry_count"`
	RetryDelaySeconds int `yaml:"retry_delay_seconds"`
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
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}
	if opts.GitTimeout == 0 {
		opts.GitTimeout = 120 * time.Second
	}
	if opts.CodexTimeout == 0 {
		opts.CodexTimeout = 30 * time.Minute
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
	defer logger.Close()

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
		logger.Log("reset branch to %s", baseRef)
		_ = runGit(ctx, wtDir, opts.GitTimeout, "fetch", "origin", "--prune")
		_ = runGit(ctx, wtDir, opts.GitTimeout, "reset", "--hard", baseRef)
	}

	titleLine := extractTitleLine(string(ticketBody))
	if titleLine == "" {
		titleLine = fmt.Sprintf("issue-%d", opts.IssueID)
	}
	commitMsg := BuildCommitMessage(titleLine)

	workDirInstruction := buildWorkDirInstruction(repoType, repoPath, workDir, repoName)
	promptFile := filepath.Join(runDir, "prompt.txt")
	if err := writePromptFile(promptFile, workDirInstruction, string(ticketBody), opts.IssueID, opts.GHTimeout); err != nil {
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

	codex := RunCodex(ctx, CodexOptions{
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
		runErr = fmt.Errorf(codex.FailureReason)
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
			runErr = fmt.Errorf("no changes staged")
			result.Error = runErr.Error()
			if trace != nil {
				_ = trace.StepEnd("failed", "no changes staged", nil)
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
				ErrorMessage:          runErr.Error(),
				FailureStage:          "git_commit",
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
			return result, runErr
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
	prURL, err := createOrFindPR(ctx, branch, prBase, commitMsg, opts.IssueID, opts.GHTimeout)
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
	postIssueComment(ctx, opts.IssueID, workerSessionID, "worker_complete", "",
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

func writePromptFile(path, workDirInstruction, ticket string, issueID int, ghTimeout time.Duration) error {
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

	builder.WriteString("\nAfter making changes:\n")
	builder.WriteString("- Print: git status --porcelain\n")
	builder.WriteString("- Print: git diff\n")
	builder.WriteString("- Run verification commands from the ticket.\n")
	builder.WriteString("- Do NOT commit or push - the runner will handle that.\n")

	return os.WriteFile(path, []byte(builder.String()), 0644)
}

func buildWorkDirInstruction(repoType, repoPath, workDir, repoName string) string {
	repoPath = strings.TrimSuffix(repoPath, "/")
	repoPath = strings.TrimSuffix(repoPath, "\\")
	if repoPath == "" || repoPath == "." {
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

func fetchReviewComments(issueID int, ghTimeout time.Duration) string {
	if _, err := exec.LookPath("gh"); err != nil {
		return ""
	}

	ctx, cancel := withOptionalTimeout(context.Background(), ghTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "view", fmt.Sprintf("%d", issueID), "--json", "comments")
	output, err := cmd.Output()
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

func runAttemptGuard(ctx context.Context, stateRoot string, issueID int, logFile string, opts RunIssueOptions) error {
	// Try Go implementation first unless AWKIT_USE_SCRIPT=1
	if os.Getenv("AWKIT_USE_SCRIPT") != "1" {
		guard := NewAttemptGuard(stateRoot, issueID)
		result, err := guard.Check()
		if err != nil {
			// Fall through to bash script
		} else {
			if !result.CanProceed {
				return fmt.Errorf("attempt_guard: %s (attempt %d)", result.Reason, result.AttemptNumber)
			}
			return nil
		}
	}

	// Fallback to bash script
	scriptPath := filepath.Join(stateRoot, ".ai", "scripts", "attempt_guard.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Script doesn't exist, return error from Go check if we had one
		return nil
	}
	cmd := exec.CommandContext(ctx, "bash", scriptPath, fmt.Sprintf("%d", issueID), "codex-auto")
	cmd.Dir = stateRoot
	cmd.Env = append(os.Environ(),
		"AI_STATE_ROOT="+stateRoot,
		"WORKER_LOG_FILE="+logFile,
		fmt.Sprintf("AI_GIT_TIMEOUT=%d", int(opts.GitTimeout.Seconds())),
		fmt.Sprintf("AI_GH_TIMEOUT=%d", int(opts.GHTimeout.Seconds())),
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attempt_guard failed: %w", err)
	}
	return nil
}

func runPreflight(ctx context.Context, stateRoot, repoType, repoPath, logFile string, opts RunIssueOptions) error {
	// Try Go implementation first unless AWKIT_USE_SCRIPT=1
	if os.Getenv("AWKIT_USE_SCRIPT") != "1" {
		preflight := NewWorkerPreflight(stateRoot, repoType, repoPath)
		preflight.Timeout = opts.GitTimeout
		result, err := preflight.Check(ctx)
		if err != nil {
			// Fall through to bash script
		} else {
			if !result.Passed {
				return fmt.Errorf("preflight: %s", result.Message)
			}
			return nil
		}
	}

	// Fallback to bash script
	scriptPath := filepath.Join(stateRoot, ".ai", "scripts", "preflight.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Script doesn't exist, return nil (no preflight check)
		return nil
	}
	cmd := exec.CommandContext(ctx, "bash", scriptPath, repoType, repoPath)
	cmd.Dir = stateRoot
	cmd.Env = append(os.Environ(),
		"AI_STATE_ROOT="+stateRoot,
		"WORKER_LOG_FILE="+logFile,
		fmt.Sprintf("AI_GIT_TIMEOUT=%d", int(opts.GitTimeout.Seconds())),
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("preflight failed: %w", err)
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

func createOrFindPR(ctx context.Context, branch, base, title string, issueID int, timeout time.Duration) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found in PATH")
	}

	client := NewGitHubClient(timeout)
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

	cmd := exec.CommandContext(prCtx, "gh", "pr", "create",
		"--base", base,
		"--head", branch,
		"--title", title,
		"--body", body,
	)
	output, err := cmd.CombinedOutput()
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

func postIssueComment(ctx context.Context, issueID int, sessionID, commentType, source, extraData string, timeout time.Duration) {
	if _, err := exec.LookPath("gh"); err != nil {
		return
	}
	ctx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	body := strings.Builder{}
	body.WriteString("<!-- AWK:session:")
	body.WriteString(sessionID)
	body.WriteString(" -->\n")
	body.WriteString("**AWK Tracking**\n\n")
	body.WriteString("| Field | Value |\n")
	body.WriteString("|-------|-------|\n")
	body.WriteString(fmt.Sprintf("| Session | `%s` |\n", sessionID))
	body.WriteString(fmt.Sprintf("| Timestamp | %s |\n", timestamp))
	body.WriteString(fmt.Sprintf("| Type | %s |\n", commentType))
	if source != "" {
		body.WriteString(fmt.Sprintf("| Source | %s |\n", source))
	}
	if extraData != "" {
		body.WriteString(extraData)
		if !strings.HasSuffix(extraData, "\n") {
			body.WriteString("\n")
		}
	}

	cmd := exec.CommandContext(ctx, "gh", "issue", "comment", fmt.Sprintf("%d", issueID), "--body", body.String())
	_ = cmd.Run()
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

func loadAttemptInfo(stateRoot string, issueID int) attemptInfo {
	result, err := LoadResult(stateRoot, issueID)
	if err != nil || result == nil {
		return attemptInfo{AttemptNumber: 1}
	}

	if result.Session.WorkerSessionID == "" {
		return attemptInfo{AttemptNumber: 1}
	}

	attempt := result.Session.AttemptNumber
	if attempt < 0 {
		attempt = 0
	}

	previous := append([]string{}, result.Session.PreviousSessionIDs...)
	previous = append(previous, result.Session.WorkerSessionID)

	return attemptInfo{
		AttemptNumber:         attempt + 1,
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
	random := make([]byte, 2)
	if _, err := rand.Read(random); err != nil {
		random = []byte{0, 0}
	}
	return fmt.Sprintf("%s-%s-%02x%02x", role, now, random[0], random[1])
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

func (l *workerLogger) Close() {
	if l.file != nil {
		_ = l.file.Close()
	}
	if l.summary != nil {
		_ = l.summary.Close()
	}
}
