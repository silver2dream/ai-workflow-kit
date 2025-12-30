package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PIDFile tracks worker process for crash recovery
type PIDFile struct {
	PID         int       `json:"pid"`
	StartTime   int64     `json:"start_time"` // Unix timestamp
	IssueNumber int       `json:"issue_number"`
	SessionID   string    `json:"session_id"`
	StartedAt   time.Time `json:"started_at"`
}

// WritePIDFile writes worker PID info for crash recovery
func WritePIDFile(stateRoot string, issueNumber int, info *PIDFile) error {
	pidDir := filepath.Join(stateRoot, ".ai", "state", "pids")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return fmt.Errorf("failed to create pids directory: %w", err)
	}

	pidPath := filepath.Join(pidDir, fmt.Sprintf("issue-%d.json", issueNumber))
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal PID info: %w", err)
	}

	return os.WriteFile(pidPath, data, 0644)
}

// ReadPIDFile reads worker PID info
func ReadPIDFile(stateRoot string, issueNumber int) (*PIDFile, error) {
	pidPath := filepath.Join(stateRoot, ".ai", "state", "pids", fmt.Sprintf("issue-%d.json", issueNumber))
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return nil, err
	}

	var info PIDFile
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse PID file: %w", err)
	}

	return &info, nil
}

// CleanupPIDFile removes the PID file
func CleanupPIDFile(stateRoot string, issueNumber int) error {
	pidPath := filepath.Join(stateRoot, ".ai", "state", "pids", fmt.Sprintf("issue-%d.json", issueNumber))
	err := os.Remove(pidPath)
	if os.IsNotExist(err) {
		return nil // Already cleaned up
	}
	return err
}

// IsProcessRunning checks if a process is still running
// Uses PID and start time to avoid false positives from PID reuse
func IsProcessRunning(pid int, expectedStartTime int64) bool {
	if pid <= 0 {
		return false
	}
	return isProcessRunningOS(pid, expectedStartTime)
}

// IsProcessRunningByPID checks if a process is running by PID only
// Less reliable due to PID reuse, use IsProcessRunning when possible
func IsProcessRunningByPID(pid int) bool {
	if pid <= 0 {
		return false
	}
	return isProcessRunningOS(pid, 0)
}
