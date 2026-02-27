package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/session"
)

func cmdContextSnapshot(args []string) int {
	stateRoot := "."
	if len(args) >= 1 && !strings.HasPrefix(args[0], "-") {
		stateRoot = args[0]
	}

	fmt.Println("# AWK Context Snapshot")
	fmt.Println("")

	// Session info
	mgr := session.NewManager(stateRoot)
	sessionID := mgr.GetCurrentSessionID()
	if sessionID != "" {
		fmt.Println("## Session")
		fmt.Printf("- Current session: `%s`\n", sessionID)
	} else {
		fmt.Println("## Session")
		fmt.Println("- No active session")
	}
	fmt.Println("")

	// State files summary
	fmt.Println("## State")
	stateDir := filepath.Join(stateRoot, ".ai", "state")
	if entries, err := os.ReadDir(stateDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".lock") {
				fmt.Printf("- Lock: `%s`\n", name)
			} else if name == "STOP" {
				fmt.Println("- **STOP file present** (workflow halted)")
			} else if name == "loop_count.txt" {
				if data, err := os.ReadFile(filepath.Join(stateDir, name)); err == nil {
					fmt.Printf("- Loop count: %s\n", strings.TrimSpace(string(data)))
				}
			}
		}
	}
	fmt.Println("")

	// Feedback summary
	fmt.Println("## Feedback")
	feedbackEntries, err := reviewer.LoadFeedback(stateRoot)
	if err == nil && len(feedbackEntries) > 0 {
		categoryCount := make(map[string]int)
		for _, e := range feedbackEntries {
			for _, c := range e.Categories {
				categoryCount[c]++
			}
		}
		fmt.Printf("- Total rejections: %d\n", len(feedbackEntries))
		if len(categoryCount) > 0 {
			var cats []string
			for k, v := range categoryCount {
				cats = append(cats, fmt.Sprintf("%s(%d)", k, v))
			}
			fmt.Printf("- Categories: %s\n", strings.Join(cats, ", "))
		}
	} else {
		fmt.Println("- No feedback recorded")
	}
	fmt.Println("")

	// Key files to re-read after compaction
	fmt.Println("## Key Files")
	fmt.Println("After context compaction, re-read these files:")
	fmt.Println("- `.ai/skills/principal-workflow/phases/main-loop.md`")
	fmt.Println("- `.ai/config/workflow.yaml`")
	fmt.Println("- Run `awkit status` for current workflow state")
	fmt.Println("- Run `awkit analyze-next --json` to determine next action")
	fmt.Println("")

	return 0
}

func usageContextSnapshot() {
	fmt.Fprint(os.Stderr, `Generate a compact context snapshot for rebuilding after compaction

Usage:
  awkit context-snapshot [project_path]

Outputs a markdown summary of current workflow state including:
  - Active session info
  - State files (locks, STOP, loop count)
  - Feedback statistics
  - Key files to re-read

Examples:
  awkit context-snapshot
  awkit context-snapshot /path/to/project
`)
}
