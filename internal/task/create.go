// Package task creates the tickets the workflow works on: GitHub issues that
// carry the task label and a body the Worker and the review can act on.
package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

// CreateTaskOptions describes a ticket to create.
type CreateTaskOptions struct {
	Title     string // the issue title, e.g. "[feat] add login"
	BodyFile  string // the ticket body, a markdown file
	Repo      string // GitHub repository (owner/name); the config's when empty
	StateRoot string
	DryRun    bool
	GHTimeout time.Duration
}

// CreateTaskResult contains the result of creating a ticket.
type CreateTaskResult struct {
	IssueNumber int
	IssueURL    string
	DryRunCmd   string // populated if DryRun is true
}

// runGH runs the gh CLI. Replaced in tests.
var runGH = func(ctx context.Context, args ...string) ([]byte, error) {
	return ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", args...)
}

// CreateTask creates a GitHub issue the workflow will pick up. The body must
// hold the sections a ticket needs (see ValidateBody); the issue gets the
// task label, which is what makes it a ticket.
func CreateTask(ctx context.Context, opts CreateTaskOptions) (*CreateTaskResult, error) {
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		return nil, fmt.Errorf("a title is required")
	}

	bodyPath, err := checkedBodyFile(opts.BodyFile)
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(opts.StateRoot, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		cfg = &analyzer.Config{}
	}
	label, repo := resolveGHParams(opts, cfg)
	ghArgs := buildGHCreateArgs(title, bodyPath, label, repo)

	if opts.DryRun {
		return &CreateTaskResult{DryRunCmd: "gh " + strings.Join(ghArgs, " ")}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.GHTimeout)
	defer cancel()
	output, err := runGH(ctx, ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh issue create failed: %s\n%s", err, string(output))
	}
	issueNumber, issueURL, err := parseIssueOutput(string(output))
	if err != nil {
		return nil, err
	}
	return &CreateTaskResult{IssueNumber: issueNumber, IssueURL: issueURL}, nil
}

// checkedBodyFile returns the absolute path of a ticket body that passed
// validation.
func checkedBodyFile(bodyFile string) (string, error) {
	if bodyFile == "" {
		return "", fmt.Errorf("a body file is required")
	}
	bodyPath, err := filepath.Abs(bodyFile)
	if err != nil {
		return "", fmt.Errorf("body file %s: %w", bodyFile, err)
	}
	data, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", fmt.Errorf("body file not found: %s", bodyPath)
	}
	if err := ValidateBody(string(data)); err != nil {
		return "", err
	}
	return bodyPath, nil
}

// resolveGHParams determines the label and repo for gh commands.
func resolveGHParams(opts CreateTaskOptions, cfg *analyzer.Config) (label, repo string) {
	label = cfg.GitHub.Labels.Task
	if label == "" {
		label = "ai-task"
	}
	repo = opts.Repo
	if repo == "" {
		repo = cfg.GitHub.Repo
	}
	return
}

// buildGHCreateArgs builds the arguments for gh issue create.
func buildGHCreateArgs(title, bodyFile, label, repo string) []string {
	args := []string{
		"issue", "create",
		"--title", title,
		"--body-file", bodyFile,
		"--label", label,
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return args
}

func parseIssueOutput(output string) (int, string, error) {
	// Match /issues/123 pattern
	re := regexp.MustCompile(`/issues/(\d+)\b`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, "", fmt.Errorf("failed to parse issue number from gh output: %s", strings.TrimSpace(output))
	}

	var num int
	fmt.Sscanf(matches[1], "%d", &num)

	// Try to extract full URL
	urlRe := regexp.MustCompile(`https://github\.com/[^\s]+/issues/\d+`)
	urlMatch := urlRe.FindString(output)

	return num, urlMatch, nil
}
