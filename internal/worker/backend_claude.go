package worker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// ClaudeCodeBackend implements WorkerBackend for Claude Code CLI.
type ClaudeCodeBackend struct {
	Model string
	// Effort is how hard the Worker thinks; empty for the CLI's own default.
	Effort                     string
	MaxTurns                   int
	DangerouslySkipPermissions bool
}

// NewClaudeCodeBackend creates a new ClaudeCodeBackend.
func NewClaudeCodeBackend(model string, maxTurns int, skipPerms bool) *ClaudeCodeBackend {
	if model == "" {
		model = "sonnet"
	}
	if maxTurns <= 0 {
		maxTurns = 50
	}
	return &ClaudeCodeBackend{
		Model:                      model,
		MaxTurns:                   maxTurns,
		DangerouslySkipPermissions: skipPerms,
	}
}

// effortArgs makes the flag of an effort, as package claudecli does. A
// variable, so tests can see what the Worker is given without a claude CLI.
var effortArgs = claudecli.EffortArgs

// args are the arguments the Worker's claude is run with.
func (b *ClaudeCodeBackend) args() []string {
	args := []string{
		"--print",
		"--model", b.Model,
		"--max-turns", strconv.Itoa(b.MaxTurns),
	}
	args = append(args, effortArgs(b.Effort)...)
	if b.DangerouslySkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}

// Name returns the backend identifier.
func (b *ClaudeCodeBackend) Name() string {
	return "claude-code"
}

// Available checks if claude CLI is in PATH.
func (b *ClaudeCodeBackend) Available() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found in PATH")
	}
	return nil
}

// Execute runs claude code CLI with retries.
func (b *ClaudeCodeBackend) Execute(ctx context.Context, opts BackendOptions) BackendResult {
	start := time.Now()
	result := BackendResult{}

	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 1
	}

	// Check availability
	if err := b.Available(); err != nil {
		result.ExitCode = 127
		result.Attempts = 1
		result.RetryCount = 0
		result.Duration = time.Since(start)
		result.FailureStage = "claude_exec"
		result.FailureReason = err.Error()
		return result
	}

	args := b.args()
	if opts.Guard != nil {
		held, err := b.guardArgs(opts.Guard)
		if err != nil {
			// The Worker is not run unguarded.
			result.ExitCode = 127
			result.Attempts = 1
			result.Duration = time.Since(start)
			result.FailureStage = "claude_guard"
			result.FailureReason = err.Error()
			return result
		}
		args = append(args, held...)
	}

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		logFile := fmt.Sprintf("%s.attempt-%d.log", opts.LogBase, attempt)
		result.LastLogFile = logFile

		if opts.Trace != nil {
			opts.Trace.StepStart(fmt.Sprintf("claude_exec_attempt_%d", attempt))
		}

		exitCode := b.runAttempt(ctx, opts, logFile, args)
		result.ExitCode = exitCode
		result.Attempts = attempt

		if opts.Trace != nil {
			status := "success"
			errorMessage := ""
			traceCtx := map[string]interface{}{"attempt": attempt, "model": b.Model, "effort": b.Effort}
			if exitCode != 0 {
				status = "failed"
				errorMessage = fmt.Sprintf("claude rc=%d", exitCode)
				if exitCode == 124 {
					errorMessage = fmt.Sprintf("claude timeout after %s", opts.Timeout)
					traceCtx["timeout_seconds"] = int(opts.Timeout.Seconds())
				}
			}
			_ = opts.Trace.StepEnd(status, errorMessage, traceCtx)
		}

		if exitCode == 0 {
			break
		}

		if exitCode == 127 {
			break
		}

		if attempt < opts.MaxAttempts && opts.RetryDelay > 0 {
			time.Sleep(opts.RetryDelay)
		}
	}

	result.RetryCount = result.Attempts - 1
	result.Duration = time.Since(start)

	if result.ExitCode != 0 {
		if result.ExitCode == 124 {
			result.FailureStage = "claude_timeout"
			result.FailureReason = fmt.Sprintf("claude timeout after %s", opts.Timeout)
		} else {
			result.FailureStage = "claude_exec"
			result.FailureReason = readFailureReason(result.LastLogFile, result.ExitCode)
		}
	}

	return result
}

func (b *ClaudeCodeBackend) runAttempt(ctx context.Context, opts BackendOptions, logFile string, args []string) int {
	_ = os.MkdirAll(filepath.Dir(logFile), 0755)

	prompt, err := os.Open(opts.PromptFile)
	if err != nil {
		writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to open prompt file: %v\n", err))
		return 127
	}
	defer func() {
		if err := prompt.Close(); err != nil {
			writeSummary(opts.SummaryFile, fmt.Sprintf("WARNING: failed to close prompt file: %v\n", err))
		}
	}()

	logHandle, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to open claude log: %v\n", err))
		return 127
	}
	defer func() {
		if err := logHandle.Close(); err != nil {
			writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to close claude log: %v\n", err))
		}
	}()

	summaryHandle, err := os.OpenFile(opts.SummaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to open summary: %v\n", err))
		return 127
	}
	defer func() {
		if err := summaryHandle.Close(); err != nil {
			writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to close summary: %v\n", err))
		}
	}()

	multiWriter := io.MultiWriter(summaryHandle, logHandle)

	execCtx, cancel := withOptionalTimeout(ctx, opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = guard.AgentEnv(os.Environ(), guard.ClaudeCredentialEnv...)
	cmd.Stdin = prompt
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter

	if err := cmd.Run(); err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return 124
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	return 0
}
