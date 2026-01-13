package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestQueryTraces_FilterByIssueID tests filtering traces by issue ID.
func TestQueryTraces_FilterByIssueID(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-query-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-filter-issue"

	// Create writer and write events with different issue IDs
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Write events for issue 42
	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(42))
	writer.Write(ComponentWorker, TypeWorkerStep, LevelInfo, WithIssue(42))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelInfo, WithIssue(42))

	// Write events for issue 99
	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(99))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelError, WithIssue(99))

	// Write event without issue ID
	writer.Write(ComponentPrincipal, TypeSessionEnd, LevelInfo)

	writer.Close()

	// Read and filter by issue 42
	reader := NewEventReader(tmpDir)
	events, err := reader.ReadSessionFiltered(sessionID, EventFilter{IssueID: 42})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events for issue 42, got %d", len(events))
	}

	// Verify all returned events have issue ID 42
	for i, e := range events {
		if e.IssueID != 42 {
			t.Errorf("event %d: expected issue_id 42, got %d", i, e.IssueID)
		}
	}

	// Read and filter by issue 99
	events99, err := reader.ReadSessionFiltered(sessionID, EventFilter{IssueID: 99})
	if err != nil {
		t.Fatalf("failed to read filtered events for issue 99: %v", err)
	}

	if len(events99) != 2 {
		t.Errorf("expected 2 events for issue 99, got %d", len(events99))
	}
}

// TestQueryTraces_JSONOutput tests that events can be marshaled to valid JSON.
func TestQueryTraces_JSONOutput(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-json-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-json-output"

	// Create writer and write events
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	issueID := 123
	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(issueID),
		WithData(map[string]string{"repo": "backend", "branch": "feat/ai-issue-test"}))
	writer.Write(ComponentWorker, TypeWorkerStep, LevelInfo, WithIssue(issueID),
		WithData(map[string]any{"attempt": 1, "step": "codex_exec"}))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelError, WithIssue(issueID),
		WithErrorString("test failure"))

	writer.Close()

	// Read events
	reader := NewEventReader(tmpDir)
	events, err := reader.ReadSession(sessionID)
	if err != nil {
		t.Fatalf("failed to read session: %v", err)
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("failed to marshal events to JSON: %v", err)
	}

	// Verify it's valid JSON by unmarshaling
	var parsed []Event
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(parsed) != 3 {
		t.Errorf("expected 3 events, got %d", len(parsed))
	}

	// Verify structure matches expected format (similar to Python test)
	type QueryResult struct {
		Count  int     `json:"count"`
		Traces []Event `json:"traces"`
	}

	result := QueryResult{
		Count:  len(events),
		Traces: events,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Verify the result can be parsed back
	var parsedResult QueryResult
	if err := json.Unmarshal(resultJSON, &parsedResult); err != nil {
		t.Fatalf("failed to unmarshal result JSON: %v", err)
	}

	if parsedResult.Count != 3 {
		t.Errorf("expected count 3, got %d", parsedResult.Count)
	}

	if len(parsedResult.Traces) != 3 {
		t.Errorf("expected 3 traces, got %d", len(parsedResult.Traces))
	}

	// Verify first trace has expected issue ID
	if parsedResult.Traces[0].IssueID != issueID {
		t.Errorf("expected issue_id %d, got %d", issueID, parsedResult.Traces[0].IssueID)
	}
}

// TestQueryTraces_NoMatchingTraces tests empty result handling.
func TestQueryTraces_NoMatchingTraces(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-nomatch-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-no-match"

	// Create writer and write events with issue 50
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(50))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelInfo, WithIssue(50))

	writer.Close()

	// Read and filter by non-existent issue 999
	reader := NewEventReader(tmpDir)
	events, err := reader.ReadSessionFiltered(sessionID, EventFilter{IssueID: 999})
	if err != nil {
		t.Fatalf("failed to read filtered events: %v", err)
	}

	// Should return no matching events (nil or empty slice is acceptable)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// TestQueryTraces_InvalidIssueID tests handling of invalid/negative issue IDs.
