package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// itoa (pid_check_unix.go) — only compiled on non-windows
// ---------------------------------------------------------------------------

func TestItoa_Zero(t *testing.T) {
	if itoa(0) != "0" {
		t.Errorf("itoa(0) = %q, want '0'", itoa(0))
	}
}

func TestItoa_Positive(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "1"},
		{42, "42"},
		{12345, "12345"},
	}
	for _, tc := range cases {
		got := itoa(tc.n)
		if got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestItoa_Negative(t *testing.T) {
	got := itoa(-7)
	if got != "-7" {
		t.Errorf("itoa(-7) = %q, want '-7'", got)
	}
}

// ---------------------------------------------------------------------------
// writeCurrentSessionInfo / loadCurrentSessionInfo (principal.go)
// ---------------------------------------------------------------------------

func TestWriteCurrentSessionInfo_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.SessionStateDir(), 0755)

	info := &CurrentSessionInfo{
		SessionID:    "principal-20240101-120000-abcd1234",
		StartedAt:    "2024-01-01T12:00:00Z",
		PID:          os.Getpid(),
		PIDStartTime: 1234567890,
	}

	if err := m.writeCurrentSessionInfo(info); err != nil {
		t.Fatalf("writeCurrentSessionInfo: %v", err)
	}

	loaded, err := m.loadCurrentSessionInfo()
	if err != nil {
		t.Fatalf("loadCurrentSessionInfo: %v", err)
	}

	if loaded.SessionID != info.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, info.SessionID)
	}
	if loaded.PID != info.PID {
		t.Errorf("PID = %d, want %d", loaded.PID, info.PID)
	}
}

func TestLoadCurrentSessionInfo_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	// No session file
	_, err := m.loadCurrentSessionInfo()
	if err == nil {
		t.Error("loadCurrentSessionInfo with no file should return error")
	}
}

func TestLoadCurrentSessionInfo_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.SessionStateDir(), 0755)
	os.WriteFile(m.SessionFile(), []byte("not json"), 0644)

	_, err := m.loadCurrentSessionInfo()
	if err == nil {
		t.Error("loadCurrentSessionInfo with invalid JSON should return error")
	}
}

// ---------------------------------------------------------------------------
// writeSessionLog / loadSessionLog (principal.go)
// ---------------------------------------------------------------------------

func TestWriteSessionLog_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.SessionsDir(), 0755)

	sessionID := "principal-20240101-120000-abcd"
	session := &PrincipalSession{
		SessionID: sessionID,
	}

	if err := m.writeSessionLog(sessionID, session); err != nil {
		t.Fatalf("writeSessionLog: %v", err)
	}

	loaded, err := m.loadSessionLog(sessionID)
	if err != nil {
		t.Fatalf("loadSessionLog: %v", err)
	}
	if loaded.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, sessionID)
	}
}

func TestLoadSessionLog_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.SessionsDir(), 0755)

	_, err := m.loadSessionLog("nonexistent-session")
	if err == nil {
		t.Error("loadSessionLog for nonexistent session should return error")
	}
}

// ---------------------------------------------------------------------------
// AppendAction with various data types (action.go)
// ---------------------------------------------------------------------------

func TestAppendAction_WithStringData(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal: %v", err)
	}

	// String data (non-JSON)
	err = m.AppendAction(sessionID, "test_action", "plain string data")
	if err != nil {
		t.Fatalf("AppendAction with string: %v", err)
	}

	session, err := m.loadSessionLog(sessionID)
	if err != nil {
		t.Fatalf("loadSessionLog: %v", err)
	}
	if len(session.Actions) != 1 {
		t.Errorf("Actions count = %d, want 1", len(session.Actions))
	}
}

func TestAppendAction_WithJSONStringData(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal: %v", err)
	}

	// String data that IS valid JSON
	err = m.AppendAction(sessionID, "test_action", `{"key":"value"}`)
	if err != nil {
		t.Fatalf("AppendAction with JSON string: %v", err)
	}
}

func TestAppendAction_WithRawMessageData(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	sessionID, err := m.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal: %v", err)
	}

	raw := json.RawMessage(`{"foo":"bar"}`)
	err = m.AppendAction(sessionID, "test_action", raw)
	if err != nil {
		t.Fatalf("AppendAction with RawMessage: %v", err)
	}
}

func TestAppendAction_NotFoundSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.SessionsDir(), 0755)

	err := m.AppendAction("nonexistent-session", "test", "data")
	if err == nil {
		t.Error("AppendAction for nonexistent session should return error")
	}
}

// ---------------------------------------------------------------------------
// UpdateResultWithPrincipalSession (result.go)
// ---------------------------------------------------------------------------

func TestUpdateResultWithPrincipalSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.ResultsDir(), 0755)

	err := m.UpdateResultWithPrincipalSession("999", "session-abc")
	if err == nil {
		t.Error("UpdateResultWithPrincipalSession for missing issue should return error")
	}
}

func TestUpdateResultWithPrincipalSession_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.ResultsDir(), 0755)

	resultFile := filepath.Join(m.ResultsDir(), "issue-42.json")
	os.WriteFile(resultFile, []byte(`{"issue_id": 42}`), 0644)

	err := m.UpdateResultWithPrincipalSession("42", "principal-session-abc")
	if err != nil {
		t.Fatalf("UpdateResultWithPrincipalSession: %v", err)
	}

	data, _ := os.ReadFile(resultFile)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	sessionInfo, ok := result["session"].(map[string]interface{})
	if !ok {
		t.Fatal("result should have session field")
	}
	if sessionInfo["principal_session_id"] != "principal-session-abc" {
		t.Errorf("principal_session_id = %v, want 'principal-session-abc'", sessionInfo["principal_session_id"])
	}
}

// ---------------------------------------------------------------------------
// UpdateResultWithReviewAudit (result.go)
// ---------------------------------------------------------------------------

func TestUpdateResultWithReviewAudit_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.ResultsDir(), 0755)

	audit := &ReviewAudit{Decision: "approve"}
	err := m.UpdateResultWithReviewAudit("999", audit)
	if err == nil {
		t.Error("UpdateResultWithReviewAudit for missing issue should return error")
	}
}

func TestUpdateResultWithReviewAudit_SetsTimestamp(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	os.MkdirAll(m.ResultsDir(), 0755)

	resultFile := filepath.Join(m.ResultsDir(), "issue-7.json")
	os.WriteFile(resultFile, []byte(`{"issue_id": 7}`), 0644)

	audit := &ReviewAudit{
		ReviewerSessionID: "reviewer-session-1",
		Decision:          "approve",
		// No ReviewTimestamp — should be auto-set
	}
	err := m.UpdateResultWithReviewAudit("7", audit)
	if err != nil {
		t.Fatalf("UpdateResultWithReviewAudit: %v", err)
	}

	data, _ := os.ReadFile(resultFile)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	reviewAudit, ok := result["review_audit"].(map[string]interface{})
	if !ok {
		t.Fatal("result should have review_audit field")
	}
	if reviewAudit["review_timestamp"] == "" || reviewAudit["review_timestamp"] == nil {
		t.Error("review_timestamp should be auto-set")
	}
	if reviewAudit["decision"] != "approve" {
		t.Errorf("decision = %v, want 'approve'", reviewAudit["decision"])
	}
}
