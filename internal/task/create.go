package task

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// CreateTaskOptions contains options for creating a task.
type CreateTaskOptions struct {
	SpecName  string
	TaskLine  int
	BodyFile  string
	Title     string // optional, will be auto-generated if empty
	Repo      string // optional, uses config if empty
	StateRoot string
	DryRun    bool
	GHTimeout time.Duration
}

// CreateTaskResult contains the result of creating a task.
type CreateTaskResult struct {
	IssueNumber int
	IssueURL    string
	Skipped     bool   // true if already has Issue ref
	DryRunCmd   string // populated if DryRun is true
}

// CreateTask creates a GitHub Issue from a tasks.md entry.
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

	// 6. Read and validate body file
	bodyPath := opts.BodyFile
	if !filepath.IsAbs(bodyPath) {
		cwd, _ := os.Getwd()
		bodyPath = filepath.Join(cwd, bodyPath)
	}
	bodyData, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, fmt.Errorf("body file not found: %s", bodyPath)
	}
	body := string(bodyData)

	if err := ValidateBody(body); err != nil {
		return nil, err
	}

	// 7. Inject AWK metadata
	body = EnsureAWKMetadata(body, opts.SpecName, opts.TaskLine)

	// 8. Write final body to temp file
	tempDir := filepath.Join(opts.StateRoot, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	finalBodyPath := filepath.Join(tempDir, fmt.Sprintf("create-task-%s-%d.md", opts.SpecName, opts.TaskLine))
	if err := os.WriteFile(finalBodyPath, []byte(body), 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp body file: %w", err)
	}

	// 9. Determine label and repo
	label := cfg.GitHub.Labels.Task
	if label == "" {
		label = "ai-task"
	}
	repo := opts.Repo
	if repo == "" {
		repo = cfg.GitHub.Repo
	}

	// 10. Build gh command
	ghArgs := []string{
		"issue", "create",
		"--title", title,
		"--body-file", finalBodyPath,
		"--label", label,
	}
	if repo != "" {
		ghArgs = append(ghArgs, "--repo", repo)
	}

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

	cmd := exec.CommandContext(ctx, "gh", ghArgs...)
	output, err := cmd.CombinedOutput()
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
		// Log warning but continue - the issue was created successfully
		fmt.Fprintf(os.Stderr, "warning: failed to commit tasks.md update: %v\n", err)
	}

	return &CreateTaskResult{
		IssueNumber: issueNumber,
		IssueURL:    issueURL,
	}, nil
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
