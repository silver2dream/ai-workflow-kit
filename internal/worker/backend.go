package worker

import (
	"context"
	"time"
)

// WorkerBackend abstracts AI worker execution.
type WorkerBackend interface {
	// Name returns the backend identifier (e.g. "codex", "claude-code").
	Name() string
	// Execute runs the AI worker with the given options.
	Execute(ctx context.Context, opts BackendOptions) BackendResult
	// Available checks if the backend CLI binary is in PATH.
	Available() error
}

// BackendOptions contains parameters for backend execution.
type BackendOptions struct {
	WorkDir     string
	PromptFile  string
	SummaryFile string
	LogBase     string
	MaxAttempts int
	RetryDelay  time.Duration
	Timeout     time.Duration
	Trace       *TraceRecorder
}

// BackendResult contains the result of backend execution.
type BackendResult struct {
	ExitCode      int
	Attempts      int
	RetryCount    int
	Duration      time.Duration
	FailureStage  string
	FailureReason string
	LastLogFile   string
}
