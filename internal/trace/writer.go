package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// EventWriter writes events to a JSONL file.
type EventWriter struct {
	sessionID string
	file      *os.File
	seq       int
	mu        sync.Mutex
}

// NewEventWriter creates a new EventWriter for the given session.
// It creates the events directory if it doesn't exist.
func NewEventWriter(stateRoot, sessionID string) (*EventWriter, error) {
	eventsDir := filepath.Join(stateRoot, ".ai", "state", "events")
	if err := os.MkdirAll(eventsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create events directory: %w", err)
	}

	filePath := filepath.Join(eventsDir, sessionID+".jsonl")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open event file: %w", err)
	}

	// Read existing events to determine starting sequence number
	seq := 0
	if existingFile, err := os.Open(filePath); err == nil {
		defer existingFile.Close()
		decoder := json.NewDecoder(existingFile)
		for decoder.More() {
			var e Event
			if err := decoder.Decode(&e); err != nil {
				break
			}
			if e.Seq > seq {
				seq = e.Seq
			}
		}
	}

	return &EventWriter{
		sessionID: sessionID,
		file:      file,
		seq:       seq,
	}, nil
}

// Write writes an event to the event stream.
func (w *EventWriter) Write(component, eventType, level string, opts ...EventOption) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.seq++
	event := NewEvent(w.seq, component, eventType, level, opts...)

	return w.writeEvent(event)
}

// WriteDecision writes a decision event to the event stream.
func (w *EventWriter) WriteDecision(component, eventType string, decision Decision, opts ...EventOption) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.seq++
	event := NewDecisionEvent(w.seq, component, eventType, decision, opts...)

	return w.writeEvent(event)
}

// writeEvent writes an event to the file (must be called with lock held).
func (w *EventWriter) writeEvent(event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Write JSON line with newline
	if _, err := w.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Sync to ensure durability
	return w.file.Sync()
}

// Close closes the event writer.
func (w *EventWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// SessionID returns the session ID for this writer.
func (w *EventWriter) SessionID() string {
	return w.sessionID
}

// FilePath returns the path to the event file.
func (w *EventWriter) FilePath() string {
	if w.file != nil {
		return w.file.Name()
	}
	return ""
}

// Seq returns the current sequence number.
func (w *EventWriter) Seq() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seq
}

// GlobalWriter is a package-level event writer for convenience.
var (
	globalWriter *EventWriter
	globalMu     sync.RWMutex
)

// InitGlobalWriter initializes the global event writer.
func InitGlobalWriter(stateRoot, sessionID string) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalWriter != nil {
		globalWriter.Close()
	}

	w, err := NewEventWriter(stateRoot, sessionID)
	if err != nil {
		return err
	}

	globalWriter = w
	return nil
}

// CloseGlobalWriter closes the global event writer.
func CloseGlobalWriter() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalWriter != nil {
		err := globalWriter.Close()
		globalWriter = nil
		return err
	}
	return nil
}

// GetGlobalWriter returns the global event writer.
func GetGlobalWriter() *EventWriter {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalWriter
}

// WriteEvent writes an event using the global writer.
// Returns silently if no global writer is initialized.
func WriteEvent(component, eventType, level string, opts ...EventOption) {
	globalMu.RLock()
	w := globalWriter
	globalMu.RUnlock()

	if w == nil {
		return
	}

	_ = w.Write(component, eventType, level, opts...)
}

// WriteDecisionEvent writes a decision event using the global writer.
// Returns silently if no global writer is initialized.
func WriteDecisionEvent(component, eventType string, decision Decision, opts ...EventOption) {
	globalMu.RLock()
	w := globalWriter
	globalMu.RUnlock()

	if w == nil {
		return
	}

	_ = w.WriteDecision(component, eventType, decision, opts...)
}
