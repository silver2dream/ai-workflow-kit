package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if m.StateRoot != tmpDir {
		t.Errorf("StateRoot = %q, want %q", m.StateRoot, tmpDir)
	}
}

func TestManagerPaths(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if !strings.HasSuffix(m.SessionStateDir(), filepath.Join(".ai", "state", "principal")) {
		t.Errorf("SessionStateDir() = %q, unexpected path", m.SessionStateDir())
	}

	if !strings.HasSuffix(m.SessionFile(), "session.json") {
		t.Errorf("SessionFile() = %q, should end with session.json", m.SessionFile())
	}

	if !strings.HasSuffix(m.SessionsDir(), "sessions") {
		t.Errorf("SessionsDir() = %q, should end with sessions", m.SessionsDir())
	}

	if !strings.HasSuffix(m.ResultsDir(), filepath.Join(".ai", "results")) {
		t.Errorf("ResultsDir() = %q, unexpected path", m.ResultsDir())
	}
}

func TestGenerateSessionID(t *testing.T) {
	tests := []struct {
		role string
	}{
		{"principal"},
		{"worker"},
		{"reviewer"},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			id := GenerateSessionID(tt.role)

			if !strings.HasPrefix(id, tt.role+"-") {
				t.Errorf("GenerateSessionID(%q) = %q, should start with %q-", tt.role, id, tt.role)
			}

			// Should have format: role-YYYYMMDD-HHMMSS-xxxxxxxx
			parts := strings.Split(id, "-")
			if len(parts) != 4 {
				t.Errorf("GenerateSessionID(%q) = %q, expected 4 parts separated by -", tt.role, id)
			}

			// Verify uniqueness
			id2 := GenerateSessionID(tt.role)
			if id == id2 {
				t.Errorf("GenerateSessionID() should generate unique IDs")
			}
		})
	}
}

func TestInitPrincipal(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal() error = %v", err)
	}

	if !strings.HasPrefix(sessionID, "principal-") {
		t.Errorf("sessionID = %q, should start with principal-", sessionID)
	}

	// Verify session file was created
	if _, err := os.Stat(m.SessionFile()); os.IsNotExist(err) {
		t.Error("session.json should be created")
	}

	// Verify session log was created
	logFile := filepath.Join(m.SessionsDir(), sessionID+".json")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("session log file should be created")
	}
}

func TestGetCurrentSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// No session yet
	if id := m.GetCurrentSessionID(); id != "" {
		t.Errorf("GetCurrentSessionID() = %q, want empty string", id)
	}

	// After init
	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal() error = %v", err)
	}

	if got := m.GetCurrentSessionID(); got != sessionID {
		t.Errorf("GetCurrentSessionID() = %q, want %q", got, sessionID)
	}
}

func TestEndPrincipal(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal() error = %v", err)
	}

	if err := m.EndPrincipal(sessionID, "completed"); err != nil {
		t.Fatalf("EndPrincipal() error = %v", err)
	}

	// Verify session was ended
	session, err := m.loadSessionLog(sessionID)
	if err != nil {
		t.Fatalf("loadSessionLog() error = %v", err)
	}

	if session.EndedAt == nil {
		t.Error("EndedAt should be set")
	}
	if session.ExitReason != "completed" {
		t.Errorf("ExitReason = %q, want %q", session.ExitReason, "completed")
	}
}

func TestEndPrincipalNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Ensure sessions dir exists
	os.MkdirAll(m.SessionsDir(), 0755)

	err := m.EndPrincipal("nonexistent-session", "test")
	if err == nil {
		t.Error("EndPrincipal() expected error for nonexistent session")
	}
}

func TestAppendAction(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal() error = %v", err)
	}

	// Append action with struct data
	err = m.AppendAction(sessionID, "test_action", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("AppendAction() error = %v", err)
	}

	// Append action with string data
	err = m.AppendAction(sessionID, "string_action", "simple string")
	if err != nil {
		t.Fatalf("AppendAction() with string error = %v", err)
	}

	// Append action with JSON string
	err = m.AppendAction(sessionID, "json_action", `{"nested": "json"}`)
	if err != nil {
		t.Fatalf("AppendAction() with JSON string error = %v", err)
	}

	// Verify actions were recorded
	session, err := m.loadSessionLog(sessionID)
	if err != nil {
		t.Fatalf("loadSessionLog() error = %v", err)
	}

	if len(session.Actions) != 3 {
		t.Errorf("len(Actions) = %d, want 3", len(session.Actions))
	}

	if session.Actions[0].Type != "test_action" {
		t.Errorf("Actions[0].Type = %q, want %q", session.Actions[0].Type, "test_action")
	}
}

func TestAppendActionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Ensure sessions dir exists
	os.MkdirAll(m.SessionsDir(), 0755)

	err := m.AppendAction("nonexistent", "test", nil)
	if err == nil {
		t.Error("AppendAction() expected error for nonexistent session")
	}
}

