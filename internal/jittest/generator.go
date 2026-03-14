package jittest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GenerateInput provides everything needed for LLM-based test generation.
type GenerateInput struct {
	Diff        string            // unified diff content
	SourceFiles map[string]string // filepath → content (diff-referenced source files, no tests)
	Language    string            // "go", "typescript", "python", etc.
	MaxTests    int               // max number of tests to generate
	Model       string            // LLM model ID
	WorkDir     string            // worktree root
}

// claudeRunner is a function variable for invoking the Claude CLI.
// Replaced in tests to avoid real LLM calls.
var claudeRunner = runClaude

// GenerateTests invokes the Claude CLI to generate JiT tests from a PR diff.
// It writes a prompt to a temp file, passes it via stdin to `claude --print`,
// then parses the output into []GeneratedTest.
func GenerateTests(ctx context.Context, input GenerateInput) ([]GeneratedTest, error) {
	if input.Diff == "" {
		return nil, fmt.Errorf("empty diff, nothing to generate tests for")
	}

	prompt := buildPrompt(input.Diff, input.SourceFiles, input.Language, input.MaxTests)

	output, err := claudeRunner(ctx, prompt, input.Model, input.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("claude CLI failed: %w", err)
	}

	tests := parseGeneratedTests(output)
	if len(tests) == 0 {
		return nil, fmt.Errorf("no tests parsed from LLM output")
	}

	return tests, nil
}

// GetDiff runs git diff to obtain the unified diff between base and head branches.
func GetDiff(ctx context.Context, workDir, baseBranch, headBranch string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", baseBranch+"..."+headBranch)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	return string(out), nil
}

// ReadSourceFiles reads the content of diff-referenced source files from the worktree.
// Test files (*_test.go, *.test.ts, etc.) are excluded to maintain independence.
func ReadSourceFiles(workDir string, diffContent string) map[string]string {
	files := parseDiffFiles(diffContent)
	sources := make(map[string]string, len(files))

	for _, f := range files {
		fullPath := filepath.Join(workDir, f)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue // skip unreadable files
		}
		// Limit file size to avoid huge prompts (128KB per file)
		if len(content) > 128*1024 {
			content = content[:128*1024]
		}
		sources[f] = string(content)
	}

	return sources
}

// runClaude invokes `claude --print --model <model> --max-turns 1` with the prompt via stdin.
func runClaude(ctx context.Context, prompt, model, workDir string) (string, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return "", fmt.Errorf("claude CLI not found in PATH")
	}

	args := []string{
		"--print",
		"--model", model,
		"--max-turns", "1",
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude CLI timed out")
		}
		return "", fmt.Errorf("claude exited with error: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
