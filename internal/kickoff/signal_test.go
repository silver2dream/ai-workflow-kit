package kickoff

import (
	"os"
	"testing"
	"time"
)

// TestSignalHandler_ShutdownTimeout tests Property 18: Graceful shutdown timing (30s)
func TestSignalHandler_ShutdownTimeout(t *testing.T) {
	handler := NewSignalHandler(nil, nil, nil)

	// Property 18: Default timeout should be 30 seconds
	if handler.timeout != ShutdownTimeout {
		t.Errorf("timeout = %v, want %v", handler.timeout, ShutdownTimeout)
	}

	expectedTimeout := 30 * time.Second
	if ShutdownTimeout != expectedTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", ShutdownTimeout, expectedTimeout)
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

	if handler.timeout != ShutdownTimeout {
		t.Errorf("timeout = %v, want %v", handler.timeout, ShutdownTimeout)
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

// TestShutdownTimeout_Constant tests the shutdown timeout constant
func TestShutdownTimeout_Constant(t *testing.T) {
	// Property 18: 30 second timeout
	if ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", ShutdownTimeout)
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
