package jittest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunTests writes generated tests to the worktree, executes the test command,
// collects results, and cleans up all JiT test files.
// Cleanup is guaranteed via defer even on panic.
func RunTests(ctx context.Context, workDir string, tests []GeneratedTest, testCommand string) *Result {
	start := time.Now()
	result := &Result{
		Generated: len(tests),
	}

	if len(tests) == 0 {
		return result
	}

	// Track written files for cleanup
	var writtenFiles []string
	defer func() {
		cleanupJiTFiles(workDir, writtenFiles)
	}()

	// Write test files to worktree
	for _, test := range tests {
		fullPath := filepath.Join(workDir, test.Filename)

		// Create parent directory if needed
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			result.Skipped++
			result.Tests = append(result.Tests, TestOutcome{
				Test:   test,
				Passed: false,
				Error:  fmt.Sprintf("failed to create directory: %v", err),
			})
			continue
		}

		if err := os.WriteFile(fullPath, []byte(test.Content), 0644); err != nil {
			result.Skipped++
			result.Tests = append(result.Tests, TestOutcome{
				Test:   test,
				Passed: false,
				Error:  fmt.Sprintf("failed to write file: %v", err),
			})
			continue
		}
		writtenFiles = append(writtenFiles, fullPath)
	}

	if len(writtenFiles) == 0 {
		result.Error = "all test files failed to write"
		result.Duration = time.Since(start)
		return result
	}

	// Execute test command
	outcome := executeTests(ctx, workDir, testCommand, writtenFiles)

	// Map outcomes to results
	if outcome.compileError {
		// Compilation failure means generated tests have issues — skip all
		result.Skipped = len(tests)
		result.Error = "compilation failed: " + outcome.stderr
		result.Tests = nil // clear any partial results
	} else {
		// Parse individual test results from output
		for _, test := range tests {
			wasWritten := false
			for _, wf := range writtenFiles {
				if filepath.Join(workDir, test.Filename) == wf {
					wasWritten = true
					break
				}
			}
			if !wasWritten {
				continue // already counted as skipped
			}

			passed := !outcome.failed && outcome.exitCode == 0
			to := TestOutcome{
				Test:   test,
				Passed: passed,
				Output: outcome.stdout,
			}
			if !passed {
				to.Error = extractTestError(outcome.stdout+outcome.stderr, test.Filename)
			}
			result.Tests = append(result.Tests, to)

			if passed {
				result.Passed++
			} else {
				result.Failed++
			}
		}
	}

	result.Duration = time.Since(start)
	return result
}

// testExecOutcome holds raw execution results.
type testExecOutcome struct {
	stdout       string
	stderr       string
	exitCode     int
	failed       bool
	compileError bool
	timedOut     bool
}

// executeTests runs the test command in the worktree.
var executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
	// Build command — use shell to support complex commands like "go test ./..."
	cmd := exec.CommandContext(ctx, "sh", "-c", testCommand)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outcome := testExecOutcome{
		stdout: stdout.String(),
		stderr: stderr.String(),
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			outcome.timedOut = true
			outcome.failed = true
			return outcome
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			outcome.exitCode = exitErr.ExitCode()
		} else {
			outcome.exitCode = 1
		}
		outcome.failed = true

		// Detect compilation errors
		combined := outcome.stdout + outcome.stderr
		if isCompileError(combined) {
			outcome.compileError = true
		}
	}

	return outcome
}

// cleanupJiTFiles removes all written JiT test files.
// Also does a glob sweep for any *jittest* files that might have been missed.
func cleanupJiTFiles(workDir string, writtenFiles []string) {
	// Remove explicitly tracked files
	for _, f := range writtenFiles {
		os.Remove(f)
	}

	// Safety sweep: remove any *jittest* files that might remain
	filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			// Skip .git and node_modules
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == ".ai" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(filepath.Base(path), "jittest") {
			os.Remove(path)
		}
		return nil
	})
}

// isCompileError checks if the output indicates a compilation failure.
func isCompileError(output string) bool {
	indicators := []string{
		"build failed",
		"compilation failed",
		"cannot find package",
		"could not import",
		"syntax error",
		"does not compile",
		"TS2",  // TypeScript error codes
		"SyntaxError:",
		"ModuleNotFoundError:",
	}
	lower := strings.ToLower(output)
	for _, ind := range indicators {
		if strings.Contains(lower, strings.ToLower(ind)) {
			return true
		}
	}
	return false
}

// extractTestError extracts a relevant error message for a specific test file from output.
func extractTestError(output, filename string) string {
	base := filepath.Base(filename)
	lines := strings.Split(output, "\n")

	// Look for lines mentioning the test file
	var relevant []string
	for _, line := range lines {
		if strings.Contains(line, base) && (strings.Contains(line, "FAIL") || strings.Contains(line, "Error") || strings.Contains(line, "error")) {
			relevant = append(relevant, strings.TrimSpace(line))
		}
	}

	if len(relevant) > 0 {
		// Return first few relevant lines
		if len(relevant) > 3 {
			relevant = relevant[:3]
		}
		return strings.Join(relevant, "; ")
	}

	// Fallback: return last non-empty line of output
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			return trimmed
		}
	}

	return "test failed (no details)"
}
