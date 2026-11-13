package analyzer

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ScopeEnv names the environment variable that limits a run to one ticket.
// `awkit run --issue N` sets it, and every awkit command the run starts
// inherits it.
const ScopeEnv = "AWKIT_RUN_ISSUE"

// ScopeFromEnv returns the issue the run is limited to, or 0 when the run
// works through every ticket. A value that is not an issue number is an error:
// a run that was meant to be limited must not quietly become one that is not.
func ScopeFromEnv() (int, error) {
	value := strings.TrimSpace(os.Getenv(ScopeEnv))
	if value == "" {
		return 0, nil
	}
	issue, err := strconv.Atoi(value)
	if err != nil || issue <= 0 {
		return 0, fmt.Errorf("%s is %q, which is not an issue number", ScopeEnv, value)
	}
	return issue, nil
}

// LimitTo makes the analyzer decide for one ticket only. No other issue is
// dispatched, reviewed, reopened or commented on, an issue that needs a person
// stops the run only if it is that ticket, and the run is complete when that
// ticket is.
//
// Call it once the GitHub client is in place. Zero (or less) limits nothing.
func (a *Analyzer) LimitTo(issue int) {
	if issue <= 0 {
		return
	}
	a.only = issue
	a.GHClient = &scopedClient{GitHubClientInterface: a.GHClient, issue: issue}
}

// Limit returns the one issue the analyzer decides for, or 0 when it decides
// for every ticket.
func (a *Analyzer) Limit() int {
	return a.only
}

// outOfScope reports whether an issue is one a limited run leaves alone.
func (a *Analyzer) outOfScope(issue int) bool {
	return a.only != 0 && issue != a.only
}

// scopedClient shows the decision engine one ticket: every list of issues
// holds that issue or nothing. What is asked about a single issue by number,
// a dependency's state for one, is answered as it is.
type scopedClient struct {
	GitHubClientInterface
	issue int
}

func (c *scopedClient) only(issues []Issue) []Issue {
	for _, issue := range issues {
		if issue.Number == c.issue {
			return []Issue{issue}
		}
	}
	return nil
}

func (c *scopedClient) ListIssuesByLabel(ctx context.Context, label string) ([]Issue, error) {
	issues, err := c.GitHubClientInterface.ListIssuesByLabel(ctx, label)
	return c.only(issues), err
}

func (c *scopedClient) ListPendingIssues(ctx context.Context, labels LabelsConfig) ([]Issue, error) {
	issues, err := c.GitHubClientInterface.ListPendingIssues(ctx, labels)
	return c.only(issues), err
}

// CountOpenIssues counts the ticket itself: one while it is open, none after.
func (c *scopedClient) CountOpenIssues(ctx context.Context, taskLabel string) (int, error) {
	issues, err := c.GitHubClientInterface.ListIssuesByLabel(ctx, taskLabel)
	if err != nil {
		return 0, err
	}
	return len(c.only(issues)), nil
}

func (c *scopedClient) TaskIssueStatuses(ctx context.Context, taskLabel string) (map[int]IssueStatus, error) {
	statuses, err := c.GitHubClientInterface.TaskIssueStatuses(ctx, taskLabel)
	if err != nil {
		return nil, err
	}
	scoped := map[int]IssueStatus{}
	if status, ok := statuses[c.issue]; ok {
		scoped[c.issue] = status
	}
	return scoped, nil
}
