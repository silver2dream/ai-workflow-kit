//go:build !windows

package session

import (
	"os"
	"syscall"
)

// IsProcessRunning checks if a process is running and hasn't been reused (Unix)
func IsProcessRunning(pid int, expectedStartTime int64) bool {
	if pid <= 0 {
		return false
	}

	// Check if process exists using kill -0
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to signal check
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false // Process does not exist
	}

	// Get actual process start time from /proc (Linux)
	// or use stat on /proc/pid (Linux)
	procDir := "/proc"
	if fi, err := os.Stat(procDir); err == nil && fi.IsDir() {
		// Linux: check /proc/[pid]/stat
		procPidDir := procDir + "/" + itoa(pid)
		if fi, err := os.Stat(procPidDir); err == nil {
			actualStart := fi.ModTime().Unix()
			// Allow 2 second tolerance for start time comparison
			diff := actualStart - expectedStartTime
			if diff < 0 {
				diff = -diff
			}
			if diff <= 2 {
				return true // Same process, still running
			}
			return false // PID was reused
		}
	}

	// Fallback: if we can't verify start time, assume it's the same process
	// This is less safe but prevents false negatives on systems without /proc
	return true
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
