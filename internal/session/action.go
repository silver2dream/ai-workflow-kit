package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Action represents a session action
type Action struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// AppendAction appends an action to a session log.
// This method is safe for concurrent use.
func (m *Manager) AppendAction(sessionID, actionType string, data interface{}) error {
	// Lock to prevent concurrent read-modify-write race conditions
	m.mu.Lock()
	defer m.mu.Unlock()

	logFile := filepath.Join(m.SessionsDir(), sessionID+".json")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return fmt.Errorf("session log not found: %s", sessionID)
	}

	session, err := m.loadSessionLog(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session log: %w", err)
	}

	// Marshal action data
	var rawData json.RawMessage
	switch v := data.(type) {
	case string:
		// Try to parse as JSON first
		if json.Valid([]byte(v)) {
			rawData = json.RawMessage(v)
		} else {
			// Wrap as raw string
			rawData, _ = json.Marshal(map[string]string{"_raw": v})
		}
	case json.RawMessage:
		rawData = v
	default:
		rawData, err = json.Marshal(data)
		if err != nil {
			rawData, _ = json.Marshal(map[string]string{"_raw": fmt.Sprintf("%v", data)})
		}
	}

	action := Action{
		Type:      actionType,
		Timestamp: time.Now().UTC(),
		Data:      rawData,
	}

	session.Actions = append(session.Actions, action)

	return m.writeSessionLog(sessionID, session)
}

// WorkerDispatchedData represents data for worker_dispatched action
type WorkerDispatchedData struct {
	IssueID string `json:"issue_id"`
	Repo    string `json:"repo"`
}

// WorkerCompletedData represents data for worker_completed action
type WorkerCompletedData struct {
	IssueID         string `json:"issue_id"`
	WorkerSessionID string `json:"worker_session_id"`
	Status          string `json:"status"`
	PRURL           string `json:"pr_url,omitempty"`
}

// WorkerFailedData represents data for worker_failed action
type WorkerFailedData struct {
	IssueID  string `json:"issue_id"`
	Attempts int    `json:"attempts"`
}

// RecordWorkerDispatched records a worker dispatch action
func (m *Manager) RecordWorkerDispatched(sessionID string, issueID, repo string) error {
	data := WorkerDispatchedData{
		IssueID: issueID,
		Repo:    repo,
	}
	return m.AppendAction(sessionID, "worker_dispatched", data)
}

// RecordWorkerCompleted records a worker completion action
func (m *Manager) RecordWorkerCompleted(sessionID, issueID, workerSessionID, status, prURL string) error {
	data := WorkerCompletedData{
		IssueID:         issueID,
		WorkerSessionID: workerSessionID,
		Status:          status,
		PRURL:           prURL,
	}
	return m.AppendAction(sessionID, "worker_completed", data)
}

// RecordWorkerFailed records a worker failure action
func (m *Manager) RecordWorkerFailed(sessionID, issueID string, attempts int) error {
	data := WorkerFailedData{
		IssueID:  issueID,
		Attempts: attempts,
	}
	return m.AppendAction(sessionID, "worker_failed", data)
}
