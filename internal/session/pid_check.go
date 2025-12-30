package session

// IsProcessRunning checks if a process is running and hasn't been reused
// pid: the process ID to check
// startTime: the expected start time (Unix timestamp)
// Returns true if the process is running and was started at the expected time
//
// This is a cross-platform function with implementations in:
// - pid_check_unix.go (Linux, macOS, BSD)
// - pid_check_windows.go (Windows)
