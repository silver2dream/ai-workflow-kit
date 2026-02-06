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

	"github.com/silver2dream/ai-workflow-kit/internal/repo"
	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/util"
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

	// 4. Check worktree path - auto-rebuild if missing
	wtPath := filepath.Join(opts.StateRoot, ".worktrees", fmt.Sprintf("issue-%d", opts.IssueNumber))
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		rc.WorktreePath = wtPath
	} else {
		// Worktree missing - try to rebuild from PR branch
		rebuilt, rebuildErr := rebuildWorktree(ctx, opts.StateRoot, opts.IssueNumber, opts.PRNumber, opts.GHTimeout)
		if rebuildErr != nil {
			return nil, fmt.Errorf("worktree not found for issue #%d and auto-rebuild failed: %w", opts.IssueNumber, rebuildErr)
		}
		rc.WorktreePath = rebuilt
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
// Returns "passed", "failed", or "unknown" (when CI status cannot be determined)
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
		lower := strings.ToLower(string(output))
		if strings.Contains(lower, "fail") {
			return "failed"
		}
		if strings.Contains(lower, "pass") {
			return "passed"
		}
	}

	// Cannot determine CI status - fail-safe: do not assume passed
	return "unknown"
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

// reviewConfig represents the review section in workflow.yaml
type reviewConfig struct {
	ScoreThreshold int    `yaml:"score_threshold"`
	MergeStrategy  string `yaml:"merge_strategy"`
}

// workflowConfig represents the workflow.yaml structure for test command extraction
type workflowConfig struct {
	Repos  []repoConfig `yaml:"repos"`
	Review reviewConfig `yaml:"review"`
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

// ReviewSettings holds review configuration loaded from workflow.yaml
type ReviewSettings struct {
	ScoreThreshold int
	MergeStrategy  string
}

// GetReviewSettings loads review config from workflow.yaml and applies defaults.
// Returns default values (score_threshold=7, merge_strategy="squash") if config cannot be read.
func GetReviewSettings(stateRoot string) ReviewSettings {
	defaults := ReviewSettings{
		ScoreThreshold: 7,
		MergeStrategy:  "squash",
	}

	configPath := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return defaults
	}

	var cfg workflowConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return defaults
	}

	result := defaults
	if cfg.Review.ScoreThreshold > 0 {
		result.ScoreThreshold = cfg.Review.ScoreThreshold
	}
	if cfg.Review.MergeStrategy != "" {
		switch cfg.Review.MergeStrategy {
		case "squash", "merge", "rebase":
			result.MergeStrategy = cfg.Review.MergeStrategy
		default:
			// Invalid strategy, keep default
		}
	}

	return result
}

// repoSettings holds test command and language from workflow.yaml
type repoSettings struct {
	TestCommand string
	Language    string
}

// getRepoSettingsFromConfig extracts test command and language from workflow.yaml
// For directory-type repos, test command returns "cd <path> && <test_command>"
// Returns empty settings (TestCommand: "", Language: "unknown") if config cannot be read
// or no matching repo is found, allowing callers to handle the fallback appropriately.
func getRepoSettingsFromConfig(stateRoot, worktreePath string) repoSettings {
	configPath := filepath.Join(stateRoot, ".ai", "config", "workflow.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		// Return empty/unknown settings instead of assuming Go project
		return repoSettings{TestCommand: "", Language: "unknown"}
	}

	var cfg workflowConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		// Return empty/unknown settings instead of assuming Go project
		return repoSettings{TestCommand: "", Language: "unknown"}
	}

	// Convert to repo.Config and use unified RepoResolver
	repoConfigs := make([]repo.Config, len(cfg.Repos))
	for i, r := range cfg.Repos {
		repoConfigs[i] = repo.Config{
			Name:     r.Name,
			Path:     r.Path,
			Type:     r.Type,
			Language: r.Language,
			Verify:   repo.VerifyConfig{Test: r.Verify.Test},
		}
	}

	resolver := repo.NewResolver(repoConfigs)
	matched := resolver.FindByWorktreePath(worktreePath)

	if matched != nil {
		return repoSettings{
			TestCommand: buildTestCommand(matched.Path, matched.Type, matched.Verify.Test),
			Language:    matched.Language,
		}
	}

	// No match found - return unknown (fail explicitly rather than wrong guess)
	return repoSettings{TestCommand: "", Language: "unknown"}
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
	// If root repo or root path, return command as-is
	if repoType == "root" || util.IsRootPath(repoPath) {
		return testCmd
	}

	// Normalize path for the cd command
	path := util.NormalizePath(repoPath)

	// For directory or submodule repos, cd into the directory first
	return fmt.Sprintf("cd %s && %s", path, testCmd)
}

// rebuildWorktree attempts to recreate a missing worktree from the PR's head branch
func rebuildWorktree(ctx context.Context, stateRoot string, issueNumber, prNumber int, timeout time.Duration) (string, error) {
	wtPath := filepath.Join(stateRoot, ".worktrees", fmt.Sprintf("issue-%d", issueNumber))

	// First, fetch latest refs
	fetchCtx, fetchCancel := context.WithTimeout(ctx, timeout)
	defer fetchCancel()
	fetchCmd := exec.CommandContext(fetchCtx, "git", "fetch", "--all")
	fetchCmd.Dir = stateRoot
	_ = fetchCmd.Run() // best-effort

	// Try branch name convention: feat/ai-issue-{N}
	branchName := fmt.Sprintf("feat/ai-issue-%d", issueNumber)

	// Try to add worktree from the branch
	addCtx, addCancel := context.WithTimeout(ctx, timeout)
	defer addCancel()
	addCmd := exec.CommandContext(addCtx, "git", "worktree", "add", wtPath, branchName)
	addCmd.Dir = stateRoot
	if err := addCmd.Run(); err != nil {
		// Try remote tracking branch
		remoteBranch := fmt.Sprintf("origin/%s", branchName)
		addCtx2, addCancel2 := context.WithTimeout(ctx, timeout)
		defer addCancel2()
		addCmd2 := exec.CommandContext(addCtx2, "git", "worktree", "add", wtPath, remoteBranch)
		addCmd2.Dir = stateRoot
		if err2 := addCmd2.Run(); err2 != nil {
			return "", fmt.Errorf("failed to rebuild worktree from branch %s or %s: %v / %v", branchName, remoteBranch, err, err2)
		}
	}

	// Verify the worktree was created
	if info, err := os.Stat(wtPath); err != nil || !info.IsDir() {
		return "", fmt.Errorf("worktree rebuild succeeded but directory not found at %s", wtPath)
	}

	fmt.Fprintf(os.Stderr, "[REVIEW] auto-rebuilt worktree at %s from branch %s\n", wtPath, branchName)
	return wtPath, nil
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
