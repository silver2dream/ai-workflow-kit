package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ReadSessionFiltered error path (reader.go)
// ---------------------------------------------------------------------------

func TestReadSessionFiltered_SessionNotFound(t *testing.T) {
	dir := t.TempDir()
	reader := NewEventReader(dir)

	_, err := reader.ReadSessionFiltered("nonexistent-session", EventFilter{})
	if err == nil {
		t.Error("ReadSessionFiltered with missing session should return error")
	}
}

func TestReadSessionFiltered_WithEvents(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, ".ai", "state", "events")
	os.MkdirAll(eventsDir, 0755)

	// Write a session file
	event := Event{
		Timestamp: time.Now(),
		Level:     "info",
		Type:      "step",
		Component: "principal",
	}
	data, _ := json.Marshal(event)
	os.WriteFile(filepath.Join(eventsDir, "test-session.jsonl"), append(data, '\n'), 0644)

	reader := NewEventReader(dir)
	events, err := reader.ReadSessionFiltered("test-session", EventFilter{Level: "info"})
	if err != nil {
		t.Fatalf("ReadSessionFiltered: %v", err)
	}
	if len(events) == 0 {
		t.Error("ReadSessionFiltered should return filtered events")
	}
}

// ---------------------------------------------------------------------------
// ReadCurrentSessionFiltered error path (reader.go)
// ---------------------------------------------------------------------------

func TestReadCurrentSessionFiltered_NoSessionFile(t *testing.T) {
	dir := t.TempDir()
	reader := NewEventReader(dir)

	// No session file — should return error
	_, err := reader.ReadCurrentSessionFiltered(EventFilter{})
	if err == nil {
		t.Error("ReadCurrentSessionFiltered with no session file should return error")
	}
}

// ---------------------------------------------------------------------------
// ListSessions error paths (reader.go)
// ---------------------------------------------------------------------------

func TestListSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, ".ai", "state", "events")
	os.MkdirAll(eventsDir, 0755)

	reader := NewEventReader(dir)
	sessions, err := reader.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions on empty dir: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListSessions on empty dir = %v, want empty", sessions)
	}
}

func TestListSessions_NonJSONLFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, ".ai", "state", "events")
	os.MkdirAll(eventsDir, 0755)

	// Write non-jsonl files
	os.WriteFile(filepath.Join(eventsDir, "session.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(eventsDir, "session.txt"), []byte("text"), 0644)
	// Write valid jsonl file
	os.WriteFile(filepath.Join(eventsDir, "session-abc.jsonl"), []byte(""), 0644)

	reader := NewEventReader(dir)
	sessions, err := reader.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("ListSessions = %v (len=%d), want 1 session (only .jsonl)", sessions, len(sessions))
	}
	if sessions[0] != "session-abc" {
		t.Errorf("sessions[0] = %q, want 'session-abc'", sessions[0])
	}
}

// ---------------------------------------------------------------------------
// ReadByIssue (reader.go) — additional coverage
// ---------------------------------------------------------------------------

func TestReadByIssue_WithMatchingEvent(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, ".ai", "state", "events")
	os.MkdirAll(eventsDir, 0755)

	event := Event{
		Timestamp: time.Now(),
		Level:     "info",
		Type:      "step",
		Component: "worker",
		IssueID:   42,
	}
	data, _ := json.Marshal(event)
	os.WriteFile(filepath.Join(eventsDir, "test-session-2.jsonl"), append(data, '\n'), 0644)

	reader := NewEventReader(dir)
	events, err := reader.ReadByIssue(42)
	if err != nil {
		t.Fatalf("ReadByIssue: %v", err)
	}
	if len(events) == 0 {
		t.Error("ReadByIssue should find matching event")
	}
}

// ---------------------------------------------------------------------------
// readEventsFromFile — invalid JSON line ignored (reader.go)
// ---------------------------------------------------------------------------

func TestReadEventsFromFile_InvalidJSONLine_Ignored(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, ".ai", "state", "events")
	os.MkdirAll(eventsDir, 0755)

	// Write one valid and one invalid JSON line
	validEvent := Event{
		Timestamp: time.Now(),
		Level:     "info",
		Type:      "step",
		Component: "principal",
	}
	validData, _ := json.Marshal(validEvent)
	content := string(validData) + "\n" + "invalid json line\n"
	os.WriteFile(filepath.Join(eventsDir, "test-session-3.jsonl"), []byte(content), 0644)

	reader := NewEventReader(dir)
	events, err := reader.ReadSession("test-session-3")
	if err != nil {
		t.Fatalf("ReadSession with partially invalid file: %v", err)
	}
	// Should have 1 valid event (invalid line skipped)
	if len(events) != 1 {
		t.Errorf("ReadSession should return 1 valid event, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Close — nil file (writer.go)
// ---------------------------------------------------------------------------

func TestEventWriter_Close_NilFile(t *testing.T) {
	// EventWriter with nil file should not panic on Close
	w := &EventWriter{}
	err := w.Close()
	if err != nil {
		t.Errorf("Close on nil file = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// WriteDecisionEvent — global writer active (writer.go)
// ---------------------------------------------------------------------------

func TestWriteDecisionEvent_WithGlobalWriter(t *testing.T) {
	dir := t.TempDir()

	if err := InitGlobalWriter(dir, "test-decision-session"); err != nil {
		t.Fatalf("InitGlobalWriter: %v", err)
	}
	defer CloseGlobalWriter()

	decision := Decision{
		Rule:   "check_tests",
		Result: "pass",
	}

	// Should not panic with global writer initialized
	WriteDecisionEvent("principal", "decision_made", decision)
}

// ---------------------------------------------------------------------------
// InitGlobalWriter — reinit with existing writer (writer.go)
// ---------------------------------------------------------------------------

func TestInitGlobalWriter_Reinit(t *testing.T) {
	dir := t.TempDir()

	// Initialize first time
	if err := InitGlobalWriter(dir, "session-1"); err != nil {
		t.Fatalf("first InitGlobalWriter: %v", err)
	}

	// Initialize again (should close existing and create new)
	if err := InitGlobalWriter(dir, "session-2"); err != nil {
		t.Fatalf("second InitGlobalWriter: %v", err)
	}

	defer CloseGlobalWriter()
}
