//go:build windows

package kickoff

import (
	"golang.org/x/sys/windows"
)

// processAlive checks if a process with the given PID is still running
func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}

	// STILL_ACTIVE means the process is still running
	return exitCode == 259 // STILL_ACTIVE
}
