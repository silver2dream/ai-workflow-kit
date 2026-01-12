package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

func usageEvents() {
	fmt.Fprint(os.Stderr, `Query unified event stream for debugging workflow issues

Usage:
  awkit events [options]

Options:
  --session <id>    Query specific session (default: current session)
  --level <level>   Filter by level: info, warn, error, decision
  --issue <n>       Filter by issue number
  --pr <n>          Filter by PR number
  --component <c>   Filter by component: principal, worker, reviewer, github
  --last <n>        Show only last N events (default: all)
  --json            Output raw JSON (for AI analysis)
  --list            List available sessions

Examples:
  awkit events                              # Show current session events
  awkit events --level decision             # Show only decision points
  awkit events --level error                # Show only errors
  awkit events --issue 25                   # Show events for issue #25
  awkit events --last 50                    # Show last 50 events
  awkit events --session principal-xxx      # Query specific session
  awkit events --list                       # List available sessions
  awkit events --json > events.jsonl        # Export for AI analysis
`)
}

func cmdEvents(args []string) int {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageEvents

	sessionID := fs.String("session", "", "")
	level := fs.String("level", "", "")
	issueNum := fs.Int("issue", 0, "")
	prNum := fs.Int("pr", 0, "")
	component := fs.String("component", "", "")
	last := fs.Int("last", 0, "")
	jsonOutput := fs.Bool("json", false, "")
	listSessions := fs.Bool("list", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageEvents()
		return 0
	}

	reader := trace.NewEventReader(".")

	// List sessions mode
	if *listSessions {
		sessions, err := reader.ListSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
			return 1
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found")
			return 0
		}
		fmt.Println("Available sessions:")
		for _, s := range sessions {
			fmt.Printf("  %s\n", s)
		}
		return 0
	}

	// Build filter
	filter := trace.EventFilter{
		Level:     *level,
		IssueID:   *issueNum,
		PRNumber:  *prNum,
		Component: *component,
		Last:      *last,
	}

	// Read events
	var events []trace.Event
	var err error

	if *sessionID != "" {
		events, err = reader.ReadSessionFiltered(*sessionID, filter)
	} else {
		events, err = reader.ReadCurrentSessionFiltered(filter)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading events: %v\n", err)
		return 1
	}

	if len(events) == 0 {
		fmt.Println("No events found")
		return 0
	}

	// Output
	if *jsonOutput {
		// Raw JSON Lines output for AI analysis
		for _, e := range events {
			data, _ := json.Marshal(e)
			fmt.Println(string(data))
		}
	} else {
		// Human readable output
		for _, e := range events {
			printEvent(e)
		}
		fmt.Printf("\nTotal: %d events\n", len(events))
	}

	return 0
}

func printEvent(e trace.Event) {
	// Format: [SEQ] TIMESTAMP LEVEL COMPONENT TYPE [ISSUE/PR] message
	ts := e.Timestamp.Format("15:04:05")
	level := colorizeLevel(e.Level)
	component := colorizeComponent(e.Component)

	// Build context string
	var context []string
	if e.IssueID > 0 {
		context = append(context, fmt.Sprintf("#%d", e.IssueID))
	}
	if e.PRNumber > 0 {
		context = append(context, fmt.Sprintf("PR#%d", e.PRNumber))
	}
	contextStr := ""
	if len(context) > 0 {
		contextStr = " [" + strings.Join(context, " ") + "]"
	}

	// Decision events get special formatting
	if e.Decision != nil {
		fmt.Printf("[%d] %s %s %s %s%s\n", e.Seq, ts, level, component, e.Type, contextStr)
		fmt.Printf("    Rule: %s\n", e.Decision.Rule)
		fmt.Printf("    Conditions: %v\n", formatConditions(e.Decision.Conditions))
		fmt.Printf("    Result: %s\n", colorizeDecisionResult(e.Decision.Result))
		if e.Error != "" {
			fmt.Printf("    Error: %s\n", e.Error)
		}
	} else {
		msg := formatEventData(e)
		if e.Error != "" {
			msg = e.Error
		}
		fmt.Printf("[%d] %s %s %s %s%s %s\n", e.Seq, ts, level, component, e.Type, contextStr, msg)
	}
}

func colorizeLevel(level string) string {
	switch level {
	case "error":
		return "\033[31mERROR\033[0m"
	case "warn":
		return "\033[33mWARN\033[0m"
	case "decision":
		return "\033[35mDECISION\033[0m"
	default:
		return "\033[32mINFO\033[0m"
	}
}

func colorizeComponent(component string) string {
	switch component {
	case "principal":
		return "\033[36mprincipal\033[0m"
	case "worker":
		return "\033[34mworker\033[0m"
	case "reviewer":
		return "\033[33mreviewer\033[0m"
	case "github":
		return "\033[35mgithub\033[0m"
	default:
		return component
	}
}

func colorizeDecisionResult(result string) string {
	switch result {
	case "CONTINUE", "RETRY", "SUCCESS":
		return "\033[32m" + result + "\033[0m"
	case "STOP_COMPLETE":
		return "\033[32m" + result + "\033[0m"
	case "STOP_NONE", "FAIL_FINAL", "PAUSE":
		return "\033[31m" + result + "\033[0m"
	default:
		return "\033[33m" + result + "\033[0m"
	}
}

func formatConditions(conditions map[string]any) string {
	if len(conditions) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(conditions))
	for k, v := range conditions {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ", ")
}

func formatEventData(e trace.Event) string {
	if e.Data == nil {
		return ""
	}

	switch data := e.Data.(type) {
	case map[string]any:
		parts := make([]string, 0)
		for k, v := range data {
			if v != nil && v != "" && v != 0 {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	case map[string]string:
		parts := make([]string, 0)
		for k, v := range data {
			if v != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return ""
}
