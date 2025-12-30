package kickoff

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// LockInfo contains information about the lock holder
type LockInfo struct {
	PID       int       `json:"pid"`
	StartTime time.Time `json:"start_time"`
	Hostname  string    `json:"hostname"`
}

// LockManager handles single-instance protection via lock files
type LockManager struct {
	lockFile string
	acquired bool
}

// ProcessAlive reports whether a process with the given PID is still running.
// This is exposed for other packages (e.g. offline status inspection).
func ProcessAlive(pid int) bool {
	return processAlive(pid)
}

// NewLockManager creates a new LockManager for the given lock file path
func NewLockManager(lockFile string) *LockManager {
	return &LockManager{
		lockFile: lockFile,
	}
}

// Acquire attempts to acquire the lock file using atomic O_EXCL to prevent TOCTOU race
func (l *LockManager) Acquire() error {
	// Ensure directory exists
	dir := filepath.Dir(l.lockFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Try to create lock file atomically with O_EXCL
	f, err := os.OpenFile(l.lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Lock file exists - check if stale
			info, readErr := l.readLockInfo()
			if readErr != nil {
				// Can't read lock info, try to remove and retry once
				os.Remove(l.lockFile)
				return l.acquireOnce()
			}

			if !processAlive(info.PID) {
				// Process is dead, remove stale lock and retry once
				os.Remove(l.lockFile)
				return l.acquireOnce()
			}

			// Another instance is running
			return fmt.Errorf("another instance is running (PID: %d, started: %s)",
				info.PID, info.StartTime.Format(time.RFC3339))
		}
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	// Write lock info to the opened file
	if err := l.writeLockInfoTo(f); err != nil {
		f.Close()
		os.Remove(l.lockFile)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(l.lockFile)
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	l.acquired = true
	return nil
}

// acquireOnce attempts to acquire the lock without retry (to prevent infinite recursion)
func (l *LockManager) acquireOnce() error {
	f, err := os.OpenFile(l.lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			info, _ := l.readLockInfo()
			if info != nil && processAlive(info.PID) {
				return fmt.Errorf("another instance is running (PID: %d, started: %s)",
					info.PID, info.StartTime.Format(time.RFC3339))
			}
			return fmt.Errorf("lock file exists and could not be acquired")
		}
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	if err := l.writeLockInfoTo(f); err != nil {
		f.Close()
		os.Remove(l.lockFile)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(l.lockFile)
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	l.acquired = true
	return nil
}

// Release removes the lock file
func (l *LockManager) Release() error {
	if !l.acquired {
		return nil
	}

	if err := os.Remove(l.lockFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	l.acquired = false
	return nil
}

// IsStale checks if the lock file is stale (process no longer running)
func (l *LockManager) IsStale() bool {
	info, err := l.readLockInfo()
	if err != nil {
		return false
	}

	return !processAlive(info.PID)
}

// SetupSignalHandler registers signal handlers to release lock on termination
func (l *LockManager) SetupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		l.Release()
		os.Exit(1)
	}()
}

// readLockInfo reads the lock file and returns the lock info
func (l *LockManager) readLockInfo() (*LockInfo, error) {
	data, err := os.ReadFile(l.lockFile)
	if err != nil {
		return nil, err
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// writeLockInfoTo writes the current process info to an already-opened file handle
func (l *LockManager) writeLockInfoTo(f *os.File) error {
	hostname, _ := os.Hostname()
	info := LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now(),
		Hostname:  hostname,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock info: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write lock info: %w", err)
	}

	return nil
}

// writeLockInfo writes the current process info to the lock file
func (l *LockManager) writeLockInfo() error {
	hostname, _ := os.Hostname()
	info := LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now(),
		Hostname:  hostname,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock info: %w", err)
	}

	if err := os.WriteFile(l.lockFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}
