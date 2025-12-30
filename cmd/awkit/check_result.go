package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

func usageCheckResult() {
	fmt.Fprint(os.Stderr, `Check worker execution result for an issue

Usage:
  awkit check-result --issue <number> [options]

Options:
  --issue         Required: Issue number to check
  --session       Optional: Principal session ID
  --state-root    Optional: Override state root (default: git root)
  --max-retries   Optional: Max retry count (default: 3)
  --timeout       Optional: GitHub API timeout (default: 30s)
  --worker-timeout Optional: Max worker runtime (default: 30m)
  --wait          Optional: Wait duration when worker running (default: 30s)
  --json          Optional: Output as JSON instead of bash vars
  --help          Show this help

Output (bash eval compatible):
  CHECK_RESULT_STATUS=<status>
  WORKER_STATUS=<status>
  PR_NUMBER=<number>

Status values:
  success           - Worker completed successfully, PR created
  failed_will_retry - Worker failed but will be retried
  failed_max_retries - Worker failed, max retries exceeded
  crashed           - Worker process terminated unexpectedly
  timeout           - Worker exceeded timeout
  not_found         - Result not ready, worker may still be running

Examples:
  awkit check-result --issue 25
  awkit check-result --issue 25 --json
  eval "$(awkit check-result --issue 25)"
`)
}

func cmdCheckResult(args []string) int {
	fs := flag.NewFlagSet("check-result", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageCheckResult

	issueNumber := fs.Int("issue", 0, "")
	sessionID := fs.String("session", "", "")
	stateRoot := fs.String("state-root", "", "")
	maxRetries := fs.Int("max-retries", 3, "")
	timeout := fs.Duration("timeout", 30*time.Second, "")
	workerTimeout := fs.Duration("worker-timeout", 30*time.Minute, "")
	waitDuration := fs.Duration("wait", 30*time.Second, "")
	jsonOutput := fs.Bool("json", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageCheckResult()
		return 0
	}

	// Validate required arguments
	if *issueNumber == 0 {
		errorf("--issue is required\n")
		fmt.Fprintln(os.Stderr, "")
		usageCheckResult()
		return 2
	}

	// Resolve state root if not provided
	if *stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
		*stateRoot = root
	}

	// Run the check
	ctx := context.Background()
	opts := worker.CheckResultOptions{
		IssueNumber:        *issueNumber,
		PrincipalSessionID: *sessionID,
		StateRoot:          *stateRoot,
		MaxRetries:         *maxRetries,
		GHTimeout:          *timeout,
		WorkerTimeout:      *workerTimeout,
		WaitDuration:       *waitDuration,
	}

	result, err := worker.CheckResult(ctx, opts)
	if err != nil {
		errorf("check-result failed: %v\n", err)
		return 1
	}

	// Output result
	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			errorf("failed to encode JSON: %v\n", err)
			return 1
		}
	} else {
		fmt.Print(result.FormatBashOutput())
	}

	// Return appropriate exit code based on status
	switch result.Status {
	case "success":
		return 0
	case "not_found":
		return 0 // Not an error, just waiting
	case "failed_will_retry":
		return 0 // Retry is expected
	case "crashed", "timeout", "failed_max_retries":
		return 1
	default:
		return 0
	}
}

// resolveGitRoot finds the git repository root
func resolveGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
