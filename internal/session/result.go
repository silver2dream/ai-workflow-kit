package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ReviewAudit represents review audit information
type ReviewAudit struct {
	ReviewerSessionID string `json:"reviewer_session_id"`
	ReviewTimestamp   string `json:"review_timestamp"`
	CIStatus          string `json:"ci_status"`
	CITimeout         bool   `json:"ci_timeout"`
	Decision          string `json:"decision"`
	MergeTimestamp    string `json:"merge_timestamp,omitempty"`
}

// ResultSession represents session info in result file
type ResultSession struct {
	PrincipalSessionID string `json:"principal_session_id,omitempty"`
	WorkerSessionID    string `json:"worker_session_id,omitempty"`
	WorkerPID          int    `json:"worker_pid,omitempty"`
	WorkerStartTime    int64  `json:"worker_start_time,omitempty"`
}

// UpdateResultWithPrincipalSession updates the result file with Principal session ID
func (m *Manager) UpdateResultWithPrincipalSession(issueID, principalSessionID string) error {
	resultFile := filepath.Join(m.ResultsDir(), fmt.Sprintf("issue-%s.json", issueID))

	if _, err := os.Stat(resultFile); os.IsNotExist(err) {
		return fmt.Errorf("result file not found: %s", resultFile)
	}

	// Read existing result
	data, err := os.ReadFile(resultFile)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}

	// Update session info
	session, ok := result["session"].(map[string]interface{})
	if !ok {
		session = make(map[string]interface{})
	}
	session["principal_session_id"] = principalSessionID
	result["session"] = session

	// Write back atomically
	newData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := resultFile + ".tmp"
	if err := os.WriteFile(tmpFile, newData, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, resultFile)
}

// UpdateResultWithReviewAudit updates the result file with review audit information
func (m *Manager) UpdateResultWithReviewAudit(issueID string, audit *ReviewAudit) error {
	resultFile := filepath.Join(m.ResultsDir(), fmt.Sprintf("issue-%s.json", issueID))

	if _, err := os.Stat(resultFile); os.IsNotExist(err) {
		return fmt.Errorf("result file not found: %s", resultFile)
	}

	// Read existing result
	data, err := os.ReadFile(resultFile)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}

	// Set review timestamp if not already set
	if audit.ReviewTimestamp == "" {
		audit.ReviewTimestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Update review audit
	result["review_audit"] = map[string]interface{}{
		"reviewer_session_id": audit.ReviewerSessionID,
		"review_timestamp":    audit.ReviewTimestamp,
		"ci_status":           audit.CIStatus,
		"ci_timeout":          audit.CITimeout,
		"decision":            audit.Decision,
		"merge_timestamp":     audit.MergeTimestamp,
	}

	// Write back atomically
	newData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := resultFile + ".tmp"
	if err := os.WriteFile(tmpFile, newData, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, resultFile)
}
