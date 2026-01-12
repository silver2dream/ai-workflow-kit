// Package trace provides unified event stream for workflow tracing.
package trace

import (
	"time"
)

// Component represents the source component of an event.
const (
	ComponentPrincipal = "principal"
	ComponentWorker    = "worker"
	ComponentReviewer  = "reviewer"
	ComponentGitHub    = "github"
)

// Level represents the severity/type of an event.
const (
	LevelInfo     = "info"
	LevelWarn     = "warn"
	LevelError    = "error"
	LevelDecision = "decision"
)

// Event types for each component.
const (
	// Principal events
	TypeSessionStart  = "session_start"
	TypeSessionEnd    = "session_end"
	TypeLoopStart     = "loop_start"
	TypeLoopDecision  = "loop_decision"
	TypeDispatchStart = "dispatch_start"
	TypeDispatchEnd   = "dispatch_end"
	TypeCheckResult   = "check_result"

	// Worker events
	TypeWorkerStart = "start"
	TypeWorkerStep  = "step"
	TypeWorkerEnd   = "end"

	// GitHub events
	TypeCommentSend   = "comment_send"
	TypeCommentFail   = "comment_fail"
	TypeLabelUpdate   = "label_update"
	TypePRCreate      = "pr_create"
	TypePRMerge       = "pr_merge"
	TypePRMergeFail   = "pr_merge_fail"

	// Reviewer events
	TypeReviewStart    = "review_start"
	TypeReviewDecision = "review_decision"
	TypeReviewEnd      = "review_end"
)

// Event represents a single trace event in the unified event stream.
type Event struct {
	// Identification
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"ts"`

	// Classification
	Component string `json:"component"`
	Type      string `json:"type"`
	Level     string `json:"level"`

	// Association
	IssueID  int `json:"issue_id,omitempty"`
	PRNumber int `json:"pr_number,omitempty"`

	// Content
	Data     any       `json:"data,omitempty"`
	Decision *Decision `json:"decision,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Decision represents a decision point with its rule, conditions, and result.
type Decision struct {
	Rule       string         `json:"rule"`
	Conditions map[string]any `json:"conditions"`
	Result     string         `json:"result"`
}

// EventOption is a functional option for configuring an Event.
type EventOption func(*Event)

// WithIssue sets the issue ID for the event.
func WithIssue(id int) EventOption {
	return func(e *Event) {
		e.IssueID = id
	}
}

// WithPR sets the PR number for the event.
func WithPR(number int) EventOption {
	return func(e *Event) {
		e.PRNumber = number
	}
}

// WithData sets arbitrary data for the event.
func WithData(data any) EventOption {
	return func(e *Event) {
		e.Data = data
	}
}

// WithError sets the error message for the event.
func WithError(err error) EventOption {
	return func(e *Event) {
		if err != nil {
			e.Error = err.Error()
		}
	}
}

// WithErrorString sets the error message from a string.
func WithErrorString(errMsg string) EventOption {
	return func(e *Event) {
		e.Error = errMsg
	}
}

// NewEvent creates a new Event with the given parameters and options.
func NewEvent(seq int, component, eventType, level string, opts ...EventOption) *Event {
	e := &Event{
		Seq:       seq,
		Timestamp: time.Now().UTC(),
		Component: component,
		Type:      eventType,
		Level:     level,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// NewDecisionEvent creates a new decision event.
func NewDecisionEvent(seq int, component, eventType string, decision Decision, opts ...EventOption) *Event {
	e := NewEvent(seq, component, eventType, LevelDecision, opts...)
	e.Decision = &decision
	return e
}
