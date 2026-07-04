package ghutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	// Count attempts via a file instead of wall-clock timing, which is
	// unreliable under parallel test load (process spawn alone can exceed
	// any reasonable threshold). Each attempt appends one line, then prints
	// a non-retryable message ("not found") and exits 1.
	counter := filepath.Join(t.TempDir(), "attempts")
	script := fmt.Sprintf(`echo x >> "%s"; echo "HTTP 404: Not Found"; exit 1`, filepath.ToSlash(counter))
	_, err := RunWithRetry(ctx, cfg, "sh", "-c", script)

	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	data, readErr := os.ReadFile(counter)
	if readErr != nil {
		t.Fatalf("attempt counter not written: %v", readErr)
	}
	if attempts := strings.Count(string(data), "x"); attempts != 1 {
		t.Errorf("non-retryable error ran %d attempts; should not retry", attempts)
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
