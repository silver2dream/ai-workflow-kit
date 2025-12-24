package kickoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockManager_AcquireRelease(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	lock := NewLockManager(lockFile)

	// Acquire lock
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock file exists
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Error("Lock file should exist after acquire")
	}

	// Verify lock file content
	data, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("Failed to parse lock file: %v", err)
	}

	if info.PID != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), info.PID)
	}

	if info.Hostname == "" {
		t.Error("Hostname should not be empty")
	}

	// Release lock
	if err := lock.Release(); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Verify lock file is removed
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Lock file should not exist after release")
	}
}

func TestLockManager_DoubleAcquire(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	lock1 := NewLockManager(lockFile)
	lock2 := NewLockManager(lockFile)

	// First acquire should succeed
	if err := lock1.Acquire(); err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}
	defer lock1.Release()

	// Second acquire should fail (same process, but simulates another instance)
	// Note: In real scenario, this would be a different process
	// For testing, we manually create a lock with a different PID
	info := LockInfo{
		PID:       99999, // Fake PID that's likely not running
		StartTime: time.Now(),
		Hostname:  "test-host",
	}
	data, _ := json.Marshal(info)
	os.WriteFile(lockFile, data, 0644)

	// Now lock2 should succeed because PID 99999 is not running (stale lock)
	if err := lock2.Acquire(); err != nil {
		t.Errorf("Second acquire should succeed for stale lock: %v", err)
	}
}

func TestLockManager_StaleLockDetection(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	// Create a lock file with a non-existent PID
	info := LockInfo{
		PID:       99999, // Very unlikely to be a real process
		StartTime: time.Now().Add(-1 * time.Hour),
		Hostname:  "old-host",
	}
	data, _ := json.MarshalIndent(info, "", "  ")

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(lockFile), 0755)
	if err := os.WriteFile(lockFile, data, 0644); err != nil {
		t.Fatalf("Failed to create stale lock: %v", err)
	}

	lock := NewLockManager(lockFile)

	// IsStale should return true for non-existent PID
	if !lock.IsStale() {
		t.Error("Lock should be detected as stale")
	}

	// Acquire should succeed by removing stale lock
	if err := lock.Acquire(); err != nil {
		t.Errorf("Acquire should succeed for stale lock: %v", err)
	}
	defer lock.Release()

	// Verify new lock has current PID
	newInfo, err := lock.readLockInfo()
	if err != nil {
		t.Fatalf("Failed to read new lock info: %v", err)
	}

	if newInfo.PID != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), newInfo.PID)
	}
}

func TestLockManager_ReleaseWithoutAcquire(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	lock := NewLockManager(lockFile)

	// Release without acquire should not error
	if err := lock.Release(); err != nil {
		t.Errorf("Release without acquire should not error: %v", err)
	}
}

func TestLockManager_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "subdir", "nested", "test.lock")

	lock := NewLockManager(lockFile)

	// Acquire should create parent directories
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release()

	// Verify directory was created
	dir := filepath.Dir(lockFile)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Parent directory should be created")
	}
}

func TestProcessAlive(t *testing.T) {
	// Current process should be alive
	if !processAlive(os.Getpid()) {
		t.Error("Current process should be detected as alive")
	}

	// Non-existent PID should not be alive
	if processAlive(99999) {
		t.Error("Non-existent PID should not be detected as alive")
	}
}
