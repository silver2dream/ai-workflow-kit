package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/evaluate"
)

func usageEvaluate() {
	fmt.Fprint(os.Stderr, `Usage: awkit evaluate [options]

Run the AWK evaluation gates against a project.

Options:
  --offline     Run offline gates only (no network required); online
                gates are counted as skipped
  --strict      Exit non-zero when any executed gate FAILs
                (e.g. P0 audit findings). Without --strict the command
                is report-only and always exits 0.
  --path        Project root to evaluate (default: current directory)
  --help, -h    Show this help

Examples:
  awkit evaluate --offline
  awkit evaluate --offline --strict   # CI / release check
`)
}

type gateRow struct {
	id     string
	name   string
	result evaluate.GateResult
}

func cmdEvaluate(args []string) int {
	fs := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageEvaluate

	offline := fs.Bool("offline", false, "")
	strict := fs.Bool("strict", false, "")
	path := fs.String("path", ".", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usageEvaluate()
		return 0
	}

	e := evaluate.New(*path)
	offlineResults := e.RunOffline()

	var onlineResults evaluate.OnlineGateResults
	if *offline {
		skip := evaluate.Skip("offline mode")
		onlineResults = evaluate.OnlineGateResults{N1: skip, N2: skip, N3: skip}
	} else {
		onlineResults = e.RunOnline()
	}

	rows := []gateRow{
		{"O0", "git ignore", offlineResults.O0},
		{"O1", "scan repo", offlineResults.O1},
		{"O3", "project audit", offlineResults.O3},
		{"O5", "config validation", offlineResults.O5},
		{"O7", "version sync", offlineResults.O7},
		{"O8", "file encoding", offlineResults.O8},
		{"O10", "test suite", offlineResults.O10},
	}
	if !*offline {
		rows = append(rows,
			gateRow{"N1", "kickoff dry-run", onlineResults.N1},
			gateRow{"N2", "rollback output", onlineResults.N2},
			gateRow{"N3", "stats JSON", onlineResults.N3},
		)
	}

	fmt.Println("AWK Evaluation")
	fmt.Println()
	failed := 0
	for _, g := range rows {
		marker := "-"
		switch g.result.Status {
		case "FAIL":
			marker = "✗"
			failed++
		case "PASS":
			marker = "✓"
		}
		fmt.Printf("  %s %-4s %-18s %-5s %s\n", marker, g.id, g.name, g.result.Status, g.result.Reason)
	}

	score := evaluate.CalculateScoreCap(offlineResults, onlineResults)
	grade := evaluate.ScoreToGrade(score)
	fmt.Println()
	fmt.Printf("Score cap: %.1f  Grade: %s — %s\n", score, grade, evaluate.GradeDescription(grade))

	if failed > 0 {
		if *strict {
			fmt.Printf("\nSTRICT: %d gate(s) failed\n", failed)
			return 1
		}
		fmt.Printf("\n%d gate(s) failed (report-only; use --strict to enforce)\n", failed)
	}
	return 0
}
