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

// CreateTaskOptions contains options for creating a task.
type CreateTaskOptions struct {
	SpecName     string
	TaskLine     int
	BodyFile     string
	Title        string // optional, will be auto-generated if empty
	Repo         string // optional, uses config if empty
	StateRoot    string
	DryRun       bool
	GHTimeout    time.Duration
	TrackingMode string // "tasks_md" (default) | "github_epic"
	EpicIssue    int    // tracking issue number (epic mode only)
	TaskText     string // task text from epic body (epic mode only)
}

// CreateTaskResult contains the result of creating a task.
type CreateTaskResult struct {
	IssueNumber int
	IssueURL    string
	Skipped     bool   // true if already has Issue ref
	DryRunCmd   string // populated if DryRun is true
}

// CreateTask creates a GitHub Issue from a tasks.md entry or epic body task.
func CreateTask(ctx context.Context, opts CreateTaskOptions) (*CreateTaskResult, error) {
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}

	// 1. Load config
	configPath := filepath.Join(opts.StateRoot, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		cfg = &analyzer.Config{}
		cfg.Specs.BasePath = ".ai/specs"
		cfg.GitHub.Labels.Task = "ai-task"
	}

	// Determine tracking mode
	trackingMode := opts.TrackingMode
	if trackingMode == "" {
		trackingMode = cfg.Specs.Tracking.Mode
	}
	if trackingMode == "" {
		trackingMode = analyzer.TrackingModeTasksMd
	}

	if trackingMode == analyzer.TrackingModeGitHubEpic {
		return createTaskEpicMode(ctx, opts, cfg)
	}
	return createTaskFileMode(ctx, opts, cfg)
}

// createTaskFileMode is the original tasks.md-based task creation flow.
func createTaskFileMode(ctx context.Context, opts CreateTaskOptions, cfg *analyzer.Config) (*CreateTaskResult, error) {
	// 2. Find tasks.md
	tasksPath := filepath.Join(opts.StateRoot, cfg.Specs.BasePath, opts.SpecName, "tasks.md")
	if _, err := os.Stat(tasksPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("tasks.md not found: %s", tasksPath)
	}

	// 3. Read task line
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks.md: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	if opts.TaskLine < 1 || opts.TaskLine > len(lines) {
		return nil, fmt.Errorf("task_line %d out of range (file has %d lines)", opts.TaskLine, len(lines))
	}

	rawTaskLine := lines[opts.TaskLine-1]

	// 4. Check idempotency (already has <!-- Issue #N -->)
	if num, found := HasIssueRef(rawTaskLine); found {
		return &CreateTaskResult{
			IssueNumber: num,
			Skipped:     true,
		}, nil
	}

	// 5. Extract task text and generate title
	taskText := ExtractTaskText(rawTaskLine)
	title := opts.Title
	if title == "" {
		title = DefaultIssueTitle(taskText)
	}

	// 6-8. Prepare body
	finalBodyPath, err := prepareBodyFile(opts, cfg)
	if err != nil {
		return nil, err
	}

	// 9. Determine label and repo
	label, repo := resolveGHParams(opts, cfg)

	// 10. Build gh command
	ghArgs := buildGHCreateArgs(title, finalBodyPath, label, repo)

	// 11. Dry run mode
	if opts.DryRun {
		cmdStr := "gh " + strings.Join(ghArgs, " ")
		return &CreateTaskResult{
			DryRunCmd: cmdStr,
		}, nil
	}

	// 12. Execute gh issue create
	ctx, cancel := context.WithTimeout(ctx, opts.GHTimeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh issue create failed: %s\n%s", err, string(output))
	}

	// 13. Parse issue number from output
	issueNumber, issueURL, err := parseIssueOutput(string(output))
	if err != nil {
		return nil, err
	}

	// 14. Append issue ref to tasks.md
	if err := AppendIssueRef(tasksPath, opts.TaskLine, issueNumber); err != nil {
		return nil, fmt.Errorf("failed to update tasks.md: %w", err)
	}

	// 15. Commit tasks.md update (best-effort, don't fail on error)
	if err := CommitTasksUpdate(tasksPath, issueNumber, "linked"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to commit tasks.md update: %v\n", err)
	}

	return &CreateTaskResult{
		IssueNumber: issueNumber,
		IssueURL:    issueURL,
	}, nil
}

