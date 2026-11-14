package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/migrate"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
)

func usageStatus() {
	fmt.Fprint(os.Stderr, `Show where the work is: the run, every ticket, and what to do next

  Run       whether a run is going, and how the last one ended
  Tickets   every open ticket by the stage it is in: needs you, in review,
            in progress, back to a Worker, ready, waiting
  Next      the one thing to do next

Usage:
  awkit status [options]

Options:
  --issue <id>  Show the files a run left for one issue: its result, its
                trace and its logs
  --offline     Do not ask GitHub: the run only, no tickets
  --json        Output JSON

Examples:
  awkit status
  awkit status --issue 42
  awkit status --json
`)
}

func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageStatus

	issueID := fs.Int("issue", 0, "")
	offline := fs.Bool("offline", false, "")
	jsonOut := fs.Bool("json", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageStatus()
		return 0
	}

	output := kickoff.NewOutputFormatter(os.Stdout)

	report, err := status.Collect(".", status.Options{IssueID: *issueID})
	if err != nil {
		output.Error(fmt.Sprintf("Failed to collect status: %v", err))
		return 1
	}

	// The configuration says which project this is and by which labels its
	// tickets are read. A directory without one is not set up; a project
	// whose files are where an earlier awkit kept them is told so below.
	cfg, cfgErr := config.LoadFile(config.Path("."))
	if cfgErr != nil {
		cfg = nil
	}
	view := collectStatusView(context.Background(), report, cfg, *offline || *issueID > 0)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			status.Report
			Tickets *status.Tickets `json:"tickets,omitempty"`
		}{report, view.Tickets}); err != nil {
			output.Error(fmt.Sprintf("Failed to write JSON: %v", err))
			return 1
		}
		return 0
	}

	fmt.Println("")

	// What follows is read from where awkit keeps a project's files now. In a
	// project that has not been upgraded it would read as "nothing has run".
	if migrate.UsesOldLayout(".") {
		output.Warning("This project keeps files where an earlier awkit put them: what is shown below does not see them. Run 'awkit upgrade'.")
	} else if cfgErr != nil && !os.IsNotExist(cfgErr) {
		output.Warning(fmt.Sprintf("The configuration cannot be used: %v", cfgErr))
	}

	if *issueID == 0 {
		printStatus(os.Stdout, view)
		return 0
	}

	// Target
	if report.Target.IssueID > 0 {
		fmt.Printf("Target: issue=%d  source=%s\n", report.Target.IssueID, report.Target.Source)
	}

	// Artifacts (only if we have a target)
	if report.Artifacts.IssueID > 0 {
		fmt.Printf("Artifacts(issue=%d):\n", report.Artifacts.IssueID)

		printArtifactLine := func(label string, a status.Artifact) {
			state := "MISSING"
			if a.Exists {
				state = "OK"
			}
			fmt.Printf("  - %s=%s  %s\n", label, state, a.Path)
		}

		if report.Artifacts.Result != nil {
			printArtifactLine("result", status.Artifact{Path: report.Artifacts.Result.Path, Exists: report.Artifacts.Result.Exists})
		} else {
			printArtifactLine("result", status.Artifact{Path: fmt.Sprintf(layout.ResultsDir+"/issue-%d.json", report.Artifacts.IssueID), Exists: false})
		}

		if report.Artifacts.Trace != nil {
			printArtifactLine("trace", status.Artifact{Path: report.Artifacts.Trace.Path, Exists: report.Artifacts.Trace.Exists})
		} else {
			printArtifactLine("trace", status.Artifact{Path: fmt.Sprintf(".ai/state/traces/issue-%d.json", report.Artifacts.IssueID), Exists: false})
		}

		printArtifactLine("run_dir", report.Artifacts.RunDir)
		printArtifactLine("summary", report.Artifacts.Summary)
		printArtifactLine("principal_log", report.Artifacts.Logs.Principal)
		printArtifactLine("worker_log", report.Artifacts.Logs.Worker)
		for i, a := range report.Artifacts.Logs.Codex {
			printArtifactLine(fmt.Sprintf("codex_log_%d", i+1), a)
		}
	}

	// Warnings / suggestions
	if len(report.Warnings) > 0 {
		fmt.Println("")
		output.Warning("Warnings:")
		for _, w := range report.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	if len(report.Suggestions) > 0 {
		fmt.Println("")
		output.Info("Next:")
		for _, s := range report.Suggestions {
			fmt.Printf("  - %s\n", s)
		}
	}

	return 0
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
