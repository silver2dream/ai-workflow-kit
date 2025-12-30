//go:build windows

package worker

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess              = modkernel32.NewProc("OpenProcess")
	procGetProcessTimes          = modkernel32.NewProc("GetProcessTimes")
	procCloseHandle              = modkernel32.NewProc("CloseHandle")
)

const (
	PROCESS_QUERY_INFORMATION = 0x0400
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

// isProcessRunningOS checks if a process is running on Windows
func isProcessRunningOS(pid int, expectedStartTime int64) bool {
	// Try to open the process with query permission
	handle, _, err := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_LIMITED_INFORMATION),
		0,
		uintptr(pid),
	)

	if handle == 0 {
		return false
	}
	defer procCloseHandle.Call(handle)

	// If no expected start time, just return true (process exists)
	if expectedStartTime == 0 {
		return true
	}

	// Get process times to verify start time
	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	ret, _, err := procGetProcessTimes.Call(
		handle,
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)

	if ret == 0 {
		// Failed to get process times, assume it's running
		_ = err
		return true
	}

	// Convert Windows FILETIME to Unix timestamp
	// FILETIME is 100-nanosecond intervals since January 1, 1601
	// Unix epoch is January 1, 1970
	const epochDiff = 116444736000000000 // 100-ns intervals between 1601 and 1970
	creationNs := (int64(creationTime.HighDateTime)<<32 | int64(creationTime.LowDateTime))
	creationUnix := (creationNs - epochDiff) / 10000000 // Convert to seconds

	// Allow 2 second tolerance for timing differences
	diff := creationUnix - expectedStartTime
	if diff < 0 {
		diff = -diff
	}

	return diff <= 2
}

// filetimeToTime converts Windows FILETIME to Go time.Time
func filetimeToTime(ft syscall.Filetime) time.Time {
	const epochDiff = 116444736000000000
	ns := (int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)) - epochDiff
	return time.Unix(0, ns*100)
}
