package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CodexOptions controls codex execution attempts.
type CodexOptions struct {
	WorkDir     string
	PromptFile  string
	SummaryFile string
	LogBase     string
	MaxAttempts int
	RetryDelay  time.Duration
	Timeout     time.Duration
	Trace       *TraceRecorder
}

// CodexResult contains codex execution details.
type CodexResult struct {
	ExitCode      int
	Attempts      int
	RetryCount    int
	Duration      time.Duration
	FailureStage  string
	FailureReason string
	LastLogFile   string
}

// RunCodex executes codex with retries and logs output to summary and per-attempt log.
func RunCodex(ctx context.Context, opts CodexOptions) CodexResult {
	start := time.Now()
	result := CodexResult{}

	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 1
	}

	cmdArgs, err := buildCodexCommand(ctx)
	if err != nil {
		result.ExitCode = 127
		result.Attempts = 1
		result.RetryCount = 0
		result.Duration = time.Since(start)
		result.FailureStage = "codex_exec"
		result.FailureReason = err.Error()
		return result
	}

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		logFile := fmt.Sprintf("%s.attempt-%d.log", opts.LogBase, attempt)
		result.LastLogFile = logFile

		if opts.Trace != nil {
			opts.Trace.StepStart(fmt.Sprintf("codex_exec_attempt_%d", attempt))
		}

		exitCode := runCodexAttempt(ctx, cmdArgs, opts, logFile)
		result.ExitCode = exitCode
		result.Attempts = attempt

		if opts.Trace != nil {
			status := "success"
			errorMessage := ""
			context := map[string]interface{}{"attempt": attempt}
			if exitCode != 0 {
				status = "failed"
				errorMessage = fmt.Sprintf("codex rc=%d", exitCode)
				if exitCode == 124 {
					errorMessage = fmt.Sprintf("codex timeout after %s", opts.Timeout)
					context["timeout_seconds"] = int(opts.Timeout.Seconds())
				}
			}
			_ = opts.Trace.StepEnd(status, errorMessage, context)
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
			result.FailureStage = "codex_timeout"
			result.FailureReason = fmt.Sprintf("codex timeout after %s", opts.Timeout)
		} else {
			result.FailureStage = "codex_exec"
			result.FailureReason = readFailureReason(result.LastLogFile, result.ExitCode)
		}
	}

	return result
}

func buildCodexCommand(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("codex"); err != nil {
		return nil, fmt.Errorf("codex CLI not found in PATH")
	}

	helpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(helpCtx, "codex", "exec", "--help")
	output, _ := cmd.CombinedOutput()
	helpText := string(output)

	args := []string{"exec"}
	if strings.Contains(helpText, "--full-auto") {
		args = append(args, "--full-auto")
	} else if strings.Contains(helpText, "--yolo") {
		args = append(args, "--yolo")
	}
	if strings.Contains(helpText, "--json") {
		args = append(args, "--json")
	}

	return args, nil
}

func runCodexAttempt(ctx context.Context, cmdArgs []string, opts CodexOptions, logFile string) int {
	_ = os.MkdirAll(filepath.Dir(logFile), 0755)

	prompt, err := os.Open(opts.PromptFile)
	if err != nil {
		writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to open prompt file: %v\n", err))
		return 127
	}
	defer prompt.Close()

	logHandle, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to open codex log: %v\n", err))
		return 127
	}
	defer func() {
		if err := logHandle.Close(); err != nil {
			writeSummary(opts.SummaryFile, fmt.Sprintf("ERROR: failed to close codex log: %v\n", err))
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

	cmd := exec.CommandContext(execCtx, "codex", cmdArgs...)
	cmd.Dir = opts.WorkDir
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

func readFailureReason(logFile string, exitCode int) string {
	lines := tailLines(logFile, 50)
	re := regexp.MustCompile(`(?i)(ERROR|FAILED|Exception|error:)`)
	for _, line := range lines {
		if re.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return fmt.Sprintf("codex exit code %d", exitCode)
}

func tailLines(path string, maxLines int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	return lines
}

func writeSummary(path, message string) {
	if path == "" {
		return
	}
	handle, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer handle.Close()
	_, _ = handle.WriteString(message)
}
