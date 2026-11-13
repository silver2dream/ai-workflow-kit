package worker

import (
	"strings"
	"testing"
)

func TestBuildWorkerPrompt_Minimal(t *testing.T) {
	prompt := buildWorkerPrompt(workerPromptInputs{Ticket: "# Task\n\nDo something", IssueID: 7})

	for _, want := range []string{
		"You are an automated coding agent running inside a git worktree.",
		"- Read and follow AGENTS.md.",
		"Ticket:\n# Task\n\nDo something\n",
		".ai/state/runs/issue-7/plan.md",
		"FORBIDDEN OPERATIONS",
		".ai/state/principal/",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}

	for _, section := range []string{
		"CODEBASE MAP",
		"PREVIOUS REVIEW FEEDBACK",
		"FILE-LEVEL REVIEW COMMENTS",
		"IMPORTANT: You are working in",
	} {
		if strings.Contains(prompt, section) {
			t.Errorf("prompt has a %q section although nothing was supplied for it", section)
		}
	}
}

func TestBuildWorkerPrompt_OptionalSectionsInOrder(t *testing.T) {
	prompt := buildWorkerPrompt(workerPromptInputs{
		WorkDirInstruction: "IMPORTANT: You are working in a MONOREPO (directory type).\n",
		Ticket:             "# Task",
		IssueID:            7,
		GraphSection:       "graph-text\n",
		ReviewComments:     "review-feedback-text",
		PRReviewComments:   "- **a.go:1** → file-comment-text\n",
		LessonsSection:     "LESSONS FROM THIS PROJECT'S REVIEW HISTORY (follow these checks):\n1. lesson-text\n",
		TopCategorySection: "top-category-text\n",
	})

	// Each piece is present, and in the order the Worker should meet it.
	order := []string{
		"IMPORTANT: You are working in a MONOREPO",
		"FORBIDDEN OPERATIONS",
		"CODEBASE MAP",
		"graph-text",
		"Ticket:\n# Task",
		"PREVIOUS REVIEW FEEDBACK",
		"review-feedback-text",
		"FILE-LEVEL REVIEW COMMENTS",
		"file-comment-text",
		"lesson-text",
		"top-category-text",
		"BEFORE making code changes",
		"After making changes:",
	}
	last := -1
	for _, piece := range order {
		at := strings.Index(prompt, piece)
		if at < 0 {
			t.Fatalf("prompt is missing %q", piece)
		}
		if at < last {
			t.Errorf("%q appears out of order", piece)
		}
		last = at
	}
}

// The Worker is no longer asked to print the status and diff the runner
// computes itself.
func TestBuildWorkerPrompt_DoesNotAskForGitOutput(t *testing.T) {
	prompt := buildWorkerPrompt(workerPromptInputs{Ticket: "# Task"})
	for _, gone := range []string{"git status --porcelain", "Print: git diff"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt still contains %q", gone)
		}
	}
}
