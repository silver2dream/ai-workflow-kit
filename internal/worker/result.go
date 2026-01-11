// Package worker provides worker execution and result checking functionality.
package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IssueResult represents the result JSON file structure (.ai/results/issue-{N}.json)
type IssueResult struct {
	IssueID           string        `json:"issue_id"`
	Status            string        `json:"status"` // success, failed, crashed, timeout, needs_conflict_resolution
	Repo              string        `json:"repo"`
	WorktreePath      string        `json:"worktree_path,omitempty"` // For conflict resolution
	RepoType          string        `json:"repo_type"`
	WorkDir           string        `json:"work_dir"`
	Branch            string        `json:"branch"`
	BaseBranch        string        `json:"base_branch"`
	HeadSHA           string        `json:"head_sha"`
	SubmoduleSHA      string        `json:"submodule_sha,omitempty"`
	ConsistencyStatus string        `json:"consistency_status,omitempty"`
	FailureStage      string        `json:"failure_stage,omitempty"`
	RecoveryCommand   string        `json:"recovery_command,omitempty"`
	TimestampUTC      string        `json:"timestamp_utc"`
	PRURL             string        `json:"pr_url,omitempty"`
	SpecName          string        `json:"spec_name,omitempty"`
	TaskLine          string        `json:"task_line,omitempty"`
	SummaryFile       string        `json:"summary_file,omitempty"`
	SubmoduleStatus   string        `json:"submodule_status,omitempty"`
	ErrorMessage      string        `json:"error,omitempty"`
	Recoverable       bool          `json:"recoverable,omitempty"`
	Session           SessionInfo   `json:"session"`
	ReviewAudit       ReviewAudit   `json:"review_audit,omitempty"`
	Metrics           ResultMetrics `json:"metrics,omitempty"`
}

// SessionInfo contains session tracking information
type SessionInfo struct {
	WorkerSessionID       string   `json:"worker_session_id,omitempty"`
	PrincipalSessionID    string   `json:"principal_session_id,omitempty"`
	AttemptNumber         int      `json:"attempt_number,omitempty"`
	PreviousSessionIDs    []string `json:"previous_session_ids,omitempty"`
	PreviousFailureReason string   `json:"previous_failure_reason,omitempty"`
	WorkerPID             int      `json:"worker_pid,omitempty"`
	WorkerStartTime       int64    `json:"worker_start_time,omitempty"`
}

// ReviewAudit contains PR review information
type ReviewAudit struct {
	ReviewerSessionID string `json:"reviewer_session_id,omitempty"`
	ReviewTimestamp   string `json:"review_timestamp,omitempty"`
	CIStatus          string `json:"ci_status,omitempty"`
	CITimeout         bool   `json:"ci_timeout,omitempty"`
	Decision          string `json:"decision,omitempty"`
	MergeTimestamp    string `json:"merge_timestamp,omitempty"`
}

// ResultMetrics contains execution metrics
type ResultMetrics struct {
	DurationSeconds int `json:"duration_seconds,omitempty"`
	RetryCount      int `json:"retry_count,omitempty"`
}

// ExecutionTrace represents the trace JSON file structure (.ai/state/traces/issue-{N}.json)
type ExecutionTrace struct {
	TraceID    string      `json:"trace_id"`
	IssueID    string      `json:"issue_id"`
	Repo       string      `json:"repo"`
	Branch     string      `json:"branch"`
	BaseBranch string      `json:"base_branch"`
	Status     string      `json:"status"` // running, success, failed
	StartedAt  string      `json:"started_at"`
	EndedAt    string      `json:"ended_at,omitempty"`
	Duration   int         `json:"duration_seconds"`
	Error      string      `json:"error,omitempty"`
	WorkerPID  int         `json:"worker_pid,omitempty"`
	WorkerStart int64      `json:"worker_start_time,omitempty"`
	Steps      []TraceStep `json:"steps"`
}

