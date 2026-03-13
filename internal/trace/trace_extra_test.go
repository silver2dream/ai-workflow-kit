package trace

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// EventWriter: SessionID, FilePath, Seq
// ---------------------------------------------------------------------------

func TestEventWriter_SessionID(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "my-session-id")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	defer w.Close()

	if got := w.SessionID(); got != "my-session-id" {
		t.Errorf("SessionID() = %q, want my-session-id", got)
	}
}

func TestEventWriter_FilePath_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "sess-abc")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	defer w.Close()

	fp := w.FilePath()
	if fp == "" {
		t.Error("FilePath() should return non-empty path")
	}
}

func TestEventWriter_FilePath_NilFile(t *testing.T) {
	// An EventWriter with nil file should return empty FilePath
	w := &EventWriter{sessionID: "test", file: nil}
	if got := w.FilePath(); got != "" {
		t.Errorf("FilePath() with nil file = %q, want empty", got)
	}
}

func TestEventWriter_Seq_StartsAtZero(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "seq-test")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	defer w.Close()

	if got := w.Seq(); got != 0 {
		t.Errorf("Seq() before any writes = %d, want 0", got)
	}
}

func TestEventWriter_Seq_IncrementsOnWrite(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "seq-test2")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	defer w.Close()

	if err := w.Write("principal", "test", "info"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := w.Seq(); got != 1 {
		t.Errorf("Seq() after 1 write = %d, want 1", got)
	}

	if err := w.Write("principal", "test2", "info"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := w.Seq(); got != 2 {
		t.Errorf("Seq() after 2 writes = %d, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// InitGlobalWriter / CloseGlobalWriter / GetGlobalWriter / WriteEvent
// ---------------------------------------------------------------------------

func TestInitGlobalWriter_CreatesWriter(t *testing.T) {
	dir := t.TempDir()
	// Ensure global writer is clean at start
	CloseGlobalWriter()

	if err := InitGlobalWriter(dir, "global-sess"); err != nil {
		t.Fatalf("InitGlobalWriter: %v", err)
	}
	defer CloseGlobalWriter()

	w := GetGlobalWriter()
	if w == nil {
		t.Error("GetGlobalWriter() should return non-nil after init")
	}
}

func TestCloseGlobalWriter_NoWriterIsNoop(t *testing.T) {
	// Close even if no writer — should not error
	_ = CloseGlobalWriter()
	if err := CloseGlobalWriter(); err != nil {
		t.Errorf("CloseGlobalWriter() with no writer: %v", err)
	}
}

func TestGetGlobalWriter_NilBeforeInit(t *testing.T) {
	CloseGlobalWriter()
	if w := GetGlobalWriter(); w != nil {
		w.Close()
		t.Error("GetGlobalWriter() should return nil before init")
	}
}

func TestWriteEvent_NoGlobalWriter_NoError(t *testing.T) {
	CloseGlobalWriter()
	// Should be a no-op, not panic
	WriteEvent("principal", "test", "info")
}

func TestWriteEvent_WithGlobalWriter(t *testing.T) {
	dir := t.TempDir()
	CloseGlobalWriter()

	if err := InitGlobalWriter(dir, "global-ev-test"); err != nil {
		t.Fatalf("InitGlobalWriter: %v", err)
	}
	defer CloseGlobalWriter()

	WriteEvent("worker", "task.complete", "info", WithIssue(42))

	// Read back to verify
	r := NewEventReader(dir)
	events, err := r.ReadSession("global-ev-test")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Component != "worker" {
		t.Errorf("Component = %q, want worker", events[0].Component)
	}
	if events[0].IssueID != 42 {
		t.Errorf("IssueID = %d, want 42", events[0].IssueID)
	}
}

// ---------------------------------------------------------------------------
// EventReader: ReadCurrentSession / ReadCurrentSessionFiltered
// ---------------------------------------------------------------------------

func TestEventReader_ReadCurrentSession_MissingSessionFile(t *testing.T) {
	dir := t.TempDir()
	r := NewEventReader(dir)
	_, err := r.ReadCurrentSession()
	if err == nil {
		t.Error("expected error when session.json is missing")
	}
}

func TestEventReader_ReadCurrentSessionFiltered_MissingSessionFile(t *testing.T) {
	dir := t.TempDir()
	r := NewEventReader(dir)
	_, err := r.ReadCurrentSessionFiltered(EventFilter{})
	if err == nil {
		t.Error("expected error when session.json is missing")
	}
}

func TestEventReader_ReadCurrentSession_WithSessionFile(t *testing.T) {
	dir := t.TempDir()

	// Write events for a session
	w, err := NewEventWriter(dir, "current-sess")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	w.Write("principal", "start", "info")
	w.Close()

	// Write session.json so ReadCurrentSession can find the session ID
	principalDir := filepath.Join(dir, ".ai", "state", "principal")
	os.MkdirAll(principalDir, 0755)
	sessionJSON := `{"session_id": "current-sess"}`
	os.WriteFile(filepath.Join(principalDir, "session.json"), []byte(sessionJSON), 0644)

	r := NewEventReader(dir)
	events, err := r.ReadCurrentSession()
	if err != nil {
		t.Fatalf("ReadCurrentSession: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// EventReader: ReadByIssue across sessions
// ---------------------------------------------------------------------------

func TestEventReader_ReadByIssue_NoneMatch(t *testing.T) {
	dir := t.TempDir()

	// Write events for issue 1
	w, err := NewEventWriter(dir, "sess-for-issue")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	w.Write("principal", "dispatch", "info", WithIssue(1))
	w.Close()

	r := NewEventReader(dir)
	events, err := r.ReadByIssue(99) // look for issue 99
	if err != nil {
		t.Fatalf("ReadByIssue: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for issue 99, got %d", len(events))
	}
}

func TestEventReader_ReadByIssue_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := NewEventReader(dir)
	events, err := r.ReadByIssue(5)
	if err != nil {
		t.Fatalf("ReadByIssue on empty dir: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// EventReader: matchesFilter (tested via ReadSessionFiltered)
// ---------------------------------------------------------------------------

func TestEventReader_Filter_ByMultipleLevels(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "multi-level-sess")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	w.Write("principal", "e1", "info")
	w.Write("principal", "e2", "error")
	w.Write("principal", "e3", "warn")
	w.Close()

	r := NewEventReader(dir)
	events, err := r.ReadSessionFiltered("multi-level-sess", EventFilter{
		Levels: []string{"info", "error"},
	})
	if err != nil {
		t.Fatalf("ReadSessionFiltered: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (info + error), got %d", len(events))
	}
}

func TestEventReader_Filter_ByMultipleComponents(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "multi-comp-sess")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	w.Write("principal", "e1", "info")
	w.Write("worker", "e2", "info")
	w.Write("reviewer", "e3", "info")
	w.Close()

	r := NewEventReader(dir)
	events, err := r.ReadSessionFiltered("multi-comp-sess", EventFilter{
		Components: []string{"principal", "worker"},
	})
	if err != nil {
		t.Fatalf("ReadSessionFiltered: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (principal + worker), got %d", len(events))
	}
}

func TestEventReader_Filter_ByPRNumber(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "pr-filter-sess")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	w.Write("reviewer", "pr.merged", "info", WithPR(100))
	w.Write("reviewer", "pr.merged", "info", WithPR(200))
	w.Close()

	r := NewEventReader(dir)
	events, err := r.ReadSessionFiltered("pr-filter-sess", EventFilter{PRNumber: 100})
	if err != nil {
		t.Fatalf("ReadSessionFiltered: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event for PR 100, got %d", len(events))
	}
	if events[0].PRNumber != 100 {
		t.Errorf("PRNumber = %d, want 100", events[0].PRNumber)
	}
}

func TestEventReader_Filter_ByTypes(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "type-filter-sess")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	w.Write("principal", "dispatch.start", "info")
	w.Write("principal", "dispatch.complete", "info")
	w.Write("principal", "review.start", "info")
	w.Close()

	r := NewEventReader(dir)
	events, err := r.ReadSessionFiltered("type-filter-sess", EventFilter{
		Types: []string{"dispatch.start", "review.start"},
	})
	if err != nil {
		t.Fatalf("ReadSessionFiltered: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// EventOption helpers: WithData, WithErrorString
// ---------------------------------------------------------------------------

func TestWithData_SetsData(t *testing.T) {
	e := &Event{}
	WithData(map[string]any{"key": "value"})(e)
	if e.Data == nil {
		t.Error("WithData should set Data field")
	}
}

func TestWithErrorString_SetsError(t *testing.T) {
	e := &Event{}
	WithErrorString("something went wrong")(e)
	if e.Error != "something went wrong" {
		t.Errorf("Error = %q, want 'something went wrong'", e.Error)
	}
}

func TestWithError_NilError_NoOp(t *testing.T) {
	e := &Event{}
	WithError(nil)(e)
	if e.Error != "" {
		t.Errorf("WithError(nil) should not set error, got %q", e.Error)
	}
}

func TestWithIssue_SetsIssueID(t *testing.T) {
	e := &Event{}
	WithIssue(123)(e)
	if e.IssueID != 123 {
		t.Errorf("IssueID = %d, want 123", e.IssueID)
	}
}

func TestWithPR_SetsPRNumber(t *testing.T) {
	e := &Event{}
	WithPR(456)(e)
	if e.PRNumber != 456 {
		t.Errorf("PRNumber = %d, want 456", e.PRNumber)
	}
}

// ---------------------------------------------------------------------------
// WriteDecision
// ---------------------------------------------------------------------------

func TestEventWriter_WriteDecision(t *testing.T) {
	dir := t.TempDir()
	w, err := NewEventWriter(dir, "decision-sess")
	if err != nil {
		t.Fatalf("NewEventWriter: %v", err)
	}
	defer w.Close()

	d := Decision{
		Rule:   "task_loop",
		Result: "CONTINUE",
	}
	if err := w.WriteDecision("principal", "decision.made", d, WithIssue(7)); err != nil {
		t.Fatalf("WriteDecision: %v", err)
	}

	r := NewEventReader(dir)
	events, err := r.ReadSession("decision-sess")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Decision == nil {
		t.Error("Decision field should be set")
	} else if events[0].Decision.Result != "CONTINUE" {
		t.Errorf("Result = %q, want CONTINUE", events[0].Decision.Result)
	}
}
