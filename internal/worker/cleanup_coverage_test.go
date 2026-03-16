package worker

import (
	"os/signal"
	"sync/atomic"
	"testing"
)

// safeCleanup tears down a CleanupManager without risking double-close or
// signal-handler os.Exit. The existing tests in worker_extra_test.go use the
// same pattern (runCleanup + cancel).
func safeCleanup(cm *CleanupManager) {
	signal.Stop(cm.sigChan)
	cm.cancel()
	cm.runCleanup()
}

// ---------------------------------------------------------------------------
// CleanupManager
// ---------------------------------------------------------------------------

func TestCov_CleanupManager_RegisterAndCleanup(t *testing.T) {
	cm := NewCleanupManager()
	defer safeCleanup(cm)

	var order []int
	cm.Register(func() { order = append(order, 1) })
	cm.Register(func() { order = append(order, 2) })
	cm.Register(func() { order = append(order, 3) })

	cm.runCleanup()

	// Should be called in reverse order (LIFO)
	if len(order) != 3 {
		t.Fatalf("expected 3 cleanup calls, got %d", len(order))
	}
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("expected LIFO order [3,2,1], got %v", order)
	}
}

func TestCov_CleanupManager_RunCleanupOnce(t *testing.T) {
	cm := NewCleanupManager()
	defer safeCleanup(cm)

	var count int32
	cm.Register(func() { atomic.AddInt32(&count, 1) })

	cm.runCleanup()
	cm.runCleanup() // second call must be a no-op

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected cleanup to run exactly once, got %d", count)
	}
}

func TestCov_CleanupManager_Context(t *testing.T) {
	cm := NewCleanupManager()
	defer safeCleanup(cm)

	ctx := cm.Context()
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	// Context should not be done yet
	select {
	case <-ctx.Done():
		t.Error("context should not be done before cleanup")
	default:
		// ok
	}
}

func TestCov_CleanupManager_Done(t *testing.T) {
	cm := NewCleanupManager()

	done := cm.Done()
	if done == nil {
		t.Error("expected non-nil done channel")
	}

	safeCleanup(cm)

	// After cleanup, done channel should be closed
	select {
	case <-done:
		// ok
	default:
		t.Error("done channel should be closed after cleanup")
	}
}

func TestCov_CleanupManager_EmptyCleanup(t *testing.T) {
	cm := NewCleanupManager()
	// No registrations, cleanup should not panic
	safeCleanup(cm)
}
