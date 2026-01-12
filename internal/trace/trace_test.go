package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventWriter_Write(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-session-001"

	// Create writer
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer writer.Close()

	// Write some events
	err = writer.Write(ComponentPrincipal, TypeSessionStart, LevelInfo)
	if err != nil {
		t.Fatalf("failed to write event: %v", err)
	}

	err = writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo,
		WithIssue(25),
		WithData(map[string]string{"repo": "backend"}))
	if err != nil {
		t.Fatalf("failed to write event with options: %v", err)
	}

	err = writer.WriteDecision(ComponentPrincipal, TypeLoopDecision, Decision{
		Rule:       "continue if: status == failed_will_retry AND attempts < max",
		Conditions: map[string]any{"status": "failed_will_retry", "attempts": 1, "max": 3},
		Result:     "CONTINUE",
	}, WithIssue(25))
	if err != nil {
		t.Fatalf("failed to write decision event: %v", err)
	}

	// Verify sequence numbers
	if writer.Seq() != 3 {
		t.Errorf("expected seq 3, got %d", writer.Seq())
	}

	// Close writer
	writer.Close()

	// Read events back
	reader := NewEventReader(tmpDir)
	events, err := reader.ReadSession(sessionID)
	if err != nil {
		t.Fatalf("failed to read session: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify first event
	if events[0].Seq != 1 {
		t.Errorf("event 0: expected seq 1, got %d", events[0].Seq)
	}
	if events[0].Component != ComponentPrincipal {
		t.Errorf("event 0: expected component %s, got %s", ComponentPrincipal, events[0].Component)
	}
	if events[0].Type != TypeSessionStart {
		t.Errorf("event 0: expected type %s, got %s", TypeSessionStart, events[0].Type)
	}

	// Verify second event has issue ID
	if events[1].IssueID != 25 {
		t.Errorf("event 1: expected issue_id 25, got %d", events[1].IssueID)
	}

	// Verify third event is a decision
	if events[2].Decision == nil {
		t.Error("event 2: expected decision, got nil")
	} else {
		if events[2].Decision.Result != "CONTINUE" {
			t.Errorf("event 2: expected decision result CONTINUE, got %s", events[2].Decision.Result)
		}
	}
}

func TestEventReader_Filter(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-session-002"

	// Create writer and write events
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Write mix of events
	writer.Write(ComponentPrincipal, TypeSessionStart, LevelInfo)
	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(25))
	writer.Write(ComponentWorker, TypeWorkerStep, LevelInfo, WithIssue(25))
	writer.Write(ComponentGitHub, TypeCommentFail, LevelError, WithIssue(25))
	writer.WriteDecision(ComponentPrincipal, TypeLoopDecision, Decision{
		Rule:   "test",
		Result: "CONTINUE",
	})
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelError, WithIssue(25))
	writer.Write(ComponentPrincipal, TypeSessionEnd, LevelInfo)

	writer.Close()

	// Read and filter
	reader := NewEventReader(tmpDir)

	// Filter by level=error
	errorEvents, err := reader.ReadSessionFiltered(sessionID, EventFilter{Level: LevelError})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}
	if len(errorEvents) != 2 {
		t.Errorf("expected 2 error events, got %d", len(errorEvents))
	}

	// Filter by level=decision
	decisionEvents, err := reader.ReadSessionFiltered(sessionID, EventFilter{Level: LevelDecision})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}
	if len(decisionEvents) != 1 {
		t.Errorf("expected 1 decision event, got %d", len(decisionEvents))
	}

	// Filter by component=worker
	workerEvents, err := reader.ReadSessionFiltered(sessionID, EventFilter{Component: ComponentWorker})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}
	if len(workerEvents) != 3 {
		t.Errorf("expected 3 worker events, got %d", len(workerEvents))
	}

	// Filter by issue_id=25
	issueEvents, err := reader.ReadSessionFiltered(sessionID, EventFilter{IssueID: 25})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}
	if len(issueEvents) != 4 {
		t.Errorf("expected 4 events for issue 25, got %d", len(issueEvents))
	}

	// Filter last 3
	lastEvents, err := reader.ReadSessionFiltered(sessionID, EventFilter{Last: 3})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}
	if len(lastEvents) != 3 {
		t.Errorf("expected 3 last events, got %d", len(lastEvents))
	}
}

