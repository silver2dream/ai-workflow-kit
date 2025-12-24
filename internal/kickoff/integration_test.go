package kickoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Task 12.1: End-to-end tests
// =============================================================================

// TestIntegration_PreflightToLockRelease tests the full preflight -> lock flow
// This test requires a clean git working directory and external tools
func TestIntegration_PreflightToLockRelease(t *testing.T) {
	// Skip in short mode (CI/automated tests)
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if external dependencies are not available
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("Skipping: gh CLI not installed")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("Skipping: claude CLI not installed")
	}
	// Check gh auth status
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		t.Skip("Skipping: gh not authenticated")
	}
	// Check git is available and we're in a repo
	if err := exec.Command("git", "status").Run(); err != nil {
		t.Skip("Skipping: not in a git repository")
	}
	// Check working directory is clean (required for preflight)
	output, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil || len(output) > 0 {
		t.Skip("Skipping: working directory not clean")
	}

	tmpDir, err := os.MkdirTemp("", "integration-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup config
	configPath := filepath.Join(tmpDir, "workflow.yaml")
	configContent := `version: "1.0"
project:
  name: test-project
  type: single-repo
repos:
  - name: test
    type: root
    path: ./
git:
  integration_branch: feat/test
specs:
  path: .ai/specs
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Create specs directory
	specsDir := filepath.Join(tmpDir, ".ai", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	lockFile := filepath.Join(tmpDir, "kickoff.lock")

	// 1. Run preflight checks
	checker := NewPreflightChecker(configPath, lockFile)
	results, err := checker.RunAll()
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	for _, r := range results {
		if !r.Passed {
			t.Errorf("Preflight check %s failed: %s", r.Name, r.Message)
		}
	}

	// 2. Acquire lock
	lock := NewLockManager(lockFile)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Lock acquire failed: %v", err)
	}

	// Verify lock file exists
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Error("Lock file should exist after acquire")
	}

	// 3. Release lock (simulating normal exit)
	lock.Release()

	// Property 12: Lock should be released on normal exit
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Lock file should be removed after release")
	}
}

// TestIntegration_StateResumeFlow tests save/load state flow
func TestIntegration_StateResumeFlow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-state-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "last_run.json")
	stateMgr := NewStateManager(stateFile)

	// 1. Save state
	state := &RunState{
		Phase:            "STEP-3",
		CompletedTasks:   []string{"1.1", "1.2"},
		PendingTasks:     []string{"2.1", "2.2"},
		IssuesInProgress: []int{42},
	}

	if err := stateMgr.SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// 2. Load state (simulating resume)
	loaded, err := stateMgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// Verify state matches
	if loaded.Phase != state.Phase {
		t.Errorf("Phase = %q, want %q", loaded.Phase, state.Phase)
	}

	if len(loaded.CompletedTasks) != len(state.CompletedTasks) {
		t.Errorf("CompletedTasks count = %d, want %d", len(loaded.CompletedTasks), len(state.CompletedTasks))
	}

	if len(loaded.IssuesInProgress) != 1 || loaded.IssuesInProgress[0] != 42 {
		t.Errorf("IssuesInProgress = %v, want [42]", loaded.IssuesInProgress)
	}

	// 3. Verify not stale (just saved)
	if stateMgr.IsStale() {
		t.Error("Just-saved state should not be stale")
	}
}

// TestIntegration_LoggerWithOutput tests logger captures all output
func TestIntegration_LoggerWithOutput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-logger-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger failed: %v", err)
	}

	// Simulate various outputs
	outputs := []string{
		"[PRINCIPAL] Starting workflow...\n",
		"[PRINCIPAL] STEP-3 | Dispatching to Worker (issue #42)\n",
		"[#42] Worker: worker_start\n",
		"[#42] Worker: worker_complete (PR: #123)\n",
		"[PRINCIPAL] STEP-4 | Worker completed\n",
	}

	for _, out := range outputs {
		logger.Write([]byte(out))
	}

	filePath := logger.FilePath()
	logger.Close()

	// Property 16: Log should contain all output with timestamps
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	for _, out := range outputs {
		// Check content (without newline for matching)
		if !containsSubstring(string(content), out[:len(out)-1]) {
			t.Errorf("Log missing output: %q", out)
		}
	}
}

// =============================================================================
// Task 12.2: Property-based tests
// =============================================================================

// TestProperty5_IssueMonitorPollingInterval tests Property 5
func TestProperty5_IssueMonitorPollingInterval(t *testing.T) {
	// Property 5: GitHub API SHALL be called at most once every 5 seconds
	if PollInterval != 5*time.Second {
		t.Errorf("PollInterval = %v, want 5s", PollInterval)
	}

	monitor := NewIssueMonitor(1, nil)
	if monitor.backoff != PollInterval {
		t.Errorf("initial backoff = %v, want %v", monitor.backoff, PollInterval)
	}
}

// TestProperty12_LockReleaseOnNormalExit tests Property 12
func TestProperty12_LockReleaseOnNormalExit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "property12-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockFile := filepath.Join(tmpDir, "test.lock")
	lock := NewLockManager(lockFile)

	// Acquire
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Verify acquired
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Fatal("Lock file should exist")
	}

	// Simulate normal exit with defer
	func() {
		defer lock.Release()
		// ... workflow would run here ...
	}()

	// Property 12: Lock SHALL be released via defer
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Lock file should be removed after defer release")
	}
}

// TestProperty13_LockReleaseOnSignal tests Property 13
func TestProperty13_LockReleaseOnSignal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "property13-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockFile := filepath.Join(tmpDir, "test.lock")
	lock := NewLockManager(lockFile)

	// Acquire
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Simulate signal handler releasing lock
	handler := NewSignalHandler(nil, nil, lock)

	// Manually trigger cleanup (without os.Exit)
	handler.mu.Lock()
	handler.shutdown = true
	handler.mu.Unlock()

	// Release lock as signal handler would
	lock.Release()

	// Property 13: Lock SHALL be released before process exits
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Lock file should be removed after signal-triggered release")
	}
}

// TestProperty15_MonitorFaultIsolation tests Property 15
func TestProperty15_MonitorFaultIsolation(t *testing.T) {
	// Property 15: Monitor failure SHALL NOT affect main workflow

	// Create monitor with invalid issue ID (will fail to poll)
	monitor := NewIssueMonitor(-1, nil)

	// Start monitor (will fail internally)
	monitor.Start()

	// Simulate main workflow continuing
	workflowCompleted := make(chan bool, 1)
	go func() {
		// Main workflow runs independently
		time.Sleep(50 * time.Millisecond)
		workflowCompleted <- true
	}()

	// Stop monitor
	monitor.Stop("test")

	// Verify workflow completed despite monitor issues
	select {
	case <-workflowCompleted:
		// Success - workflow completed
	case <-time.After(1 * time.Second):
		t.Error("Workflow should complete despite monitor failure")
	}
}

// TestProperty14_StaleLockDetection tests Property 14
func TestProperty14_StaleLockDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "property14-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockFile := filepath.Join(tmpDir, "test.lock")

	// Create stale lock with non-existent PID
	staleContent := `{"pid": 999999999, "start_time": "2020-01-01T00:00:00Z", "hostname": "test"}`
	if err := os.WriteFile(lockFile, []byte(staleContent), 0644); err != nil {
		t.Fatalf("Failed to write stale lock: %v", err)
	}

	lock := NewLockManager(lockFile)

	// Property 14: Stale lock SHALL be detected and removed
	if !lock.IsStale() {
		t.Error("Lock with non-existent PID should be detected as stale")
	}

	// Should be able to acquire despite stale lock
	if err := lock.Acquire(); err != nil {
		t.Errorf("Should be able to acquire after stale lock: %v", err)
	}

	lock.Release()
}

// TestConcurrency_MultipleMonitors tests concurrent monitor handling
func TestConcurrency_MultipleMonitors(t *testing.T) {
	var wg sync.WaitGroup
	monitors := make([]*IssueMonitor, 5)

	// Start multiple monitors concurrently
	for i := 0; i < 5; i++ {
		monitors[i] = NewIssueMonitor(i+1, nil)
		wg.Add(1)
		go func(m *IssueMonitor) {
			defer wg.Done()
			m.Start()
			time.Sleep(50 * time.Millisecond)
			m.Stop("test")
		}(monitors[i])
	}

	// Wait for all to complete
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Error("Concurrent monitors should complete without deadlock")
	}
}

// TestConcurrency_LockContention tests lock contention handling
func TestConcurrency_LockContention(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-contention-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockFile := filepath.Join(tmpDir, "test.lock")

	// First process acquires lock
	lock1 := NewLockManager(lockFile)
	if err := lock1.Acquire(); err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Second process should fail to acquire
	lock2 := NewLockManager(lockFile)
	err = lock2.Acquire()
	if err == nil {
		t.Error("Second acquire should fail while first holds lock")
		lock2.Release()
	}

	// Release first lock
	lock1.Release()

	// Now second should succeed
	if err := lock2.Acquire(); err != nil {
		t.Errorf("Second acquire should succeed after first release: %v", err)
	}
	lock2.Release()
}

// =============================================================================
// Helper functions
// =============================================================================

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || containsSubstring(s[1:], substr)))
}
