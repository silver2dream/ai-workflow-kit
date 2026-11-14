package reviewer

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The review runs the tests again and reads what they printed, both streams
// together. A suite that is out of time is stopped with the processes it
// started: the test binaries outlive the shell, and the review waited for
// them however long they took.
func TestExecuteTests(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on the PATH")
	}
	ctx, dir := context.Background(), t.TempDir()

	out, err := ExecuteTests(ctx, dir, "echo '--- PASS: TestOne'; echo 'a warning' >&2", time.Minute)
	if err != nil || !strings.Contains(out, "--- PASS: TestOne") || !strings.Contains(out, "a warning") {
		t.Errorf("a passing suite: %q, %v; want both streams and no error", out, err)
	}

	out, err = ExecuteTests(ctx, dir, "echo '--- FAIL: TestTwo'; exit 1", time.Minute)
	if err == nil || !strings.Contains(out, "--- FAIL: TestTwo") {
		t.Errorf("a failing suite: %q, %v; want its output and an error", out, err)
	}

	started := time.Now()
	out, err = ExecuteTests(ctx, dir, "echo '--- PASS: TestEarly'; sleep 5; echo '--- PASS: TestLate'", 300*time.Millisecond)
	if err == nil || !strings.Contains(out, "TestEarly") || strings.Contains(out, "TestLate") {
		t.Errorf("a suite out of time: %q, %v; want an error and what was printed before the limit", out, err)
	}
	if took := time.Since(started); took > 3*time.Second {
		t.Errorf("the suite was waited for %v after a limit of 300ms", took)
	}

	// The suite runs in the worktree it is given.
	out, err = ExecuteTests(ctx, dir, "pwd", time.Minute)
	if err != nil || strings.TrimSpace(out) == "" {
		t.Errorf("pwd in the worktree: %q, %v", out, err)
	}
}
