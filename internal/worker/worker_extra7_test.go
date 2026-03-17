package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WriteFileAtomic (result.go) — additional coverage
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.json")
	data := []byte(`{"key":"value"}`)

	if err := WriteFileAtomic(path, data, 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}

func TestWriteFileAtomic_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")

	// Write initial content
	os.WriteFile(path, []byte("old content"), 0644)

	// Overwrite
	newData := []byte("new content")
	if err := WriteFileAtomic(path, newData, 0644); err != nil {
		t.Fatalf("WriteFileAtomic overwrite: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new content" {
		t.Errorf("content = %q, want 'new content'", got)
	}

	// Ensure .bak file is cleaned up
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error(".bak file should be removed after successful write")
	}
}

func TestWriteFileAtomic_EmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	if err := WriteFileAtomic(path, []byte{}, 0644); err != nil {
		t.Fatalf("WriteFileAtomic empty: %v", err)
	}

	got, _ := os.ReadFile(path)
	if len(got) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(got))
	}
}

// ---------------------------------------------------------------------------
// WriteResultAtomic (result.go)
// ---------------------------------------------------------------------------

func TestWriteResultAtomic_Success(t *testing.T) {
	dir := t.TempDir()
	result := &IssueResult{
		IssueID: "42",
		Status:  "success",
		Branch:  "feat/ai-issue-42",
		Session: SessionInfo{
			WorkerSessionID: "session-xyz",
		},
	}

	if err := WriteResultAtomic(dir, 42, result); err != nil {
		t.Fatalf("WriteResultAtomic: %v", err)
	}

	// Verify file was created
	resultPath := filepath.Join(dir, ".ai", "results", "issue-42.json")
	if _, err := os.Stat(resultPath); err != nil {
		t.Fatalf("result file not created: %v", err)
	}

	// Verify content
	data, _ := os.ReadFile(resultPath)
	var loaded IssueResult
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if loaded.IssueID != "42" {
		t.Errorf("IssueID = %q, want '42'", loaded.IssueID)
	}
}

// ---------------------------------------------------------------------------
// ResetFailCount (result.go)
// ---------------------------------------------------------------------------

func TestResetFailCount_WithExistingFile(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, ".ai", "runs", "issue-7")
	os.MkdirAll(runDir, 0755)
	failCountPath := filepath.Join(runDir, "fail_count.txt")
	os.WriteFile(failCountPath, []byte("3"), 0644)

	if err := ResetFailCount(dir, 7); err != nil {
		t.Fatalf("ResetFailCount: %v", err)
	}

	if _, err := os.Stat(failCountPath); !os.IsNotExist(err) {
		t.Error("fail_count.txt should be removed after reset")
	}
}

func TestResetFailCount_NoFilePublic(t *testing.T) {
	dir := t.TempDir()
	// No fail count file — should return nil (not error)
	if err := ResetFailCount(dir, 99); err != nil {
		t.Errorf("ResetFailCount with no file should return nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetStartedAtTime (result.go)
// ---------------------------------------------------------------------------

func TestGetStartedAtTime_ValidRFC3339(t *testing.T) {
	trace := &ExecutionTrace{
		StartedAt: "2024-03-15T10:30:00Z",
	}
	tm, err := trace.GetStartedAtTime()
	if err != nil {
		t.Fatalf("GetStartedAtTime: %v", err)
	}
	if tm.Year() != 2024 || tm.Month() != 3 || tm.Day() != 15 {
		t.Errorf("time = %v, want 2024-03-15", tm)
	}
}

func TestGetStartedAtTime_EmptyString(t *testing.T) {
	trace := &ExecutionTrace{StartedAt: ""}
	_, err := trace.GetStartedAtTime()
	if err == nil {
		t.Error("GetStartedAtTime with empty string should return error")
	}
}

// ---------------------------------------------------------------------------
// BuildCommitMessage (commit.go)
// ---------------------------------------------------------------------------

func TestBuildCommitMessage_EmptyInput(t *testing.T) {
	got := BuildCommitMessage("")
	if got != "[chore] issue" {
		t.Errorf("BuildCommitMessage('') = %q, want '[chore] issue'", got)
	}
}

func TestBuildCommitMessage_WithPrefix(t *testing.T) {
	got := BuildCommitMessage("[feat] add new feature")
	if got != "[feat] add new feature" {
		t.Errorf("BuildCommitMessage('[feat] ...') = %q", got)
	}
}

func TestBuildCommitMessage_WithoutPrefix(t *testing.T) {
	got := BuildCommitMessage("implement the feature")
	if !strings.HasPrefix(got, "[chore]") {
		t.Errorf("BuildCommitMessage without prefix = %q, should start with [chore]", got)
	}
}

func TestBuildCommitMessage_SpecialChars(t *testing.T) {
	got := BuildCommitMessage("[fix] handle UTF-8 characters: émojis 🎉")
	// Special characters should be replaced with spaces or removed
	if strings.Contains(got, "🎉") {
		t.Errorf("BuildCommitMessage should remove emojis, got: %q", got)
	}
}

func TestBuildCommitMessage_OnlySpecialChars(t *testing.T) {
	got := BuildCommitMessage("[feat] !!!@@@###")
	// After normalizing special chars, subject should fall back to "issue"
	if got == "" {
		t.Error("BuildCommitMessage should not return empty string")
	}
}

// ---------------------------------------------------------------------------
// NewTraceRecorder uncovered path (trace.go)
// ---------------------------------------------------------------------------

func TestNewTraceRecorder_WithPID(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	rec, err := NewTraceRecorder(dir, 200, "owner/repo", "feat/main", "main", 12345, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder with PID: %v", err)
	}
	if rec == nil {
		t.Fatal("NewTraceRecorder returned nil")
	}
	if rec.trace.WorkerPID != 12345 {
		t.Errorf("WorkerPID = %d, want 12345", rec.trace.WorkerPID)
	}
}

func TestNewTraceRecorder_CreatesTraceFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	rec, err := NewTraceRecorder(dir, 300, "repo", "branch", "main", 0, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	// Verify the trace file was created
	if _, err := os.Stat(rec.path); err != nil {
		t.Errorf("trace file not created at %s: %v", rec.path, err)
	}
}

// ---------------------------------------------------------------------------
// ReadConsecutiveFailures / ResetConsecutiveFailures (result.go)
// ---------------------------------------------------------------------------

func TestReadConsecutiveFailures_NoFile(t *testing.T) {
	dir := t.TempDir()
	got := ReadConsecutiveFailures(dir)
	if got != 0 {
		t.Errorf("ReadConsecutiveFailures with no file = %d, want 0", got)
	}
}

func TestReadConsecutiveFailures_WithFile(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("5"), 0644)

	got := ReadConsecutiveFailures(dir)
	if got != 5 {
		t.Errorf("ReadConsecutiveFailures = %d, want 5", got)
	}
}

func TestResetConsecutiveFailures_WritesZeroV2(t *testing.T) {
	dir := t.TempDir()
	if err := ResetConsecutiveFailures(dir); err != nil {
		t.Fatalf("ResetConsecutiveFailures: %v", err)
	}

	got := ReadConsecutiveFailures(dir)
	if got != 0 {
		t.Errorf("after reset, ReadConsecutiveFailures = %d, want 0", got)
	}
}
