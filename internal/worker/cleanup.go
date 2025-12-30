package worker

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// CleanupManager handles cleanup on exit with proper signal handling
// This fixes S2: dispatch_worker cleanup trap doesn't execute
type CleanupManager struct {
	mu         sync.Mutex
	done       bool
	cleanupFns []func()
	sigChan    chan os.Signal
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewCleanupManager creates a new cleanup manager with signal handling
func NewCleanupManager() *CleanupManager {
	ctx, cancel := context.WithCancel(context.Background())
	cm := &CleanupManager{
		sigChan: make(chan os.Signal, 1),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Register signal handlers
	signal.Notify(cm.sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler goroutine
	go cm.handleSignals()

	return cm
}

// Register adds a cleanup function to be called on exit
// Cleanup functions are called in reverse order (LIFO)
func (cm *CleanupManager) Register(fn func()) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cleanupFns = append(cm.cleanupFns, fn)
}

// Context returns a context that is cancelled on signal
func (cm *CleanupManager) Context() context.Context {
	return cm.ctx
}

// Done returns a channel that is closed when cleanup is triggered
func (cm *CleanupManager) Done() <-chan struct{} {
	return cm.ctx.Done()
}

// handleSignals handles incoming signals
func (cm *CleanupManager) handleSignals() {
	select {
	case sig := <-cm.sigChan:
		// Cancel context first
		cm.cancel()
		// Run cleanup
		cm.runCleanup()
		// Exit with appropriate code
		if sig == syscall.SIGINT {
			os.Exit(130) // 128 + SIGINT(2)
		}
		os.Exit(143) // 128 + SIGTERM(15)
	case <-cm.ctx.Done():
		// Context cancelled by Cleanup()
		return
	}
}

// Cleanup runs all cleanup functions and should be called via defer
func (cm *CleanupManager) Cleanup() {
	cm.runCleanup()
	// Cancel context to stop signal handler
	cm.cancel()
	// Stop receiving signals
	signal.Stop(cm.sigChan)
	close(cm.sigChan)
}

// runCleanup executes cleanup functions in reverse order
func (cm *CleanupManager) runCleanup() {
	cm.mu.Lock()
	if cm.done {
		cm.mu.Unlock()
		return
	}
	cm.done = true
	fns := make([]func(), len(cm.cleanupFns))
	copy(fns, cm.cleanupFns)
	cm.mu.Unlock()

	// Run in reverse order (LIFO)
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}

// DispatchCleanup contains state for dispatch-specific cleanup
type DispatchCleanup struct {
	IssueNumber int
	StateRoot   string
	GHClient    *GitHubClient
	done        bool
	mu          sync.Mutex
}

// NewDispatchCleanup creates a new dispatch cleanup handler
func NewDispatchCleanup(issueNumber int, stateRoot string, ghClient *GitHubClient) *DispatchCleanup {
	return &DispatchCleanup{
		IssueNumber: issueNumber,
		StateRoot:   stateRoot,
		GHClient:    ghClient,
	}
}

// Run performs cleanup for dispatch worker
// - Removes in-progress label if result is not success
// - Cleans up PID file
func (dc *DispatchCleanup) Run() {
	dc.mu.Lock()
	if dc.done {
		dc.mu.Unlock()
		return
	}
	dc.done = true
	dc.mu.Unlock()

	ctx := context.Background()

	// Check if result indicates success
	result, err := LoadResult(dc.StateRoot, dc.IssueNumber)
	if err == nil && result.Status == "success" {
		// Don't remove in-progress label on success
		// The main flow will update labels appropriately
		return
	}

	// Remove in-progress label on failure/crash/signal
	if dc.GHClient != nil {
		_ = dc.GHClient.RemoveLabel(ctx, dc.IssueNumber, "in-progress")
	}

	// Cleanup PID file
	_ = CleanupPIDFile(dc.StateRoot, dc.IssueNumber)
}
