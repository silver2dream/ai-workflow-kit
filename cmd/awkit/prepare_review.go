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

func usagePrepareReview() {
	fmt.Fprint(os.Stderr, `Prepare PR review context

Usage:
  awkit prepare-review --pr <number> --issue <number>

Arguments:
  --pr       PR number (required)
  --issue    Issue number (required)

Options:
  --state-root    Override state root (default: git root)
  --json          Output as JSON instead of formatted text
  --help          Show this help

Examples:
  awkit prepare-review --pr 42 --issue 25
  awkit prepare-review --pr 42 --issue 25 --json
`)
}

func cmdPrepareReview(args []string) int {
	fs := flag.NewFlagSet("prepare-review", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usagePrepareReview

	prNumber := fs.Int("pr", 0, "")
	issueNumber := fs.Int("issue", 0, "")
	stateRoot := fs.String("state-root", "", "")
	jsonOutput := fs.Bool("json", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usagePrepareReview()
		return 0
	}

	if *prNumber <= 0 {
		errorf("Error: --pr is required and must be positive\n\n")
		usagePrepareReview()
		return 2
	}

	if *issueNumber <= 0 {
		errorf("Error: --issue is required and must be positive\n\n")
		usagePrepareReview()
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
			fmt.Fprintf(os.Stderr, "[prepare-review] warning: failed to init event writer: %v\n", err)
		} else {
			defer trace.CloseGlobalWriter()
		}
	}

	// Check for script fallback
	if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
		return runPrepareReviewScript(*stateRoot, *prNumber, *issueNumber)
	}

	// Run Go implementation
	ctx := context.Background()
	result, err := reviewer.PrepareReview(ctx, reviewer.PrepareReviewOptions{
		PRNumber:    *prNumber,
		IssueNumber: *issueNumber,
		StateRoot:   *stateRoot,
		GHTimeout:   60 * time.Second,
	})

	if err != nil {
		errorf("prepare-review failed: %v\n", err)
		return 1
	}

	// Output result
	if *jsonOutput {
		jsonStr, err := result.ToJSON()
		if err != nil {
			errorf("failed to encode JSON: %v\n", err)
			return 1
		}
		fmt.Println(jsonStr)
	} else {
		fmt.Print(result.FormatOutput())
	}

	return 0
}

func runPrepareReviewScript(stateRoot string, prNumber, issueNumber int) int {
	scriptPath := filepath.Join(stateRoot, ".ai/scripts/prepare_review.sh")
	cmd := exec.Command("bash", scriptPath, strconv.Itoa(prNumber), strconv.Itoa(issueNumber))
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
