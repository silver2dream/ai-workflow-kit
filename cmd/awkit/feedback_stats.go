package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
)

func cmdFeedbackStats(args []string) int {
	stateRoot := "."
	if len(args) >= 1 && !strings.HasPrefix(args[0], "-") {
		stateRoot = args[0]
	}

	entries, err := reviewer.LoadFeedback(stateRoot)
	if err != nil {
		errorf("Failed to load feedback: %v\n", err)
		return 1
	}

	if len(entries) == 0 {
		fmt.Println("No feedback recorded yet.")
		return 0
	}

	// Calculate stats
	totalReviews := len(entries)
	categoryCount := make(map[string]int)
	totalScore := 0

	for _, e := range entries {
		totalScore += e.Score
		for _, c := range e.Categories {
			categoryCount[c]++
		}
	}

	avgScore := float64(totalScore) / float64(totalReviews)

	fmt.Println("")
	fmt.Println(bold("Review Feedback Statistics"))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  Total rejections:  %d\n", totalReviews)
	fmt.Printf("  Average score:     %.1f/10\n", avgScore)
	fmt.Println("")

	// Top rejection categories
	if len(categoryCount) > 0 {
		type catEntry struct {
			Name  string
			Count int
		}
		var sorted []catEntry
		for name, count := range categoryCount {
			sorted = append(sorted, catEntry{name, count})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Count > sorted[j].Count
		})

		fmt.Println(bold("Top Rejection Categories"))
		fmt.Println(strings.Repeat("─", 40))
		for _, c := range sorted {
			bar := strings.Repeat("█", c.Count)
			fmt.Printf("  %-18s %s (%d)\n", c.Name, bar, c.Count)
		}
		fmt.Println("")
	}

	// Recent rejections
	recentCount := 5
	if recentCount > len(entries) {
		recentCount = len(entries)
	}
	recent := entries[len(entries)-recentCount:]

	fmt.Println(bold("Recent Rejections"))
	fmt.Println(strings.Repeat("─", 40))
	for _, e := range recent {
		cats := "none"
		if len(e.Categories) > 0 {
			cats = strings.Join(e.Categories, ", ")
		}
		fmt.Printf("  Issue #%-4d  Score: %d/10  [%s]\n", e.IssueID, e.Score, cats)
	}
	fmt.Println("")

	return 0
}

func usageFeedbackStats() {
	fmt.Fprint(os.Stderr, `Show review feedback statistics

Usage:
  awkit feedback-stats [project_path]

Shows:
  - Total rejections and average score
  - Top rejection categories
  - Recent rejection trend

Examples:
  awkit feedback-stats
  awkit feedback-stats /path/to/project
`)
}
