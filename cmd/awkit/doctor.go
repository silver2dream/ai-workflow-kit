package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
	"github.com/silver2dream/ai-workflow-kit/internal/reset"
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
	var resettable bool

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

		if r.CanClean {
			resettable = true
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

	if resettable {
		fmt.Println()
		fmt.Println("To reset state, run:")
		fmt.Printf("  awkit reset\n")
	}

	if errors > 0 {
		return 1
	}
	return 0
}

func cmdReset(args []string) int {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Show what would be reset without making changes")
	all := fs.Bool("all", false, "Reset all state including results")
	state := fs.Bool("state", false, "Reset state files (loop_count, consecutive_failures)")
	attempts := fs.Bool("attempts", false, "Reset attempt tracking files")
	stop := fs.Bool("stop", false, "Remove STOP marker")
	lock := fs.Bool("lock", false, "Remove lock file")
	deprecated := fs.Bool("deprecated", false, "Remove deprecated files")
	results := fs.Bool("results", false, "Reset result files")
	traces := fs.Bool("traces", false, "Reset old trace files (deprecated, use events)")
	events := fs.Bool("events", false, "Reset event stream files")
	labels := fs.Bool("labels", false, "Reset review-failed labels to pr-ready on GitHub")
	fs.Usage = usageReset

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// If no specific flags, default to common reset
	noFlags := !*state && !*attempts && !*stop && !*lock && !*deprecated && !*results && !*traces && !*events && !*labels && !*all

	cwd, _ := os.Getwd()
	resetter := reset.New(cwd)
	resetter.SetDryRun(*dryRun)

	if *dryRun {
		fmt.Println("Dry run mode - no changes will be made")
		fmt.Println()
	}

	fmt.Println("AWK Reset")
	fmt.Println("=========")
	fmt.Println()

	ctx := context.Background()
	var allResults []reset.Result

	if *all || *state || noFlags {
		allResults = append(allResults, resetter.ResetState()...)
	}
	if *all || *attempts || noFlags {
		allResults = append(allResults, resetter.ResetAttempts()...)
	}
	if *all || *stop || noFlags {
		allResults = append(allResults, resetter.ResetStop())
	}
	if *all || *lock {
		allResults = append(allResults, resetter.ResetLock())
	}
	if *all || *deprecated || noFlags {
		allResults = append(allResults, resetter.ResetDeprecated()...)
	}
	if *all || *results {
		allResults = append(allResults, resetter.Results()...)
	}
	if *all || *traces {
		allResults = append(allResults, resetter.ResetTraces()...)
	}
	if *all || *events {
		allResults = append(allResults, resetter.ResetEvents()...)
	}
	if *labels {
		allResults = append(allResults, resetter.ResetGitHubLabel(ctx, "review-failed", "pr-ready")...)
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
		fmt.Println("Nothing to reset.")
	} else {
		fmt.Println()
		if failed > 0 {
			fmt.Printf("%s%d failed%s, ", colorRed, failed, colorReset)
		}
		fmt.Printf("%s%d reset%s\n", colorGreen, success, colorReset)
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

func usageReset() {
	fmt.Fprint(os.Stderr, `Reset AWK project state for a fresh start

This command resets state files to allow a fresh start.
Without flags, it resets common state (loop_count, consecutive_failures,
attempts, STOP marker, deprecated files).

Usage:
  awkit reset [options]

Options:
  --dry-run     Show what would be reset without making changes
  --all         Reset all state including results, traces, events, and lock
  --state       Reset state files (loop_count, consecutive_failures)
  --attempts    Reset attempt tracking files
  --stop        Remove STOP marker
  --lock        Remove lock file (use with caution)
  --deprecated  Remove deprecated files
  --results     Reset result files
  --traces      Reset old trace files (.ai/state/traces/)
  --events      Reset event stream files (.ai/state/events/)
  --labels      Reset review-failed labels to pr-ready on GitHub

Examples:
  awkit reset              # Reset common state
  awkit reset --dry-run    # Preview what would be reset
  awkit reset --all        # Reset everything (including traces and events)
  awkit reset --traces     # Clean old trace files after upgrade
  awkit reset --labels     # Reset stuck review labels on GitHub

Note: To fix missing permissions, use 'awkit upgrade' instead.
`)
}