func TestQueryTraces_InvalidIssueID(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-invalid-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-invalid-issue"

	// Create writer and write events
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(10))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelInfo, WithIssue(10))

	writer.Close()

	reader := NewEventReader(tmpDir)

	// Filter with issue ID 0 (should not filter, return all)
	eventsZero, err := reader.ReadSessionFiltered(sessionID, EventFilter{IssueID: 0})
	if err != nil {
		t.Fatalf("failed to read with issue ID 0: %v", err)
	}

	// IssueID 0 means no filter, should return all events
	if len(eventsZero) != 2 {
		t.Errorf("expected 2 events with no issue filter, got %d", len(eventsZero))
	}

	// Note: Negative issue IDs are treated the same as 0 (no filter) in the current implementation
	// because the filter uses `filter.IssueID > 0` as the condition.
	// This test documents the actual behavior.
	eventsNeg, err := reader.ReadSessionFiltered(sessionID, EventFilter{IssueID: -1})
	if err != nil {
		t.Fatalf("failed to read with negative issue ID: %v", err)
	}

	// Negative IDs don't filter (same as 0), so all events are returned
	if len(eventsNeg) != 2 {
		t.Errorf("expected 2 events for negative issue ID (no filter applied), got %d", len(eventsNeg))
	}
}

// TestQueryTraces_ReadByIssueAcrossSessions tests reading events by issue across multiple sessions.
func TestQueryTraces_ReadByIssueAcrossSessions(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-multisession-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	issueID := 77

	// Create first session
	writer1, err := NewEventWriter(tmpDir, "session-001")
	if err != nil {
		t.Fatalf("failed to create writer 1: %v", err)
	}
	writer1.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(issueID))
	writer1.Write(ComponentWorker, TypeWorkerEnd, LevelError, WithIssue(issueID))
	writer1.Close()

	// Create second session
	writer2, err := NewEventWriter(tmpDir, "session-002")
	if err != nil {
		t.Fatalf("failed to create writer 2: %v", err)
	}
	writer2.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(issueID))
	writer2.Write(ComponentWorker, TypeWorkerStep, LevelInfo, WithIssue(issueID))
	writer2.Write(ComponentWorker, TypeWorkerEnd, LevelInfo, WithIssue(issueID))
	writer2.Close()

	// Read by issue across all sessions
	reader := NewEventReader(tmpDir)
	events, err := reader.ReadByIssue(issueID)
	if err != nil {
		t.Fatalf("failed to read by issue: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("expected 5 events across sessions, got %d", len(events))
	}

	// Verify all have correct issue ID
	for i, e := range events {
		if e.IssueID != issueID {
			t.Errorf("event %d: expected issue_id %d, got %d", i, issueID, e.IssueID)
		}
	}
}

// TestQueryTraces_CombinedFilters tests multiple filter criteria together.
func TestQueryTraces_CombinedFilters(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "trace-combined-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "test-combined-filters"

	// Create writer and write mixed events
	writer, err := NewEventWriter(tmpDir, sessionID)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Issue 25: 2 info, 1 error
	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(25))
	writer.Write(ComponentWorker, TypeWorkerStep, LevelInfo, WithIssue(25))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelError, WithIssue(25))

	// Issue 30: 1 info, 1 error
	writer.Write(ComponentWorker, TypeWorkerStart, LevelInfo, WithIssue(30))
	writer.Write(ComponentWorker, TypeWorkerEnd, LevelError, WithIssue(30))

	writer.Close()

	reader := NewEventReader(tmpDir)

	// Filter by issue 25 AND level error
	events, err := reader.ReadSessionFiltered(sessionID, EventFilter{
		IssueID: 25,
		Level:   LevelError,
	})
	if err != nil {
		t.Fatalf("failed to read with combined filter: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 error event for issue 25, got %d", len(events))
	}

	if len(events) > 0 {
		if events[0].IssueID != 25 {
			t.Errorf("expected issue_id 25, got %d", events[0].IssueID)
		}
		if events[0].Level != LevelError {
			t.Errorf("expected level error, got %s", events[0].Level)
		}
	}
}

// TestQueryTraces_EventFilePath tests that event file paths are constructed correctly.
func TestQueryTraces_EventFilePath(t *testing.T) {
	tmpDir := "/tmp/test-trace-root"

	reader := NewEventReader(tmpDir)
	sessionID := "session-20251227-150000"

	expected := filepath.Join(tmpDir, ".ai", "state", "events", sessionID+".jsonl")
	actual := reader.GetSessionFilePath(sessionID)

	if actual != expected {
		t.Errorf("expected path %s, got %s", expected, actual)
	}
}
