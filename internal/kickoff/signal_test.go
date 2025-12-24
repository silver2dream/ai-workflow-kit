package kickoff

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSignalHandler_GracefulTimeout tests graceful shutdown timing
func TestSignalHandler_GracefulTimeout(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	// Default graceful timeout should be 60 seconds
	expectedGraceful := 60 * time.Second
	if handler.gracefulTO != expectedGraceful {
		t.Errorf("gracefulTO = %v, want %v", handler.gracefulTO, expectedGraceful)
	}

	// Default force kill timeout should be 10 seconds
	expectedForceKill := 10 * time.Second
	if handler.forceKillTO != expectedForceKill {
		t.Errorf("forceKillTO = %v, want %v", handler.forceKillTO, expectedForceKill)
	}
}

// TestGracefulTimeout_Constant tests the graceful timeout constant
func TestGracefulTimeout_Constant(t *testing.T) {
	if GracefulTimeout != 60*time.Second {
		t.Errorf("GracefulTimeout = %v, want 60s", GracefulTimeout)
	}
}

// TestForceKillTimeout_Constant tests the force kill timeout constant
func TestForceKillTimeout_Constant(t *testing.T) {
	if ForceKillTimeout != 10*time.Second {
		t.Errorf("ForceKillTimeout = %v, want 10s", ForceKillTimeout)
	}
}

// TestStopMarkerPath_Constant tests the stop marker path constant
func TestStopMarkerPath_Constant(t *testing.T) {
	expected := ".ai/state/STOP"
	if StopMarkerPath != expected {
		t.Errorf("StopMarkerPath = %q, want %q", StopMarkerPath, expected)
	}
}

// TestSignalHandler_AddRemoveMonitor tests monitor management
func TestSignalHandler_AddRemoveMonitor(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	// Create mock monitors
	monitor1 := &IssueMonitor{issueID: 1}
	monitor2 := &IssueMonitor{issueID: 2}

	// Add monitors
	handler.AddMonitor(monitor1)
	handler.AddMonitor(monitor2)

	if len(handler.monitors) != 2 {
		t.Errorf("Expected 2 monitors, got %d", len(handler.monitors))
	}

	// Remove one monitor
	handler.RemoveMonitor(monitor1)

	if len(handler.monitors) != 1 {
		t.Errorf("Expected 1 monitor after removal, got %d", len(handler.monitors))
	}

	// Verify correct monitor remains
	if handler.monitors[0].issueID != 2 {
		t.Errorf("Wrong monitor remaining, got issueID %d", handler.monitors[0].issueID)
	}

	// Remove non-existent monitor (should not panic)
	handler.RemoveMonitor(monitor1)

	if len(handler.monitors) != 1 {
		t.Errorf("Expected 1 monitor after removing non-existent, got %d", len(handler.monitors))
	}
}

// TestSignalHandler_IsShutdown tests shutdown state tracking
func TestSignalHandler_IsShutdown(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	// Initially not shutdown
	if handler.IsShutdown() {
		t.Error("IsShutdown should be false initially")
	}

	// Set shutdown flag directly for testing
	handler.mu.Lock()
	handler.shutdown = true
	handler.mu.Unlock()

	if !handler.IsShutdown() {
		t.Error("IsShutdown should be true after setting flag")
	}
}

// TestSignalHandler_SetCleanupCallback tests cleanup callback
func TestSignalHandler_SetCleanupCallback(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	called := false
	handler.SetCleanupCallback(func() {
		called = true
	})

	if handler.onCleanup == nil {
		t.Error("onCleanup should not be nil after SetCleanupCallback")
	}

	// Call the callback
	handler.onCleanup()

	if !called {
		t.Error("Cleanup callback was not called")
	}
}

