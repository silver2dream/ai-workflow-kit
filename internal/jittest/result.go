package jittest

import (
	"fmt"
	"strings"
)

// FormatComment formats the JiTTest result as a markdown comment for PR review.
func (r *Result) FormatComment() string {
	var b strings.Builder

	b.WriteString("### JiT Test Results\n")
	b.WriteString(fmt.Sprintf("Generated: %d | Passed: %d | Failed: %d | Skipped: %d\n",
		r.Generated, r.Passed, r.Failed, r.Skipped))

	if r.Error != "" {
		b.WriteString(fmt.Sprintf("\n**Error:** %s\n", r.Error))
	}

	if r.Failed > 0 {
		b.WriteString("\n**Failed:**\n")
		for _, t := range r.Tests {
			if !t.Passed && t.Error != "" {
				b.WriteString(fmt.Sprintf("- `%s`: %s\n", t.Test.Filename, t.Error))
			}
		}
	}

	b.WriteString("\n> JiT tests are independent tests generated from the PR diff. They do not enter the codebase.\n")

	return b.String()
}

// FormatFeedback formats JiT test failures for injection into Worker prompts.
// This is used when failure_policy=block and the Worker needs to retry.
func (r *Result) FormatFeedback() string {
	if r.Failed == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Previous JiT test failures:\n")
	for _, t := range r.Tests {
		if !t.Passed && t.Error != "" {
			b.WriteString(fmt.Sprintf("- %s: %s\n", t.Test.Filename, t.Error))
		}
	}
	return b.String()
}

// IsBlocking returns true if the result should block merge based on the failure policy.
func (r *Result) IsBlocking(failurePolicy string) bool {
	return failurePolicy == "block" && r.Failed > 0
}
