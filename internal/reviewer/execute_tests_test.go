package reviewer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	if err == nil || testsCutOff(err) || !strings.Contains(out, "--- FAIL: TestTwo") {
		t.Errorf("a failing suite: %q, %v; want its output and an error that is no time limit", out, err)
	}

	started := time.Now()
	out, err = ExecuteTests(ctx, dir, "echo '--- PASS: TestEarly'; sleep 5; echo '--- PASS: TestLate'", 300*time.Millisecond)
	if took := time.Since(started); took > 3*time.Second {
		t.Errorf("the suite was waited for %v after a limit of 300ms", took)
	}
	if err == nil || !strings.Contains(out, "TestEarly") || strings.Contains(out, "TestLate") {
		t.Errorf("a suite out of time: %q, %v; want an error and what was printed before the limit", out, err)
	}
	// The error says that it was the time limit, and which setting that is.
	if !testsCutOff(err) || !strings.Contains(err.Error(), "ran out of time after 300ms") || !strings.Contains(err.Error(), "timeouts.verify_minutes in .ai/config.yaml") {
		t.Errorf("a suite out of time: err = %v, want one that names the limit and its setting", err)
	}

	// The suite runs in the worktree it is given.
	out, err = ExecuteTests(ctx, dir, "pwd", time.Minute)
	if err != nil || strings.TrimSpace(out) == "" {
		t.Errorf("pwd in the worktree: %q, %v", out, err)
	}
}

// A suite that was cut off at its time limit is no evidence, also where the
// test a criterion is tied to had passed by then: what was to run after the
// cut is not known to pass. The review says that the limit was hit, and
// which setting it is. Given the time to finish, the same suite is evidence.
func TestVerifyTestEvidence_ASuiteCutOffIsNoEvidence(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not on the PATH")
	}
	dir := t.TempDir()
	testFile := "package parse\n\nfunc TestParseAcceptsInput(t *testing.T) { assert.NoError(t, err) }\n"
	if err := os.WriteFile(filepath.Join(dir, "parse_test.go"), []byte(testFile), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"echo '--- PASS: TestParseAcceptsInput (0.00s)'\n" +
		"sleep 1\n" +
		"echo '--- PASS: TestParseTheRest (0.00s)'\n"
	scriptPath := filepath.Join(dir, "run-test.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	verify := func(limit time.Duration) *EvidenceError {
		return VerifyTestEvidence(context.Background(), VerifyOptions{
			Ticket:       "## Acceptance Criteria\n- [ ] Input is accepted",
			WorktreePath: dir,
			TestCommand:  "sh " + filepath.ToSlash(scriptPath),
			Language:     "go",
			TestTimeout:  limit,
			Verifications: []CriteriaVerification{{
				Criteria:       "Input is accepted",
				Implementation: "Parse returns the value",
				TestName:       "TestParseAcceptsInput",
				Assertion:      "assert.NoError(t, err)",
			}},
		})
	}

	cut := verify(300 * time.Millisecond)
	if cut == nil {
		t.Fatal("a suite cut off at its time limit was accepted as evidence")
	}
	if cut.Code != 2 || !strings.Contains(cut.Message, "ran out of time after 300ms") || !strings.Contains(cut.Message, "timeouts.verify_minutes") {
		t.Errorf("the review of a suite cut off: code %d, %q; want a failed run that names the limit and its setting", cut.Code, cut.Message)
	}
	if details := strings.Join(cut.Details, "\n"); !strings.Contains(details, "1 test(s) had passed and 0 had failed when the run was stopped") {
		t.Errorf("the details do not say how far the run had come: %q", details)
	}

	if whole := verify(30 * time.Second); whole != nil {
		t.Errorf("the same suite with time to finish was refused: %s %v", whole.Message, whole.Details)
	}
}
