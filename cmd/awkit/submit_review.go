package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/hooks"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
	"github.com/silver2dream/ai-workflow-kit/internal/util"
)

func usageSubmitReview() {
	fmt.Fprint(os.Stderr, `Submit PR review result

Usage (structured, preferred):
  awkit submit-review --pr <number> --issue <number> --ci-status <passed|failed> --body-file review.json

Usage (legacy markdown — DEPRECATED, scheduled for removal in v0.16):
  awkit submit-review --pr <number> --issue <number> --score <1-10> --ci-status <passed|failed> --body <review>

Arguments:
  --pr          PR number (required)
  --issue       Issue number (required)
  --ci-status   CI status: passed or failed (required)
  --body-file   Path to a structured review JSON file (score, criteria[],
                improvements[]; schema is printed by prepare-review).
                Score is taken from the file; --score must be omitted or match.
  --score       Review score 1-10 (required with --body)
  --body        Review body markdown (DEPRECATED: parsed by regex; use --body-file.
                Scheduled for removal in v0.16)

Options:
  --state-root  Override state root (default: git root)
  --help        Show this help

Exit codes:
  0  review submitted (see RESULT= line for the verdict)
  2  SUBMISSION INVALID — the JSON failed schema validation; the listed
     fields tell you exactly what to fix. Fix and resubmit in this session.
  1  operational failure (GitHub, config, ...)

Config (workflow.yaml):
  review.score_threshold  Minimum score to approve (default: 7)
  review.merge_strategy   Merge strategy: squash, merge, rebase (default: squash)

Examples:
  awkit submit-review --pr 42 --issue 25 --ci-status passed --body-file .ai/state/reviews/pr-42/review.json
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
	bodyFile := fs.String("body-file", "", "")
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

	if *ciStatus != "passed" && *ciStatus != "failed" {
		errorf("Error: --ci-status must be 'passed' or 'failed'\n\n")
		usageSubmitReview()
		return 2
	}

	// Structured path: parse + validate the JSON BEFORE any side effects so
	// a malformed submission is corrected in-session (exit 2), never burned
	// as a review_blocked round.
	var structured *reviewer.StructuredReview
	if *bodyFile != "" {
		data, err := os.ReadFile(*bodyFile)
		if err != nil {
			errorf("Error: cannot read --body-file: %v\n", err)
			return 2
		}
		parsed, verrs := reviewer.ParseStructuredReview(data)
		if len(verrs) > 0 {
			fmt.Println("SUBMISSION INVALID (fix the fields below and resubmit in this session):")
			for _, ve := range verrs {
				fmt.Printf("  - %s\n", ve.String())
			}
			return 2
		}
		if *score != 0 && *score != parsed.Score {
			fmt.Println("SUBMISSION INVALID (fix the fields below and resubmit in this session):")
			fmt.Printf("  - score: --score %d contradicts the file's \"score\": %d — omit --score or make them match\n", *score, parsed.Score)
			return 2
		}
		structured = parsed
		*score = parsed.Score
		*body = parsed.RenderMarkdown()
	} else {
		if *score < 1 || *score > 10 {
			errorf("Error: --score must be between 1 and 10\n\n")
			usageSubmitReview()
			return 2
		}
		if *body == "" {
			errorf("Error: --body or --body-file is required\n\n")
			usageSubmitReview()
			return 2
		}
		fmt.Fprintln(os.Stderr, "[submit-review] WARNING: the --body markdown path is deprecated and scheduled for removal in v0.16 — submit a structured review via --body-file instead")
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

	// Managed migration: make every use of the legacy --body path observable
	// so the v0.16 removal decision is grounded in real usage data.
	if *bodyFile == "" {
		trace.WriteEvent(trace.ComponentReviewer, trace.TypeDeprecatedPath, trace.LevelWarn,
			trace.WithData(map[string]any{
				"path":    "submit-review --body",
				"removal": "v0.16",
				"pr":      *prNumber,
			}))
	}

	// Load review settings from workflow.yaml
	reviewSettings := reviewer.GetReviewSettings(*stateRoot)

	// Load hooks from config
	var hookRunner *hooks.HookRunner
	configPath := filepath.Join(*stateRoot, ".ai", "config", "workflow.yaml")
	if cfg, err := analyzer.LoadConfig(configPath); err == nil {
		hookRunner = hooks.NewHookRunner(cfg.Hooks, *stateRoot, os.Stderr)
	}

	// Run Go implementation
	ctx := context.Background()
	result, err := reviewer.SubmitReview(ctx, reviewer.SubmitReviewOptions{
		PRNumber:       *prNumber,
		IssueNumber:    *issueNumber,
		Score:          *score,
		CIStatus:       *ciStatus,
		ReviewBody:     *body,
		StateRoot:      *stateRoot,
		ScoreThreshold: reviewSettings.ScoreThreshold,
		MergeStrategy:  reviewSettings.MergeStrategy,
		GHTimeout:      60 * time.Second,
		HookRunner:     hookRunner,
		Structured:     structured,
	})

	if err != nil {
		errorf("submit-review failed: %v\n", err)
		return 1
	}

	// Output result
	fmt.Printf("RESULT=%s\n", result.Result)
	if result.Reason != "" {
		fmt.Printf("REASON=%s\n", util.ShellSafe(result.Reason))
	}

	return 0
}
