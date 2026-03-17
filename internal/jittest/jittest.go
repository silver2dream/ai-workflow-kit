package jittest

import (
	"context"
	"fmt"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// Input provides all information needed to run JiT tests for a PR.
type Input struct {
	PRNumber    int
	IssueNumber int
	WorkDir     string // worktree path
	BaseBranch  string // e.g. "main" or "feat/example"
	HeadBranch  string // e.g. "feat/ai-issue-42"
	Language    string // e.g. "go", "typescript", "python"
	TestCommand string // e.g. "go test ./..."
}

// GeneratedTest represents a single test file produced by the LLM.
type GeneratedTest struct {
	Filename string // e.g. "config_jittest_test.go"
	Content  string // complete test file content
	Target   string // target function name being tested
}

// TestOutcome represents the result of executing a single generated test.
type TestOutcome struct {
	Test   GeneratedTest
	Passed bool
	Output string // stdout+stderr from test execution
	Error  string // error message if failed
}

// Result holds the outcome of a complete JiTTest run.
type Result struct {
	Generated int           `json:"generated"`
	Passed    int           `json:"passed"`
	Failed    int           `json:"failed"`
	Skipped   int           `json:"skipped"`
	Tests     []TestOutcome `json:"tests,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
}

// Run executes the full JiTTest pipeline: generate → execute → cleanup → report.
// This is the top-level entry point called from the review pipeline.
// Returns a Result even on partial failure (e.g. LLM timeout returns skipped tests).
func Run(ctx context.Context, input Input, cfg analyzer.JiTTestConfig) (*Result, error) {
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("jittest is not enabled")
	}

	start := time.Now()

	// Step 1: Get diff
	diff, err := getDiffFunc(ctx, input.WorkDir, input.BaseBranch, input.HeadBranch)
	if err != nil {
		return &Result{Error: fmt.Sprintf("failed to get diff: %v", err), Duration: time.Since(start)}, nil
	}
	if diff == "" {
		return &Result{Error: "empty diff", Duration: time.Since(start)}, nil
	}

	// Step 2: Read source files referenced in diff
	sourceFiles := ReadSourceFiles(input.WorkDir, diff)

	// Step 3: Generate tests via LLM
	genInput := GenerateInput{
		Diff:        diff,
		SourceFiles: sourceFiles,
		Language:    input.Language,
		MaxTests:    cfg.MaxTests,
		Model:       cfg.Model,
		WorkDir:     input.WorkDir,
	}

	tests, err := GenerateTests(ctx, genInput)
	if err != nil {
		return &Result{
			Error:    fmt.Sprintf("generation failed: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// Step 4: Execute tests in worktree (cleanup is handled by RunTests)
	result := RunTests(ctx, input.WorkDir, tests, input.TestCommand)
	result.Duration = time.Since(start)

	return result, nil
}