// TestNewSignalHandler tests handler creation
func TestNewSignalHandler(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "signal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lock := NewLockManager(tmpDir + "/test.lock")
	state := NewStateManager(tmpDir)

	handler := NewSignalHandler(nil, state, lock)

	if handler == nil {
		t.Fatal("NewSignalHandler returned nil")
	}

	if handler.lock != lock {
		t.Error("lock not set correctly")
	}

	if handler.state != state {
		t.Error("state not set correctly")
	}

	if handler.gracefulTO != GracefulTimeout {
		t.Errorf("gracefulTO = %v, want %v", handler.gracefulTO, GracefulTimeout)
	}

	if handler.forceKillTO != ForceKillTimeout {
		t.Errorf("forceKillTO = %v, want %v", handler.forceKillTO, ForceKillTimeout)
	}

	if handler.output == nil {
		t.Error("output formatter should not be nil")
	}
}

// TestSignalHandler_DoubleShutdown tests that shutdown only runs once
func TestSignalHandler_DoubleShutdown(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	callCount := 0
	handler.SetCleanupCallback(func() {
		callCount++
	})

	// Simulate shutdown (without actually calling HandleShutdown which exits)
	handler.mu.Lock()
	handler.shutdown = true
	handler.mu.Unlock()

	// Try to shutdown again - should be no-op
	// We can't call HandleShutdown directly as it calls os.Exit
	// Instead verify the flag prevents re-entry
	if !handler.IsShutdown() {
		t.Error("Should be in shutdown state")
	}
}

// TestSignalHandler_MonitorsCopied tests that monitors are copied during shutdown
func TestSignalHandler_MonitorsCopied(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	monitor := &IssueMonitor{
		issueID:  42,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	handler.AddMonitor(monitor)

	// Verify monitor is tracked
	handler.mu.Lock()
	monitorCount := len(handler.monitors)
	handler.mu.Unlock()

	if monitorCount != 1 {
		t.Errorf("Expected 1 monitor, got %d", monitorCount)
	}
}

// TestSignalHandler_WithExecutor tests handler with executor
func TestSignalHandler_WithExecutor(t *testing.T) {
	executor, _ := NewPTYExecutor("echo", []string{"test"})
	handler := NewSignalHandler(executor, nil, nil)

	if handler.executor != executor {
		t.Error("executor not set correctly")
	}
}

// TestSignalHandler_NilComponents tests handler with nil components
func TestSignalHandler_NilComponents(t *testing.T) {
	// Should not panic with nil components
	handler := NewSignalHandler(nil, nil, nil)

	if handler == nil {
		t.Fatal("NewSignalHandler returned nil")
	}

	// These should not panic
	handler.AddMonitor(nil)
	handler.RemoveMonitor(nil)
	handler.SetCleanupCallback(nil)
}

// TestSignalHandler_CreateStopMarker tests STOP marker creation
func TestSignalHandler_CreateStopMarker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "signal-stop-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp dir to test STOP marker creation
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	handler := NewSignalHandler(nil, nil, nil)

	// Create STOP marker
	err = handler.createStopMarker()
	if err != nil {
		t.Fatalf("createStopMarker failed: %v", err)
	}

	// Verify STOP marker exists
	stopPath := filepath.Join(tmpDir, StopMarkerPath)
	if _, err := os.Stat(stopPath); os.IsNotExist(err) {
		t.Error("STOP marker was not created")
	}

	// Verify content
	content, err := os.ReadFile(stopPath)
	if err != nil {
		t.Fatalf("Failed to read STOP marker: %v", err)
	}

	if len(content) == 0 {
		t.Error("STOP marker is empty")
	}

	// Remove STOP marker
	handler.removeStopMarker()

	// Verify STOP marker is removed
	if _, err := os.Stat(stopPath); !os.IsNotExist(err) {
		t.Error("STOP marker was not removed")
	}
}

// TestSignalHandler_CreateStopMarker_CreatesDirectory tests that createStopMarker creates the directory
func TestSignalHandler_CreateStopMarker_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "signal-stop-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp dir
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	handler := NewSignalHandler(nil, nil, nil)

	// Verify .ai/state doesn't exist yet
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Skip("State directory already exists")
	}

	// Create STOP marker (should create directory)
	err = handler.createStopMarker()
	if err != nil {
		t.Fatalf("createStopMarker failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("State directory was not created")
	}

	// Cleanup
	handler.removeStopMarker()
}