// createTaskEpicMode creates an issue and links it in the epic body.
func createTaskEpicMode(ctx context.Context, opts CreateTaskOptions, cfg *analyzer.Config) (*CreateTaskResult, error) {
	epicIssue := opts.EpicIssue
	if epicIssue == 0 {
		epicIssue = cfg.GetEpicIssue(opts.SpecName)
	}
	if epicIssue == 0 {
		return nil, fmt.Errorf("no epic issue configured for spec %q", opts.SpecName)
	}

	// Generate title from TaskText
	title := opts.Title
	if title == "" && opts.TaskText != "" {
		title = DefaultIssueTitle(opts.TaskText)
	}
	if title == "" {
		title = "[feat] implement task"
	}

	// Prepare body
	finalBodyPath, err := prepareBodyFile(opts, cfg)
	if err != nil {
		return nil, err
	}

	// Determine label and repo
	label, repo := resolveGHParams(opts, cfg)

	// Build gh command
	ghArgs := buildGHCreateArgs(title, finalBodyPath, label, repo)

	// Dry run mode
	if opts.DryRun {
		cmdStr := "gh " + strings.Join(ghArgs, " ")
		return &CreateTaskResult{
			DryRunCmd: cmdStr,
		}, nil
	}

	// Execute gh issue create
	createCtx, cancel := context.WithTimeout(ctx, opts.GHTimeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(createCtx, ghutil.DefaultRetryConfig(), "gh", ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh issue create failed: %s\n%s", err, string(output))
	}

	issueNumber, issueURL, err := parseIssueOutput(string(output))
	if err != nil {
		return nil, err
	}

	// Link in epic body: read → append → write
	if err := linkIssueToEpic(ctx, epicIssue, issueNumber, opts.TaskText, repo, opts.GHTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "warning: issue #%d created but failed to link to epic #%d: %v\n", issueNumber, epicIssue, err)
	}

	return &CreateTaskResult{
		IssueNumber: issueNumber,
		IssueURL:    issueURL,
	}, nil
}

// prepareBodyFile reads, validates, and writes the body to a temp file.
func prepareBodyFile(opts CreateTaskOptions, cfg *analyzer.Config) (string, error) {
	bodyPath := opts.BodyFile
	if bodyPath == "" {
		return "", fmt.Errorf("body file is required")
	}
	if !filepath.IsAbs(bodyPath) {
		cwd, _ := os.Getwd()
		bodyPath = filepath.Join(cwd, bodyPath)
	}
	bodyData, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", fmt.Errorf("body file not found: %s", bodyPath)
	}
	body := string(bodyData)

	if err := ValidateBody(body); err != nil {
		return "", err
	}

	body = EnsureAWKMetadata(body, opts.SpecName, opts.TaskLine)

	tempDir := filepath.Join(opts.StateRoot, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	finalBodyPath := filepath.Join(tempDir, fmt.Sprintf("create-task-%s-%d.md", opts.SpecName, opts.TaskLine))
	if err := os.WriteFile(finalBodyPath, []byte(body), 0644); err != nil {
		return "", fmt.Errorf("failed to write temp body file: %w", err)
	}

	return finalBodyPath, nil
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

// linkIssueToEpic reads the epic body, appends a task reference, and updates it.
func linkIssueToEpic(ctx context.Context, epicIssue, issueNumber int, description, repo string, timeout time.Duration) error {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Read current epic body
	readArgs := []string{"issue", "view", fmt.Sprintf("%d", epicIssue), "--json", "body", "--jq", ".body"}
	if repo != "" {
		readArgs = append(readArgs, "--repo", repo)
	}
	output, err := ghutil.RunWithRetry(readCtx, ghutil.DefaultRetryConfig(), "gh", readArgs...)
	if err != nil {
		return fmt.Errorf("failed to read epic #%d: %w", epicIssue, err)
	}
	epicBody := strings.TrimSpace(string(output))

	// Append task reference
	newBody := analyzer.AppendTaskToEpicBody(epicBody, issueNumber, description)

	// Write updated body via temp file
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("awkit-epic-update-%d.md", epicIssue))
	if err := os.WriteFile(tmpFile, []byte(newBody), 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	defer os.Remove(tmpFile)

	writeCtx, writeCancel := context.WithTimeout(ctx, timeout)
	defer writeCancel()

	writeArgs := []string{"issue", "edit", fmt.Sprintf("%d", epicIssue), "--body-file", tmpFile}
	if repo != "" {
		writeArgs = append(writeArgs, "--repo", repo)
	}
	if _, err := ghutil.RunWithRetry(writeCtx, ghutil.DefaultRetryConfig(), "gh", writeArgs...); err != nil {
		return fmt.Errorf("failed to update epic #%d: %w", epicIssue, err)
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
