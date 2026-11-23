package ghutil

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// A gh call that would write a credential to GitHub is not made: the guard
// stops it before gh runs, and it is not retried.
func TestRunWithRetry_StopsACredentialBeforeGHRuns(t *testing.T) {
	secret := "gh" + "p_" + strings.Repeat("aB3dE5", 6)
	cfg := RetryConfig{MaxAttempts: 3}
	// A gh that does not exist: had the call been made, the error would say so.
	out, err := RunWithRetry(context.Background(), cfg, "./no-such-dir/gh", "pr", "comment", "4", "--body", "token "+secret)
	if err == nil || !strings.HasPrefix(err.Error(), "AWK guard:") {
		t.Fatalf("err = %v, want the guard's refusal", err)
	}
	if out != nil {
		t.Errorf("output %q from a call that was not made", out)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the refusal holds the credential: %v", err)
	}
}

func TestRunWithRetry_OnlyGHIsScreened(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	secret := "gh" + "p_" + strings.Repeat("aB3dE5", 6)
	// Another program's arguments are none of the guard's business here.
	_, err := RunWithRetry(context.Background(), RetryConfig{MaxAttempts: 1}, "git", "--version", "--body", secret)
	if err != nil && strings.HasPrefix(err.Error(), "AWK guard:") {
		t.Errorf("git was screened as gh: %v", err)
	}
}

func TestIsGH(t *testing.T) {
	for name, want := range map[string]bool{
		"gh": true, "gh.exe": true, "GH.EXE": true, `C:\tools\gh.exe`: true, "/usr/bin/gh": true,
		"git": false, "ghost": false, "": false,
	} {
		if got := isGH(name); got != want {
			t.Errorf("isGH(%q) = %v", name, got)
		}
	}
}
