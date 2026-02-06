package worker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	evtrace "github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// CheckResultOptions contains options for CheckResult
type CheckResultOptions struct {
	IssueNumber        int
	PrincipalSessionID string
	StateRoot          string        // defaults to git root
	MaxRetries         int           // defaults to 3
	GHTimeout          time.Duration // defaults to 30s
	WorkerTimeout      time.Duration // max worker runtime before timeout (defaults to 30m)
	WaitDuration       time.Duration // how long to wait when worker is running (defaults to 30s)
}

// CheckResultOutput is the output for bash eval compatibility
type CheckResultOutput struct {
	Status   string // not_found, success, failed, failed_will_retry, failed_max_retries, crashed, timeout
	PRNumber string
	Error    string
}

// CheckResult checks the worker execution result for an issue
// Returns structured output that can be converted to bash eval format
func CheckResult(ctx context.Context, opts CheckResultOptions) (output *CheckResultOutput, err error) {
	// Set defaults
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 30 * time.Second
	}
	if opts.WorkerTimeout == 0 {
		opts.WorkerTimeout = 30 * time.Minute
	}
	if opts.WaitDuration == 0 {
		opts.WaitDuration = 30 * time.Second
	}

	// Write check_result decision event on function return
	defer func() {
		if output == nil {
			return
		}
		// Determine decision result
		var decisionResult string
		switch output.Status {
		case "success":
			decisionResult = "SUCCESS"
		case "not_found":
			decisionResult = "WAIT"
		case "failed_will_retry":
			decisionResult = "RETRY"
		case "failed_max_retries":
			decisionResult = "FAIL_FINAL"
		case "crashed", "timeout":
			decisionResult = "FAIL_RECOVERABLE"
		default:
			decisionResult = "UNKNOWN"
		}

		evtrace.WriteDecisionEvent(evtrace.ComponentPrincipal, evtrace.TypeCheckResult, evtrace.Decision{
			Rule: "check worker result and determine next action",
			Conditions: map[string]any{
				"status":      output.Status,
				"pr_number":   output.PRNumber,
				"error":       output.Error,
				"max_retries": opts.MaxRetries,
			},
			Result: decisionResult,
		}, evtrace.WithIssue(opts.IssueNumber))
	}()

	// Try to load the result file
	result, err := LoadResult(opts.StateRoot, opts.IssueNumber)

	if os.IsNotExist(err) {
		// Result file doesn't exist - check worker status
		return handleMissingResult(ctx, opts)
	}

	if err != nil {
		return &CheckResultOutput{
			Status: "error",
			Error:  fmt.Sprintf("failed to load result: %v", err),
		}, nil
	}

	// Result exists - process it
	return processResult(ctx, opts, result)
}

// handleMissingResult handles the case when result file doesn't exist
func handleMissingResult(ctx context.Context, opts CheckResultOptions) (*CheckResultOutput, error) {
	// Load trace to check worker status
	trace, err := LoadTrace(opts.StateRoot, opts.IssueNumber)

	if err != nil {
		// No trace file either - worker hasn't started or very early failure
		return waitAndReturnNotFound(opts.WaitDuration)
	}

	// Check if worker is still running via PID
	if trace.WorkerPID > 0 {
		if !IsProcessRunning(trace.WorkerPID, trace.WorkerStart) {
			// Worker process is dead but no result - crashed
			return handleCrashedWorker(ctx, opts, trace)
		}
	}

	// Check for timeout based on trace start time
	startTime, err := trace.GetStartedAtTime()
	if err == nil {
		elapsed := time.Since(startTime)
		if elapsed > opts.WorkerTimeout {
			// Worker has been running too long - timeout
			return handleTimeoutWorker(ctx, opts, trace, elapsed)
		}
	}

	// Worker is still running - wait and return not_found
	return waitAndReturnNotFound(opts.WaitDuration)
}

// handleCrashedWorker handles a crashed worker (dead process, no result)
func handleCrashedWorker(ctx context.Context, opts CheckResultOptions, trace *ExecutionTrace) (*CheckResultOutput, error) {
	ghClient := NewGitHubClient(opts.GHTimeout)

	// Remove in-progress label
	_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "in-progress")

	// Write a crashed result file so it's not retried infinitely
	crashResult := &IssueResult{
		IssueID:      fmt.Sprintf("%d", opts.IssueNumber),
		Status:       "crashed",
		FailureStage: "unknown",
		ErrorMessage: "worker process terminated unexpectedly",
		Recoverable:  true,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
	_ = WriteResultAtomic(opts.StateRoot, opts.IssueNumber, crashResult)

	return &CheckResultOutput{
		Status: "crashed",
		Error:  "worker process terminated unexpectedly",
	}, nil
}

