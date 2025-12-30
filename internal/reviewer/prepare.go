package reviewer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
)

// PrepareReviewOptions configures the prepare review operation
type PrepareReviewOptions struct {
	PRNumber    int
	IssueNumber int
	StateRoot   string
	GHTimeout   time.Duration
}

// ReviewContext holds all information needed for PR review
type ReviewContext struct {
	PRNumber           int    `json:"pr_number"`
	IssueNumber        int    `json:"issue_number"`
	PrincipalSessionID string `json:"principal_session_id"`
	CIStatus           string `json:"ci_status"` // "passed" | "failed"
	DiffHash           string `json:"diff_hash"`
	DiffBytes          int64  `json:"diff_bytes"`
	ReviewDir          string `json:"review_dir"`
	WorktreePath       string `json:"worktree_path"`
	Diff               string `json:"diff,omitempty"`
	IssueJSON          string `json:"issue_json,omitempty"`
	TaskContent        string `json:"task_content,omitempty"`
	CommitsJSON        string `json:"commits_json,omitempty"`
}

// CommitInfo represents a single commit
type CommitInfo struct {
	OID             string `json:"oid"`
	MessageHeadline string `json:"messageHeadline"`
}

// PrepareReview collects all information needed for PR review
func PrepareReview(ctx context.Context, opts PrepareReviewOptions) (*ReviewContext, error) {
	if opts.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}

	rc := &ReviewContext{
		PRNumber:    opts.PRNumber,
		IssueNumber: opts.IssueNumber,
	}

	// 1. Get session ID
	sessionMgr := session.NewManager(opts.StateRoot)
	rc.PrincipalSessionID = sessionMgr.GetCurrentSessionID()
	if rc.PrincipalSessionID == "" {
		rc.PrincipalSessionID = "unknown"
	}

	// 2. Check CI status
	rc.CIStatus = getCIStatus(ctx, opts.PRNumber, opts.GHTimeout)

	// 3. Prepare review directory
	rc.ReviewDir = filepath.Join(opts.StateRoot, ".ai", "state", "reviews", fmt.Sprintf("pr-%d", opts.PRNumber))
	if err := os.MkdirAll(rc.ReviewDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create review directory: %w", err)
	}

	// 4. Fetch and save diff
	diffPath := filepath.Join(rc.ReviewDir, "diff.patch")
	diff, err := fetchPRDiff(ctx, opts.PRNumber, opts.GHTimeout)
	if err != nil {
		rc.DiffHash = "unavailable"
		rc.DiffBytes = 0
	} else {
		// Write diff atomically
		tmpPath := diffPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(diff), 0644); err == nil {
			_ = os.Rename(tmpPath, diffPath)
		}
		rc.Diff = diff
		rc.DiffBytes = int64(len(diff))
		if len(diff) == 0 {
			rc.DiffHash = "empty"
		} else {
			rc.DiffHash = sha256_16(diff)
		}
	}

	// 5. Check worktree path
	wtPath := filepath.Join(opts.StateRoot, ".worktrees", fmt.Sprintf("issue-%d", opts.IssueNumber))
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		rc.WorktreePath = wtPath
	} else {
		rc.WorktreePath = "NOT_FOUND"
	}

	// 6. Fetch issue details
	rc.IssueJSON = fetchIssueJSON(ctx, opts.IssueNumber, opts.GHTimeout)

	// 7. Read task file
	taskPath := filepath.Join(opts.StateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", opts.IssueNumber), "prompt.txt")
	if content, err := os.ReadFile(taskPath); err == nil {
		rc.TaskContent = string(content)
	}

	// 8. Fetch commits
	rc.CommitsJSON = fetchPRCommits(ctx, opts.PRNumber, opts.GHTimeout)

	return rc, nil
}

