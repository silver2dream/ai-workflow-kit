package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
)

func usageNext() {
	fmt.Fprint(os.Stderr, `Show suggested next actions (offline)

Usage:
  awkit next [options]

Options:
  --issue <id>  Suggest next actions for a specific issue
  --json        Output JSON

Examples:
  awkit next
  awkit next --issue 42
`)
}

func cmdNext(args []string) int {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageNext

	issueID := fs.Int("issue", 0, "")
	jsonOut := fs.Bool("json", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageNext()
		return 0
	}

	output := kickoff.NewOutputFormatter(os.Stdout)

	report, err := status.Collect(".", status.Options{IssueID: *issueID})
	if err != nil {
		output.Error(fmt.Sprintf("Failed to collect status: %v", err))
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"timestamp_utc": report.TimestampUTC,
			"target":        report.Target,
			"suggestions":   report.Suggestions,
			"warnings":      report.Warnings,
		})
		return 0
	}

	if len(report.Warnings) > 0 {
		output.Warning("Warnings:")
		for _, w := range report.Warnings {
			fmt.Printf("  - %s\n", w)
		}
		fmt.Println("")
	}

	if len(report.Suggestions) == 0 {
		fmt.Println("No suggestions available.")
		return 0
	}

	fmt.Println("Next:")
	for _, s := range report.Suggestions {
		fmt.Printf("  - %s\n", s)
	}
	return 0
}
