package analyzer

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Issue represents a GitHub issue
type Issue struct {
	Number int      `json:"number"`
	Body   string   `json:"body"`
	Labels []Label  `json:"labels"`
	State  string   `json:"state"`
}

// Label represents a GitHub label
type Label struct {
	Name string `json:"name"`
}

// HasLabel checks if issue has a specific label
func (i *Issue) HasLabel(name string) bool {
	for _, l := range i.Labels {
		if l.Name == name {
			return true
		}
	}
	return false
}

// GitHubClient wraps GitHub CLI operations
type GitHubClient struct {
	Timeout time.Duration
}

// NewGitHubClient creates a new GitHub client
func NewGitHubClient(timeout time.Duration) *GitHubClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &GitHubClient{Timeout: timeout}
}

// ListIssuesByLabel lists issues with a specific label
func (c *GitHubClient) ListIssuesByLabel(ctx context.Context, label string) ([]Issue, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--label", label,
		"--state", "open",
		"--json", "number,body,labels,state")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var issues []Issue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, err
	}

	return issues, nil
}

// ListPendingIssues lists issues with task label but without in-progress, pr-ready, worker-failed, or needs-review labels
func (c *GitHubClient) ListPendingIssues(ctx context.Context, labels LabelsConfig) ([]Issue, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--label", labels.Task,
		"--state", "open",
		"--json", "number,body,labels,state")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var allIssues []Issue
	if err := json.Unmarshal(output, &allIssues); err != nil {
		return nil, err
	}

	// Filter out issues with blocking labels
	var pendingIssues []Issue
	for _, issue := range allIssues {
		if !issue.HasLabel(labels.InProgress) &&
			!issue.HasLabel(labels.PRReady) &&
			!issue.HasLabel(labels.WorkerFailed) &&
			!issue.HasLabel(labels.NeedsHumanReview) &&
			!issue.HasLabel(labels.ReviewFailed) {
			pendingIssues = append(pendingIssues, issue)
		}
	}

	return pendingIssues, nil
}

// CountOpenIssues counts open issues with the task label
func (c *GitHubClient) CountOpenIssues(ctx context.Context, taskLabel string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "list",
		"--label", taskLabel,
		"--state", "open",
		"--json", "number",
		"--jq", ". | length")

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}

	return count, nil
}

// RemoveLabel removes a label from an issue
func (c *GitHubClient) RemoveLabel(ctx context.Context, issueNumber int, label string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "edit",
		strconv.Itoa(issueNumber),
		"--remove-label", label)

	return cmd.Run()
}

// AddLabel adds a label to an issue
func (c *GitHubClient) AddLabel(ctx context.Context, issueNumber int, label string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "edit",
		strconv.Itoa(issueNumber),
		"--add-label", label)

	return cmd.Run()
}

// ExtractPRNumber extracts PR number from issue body or result file
func ExtractPRNumber(body string) int {
	// Try to extract from pull URL pattern
	prURLPattern := regexp.MustCompile(`/pull/(\d+)`)
	if matches := prURLPattern.FindStringSubmatch(body); len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}

	// Try to extract from #number pattern
	prRefPattern := regexp.MustCompile(`#(\d+)`)
	if matches := prRefPattern.FindStringSubmatch(body); len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}

	return 0
}

// IsPRMerged checks if a PR has been merged
func (c *GitHubClient) IsPRMerged(ctx context.Context, prNumber int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view",
		strconv.Itoa(prNumber),
		"--json", "state,mergedAt")

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	var pr struct {
		State    string `json:"state"`
		MergedAt string `json:"mergedAt"`
	}
	if err := json.Unmarshal(output, &pr); err != nil {
		return false, err
	}

	return pr.State == "MERGED" || pr.MergedAt != "", nil
}

// CloseIssue closes an issue
func (c *GitHubClient) CloseIssue(ctx context.Context, issueNumber int) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "close", strconv.Itoa(issueNumber))
	return cmd.Run()
}
