package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"gopkg.in/yaml.v3"
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
	ReviewDir          string `json:"review_dir"`
	WorktreePath       string `json:"worktree_path"`
	TestCommand        string `json:"test_command"`
	Language           string `json:"language"` // Programming language for test name validation
	Ticket             string `json:"ticket,omitempty"`    // Issue body with acceptance criteria
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

	// 4. Check worktree path - fail fast if worktree doesn't exist
	// This prevents pr-reviewer agent from attempting to cd to non-existent directory
	wtPath := filepath.Join(opts.StateRoot, ".worktrees", fmt.Sprintf("issue-%d", opts.IssueNumber))
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		rc.WorktreePath = wtPath
	} else {
		return nil, fmt.Errorf("worktree not found for issue #%d: expected at %s", opts.IssueNumber, wtPath)
	}

	// 5. Fetch issue details (ticket with acceptance criteria)
	rc.IssueJSON = fetchIssueJSON(ctx, opts.IssueNumber, opts.GHTimeout)
	rc.Ticket = extractIssueBody(rc.IssueJSON)

	// 6. Read task file
	taskPath := filepath.Join(opts.StateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", opts.IssueNumber), "prompt.txt")
	if content, err := os.ReadFile(taskPath); err == nil {
		rc.TaskContent = string(content)
	}

	// 7. Fetch commits
	rc.CommitsJSON = fetchPRCommits(ctx, opts.PRNumber, opts.GHTimeout)

	// 8. Get test command and language from workflow.yaml
	settings := getRepoSettingsFromConfig(opts.StateRoot, rc.WorktreePath)
	rc.TestCommand = settings.TestCommand
	rc.Language = settings.Language

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

// extractIssueBody extracts the body from issue JSON
func extractIssueBody(issueJSON string) string {
	if issueJSON == "" || strings.HasPrefix(issueJSON, "ERROR:") {
		return ""
	}

	var issue struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(issueJSON), &issue); err != nil {
		return ""
	}
	return issue.Body
}

// workflowConfig represents the workflow.yaml structure for test command extraction
type workflowConfig struct {
	Repos []repoConfig `yaml:"repos"`
}

type repoConfig struct {
	Name     string       `yaml:"name"`
	Path     string       `yaml:"path"`
	Type     string       `yaml:"type"`
	Language string       `yaml:"language"`
	Verify   verifyConfig `yaml:"verify"`
}

type verifyConfig struct {
	Test string `yaml:"test"`
}

// repoSettings holds test command and language from workflow.yaml
type repoSettings struct {
	TestCommand string
	Language    string
}

// getRepoSettingsFromConfig extracts test command and language from workflow.yaml
// For directory-type repos, test command returns "cd <path> && <test_command>"
func getRepoSettingsFromConfig(stateRoot, worktreePath string) repoSettings {
	configPath := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return repoSettings{TestCommand: "go test -v ./...", Language: "go"}
	}

	var cfg workflowConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return repoSettings{TestCommand: "go test -v ./...", Language: "go"}
	}

	// Try to match repo based on worktree path
	for _, repo := range cfg.Repos {
		if worktreePath != "" && worktreePath != "NOT_FOUND" {
			if strings.Contains(worktreePath, repo.Path) || strings.Contains(worktreePath, repo.Name) {
				return repoSettings{
					TestCommand: buildTestCommand(repo.Path, repo.Type, repo.Verify.Test),
					Language:    repo.Language,
				}
			}
		}
	}

	// Return first repo's settings if available
	for _, repo := range cfg.Repos {
		if repo.Verify.Test != "" {
			return repoSettings{
				TestCommand: buildTestCommand(repo.Path, repo.Type, repo.Verify.Test),
				Language:    repo.Language,
			}
		}
	}

	return repoSettings{TestCommand: "go test -v ./...", Language: "go"}
}

// getTestCommandFromConfig extracts test command from workflow.yaml
// For directory-type repos, returns "cd <path> && <test_command>"
// Deprecated: Use getRepoSettingsFromConfig instead for access to both test command and language
func getTestCommandFromConfig(stateRoot, worktreePath string) string {
	return getRepoSettingsFromConfig(stateRoot, worktreePath).TestCommand
}

// buildTestCommand constructs test command with proper directory handling
// For directory-type repos (path != "./" and path != ""), prepends "cd <path> &&"
func buildTestCommand(repoPath, repoType, testCmd string) string {
	// Normalize path
	path := strings.TrimSuffix(repoPath, "/")

	// If root repo or no path, return command as-is
	if path == "" || path == "." || repoType == "root" {
		return testCmd
	}

	// For directory or submodule repos, cd into the directory first
	return fmt.Sprintf("cd %s && %s", path, testCmd)
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
	sb.WriteString(fmt.Sprintf("WORKTREE_PATH: %s\n", rc.WorktreePath))
	sb.WriteString(fmt.Sprintf("TEST_COMMAND: %s\n", rc.TestCommand))
	sb.WriteString(fmt.Sprintf("LANGUAGE: %s\n", rc.Language))
	sb.WriteString("============================================================\n\n")

	// Ticket with acceptance criteria
	sb.WriteString(fmt.Sprintf("## TICKET (Issue #%d)\n\n", rc.IssueNumber))
	if rc.Ticket != "" {
		sb.WriteString(rc.Ticket)
	} else {
		sb.WriteString(rc.IssueJSON)
	}
	sb.WriteString("\n\n")

	// Task file
	if rc.TaskContent != "" {
		sb.WriteString("## TASK FILE\n\n")
		sb.WriteString(rc.TaskContent)
		sb.WriteString("\n\n")
	}

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
