package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
)

func usageStatus() {
	fmt.Fprint(os.Stderr, `Show offline workflow status (no GitHub calls)

Usage:
  awkit status [options]

Options:
  --issue <id>  Show status for a specific issue
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

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			output.Error(fmt.Sprintf("Failed to write JSON: %v", err))
			return 1
		}
		return 0
	}

	fmt.Println("")

	// Run
	runLine := fmt.Sprintf("Run: %s", report.Run.State)
	if report.Run.PID != 0 {
		runLine += fmt.Sprintf("  pid=%d", report.Run.PID)
	}
	if report.Run.Session.SessionID != "" {
		runLine += fmt.Sprintf("  session=%s", report.Run.Session.SessionID)
	}
	if report.Run.Session.EndedAt != "" {
		runLine += fmt.Sprintf("  ended=%s", report.Run.Session.EndedAt)
	}
	fmt.Println(runLine)

	// Config (rules/agents)
	if len(report.Config.RulesKit) > 0 || len(report.Config.RulesCustom) > 0 ||
		len(report.Config.AgentsBuiltin) > 0 || len(report.Config.AgentsCustom) > 0 {
		configLine := "Config:"
		if len(report.Config.RulesKit) > 0 {
			configLine += fmt.Sprintf("  rules.kit=[%s]", strings.Join(report.Config.RulesKit, ","))
		}
		if len(report.Config.RulesCustom) > 0 {
			configLine += fmt.Sprintf("  rules.custom=[%s]", strings.Join(report.Config.RulesCustom, ","))
		}
		if len(report.Config.AgentsBuiltin) > 0 {
			configLine += fmt.Sprintf("  agents.builtin=[%s]", strings.Join(report.Config.AgentsBuiltin, ","))
		}
		if len(report.Config.AgentsCustom) > 0 {
			configLine += fmt.Sprintf("  agents.custom=[%s]", strings.Join(report.Config.AgentsCustom, ","))
		}
		fmt.Println(configLine)
	}

	// Control
	controlLine := fmt.Sprintf("Control: STOP=%s", ternary(report.Control.StopPresent, "present", "absent"))
	if report.Control.LoopCount != nil {
		controlLine += fmt.Sprintf("  loop_count=%d", *report.Control.LoopCount)
	}
	if report.Control.ConsecutiveFailures != nil {
		controlLine += fmt.Sprintf("  consecutive_failures=%d", *report.Control.ConsecutiveFailures)
	}
	fmt.Println(controlLine)

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
			printArtifactLine("result", status.Artifact{Path: fmt.Sprintf(".ai/results/issue-%d.json", report.Artifacts.IssueID), Exists: false})
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
