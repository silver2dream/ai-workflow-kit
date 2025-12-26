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
	executor    *PTYExecutor
	state       *StateManager
	lock        *LockManager
	fanIn       *FanInManager
	monitors    []*IssueMonitor
	output      *OutputFormatter
	gracefulTO  time.Duration
	forceKillTO time.Duration
	mu          sync.Mutex
	shutdown    bool
	onCleanup   func()
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
func (s *SignalHandler) HandleShutdown(sig os.Signal) {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return
	}
	s.shutdown = true
	monitors := make([]*IssueMonitor, len(s.monitors))
	copy(monitors, s.monitors)
	s.mu.Unlock()

	fmt.Println("")
	s.output.Warning(fmt.Sprintf("收到中斷信號 (%v)，開始優雅關閉...", sig))

	// Step 1: Create STOP marker to signal Principal
	s.output.Info("建立 STOP marker，等待 Principal 優雅退出...")
	if err := s.createStopMarker(); err != nil {
		s.output.Error(fmt.Sprintf("無法建立 STOP marker: %v", err))
		s.output.Warning("將直接終止進程")
		s.forceShutdown(monitors)
		return
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
				s.output.Success("Principal 已優雅退出")
				s.cleanup(monitors, true)
				return

			case <-ticker.C:
				elapsed += 10
				remaining := int(s.gracefulTO.Seconds()) - elapsed
				s.output.Info(fmt.Sprintf("等待 Principal 退出... (剩餘 %d 秒)", remaining))

			case <-gracefulDeadline:
				s.output.Warning(fmt.Sprintf("等待超過 %v，Principal 未退出", s.gracefulTO))
				s.output.Warning(fmt.Sprintf("再等待 %v 後將強制終止...", s.forceKillTO))

				// Give a bit more time before force kill
				forceDeadline := time.After(s.forceKillTO)
				select {
				case <-done:
					s.output.Success("Principal 已退出")
					s.cleanup(monitors, true)
					return
				case <-forceDeadline:
					s.output.Error("強制終止進程")
					s.forceShutdown(monitors)
					return
				}
			}
		}
	} else {
		s.cleanup(monitors, true)
	}
}

// forceShutdown forcefully terminates all processes
func (s *SignalHandler) forceShutdown(monitors []*IssueMonitor) {
	// Kill executor
	if s.executor != nil {
		s.output.Warning("強制終止 Claude 進程...")
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
					s.output.Warning(fmt.Sprintf("終止 Worker 進程 (PID: %d)...", pid))
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

	if graceful {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

// printSummary prints a summary of the shutdown
func (s *SignalHandler) printSummary(graceful bool) {
	fmt.Println("")
	fmt.Println("========================================")
	if graceful {
		fmt.Println("  工作流程已優雅停止")
		fmt.Println("  報告已生成（由 Principal 執行 /stop-work）")
	} else {
		fmt.Println("  工作流程已強制停止")
		fmt.Println("  ⚠ 報告可能未生成")
		fmt.Println("  ⚠ 可能有孤立的 Worker 進程")
	}
	fmt.Println("========================================")
	fmt.Println("")
	fmt.Println("下次啟動前，請確認：")
	fmt.Println("  1. 檢查是否有孤立進程: ps aux | grep codex")
	fmt.Println("  2. 檢查 worktree 狀態: ls .worktrees/")
	fmt.Println("  3. 重新啟動: awkit kickoff")
	fmt.Println("")
}

// IsShutdown returns true if shutdown has been initiated
func (s *SignalHandler) IsShutdown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdown
}
