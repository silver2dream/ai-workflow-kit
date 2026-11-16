package jittest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/util"
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

	// Write test files to worktree. The file names come from a model that
	// read the diff, so each is checked before anything is written.
	writtenAs := make(map[int]string, len(tests))
	for i, test := range tests {
		fullPath, pathErr := jitTestPath(workDir, test.Filename)
		if pathErr != nil {
			result.Skipped++
			result.Tests = append(result.Tests, TestOutcome{
				Test:   test,
				Passed: false,
				Error:  fmt.Sprintf("refused to write %q: %v", test.Filename, pathErr),
			})
			continue
		}

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
		writtenAs[i] = fullPath
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
		for i, test := range tests {
			if _, wasWritten := writtenAs[i]; !wasWritten {
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

// jitTestPath returns where a generated test is written, or why it is not.
//
// The name must stay inside the worktree (also once symlinks are followed),
// keep out of dot-directories such as .git and .github, hold "jittest" so
// the cleanup finds the file, and name a file that does not exist yet: the
// cleanup deletes whatever was written.
func jitTestPath(workDir, filename string) (string, error) {
	name := filepath.FromSlash(strings.TrimSpace(filename))
	if name == "" || filepath.IsAbs(name) || filepath.VolumeName(name) != "" || strings.HasPrefix(name, string(filepath.Separator)) {
		return "", fmt.Errorf("not a path inside the worktree")
	}
	full := filepath.Join(workDir, name)
	rel, err := filepath.Rel(workDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("the path leaves the worktree")
	}
	segments := strings.Split(rel, string(filepath.Separator))
	for _, segment := range segments[:len(segments)-1] {
		if strings.HasPrefix(segment, ".") {
			return "", fmt.Errorf("tests do not go into %s", segment)
		}
	}
	if !strings.Contains(filepath.Base(full), "jittest") {
		return "", fmt.Errorf(`the file name must contain "jittest"`)
	}
	if _, err := os.Lstat(full); err == nil {
		return "", fmt.Errorf("the file already exists")
	}

	// A directory of the worktree may be a link to somewhere else.
	realRoot, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return "", fmt.Errorf("the worktree cannot be resolved: %w", err)
	}
	ancestor := filepath.Dir(full)
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		ancestor = parent
	}
	realAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("the directory cannot be resolved: %w", err)
	}
	if inside, err := filepath.Rel(realRoot, realAncestor); err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("the directory is a link out of the worktree")
	}
	return full, nil
}

// inAnyDir reports whether a file name lies inside one of the directories.
// Both use forward slashes and are relative to the worktree.
func inAnyDir(filename string, dirs []string) bool {
	name := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(filename))))
	for _, dir := range dirs {
		if dir == "." || strings.HasPrefix(name, strings.TrimSuffix(dir, "/")+"/") {
			return true
		}
	}
	return false
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

	// When the time is up the command is stopped together with the test
	// binaries it started: they hold its output open, and the wait went on
	// until they were done. One that escapes is waited for this much longer.
	cmd.WaitDelay = 5 * time.Second
	err := util.RunWithChildren(cmd)
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

// csharpCompileError is a C# compiler diagnostic, as in "error CS0103: ...".
var csharpCompileError = regexp.MustCompile(`\berror CS\d{4}\b`)

// isCompileError checks if the output indicates a compilation failure.
func isCompileError(output string) bool {
	if csharpCompileError.MatchString(output) {
		return true
	}
	indicators := []string{
		"build failed",
		"compilation failed",
		"cannot find package",
		"could not import",
		"syntax error",
		"does not compile",
		"TS2", // TypeScript error codes
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
