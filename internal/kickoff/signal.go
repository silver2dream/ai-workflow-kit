package kickoff

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	// GracefulTimeout is the time to wait for Principal to detect STOP marker and exit gracefully
	GracefulTimeout = 60 * time.Second

	// ForceKillTimeout is the additional time after graceful timeout before force killing
	ForceKillTimeout = 10 * time.Second

	// StopMarkerPath is the path to the STOP marker file
	StopMarkerPath = ".ai/state/STOP"
)

// SignalHandler handles graceful shutdown on signals
type SignalHandler struct {
	executor     *PTYExecutor
	state        *StateManager
	lock         *LockManager
	fanIn        *FanInManager
	monitors     []*IssueMonitor
	output       *OutputFormatter
	gracefulTO   time.Duration
	forceKillTO  time.Duration
	mu           sync.Mutex
	shutdown     bool
	shutdownOnce sync.Once
	exitCode     int
	onCleanup    func()
}

// NewSignalHandler creates a new SignalHandler
func NewSignalHandler(executor *PTYExecutor, state *StateManager, lock *LockManager) *SignalHandler {
	return &SignalHandler{
		executor:    executor,
		state:       state,
		lock:        lock,
		gracefulTO:  GracefulTimeout,
		forceKillTO: ForceKillTimeout,
		output:      NewOutputFormatter(os.Stdout),
	}
}

// SetExecutor updates the currently running Principal executor.
// This enables multi-session kickoff loops to reuse a single SignalHandler instance.
func (s *SignalHandler) SetExecutor(executor *PTYExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
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

// SetFanInManager sets the FanInManager for cleanup
func (s *SignalHandler) SetFanInManager(fanIn *FanInManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fanIn = fanIn
}

// Setup registers signal handlers for graceful shutdown
func (s *SignalHandler) Setup() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		s.HandleShutdown(sig)
	}()
}

