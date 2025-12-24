//go:build !windows

package kickoff

import (
	"os"
	"syscall"
)

// processAlive checks if a process with the given PID is still running
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
