package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	SessionID  string   // Filter by session ID
	Level      string   // Filter by level (info, warn, error, decision)
	Levels     []string // Filter by multiple levels
	Component  string   // Filter by component
	Components []string // Filter by multiple components
	IssueID    int      // Filter by issue ID (0 means no filter)
	PRNumber   int      // Filter by PR number (0 means no filter)
	Types      []string // Filter by event types
	Last       int      // Return only last N events (0 means all)
}

// EventReader reads events from JSONL files.
type EventReader struct {
	stateRoot string
}

// NewEventReader creates a new EventReader.
func NewEventReader(stateRoot string) *EventReader {
	return &EventReader{stateRoot: stateRoot}
}

// ReadSession reads all events from a specific session.
func (r *EventReader) ReadSession(sessionID string) ([]Event, error) {
	filePath := filepath.Join(r.stateRoot, ".ai", "state", "events", sessionID+".jsonl")
	return r.readEventsFromFile(filePath)
}

// ReadSessionFiltered reads events from a session with filtering.
func (r *EventReader) ReadSessionFiltered(sessionID string, filter EventFilter) ([]Event, error) {
	events, err := r.ReadSession(sessionID)
	if err != nil {
		return nil, err
	}

	return r.applyFilter(events, filter), nil
}

// ReadCurrentSession reads events from the current principal session.
func (r *EventReader) ReadCurrentSession() ([]Event, error) {
	sessionID, err := r.getCurrentSessionID()
	if err != nil {
		return nil, err
	}

	return r.ReadSession(sessionID)
}

// ReadCurrentSessionFiltered reads events from current session with filtering.
func (r *EventReader) ReadCurrentSessionFiltered(filter EventFilter) ([]Event, error) {
	sessionID, err := r.getCurrentSessionID()
	if err != nil {
		return nil, err
	}

	return r.ReadSessionFiltered(sessionID, filter)
}

// ListSessions returns a list of available session IDs.
func (r *EventReader) ListSessions() ([]string, error) {
	eventsDir := filepath.Join(r.stateRoot, ".ai", "state", "events")
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read events directory: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
			sessions = append(sessions, sessionID)
		}
	}

	// Sort by name (which includes timestamp)
	sort.Sort(sort.Reverse(sort.StringSlice(sessions)))

	return sessions, nil
}

// ReadByIssue reads all events related to a specific issue across all sessions.
func (r *EventReader) ReadByIssue(issueID int) ([]Event, error) {
	sessions, err := r.ListSessions()
	if err != nil {
		return nil, err
	}

	var allEvents []Event
	for _, sessionID := range sessions {
		events, err := r.ReadSession(sessionID)
		if err != nil {
			continue // Skip sessions that can't be read
		}

		for _, e := range events {
			if e.IssueID == issueID {
				allEvents = append(allEvents, e)
			}
		}
	}

	// Sort by timestamp
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Timestamp.Before(allEvents[j].Timestamp)
	})

	return allEvents, nil
}

// readEventsFromFile reads all events from a JSONL file.
func (r *EventReader) readEventsFromFile(filePath string) ([]Event, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: %s", filePath)
		}
		return nil, fmt.Errorf("failed to open event file: %w", err)
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)

	// Increase buffer size for potentially long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Log warning but continue reading
			fmt.Fprintf(os.Stderr, "warning: failed to parse event at line %d: %v\n", lineNum, err)
			continue
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("error reading event file: %w", err)
	}

	return events, nil
}

// applyFilter filters events based on the given criteria.
func (r *EventReader) applyFilter(events []Event, filter EventFilter) []Event {
	var filtered []Event

	for _, e := range events {
		if !r.matchesFilter(e, filter) {
			continue
		}
		filtered = append(filtered, e)
	}

	// Apply "last N" filter
	if filter.Last > 0 && len(filtered) > filter.Last {
		filtered = filtered[len(filtered)-filter.Last:]
	}

	return filtered
}

// matchesFilter checks if an event matches the filter criteria.
func (r *EventReader) matchesFilter(e Event, filter EventFilter) bool {
	// Level filter
	if filter.Level != "" && e.Level != filter.Level {
		return false
	}

	// Multiple levels filter
	if len(filter.Levels) > 0 {
		matched := false
		for _, l := range filter.Levels {
			if e.Level == l {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Component filter
	if filter.Component != "" && e.Component != filter.Component {
		return false
	}

	// Multiple components filter
	if len(filter.Components) > 0 {
		matched := false
		for _, c := range filter.Components {
			if e.Component == c {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Issue ID filter
	if filter.IssueID > 0 && e.IssueID != filter.IssueID {
		return false
	}

	// PR number filter
	if filter.PRNumber > 0 && e.PRNumber != filter.PRNumber {
		return false
	}

	// Event types filter
	if len(filter.Types) > 0 {
		matched := false
		for _, t := range filter.Types {
			if e.Type == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// getCurrentSessionID reads the current session ID from the principal session file.
func (r *EventReader) getCurrentSessionID() (string, error) {
	sessionFile := filepath.Join(r.stateRoot, ".ai", "state", "principal", "session.json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return "", fmt.Errorf("failed to read current session: %w", err)
	}

	var session struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return "", fmt.Errorf("failed to parse session file: %w", err)
	}

	if session.SessionID == "" {
		return "", fmt.Errorf("no active session found")
	}

	return session.SessionID, nil
}

// GetSessionFilePath returns the file path for a session's event file.
func (r *EventReader) GetSessionFilePath(sessionID string) string {
	return filepath.Join(r.stateRoot, ".ai", "state", "events", sessionID+".jsonl")
}
