package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

func usageDispatchWorker() {
	fmt.Fprint(os.Stderr, `Dispatch an issue to a worker for execution

Usage:
  awkit dispatch-worker --issue <number> [options]

Options:
  --issue            Required: Issue number to dispatch
  --pr               Optional: PR number (used with --merge-issue to get base branch)
  --session          Optional: Principal session ID
  --state-root       Optional: Override state root (default: git root)
  --gh-timeout       Optional: GitHub API timeout (default: 30s)
  --worker-timeout   Optional: Worker execution timeout (default: 60m)
  --max-retries      Optional: Max retry count (default: 3)
  --merge-issue      Optional: Merge issue type (conflict | rebase)
  --json             Optional: Output as JSON instead of bash vars
  --help             Show this help

Output (bash eval compatible):
  WORKER_STATUS=<status>

Status values:
  success      - Worker completed successfully, PR created
  failed       - Worker failed (may be retried)
  in_progress  - Issue is already being processed

Examples:
  awkit dispatch-worker --issue 25
  awkit dispatch-worker --issue 25 --merge-issue conflict --pr 29
  awkit dispatch-worker --issue 25 --json
  eval "$(awkit dispatch-worker --issue 25)"

Notes:
  - This command runs the worker synchronously and waits for completion
  - Use --worker-timeout to limit execution time
  - Use --merge-issue with --pr to indicate Worker needs to fix merge issues
  - Cleanup is performed automatically on signal (SIGINT, SIGTERM)
`)
}

func cmdDispatchWorker(args []string) int {
	fs := flag.NewFlagSet("dispatch-worker", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageDispatchWorker

	issueNumber := fs.Int("issue", 0, "")
	prNumber := fs.Int("pr", 0, "")
	sessionID := fs.String("session", "", "")
	stateRoot := fs.String("state-root", "", "")
	ghTimeout := fs.Duration("gh-timeout", 30*time.Second, "")
	workerTimeout := fs.Duration("worker-timeout", 60*time.Minute, "")
	maxRetries := fs.Int("max-retries", 3, "")
	mergeIssue := fs.String("merge-issue", "", "")
	jsonOutput := fs.Bool("json", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageDispatchWorker()
		return 0
	}

	// Validate required arguments
	if *issueNumber == 0 {
		errorf("--issue is required\n")
		fmt.Fprintln(os.Stderr, "")
		usageDispatchWorker()
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

	// Set up cleanup manager for signal handling
	cleanupMgr := worker.NewCleanupManager()
	defer cleanupMgr.Cleanup()

	// Create GitHub client for cleanup
	ghClient := worker.NewGitHubClient(*ghTimeout)
	dispatchCleanup := worker.NewDispatchCleanup(*issueNumber, *stateRoot, ghClient)
	cleanupMgr.Register(dispatchCleanup.Run)

	// Run the dispatch
	ctx := cleanupMgr.Context()
	opts := worker.DispatchOptions{
		IssueNumber:        *issueNumber,
		PRNumber:           *prNumber,
		PrincipalSessionID: *sessionID,
		StateRoot:          *stateRoot,
		GHTimeout:          *ghTimeout,
		WorkerTimeout:      *workerTimeout,
		MaxRetries:         *maxRetries,
		MergeIssue:         *mergeIssue,
	}

	result, err := worker.DispatchWorker(ctx, opts)
	if err != nil {
		errorf("dispatch-worker failed: %v\n", err)
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

	// Return appropriate exit code
	switch result.Status {
	case "success":
		return 0
	case "in_progress":
		return 0 // Not an error, just already running
	case "failed":
		return 1
	default:
		return 0
	}
}
