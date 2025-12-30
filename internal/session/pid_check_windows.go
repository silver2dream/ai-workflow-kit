//go:build windows

package session

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess         = kernel32.NewProc("OpenProcess")
	procGetProcessTimes     = kernel32.NewProc("GetProcessTimes")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
)

const (
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

// IsProcessRunning checks if a process is running and hasn't been reused (Windows)
func IsProcessRunning(pid int, expectedStartTime int64) bool {
	if pid <= 0 {
		return false
	}

	// Open process with limited query rights
	handle, _, err := procOpenProcess.Call(
		PROCESS_QUERY_LIMITED_INFORMATION,
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return false // Process does not exist or access denied
	}
	defer procCloseHandle.Call(handle)

	// Get process times
	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	ret, _, err := procGetProcessTimes.Call(
		handle,
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if ret == 0 {
		// Can't get process times, assume running if handle was valid
		return err == nil || err == syscall.Errno(0)
	}

	// Convert FILETIME to Unix timestamp
	// FILETIME is 100-nanosecond intervals since January 1, 1601 UTC
	actualStart := filetimeToUnix(creationTime)

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

// filetimeToUnix converts Windows FILETIME to Unix timestamp
func filetimeToUnix(ft syscall.Filetime) int64 {
	// Convert to time.Time using standard library method
	nsec := int64(ft.HighDateTime)<<32 + int64(ft.LowDateTime)
	// FILETIME starts from 1601-01-01, Unix from 1970-01-01
	// Difference is 116444736000000000 100-nanosecond intervals
	nsec -= 116444736000000000
	if nsec < 0 {
		return 0
	}
	t := time.Unix(0, nsec*100)
	return t.Unix()
}