func TestGlobalWriter(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-global-session"

	// Initialize global writer
	err = InitGlobalWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to init global writer: %v", err)
	}
	defer CloseGlobalWriter()

	// Write using global functions
	WriteEvent(ComponentPrincipal, TypeSessionStart, LevelInfo)
	WriteEvent(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(10))
	WriteDecisionEvent(ComponentPrincipal, TypeCheckResult, Decision{
		Rule:   "retry if failed",
		Result: "RETRY",
	}, WithIssue(10))

	// Close and verify
	CloseGlobalWriter()

	reader := NewEventReader(tmpDir)
	events, err := reader.ReadSession(sessionID)
	if err != nil {
		t.Fatalf("failed to read session: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestEventWriter_Resume(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-resume-session"

	// First writer
	writer1, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	writer1.Write(ComponentPrincipal, TypeSessionStart, LevelInfo)
	writer1.Write(ComponentWorker, TypeWorkerStart, LevelInfo)
	writer1.Close()

	// Second writer (resume)
	writer2, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer for resume: %v", err)
	}

	// Should continue from seq 3
	if writer2.Seq() != 2 {
		t.Errorf("expected seq 2 after resume, got %d", writer2.Seq())
	}

	writer2.Write(ComponentWorker, TypeWorkerEnd, LevelInfo)
	writer2.Close()

	// Verify all events
	reader := NewEventReader(tmpDir)
	events, err := reader.ReadSession(sessionID)
	if err != nil {
		t.Fatalf("failed to read session: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	if events[2].Seq != 3 {
		t.Errorf("expected seq 3 for last event, got %d", events[2].Seq)
	}
}

func TestEventOption_WithError(t *testing.T) {
	event := NewEvent(1, ComponentGitHub, TypeCommentFail, LevelError,
		WithError(os.ErrNotExist),
		WithIssue(42))

	if event.Error != "file does not exist" {
		t.Errorf("expected error message, got %s", event.Error)
	}

	if event.IssueID != 42 {
		t.Errorf("expected issue_id 42, got %d", event.IssueID)
	}
}

func TestNewDecisionEvent(t *testing.T) {
	decision := Decision{
		Rule: "continue if pending",
		Conditions: map[string]any{
			"pending":   true,
			"max_loops": 100,
			"current":   5,
		},
		Result: "CONTINUE",
	}

	event := NewDecisionEvent(10, ComponentPrincipal, TypeLoopDecision, decision, WithIssue(30))

	if event.Level != LevelDecision {
		t.Errorf("expected level %s, got %s", LevelDecision, event.Level)
	}

	if event.Decision == nil {
		t.Fatal("expected decision, got nil")
	}

	if event.Decision.Result != "CONTINUE" {
		t.Errorf("expected result CONTINUE, got %s", event.Decision.Result)
	}

	if event.IssueID != 30 {
		t.Errorf("expected issue_id 30, got %d", event.IssueID)
	}
}

func TestEventReader_ListSessions(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple sessions
	sessions := []string{"session-20251227-100000", "session-20251227-110000", "session-20251227-120000"}
	for _, s := range sessions {
		writer, err := NewEventWriter(tmpDir, s)
		if err != nil {
			t.Fatalf("failed to create writer for %s: %v", s, err)
		}
		writer.Write(ComponentPrincipal, TypeSessionStart, LevelInfo)
		writer.Close()
	}

	// List sessions
	reader := NewEventReader(tmpDir)
	listed, err := reader.ListSessions()
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(listed) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(listed))
	}

	// Should be sorted in reverse order (newest first)
	if listed[0] != "session-20251227-120000" {
		t.Errorf("expected newest session first, got %s", listed[0])
	}
}

func TestEvent_Timestamp(t *testing.T) {
	before := time.Now().UTC()
	event := NewEvent(1, ComponentPrincipal, TypeSessionStart, LevelInfo)
	after := time.Now().UTC()

	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("timestamp %v not in expected range [%v, %v]", event.Timestamp, before, after)
	}
}

func TestEventReader_NonExistentSession(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	reader := NewEventReader(tmpDir)
	_, err = reader.ReadSession("non-existent")
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

func TestEventWriter_FilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "trace-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-filepath"
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer writer.Close()

	expected := filepath.Join(tmpDir, ".ai", "state", "events", sessionID+".jsonl")
	if writer.FilePath() != expected {
		t.Errorf("expected path %s, got %s", expected, writer.FilePath())
	}
}
