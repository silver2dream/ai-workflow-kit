package ghutil

import "context"

// GitHubCLI abstracts GitHub CLI operations for testability.
// Callers that currently use RunWithRetry directly can accept this interface
// to enable unit testing without a real gh binary.
type GitHubCLI interface {
	// RunWithRetry executes a CLI command (typically "gh") with exponential-backoff
	// retry. Returns combined stdout+stderr output and any error from the last
	// attempt.
	RunWithRetry(ctx context.Context, cfg RetryConfig, name string, args ...string) ([]byte, error)
}

// realGitHubCLI is the production implementation backed by the gh binary.
type realGitHubCLI struct{}

// NewGitHubCLI returns the production GitHubCLI that delegates to RunWithRetry.
func NewGitHubCLI() GitHubCLI {
	return &realGitHubCLI{}
}

func (r *realGitHubCLI) RunWithRetry(ctx context.Context, cfg RetryConfig, name string, args ...string) ([]byte, error) {
	return RunWithRetry(ctx, cfg, name, args...)
}
