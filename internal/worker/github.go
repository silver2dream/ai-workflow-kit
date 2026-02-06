package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// GitHubClient provides GitHub operations via gh CLI
type GitHubClient struct {
	Timeout time.Duration
	Retry   ghutil.RetryConfig
}

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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "view", fmt.Sprintf("%d", number), "--json", "number,title,body,labels,state")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout fetching issue %d", number)
		}
		return nil, fmt.Errorf("gh issue view failed: %s", strings.TrimSpace(string(output)))
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

	if err := json.Unmarshal(output, &result); err != nil {
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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", args...)
	if err != nil {
		errMsg := ""
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("timeout editing issue %d labels", number)
		} else {
			errMsg = fmt.Sprintf("gh issue edit failed: %s", strings.TrimSpace(string(output)))
		}
		// Write label update failure event
		trace.WriteEvent(trace.ComponentGitHub, trace.TypeLabelUpdate, trace.LevelError,
			trace.WithIssue(number),
			trace.WithData(map[string]any{"add": add, "remove": remove}),
			trace.WithErrorString(errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Write label update success event
	trace.WriteEvent(trace.ComponentGitHub, trace.TypeLabelUpdate, trace.LevelInfo,
		trace.WithIssue(number),
		trace.WithData(map[string]any{"add": add, "remove": remove}))

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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "comment", fmt.Sprintf("%d", number), "--body", body)
	if err != nil {
		errMsg := ""
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("timeout commenting on issue %d", number)
		} else {
			errMsg = fmt.Sprintf("gh issue comment failed: %s", strings.TrimSpace(string(output)))
		}
		// Write comment failure event - this is critical for tracking
		trace.WriteEvent(trace.ComponentGitHub, trace.TypeCommentFail, trace.LevelError,
			trace.WithIssue(number),
			trace.WithErrorString(errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Write comment success event
	trace.WriteEvent(trace.ComponentGitHub, trace.TypeCommentSend, trace.LevelInfo,
		trace.WithIssue(number))

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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "pr", "list", "--head", branch, "--json", "number,url,state", "--limit", "1")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout listing PRs for branch %s", branch)
		}
		return nil, fmt.Errorf("gh pr list failed: %s", strings.TrimSpace(string(output)))
	}

	var prs []PRInfo
	if err := json.Unmarshal(output, &prs); err != nil {
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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "baseRefName", "-q", ".baseRefName")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout getting PR #%d base branch", prNumber)
		}
		return "", fmt.Errorf("gh pr view failed: %s", strings.TrimSpace(string(output)))
	}

	baseBranch := strings.TrimSpace(string(output))
	return baseBranch, nil
}

// GetPRState gets the state of a PR (OPEN, CLOSED, MERGED)
func (c *GitHubClient) GetPRState(ctx context.Context, prNumber int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "state", "-q", ".state")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout getting PR #%d state", prNumber)
		}
		return "", fmt.Errorf("gh pr view failed: %s", strings.TrimSpace(string(output)))
	}

	state := strings.TrimSpace(string(output))
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

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "mergeStateStatus", "-q", ".mergeStateStatus")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout getting PR #%d merge state", prNumber)
		}
		return "", fmt.Errorf("gh pr view failed: %s", strings.TrimSpace(string(output)))
	}

	mergeState := strings.TrimSpace(string(output))
	return mergeState, nil
}

// ExtractPRNumber extracts PR number from a GitHub PR URL or text containing PR references.
// It supports multiple formats:
// - Full URL: https://github.com/owner/repo/pull/123
// - Relative URL: /pull/123
// - PR reference: PR #123, PR#123, pull request #123
// Returns 0 if no PR number is found.
func ExtractPRNumber(body string) int {
	if body == "" {
		return 0
	}

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

// CloseIssue closes an issue
func (c *GitHubClient) CloseIssue(ctx context.Context, number int) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "close", strconv.Itoa(number))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout closing issue %d", number)
		}
		return fmt.Errorf("gh issue close failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
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

// IssueComment represents a comment on an issue
type IssueComment struct {
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
}

// GetIssueComments fetches the most recent comments on an issue
func (c *GitHubClient) GetIssueComments(ctx context.Context, number int, limit int) ([]IssueComment, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	if limit <= 0 {
		limit = 10
	}

	output, err := ghutil.RunWithRetry(ctx, c.Retry, "gh", "issue", "view", fmt.Sprintf("%d", number),
		"--json", "comments",
		"--jq", fmt.Sprintf(".comments | .[-%d:]", limit))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout fetching comments for issue %d", number)
		}
		return nil, fmt.Errorf("gh issue view failed: %s", strings.TrimSpace(string(output)))
	}

	var comments []IssueComment
	if err := json.Unmarshal(output, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse comments JSON: %w", err)
	}

	return comments, nil
}

// GetLatestReviewBlockedReason finds the most recent "AWK Review blocked" comment
// and extracts the failure reason and details.
func (c *GitHubClient) GetLatestReviewBlockedReason(ctx context.Context, issueNumber int) (string, error) {
	comments, err := c.GetIssueComments(ctx, issueNumber, 20)
	if err != nil {
		return "", err
	}

	// Search from newest to oldest for "AWK Review blocked" comment
	for i := len(comments) - 1; i >= 0; i-- {
		body := comments[i].Body
		if strings.Contains(body, "## AWK Review blocked") {
			// Extract the relevant content after the header
			lines := strings.Split(body, "\n")
			var result strings.Builder
			inCodeBlock := false

			for _, line := range lines {
				// Skip the header line
				if strings.HasPrefix(line, "## AWK Review blocked") {
					continue
				}
				// Skip the "已標記" and "當前 session" lines
				if strings.Contains(line, "已標記") || strings.Contains(line, "當前 session") {
					continue
				}
				// Track code blocks for test failure details
				if strings.HasPrefix(line, "```") {
					inCodeBlock = !inCodeBlock
				}
				// Include error message and details
				if strings.HasPrefix(line, "錯誤:") || strings.HasPrefix(line, "PR:") ||
					strings.HasPrefix(line, "審查驗證失敗") || inCodeBlock ||
					strings.HasPrefix(line, "- ") {
					result.WriteString(line)
					result.WriteString("\n")
				}
			}

			return strings.TrimSpace(result.String()), nil
		}
	}

	return "", nil // No review blocked comment found
}
