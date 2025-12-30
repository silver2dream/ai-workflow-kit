package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TraceRecorder records execution trace steps for a worker run.
type TraceRecorder struct {
	mu                  sync.Mutex
	path                string
	trace               *ExecutionTrace
	startTime           time.Time
	currentStepName     string
	currentStepStart    time.Time
	currentStepStartISO string
}

// NewTraceRecorder initializes a trace file with running status.
func NewTraceRecorder(stateRoot string, issueID int, repo, branch, baseBranch string, pid int, startTime time.Time) (*TraceRecorder, error) {
	traceDir := filepath.Join(stateRoot, ".ai", "state", "traces")
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create trace dir: %w", err)
	}

	traceID := fmt.Sprintf("issue-%d-%s", issueID, time.Now().UTC().Format("20060102T150405Z"))
	tracePath := filepath.Join(traceDir, fmt.Sprintf("issue-%d.json", issueID))
	utcStart := startTime.UTC()

	trace := &ExecutionTrace{
		TraceID:     traceID,
		IssueID:     fmt.Sprintf("%d", issueID),
		Repo:        repo,
		Branch:      branch,
		BaseBranch:  baseBranch,
		Status:      "running",
		StartedAt:   utcStart.Format(time.RFC3339),
		EndedAt:     "",
		Duration:    0,
		Error:       "",
		WorkerPID:   pid,
		WorkerStart: startTime.Unix(),
		Steps:       []TraceStep{},
	}

	rec := &TraceRecorder{
		path:      tracePath,
		trace:     trace,
		startTime: startTime,
	}

	if err := rec.writeLocked(); err != nil {
		return nil, err
	}

	return rec, nil
}

// StepStart marks a new trace step.
func (r *TraceRecorder) StepStart(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentStepName = name
	r.currentStepStart = time.Now()
	r.currentStepStartISO = r.currentStepStart.UTC().Format(time.RFC3339)
}

// StepEnd closes the current trace step with status and optional context.
func (r *TraceRecorder) StepEnd(status, errorMessage string, context map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentStepName == "" {
		return nil
	}

	endTime := time.Now()
	duration := int(endTime.Sub(r.currentStepStart).Seconds())

	step := TraceStep{
		Name:      r.currentStepName,
		Status:    status,
		StartedAt: r.currentStepStartISO,
		EndedAt:   endTime.UTC().Format(time.RFC3339),
		Duration:  duration,
		Error:     errorMessage,
		Context:   context,
	}

	r.trace.Steps = append(r.trace.Steps, step)
	if errorMessage != "" {
		r.trace.Error = errorMessage
	}

	r.currentStepName = ""
	r.currentStepStartISO = ""

	return r.writeLocked()
}

// Finalize writes final status, end time, and duration.
func (r *TraceRecorder) Finalize(runErr error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	status := "success"
	if runErr != nil {
		status = "failed"
		r.trace.Error = runErr.Error()
	}

	now := time.Now().UTC()
	r.trace.Status = status
	r.trace.EndedAt = now.Format(time.RFC3339)
	r.trace.Duration = int(now.Sub(r.startTime).Seconds())

	return r.writeLocked()
}

func (r *TraceRecorder) writeLocked() error {
	data, err := json.MarshalIndent(r.trace, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal trace: %w", err)
	}

	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write trace temp file: %w", err)
	}

	if err := os.Rename(tmpPath, r.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename trace temp file: %w", err)
	}

	return nil
}
