package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AttemptGuard manages retry logic and failure tracking
type AttemptGuard struct {
	StateRoot   string
	IssueID     int
	MaxAttempts int
}

// AttemptResult represents the result of an attempt check
type AttemptResult struct {
	CanProceed    bool
	AttemptNumber int
	Reason        string // "ok", "max_attempts", "non_retryable"
	ErrorType     string
	Retryable     bool
}

// FailureEntry represents an entry in failure history
type FailureEntry struct {
	Timestamp string `json:"timestamp"`
	IssueID   int    `json:"issue_id"`
	Attempt   int    `json:"attempt"`
	PatternID string `json:"pattern_id"`
	Type      string `json:"type"`
	Retryable bool   `json:"retryable"`
}

// NewAttemptGuard creates a new AttemptGuard
func NewAttemptGuard(stateRoot string, issueID int) *AttemptGuard {
	maxAttempts := 3
	if val := os.Getenv("AI_MAX_ATTEMPTS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			maxAttempts = n
		}
	}

	return &AttemptGuard{
		StateRoot:   stateRoot,
		IssueID:     issueID,
		MaxAttempts: maxAttempts,
	}
}

// Check determines if the worker can proceed with the current attempt
func (g *AttemptGuard) Check() (*AttemptResult, error) {
	runDir := filepath.Join(g.StateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", g.IssueID))
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory: %w", err)
	}

	stateDir := filepath.Join(g.StateRoot, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	// Read current fail count
	countFile := filepath.Join(runDir, "fail_count.txt")
	count := g.readFailCount(countFile)

	// Check if max attempts exceeded
	if count >= g.MaxAttempts {
		// Check if worker-failed label was removed (human intervention)
		// This would allow reset, but we'll keep it simple for now
		return &AttemptResult{
			CanProceed:    false,
			AttemptNumber: count,
			Reason:        "max_attempts",
		}, nil
	}

	// Increment attempt count
	count++
	if err := g.writeFailCount(countFile, count); err != nil {
		return nil, fmt.Errorf("failed to write fail count: %w", err)
	}

	return &AttemptResult{
		CanProceed:    true,
		AttemptNumber: count,
		Reason:        "ok",
	}, nil
}

// RecordFailure records a failure to the history
func (g *AttemptGuard) RecordFailure(errorType string, retryable bool) error {
	stateDir := filepath.Join(g.StateRoot, ".ai", "state")
	historyFile := filepath.Join(stateDir, "failure_history.jsonl")

	runDir := filepath.Join(g.StateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", g.IssueID))
	countFile := filepath.Join(runDir, "fail_count.txt")
	attempt := g.readFailCount(countFile)

	entry := FailureEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		IssueID:   g.IssueID,
		Attempt:   attempt,
		PatternID: "go-impl",
		Type:      errorType,
		Retryable: retryable,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(string(data) + "\n")
	return err
}

// Reset resets the fail count
func (g *AttemptGuard) Reset() error {
	runDir := filepath.Join(g.StateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", g.IssueID))
	countFile := filepath.Join(runDir, "fail_count.txt")
	return os.WriteFile(countFile, []byte("0"), 0644)
}

// readFailCount reads the fail count from file
func (g *AttemptGuard) readFailCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return count
}

// writeFailCount writes the fail count to file
func (g *AttemptGuard) writeFailCount(path string, count int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(count)), 0644)
}
