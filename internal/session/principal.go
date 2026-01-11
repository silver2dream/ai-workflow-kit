package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PrincipalSession represents a Principal session
type PrincipalSession struct {
	SessionID    string    `json:"session_id"`
	StartedAt    time.Time `json:"started_at"`
	PID          int       `json:"pid"`
	PIDStartTime int64     `json:"pid_start_time"`
	EndedAt      *string   `json:"ended_at,omitempty"`
	ExitReason   string    `json:"exit_reason,omitempty"`
	Actions      []Action  `json:"actions,omitempty"`
}

// CurrentSessionInfo is the minimal info stored in session.json
type CurrentSessionInfo struct {
	SessionID    string `json:"session_id"`
	StartedAt    string `json:"started_at"`
	PID          int    `json:"pid"`
	PIDStartTime int64  `json:"pid_start_time"`
}

// InitPrincipal initializes a new Principal session
// Returns the session ID or an error if another Principal is running
// This method is safe for concurrent use.
func (m *Manager) InitPrincipal() (string, error) {
	// Lock to prevent concurrent session initialization race conditions
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure directories exist
	if err := os.MkdirAll(m.SessionsDir(), 0755); err != nil {
		return "", fmt.Errorf("failed to create sessions dir: %w", err)
	}

	currentPID := os.Getpid()
	currentStart := time.Now().Unix()

	// Check if another Principal is running
	if _, err := os.Stat(m.SessionFile()); err == nil {
		info, err := m.loadCurrentSessionInfo()
		if err == nil && info.PID != 0 {
			if IsProcessRunning(info.PID, info.PIDStartTime) {
				return "", fmt.Errorf("another Principal is already running (PID: %d, Session: %s)",
					info.PID, info.SessionID)
			}

			// Old Principal is dead, mark as interrupted if not already ended
			if info.SessionID != "" {
				sessionFile := filepath.Join(m.SessionsDir(), info.SessionID+".json")
				if _, err := os.Stat(sessionFile); err == nil {
					session, err := m.loadSessionLog(info.SessionID)
					if err == nil && session.EndedAt == nil {
						_ = m.endPrincipalLocked(info.SessionID, "interrupted")
					}
				}
			}
		}
	}

	// Generate new session
	sessionID := GenerateSessionID("principal")
	startedAt := time.Now().UTC()

	// Write current session file
	currentInfo := CurrentSessionInfo{
		SessionID:    sessionID,
		StartedAt:    startedAt.Format(time.RFC3339),
		PID:          currentPID,
		PIDStartTime: currentStart,
	}

	if err := m.writeCurrentSessionInfo(&currentInfo); err != nil {
		return "", fmt.Errorf("failed to write session file: %w", err)
	}

	// Initialize session log
	session := PrincipalSession{
		SessionID: sessionID,
		StartedAt: startedAt,
		Actions:   []Action{},
	}

	if err := m.writeSessionLog(sessionID, &session); err != nil {
		return "", fmt.Errorf("failed to write session log: %w", err)
	}

	return sessionID, nil
}

// GetCurrentSessionID returns the current Principal session ID
// This method is safe for concurrent use.
func (m *Manager) GetCurrentSessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, err := m.loadCurrentSessionInfo()
	if err != nil {
		return ""
	}
	return info.SessionID
}

// EndPrincipal ends a Principal session with a reason
// This method is safe for concurrent use.
func (m *Manager) EndPrincipal(sessionID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.endPrincipalLocked(sessionID, reason)
}

// endPrincipalLocked is the internal version that assumes the lock is already held.
// This prevents deadlock when called from InitPrincipal which already holds the lock.
func (m *Manager) endPrincipalLocked(sessionID, reason string) error {
	session, err := m.loadSessionLog(sessionID)
	if err != nil {
		return fmt.Errorf("session log not found: %w", err)
	}

	endedAt := time.Now().UTC().Format(time.RFC3339)
	session.EndedAt = &endedAt
	session.ExitReason = reason

	return m.writeSessionLog(sessionID, session)
}

// IsPrincipalRunning checks if the Principal with given PID is still running
func (m *Manager) IsPrincipalRunning(pid int, startTime int64) bool {
	return IsProcessRunning(pid, startTime)
}

// loadCurrentSessionInfo loads the current session info from session.json
func (m *Manager) loadCurrentSessionInfo() (*CurrentSessionInfo, error) {
	data, err := os.ReadFile(m.SessionFile())
	if err != nil {
		return nil, err
	}

	var info CurrentSessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// writeCurrentSessionInfo writes the current session info atomically
// Note: On Windows, os.Rename cannot overwrite existing files, so we remove first
func (m *Manager) writeCurrentSessionInfo(info *CurrentSessionInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	targetFile := m.SessionFile()
	tmpFile := targetFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	// Remove target file first for Windows compatibility
	// Check error to handle file-in-use scenarios on Windows
	if err := os.Remove(targetFile); err != nil && !os.IsNotExist(err) {
		// File exists but cannot be removed (possibly in use)
		// Clean up temp file and return error
		os.Remove(tmpFile)
		return fmt.Errorf("failed to remove existing session file: %w", err)
	}

	if err := os.Rename(tmpFile, targetFile); err != nil {
		os.Remove(tmpFile) // cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

// loadSessionLog loads a session log file
func (m *Manager) loadSessionLog(sessionID string) (*PrincipalSession, error) {
	logFile := filepath.Join(m.SessionsDir(), sessionID+".json")
	data, err := os.ReadFile(logFile)
	if err != nil {
		return nil, err
	}

	var session PrincipalSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// writeSessionLog writes a session log file atomically
// Note: On Windows, os.Rename cannot overwrite existing files, so we remove first
func (m *Manager) writeSessionLog(sessionID string, session *PrincipalSession) error {
	logFile := filepath.Join(m.SessionsDir(), sessionID+".json")

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := logFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	// Remove target file first for Windows compatibility
	// Check error to handle file-in-use scenarios on Windows
	if err := os.Remove(logFile); err != nil && !os.IsNotExist(err) {
		// File exists but cannot be removed (possibly in use)
		// Clean up temp file and return error
		os.Remove(tmpFile)
		return fmt.Errorf("failed to remove existing session log: %w", err)
	}

	if err := os.Rename(tmpFile, logFile); err != nil {
		os.Remove(tmpFile) // cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}