// createStopMarker creates the STOP marker file to signal Principal to stop
func (s *SignalHandler) createStopMarker() error {
	// Ensure directory exists
	dir := filepath.Dir(StopMarkerPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Create STOP marker with timestamp
	content := fmt.Sprintf("STOP requested at %s\nReason: user_interrupted (Ctrl+C)\n",
		time.Now().Format(time.RFC3339))

	if err := os.WriteFile(StopMarkerPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create STOP marker: %w", err)
	}

	return nil
}

// removeStopMarker removes the STOP marker file
func (s *SignalHandler) removeStopMarker() {
	os.Remove(StopMarkerPath)
}

// HandleShutdown performs graceful shutdown
// Flow:
// 1. Create STOP marker → Principal detects it → Principal runs /stop-work → Principal exits
// 2. Wait for graceful timeout
// 3. If still running, force kill
// Uses sync.Once to ensure shutdown logic runs exactly once (G3 fix)
func (s *SignalHandler) HandleShutdown(sig os.Signal) {
	s.shutdownOnce.Do(func() {
		s.doShutdown(sig)
	})
}

// doShutdown performs the actual shutdown logic (called via sync.Once)
func (s *SignalHandler) doShutdown(sig os.Signal) {
	s.mu.Lock()
	s.shutdown = true
	monitors := make([]*IssueMonitor, len(s.monitors))
	copy(monitors, s.monitors)
	s.mu.Unlock()

	fmt.Println("")
	s.output.Warning(fmt.Sprintf("Received signal (%v), starting graceful shutdown...", sig))

	// Step 1: Create STOP marker to signal Principal
	s.output.Info("Creating STOP marker, waiting for Principal to exit gracefully...")
	if err := s.createStopMarker(); err != nil {
		s.output.Error(fmt.Sprintf("Failed to create STOP marker: %v", err))
		s.output.Warning("Will terminate process directly")
		s.forceShutdown(monitors)
		os.Exit(s.exitCode)
	}

	// Step 2: Wait for executor to finish gracefully
	if s.executor != nil {
		done := make(chan bool, 1)
		go func() {
			s.executor.Wait()
			done <- true
		}()

		// Show progress while waiting
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		gracefulDeadline := time.After(s.gracefulTO)
		elapsed := 0

		for {
			select {
			case <-done:
				s.output.Success("Principal exited gracefully")
				s.cleanup(monitors, true)
				os.Exit(s.exitCode)

			case <-ticker.C:
				elapsed += 10
				remaining := int(s.gracefulTO.Seconds()) - elapsed
				s.output.Info(fmt.Sprintf("Waiting for Principal to exit... (%d seconds remaining)", remaining))

			case <-gracefulDeadline:
				s.output.Warning(fmt.Sprintf("Timeout after %v, Principal did not exit", s.gracefulTO))
				s.output.Warning(fmt.Sprintf("Waiting %v more before force kill...", s.forceKillTO))

				// Give a bit more time before force kill
				forceDeadline := time.After(s.forceKillTO)
				select {
				case <-done:
					s.output.Success("Principal exited")
					s.cleanup(monitors, true)
					os.Exit(s.exitCode)
				case <-forceDeadline:
					s.output.Error("Force killing process")
					s.forceShutdown(monitors)
					os.Exit(s.exitCode)
				}
			}
		}
	} else {
		s.cleanup(monitors, true)
		os.Exit(s.exitCode)
	}
}

// forceShutdown forcefully terminates all processes
func (s *SignalHandler) forceShutdown(monitors []*IssueMonitor) {
	// Kill executor
	if s.executor != nil {
		s.output.Warning("Force killing Claude process...")
		s.executor.Kill()
	}

	// Kill any worker processes
	s.killWorkerProcesses()

	s.cleanup(monitors, false)
}

// killWorkerProcesses attempts to kill any running worker (codex) processes
func (s *SignalHandler) killWorkerProcesses() {
	// Try to find and kill codex processes started by this workflow
	// This is a best-effort cleanup

	// Method 1: Check for PID files in .ai/state/
	pidFiles := []string{
		".ai/state/worker_pid.txt",
		".ai/state/codex_pid.txt",
	}

	for _, pidFile := range pidFiles {
		if data, err := os.ReadFile(pidFile); err == nil {
			var pid int
			if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil && pid > 0 {
				if proc, err := os.FindProcess(pid); err == nil {
					s.output.Warning(fmt.Sprintf("Killing Worker process (PID: %d)...", pid))
					proc.Signal(syscall.SIGTERM)
					// Give it a moment to terminate
					time.Sleep(500 * time.Millisecond)
					proc.Kill()
				}
			}
			os.Remove(pidFile)
		}
	}
}

// cleanup performs final cleanup operations
// G6 fix: Does not call os.Exit() directly - caller handles exit
func (s *SignalHandler) cleanup(monitors []*IssueMonitor, graceful bool) {
	// Stop all monitors
	for _, m := range monitors {
		m.Stop("process_exit")
	}

	// Stop fan-in manager (stops all tailers, waits, closes channel)
	if s.fanIn != nil {
		s.fanIn.Stop()
	}

	// Run cleanup callback
	if s.onCleanup != nil {
		s.onCleanup()
	}

	// Release lock
	if s.lock != nil {
		s.lock.Release()
	}

	// Close executor
	if s.executor != nil {
		s.executor.Close()
	}

	// Print summary
	s.printSummary(graceful)

	// Store exit code instead of calling os.Exit() (G6 fix)
	if graceful {
		s.exitCode = 0
	} else {
		s.exitCode = 1
	}
}

// printSummary prints a summary of the shutdown
func (s *SignalHandler) printSummary(graceful bool) {
	fmt.Println("")
	fmt.Println("========================================")
	if graceful {
		fmt.Println("  Workflow stopped gracefully")
		fmt.Println("  Report generated (by Principal /stop-work)")
	} else {
		fmt.Println("  Workflow stopped forcefully")
		fmt.Println("  Warning: Report may not be generated")
		fmt.Println("  Warning: Orphan Worker processes may exist")
	}
	fmt.Println("========================================")
	fmt.Println("")
	fmt.Println("Before next run, please verify:")
	fmt.Println("  1. Check for orphan processes: ps aux | grep codex")
	fmt.Println("  2. Check worktree status: ls .worktrees/")
	fmt.Println("  3. Restart: awkit kickoff")
	fmt.Println("")
}

// IsShutdown returns true if shutdown has been initiated
func (s *SignalHandler) IsShutdown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdown
}
