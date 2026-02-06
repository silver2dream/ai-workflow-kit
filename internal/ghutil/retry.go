package ghutil

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RetryConfig holds retry parameters for gh CLI calls.
type RetryConfig struct {
	MaxAttempts int           // default 3
	BaseDelay   time.Duration // default 2s
	MaxDelay    time.Duration // default 30s
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   2 * time.Second,
		MaxDelay:    30 * time.Second,
	}
}

// IsRetryable checks if a gh CLI error is worth retrying.
// It returns false for auth/validation errors and true for transient failures
// such as rate limits, network issues, and server errors.
func IsRetryable(output string, exitCode int) bool {
	// Never retry auth or validation errors
	nonRetryable := []string{
		"authentication", "auth", "login",
		"not found", "404",
		"422", "validation failed",
		"already exists",
	}
	lower := strings.ToLower(output)
	for _, s := range nonRetryable {
		if strings.Contains(lower, s) {
			return false
		}
	}

	// Retry on rate limit, network, server errors
	retryable := []string{
		"rate limit", "rate_limit", "403",
		"500", "502", "503", "504",
		"timeout", "timed out",
		"connection refused", "connection reset",
		"no such host", "network",
		"eagain", "temporary failure",
	}
	for _, s := range retryable {
		if strings.Contains(lower, s) {
			return true
		}
	}

	// Also retry generic non-zero exit (could be transient)
	return exitCode != 0
}

// RunWithRetry executes a command with exponential backoff retry.
// It captures combined stdout+stderr output from the command.
// Non-retryable errors are returned immediately without further attempts.
func RunWithRetry(ctx context.Context, cfg RetryConfig, name string, args ...string) ([]byte, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}

	var lastErr error
	var lastOutput []byte

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		cmd := exec.CommandContext(ctx, name, args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return output, nil
		}

		lastErr = err
		lastOutput = output

		// Check if retryable
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		if !IsRetryable(string(output), exitCode) {
			return output, err
		}

		if attempt < cfg.MaxAttempts {
			delay := time.Duration(float64(cfg.BaseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
			fmt.Fprintf(os.Stderr, "[ghutil] gh command failed (attempt %d/%d), retrying in %v: %s\n",
				attempt, cfg.MaxAttempts, delay, strings.TrimSpace(string(output)))

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return lastOutput, ctx.Err()
			}
		}
	}

	return lastOutput, fmt.Errorf("gh command failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}
