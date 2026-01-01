package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/clean"
	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
)

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fs.Usage = usageDoctor

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd, _ := os.Getwd()
	doc := doctor.New(cwd)

	fmt.Println("AWK Health Check")
	fmt.Println("================")
	fmt.Println()

	ctx := context.Background()
	results := doc.RunAll(ctx)

	var warnings, errors int
	var cleanable []string

	for _, r := range results {
		var status string
		switch r.Status {
		case "ok":
			status = colorGreen + "OK" + colorReset
		case "warning":
			status = colorYellow + "WARNING" + colorReset
			warnings++
		case "error":
			status = colorRed + "ERROR" + colorReset
			errors++
		}

		fmt.Printf("[%s] %s: %s\n", status, r.Name, r.Message)

		if r.CanClean && r.CleanKey != "" {
			cleanable = append(cleanable, r.CleanKey)
		}
	}

	fmt.Println()

	if errors > 0 {
		fmt.Printf("%sFound %d error(s)%s\n", colorRed, errors, colorReset)
	}
	if warnings > 0 {
		fmt.Printf("%sFound %d warning(s)%s\n", colorYellow, warnings, colorReset)
	}
	if errors == 0 && warnings == 0 {
		fmt.Printf("%sAll checks passed!%s\n", colorGreen, colorReset)
	}

	if len(cleanable) > 0 {
		fmt.Println()
		fmt.Println("To clean up, run:")
		fmt.Printf("  awkit clean\n")
	}

	if errors > 0 {
		return 1
	}
	return 0
}

func cmdClean(args []string) int {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Show what would be cleaned without making changes")
	all := fs.Bool("all", false, "Clean all state including results")
	state := fs.Bool("state", false, "Clean state files (loop_count, consecutive_failures)")
	attempts := fs.Bool("attempts", false, "Clean attempt tracking files")
	stop := fs.Bool("stop", false, "Remove STOP marker")
	lock := fs.Bool("lock", false, "Remove lock file")
	deprecated := fs.Bool("deprecated", false, "Remove deprecated files")
	results := fs.Bool("results", false, "Clean result files")
	resetLabels := fs.Bool("reset-labels", false, "Reset review-failed labels to pr-ready")
	fs.Usage = usageClean

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// If no specific flags, default to common cleanup
	noFlags := !*state && !*attempts && !*stop && !*lock && !*deprecated && !*results && !*resetLabels && !*all

	cwd, _ := os.Getwd()
	cleaner := clean.New(cwd)
	cleaner.SetDryRun(*dryRun)

	if *dryRun {
		fmt.Println("Dry run mode - no changes will be made")
		fmt.Println()
	}

	fmt.Println("AWK Clean")
	fmt.Println("=========")
	fmt.Println()

	ctx := context.Background()
	var allResults []clean.CleanResult

	if *all || *state || noFlags {
		allResults = append(allResults, cleaner.CleanState()...)
	}
	if *all || *attempts || noFlags {
		allResults = append(allResults, cleaner.CleanAttempts()...)
	}
	if *all || *stop || noFlags {
		allResults = append(allResults, cleaner.CleanStop())
	}
	if *all || *lock {
		allResults = append(allResults, cleaner.CleanLock())
	}
	if *all || *deprecated || noFlags {
		allResults = append(allResults, cleaner.CleanDeprecated()...)
	}
	if *all || *results {
		allResults = append(allResults, cleaner.CleanResults()...)
	}
	if *resetLabels {
		allResults = append(allResults, cleaner.ResetGitHubLabel(ctx, "review-failed", "pr-ready")...)
	}

	var success, failed int
	for _, r := range allResults {
		var status string
		if r.Success {
			status = colorGreen + "OK" + colorReset
			success++
		} else {
			status = colorRed + "FAILED" + colorReset
			failed++
		}
		fmt.Printf("[%s] %s: %s\n", status, r.Name, r.Message)
	}

	if len(allResults) == 0 {
		fmt.Println("Nothing to clean.")
	} else {
		fmt.Println()
		if failed > 0 {
			fmt.Printf("%s%d failed%s, ", colorRed, failed, colorReset)
		}
		fmt.Printf("%s%d cleaned%s\n", colorGreen, success, colorReset)
	}

	if failed > 0 {
		return 1
	}
	return 0
}

func usageDoctor() {
	fmt.Fprint(os.Stderr, `Check AWK project health and identify issues

This command performs health checks on your AWK project and reports:
- Local state files that may need cleanup
- GitHub labels that indicate stalled work
- Deprecated files that should be removed

Usage:
  awkit doctor

Examples:
  awkit doctor
`)
}

func usageClean() {
	fmt.Fprint(os.Stderr, `Clean up AWK project state

This command cleans up state files to allow a fresh start.
Without flags, it cleans common state (loop_count, consecutive_failures,
attempts, STOP marker, deprecated files).

Usage:
  awkit clean [options]

Options:
  --dry-run       Show what would be cleaned without making changes
  --all           Clean all state including results and lock
  --state         Clean state files (loop_count, consecutive_failures)
  --attempts      Clean attempt tracking files
  --stop          Remove STOP marker
  --lock          Remove lock file (use with caution)
  --deprecated    Remove deprecated files
  --results       Clean result files
  --reset-labels  Reset review-failed labels to pr-ready on GitHub

Examples:
  awkit clean                  # Clean common state
  awkit clean --dry-run        # Preview what would be cleaned
  awkit clean --all            # Clean everything
  awkit clean --reset-labels   # Reset stuck review labels
`)
}
