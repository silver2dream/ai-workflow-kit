package kickoff

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout = 30 * time.Second
)

// SignalHandler handles graceful shutdown on signals
type SignalHandler struct {
	executor  *PTYExecutor
	state     *StateManager
	lock      *LockManager
	monitors  []*IssueMonitor
	output    *OutputFormatter
	timeout   time.Duration
	mu        sync.Mutex
	shutdown  bool
	onCleanup func()
}

// NewSignalHandler creates a new SignalHandler
func NewSignalHandler(executor *PTYExecutor, state *StateManager, lock *LockManager) *SignalHandler {
	return &SignalHandler{
		executor: executor,
		state:    state,
		lock:     lock,
		timeout:  ShutdownTimeout,
		output:   NewOutputFormatter(os.Stdout),
	}
}

// AddMonitor adds an IssueMonitor to be cleaned up on shutdown
func (s *SignalHandler) AddMonitor(m *IssueMonitor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.monitors = append(s.monitors, m)
}

// RemoveMonitor removes an IssueMonitor from the cleanup list
func (s *SignalHandler) RemoveMonitor(m *IssueMonitor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, monitor := range s.monitors {
		if monitor == m {
			s.monitors = append(s.monitors[:i], s.monitors[i+1:]...)
			break
		}
	}
}

// SetCleanupCallback sets a callback to be called during cleanup
func (s *SignalHandler) SetCleanupCallback(cb func()) {
	s.onCleanup = cb
}

// Setup registers signal handlers for graceful shutdown
func (s *SignalHandler) Setup() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		s.HandleShutdown()
	}()
}

// HandleShutdown performs graceful shutdown
func (s *SignalHandler) HandleShutdown() {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return
	}
	s.shutdown = true
	monitors := make([]*IssueMonitor, len(s.monitors))
	copy(monitors, s.monitors)
	s.mu.Unlock()

	fmt.Println("\n收到中斷信號，正在關閉...")

	// Stop all monitors
	for _, m := range monitors {
		m.Stop("process_exit")
	}

	// Run cleanup callback
	if s.onCleanup != nil {
		s.onCleanup()
	}

	// Release lock
	if s.lock != nil {
		s.lock.Release()
	}

	// Wait for executor to finish with timeout
	if s.executor != nil {
		done := make(chan bool, 1)
		go func() {
			s.executor.Wait()
			done <- true
		}()

		select {
		case <-done:
			s.output.Info("操作已完成")
		case <-time.After(s.timeout):
			s.output.Warning(fmt.Sprintf("等待超過 %v，強制終止", s.timeout))
			s.executor.Kill()
		}
	}

	// Save state
	if s.state != nil {
		// State saving is handled by the caller
	}

	// Print summary
	s.printSummary()

	os.Exit(1)
}

// printSummary prints a summary of the shutdown
func (s *SignalHandler) printSummary() {
	fmt.Println("")
	fmt.Println("工作流程已中斷")
	fmt.Println("使用 'awkit kickoff --resume' 繼續上次的工作")
}

// IsShutdown returns true if shutdown has been initiated
func (s *SignalHandler) IsShutdown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdown
}
