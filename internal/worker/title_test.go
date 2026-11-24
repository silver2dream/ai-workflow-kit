package worker

import (
	"slices"
	"strings"
	"testing"
)

// A ticket's commit and pull request are titled with the issue's title. They
// were titled with the first heading of the ticket's body, and a ticket
// written to ticket.md starts with "## Summary": every pull request of a real
// run was titled "[chore] summary", and so was the commit it was merged as.
func TestPRTitle(t *testing.T) {
	ticket := "## Summary\n\nAdd `Winner`.\n\n## Scope\n\n- rally.go\n"
	tests := []struct {
		name, issueTitle, ticket, want string
	}{
		{"the issue's title", "[feat] say who wins a game", ticket, "[feat] say who wins a game"},
		{"a title with no type", "Say who wins a game", ticket, "[chore] say who wins a game"},
		{"a title in Chinese keeps its words", "[feat] 顯示比分 Score", ticket, "[feat] 顯示比分 score"},
		{"a section of the ticket is not its title", "", ticket, "[chore] issue-7"},
		{"a blank title is no title", "  ", ticket, "[chore] issue-7"},
		{"a ticket's own title heading, without the issue's", "", "# [fix] crash on start\n\n## Summary\n", "[fix] crash on start"},
		{"the issue's title before the ticket's heading", "[feat] from the issue", "# [fix] from the body\n", "[feat] from the issue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildCommitMessage(prTitle(tt.issueTitle, tt.ticket, 7)); got != tt.want {
				t.Errorf("titled %q, want %q", got, tt.want)
			}
		})
	}
}

// The dispatch passes the issue's title to run-issue as one argument, so that
// a title starting with a hyphen is not taken for a flag; with no title, no
// flag.
func TestRunIssueArgs_TheTitle(t *testing.T) {
	args := runIssueArgs("/p", 7, "  -[feat] odd title ", "/p/ticket.md", "root", "", 0)
	if !slices.Contains(args, "--title=-[feat] odd title") {
		t.Errorf("the title is not passed as one argument: %q", args)
	}
	for _, a := range runIssueArgs("/p", 7, " ", "/p/ticket.md", "", "", 0) {
		if strings.HasPrefix(a, "--title") {
			t.Errorf("a blank title is passed: %q", a)
		}
	}
	want := []string{"run-issue", "--issue", "7", "--ticket", "/p/ticket.md", "--state-root", "/p", "--title=x", "--repo", "root", "--merge-issue", "conflict", "--pr-number", "30"}
	if got := runIssueArgs("/p", 7, "x", "/p/ticket.md", "root", "conflict", 30); !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}
