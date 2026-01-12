package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/trace"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

func usageRunIssue() {
	fmt.Fprint(os.Stderr, `Run a worker for a single issue

Usage:
  awkit run-issue --issue <number> --ticket <file> [options]
  awkit run-issue <issue_id> <ticket_file> [repo]

Options:
  --issue         Required: Issue number
  --ticket        Required: Ticket file path
  --repo          Optional: Repo override (root/backend/frontend)
  --state-root    Optional: Override state root (default: git root)
  --gh-timeout    Optional: GitHub CLI timeout (default: 60s)
  --git-timeout   Optional: Git timeout (default: 120s)
  --codex-timeout Optional: Codex timeout (default: 30m)
  --merge-issue   Optional: Merge issue type (conflict/rebase)
  --pr-number     Optional: PR number for merge issue resolution
  --json          Optional: Output result as JSON
  --help          Show this help

Examples:
  awkit run-issue --issue 25 --ticket .ai/temp/ticket-25.md
  awkit run-issue 25 .ai/temp/ticket-25.md backend
  awkit run-issue --issue 25 --ticket .ai/temp/ticket-25.md --merge-issue conflict --pr-number 30
`)
}

func cmdRunIssue(args []string) int {
	fs := flag.NewFlagSet("run-issue", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageRunIssue

	issueNumber := fs.Int("issue", 0, "")
	ticketFile := fs.String("ticket", "", "")
	repo := fs.String("repo", "", "")
	stateRoot := fs.String("state-root", "", "")
	ghTimeout := fs.Duration("gh-timeout", 60*time.Second, "")
	gitTimeout := fs.Duration("git-timeout", 120*time.Second, "")
	codexTimeout := fs.Duration("codex-timeout", 30*time.Minute, "")
	mergeIssue := fs.String("merge-issue", "", "")
	prNumber := fs.Int("pr-number", 0, "")
	jsonOutput := fs.Bool("json", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageRunIssue()
		return 0
	}

	remaining := fs.Args()
	if *issueNumber == 0 && len(remaining) > 0 {
		if value, err := strconv.Atoi(remaining[0]); err == nil {
			*issueNumber = value
		}
	}
	if *ticketFile == "" && len(remaining) > 1 {
		*ticketFile = remaining[1]
	}
	if *repo == "" && len(remaining) > 2 {
		*repo = remaining[2]
	}

	if *issueNumber == 0 || *ticketFile == "" {
		errorf("--issue and --ticket are required\n")
		fmt.Fprintln(os.Stderr, "")
		usageRunIssue()
		return 2
	}

	if *stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
		*stateRoot = root
	}

	// Initialize event writer for tracing
	eventSessionID := readCurrentPrincipalSession(*stateRoot)
	if eventSessionID != "" {
		if err := trace.InitGlobalWriter(*stateRoot, eventSessionID); err != nil {
			fmt.Fprintf(os.Stderr, "[run-issue] warning: failed to init event writer: %v\n", err)
		} else {
			defer trace.CloseGlobalWriter()
		}
	}

	ctx := context.Background()
	result, err := worker.RunIssue(ctx, worker.RunIssueOptions{
		IssueID:      *issueNumber,
		TicketFile:   *ticketFile,
		RepoOverride: *repo,
		StateRoot:    *stateRoot,
		GHTimeout:    *ghTimeout,
		GitTimeout:   *gitTimeout,
		CodexTimeout: *codexTimeout,
		MergeIssue:   *mergeIssue,
		PRNumber:     *prNumber,
	})
	if err != nil && result == nil {
		errorf("run-issue failed: %v\n", err)
		return 1
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			errorf("failed to encode JSON: %v\n", err)
			return 1
		}
	} else if result != nil {
		fmt.Fprintf(os.Stdout, "STATUS=%s\n", result.Status)
		if result.PRURL != "" {
			fmt.Fprintf(os.Stdout, "PR_URL=%s\n", result.PRURL)
		}
	}

	if result != nil && result.ExitCode != 0 {
		return result.ExitCode
	}
	if err != nil {
		return 1
	}
	return 0
}
