package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// GitHubClient provides GitHub operations via gh CLI
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

// IssueInfo contains basic issue information
type IssueInfo struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
	State  string   `json:"state"` // OPEN, CLOSED
}

// GetIssue fetches issue info from GitHub
func (c *GitHubClient) GetIssue(ctx context.Context, number int) (*IssueInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "view", fmt.Sprintf("%d", number), "--json", "number,title,body,labels,state")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout fetching issue %d", number)
		}
		return nil, fmt.Errorf("gh issue view failed: %s", stderr.String())
	}

	var result struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		State string `json:"state"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse issue JSON: %w", err)
	}

	labels := make([]string, len(result.Labels))
	for i, l := range result.Labels {
		labels[i] = l.Name
	}

	return &IssueInfo{
		Number: result.Number,
		Title:  result.Title,
		Body:   result.Body,
		Labels: labels,
		State:  result.State,
	}, nil
}

// EditIssueLabels modifies issue labels
func (c *GitHubClient) EditIssueLabels(ctx context.Context, number int, add, remove []string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	args := []string{"issue", "edit", fmt.Sprintf("%d", number)}

	for _, label := range add {
		args = append(args, "--add-label", label)
	}
	for _, label := range remove {
		args = append(args, "--remove-label", label)
	}

	if len(add) == 0 && len(remove) == 0 {
		return nil // Nothing to do
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout editing issue %d labels", number)
		}
		return fmt.Errorf("gh issue edit failed: %s", stderr.String())
	}

	return nil
}

// AddLabel adds a single label to an issue
func (c *GitHubClient) AddLabel(ctx context.Context, number int, label string) error {
	return c.EditIssueLabels(ctx, number, []string{label}, nil)
}

// RemoveLabel removes a single label from an issue
func (c *GitHubClient) RemoveLabel(ctx context.Context, number int, label string) error {
	return c.EditIssueLabels(ctx, number, nil, []string{label})
}

// CommentOnIssue adds a comment to an issue
func (c *GitHubClient) CommentOnIssue(ctx context.Context, number int, body string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "comment", fmt.Sprintf("%d", number), "--body", body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout commenting on issue %d", number)
		}
		return fmt.Errorf("gh issue comment failed: %s", stderr.String())
	}

	return nil
}

// PRInfo contains basic PR information
type PRInfo struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

// GetPRByBranch finds a PR by branch name
func (c *GitHubClient) GetPRByBranch(ctx context.Context, branch string) (*PRInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "list", "--head", branch, "--json", "number,url,state", "--limit", "1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout listing PRs for branch %s", branch)
		}
		return nil, fmt.Errorf("gh pr list failed: %s", stderr.String())
	}

	var prs []PRInfo
	if err := json.Unmarshal(stdout.Bytes(), &prs); err != nil {
		return nil, fmt.Errorf("failed to parse PR JSON: %w", err)
	}

	if len(prs) == 0 {
		return nil, nil // No PR found
	}

	return &prs[0], nil
}

// GetPRBaseBranch gets the base branch of a PR
func (c *GitHubClient) GetPRBaseBranch(ctx context.Context, prNumber int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "baseRefName", "-q", ".baseRefName")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout getting PR #%d base branch", prNumber)
		}
		return "", fmt.Errorf("gh pr view failed: %s", stderr.String())
	}

	baseBranch := strings.TrimSpace(stdout.String())
	return baseBranch, nil
}

// GetPRState gets the state of a PR (OPEN, CLOSED, MERGED)
func (c *GitHubClient) GetPRState(ctx context.Context, prNumber int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "state", "-q", ".state")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout getting PR #%d state", prNumber)
		}
		return "", fmt.Errorf("gh pr view failed: %s", stderr.String())
	}

	state := strings.TrimSpace(stdout.String())
	return state, nil
}

// IsPROpen checks if a PR is still open (not closed or merged)
func (c *GitHubClient) IsPROpen(ctx context.Context, prNumber int) (bool, error) {
	state, err := c.GetPRState(ctx, prNumber)
	if err != nil {
		return false, err
	}
	return state == "OPEN", nil
}

// GetPRMergeState gets the merge state status of a PR (DIRTY, BEHIND, BLOCKED, CLEAN, etc.)
// Note: Only meaningful for OPEN PRs. For CLOSED/MERGED PRs, the result may be unexpected.
func (c *GitHubClient) GetPRMergeState(ctx context.Context, prNumber int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "mergeStateStatus", "-q", ".mergeStateStatus")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout getting PR #%d merge state", prNumber)
		}
		return "", fmt.Errorf("gh pr view failed: %s", stderr.String())
	}

	mergeState := strings.TrimSpace(stdout.String())
	return mergeState, nil
}

// ExtractPRNumber extracts PR number from a GitHub PR URL.
// It validates that the URL is from github.com to prevent URL spoofing.
func ExtractPRNumber(prURL string) string {
	if prURL == "" {
		return ""
	}

	// Match pattern: https://github.com/owner/repo/pull/123
	// GitHub PR URLs always use "pull" (singular), not "pulls"
	// Validate the full URL structure to prevent spoofing from other domains
	re := regexp.MustCompile(`^https://github\.com/[^/]+/[^/]+/pull/(\d+)(?:\?|#|$)`)
	matches := re.FindStringSubmatch(prURL)
	if len(matches) >= 2 {
		return matches[1]
	}

	return ""
}

// HasLabel checks if an issue has a specific label
func (info *IssueInfo) HasLabel(label string) bool {
	for _, l := range info.Labels {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}

// IsOpen checks if an issue is open
func (info *IssueInfo) IsOpen() bool {
	return strings.ToUpper(info.State) == "OPEN"
}
