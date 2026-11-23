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
	// Guard is what the Worker's agent is held to while it runs; nil for
	// nothing beyond the backend's own defaults.
	Guard *WorkerGuard
}

// WorkerGuard is what a Worker's agent is held to while it runs: the tool
// policy of its role (internal/guard), enforced by a hook the agent cannot
// turn off, and settings of its own.
type WorkerGuard struct {
	// StateRoot is the project's root, where verdicts are recorded.
	StateRoot string
	// Protected are the branches pull requests are merged into: nothing is
	// pushed to them.
	Protected []string
	// AllowControl lets the Worker write the workflow's own files, as a
	// ticket that sets allow_script_changes allows.
	AllowControl bool
	// SettingsFile is where the agent's settings for the run are written.
	SettingsFile string
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
