package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

func usageSubmitReview() {
	fmt.Fprint(os.Stderr, `Submit PR review result

Usage:
  awkit submit-review --pr <number> --issue <number> --score <1-10> --ci-status <passed|failed> --body <review>

Arguments:
  --pr          PR number (required)
  --issue       Issue number (required)
  --score       Review score 1-10 (required, >= 7 approves)
  --ci-status   CI status: passed or failed (required)
  --body        Review body text (required)

Options:
  --state-root  Override state root (default: git root)
  --help        Show this help

Examples:
  awkit submit-review --pr 42 --issue 25 --score 8 --ci-status passed --body "LGTM. EVIDENCE: func NewHandler"
  awkit submit-review --pr 42 --issue 25 --score 5 --ci-status failed --body "Needs work"
`)
}

func cmdSubmitReview(args []string) int {
	fs := flag.NewFlagSet("submit-review", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageSubmitReview

	prNumber := fs.Int("pr", 0, "")
	issueNumber := fs.Int("issue", 0, "")
	score := fs.Int("score", 0, "")
	ciStatus := fs.String("ci-status", "", "")
	body := fs.String("body", "", "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageSubmitReview()
		return 0
	}

	// Validate inputs
	if *prNumber <= 0 {
		errorf("Error: --pr is required and must be positive\n\n")
		usageSubmitReview()
		return 2
	}

	if *issueNumber <= 0 {
		errorf("Error: --issue is required and must be positive\n\n")
		usageSubmitReview()
		return 2
	}

	if *score < 1 || *score > 10 {
		errorf("Error: --score must be between 1 and 10\n\n")
		usageSubmitReview()
		return 2
	}

	if *ciStatus != "passed" && *ciStatus != "failed" {
		errorf("Error: --ci-status must be 'passed' or 'failed'\n\n")
		usageSubmitReview()
		return 2
	}

	if *body == "" {
		errorf("Error: --body is required\n\n")
		usageSubmitReview()
		return 2
	}

	// Resolve state root
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
			fmt.Fprintf(os.Stderr, "[submit-review] warning: failed to init event writer: %v\n", err)
		} else {
			defer trace.CloseGlobalWriter()
		}
	}

	// Check for script fallback
	if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
		return runSubmitReviewScript(*stateRoot, *prNumber, *issueNumber, *score, *ciStatus, *body)
	}

	// Run Go implementation
	ctx := context.Background()
	result, err := reviewer.SubmitReview(ctx, reviewer.SubmitReviewOptions{
		PRNumber:    *prNumber,
		IssueNumber: *issueNumber,
		Score:       *score,
		CIStatus:    *ciStatus,
		ReviewBody:  *body,
		StateRoot:   *stateRoot,
		GHTimeout:   60 * time.Second,
	})

	if err != nil {
		errorf("submit-review failed: %v\n", err)
		return 1
	}

	// Output result
	fmt.Printf("RESULT=%s\n", result.Result)
	if result.Reason != "" {
		fmt.Printf("REASON=%s\n", result.Reason)
	}

	return 0
}

func runSubmitReviewScript(stateRoot string, prNumber, issueNumber, score int, ciStatus, body string) int {
	scriptPath := filepath.Join(stateRoot, ".ai/scripts/submit_review.sh")
	cmd := exec.Command("bash", scriptPath,
		strconv.Itoa(prNumber),
		strconv.Itoa(issueNumber),
		strconv.Itoa(score),
		ciStatus,
		body)
	cmd.Dir = stateRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
