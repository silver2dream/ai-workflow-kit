// Package task creates the tickets the workflow works on: GitHub issues that
// carry the task label and a body the Worker and the review can act on.
package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// CreateTaskOptions describes a ticket to create.
type CreateTaskOptions struct {
	Title     string // the issue title, e.g. "[feat] add login"
	BodyFile  string // the ticket body, a markdown file
	Repo      string // GitHub repository (owner/name); the config's when empty
	StateRoot string
	DryRun    bool
	GHTimeout time.Duration
	// Unlabeled creates the issue without the task label: it is no ticket,
	// and no run takes it up, until Label puts the label on. An approval
	// that brings the plan's documents to the integration branch first does
	// so, so that no Worker starts before they are there.
	Unlabeled bool
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
// hold what a ticket needs (see ValidateBody), and its awk-ticket block must
// name a repo of the config; the issue gets the task label, which is what
// makes it a ticket.
func CreateTask(ctx context.Context, opts CreateTaskOptions) (*CreateTaskResult, error) {
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		return nil, fmt.Errorf("a title is required")
	}

	bodyPath, block, err := checkedBodyFile(opts.BodyFile)
	if err != nil {
		return nil, err
	}

	configPath := config.Path(opts.StateRoot)
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		cfg = &analyzer.Config{}
	} else if !cfg.HasTicketRepo(block.Repo) {
		// The Worker would refuse the ticket; better not to create it.
		return nil, fmt.Errorf("ticket body: the awk-ticket block names repo %q, which is not a repo of "+layout.ConfigFile+" (it has: %s)",
			block.Repo, strings.Join(cfg.TicketRepos(), ", "))
	}
	if err := block.CheckRepo(cfg.TicketRepoDir(block.Repo)); err != nil {
		return nil, fmt.Errorf("ticket body: the awk-ticket block is not valid: %w", err)
	}
	label, repo := resolveGHParams(opts, cfg)
	if opts.Unlabeled {
		label = ""
	}
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
// validation, and the body's awk-ticket block.
func checkedBodyFile(bodyFile string) (string, ticket.Meta, error) {
	if bodyFile == "" {
		return "", ticket.Meta{}, fmt.Errorf("a body file is required")
	}
	bodyPath, err := filepath.Abs(bodyFile)
	if err != nil {
		return "", ticket.Meta{}, fmt.Errorf("body file %s: %w", bodyFile, err)
	}
	data, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", ticket.Meta{}, fmt.Errorf("body file not found: %s", bodyPath)
	}
	block, err := validatedBlock(string(data))
	if err != nil {
		return "", ticket.Meta{}, err
	}
	return bodyPath, block, nil
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
	}
	if label != "" {
		args = append(args, "--label", label)
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return args
}

// Label puts the task label on issues, which makes each a ticket a run takes
// up. An issue that has it already keeps it.
func Label(ctx context.Context, stateRoot string, issues []int) error {
	cfg, err := analyzer.LoadConfig(config.Path(stateRoot))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		cfg = &analyzer.Config{}
	}
	label, repo := resolveGHParams(CreateTaskOptions{}, cfg)
	if len(issues) == 0 {
		return nil
	}
	// A repository that never had a run has no task label yet, and gh will
	// not put a label on that does not exist. One that is there is left as
	// it is.
	create := []string{"label", "create", label, "--color", "1d76db", "--description", "AWK automated task"}
	if repo != "" {
		create = append(create, "--repo", repo)
	}
	call, cancel := context.WithTimeout(ctx, 60*time.Second)
	if output, err := runGH(call, create...); err != nil && !strings.Contains(strings.ToLower(err.Error()+string(output)), "already exists") {
		cancel()
		return fmt.Errorf("the %s label cannot be made: %s\n%s", label, err, string(output))
	}
	cancel()
	for _, issue := range issues {
		args := []string{"issue", "edit", strconv.Itoa(issue), "--add-label", label}
		if repo != "" {
			args = append(args, "--repo", repo)
		}
		call, cancel := context.WithTimeout(ctx, 60*time.Second)
		output, err := runGH(call, args...)
		cancel()
		if err != nil {
			return fmt.Errorf("issue #%d cannot be given the %s label: %s\n%s", issue, label, err, string(output))
		}
	}
	return nil
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
