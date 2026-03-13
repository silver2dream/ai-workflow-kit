package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WritePIDFile / ReadPIDFile / CleanupPIDFile
// ---------------------------------------------------------------------------

func TestCov_PIDFileLifecycle(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	info := &PIDFile{
		PID:         12345,
		StartTime:   now.Unix(),
		IssueNumber: 42,
		SessionID:   "worker-test-session",
		StartedAt:   now,
	}

	// Write
	err := WritePIDFile(dir, 42, info)
	if err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}

	// Read
	loaded, err := ReadPIDFile(dir, 42)
	if err != nil {
		t.Fatalf("ReadPIDFile failed: %v", err)
	}
	if loaded.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", loaded.PID)
	}
	if loaded.SessionID != "worker-test-session" {
		t.Errorf("expected session ID 'worker-test-session', got %q", loaded.SessionID)
	}
	if loaded.IssueNumber != 42 {
		t.Errorf("expected issue 42, got %d", loaded.IssueNumber)
	}

	// Cleanup
	err = CleanupPIDFile(dir, 42)
	if err != nil {
		t.Fatalf("CleanupPIDFile failed: %v", err)
	}

	// Read should fail
	_, err = ReadPIDFile(dir, 42)
	if err == nil {
		t.Error("expected error after cleanup")
	}

	// Cleanup again should not error (already cleaned)
	err = CleanupPIDFile(dir, 42)
	if err != nil {
		t.Errorf("double cleanup should not error: %v", err)
	}
}

func TestCov_ReadPIDFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	pidDir := filepath.Join(dir, ".ai", "state", "pids")
	os.MkdirAll(pidDir, 0700)
	os.WriteFile(filepath.Join(pidDir, "issue-1.json"), []byte("not json"), 0600)

	_, err := ReadPIDFile(dir, 1)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCov_PIDFileJSON(t *testing.T) {
	now := time.Now()
	info := &PIDFile{
		PID:         9999,
		StartTime:   now.Unix(),
		IssueNumber: 10,
		SessionID:   "sess-123",
		StartedAt:   now,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded PIDFile
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if loaded.PID != 9999 {
		t.Errorf("PID mismatch: %d", loaded.PID)
	}
}

// ---------------------------------------------------------------------------
// IsProcessRunning / IsProcessRunningByPID
// ---------------------------------------------------------------------------

func TestCov_IsProcessRunning_InvalidPID(t *testing.T) {
	if IsProcessRunning(0, 0) {
		t.Error("expected false for PID 0")
	}
	if IsProcessRunning(-1, 0) {
		t.Error("expected false for negative PID")
	}
}

func TestCov_IsProcessRunningByPID_InvalidPID(t *testing.T) {
	if IsProcessRunningByPID(0) {
		t.Error("expected false for PID 0")
	}
	if IsProcessRunningByPID(-1) {
		t.Error("expected false for negative PID")
	}
}

func TestCov_IsProcessRunning_CurrentProcess(t *testing.T) {
	pid := os.Getpid()
	// Current process should be running (start time 0 means skip time check)
	if !IsProcessRunning(pid, 0) {
		t.Error("expected current process to be running")
	}
}

func TestCov_IsProcessRunningByPID_CurrentProcess(t *testing.T) {
	pid := os.Getpid()
	if !IsProcessRunningByPID(pid) {
		t.Error("expected current process to be running")
	}
}
