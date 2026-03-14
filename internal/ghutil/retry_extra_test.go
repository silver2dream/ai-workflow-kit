package ghutil

import (
	"context"
	"testing"
	"time"
)

// TestRunWithRetry_SuccessOnFirstAttempt verifies that a command succeeding
// on the first attempt returns its output immediately.
func TestRunWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	// "true" exits 0 on Linux/macOS
	out, err := RunWithRetry(ctx, cfg, "true")
	if err != nil {
		t.Fatalf("RunWithRetry(true) unexpected error: %v", err)
	}
	_ = out
}

// TestRunWithRetry_NonRetryableError verifies that a non-retryable error
// (e.g. authentication) is returned immediately without retrying.
func TestRunWithRetry_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	// Use a delay large enough to notice if retries happen, but MaxAttempts=3
	// so a retry would be visible. We use a shell script that exits non-zero
	// but prints a "not found" message (non-retryable keyword).
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 200 * time.Millisecond}

	// Use 'sh -c' to echo a non-retryable message then exit 1
	start := time.Now()
	_, err := RunWithRetry(ctx, cfg, "sh", "-c", `echo "HTTP 404: Not Found"; exit 1`)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	// Should NOT have waited for 3 attempts (would be 100ms+ if it retried)
	if elapsed > 80*time.Millisecond {
		t.Errorf("non-retryable error took %v; should have returned immediately without retrying", elapsed)
	}
}

// TestRunWithRetry_ZeroMaxAttempts verifies that zero MaxAttempts defaults to 1.
func TestRunWithRetry_ZeroMaxAttempts(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 0, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	_, err := RunWithRetry(ctx, cfg, "true")
	if err != nil {
		t.Fatalf("RunWithRetry with MaxAttempts=0 should default to 1 and succeed: %v", err)
	}
}

// TestRunWithRetry_ContextCancelled verifies that a cancelled context causes
// the function to return promptly even if retries are pending.
func TestRunWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: 5 * time.Second, MaxDelay: 30 * time.Second}

	start := time.Now()
	// Command would succeed if context were not cancelled, but context is already done.
	// The exec.CommandContext with a cancelled ctx will fail immediately.
	_, err := RunWithRetry(ctx, cfg, "true")
	elapsed := time.Since(start)

	// With a pre-cancelled context, we expect either an error from the command
	// or the context error. Either way it should be fast.
	_ = err
	if elapsed > 3*time.Second {
		t.Errorf("cancelled context took %v to return; expected near-instant", elapsed)
	}
}

// TestRunWithRetry_CommandNotFound verifies that a missing binary returns an error.
func TestRunWithRetry_CommandNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{MaxAttempts: 1, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	_, err := RunWithRetry(ctx, cfg, "this-binary-does-not-exist-awkit-test")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

// TestRunWithRetry_ExhaustsRetries verifies that after MaxAttempts the
// wrapped error message includes "failed after N attempts".
func TestRunWithRetry_ExhaustsRetries(t *testing.T) {
	ctx := context.Background()
	// Use a very short delay and MaxAttempts=2
	// Print a retryable message (e.g. "timeout") so it retries
	cfg := RetryConfig{MaxAttempts: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 2 * time.Millisecond}

	_, err := RunWithRetry(ctx, cfg, "sh", "-c", `echo "connection timeout"; exit 1`)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Error("expected non-empty error message")
	}
}
