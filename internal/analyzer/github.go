package analyzer

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
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

// GitHubClientInterface defines the interface for GitHub operations
// This allows for mocking in tests
type GitHubClientInterface interface {
	ListIssuesByLabel(ctx context.Context, label string) ([]Issue, error)
	ListPendingIssues(ctx context.Context, labels LabelsConfig) ([]Issue, error)
	CountOpenIssues(ctx context.Context, taskLabel string) (int, error)
	RemoveLabel(ctx context.Context, issueNumber int, label string) error
	AddLabel(ctx context.Context, issueNumber int, label string) error
	IsPRMerged(ctx context.Context, prNumber int) (bool, error)
	CloseIssue(ctx context.Context, issueNumber int) error
	FindPRByBranch(ctx context.Context, branchName string) (int, error)
}

// GitHubClient wraps GitHub CLI operations
type GitHubClient struct {
	Timeout time.Duration
	Retry   ghutil.RetryConfig
}

// Ensure GitHubClient implements GitHubClientInterface
var _ GitHubClientInterface = (*GitHubClient)(nil)

// NewGitHubClient creates a new GitHub client
func NewGitHubClient(timeout time.Duration) *GitHubClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &GitHubClient{
		Timeout: timeout,
		Retry:   ghutil.DefaultRetryConfig(),
	}
}

// ListIssuesByLabel lists issues with a specific label
func (c *GitHubClient) ListIssuesByLabel(ctx context.Context, label string) ([]Issue, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "list",
		"--label", label,
		"--state", "open",
		"--limit", "200",
		"--json", "number,body,labels,state")
	if err != nil {
		return nil, err
	}

	var issues []Issue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, err
	}

	return issues, nil
}

// ListPendingIssues lists issues with task label but without blocking labels
// (in-progress, pr-ready, worker-failed, needs-human-review, review-failed, merge-conflict, needs-rebase, completed)
func (c *GitHubClient) ListPendingIssues(ctx context.Context, labels LabelsConfig) ([]Issue, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "list",
		"--label", labels.Task,
		"--state", "open",
		"--limit", "200",
		"--json", "number,body,labels,state")
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
			!issue.HasLabel(labels.ReviewFailed) &&
			!issue.HasLabel(labels.MergeConflict) &&
			!issue.HasLabel(labels.NeedsRebase) &&
			!issue.HasLabel(labels.Completed) {
			pendingIssues = append(pendingIssues, issue)
		}
	}

	return pendingIssues, nil
}

// CountOpenIssues counts open issues with the task label
func (c *GitHubClient) CountOpenIssues(ctx context.Context, taskLabel string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "list",
		"--label", taskLabel,
		"--state", "open",
		"--limit", "200",
		"--json", "number",
		"--jq", ". | length")
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

	_, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "edit",
		strconv.Itoa(issueNumber),
		"--remove-label", label)
	return err
}

// AddLabel adds a label to an issue
func (c *GitHubClient) AddLabel(ctx context.Context, issueNumber int, label string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "edit",
		strconv.Itoa(issueNumber),
		"--add-label", label)
	return err
}

// ExtractPRNumber extracts PR number from issue body or result file
// Only matches explicit PR URL patterns to avoid false positives from Issue references (#123)
func ExtractPRNumber(body string) int {
	// Try to extract from full GitHub PR URL pattern
	// Matches: https://github.com/owner/repo/pull/123
	fullURLPattern := regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/(\d+)`)
	if matches := fullURLPattern.FindStringSubmatch(body); len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}

	// Try to extract from relative pull URL pattern
	// Matches: /pull/123 (but not /pulls/123 which is a list endpoint)
	relativeURLPattern := regexp.MustCompile(`/pull/(\d+)(?:[^\d]|$)`)
	if matches := relativeURLPattern.FindStringSubmatch(body); len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			return num
		}
	}

	// Try to extract from PR reference pattern (explicitly marked as PR)
	// Matches: PR #123, PR#123, pull request #123
	prRefPattern := regexp.MustCompile(`(?i)(?:PR\s*#|pull\s+request\s*#)(\d+)`)
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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "pr", "view",
		strconv.Itoa(prNumber),
		"--json", "state,mergedAt")
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

// FindPRByBranch finds an open PR by head branch name and returns its number
// Returns 0 if no open PR is found for the branch
func (c *GitHubClient) FindPRByBranch(ctx context.Context, branchName string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "pr", "list",
		"--head", branchName,
		"--state", "open",
		"--limit", "1",
		"--json", "number",
		"--jq", ".[0].number")
	if err != nil {
		return 0, err
	}

	numStr := strings.TrimSpace(string(output))
	if numStr == "" || numStr == "null" {
		return 0, nil
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, nil
	}

	return num, nil
}

// CloseIssue closes an issue
func (c *GitHubClient) CloseIssue(ctx context.Context, issueNumber int) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	_, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "close", strconv.Itoa(issueNumber))
	return err
}