// getCIStatus checks PR CI status via gh cli
func getCIStatus(ctx context.Context, prNumber int, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try --json first (gh >= 2.12)
	cmd := exec.CommandContext(ctx, "gh", "pr", "checks", fmt.Sprintf("%d", prNumber),
		"--json", "state", "--jq", ".[].state")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		if strings.Contains(string(output), "FAILURE") {
			return "failed"
		}
		return "passed"
	}

	// Fallback for older gh versions
	cmd = exec.CommandContext(ctx, "gh", "pr", "checks", fmt.Sprintf("%d", prNumber))
	output, err = cmd.Output()
	if err == nil {
		if strings.Contains(strings.ToLower(string(output)), "fail") {
			return "failed"
		}
	}

	return "passed"
}

// fetchPRDiff fetches the PR diff
func fetchPRDiff(ctx context.Context, prNumber int, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "diff", fmt.Sprintf("%d", prNumber))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to fetch diff: %w", err)
	}
	return string(output), nil
}

// fetchIssueJSON fetches issue details as JSON
func fetchIssueJSON(ctx context.Context, issueNumber int, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "issue", "view", fmt.Sprintf("%d", issueNumber),
		"--json", "title,body,labels")
	output, err := cmd.Output()
	if err != nil {
		return "ERROR: Cannot fetch issue"
	}
	return string(output)
}

// fetchPRCommits fetches PR commits
func fetchPRCommits(ctx context.Context, prNumber int, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprintf("%d", prNumber),
		"--json", "commits", "--jq", `.commits[] | "- \(.oid[0:7]) \(.messageHeadline)"`)
	output, err := cmd.Output()
	if err != nil {
		return "ERROR: Cannot fetch commits"
	}
	return string(output)
}

// sha256_16 returns the first 16 characters of the SHA256 hash
func sha256_16(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:16]
}

// FormatOutput generates the standardized output format
func (rc *ReviewContext) FormatOutput() string {
	var sb strings.Builder

	// Header
	sb.WriteString("============================================================\n")
	sb.WriteString("AWK PR REVIEW CONTEXT\n")
	sb.WriteString("============================================================\n")
	sb.WriteString(fmt.Sprintf("PR_NUMBER: %d\n", rc.PRNumber))
	sb.WriteString(fmt.Sprintf("ISSUE_NUMBER: %d\n", rc.IssueNumber))
	sb.WriteString(fmt.Sprintf("PRINCIPAL_SESSION_ID: %s\n", rc.PrincipalSessionID))
	sb.WriteString(fmt.Sprintf("CI_STATUS: %s\n", rc.CIStatus))
	sb.WriteString(fmt.Sprintf("DIFF_HASH: %s\n", rc.DiffHash))
	sb.WriteString(fmt.Sprintf("DIFF_BYTES: %d\n", rc.DiffBytes))
	sb.WriteString(fmt.Sprintf("REVIEW_DIR: %s\n", rc.ReviewDir))
	sb.WriteString(fmt.Sprintf("WORKTREE_PATH: %s\n", rc.WorktreePath))
	sb.WriteString("============================================================\n\n")

	// Issue content
	sb.WriteString(fmt.Sprintf("## TICKET REQUIREMENTS (Issue #%d)\n\n", rc.IssueNumber))
	sb.WriteString(rc.IssueJSON)
	sb.WriteString("\n\n")

	// Task file
	if rc.TaskContent != "" {
		sb.WriteString("## TASK FILE\n\n")
		sb.WriteString(rc.TaskContent)
		sb.WriteString("\n\n")
	}

	// PR Diff
	sb.WriteString("============================================================\n")
	sb.WriteString("## PR DIFF\n")
	sb.WriteString("============================================================\n\n")
	if rc.Diff != "" {
		sb.WriteString(rc.Diff)
	} else {
		sb.WriteString("ERROR: Diff not available\n")
	}
	sb.WriteString("\n")

	// PR Commits
	sb.WriteString("============================================================\n")
	sb.WriteString("## PR COMMITS\n")
	sb.WriteString("============================================================\n\n")
	sb.WriteString(rc.CommitsJSON)
	sb.WriteString("\n")

	sb.WriteString("============================================================\n")
	sb.WriteString("END OF REVIEW CONTEXT\n")
	sb.WriteString("============================================================\n")

	return sb.String()
}

// ToJSON returns the context as JSON
func (rc *ReviewContext) ToJSON() (string, error) {
	data, err := json.MarshalIndent(rc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
