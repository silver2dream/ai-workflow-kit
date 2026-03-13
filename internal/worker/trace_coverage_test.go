package worker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TraceRecorder
// ---------------------------------------------------------------------------

func TestCov_TraceRecorder_NewAndStepLifecycle(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 42, "backend", "feat/ai-issue-42", "develop", 12345, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder failed: %v", err)
	}

	// Verify initial trace file
	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-42.json")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("trace file not found: %v", err)
	}

	var trace ExecutionTrace
	if err := json.Unmarshal(data, &trace); err != nil {
		t.Fatalf("failed to parse trace: %v", err)
	}
	if trace.Status != "running" {
		t.Errorf("expected status 'running', got %q", trace.Status)
	}
	if trace.Repo != "backend" {
		t.Errorf("expected repo 'backend', got %q", trace.Repo)
	}
	if trace.WorkerPID != 12345 {
		t.Errorf("expected PID 12345, got %d", trace.WorkerPID)
	}

	// Add a step
	rec.StepStart("preflight")
	time.Sleep(1 * time.Millisecond) // ensure non-zero duration
	if err := rec.StepEnd("success", "", nil); err != nil {
		t.Fatalf("StepEnd failed: %v", err)
	}

	// Add another step with error
	rec.StepStart("worker")
	if err := rec.StepEnd("failed", "test error", map[string]interface{}{"exit": 1}); err != nil {
		t.Fatalf("StepEnd failed: %v", err)
	}

	// Finalize
	if err := rec.Finalize(nil); err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Verify final state
	data, _ = os.ReadFile(tracePath)
	json.Unmarshal(data, &trace)
	if trace.Status != "success" {
		t.Errorf("expected status 'success', got %q", trace.Status)
	}
	if len(trace.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(trace.Steps))
	}
	if trace.EndedAt == "" {
		t.Error("expected non-empty EndedAt")
	}
}

func TestCov_TraceRecorder_FinalizeWithError(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewTraceRecorder(dir, 10, "root", "feat/test", "main", 1, time.Now())
	if err != nil {
		t.Fatalf("NewTraceRecorder failed: %v", err)
	}

	runErr := errors.New("something failed")
	if err := rec.Finalize(runErr); err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	tracePath := filepath.Join(dir, ".ai", "state", "traces", "issue-10.json")
	data, _ := os.ReadFile(tracePath)

	var trace ExecutionTrace
	json.Unmarshal(data, &trace)
	if trace.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", trace.Status)
	}
	if trace.Error != "something failed" {
		t.Errorf("expected error 'something failed', got %q", trace.Error)
	}
}

func TestCov_TraceRecorder_StepEndWithoutStart(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewTraceRecorder(dir, 1, "root", "main", "main", 1, time.Now())
	if err != nil {
		t.Fatalf("NewTraceRecorder failed: %v", err)
	}

	// StepEnd without StepStart should be a no-op
	err = rec.StepEnd("success", "", nil)
	if err != nil {
		t.Errorf("StepEnd without StepStart should not error: %v", err)
	}

	rec.Finalize(nil)
}
