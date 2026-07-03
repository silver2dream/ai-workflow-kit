package worker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// stubGH puts a no-op gh executable on PATH so recovery handlers can "call"
// GitHub without touching the network.
func stubGH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(dir, "gh.bat"), []byte("@echo off\r\nexit /b 0\r\n"), 0755); err != nil {
			t.Fatalf("write gh stub: %v", err)
		}
	} else {
		if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write gh stub: %v", err)
		}
	}
	t.Setenv("PATH", dir)
}

// writeTrace writes an execution trace file for the given issue.
func writeTrace(t *testing.T, stateRoot string, issue int, trace ExecutionTrace) {
	t.Helper()
	dir := filepath.Join(stateRoot, ".ai", "state", "traces")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir traces: %v", err)
	}
	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "issue-7.json"), data, 0644); err != nil {
		t.Fatalf("write trace: %v", err)
	}
}

// reapedPID starts and reaps a real short-lived process, returning a PID that
// is guaranteed dead (or, if reused, has a start time that cannot match the
// bogus WorkerStart the tests pass).
func reapedPID(t *testing.T) int {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit", "0")
	} else {
		cmd = exec.Command("sh", "-c", "exit 0")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start throwaway process: %v", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

func TestCheckResult_CrashedWorkerWritesRecoverableResult(t *testing.T) {
	deadPID := reapedPID(t) // before stubGH, which empties PATH
	stubGH(t)
	stateRoot := t.TempDir()

	writeTrace(t, stateRoot, 7, ExecutionTrace{
		IssueID:     "7",
		Status:      "running",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		WorkerPID:   deadPID,
		WorkerStart: 12345, // deliberately wrong so PID reuse cannot match
	})

	out, err := CheckResult(context.Background(), CheckResultOptions{
		IssueNumber:  7,
		StateRoot:    stateRoot,
		WaitDuration: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CheckResult error: %v", err)
	}
	if out.Status != "crashed" {
		t.Fatalf("Status = %q, want crashed", out.Status)
	}

	// The crash must be persisted so the issue is not retried infinitely.
	result, lerr := LoadResult(stateRoot, 7)
	if lerr != nil {
		t.Fatalf("expected crash result file to be written: %v", lerr)
	}
	if result.Status != "crashed" || !result.Recoverable {
		t.Errorf("persisted result = status %q recoverable %v, want crashed/true", result.Status, result.Recoverable)
	}
}

func TestCheckResult_TimeoutWorkerWritesRecoverableResult(t *testing.T) {
	stubGH(t)
	stateRoot := t.TempDir()

	writeTrace(t, stateRoot, 7, ExecutionTrace{
		IssueID:   "7",
		Status:    "running",
		StartedAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		// WorkerPID 0: no liveness info, so the elapsed-time check decides.
	})

	out, err := CheckResult(context.Background(), CheckResultOptions{
		IssueNumber:   7,
		StateRoot:     stateRoot,
		WorkerTimeout: time.Hour,
		WaitDuration:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CheckResult error: %v", err)
	}
	if out.Status != "timeout" {
		t.Fatalf("Status = %q, want timeout", out.Status)
	}
	if !strings.Contains(out.Error, "timeout") {
		t.Errorf("Error = %q, want timeout mention", out.Error)
	}

	result, lerr := LoadResult(stateRoot, 7)
	if lerr != nil {
		t.Fatalf("expected timeout result file to be written: %v", lerr)
	}
	if result.Status != "timeout" || !result.Recoverable {
		t.Errorf("persisted result = status %q recoverable %v, want timeout/true", result.Status, result.Recoverable)
	}
	if result.FailureStage != "execution" {
		t.Errorf("FailureStage = %q, want execution", result.FailureStage)
	}
}

func TestCheckResult_RunningWorkerWithinBudgetWaits(t *testing.T) {
	stubGH(t)
	stateRoot := t.TempDir()

	writeTrace(t, stateRoot, 7, ExecutionTrace{
		IssueID:   "7",
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})

	out, err := CheckResult(context.Background(), CheckResultOptions{
		IssueNumber:   7,
		StateRoot:     stateRoot,
		WorkerTimeout: time.Hour,
		WaitDuration:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CheckResult error: %v", err)
	}
	if out.Status != "not_found" {
		t.Fatalf("Status = %q, want not_found (worker still within budget)", out.Status)
	}

	// No result file must be fabricated for a healthy in-flight worker.
	if _, lerr := LoadResult(stateRoot, 7); !os.IsNotExist(lerr) {
		t.Errorf("expected no result file for running worker, got err=%v", lerr)
	}
}

func TestCheckResult_NoTraceReturnsNotFound(t *testing.T) {
	stubGH(t)
	stateRoot := t.TempDir()

	out, err := CheckResult(context.Background(), CheckResultOptions{
		IssueNumber:  7,
		StateRoot:    stateRoot,
		WaitDuration: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CheckResult error: %v", err)
	}
	if out.Status != "not_found" {
		t.Fatalf("Status = %q, want not_found (no trace yet)", out.Status)
	}
}