// TraceStep represents a single step in the execution trace
type TraceStep struct {
	Name      string                 `json:"name"`
	Status    string                 `json:"status"`
	StartedAt string                 `json:"started_at"`
	EndedAt   string                 `json:"ended_at"`
	Duration  int                    `json:"duration_seconds"`
	Error     string                 `json:"error,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// GetStartedAtTime parses the started_at field as time.Time
func (t *ExecutionTrace) GetStartedAtTime() (time.Time, error) {
	if t.StartedAt == "" {
		return time.Time{}, fmt.Errorf("started_at is empty")
	}
	// Try RFC3339 first
	if tm, err := time.Parse(time.RFC3339, t.StartedAt); err == nil {
		return tm, nil
	}
	// Try with Z suffix
	if tm, err := time.Parse("2006-01-02T15:04:05Z", t.StartedAt); err == nil {
		return tm, nil
	}
	return time.Parse(time.RFC3339Nano, t.StartedAt)
}

// LoadResult loads an issue result from the results directory
func LoadResult(stateRoot string, issueNumber int) (*IssueResult, error) {
	resultPath := filepath.Join(stateRoot, ".ai", "results", fmt.Sprintf("issue-%d.json", issueNumber))
	data, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, err
	}

	var result IssueResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result JSON: %w", err)
	}

	return &result, nil
}

// LoadTrace loads an execution trace from the traces directory
func LoadTrace(stateRoot string, issueNumber int) (*ExecutionTrace, error) {
	tracePath := filepath.Join(stateRoot, ".ai", "state", "traces", fmt.Sprintf("issue-%d.json", issueNumber))
	data, err := os.ReadFile(tracePath)
	if err != nil {
		return nil, err
	}

	var trace ExecutionTrace
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, fmt.Errorf("failed to parse trace JSON: %w", err)
	}

	return &trace, nil
}

// WriteFileAtomic writes data to a file atomically using tmp+rename pattern.
// This prevents file corruption if the process crashes during write.
//
// Platform notes:
// - On Unix: os.Rename is atomic and overwrites existing files
// - On Windows: os.Rename fails if destination exists, so we remove first
//
// The remove+rename sequence on Windows is not truly atomic, but is the best
// we can do without using Windows-specific APIs. The window of vulnerability
// is minimized by performing both operations in quick succession.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync the temp file to ensure data is flushed to disk before rename
	if f, err := os.OpenFile(tmpPath, os.O_RDWR, 0); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}

	// Remove target file first for Windows compatibility
	// On Windows, os.Rename fails if destination exists
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// If we can't remove the existing file (e.g., it's locked), cleanup and fail
		os.Remove(tmpPath)
		return fmt.Errorf("failed to remove existing file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// WriteResultAtomic writes an issue result atomically using tmp+rename pattern
// Note: On Windows, os.Rename cannot overwrite existing files, so we remove first
func WriteResultAtomic(stateRoot string, issueNumber int, result *IssueResult) error {
	resultDir := filepath.Join(stateRoot, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	resultPath := filepath.Join(resultDir, fmt.Sprintf("issue-%d.json", issueNumber))
	tmpPath := resultPath + ".tmp"

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync the temp file to ensure data is flushed to disk before rename
	if f, err := os.OpenFile(tmpPath, os.O_RDWR, 0); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}

	// Remove target file first for Windows compatibility
	// On Windows, os.Rename fails if destination exists
	_ = os.Remove(resultPath)

	if err := os.Rename(tmpPath, resultPath); err != nil {
		os.Remove(tmpPath) // cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// ReadFailCount reads the fail count for an issue from the runs directory
func ReadFailCount(stateRoot string, issueNumber int) int {
	failCountPath := filepath.Join(stateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", issueNumber), "fail_count.txt")
	data, err := os.ReadFile(failCountPath)
	if err != nil {
		return 0
	}

	var count int
	if _, err := fmt.Sscanf(string(data), "%d", &count); err != nil {
		return 0
	}
	return count
}

// ResetFailCount resets the fail count for an issue
func ResetFailCount(stateRoot string, issueNumber int) error {
	failCountPath := filepath.Join(stateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", issueNumber), "fail_count.txt")
	// Remove the file to reset count to 0
	if err := os.Remove(failCountPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ReadConsecutiveFailures reads the consecutive failures counter
func ReadConsecutiveFailures(stateRoot string) int {
	path := filepath.Join(stateRoot, ".ai", "state", "consecutive_failures")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var count int
	if _, err := fmt.Sscanf(string(data), "%d", &count); err != nil {
		return 0
	}
	return count
}

// ResetConsecutiveFailures resets the consecutive failures counter
func ResetConsecutiveFailures(stateRoot string) error {
	path := filepath.Join(stateRoot, ".ai", "state", "consecutive_failures")
	return os.WriteFile(path, []byte("0"), 0644)
}
