package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

// GitHub is what the engine reads from GitHub and does to it, besides what
// its steps do: the tickets, their labels (the lanes' projection), a
// comment, and whether a pull request can be merged.
type GitHub interface {
	// Tickets are the open issues with the task label: the workflow's.
	Tickets(ctx context.Context) ([]Ticket, error)
	// Ticket is one issue, open or closed.
	Ticket(ctx context.Context, issue int) (Ticket, error)
	// Edit changes a ticket's labels, and closes or reopens it.
	Edit(ctx context.Context, issue int, ops Ops) error
	Comment(ctx context.Context, issue int, body string) error
	// Mergeable is GitHub's word on a pull request: MERGEABLE, CONFLICTING
	// or UNKNOWN.
	Mergeable(ctx context.Context, pr int) (string, error)
}

// Ticket is an issue as the engine reads it.
type Ticket struct {
	Number int
	Body   string
	Labels []string
	Closed bool
}

// ghCLI is GitHub through the gh CLI; every call that writes goes through
// the guard's outbound check (ghutil.RunWithRetry).
type ghCLI struct {
	task    string
	timeout time.Duration
}

// NewGitHub is GitHub through the gh CLI, for the tickets with the task
// label.
func NewGitHub(taskLabel string, timeout time.Duration) GitHub {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return ghCLI{task: taskLabel, timeout: timeout}
}

func (g ghCLI) gh(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	return ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", args...)
}

type ghIssue struct {
	Number int    `json:"number"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (i ghIssue) ticket() Ticket {
	t := Ticket{Number: i.Number, Body: i.Body, Closed: strings.EqualFold(i.State, "closed")}
	for _, l := range i.Labels {
		t.Labels = append(t.Labels, l.Name)
	}
	return t
}

func (g ghCLI) Tickets(ctx context.Context) ([]Ticket, error) {
	out, err := g.gh(ctx, "issue", "list", "--state", "open", "--label", g.task, "--limit", "500", "--json", "number,body,labels,state")
	if err != nil {
		return nil, fmt.Errorf("the tickets could not be listed: %w", err)
	}
	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("the tickets could not be read: %w", err)
	}
	tickets := make([]Ticket, len(issues))
	for i, issue := range issues {
		tickets[i] = issue.ticket()
	}
	return tickets, nil
}

func (g ghCLI) Ticket(ctx context.Context, issue int) (Ticket, error) {
	out, err := g.gh(ctx, "issue", "view", strconv.Itoa(issue), "--json", "number,body,labels,state")
	if err != nil {
		return Ticket{}, fmt.Errorf("issue #%d could not be read: %w", issue, err)
	}
	var i ghIssue
	if err := json.Unmarshal(out, &i); err != nil {
		return Ticket{}, fmt.Errorf("issue #%d could not be read: %w", issue, err)
	}
	return i.ticket(), nil
}

func (g ghCLI) Edit(ctx context.Context, issue int, ops Ops) error {
	n := strconv.Itoa(issue)
	if ops.Reopen {
		if _, err := g.gh(ctx, "issue", "reopen", n); err != nil {
			return err
		}
	}
	if len(ops.Add) > 0 || len(ops.Remove) > 0 {
		args := []string{"issue", "edit", n}
		if len(ops.Add) > 0 {
			args = append(args, "--add-label", strings.Join(ops.Add, ","))
		}
		if len(ops.Remove) > 0 {
			args = append(args, "--remove-label", strings.Join(ops.Remove, ","))
		}
		if _, err := g.gh(ctx, args...); err != nil {
			return err
		}
	}
	if ops.Close {
		if _, err := g.gh(ctx, "issue", "close", n); err != nil {
			return err
		}
	}
	return nil
}

func (g ghCLI) Comment(ctx context.Context, issue int, body string) error {
	_, err := g.gh(ctx, "issue", "comment", strconv.Itoa(issue), "--body", body)
	return err
}

// Mergeable asks as the Principal's workflow asks: GitHub works mergeability
// out lazily, and an UNKNOWN is asked again.
func (g ghCLI) Mergeable(ctx context.Context, pr int) (string, error) {
	return analyzer.NewGitHubClient(g.timeout).PRMergeable(ctx, pr)
}
