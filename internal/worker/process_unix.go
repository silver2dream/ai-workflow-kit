//go:build !windows

package worker

import (
	"os"
	"syscall"
)

// isProcessRunningOS checks if a process is running on Unix systems
func isProcessRunningOS(pid int, expectedStartTime int64) bool {
	// Try to find the process
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds - we need to send signal 0 to check
	err = proc.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}

	// Process exists. If we have an expected start time, we should verify it
	// but on Unix getting process start time requires reading /proc which may not
	// be available on all systems (e.g., macOS). For now, just return true.
	// The caller can use additional checks if needed.
	return true
}