// handleTimeoutWorker handles a timed out worker
func handleTimeoutWorker(ctx context.Context, opts CheckResultOptions, trace *ExecutionTrace, elapsed time.Duration) (*CheckResultOutput, error) {
	ghClient := NewGitHubClient(opts.GHTimeout)

	// Remove in-progress label
	_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "in-progress")

	// Write a timeout result file
	timeoutResult := &IssueResult{
		IssueID:      fmt.Sprintf("%d", opts.IssueNumber),
		Status:       "timeout",
		FailureStage: "execution",
		ErrorMessage: fmt.Sprintf("worker exceeded timeout after %v", elapsed.Round(time.Minute)),
		Recoverable:  true,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
	_ = WriteResultAtomic(opts.StateRoot, opts.IssueNumber, timeoutResult)

	return &CheckResultOutput{
		Status: "timeout",
		Error:  fmt.Sprintf("worker timeout after %v", elapsed.Round(time.Minute)),
	}, nil
}

// waitAndReturnNotFound waits for the specified duration and returns not_found
func waitAndReturnNotFound(duration time.Duration) (*CheckResultOutput, error) {
	time.Sleep(duration)
	return &CheckResultOutput{
		Status: "not_found",
	}, nil
}

// processResult processes an existing result file
func processResult(ctx context.Context, opts CheckResultOptions, result *IssueResult) (*CheckResultOutput, error) {
	ghClient := NewGitHubClient(opts.GHTimeout)

	switch result.Status {
	case "success":
		// Validate PRURL before marking as pr-ready
		if result.PRURL == "" {
			// Success reported but no PR URL - this is an anomaly
			_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"worker-failed"}, []string{"in-progress"})
			_ = ghClient.CommentOnIssue(ctx, opts.IssueNumber,
				"Worker reported success but no PR was created. Manual intervention required.")
			return &CheckResultOutput{
				Status: "failed",
				Error:  "success reported but no PR URL found",
			}, nil
		}

		// Success with valid PR - reset consecutive failures, update labels
		_ = ResetConsecutiveFailures(opts.StateRoot)
		_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"pr-ready"}, []string{"in-progress"})

		prNum := ExtractPRNumber(result.PRURL)
		prNumStr := ""
		if prNum > 0 {
			prNumStr = strconv.Itoa(prNum)
		}
		return &CheckResultOutput{
			Status:   "success",
			PRNumber: prNumStr,
		}, nil

	case "success_no_changes":
		// Task completed successfully but no code changes were needed
		_ = ResetConsecutiveFailures(opts.StateRoot)
		_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"completed"}, []string{"in-progress", "ai-task"})
		_ = ghClient.CommentOnIssue(ctx, opts.IssueNumber,
			"Worker completed successfully. No code changes were required for this task.")
		_ = ghClient.CloseIssue(ctx, opts.IssueNumber)

		return &CheckResultOutput{
			Status: "success_no_changes",
		}, nil

	case "failed", "crashed", "timeout":
		// Check retry count (AttemptGuard already incremented before worker started)
		failCount := ReadFailCount(opts.StateRoot, opts.IssueNumber)

		if failCount >= opts.MaxRetries {
			// Max retries exceeded
			_ = ghClient.EditIssueLabels(ctx, opts.IssueNumber, []string{"worker-failed"}, []string{"in-progress"})
			_ = ghClient.CommentOnIssue(ctx, opts.IssueNumber, fmt.Sprintf(
				"Worker failed after %d attempts. Manual intervention required.\n\nLast error: %s",
				failCount, result.ErrorMessage,
			))

			return &CheckResultOutput{
				Status: "failed_max_retries",
				Error:  result.ErrorMessage,
			}, nil
		}

		// Will retry - remove in-progress for next attempt
		_ = ghClient.RemoveLabel(ctx, opts.IssueNumber, "in-progress")

		return &CheckResultOutput{
			Status: "failed_will_retry",
			Error:  result.ErrorMessage,
		}, nil

	default:
		// Unknown status
		return &CheckResultOutput{
			Status: result.Status,
			Error:  result.ErrorMessage,
		}, nil
	}
}

// FormatBashOutput formats the output for bash eval
func (o *CheckResultOutput) FormatBashOutput() string {
	lines := []string{
		fmt.Sprintf("CHECK_RESULT_STATUS=%s", o.Status),
		fmt.Sprintf("WORKER_STATUS=%s", o.Status),
	}

	if o.PRNumber != "" {
		lines = append(lines, fmt.Sprintf("PR_NUMBER=%s", o.PRNumber))
	} else {
		lines = append(lines, "PR_NUMBER=")
	}

	return fmt.Sprintf("%s\n", joinLines(lines))
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
