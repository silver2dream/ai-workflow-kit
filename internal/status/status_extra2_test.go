package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// readSessionFile (status.go)
// ---------------------------------------------------------------------------

func TestReadSessionFile_Valid(t *testing.T) {
	dir := t.TempDir()
	session := rawSessionFile{
		SessionID: "test-session-123",
		StartedAt: "2024-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(session)
	path := filepath.Join(dir, "session.json")
	os.WriteFile(path, data, 0644)

	got, err := readSessionFile(path)
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	if got.SessionID != "test-session-123" {
		t.Errorf("SessionID = %q, want 'test-session-123'", got.SessionID)
	}
}

func TestReadSessionFile_NotFound(t *testing.T) {
	_, err := readSessionFile("/nonexistent/path/session.json")
	if err == nil {
		t.Error("readSessionFile with missing file should return error")
	}
}

func TestReadSessionFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	os.WriteFile(path, []byte("not valid json"), 0644)

	_, err := readSessionFile(path)
	if err == nil {
		t.Error("readSessionFile with invalid JSON should return error")
	}
}

// ---------------------------------------------------------------------------
// readResultFile (status.go)
// ---------------------------------------------------------------------------

func TestReadResultFile_Valid(t *testing.T) {
	dir := t.TempDir()
	result := rawResultFile{
		IssueID: "42",
		Status:  "success",
	}
	data, _ := json.Marshal(result)
	path := filepath.Join(dir, "result.json")
	os.WriteFile(path, data, 0644)

	got, err := readResultFile(path)
	if err != nil {
		t.Fatalf("readResultFile: %v", err)
	}
	if got.IssueID != "42" {
		t.Errorf("IssueID = %q, want '42'", got.IssueID)
	}
}

func TestReadResultFile_NotFound(t *testing.T) {
	_, err := readResultFile("/nonexistent/result.json")
	if err == nil {
		t.Error("readResultFile with missing file should return error")
	}
}

func TestReadResultFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	os.WriteFile(path, []byte("{invalid json"), 0644)

	_, err := readResultFile(path)
	if err == nil {
		t.Error("readResultFile with invalid JSON should return error")
	}
}

// ---------------------------------------------------------------------------
// readTraceFile (status.go)
// ---------------------------------------------------------------------------

func TestReadTraceFile_Valid(t *testing.T) {
	dir := t.TempDir()
	trace := rawTraceFile{
		IssueID: "5",
		Status:  "running",
	}
	data, _ := json.Marshal(trace)
	path := filepath.Join(dir, "trace.json")
	os.WriteFile(path, data, 0644)

	got, err := readTraceFile(path)
	if err != nil {
		t.Fatalf("readTraceFile: %v", err)
	}
	if got.IssueID != "5" {
		t.Errorf("IssueID = %q, want '5'", got.IssueID)
	}
}

func TestReadTraceFile_NotFound(t *testing.T) {
	_, err := readTraceFile("/nonexistent/trace.json")
	if err == nil {
		t.Error("readTraceFile with missing file should return error")
	}
}

func TestReadTraceFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	os.WriteFile(path, []byte("not json"), 0644)

	_, err := readTraceFile(path)
	if err == nil {
		t.Error("readTraceFile with invalid JSON should return error")
	}
}