func TestRecordWorkerDispatched(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	sessionID, _ := m.InitPrincipal()

	err := m.RecordWorkerDispatched(sessionID, "123", "backend")
	if err != nil {
		t.Fatalf("RecordWorkerDispatched() error = %v", err)
	}

	session, _ := m.loadSessionLog(sessionID)
	if len(session.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(session.Actions))
	}

	if session.Actions[0].Type != "worker_dispatched" {
		t.Errorf("action type = %q, want worker_dispatched", session.Actions[0].Type)
	}
}

func TestRecordWorkerCompleted(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	sessionID, _ := m.InitPrincipal()

	err := m.RecordWorkerCompleted(sessionID, "123", "worker-123", "success", "https://github.com/test/pr/1")
	if err != nil {
		t.Fatalf("RecordWorkerCompleted() error = %v", err)
	}

	session, _ := m.loadSessionLog(sessionID)
	if session.Actions[0].Type != "worker_completed" {
		t.Errorf("action type = %q, want worker_completed", session.Actions[0].Type)
	}
}

func TestRecordWorkerFailed(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	sessionID, _ := m.InitPrincipal()

	err := m.RecordWorkerFailed(sessionID, "123", 3)
	if err != nil {
		t.Fatalf("RecordWorkerFailed() error = %v", err)
	}

	session, _ := m.loadSessionLog(sessionID)
	if session.Actions[0].Type != "worker_failed" {
		t.Errorf("action type = %q, want worker_failed", session.Actions[0].Type)
	}
}

func TestUpdateResultWithPrincipalSession(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Create results dir and a result file
	resultsDir := m.ResultsDir()
	os.MkdirAll(resultsDir, 0755)

	resultFile := filepath.Join(resultsDir, "issue-123.json")
	initialResult := map[string]interface{}{
		"issue_id": "123",
		"status":   "success",
	}
	data, _ := json.Marshal(initialResult)
	os.WriteFile(resultFile, data, 0644)

	// Update with principal session
	err := m.UpdateResultWithPrincipalSession("123", "principal-test-123")
	if err != nil {
		t.Fatalf("UpdateResultWithPrincipalSession() error = %v", err)
	}

	// Verify update
	updatedData, _ := os.ReadFile(resultFile)
	var result map[string]interface{}
	json.Unmarshal(updatedData, &result)

	session, ok := result["session"].(map[string]interface{})
	if !ok {
		t.Fatal("session field not found in result")
	}

	if session["principal_session_id"] != "principal-test-123" {
		t.Errorf("principal_session_id = %v, want principal-test-123", session["principal_session_id"])
	}
}

func TestUpdateResultWithPrincipalSessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	os.MkdirAll(m.ResultsDir(), 0755)

	err := m.UpdateResultWithPrincipalSession("nonexistent", "principal-123")
	if err == nil {
		t.Error("expected error for nonexistent result file")
	}
}

func TestUpdateResultWithReviewAudit(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Create results dir and a result file
	resultsDir := m.ResultsDir()
	os.MkdirAll(resultsDir, 0755)

	resultFile := filepath.Join(resultsDir, "issue-456.json")
	initialResult := map[string]interface{}{
		"issue_id": "456",
		"status":   "success",
	}
	data, _ := json.Marshal(initialResult)
	os.WriteFile(resultFile, data, 0644)

	// Update with review audit
	audit := &ReviewAudit{
		ReviewerSessionID: "reviewer-test-456",
		CIStatus:          "passed",
		CITimeout:         false,
		Decision:          "approved",
	}

	err := m.UpdateResultWithReviewAudit("456", audit)
	if err != nil {
		t.Fatalf("UpdateResultWithReviewAudit() error = %v", err)
	}

	// Verify update
	updatedData, _ := os.ReadFile(resultFile)
	var result map[string]interface{}
	json.Unmarshal(updatedData, &result)

	reviewAudit, ok := result["review_audit"].(map[string]interface{})
	if !ok {
		t.Fatal("review_audit field not found in result")
	}

	if reviewAudit["decision"] != "approved" {
		t.Errorf("decision = %v, want approved", reviewAudit["decision"])
	}
	if reviewAudit["ci_status"] != "passed" {
		t.Errorf("ci_status = %v, want passed", reviewAudit["ci_status"])
	}
}

func TestCurrentSessionInfo(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Ensure directory exists
	os.MkdirAll(m.SessionStateDir(), 0755)

	info := &CurrentSessionInfo{
		SessionID:    "test-session-123",
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		PID:          12345,
		PIDStartTime: time.Now().Unix(),
	}

	// Write
	err := m.writeCurrentSessionInfo(info)
	if err != nil {
		t.Fatalf("writeCurrentSessionInfo() error = %v", err)
	}

	// Read back
	loaded, err := m.loadCurrentSessionInfo()
	if err != nil {
		t.Fatalf("loadCurrentSessionInfo() error = %v", err)
	}

	if loaded.SessionID != info.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, info.SessionID)
	}
	if loaded.PID != info.PID {
		t.Errorf("PID = %d, want %d", loaded.PID, info.PID)
	}
}
