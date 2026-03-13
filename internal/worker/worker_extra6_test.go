package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// loadAttemptInfo additional paths (runner.go)
// ---------------------------------------------------------------------------

func TestLoadAttemptInfo_WithPreviousSessionID(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, ".ai", "results")
	os.MkdirAll(resultsDir, 0755)

	result := &IssueResult{
		IssueID: "10",
		Session: SessionInfo{
			WorkerSessionID: "worker-session-abc",
		},
	}
	data, _ := json.Marshal(result)
	os.WriteFile(filepath.Join(resultsDir, "issue-10.json"), data, 0644)

	info := loadAttemptInfo(dir, 10)
	if info.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", info.AttemptNumber)
	}
	if len(info.PreviousSessionIDs) == 0 {
		t.Error("PreviousSessionIDs should contain the previous worker session ID")
	}
}

func TestLoadAttemptInfo_MaxPreviousSessionIDs(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, ".ai", "results")
	os.MkdirAll(resultsDir, 0755)

	// Create a result with more than maxPreviousSessionIDs session IDs
	prevIDs := make([]string, maxPreviousSessionIDs+2)
	for i := range prevIDs {
		prevIDs[i] = "session-" + strings.Repeat("a", i+1)
	}
	result := &IssueResult{
		IssueID: "20",
		Session: SessionInfo{
			WorkerSessionID:    "current-session",
			PreviousSessionIDs: prevIDs,
		},
	}
	data, _ := json.Marshal(result)
	os.WriteFile(filepath.Join(resultsDir, "issue-20.json"), data, 0644)

	info := loadAttemptInfo(dir, 20)
	if len(info.PreviousSessionIDs) > maxPreviousSessionIDs {
		t.Errorf("PreviousSessionIDs count = %d, should be capped at %d",
			len(info.PreviousSessionIDs), maxPreviousSessionIDs)
	}
}

// ---------------------------------------------------------------------------
// TraceRecorder.writeLocked (trace.go)
// ---------------------------------------------------------------------------

func TestTraceRecorder_WriteLocked_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	rec, err := NewTraceRecorder(dir, 100, "owner/repo", "feat/test", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	// writeLocked is called by Finalize — test via Finalize
	rec.StepStart("setup")
	rec.StepEnd("success", "Done", nil)
	if err := rec.Finalize(nil); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Verify the written file is valid JSON
	data, err := os.ReadFile(rec.path)
	if err != nil {
		t.Fatalf("ReadFile trace: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("trace file is not valid JSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// cleanupIndexLocks (runner.go) — takes wtDir and logf
// ---------------------------------------------------------------------------

func TestCleanupIndexLocks_NoGitDir(t *testing.T) {
	dir := t.TempDir()
	// No .git directory — should not panic
	cleanupIndexLocks(dir, nil)
}

func TestCleanupIndexLocks_WithLockFile(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0755)

	lockFile := filepath.Join(gitDir, "index.lock")
	os.WriteFile(lockFile, []byte("lock content"), 0644)

	var logged []string
	logf := func(format string, args ...interface{}) {
		logged = append(logged, format)
	}

	cleanupIndexLocks(dir, logf)

	// After cleanup, lock file should be removed
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("index.lock should be removed by cleanupIndexLocks")
	}
	if len(logged) == 0 {
		t.Error("logf should have been called")
	}
}

func TestCleanupIndexLocks_NilLogf(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "index.lock"), []byte("lock"), 0644)

	// nil logf should not panic
	cleanupIndexLocks(dir, nil)
}

// ---------------------------------------------------------------------------
// resetFailCount (runner.go) — takes runDir and logf
// ---------------------------------------------------------------------------

func TestResetFailCount_FileExists(t *testing.T) {
	dir := t.TempDir()
	// resetFailCount takes the run directory directly (not stateRoot+issueID)
	os.WriteFile(filepath.Join(dir, "fail_count.txt"), []byte("3"), 0644)

	var logged []string
	logf := func(format string, args ...interface{}) {
		logged = append(logged, format)
	}
	resetFailCount(dir, logf)

	// After reset, fail_count.txt should be removed
	if _, err := os.Stat(filepath.Join(dir, "fail_count.txt")); !os.IsNotExist(err) {
		t.Error("fail_count.txt should be removed by resetFailCount")
	}
	if len(logged) == 0 {
		t.Error("logf should have been called")
	}
}

func TestResetFailCount_NoFile(t *testing.T) {
	dir := t.TempDir()
	// No fail count file — should not panic
	resetFailCount(dir, nil)
}
