package worker

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// CleanupManager (cleanup.go)
// ---------------------------------------------------------------------------

func TestNewCleanupManager_NotNil(t *testing.T) {
	cm := NewCleanupManager()
	if cm == nil {
		t.Fatal("NewCleanupManager should return non-nil")
	}
	// Clean up properly (avoid calling cm.Cleanup() which closes sigChan
	// and can race with the signal handler goroutine)
	safeCleanup(cm)
}

func TestCleanupManager_Context(t *testing.T) {
	cm := NewCleanupManager()
	ctx := cm.Context()
	if ctx == nil {
		t.Fatal("Context should not be nil")
	}
	safeCleanup(cm)
}

func TestCleanupManager_Done_ClosedAfterCleanup(t *testing.T) {
	cm := NewCleanupManager()
	safeCleanup(cm)

	// After cleanup, context should be done
	select {
	case <-cm.Done():
		// Expected - done channel is closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Done() should be closed after Cleanup()")
	}
}

func TestCleanupManager_Register_CallsOnCleanup(t *testing.T) {
	cm := NewCleanupManager()
	called := false
	cm.Register(func() {
		called = true
	})

	safeCleanup(cm)

	if !called {
		t.Error("Registered cleanup function should have been called")
	}
}

func TestCleanupManager_Register_MultipleCallbacks(t *testing.T) {
	cm := NewCleanupManager()
	order := []int{}
	cm.Register(func() { order = append(order, 1) })
	cm.Register(func() { order = append(order, 2) })
	cm.Register(func() { order = append(order, 3) })

	safeCleanup(cm)

	// LIFO order: 3, 2, 1
	if len(order) != 3 {
		t.Fatalf("expected 3 callbacks called, got %d", len(order))
	}
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("cleanup functions should be called in LIFO order, got: %v", order)
	}
}

func TestCleanupManager_CallbackCalledOnce(t *testing.T) {
	cm := NewCleanupManager()
	count := 0
	cm.Register(func() { count++ })

	safeCleanup(cm)

	if count != 1 {
		t.Errorf("cleanup function should be called exactly once, got %d times", count)
	}
}

func TestCleanupManager_Register_NoCallbacks(t *testing.T) {
	cm := NewCleanupManager()
	// No functions registered — should not panic
	safeCleanup(cm)
}

// ---------------------------------------------------------------------------
// DispatchCleanup (cleanup.go)
// ---------------------------------------------------------------------------

func TestNewDispatchCleanup_NotNil(t *testing.T) {
	dc := NewDispatchCleanup(42, "/tmp/state", nil)
	if dc == nil {
		t.Fatal("NewDispatchCleanup should return non-nil")
	}
	if dc.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", dc.IssueNumber)
	}
}

func TestDispatchCleanup_RunNilGHClient(t *testing.T) {
	dc := NewDispatchCleanup(1, "/tmp/state", nil)
	// Should not panic even with nil GH client
	dc.Run()
}

func TestDispatchCleanup_DoubleRun(t *testing.T) {
	dc := NewDispatchCleanup(1, "/tmp/state", nil)
	dc.Run()
	// Second call should be no-op
	dc.Run()
}
