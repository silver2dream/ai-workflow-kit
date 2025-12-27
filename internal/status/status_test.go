package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

func TestCollect_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	report, err := Collect(tmpDir, Options{})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if report.Run.State != "no_lock" {
		t.Fatalf("Run.State = %q, want %q", report.Run.State, "no_lock")
	}
	if report.Control.StopPresent {
		t.Fatal("StopPresent should be false when STOP file is missing")
	}
	if report.Target.Source != "none" {
		t.Fatalf("Target.Source = %q, want %q", report.Target.Source, "none")
	}
}

func TestCollect_LockRunning(t *testing.T) {
	tmpDir := t.TempDir()

	lockPath := filepath.Join(tmpDir, ".ai", "state", "kickoff.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	info := kickoff.LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now(),
		Hostname:  "test-host",
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	report, err := Collect(tmpDir, Options{})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if report.Run.State != "running" {
		t.Fatalf("Run.State = %q, want %q", report.Run.State, "running")
	}
	if report.Run.ProcessAlive == nil || !*report.Run.ProcessAlive {
		t.Fatalf("Run.ProcessAlive = %v, want true", report.Run.ProcessAlive)
	}
}

func TestCollect_TargetFromTrace(t *testing.T) {
	tmpDir := t.TempDir()

	tracePath := filepath.Join(tmpDir, ".ai", "state", "traces", "issue-42.json")
	if err := os.MkdirAll(filepath.Dir(tracePath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	traceJSON := `{
  "trace_id": "issue-42-test",
  "issue_id": "42",
  "repo": "root",
  "branch": "feat/ai-issue-42",
  "base_branch": "feat/example",
  "status": "running",
  "started_at": "2025-12-27T00:00:00Z",
  "ended_at": "",
  "duration_seconds": 0,
  "error": "",
  "steps": []
}`
	if err := os.WriteFile(tracePath, []byte(traceJSON), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	report, err := Collect(tmpDir, Options{})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if report.Target.IssueID != 42 || report.Target.Source != "trace" {
		t.Fatalf("Target = (%d,%q), want (42,%q)", report.Target.IssueID, report.Target.Source, "trace")
	}
	if report.Artifacts.Trace == nil || !report.Artifacts.Trace.Exists {
		t.Fatalf("Artifacts.Trace.Exists = %v, want true", report.Artifacts.Trace)
	}
}

func TestCollect_TargetFromResult(t *testing.T) {
	tmpDir := t.TempDir()

	resultsDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	issue4 := filepath.Join(resultsDir, "issue-4.json")
	issue5 := filepath.Join(resultsDir, "issue-5.json")

	result4 := `{
  "issue_id": "4",
  "status": "failed",
  "repo": "root",
  "branch": "feat/ai-issue-4",
  "base_branch": "feat/example",
  "head_sha": "deadbeef",
  "timestamp_utc": "2025-12-26T00:00:00Z",
  "pr_url": "",
  "summary_file": ".ai/runs/issue-4/summary.txt",
  "metrics": {"duration_seconds": 1, "retry_count": 0}
}`
	result5 := `{
  "issue_id": "5",
  "status": "success",
  "repo": "root",
  "branch": "feat/ai-issue-5",
  "base_branch": "feat/example",
  "head_sha": "cafebabe",
  "timestamp_utc": "2025-12-27T00:00:00Z",
  "pr_url": "https://example.com/pr/5",
  "summary_file": ".ai/runs/issue-5/summary.txt",
  "metrics": {"duration_seconds": 2, "retry_count": 1}
}`

	if err := os.WriteFile(issue4, []byte(result4), 0644); err != nil {
		t.Fatalf("WriteFile issue4 failed: %v", err)
	}
	if err := os.WriteFile(issue5, []byte(result5), 0644); err != nil {
		t.Fatalf("WriteFile issue5 failed: %v", err)
	}

	exeLogsDir := filepath.Join(tmpDir, ".ai", "exe-logs")
	if err := os.MkdirAll(exeLogsDir, 0755); err != nil {
		t.Fatalf("MkdirAll exe-logs failed: %v", err)
	}
	// Codex logs for issue 5
	if err := os.WriteFile(filepath.Join(exeLogsDir, "issue-5.root.codex.attempt-1.log"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile codex log failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(exeLogsDir, "issue-5.root.codex.early-failure.log"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile early-failure log failed: %v", err)
	}

	report, err := Collect(tmpDir, Options{})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if report.Target.IssueID != 5 || report.Target.Source != "result" {
		t.Fatalf("Target = (%d,%q), want (5,%q)", report.Target.IssueID, report.Target.Source, "result")
	}
	if report.Artifacts.Result == nil || report.Artifacts.Result.Status != "success" {
		t.Fatalf("Artifacts.Result.Status = %v, want %q", report.Artifacts.Result, "success")
	}
	if len(report.Artifacts.Logs.Codex) != 2 {
		t.Fatalf("Artifacts.Logs.Codex len = %d, want 2", len(report.Artifacts.Logs.Codex))
	}
}
