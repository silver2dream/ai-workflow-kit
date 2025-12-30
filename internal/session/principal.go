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
func (m *Manager) InitPrincipal() (string, error) {
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
						_ = m.EndPrincipal(info.SessionID, "interrupted")
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
func (m *Manager) GetCurrentSessionID() string {
	info, err := m.loadCurrentSessionInfo()
	if err != nil {
		return ""
	}
	return info.SessionID
}

// EndPrincipal ends a Principal session with a reason
func (m *Manager) EndPrincipal(sessionID, reason string) error {
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
func (m *Manager) writeCurrentSessionInfo(info *CurrentSessionInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := m.SessionFile() + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, m.SessionFile())
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

	return os.Rename(tmpFile, logFile)
}
